// Package store keeps uploaded files on disk and tracks how much of each file
// has arrived, which is what makes an interrupted upload resumable.
//
// Every file owns three paths under the store directory: "<id>.json" holds the
// metadata, "<id>.part" the bytes of an incomplete upload and "<id>.bin" the
// bytes of a complete one. How many bytes arrived is never written to the
// metadata: the size of the part file cannot disagree with the data,
// and [New] reads it back after a restart.
//
// The uploaded name never reaches a path. Both blobs are named after the
// identifier the store mints, which leaves "../../etc/passwd" a string this
// package stores and nothing else.
package store

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// MaxNameLen is the longest file name the store accepts, in bytes.
const MaxNameLen = 255

var (
	ErrNotFound     = errors.New("file not found")
	ErrInvalidName  = errors.New("invalid file name")
	ErrInvalidSize  = errors.New("invalid file size")
	ErrNotUploading = errors.New("file is not uploading")
	ErrGap          = errors.New("offset is past the received bytes")
	ErrIncomplete   = errors.New("file is incomplete")
)

// errComplete refuses a change once every announced byte arrived.
var errComplete = fmt.Errorf("%w: file is complete", ErrNotUploading)

// Status is where a file stands.
// A file that holds every announced byte is complete and cannot leave that state.
type Status string

const (
	StatusUploading Status = "uploading"
	StatusPaused    Status = "paused"
	StatusComplete  Status = "complete"
)

// File is one uploaded file.
type File struct {
	ID   string `json:"id"`
	Name string `json:"name"`

	// OriginalName is what the browser called the file. A visitor renaming a
	// file before it starts is why a transfer taken over in another session
	// is matched against this name and not against Name.
	OriginalName string `json:"originalName"`

	ContentType string    `json:"contentType"`
	Size        int64     `json:"size"`
	Status      Status    `json:"status"`
	CreatedAt   time.Time `json:"createdAt"`

	// Received is derived from the size of the blob on disk.
	Received int64 `json:"-"`

	// Owner names the browser tab holding the bytes that are still missing,
	// and is empty when nobody does. Only that tab can go on with the transfer,
	// hence only it may pause or resume one. A restart and a closed
	// tab both leave it empty, which is why it is not written to disk.
	Owner string `json:"-"`
}

// Percent is how much of the file arrived, rounded down.
// A file of no bytes has them all.
func (f File) Percent() int {
	if f.Size <= 0 {
		return 100
	}
	return int(f.Received * 100 / f.Size)
}

// Complete reports whether every announced byte arrived.
func (f File) Complete() bool { return f.Status == StatusComplete }

// entry is one file and two locks. They are two because a chunk can take a long time,
// at the speed of the client and of whatever limit the server puts on it:
// listing the files must not wait for that, and the bytes of a chunk
// have to show up while they arrive rather than when the request ends.
type entry struct {
	tx sync.Mutex // Held for a whole chunk.

	lock sync.Mutex // Held briefly, for file and gone.
	file File
	gone bool
}

// snapshot is the file as it stands, and whether it still exists.
func (e *entry) snapshot() (File, bool) {
	e.lock.Lock()
	defer e.lock.Unlock()
	return e.file, !e.gone
}

// advance records n more received bytes and reports whether the transfer may go on,
// so that a file paused or deleted during a chunk ends it at the next
// write rather than at the end of the request.
func (e *entry) advance(n int64) error {
	e.lock.Lock()
	defer e.lock.Unlock()
	switch {
	case e.gone:
		return ErrNotFound
	case e.file.Status != StatusUploading:
		return fmt.Errorf("%w: %s", ErrNotUploading, e.file.Status)
	}
	e.file.Received += n
	return nil
}

// blobWriter writes to the blob and records what arrived as it goes.
type blobWriter struct {
	blob  *os.File
	entry *entry
}

func (w *blobWriter) Write(p []byte) (int, error) {
	n, err := w.blob.Write(p)
	if n > 0 {
		if errAdvance := w.entry.advance(int64(n)); errAdvance != nil && err == nil {
			err = errAdvance
		}
	}
	return n, err
}

// Store is a directory of uploaded files. It is safe for concurrent use.
type Store struct {
	dir string

	lock    sync.Mutex
	entries map[string]*entry
}

// New opens the store in dir, creating the directory when it is missing,
// and reads back the files of an earlier run. An incomplete upload whose
// blob disappeared resumes from zero; a complete one is forgotten.
func New(dir string) (*Store, error) {
	// Absolute from here on: the process may change its working directory,
	// and a relative path would then name somewhere else.
	dir, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("resolving store directory: %w", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating store directory: %w", err)
	}
	s := &Store{dir: dir, entries: make(map[string]*entry)}
	metaFiles, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, fmt.Errorf("listing metadata: %w", err)
	}
	for _, p := range metaFiles {
		f, err := readMeta(p)
		if err != nil {
			return nil, err
		}
		received, ok, err := s.reconcile(f)
		if err != nil {
			return nil, err
		}
		if !ok {
			if err := os.Remove(p); err != nil {
				return nil, fmt.Errorf("removing orphaned metadata: %w", err)
			}
			continue
		}
		f.Received = received
		s.entries[f.ID] = &entry{file: f}
	}
	return s, nil
}

// reconcile reports how many bytes of f are on disk and whether f survives the restart.
// A complete file without its blob does not.
func (s *Store) reconcile(f File) (received int64, keep bool, err error) {
	if f.Status == StatusComplete {
		info, err := os.Stat(s.path(f.ID, ".bin"))
		if err != nil {
			return 0, false, nil
		}
		return info.Size(), true, nil
	}
	info, err := os.Stat(s.path(f.ID, ".part"))
	if err != nil {
		// The next chunk needs a file to append to.
		if err := os.WriteFile(s.path(f.ID, ".part"), nil, 0o600); err != nil {
			return 0, false, fmt.Errorf("recreating blob of %s: %w", f.ID, err)
		}
		return 0, true, nil
	}
	return min(info.Size(), f.Size), true, nil
}

// Dir is the absolute path the files are kept in.
func (s *Store) Dir() string { return s.dir }

// List returns every file, newest first.
func (s *Store) List() []File {
	entries := s.snapshotEntries()
	files := make([]File, 0, len(entries))
	for _, e := range entries {
		if f, ok := e.snapshot(); ok {
			files = append(files, f)
		}
	}
	slices.SortFunc(files, func(a, b File) int {
		if c := b.CreatedAt.Compare(a.CreatedAt); c != 0 {
			return c
		}
		return strings.Compare(a.ID, b.ID)
	})
	return files
}

// Get returns the file id refers to.
func (s *Store) Get(id string) (File, error) {
	e, err := s.entry(id)
	if err != nil {
		return File{}, err
	}
	f, ok := e.snapshot()
	if !ok {
		return File{}, ErrNotFound
	}
	return f, nil
}

// Create registers a file of size bytes under the chosen name, with original
// as what the browser called it, and returns it ready for its first chunk.
func (s *Store) Create(name, original, contentType string, size int64) (File, error) {
	name, err := CleanName(name)
	if err != nil {
		return File{}, err
	}
	original, err = CleanName(original)
	if err != nil {
		return File{}, err
	}
	if size < 0 {
		return File{}, fmt.Errorf("%w: %d", ErrInvalidSize, size)
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	id, err := newID()
	if err != nil {
		return File{}, err
	}
	f := File{
		ID:           id,
		Name:         name,
		OriginalName: original,
		ContentType:  contentType,
		Size:         size,
		Status:       StatusUploading,
		CreatedAt:    time.Now(),
	}
	if size == 0 {
		// Nothing will ever be appended, hence nothing would complete it.
		f.Status = StatusComplete
	}

	// The entry is reserved before the blob is written, so that a concurrent
	// Create cannot settle on the same name.
	s.lock.Lock()
	f.Name = s.uniqueNameLocked(name, "")
	s.entries[id] = &entry{file: f}
	s.lock.Unlock()

	blob := ".part"
	if size == 0 {
		blob = ".bin"
	}
	if err := os.WriteFile(s.path(id, blob), nil, 0o600); err != nil {
		_ = s.Delete(id)
		return File{}, fmt.Errorf("creating blob: %w", err)
	}
	if err := s.writeMeta(f); err != nil {
		_ = s.Delete(id)
		return File{}, err
	}
	return f, nil
}

// Rename changes the name a file is listed under. It changes nothing a transfer
// is matched against: [File.OriginalName] stays what the browser called it.
func (s *Store) Rename(id, name string) (File, error) {
	name, err := CleanName(name)
	if err != nil {
		return File{}, err
	}
	s.lock.Lock()
	unique := s.uniqueNameLocked(name, id)
	s.lock.Unlock()
	return s.mutate(id, func(f *File) error {
		f.Name = unique
		return s.writeMeta(*f)
	})
}

// uniqueNameLocked counts a name up until no other file carries it,
// so that the list never shows two rows a visitor cannot tell apart.
// except is the file that may keep the name it already has.
func (s *Store) uniqueNameLocked(name, except string) string {
	taken := make(map[string]bool, len(s.entries))
	for id, e := range s.entries {
		if id == except {
			continue
		}
		if f, ok := e.snapshot(); ok {
			taken[f.Name] = true
		}
	}
	if !taken[name] {
		return name
	}
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	for n := 2; ; n++ {
		candidate := base + " (" + strconv.Itoa(n) + ")" + ext
		if !taken[candidate] {
			return candidate
		}
	}
}

// Append writes the chunk r holds at offset and completes
// the file once every announced byte arrived.
//
// A chunk that repeats bytes the store already has is accepted and its overlap dropped,
// which is what makes a retried request harmless.
// One that starts past the received bytes would leave a hole and returns [ErrGap].
func (s *Store) Append(id string, offset int64, r io.Reader) (File, error) {
	e, err := s.entry(id)
	if err != nil {
		return File{}, err
	}
	e.tx.Lock()
	defer e.tx.Unlock()

	f, ok := e.snapshot()
	switch {
	case !ok:
		return File{}, ErrNotFound
	case f.Status != StatusUploading:
		return File{}, fmt.Errorf("%w: %s", ErrNotUploading, f.Status)
	case offset < 0 || offset > f.Received:
		return File{}, fmt.Errorf("%w: received %d, got offset %d",
			ErrGap, f.Received, offset)
	}
	if skip := f.Received - offset; skip > 0 {
		if _, err := io.CopyN(io.Discard, r, skip); err != nil {
			if errors.Is(err, io.EOF) {
				return f, nil // The chunk repeated received bytes only.
			}
			return File{}, fmt.Errorf("reading repeated bytes: %w", err)
		}
	}

	blob, err := os.OpenFile(s.path(id, ".part"), os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return File{}, fmt.Errorf("opening blob: %w", err)
	}
	// O_APPEND writes where the received bytes end: no seek, and no chance of
	// two chunks landing on the same offset.
	_, copyErr := io.Copy(
		&blobWriter{blob: blob, entry: e},
		io.LimitReader(r, f.Size-f.Received),
	)
	closeErr := blob.Close()
	switch {
	case copyErr != nil &&
		(errors.Is(copyErr, ErrNotFound) || errors.Is(copyErr, ErrNotUploading)):
		return File{}, copyErr
	case copyErr != nil:
		return File{}, fmt.Errorf("writing blob: %w", copyErr)
	case closeErr != nil:
		return File{}, fmt.Errorf("closing blob: %w", closeErr)
	}

	f, ok = e.snapshot()
	switch {
	case !ok:
		return File{}, ErrNotFound
	case f.Received < f.Size:
		return f, nil
	}
	if err := os.Rename(s.path(id, ".part"), s.path(id, ".bin")); err != nil {
		return File{}, fmt.Errorf("sealing blob: %w", err)
	}
	e.lock.Lock()
	e.file.Status = StatusComplete
	f = e.file
	e.lock.Unlock()
	if err := s.writeMeta(f); err != nil {
		return File{}, err
	}
	return f, nil
}

// SetStatus pauses or resumes an incomplete file. Setting the status it already has
// reports no error, which lets a client pause a transfer it's no longer sure about.
func (s *Store) SetStatus(id string, status Status) (File, error) {
	if status != StatusUploading && status != StatusPaused {
		return File{}, fmt.Errorf("cannot set status %q", status)
	}
	return s.mutate(id, func(f *File) error {
		switch f.Status {
		case StatusComplete:
			return errComplete
		case status:
			return nil
		}
		f.Status = status
		return s.writeMeta(*f)
	})
}

// SetOwner records which tab holds the bytes of the file id refers to,
// or that nobody does when owner is empty. A complete file takes no owner.
func (s *Store) SetOwner(id, owner string) (File, error) {
	return s.mutate(id, func(f *File) error {
		if f.Status == StatusComplete {
			return errComplete
		}
		f.Owner = owner
		return nil
	})
}

// ReleaseOwner drops owner from every file it holds the missing bytes of and
// reports whether it held any. A complete file keeps it, as in [Store.SetOwner].
func (s *Store) ReleaseOwner(owner string) bool {
	if owner == "" {
		return false
	}
	var released bool
	for _, e := range s.snapshotEntries() {
		e.lock.Lock()
		if !e.gone && e.file.Owner == owner && e.file.Status != StatusComplete {
			e.file.Owner = ""
			released = true
		}
		e.lock.Unlock()
	}
	return released
}

// Delete removes the file and its blob. A chunk still arriving for it
// writes to a blob nothing can reach any more and ends at its next write.
func (s *Store) Delete(id string) error {
	s.lock.Lock()
	e, ok := s.entries[id]
	delete(s.entries, id)
	s.lock.Unlock()
	if !ok {
		return ErrNotFound
	}
	e.lock.Lock()
	defer e.lock.Unlock()
	e.gone = true
	var errs []error
	for _, ext := range []string{".part", ".bin", ".json"} {
		if err := os.Remove(s.path(id, ext)); err != nil && !os.IsNotExist(err) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// Open returns the blob of a complete file for reading.
// An incomplete file has no blob to serve and returns [ErrIncomplete].
func (s *Store) Open(id string) (*os.File, error) {
	f, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	if !f.Complete() {
		return nil, ErrIncomplete
	}
	blob, err := os.Open(s.path(id, ".bin"))
	if err != nil {
		return nil, fmt.Errorf("opening blob: %w", err)
	}
	return blob, nil
}

func (s *Store) entry(id string) (*entry, error) {
	s.lock.Lock()
	defer s.lock.Unlock()
	e, ok := s.entries[id]
	if !ok {
		return nil, ErrNotFound
	}
	return e, nil
}

// mutate applies change to the file id refers to and returns it as change left it.
// change holds the lock of the entry and must not reach back into the store:
// [Store.uniqueNameLocked] takes the store lock and then this one.
func (s *Store) mutate(id string, change func(f *File) error) (File, error) {
	e, err := s.entry(id)
	if err != nil {
		return File{}, err
	}
	e.lock.Lock()
	defer e.lock.Unlock()
	if e.gone {
		return File{}, ErrNotFound
	}
	if err := change(&e.file); err != nil {
		return File{}, err
	}
	return e.file, nil
}

// snapshotEntries is every entry registered at this moment. The store lock is
// released before the caller reads any of them, so that a chunk holding the
// lock of one entry blocks nothing else.
func (s *Store) snapshotEntries() []*entry {
	s.lock.Lock()
	defer s.lock.Unlock()
	entries := make([]*entry, 0, len(s.entries))
	for _, e := range s.entries {
		entries = append(entries, e)
	}
	return entries
}

func (s *Store) path(id, ext string) string {
	return filepath.Join(s.dir, id+ext)
}

func (s *Store) writeMeta(f File) error {
	b, err := json.Marshal(f)
	if err != nil {
		return fmt.Errorf("encoding metadata: %w", err)
	}
	if err := os.WriteFile(s.path(f.ID, ".json"), b, 0o600); err != nil {
		return fmt.Errorf("writing metadata: %w", err)
	}
	return nil
}

func readMeta(path string) (File, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return File{}, fmt.Errorf("reading metadata: %w", err)
	}
	var f File
	if err := json.Unmarshal(b, &f); err != nil {
		return File{}, fmt.Errorf("decoding %s: %w", path, err)
	}
	if f.ID == "" {
		return File{}, fmt.Errorf("metadata %s has no id", path)
	}
	return f, nil
}

// CleanName trims name and replaces the control characters and path separators
// that would misread in a listing or in the download the browser saves.
// It resolves no path: no name reaches the file system.
func CleanName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("%w: empty", ErrInvalidName)
	}
	if len(name) > MaxNameLen {
		return "", fmt.Errorf("%w: longer than %d bytes", ErrInvalidName, MaxNameLen)
	}
	if !utf8.ValidString(name) {
		return "", fmt.Errorf("%w: not valid UTF-8", ErrInvalidName)
	}
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || r == '/' || r == '\\' {
			return '_'
		}
		return r
	}, name), nil
}

func newID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generating id: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}

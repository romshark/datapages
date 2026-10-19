// Package disk stores compressed static assets in a shared directory.
package disk

import (
	"cmp"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"regexp"
	"sync"
	"time"
)

// DefaultMaxUnused is what [Config.MaxUnused] falls back to.
const DefaultMaxUnused = 7 * 24 * time.Hour

// markInterval caps how often a copy's timestamp is refreshed. [New] lowers it
// to 1/7 of a shorter MaxUnused, which keeps a sweep from taking copies in use.
const markInterval = 24 * time.Hour

// copyName and tempName match files owned by the store.
var (
	copyName = regexp.MustCompile(`^[0-9a-f]{64}\.[a-z]+[0-9]+$`)
	tempName = regexp.MustCompile(`^\.[0-9a-f]{64}\.[a-z]+[0-9]+\.[A-Z2-7]+\.tmp$`)
)

// Store implements [github.com/romshark/datapages/modules/assetcompress.Store]
// with 1 file per copy. Open returns an [*os.File], which net/http can send
// with sendfile.
type Store struct {
	root      *os.Root
	marked    sync.Map // Key to last timestamp refresh.
	markEvery time.Duration
}

// Config configures a [Store]. Its zero value selects the defaults.
type Config struct {
	// MaxUnused is how long [New] keeps copies that no server opened.
	// 0 selects [DefaultMaxUnused], and a negative value keeps every copy.
	MaxUnused time.Duration
}

// New opens dir, creates it with mode 0700, and removes copies unused for
// conf.MaxUnused. Cleanup errors are ignored. A writer to dir can control
// asset responses. On Unix, dir must be owned by the server or root and must
// not be world-writable. Use a missing subdirectory inside a Kubernetes emptyDir.
func New(dir string, conf Config) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("creating the asset store: %w", err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("opening the asset store: %w", err)
	}
	if err := checkPrivate(root); err != nil {
		_ = root.Close()
		return nil, fmt.Errorf("checking the asset store: %w", err)
	}
	s := &Store{root: root, markEvery: markInterval}
	if maxUnused := cmp.Or(conf.MaxUnused, DefaultMaxUnused); maxUnused > 0 {
		s.markEvery = min(markInterval, maxUnused/7)
		_ = s.Sweep(maxUnused)
	}
	return s, nil
}

// Close releases the directory; subsequent use is invalid.
func (s *Store) Close() error { return s.root.Close() }

// Open returns key and updates its last-used time at most once per day.
func (s *Store) Open(key string) (io.ReadSeekCloser, error) {
	if !validKey(key) {
		return nil, &fs.PathError{Op: "open", Path: key, Err: fs.ErrInvalid}
	}
	f, err := s.root.Open(key)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	if v, ok := s.marked.Load(key); !ok || now.Sub(v.(time.Time)) >= s.markEvery {
		s.marked.Store(key, now)
		_ = s.root.Chtimes(key, now, now)
	}
	return f, nil
}

// Sweep removes copies and temporary files unused within maxUnused.
// It keeps other names and joins removal errors.
func (s *Store) Sweep(maxUnused time.Duration) error {
	entries, err := fs.ReadDir(s.root.FS(), ".")
	if err != nil {
		return err
	}
	cutoff := time.Now().Add(-maxUnused)
	var errs []error
	for _, e := range entries {
		name := e.Name()
		if !e.Type().IsRegular() {
			continue
		}
		if !copyName.MatchString(name) && !tempName.MatchString(name) {
			continue
		}
		if info, err := e.Info(); err != nil || info.ModTime().After(cutoff) {
			continue
		}
		if err := s.root.Remove(name); err != nil && !errors.Is(err, fs.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// Put atomically writes r under key and syncs it before renaming.
// A crash may leave a dot-prefixed temporary file for [Store.Sweep].
func (s *Store) Put(key string, r io.Reader) error {
	if !validKey(key) {
		return &fs.PathError{Op: "put", Path: key, Err: fs.ErrInvalid}
	}
	tmp := "." + key + "." + rand.Text() + ".tmp"
	f, err := s.root.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	_, err = io.Copy(f, r)
	if err == nil {
		err = f.Sync()
	}
	if errClose := f.Close(); err == nil {
		err = errClose
	}
	if err == nil {
		err = s.root.Rename(tmp, key)
	}
	if err != nil {
		return errors.Join(err, s.root.Remove(tmp))
	}
	s.marked.Store(key, time.Now())
	return nil
}

func validKey(key string) bool {
	if key == "" || key[0] == '.' {
		return false
	}
	for i := range len(key) {
		switch c := key[i]; {
		case 'a' <= c && c <= 'z', '0' <= c && c <= '9', c == '.':
		default:
			return false
		}
	}
	return true
}

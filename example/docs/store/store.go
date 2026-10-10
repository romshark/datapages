package store

import (
	"cmp"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/romshark/datapages/example/docs/delta"
	"github.com/romshark/datapages/example/docs/richtext"
)

var (
	// ErrNotFound reports a document that does not exist or was deleted.
	ErrNotFound = errors.New("document not found")

	// ErrStale reports a revision outside the history.
	// Its editor must reload the document.
	ErrStale = errors.New("revision is no longer known")

	// ErrInvalid reports a change or argument the editor never sends.
	ErrInvalid = richtext.ErrInvalid
)

const (
	// MaxLength bounds a document in UTF-16 code units.
	MaxLength = 500_000

	// maxTitleLen bounds a title in runes.
	maxTitleLen = 120

	// DefaultTitle replaces an empty title.
	DefaultTitle = "Untitled document"

	// historyLen is how many changes a document keeps to transform changes
	// made at earlier revisions. An editor further behind, such as a tab
	// offline for long, reloads the document.
	historyLen = 1000

	// compactAfter is how many logged changes trigger a new snapshot.
	compactAfter = 200

	// keepLogged is how many changes the log keeps after a snapshot,
	// to restore the history after a restart.
	keepLogged = 100
)

// Store holds the documents of one directory.
type Store struct {
	dir string

	mu   sync.Mutex
	docs map[string]*doc
}

// Summary describes a document in the document list.
type Summary struct {
	ID      string
	Title   string
	Updated time.Time
	// Preview is the markup of the thumbnail, from [richtext.Preview].
	Preview string
}

// Snapshot is a document at one revision.
type Snapshot struct {
	ID      string
	Title   string
	Rev     int
	Content delta.Delta
}

// Open loads the documents in dir, creating dir if needed.
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating store directory: %w", err)
	}
	snapshots, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	s := &Store{dir: dir, docs: make(map[string]*doc, len(snapshots))}
	for _, path := range snapshots {
		d, err := load(dir, strings.TrimSuffix(filepath.Base(path), ".json"))
		if err != nil {
			_ = s.Close()
			return nil, fmt.Errorf("loading %s: %w", path, err)
		}
		s.docs[d.id] = d
	}
	return s, nil
}

// Close closes the logs, which hold every change since the last snapshot.
// [Open] replays them.
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var errs []error
	for _, d := range s.docs {
		d.mu.Lock()
		errs = append(errs, d.log.Close())
		d.mu.Unlock()
	}
	return errors.Join(errs...)
}

// List returns the documents, most recently updated first.
func (s *Store) List() []Summary {
	s.mu.Lock()
	defer s.mu.Unlock()
	list := make([]Summary, 0, len(s.docs))
	for _, d := range s.docs {
		d.mu.Lock()
		if d.previewRev != d.rev {
			d.preview, d.previewRev = richtext.Preview(d.content), d.rev
		}
		list = append(list, Summary{
			ID: d.id, Title: d.title, Updated: d.updated, Preview: d.preview,
		})
		d.mu.Unlock()
	}
	slices.SortFunc(list, func(a, b Summary) int {
		return cmp.Or(b.Updated.Compare(a.Updated), strings.Compare(a.ID, b.ID))
	})
	return list
}

// Create adds a document and returns its id. It cleans title like
// [Store.Rename] and rejects content that [richtext.CheckDocument] rejects,
// which would make [Open] fail.
func (s *Store) Create(title string, content delta.Delta) (string, error) {
	if err := richtext.CheckDocument(content); err != nil {
		return "", err
	}
	var b [12]byte
	_, _ = rand.Read(b[:])
	id := base64.RawURLEncoding.EncodeToString(b[:])
	d := &doc{
		dir:        s.dir,
		id:         id,
		title:      cleanTitle(title),
		updated:    time.Now(),
		previewRev: -1,
		content:    content,
		editors:    make(map[string]*editor),
	}
	if err := d.compact(); err != nil {
		return "", err
	}
	if err := d.openLog(); err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.docs[id] = d
	return id, nil
}

// Delete removes a document and its files.
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	d, ok := s.docs[id]
	delete(s.docs, id)
	s.mu.Unlock()
	if !ok {
		return ErrNotFound
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.deleted = true
	return errors.Join(
		d.log.Close(),
		os.Remove(d.path(".log")),
		os.Remove(d.path(".json")),
	)
}

// Rename sets the title of a document, with control characters as spaces,
// cut to [maxTitleLen] runes and trimmed.
func (s *Store) Rename(id, title string) error {
	title = cleanTitle(title)
	return s.with(id, func(d *doc) error {
		if title == d.title {
			return nil
		}
		previous := d.title
		d.title = title
		if err := d.compact(); err != nil {
			d.title = previous
			return err
		}
		return nil
	})
}

func cleanTitle(title string) string {
	title = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, title)
	if r := []rune(title); len(r) > maxTitleLen {
		title = string(r[:maxTitleLen])
	}
	if title = strings.TrimSpace(title); title == "" {
		return DefaultTitle
	}
	return title
}

// Get returns the current revision of a document.
func (s *Store) Get(id string) (Snapshot, error) {
	var snap Snapshot
	err := s.with(id, func(d *doc) error {
		snap = Snapshot{ID: d.id, Title: d.title, Rev: d.rev, Content: d.content}
		return nil
	})
	return snap, err
}

// with runs f with the document locked.
func (s *Store) with(id string, f func(d *doc) error) error {
	s.mu.Lock()
	d, ok := s.docs[id]
	s.mu.Unlock()
	if !ok {
		return ErrNotFound
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.deleted {
		return ErrNotFound
	}
	return f(d)
}

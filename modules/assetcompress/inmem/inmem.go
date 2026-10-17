// Package inmem stores compressed static assets in memory.
package inmem

import (
	"bytes"
	"io"
	"io/fs"
	"sync"
)

// Store implements [github.com/romshark/datapages/modules/assetcompress.Store]
// and retains every copy until exit.
type Store struct {
	files sync.Map // Key to []byte.
}

// New returns an empty store.
func New() *Store { return new(Store) }

func (s *Store) Open(key string) (io.ReadSeekCloser, error) {
	b, ok := s.files.Load(key)
	if !ok {
		return nil, &fs.PathError{Op: "open", Path: key, Err: fs.ErrNotExist}
	}
	return file{bytes.NewReader(b.([]byte))}, nil
}

// Put stores r atomically.
func (s *Store) Put(key string, r io.Reader) error {
	b, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	s.files.LoadOrStore(key, b)
	return nil
}

type file struct{ *bytes.Reader }

func (file) Close() error { return nil }

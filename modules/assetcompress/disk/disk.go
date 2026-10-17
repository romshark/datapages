// Package disk stores compressed static assets in a shared directory.
package disk

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
)

// Store implements [github.com/romshark/datapages/modules/assetcompress.Store]
// with 1 file per copy. Open returns an [*os.File], which net/http can send
// with sendfile.
type Store struct{ root *os.Root }

// New returns a store in dir, creating missing directories with mode 0700.
// A writer to dir can control the server's asset responses.
func New(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("creating the asset store: %w", err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("opening the asset store: %w", err)
	}
	return &Store{root: root}, nil
}

// Close releases the directory. The store must not be used after.
func (s *Store) Close() error { return s.root.Close() }

func (s *Store) Open(key string) (io.ReadSeekCloser, error) {
	if !validKey(key) {
		return nil, &fs.PathError{Op: "open", Path: key, Err: fs.ErrInvalid}
	}
	return s.root.Open(key)
}

// Put stores r atomically through a temporary file.
// A crash may leave the dot-prefixed temporary file behind.
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
	if errClose := f.Close(); err == nil {
		err = errClose
	}
	if err == nil {
		err = s.root.Rename(tmp, key)
	}
	if err != nil {
		return errors.Join(err, s.root.Remove(tmp))
	}
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

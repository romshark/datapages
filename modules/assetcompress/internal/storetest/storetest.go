// Package storetest tests [assetcompress.Store] implementations.
package storetest

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/modules/assetcompress"
)

const key = "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08.br6"

var errRead = errors.New("read failed")

type failingReader struct{ n int }

func (r *failingReader) Read(p []byte) (int, error) {
	if r.n > 0 {
		n := min(r.n, len(p))
		r.n -= n
		return n, nil
	}
	return 0, errRead
}

func readAll(t *testing.T, s assetcompress.Store, key string) string {
	t.Helper()
	f, err := s.Open(key)
	require.NoError(t, err)
	defer func() { require.NoError(t, f.Close()) }()
	size, err := f.Seek(0, io.SeekEnd)
	require.NoError(t, err)
	_, err = f.Seek(0, io.SeekStart)
	require.NoError(t, err)
	b, err := io.ReadAll(f)
	require.NoError(t, err)
	require.Equal(t, int64(len(b)), size, "the size Seek reports")
	return string(b)
}

// Run tests stores returned by newStore.
func Run(t *testing.T, newStore func(t *testing.T) assetcompress.Store) {
	t.Run("missing", func(t *testing.T) {
		_, err := newStore(t).Open(key)
		require.ErrorIs(t, err, fs.ErrNotExist)
	})
	t.Run("put and open", func(t *testing.T) {
		s := newStore(t)
		require.NoError(t, s.Put(key, strings.NewReader("compressed bytes")))
		require.Equal(t, "compressed bytes", readAll(t, s, key))
	})
	t.Run("failed put leaves no copy", func(t *testing.T) {
		s := newStore(t)
		require.ErrorIs(t, s.Put(key, &failingReader{n: 64 << 10}), errRead)
		_, err := s.Open(key)
		require.ErrorIs(t, err, fs.ErrNotExist)
	})
	t.Run("put again", func(t *testing.T) {
		s := newStore(t)
		require.NoError(t, s.Put(key, strings.NewReader("compressed bytes")))
		require.NoError(t, s.Put(key, strings.NewReader("compressed bytes")))
		require.Equal(t, "compressed bytes", readAll(t, s, key))
	})
	t.Run("concurrent", func(t *testing.T) {
		s := newStore(t)
		data := bytes.Repeat([]byte("copy "), 64<<10)
		var wg sync.WaitGroup
		for range 8 {
			wg.Go(func() {
				require.NoError(t, s.Put(key, bytes.NewReader(data)))
				f, err := s.Open(key)
				require.NoError(t, err)
				b, err := io.ReadAll(f)
				require.NoError(t, err)
				require.NoError(t, f.Close())
				require.Equal(t, data, b, "a copy read while others are put")
			})
		}
		wg.Wait()
	})
}

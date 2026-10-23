package exactpath_test

import (
	"errors"
	"io/fs"
	"net/http"
	"sync/atomic"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/internal/exactpath"
)

func files() http.FileSystem {
	return http.FS(fstest.MapFS{
		"style.css":      {Data: []byte("a")},
		"sub/nested.css": {Data: []byte("b")},
	})
}

// TestExact tests paths with exact, other and missing spellings.
func TestExact(t *testing.T) {
	t.Parallel()

	l := exactpath.New(files())
	for p, want := range map[string]bool{
		"":               true,
		"style.css":      true,
		"sub":            true,
		"sub/nested.css": true,
		"Style.css":      false,
		"SUB/nested.css": false,
		"sub/Nested.css": false,
		"missing.css":    false,
		"sub/missing":    false,
		"../style.css":   false,
	} {
		require.Equal(t, want, l.Exact(p), p)
	}
}

// TestClean tests clean, unclean and root paths.
func TestClean(t *testing.T) {
	for p, want := range map[string]string{
		"":          "",
		"/":         "",
		".":         "",
		"..":        "",
		"a.css":     "a.css",
		"/a.css":    "a.css",
		"sub/a.css": "sub/a.css",
		"sub/":      "sub",
		"//a.css":   "a.css",
		"sub//a":    "sub/a",
		"./a.css":   "a.css",
		"sub/../a":  "a",
		"../a.css":  "a.css",
		"..a.css":   "..a.css",
	} {
		require.Equal(t, want, exactpath.Clean(p), p)
	}
	require.Zero(t, testing.AllocsPerRun(10, func() {
		_ = exactpath.Clean("/sub/a.css")
	}), "a clean path")
}

// countingFS counts the directories it lists and fails to list them when
// broken is set.
type countingFS struct {
	http.FileSystem
	broken bool
	lists  atomic.Int64
}

func (fsys *countingFS) Open(name string) (http.File, error) {
	f, err := fsys.FileSystem.Open(name)
	if err != nil {
		return nil, err
	}
	return &countingFile{File: f, fsys: fsys}, nil
}

type countingFile struct {
	http.File
	fsys *countingFS
}

func (f *countingFile) Readdir(n int) ([]fs.FileInfo, error) {
	f.fsys.lists.Add(1)
	if f.fsys.broken {
		return nil, errors.New("listing refused")
	}
	return f.File.Readdir(n)
}

// TestExactListsOnce tests that each directory is listed once.
func TestExactListsOnce(t *testing.T) {
	t.Parallel()

	fsys := &countingFS{FileSystem: files()}
	l := exactpath.New(fsys)
	for range 3 {
		require.True(t, l.Exact("sub/nested.css"))
		require.False(t, l.Exact("SUB/nested.css"))
	}
	require.Equal(t, int64(2), fsys.lists.Load(), "the root and sub")
}

// TestExactUnlistable tests that a directory that cannot be listed admits
// every spelling and is not listed again.
func TestExactUnlistable(t *testing.T) {
	t.Parallel()

	fsys := &countingFS{FileSystem: files(), broken: true}
	l := exactpath.New(fsys)
	require.True(t, l.Exact("Style.css"))
	require.True(t, l.Exact("style.css"))
	require.Equal(t, int64(1), fsys.lists.Load())
}

package disk_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/modules/assetcompress"
	"github.com/romshark/datapages/modules/assetcompress/disk"
	"github.com/romshark/datapages/modules/assetcompress/internal/storetest"
)

const key = "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08.br6"

func newStore(t *testing.T, dir string) *disk.Store {
	t.Helper()
	s, err := disk.New(dir)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, s.Close()) })
	return s
}

// TestStore tests the contract and cleanup after failed puts.
func TestStore(t *testing.T) {
	storetest.Run(t, func(t *testing.T) assetcompress.Store {
		dir := t.TempDir()
		t.Cleanup(func() {
			entries, err := os.ReadDir(dir)
			require.NoError(t, err)
			for _, e := range entries {
				require.False(t, strings.HasPrefix(e.Name(), "."),
					"temporary file %s left behind", e.Name())
			}
		})
		return newStore(t, dir)
	})
}

// TestNewCreatesPrivateDir tests permissions on a created directory.
func TestNewCreatesPrivateDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no permission bits")
	}
	dir := filepath.Join(t.TempDir(), "a", "assets")
	newStore(t, dir)
	st, err := os.Stat(dir)
	require.NoError(t, err)
	require.Equal(t, fs.FileMode(0o700), st.Mode().Perm())
}

// TestCopiesOutliveTheStore tests persistence and the sendfile type.
func TestCopiesOutliveTheStore(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, newStore(t, dir).Put(key, strings.NewReader("compressed")))

	f, err := newStore(t, dir).Open(key)
	require.NoError(t, err)
	defer func() { require.NoError(t, f.Close()) }()
	_, ok := f.(*os.File)
	require.True(t, ok, "Open returns %T, which net/http cannot send with sendfile", f)
}

// TestInvalidKeys tests rejection of paths and temporary-file names.
func TestInvalidKeys(t *testing.T) {
	s := newStore(t, t.TempDir())
	for name, key := range map[string]string{
		"empty":           "",
		"parent":          "../" + key,
		"subdirectory":    "a/" + key,
		"dot first":       "." + key,
		"upper case":      strings.ToUpper(key),
		"path separator":  `a\b`,
		"temporary files": "." + key + ".X.tmp",
	} {
		t.Run(name, func(t *testing.T) {
			require.ErrorIs(t, s.Put(key, strings.NewReader("x")), fs.ErrInvalid)
			_, err := s.Open(key)
			require.ErrorIs(t, err, fs.ErrInvalid)
		})
	}
}

package disk_test

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/modules/assetcompress"
	"github.com/romshark/datapages/modules/assetcompress/disk"
	"github.com/romshark/datapages/modules/assetcompress/internal/storetest"
)

const key = "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08.br6"

func newStore(t *testing.T, dir string) *disk.Store {
	t.Helper()
	s, err := disk.New(dir, disk.Config{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, s.Close()) })
	return s
}

// TestStore covers the contract and temporary-file cleanup after failed puts.
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

// TestNewCreatesPrivateDir verifies mode 0700 on a created directory.
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

// TestNewRefusesSharedDir covers unsafe roots and a private child beneath one.
func TestNewRefusesSharedDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no permission bits")
	}
	for name, tc := range map[string]struct {
		prepare func(t *testing.T, dir string)
		wantErr string // %s is the directory.
	}{
		"writable by every user": {
			prepare: func(t *testing.T, dir string) {
				require.NoError(t, os.Chmod(dir, 0o777))
			},
			wantErr: "checking the asset store: %s is writable by every user",
		},
		"owned by another user": {
			prepare: func(t *testing.T, dir string) {
				if os.Geteuid() != 0 {
					t.Skip("only root can give a directory away")
				}
				require.NoError(t, os.Chown(dir, 65534, -1))
			},
			wantErr: "checking the asset store: %s belongs to user 65534, " +
				"neither the server's user nor root",
		},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			tc.prepare(t, dir)
			_, err := disk.New(dir, disk.Config{})
			require.EqualError(t, err, fmt.Sprintf(tc.wantErr, dir))
			newStore(t, filepath.Join(dir, "assets"))
		})
	}
}

// TestCopiesOutliveTheStore covers persistence and the sendfile type.
func TestCopiesOutliveTheStore(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, newStore(t, dir).Put(key, strings.NewReader("compressed")))

	f, err := newStore(t, dir).Open(key)
	require.NoError(t, err)
	defer func() { require.NoError(t, f.Close()) }()
	_, ok := f.(*os.File)
	require.True(t, ok, "Open returns %T, which net/http cannot send with sendfile", f)
}

// TestSweep covers startup expiry, name filtering and zero retention.
func TestSweep(t *testing.T) {
	dir := t.TempDir()
	oldKey := strings.Repeat("a", 64) + ".br6"
	oldTemp := "." + oldKey + ".ABCDEFGHIJKLMNOPQRSTUVWXYZ.tmp"
	old := time.Now().Add(-disk.DefaultMaxUnused - time.Hour)
	for name, mt := range map[string]time.Time{
		oldKey:      old,
		oldTemp:     old,
		key:         time.Now().Add(-time.Hour),
		"notes.txt": old,
		"abc.br6":   old,
	} {
		file := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(file, []byte("x"), 0o600))
		require.NoError(t, os.Chtimes(file, mt, mt))
	}
	exists := func(name string) bool {
		_, err := os.Stat(filepath.Join(dir, name))
		return err == nil
	}

	s := newStore(t, dir)
	require.False(t, exists(oldKey))
	require.False(t, exists(oldTemp))
	require.True(t, exists(key))
	require.True(t, exists("notes.txt"))
	require.True(t, exists("abc.br6"), "not a SHA-256")

	require.NoError(t, s.Sweep(0))
	require.False(t, exists(key))
	require.True(t, exists("notes.txt"))
}

// TestNewMaxUnused tests that New sweeps with Config.MaxUnused, and that
// a negative MaxUnused keeps every copy.
func TestNewMaxUnused(t *testing.T) {
	oldKey := strings.Repeat("a", 64) + ".br6"
	for name, tc := range map[string]struct {
		maxUnused  time.Duration
		wantOld    bool // Unused for 8 days.
		wantRecent bool // Unused for 1 hour.
	}{
		"default":  {wantRecent: true},
		"1 minute": {maxUnused: time.Minute},
		"negative": {maxUnused: -1, wantOld: true, wantRecent: true},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			for name, age := range map[string]time.Duration{
				oldKey: 8 * 24 * time.Hour,
				key:    time.Hour,
			} {
				file := filepath.Join(dir, name)
				require.NoError(t, os.WriteFile(file, []byte("x"), 0o600))
				mt := time.Now().Add(-age)
				require.NoError(t, os.Chtimes(file, mt, mt))
			}
			s, err := disk.New(dir, disk.Config{MaxUnused: tc.maxUnused})
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, s.Close()) })
			_, err = os.Stat(filepath.Join(dir, oldKey))
			require.Equal(t, tc.wantOld, err == nil, "copy unused for 8 days")
			_, err = os.Stat(filepath.Join(dir, key))
			require.Equal(t, tc.wantRecent, err == nil, "copy unused for 1 hour")
		})
	}
}

// TestOpenMarksUse verifies that Open refreshes an old mtime after restart.
func TestOpenMarksUse(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, newStore(t, dir).Put(key, strings.NewReader("compressed")))
	file := filepath.Join(dir, key)
	old := time.Now().Add(-48 * time.Hour)
	require.NoError(t, os.Chtimes(file, old, old))

	f, err := newStore(t, dir).Open(key)
	require.NoError(t, err)
	require.NoError(t, f.Close())
	st, err := os.Stat(file)
	require.NoError(t, err)
	require.WithinDuration(t, time.Now(), st.ModTime(), time.Minute)
}

// TestInvalidKeys covers paths and temporary-file names.
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

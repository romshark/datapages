package store_test

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/example/file-upload/store"
)

const contents = "0123456789abcdefghij"

func newStore(t *testing.T, dir string) *store.Store {
	t.Helper()
	s, err := store.New(dir)
	require.NoError(t, err, "opening store")
	return s
}

// create registers contents under name and returns it.
func create(t *testing.T, s *store.Store) store.File {
	t.Helper()
	f, err := s.Create("notes.txt", "notes.txt", "text/plain", int64(len(contents)))
	require.NoError(t, err, "creating file")
	return f
}

// TestAppendCompletes tests that a file uploaded in chunks holds what was sent
// and becomes downloadable exactly once every byte arrived.
func TestAppendCompletes(t *testing.T) {
	s := newStore(t, t.TempDir())
	f := create(t, s)

	f, err := s.Append(f.ID, 0, strings.NewReader(contents[:10]))
	require.NoError(t, err, "first chunk")
	require.Equal(t, int64(10), f.Received)
	require.Equal(t, store.StatusUploading, f.Status)
	require.Equal(t, 50, f.Percent())

	_, err = s.Open(f.ID)
	require.ErrorIs(t, err, store.ErrIncomplete,
		"an incomplete file must not be downloadable")

	f, err = s.Append(f.ID, 10, strings.NewReader(contents[10:]))
	require.NoError(t, err, "second chunk")
	require.Equal(t, store.StatusComplete, f.Status)
	require.Equal(t, 100, f.Percent())

	blob, err := s.Open(f.ID)
	require.NoError(t, err, "opening the completed file")
	defer func() { _ = blob.Close() }()
	got, err := os.ReadFile(blob.Name())
	require.NoError(t, err, "reading the completed file")
	require.Equal(t, contents, string(got))
}

// TestAppendOffsets tests what the store does with an offset that isn't the
// next byte: a repeat is accepted and deduplicated, a hole is refused.
func TestAppendOffsets(t *testing.T) {
	for name, tc := range map[string]struct {
		offset   int64
		chunk    string
		wantErr  error
		wantRecv int64
	}{
		"next byte":      {offset: 10, chunk: contents[10:15], wantRecv: 15},
		"full repeat":    {offset: 0, chunk: contents[:10], wantRecv: 10},
		"partial repeat": {offset: 5, chunk: contents[5:15], wantRecv: 15},
		"hole":           {offset: 11, chunk: contents[11:], wantErr: store.ErrGap, wantRecv: 10},
		"negative":       {offset: -1, chunk: contents, wantErr: store.ErrGap, wantRecv: 10},
		"past the size":  {offset: 10, chunk: contents[10:] + "overflow", wantRecv: 20},
	} {
		t.Run(name, func(t *testing.T) {
			s := newStore(t, t.TempDir())
			f := create(t, s)
			_, err := s.Append(f.ID, 0, strings.NewReader(contents[:10]))
			require.NoError(t, err, "first chunk")

			_, err = s.Append(f.ID, tc.offset, strings.NewReader(tc.chunk))
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
			} else {
				require.NoError(t, err)
			}
			got, err := s.Get(f.ID)
			require.NoError(t, err, "reading the file back")
			require.Equal(t, tc.wantRecv, got.Received)
		})
	}
}

// TestAppendDuringChunk tests that the bytes of a chunk show up while it's
// still arriving and that pausing ends it there, rather than both waiting for
// the end of the request.
func TestAppendDuringChunk(t *testing.T) {
	s := newStore(t, t.TempDir())
	f := create(t, s)

	pr, pw := io.Pipe()
	t.Cleanup(func() { _ = pw.Close() })
	done := make(chan error, 1)
	go func() {
		_, err := s.Append(f.ID, 0, pr)
		done <- err
	}()

	_, err := pw.Write([]byte(contents[:5]))
	require.NoError(t, err, "first bytes of the chunk")
	require.Eventually(t, func() bool {
		got, err := s.Get(f.ID)
		return err == nil && got.Received == 5
	}, time.Second, 5*time.Millisecond,
		"the file list did not see the bytes of an open chunk")

	_, err = s.SetStatus(f.ID, store.StatusPaused)
	require.NoError(t, err, "pausing while the chunk is open")
	_, err = pw.Write([]byte(contents[5:10]))
	require.NoError(t, err, "further bytes of the chunk")
	require.ErrorIs(t, <-done, store.ErrNotUploading,
		"the chunk went on after the pause")
}

// TestAppendRefusesPaused tests that a paused file takes no more bytes,
// which is what stops an uploader that has not seen the pause yet.
func TestAppendRefusesPaused(t *testing.T) {
	s := newStore(t, t.TempDir())
	f := create(t, s)

	_, err := s.SetStatus(f.ID, store.StatusPaused)
	require.NoError(t, err, "pausing")
	_, err = s.Append(f.ID, 0, strings.NewReader(contents))
	require.ErrorIs(t, err, store.ErrNotUploading)

	_, err = s.SetStatus(f.ID, store.StatusUploading)
	require.NoError(t, err, "resuming")
	_, err = s.Append(f.ID, 0, strings.NewReader(contents))
	require.NoError(t, err, "a resumed file takes bytes again")

	_, err = s.SetStatus(f.ID, store.StatusPaused)
	require.ErrorIs(t, err, store.ErrNotUploading,
		"a complete file must not go back to paused")
}

// TestReopenKeepsReceived tests that a restart resumes where the process stopped,
// which the size of the blob on disk decides.
func TestReopenKeepsReceived(t *testing.T) {
	dir := t.TempDir()
	s := newStore(t, dir)
	f := create(t, s)
	_, err := s.Append(f.ID, 0, strings.NewReader(contents[:10]))
	require.NoError(t, err, "first chunk")

	s = newStore(t, dir)
	got, err := s.Get(f.ID)
	require.NoError(t, err, "the file did not survive the restart")
	require.Equal(t, int64(10), got.Received)
	require.Equal(t, "notes.txt", got.Name)
	require.Empty(t, got.Owner, "a restart cannot know who was sending it")

	got, err = s.Append(f.ID, 10, strings.NewReader(contents[10:]))
	require.NoError(t, err, "second chunk after the restart")
	require.Equal(t, store.StatusComplete, got.Status)
}

// TestDelete tests that deleting removes the file and everything it wrote.
func TestDelete(t *testing.T) {
	dir := t.TempDir()
	s := newStore(t, dir)
	f := create(t, s)
	_, err := s.Append(f.ID, 0, strings.NewReader(contents[:10]))
	require.NoError(t, err, "first chunk")

	require.NoError(t, s.Delete(f.ID), "deleting")
	require.ErrorIs(t, s.Delete(f.ID), store.ErrNotFound, "deleting twice")

	_, err = s.Get(f.ID)
	require.ErrorIs(t, err, store.ErrNotFound)
	left, err := filepath.Glob(filepath.Join(dir, "*"))
	require.NoError(t, err, "listing the store directory")
	require.Empty(t, left, "the deleted file left something behind")
}

// TestCleanName tests the names the store accepts and what it makes of them.
func TestCleanName(t *testing.T) {
	for name, tc := range map[string]struct {
		in      string
		want    string
		wantErr error
	}{
		"plain":           {in: "notes.txt", want: "notes.txt"},
		"trimmed":         {in: "  notes.txt \n", want: "notes.txt"},
		"path separators": {in: "../../etc/passwd", want: ".._.._etc_passwd"},
		"backslashes":     {in: `C:\tmp\x`, want: "C:_tmp_x"},
		"control chars":   {in: "a\u0000b\u001fc", want: "a_b_c"},
		"empty":           {in: "   ", wantErr: store.ErrInvalidName},
		"too long": {
			in:      strings.Repeat("a", store.MaxNameLen+1),
			wantErr: store.ErrInvalidName,
		},
		"invalid utf8":    {in: "a\xffb", wantErr: store.ErrInvalidName},
		"unicode is kept": {in: "Bilder für 2026.zip", want: "Bilder für 2026.zip"},
		"quotes are kept": {in: `he said "hi".txt`, want: `he said "hi".txt`},
		"markup is kept":  {in: "<script>.txt", want: "<script>.txt"},
		"max length fits": {
			in:   strings.Repeat("a", store.MaxNameLen),
			want: strings.Repeat("a", store.MaxNameLen),
		},
		"leading dot kept": {in: ".hidden", want: ".hidden"},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := store.CleanName(tc.in)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

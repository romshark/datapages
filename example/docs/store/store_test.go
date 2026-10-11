package store_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/example/docs/delta"
	"github.com/romshark/datapages/example/docs/store"
)

type attrs = delta.Attributes

func ins(text string, a attrs) delta.Op { return delta.Op{Insert: text, Attributes: a} }
func ret(n int, a attrs) delta.Op       { return delta.Op{Retain: n, Attributes: a} }
func del(n int) delta.Op                { return delta.Op{Delete: n} }
func ops(o ...delta.Op) delta.Delta     { return delta.Delta{Ops: o} }

func open(t *testing.T, dir string) *store.Store {
	t.Helper()
	s, err := store.Open(dir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func create(t *testing.T, s *store.Store) string {
	t.Helper()
	id, err := s.Create("", ops(ins("\n", nil)))
	require.NoError(t, err)
	return id
}

func content(t *testing.T, s *store.Store, id string) (delta.Delta, int) {
	t.Helper()
	snap, err := s.Get(id)
	require.NoError(t, err)
	return snap.Content, snap.Rev
}

// TestCreate tests the title and content a new document starts with.
func TestCreate(t *testing.T) {
	for name, tc := range map[string]struct {
		title, wantTitle string
		content          delta.Delta
	}{
		"blank": {"", store.DefaultTitle, ops(ins("\n", nil))},
		"letter": {
			" Letter ", "Letter",
			ops(ins("Dear", attrs{"bold": true}), ins("\n", attrs{"header": 1.0})),
		},
	} {
		t.Run(name, func(t *testing.T) {
			s := open(t, t.TempDir())
			id, err := s.Create(tc.title, tc.content)
			require.NoError(t, err)
			snap, err := s.Get(id)
			require.NoError(t, err)
			require.Equal(t, store.Snapshot{
				ID: id, Title: tc.wantTitle, Content: tc.content,
			}, snap)
			list := s.List()
			require.Len(t, list, 1)
			require.Equal(t, id, list[0].ID)
		})
	}
}

// TestCreateInvalid tests that [store.Store.Create] rejects content Quill cannot hold.
func TestCreateInvalid(t *testing.T) {
	s := open(t, t.TempDir())
	_, err := s.Create("", ops(ins("no final line break", nil)))
	require.ErrorIs(t, err, store.ErrInvalid)
	require.Empty(t, s.List())
}

// TestRename tests how titles are cleaned.
func TestRename(t *testing.T) {
	s := open(t, t.TempDir())
	id := create(t, s)
	for name, tc := range map[string]struct{ in, want string }{
		"plain":         {"Plan", "Plan"},
		"trimmed":       {"  Plan \n", "Plan"},
		"control":       {"a\tb\x00c", "a b c"},
		"empty":         {"   ", store.DefaultTitle},
		"long":          {strings.Repeat("é", 200), strings.Repeat("é", 120)},
		"html stays as": {"<b>", "<b>"},
	} {
		t.Run(name, func(t *testing.T) {
			require.NoError(t, s.Rename(id, tc.in))
			snap, err := s.Get(id)
			require.NoError(t, err)
			require.Equal(t, tc.want, snap.Title)
		})
	}
}

// TestDelete tests that a deleted document and its files are gone.
func TestDelete(t *testing.T) {
	dir := t.TempDir()
	s := open(t, dir)
	id := create(t, s)
	require.NoError(t, s.Delete(id))
	_, err := s.Get(id)
	require.ErrorIs(t, err, store.ErrNotFound)
	require.ErrorIs(t, s.Delete(id), store.ErrNotFound)
	files, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Empty(t, files)
}

// TestSubmitConcurrent tests that changes made at the same revision are
// transformed against the ones the server applied first.
func TestSubmitConcurrent(t *testing.T) {
	s := open(t, t.TempDir())
	id := create(t, s)
	a, b := store.NewClient(), store.NewClient()

	require.NoError(t, s.Submit(id, store.Submission{
		Client: a, Seq: 1, Rev: 0, Delta: ops(ins("Hello", nil)),
	}))
	// b types at the start before seeing "Hello".
	require.NoError(t, s.Submit(id, store.Submission{
		Client: b, Seq: 1, Rev: 0, Delta: ops(ins("World", nil)),
	}))
	// a formats what it typed, still at revision 1.
	require.NoError(t, s.Submit(id, store.Submission{
		Client: a, Seq: 2, Rev: 1, Delta: ops(ret(5, attrs{"bold": true})),
	}))

	doc, rev := content(t, s, id)
	require.Equal(t, 3, rev)
	require.Equal(t, ops(ins("Hello", attrs{"bold": true}), ins("World\n", nil)), doc)

	v, err := s.View(id, a, 1)
	require.NoError(t, err)
	require.Len(t, v.Changes, 2)
	require.Equal(t, ops(ret(5, nil), ins("World", nil)), v.Changes[0].Delta)
	require.Equal(t, b, v.Changes[0].Client)
}

// TestSubmitRetry tests that a change sent again is applied once.
func TestSubmitRetry(t *testing.T) {
	s := open(t, t.TempDir())
	id := create(t, s)
	a := store.NewClient()
	sub := store.Submission{Client: a, Seq: 1, Rev: 0, Delta: ops(ins("x", nil))}
	require.NoError(t, s.Submit(id, sub))
	require.NoError(t, s.Submit(id, sub))
	doc, rev := content(t, s, id)
	require.Equal(t, 1, rev)
	require.Equal(t, ops(ins("x\n", nil)), doc)
}

// TestSubmitRejects tests the changes the store refuses.
func TestSubmitRejects(t *testing.T) {
	s := open(t, t.TempDir())
	id := create(t, s)
	a := store.NewClient()
	require.NoError(t, s.Submit(id, store.Submission{
		Client: a, Seq: 1, Rev: 0, Delta: ops(ins("a😀", nil)),
	}))
	for name, tc := range map[string]struct {
		sub  store.Submission
		want error
	}{
		"future revision": {
			store.Submission{Client: a, Seq: 9, Rev: 5, Delta: ops(ins("x", nil))},
			store.ErrStale,
		},
		"bad client": {
			store.Submission{Client: "x", Seq: 9, Rev: 1, Delta: ops(ins("x", nil))},
			store.ErrInvalid,
		},
		"no sequence number": {
			store.Submission{Client: a, Rev: 1, Delta: ops(ins("x", nil))},
			store.ErrInvalid,
		},
		"past the end": {
			store.Submission{Client: a, Seq: 9, Rev: 1, Delta: ops(ret(5, nil), ins("x", nil))},
			store.ErrInvalid,
		},
		"final line break": {
			store.Submission{Client: a, Seq: 9, Rev: 1, Delta: ops(ret(3, nil), del(1))},
			store.ErrInvalid,
		},
		"surrogate split": {
			store.Submission{Client: a, Seq: 9, Rev: 1, Delta: ops(ret(2, nil), ins("x", nil))},
			store.ErrInvalid,
		},
		"format on line break": {
			store.Submission{Client: a, Seq: 9, Rev: 1, Delta: ops(ret(3, nil), ret(1, attrs{"bold": true}))},
			store.ErrInvalid,
		},
		"unknown format": {
			store.Submission{Client: a, Seq: 9, Rev: 1, Delta: ops(ret(1, attrs{"size": "huge"}))},
			store.ErrInvalid,
		},
		"missing document": {
			store.Submission{Client: a, Seq: 9, Rev: 1, Delta: ops(ins("x", nil))},
			store.ErrNotFound,
		},
	} {
		t.Run(name, func(t *testing.T) {
			doc := id
			if name == "missing document" {
				doc = "missing"
			}
			require.ErrorIs(t, s.Submit(doc, tc.sub), tc.want)
			got, rev := content(t, s, id)
			require.Equal(t, 1, rev)
			require.Equal(t, ops(ins("a😀\n", nil)), got)
		})
	}
}

// TestSubmitRepair tests that a conflict of line types is repaired in the
// revision after the change that caused it.
func TestSubmitRepair(t *testing.T) {
	s := open(t, t.TempDir())
	id := create(t, s)
	a, b := store.NewClient(), store.NewClient()
	require.NoError(t, s.Submit(id, store.Submission{
		Client: a, Seq: 1, Rev: 0, Delta: ops(ins("Title", nil)),
	}))
	require.NoError(t, s.Submit(id, store.Submission{
		Client: a, Seq: 2, Rev: 1, Delta: ops(ret(5, nil), ret(1, attrs{"header": 1.0})),
	}))
	require.NoError(t, s.Submit(id, store.Submission{
		Client: b, Seq: 1, Rev: 1, Delta: ops(ret(5, nil), ret(1, attrs{"list": "bullet"})),
	}))
	doc, rev := content(t, s, id)
	require.Equal(t, 4, rev)
	require.Equal(t, ops(ins("Title", nil), ins("\n", attrs{"list": "bullet"})), doc)

	v, err := s.View(id, a, 3)
	require.NoError(t, err)
	require.Equal(t, []store.Change{{
		Rev: 4, Delta: ops(ret(5, nil), ret(1, attrs{"header": nil, "list": "bullet"})),
		At: v.Changes[0].At,
	}}, v.Changes)
}

// TestRestore tests that the changes a closed editor kept end as they would
// have had the editor stayed: transformed against the changes it missed,
// without the one the store applied before it closed.
func TestRestore(t *testing.T) {
	for name, tc := range map[string]struct {
		// before runs before the restore. a is the closed editor, b another.
		before   func(submit func(client string, seq, rev int, change delta.Delta))
		pending  store.Pending
		restores int
		want     delta.Delta
		wantRev  int
	}{
		"never submitted": {
			before: func(submit func(string, int, int, delta.Delta)) {
				submit("b", 1, 0, ops(ins("Hi ", nil)))
			},
			pending: store.Pending{
				Seq: 1, Rev: 0,
				Sent:   ops(ins("Hello", nil)),
				Buffer: ops(ret(5, nil), ins(" World", nil)),
			},
			restores: 1,
			want:     ops(ins("Hi Hello World\n", nil)),
			wantRev:  3,
		},
		"sent applied": {
			before: func(submit func(string, int, int, delta.Delta)) {
				submit("a", 1, 0, ops(ins("Hello", nil)))
				submit("b", 1, 1, ops(ret(5, nil), ins("!", nil)))
			},
			pending: store.Pending{
				Seq: 1, Rev: 0,
				Sent:   ops(ins("Hello", nil)),
				Buffer: ops(ret(5, nil), ins(" World", nil)),
			},
			restores: 1,
			want:     ops(ins("Hello! World\n", nil)),
			wantRev:  3,
		},
		"restored 2 times": {
			before: func(submit func(string, int, int, delta.Delta)) {
				submit("b", 1, 0, ops(ins("Hi ", nil)))
			},
			pending: store.Pending{
				Seq: 1, Rev: 0,
				Sent:   ops(ins("Hello", nil)),
				Buffer: ops(ret(5, nil), ins(" World", nil)),
			},
			restores: 2,
			want:     ops(ins("Hi Hello World\n", nil)),
			wantRev:  3,
		},
		// The buffer was made before the repair that follows sent. The repair
		// takes priority over it, as it would have for the editor.
		"after a repair": {
			before: func(submit func(string, int, int, delta.Delta)) {
				submit("b", 1, 0, ops(ins("x", nil)))
				submit("b", 2, 1, ops(ret(1, nil), ret(1, attrs{"header": 1.0})))
			},
			pending: store.Pending{
				Seq: 1, Rev: 1,
				Sent: ops(ret(1, nil), ret(1, attrs{"list": "bullet"})),
				// Quill replaces a line type with another.
				Buffer: ops(ret(1, nil), ret(1, attrs{"header": 2.0, "list": nil})),
			},
			restores: 1,
			want:     ops(ins("x", nil), ins("\n", attrs{"list": "bullet"})),
			wantRev:  5,
		},
	} {
		t.Run(name, func(t *testing.T) {
			s := open(t, t.TempDir())
			id := create(t, s)
			clients := map[string]string{"a": store.NewClient(), "b": store.NewClient()}
			tc.before(func(client string, seq, rev int, change delta.Delta) {
				require.NoError(t, s.Submit(id, store.Submission{
					Client: clients[client], Seq: seq, Rev: rev, Delta: change,
				}))
			})
			tc.pending.Client = clients["a"]
			for range tc.restores {
				require.NoError(t, s.Restore(id, tc.pending))
			}
			doc, rev := content(t, s, id)
			require.Equal(t, tc.want, doc)
			require.Equal(t, tc.wantRev, rev)
		})
	}
}

// TestRestoreStale tests that changes made at a revision older than the
// history are refused.
func TestRestoreStale(t *testing.T) {
	s := open(t, t.TempDir())
	id := create(t, s)
	a, b := store.NewClient(), store.NewClient()
	for i := range 1001 {
		require.NoError(t, s.Submit(id, store.Submission{
			Client: b, Seq: i + 1, Rev: i, Delta: ops(ins("x", nil)),
		}))
	}
	err := s.Restore(id, store.Pending{
		Client: a,
		Seq:    1,
		Rev:    0,
		Sent:   ops(ins("y", nil)),
	})
	require.ErrorIs(t, err, store.ErrStale)
}

// TestReopen tests that a document survives a restart with its history,
// after [store.Store.Close] and after a crash.
func TestReopen(t *testing.T) {
	for name, closing := range map[string]func(*store.Store){
		"close": func(s *store.Store) { require.NoError(t, s.Close()) },
		"crash": func(*store.Store) {},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			s, err := store.Open(dir)
			require.NoError(t, err)
			id := create(t, s)
			a := store.NewClient()
			for i, text := range []string{"a", "b", "c"} {
				require.NoError(t, s.Submit(id, store.Submission{
					Client: a, Seq: i + 1, Rev: i, Delta: ops(ins(text, nil)),
				}))
			}
			closing(s)

			s = open(t, dir)
			doc, rev := content(t, s, id)
			require.Equal(t, 3, rev)
			require.Equal(t, ops(ins("cba\n", nil)), doc)
			v, err := s.View(id, a, 0)
			require.NoError(t, err)
			require.Len(t, v.Changes, 3)
		})
	}
}

// TestReopenPartialLine tests that a partial last log line from a crash is
// dropped and the next change follows the last complete one.
func TestReopenPartialLine(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Open(dir)
	require.NoError(t, err)
	id := create(t, s)
	a := store.NewClient()
	require.NoError(t, s.Submit(id, store.Submission{
		Client: a, Seq: 1, Rev: 0, Delta: ops(ins("a", nil)),
	}))
	f, err := os.OpenFile(filepath.Join(dir, id+".log"), os.O_WRONLY|os.O_APPEND, 0)
	require.NoError(t, err)
	_, err = f.WriteString(`{"rev":2,"delta":{"ops":[{"ins`)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	s = open(t, dir)
	require.NoError(t, s.Submit(id, store.Submission{
		Client: a, Seq: 2, Rev: 1, Delta: ops(ret(1, nil), ins("b", nil)),
	}))
	require.NoError(t, s.Close())

	s = open(t, dir)
	doc, rev := content(t, s, id)
	require.Equal(t, 2, rev)
	require.Equal(t, ops(ins("ab\n", nil)), doc)
}

// TestCompaction tests that a long log becomes a snapshot and that the log
// keeps the last changes before it for the history after a restart.
func TestCompaction(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Open(dir)
	require.NoError(t, err)
	id := create(t, s)
	a := store.NewClient()
	for i := range 250 {
		require.NoError(t, s.Submit(id, store.Submission{
			Client: a, Seq: i + 1, Rev: i, Delta: ops(ins("x", nil)),
		}))
	}
	require.NoError(t, s.Close())
	// The snapshot at 200 kept revisions 101 to 200. 201 to 250 followed.
	log, err := os.ReadFile(filepath.Join(dir, id+".log"))
	require.NoError(t, err)
	require.Equal(t, 150, strings.Count(string(log), "\n"))

	s = open(t, dir)
	doc, rev := content(t, s, id)
	require.Equal(t, 250, rev)
	require.Equal(t, 251, doc.Length())
	_, err = s.View(id, a, 100)
	require.NoError(t, err)
	_, err = s.View(id, a, 99)
	require.ErrorIs(t, err, store.ErrStale)
}

// TestStale tests that a revision older than the history is refused.
func TestStale(t *testing.T) {
	s := open(t, t.TempDir())
	id := create(t, s)
	a := store.NewClient()
	for i := range 1001 {
		require.NoError(t, s.Submit(id, store.Submission{
			Client: a, Seq: i + 1, Rev: i, Delta: ops(ins("x", nil)),
		}))
	}
	_, err := s.View(id, a, 0)
	require.ErrorIs(t, err, store.ErrStale)
	_, err = s.View(id, a, 1)
	require.NoError(t, err)
	err = s.Submit(id, store.Submission{Client: a, Seq: 2000, Rev: 0, Delta: ops(ins("x", nil))})
	require.ErrorIs(t, err, store.ErrStale)
}

// TestPresence tests who a view lists, and that a reconnect opening its
// stream before the old one closes keeps the editor listed.
func TestPresence(t *testing.T) {
	s := open(t, t.TempDir())
	id := create(t, s)
	a, b := store.NewClient(), store.NewClient()
	require.NoError(t, s.Join(id, a))
	require.NoError(t, s.Join(id, b))
	require.NoError(t, s.Join(id, a))
	require.NoError(t, s.Leave(id, a))

	v, err := s.View(id, b, 0)
	require.NoError(t, err)
	require.Len(t, v.Collaborators, 2)
	// a joined first.
	require.False(t, v.Collaborators[0].You)
	require.True(t, v.Collaborators[1].You)
	presence := v.Presence

	require.NoError(t, s.Leave(id, a))
	v, err = s.View(id, b, 0)
	require.NoError(t, err)
	require.Len(t, v.Collaborators, 1)
	require.Greater(t, v.Presence, presence)
}

// TestCursors tests that cursors move with the text: past an insert of their
// own editor at the cursor, not past one of another editor.
func TestCursors(t *testing.T) {
	s := open(t, t.TempDir())
	id := create(t, s)
	a, b := store.NewClient(), store.NewClient()
	require.NoError(t, s.Join(id, a))
	require.NoError(t, s.Join(id, b))
	require.NoError(t, s.Submit(id, store.Submission{
		Client: a, Seq: 1, Rev: 0, Delta: ops(ins("hello world", nil)),
	}))

	moved, err := s.MoveCursor(id, a, 1, 1, store.Cursor{Index: 5})
	require.NoError(t, err)
	require.True(t, moved)
	// b selects "world" before seeing the next change of a.
	require.NoError(t, s.Submit(id, store.Submission{
		Client: a, Seq: 2, Rev: 1, Delta: ops(ret(5, nil), ins(",", nil)),
	}))
	moved, err = s.MoveCursor(id, b, 1, 1, store.Cursor{Index: 6, Length: 5})
	require.NoError(t, err)
	require.True(t, moved)
	// A cursor older than the last one is dropped.
	moved, err = s.MoveCursor(id, b, 1, 2, store.Cursor{Index: 0})
	require.NoError(t, err)
	require.False(t, moved)

	// b types at the cursor of a.
	require.NoError(t, s.Submit(id, store.Submission{
		Client: b, Seq: 1, Rev: 2, Delta: ops(ret(6, nil), ins("!", nil)),
	}))
	v, err := s.View(id, a, 0)
	require.NoError(t, err)
	require.Equal(t, "hello,! world\n", plain(t, s, id))
	cursors := map[bool]*store.Cursor{}
	for _, c := range v.Collaborators {
		cursors[c.You] = c.Cursor
	}
	require.Equal(t, &store.Cursor{Index: 6}, cursors[true], "cursor of a")
	require.Equal(t, &store.Cursor{Index: 8, Length: 5}, cursors[false], "cursor of b")
}

func plain(t *testing.T, s *store.Store, id string) string {
	t.Helper()
	doc, _ := content(t, s, id)
	var b strings.Builder
	for _, op := range doc.Ops {
		b.WriteString(op.Insert)
	}
	return b.String()
}

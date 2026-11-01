// Tests the editing protocol over HTTP: tabs that submit changes and receive
// them on their streams, transformed, acknowledged and in order.

package app_test

import (
	"cmp"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/net/html"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/datapagestest"
	"github.com/romshark/datapages/example/docs/app"
	"github.com/romshark/datapages/example/docs/app/datapagesgen"
	"github.com/romshark/datapages/example/docs/app/datapagesgen/action"
	"github.com/romshark/datapages/example/docs/app/datapagesgen/href"
	"github.com/romshark/datapages/example/docs/delta"
	"github.com/romshark/datapages/example/docs/images"
	"github.com/romshark/datapages/example/docs/store"
	"github.com/romshark/datapages/example/docs/templates"
	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/messaging/inmem"
)

// server is a datapagestest server of the app, with the store the app reads and the
// browser whose tabs open the documents.
type server struct {
	*datapagestest.Server
	docs    *store.Store
	browser *datapagestest.Visitor
}

func newServer(t *testing.T) *server {
	t.Helper()
	docs, err := store.Open(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { _ = docs.Close() })
	imgs, err := images.Open(t.TempDir())
	require.NoError(t, err)
	a := app.NewApp(docs, imgs)
	s, err := datapages.NewServer[
		app.App,
		datapages.DisableSessions,
		datapages.DisablePrometheus,
		datapagesgen.Server,
	](a, inmem.New(messaging.DefaultBrokerChanBuffer),
		datapages.WithAssets(app.StaticFS, false),
		datapages.WithBodySizeLimit(app.BodySizeLimit))
	require.NoError(t, err)
	srv := datapagestest.New(t, s)
	return &server{Server: srv, docs: docs, browser: srv.Visitor(t)}
}

func (s *server) create(t *testing.T) string {
	t.Helper()
	id, err := s.docs.Create("", templates.Blank.Content)
	require.NoError(t, err)
	return id
}

func (s *server) text(t *testing.T, doc string) string {
	t.Helper()
	snap, err := s.docs.Get(doc)
	require.NoError(t, err)
	var b strings.Builder
	for _, op := range snap.Content.Ops {
		b.WriteString(op.Insert)
	}
	return b.String()
}

// tab holds the page of a document, its stream and the client id the page passes
// to the editor, which sends it with each change.
type tab struct {
	*datapagestest.Tab
	stream *datapagestest.Stream
	doc    string
	client string
}

// open loads the page of doc in a new tab and connects its stream.
func (s *server) open(t *testing.T, doc string) *tab {
	t.Helper()
	page := s.browser.Load(t, href.PageDoc(doc))
	require.NotEmpty(t, page.Instance(), "the page sets no Datapages-Instance")
	signals := page.Page().PublicSignals(t)
	client, _ := signals["client"].(string)
	require.NotEmpty(t, client, "the page sets no client signal")
	require.Contains(t, signals, "rev", "the page sets no rev signal")
	return &tab{Tab: page, stream: page.Connect(t, signals), doc: doc, client: client}
}

// reconnect opens a new stream of the tab with rev,
// the revision the editor reports in the rev signal.
func (tb *tab) reconnect(t *testing.T, rev int) {
	t.Helper()
	signals := tb.Page().PublicSignals(t)
	signals["rev"] = rev
	tb.stream = tb.Connect(t, signals)
}

// ops returns the delta of the operations in the JSON array list.
func ops(list string) json.RawMessage {
	return json.RawMessage(`{"ops":` + list + `}`)
}

// submit sends a change the way the editor does.
func (tb *tab) submit(t *testing.T, seq, rev int, list string) *datapagestest.Response {
	t.Helper()
	resp := tb.Do(t, action.PageDoc.Submit.POST(tb.doc), map[string]any{
		"client": tb.client, "seq": seq, "rev": rev, "delta": ops(list),
	})
	require.Equal(t, http.StatusOK, resp.Status, resp.Body)
	return resp
}

// restore sends what the closed tab of client kept, the way the editor does.
func (tb *tab) restore(
	t *testing.T, client string, seq, rev int, sent, buffer, content string,
) *datapagestest.Response {
	t.Helper()
	resp := tb.Do(t, action.PageDoc.Restore.POST(tb.doc), map[string]any{
		"client": client, "seq": seq, "rev": rev,
		"sent": ops(sent), "buffer": ops(buffer),
		"title": "Notes", "content": ops(content),
	})
	require.Equal(t, http.StatusOK, resp.Status, resp.Body)
	return resp
}

// editorCall returns the argument that the script of ev passes to method of the
// editor element, and false when ev calls no such method.
func editorCall(ev datapagestest.Event, method string) (json.RawMessage, bool) {
	script, ok := ev.Script()
	if !ok {
		return nil, false
	}
	_, arg, ok := strings.Cut(script, `document.getElementById("editor").`+method+"(")
	if !ok {
		return nil, false
	}
	// The app wraps the call in customElements.whenDefined(...).then(...).
	// The last two parentheses close the call of method and the call of then.
	if arg, ok = strings.CutSuffix(arg, "))"); !ok {
		return nil, false
	}
	return json.RawMessage(arg), true
}

// calls matches the script that calls method of the editor.
func calls(method string) datapagestest.Match {
	return datapagestest.Func("a call of "+method+" of the editor",
		func(ev datapagestest.Event) bool {
			_, ok := editorCall(ev, method)
			return ok
		})
}

// callArg returns the argument of the first call of method of the editor among the
// events of resp and fails the test when none calls it.
func callArg(t *testing.T, resp *datapagestest.Response, method string) json.RawMessage {
	t.Helper()
	arg, _ := editorCall(resp.Event(t, calls(method)), method)
	return arg
}

// update is the argument of receive of the editor.
// Changes keeps each change in the JSON the server writes.
type update struct {
	Changes []json.RawMessage `json:"changes"`
	Cursors []cursor          `json:"cursors"`
}

type cursor struct {
	Name   string `json:"name"`
	Color  string `json:"color"`
	Index  int    `json:"index"`
	Length int    `json:"length"`
}

// receivesUpdate matches the call of receive of the editor with
// an update that ok accepts. what names the update in failure messages.
func receivesUpdate(what string, ok func(update) bool) datapagestest.Match {
	return datapagestest.Func(what, func(ev datapagestest.Event) bool {
		arg, found := editorCall(ev, "receive")
		var u update
		return found && json.Unmarshal(arg, &u) == nil && ok(u)
	})
}

// receives matches an update that carries each of changes.
// A change must equal the JSON the server writes, byte for byte.
func receives(changes ...string) datapagestest.Match {
	return receivesUpdate("an update with "+strings.Join(changes, " and "),
		func(u update) bool {
			for _, want := range changes {
				if !slices.ContainsFunc(u.Changes, func(c json.RawMessage) bool {
					return string(c) == want
				}) {
					return false
				}
			}
			return true
		})
}

// collaborators matches a patch of the collaborator list whose avatars have the
// titles, in this order, and no others.
func collaborators(titles ...string) datapagestest.Match {
	return datapagestest.Func(fmt.Sprintf("a collaborator list of %q", titles),
		func(ev datapagestest.Event) bool {
			list := datapagestest.ElementByID(ev.Root(), "collaborators")
			if list == nil {
				return false
			}
			var got []string
			for n := range list.ChildNodes() {
				if title, ok := datapagestest.Attr(n, "title"); ok {
					got = append(got, title)
				}
			}
			return slices.Equal(got, titles)
		})
}

// links returns the targets of the links under n.
func links(n *html.Node) []string {
	var targets []string
	for d := range n.Descendants() {
		if d.Type == html.ElementNode && d.Data == "a" {
			target, _ := datapagestest.Attr(d, "href")
			targets = append(targets, target)
		}
	}
	return targets
}

// TestEditTogether tests that 2 tabs typing at once receive each change in
// the order the server applied it, the change of the other transformed,
// and their own as the acknowledgment.
func TestEditTogether(t *testing.T) {
	s := newServer(t)
	doc := s.create(t)
	a, b := s.open(t, doc), s.open(t, doc)

	a.submit(t, 1, 0, `[{"insert":"Hello"}]`)
	a.stream.WaitFor(t,
		receives(`{"rev":1,"delta":{"ops":[{"insert":"Hello"}]},"ack":1}`))
	b.stream.WaitFor(t,
		receives(`{"rev":1,"delta":{"ops":[{"insert":"Hello"}]}}`))

	// b typed at the start before "Hello" reached it.
	b.submit(t, 1, 0, `[{"insert":"World"}]`)
	change := `{"rev":2,"delta":{"ops":[{"retain":5},{"insert":"World"}]}`
	b.stream.WaitFor(t, receives(change+`,"ack":1}`))
	a.stream.WaitFor(t, receives(change+`}`))
	require.Equal(t, "HelloWorld\n", s.text(t, doc))
}

// TestSubmitRetry tests that a change sent 2 times is applied once.
func TestSubmitRetry(t *testing.T) {
	s := newServer(t)
	doc := s.create(t)
	a := s.open(t, doc)
	a.submit(t, 1, 0, `[{"insert":"x"}]`)
	a.submit(t, 1, 0, `[{"insert":"x"}]`)
	a.stream.WaitFor(t, receives(`{"rev":1,"delta":{"ops":[{"insert":"x"}]},"ack":1}`))
	require.Equal(t, "x\n", s.text(t, doc))
}

// TestRejectedChange tests that a change the server cannot apply stops the
// editor that sent it.
func TestRejectedChange(t *testing.T) {
	s := newServer(t)
	doc := s.create(t)
	a := s.open(t, doc)
	for name, change := range map[string]string{
		"past the end":   `[{"retain":5},{"insert":"x"}]`,
		"unknown format": `[{"insert":"x","attributes":{"size":"huge"}}]`,
		"embed":          `[{"insert":{"image":"x.png"}}]`,
	} {
		t.Run(name, func(t *testing.T) {
			a.submit(t, 1, 0, change).Event(t, calls("stop"))
		})
	}
	require.Equal(t, "\n", s.text(t, doc))
}

// TestPresence tests that a tab sees who else has the document open.
func TestPresence(t *testing.T) {
	s := newServer(t)
	doc := s.create(t)
	a := s.open(t, doc)
	me := store.Identity(a.client).Name
	// a joins before b, which fixes the order of the avatars.
	a.stream.WaitFor(t, collaborators(me+" (you)"))
	b := s.open(t, doc)
	a.stream.WaitFor(t, collaborators(me+" (you)", store.Identity(b.client).Name))

	// After b leaves, a is last.
	b.stream.Close()
	ev := a.stream.WaitFor(t, collaborators(me+" (you)"))
	initial := me[strings.LastIndexByte(me, ' ')+1:][:1]
	require.Equal(t, initial, ev.Get(t, datapagestest.ID("collaborators")).Text())
}

// TestCursor tests that a tab sees the cursor of another tab.
func TestCursor(t *testing.T) {
	s := newServer(t)
	doc := s.create(t)
	a, b := s.open(t, doc), s.open(t, doc)
	a.submit(t, 1, 0, `[{"insert":"Hello"}]`)
	b.stream.WaitFor(t,
		receives(`{"rev":1,"delta":{"ops":[{"insert":"Hello"}]}}`))
	// The editor reports its cursor at the revision it has, after the
	// acknowledgment. The server drops the cursor of a tab whose stream has not
	// joined the document yet.
	a.stream.WaitFor(t,
		receives(`{"rev":1,"delta":{"ops":[{"insert":"Hello"}]},"ack":1}`))

	resp := a.Do(t, action.PageDoc.Cursor.POST(doc), map[string]any{
		"client": a.client, "seq": 1, "rev": 1, "index": 2, "length": 3,
	})
	require.Equal(t, http.StatusOK, resp.Status)
	who := store.Identity(a.client)
	moved := cursor{Name: who.Name, Color: who.Color, Index: 2, Length: 3}
	b.stream.WaitFor(t, receivesUpdate("an update with the cursor of a",
		func(u update) bool { return slices.Contains(u.Cursors, moved) }))
}

// TestRename tests that a new title reaches every tab.
func TestRename(t *testing.T) {
	s := newServer(t)
	doc := s.create(t)
	a, b := s.open(t, doc), s.open(t, doc)
	resp := a.Do(t, action.PageDoc.Rename.POST(doc),
		map[string]string{"title": " Plans "})
	require.Equal(t, http.StatusOK, resp.Status)
	b.stream.WaitFor(t, datapagestest.Func(`the title field with the value "Plans"`,
		func(ev datapagestest.Event) bool {
			n := datapagestest.ElementByID(ev.Root(), "doc-title")
			if n == nil {
				return false
			}
			value, _ := datapagestest.Attr(n, "value")
			return value == "Plans"
		}))
	ev := b.stream.WaitFor(t, datapagestest.TextContains("head-title", "Plans"))
	require.Equal(t, "Plans - Docs", ev.Get(t, datapagestest.ID("head-title")).Text())
}

// TestDelete tests that the tabs of a deleted document leave it.
func TestDelete(t *testing.T) {
	s := newServer(t)
	doc := s.create(t)
	a := s.open(t, doc)
	resp := a.Do(t, action.PageIndex.Doc.DELETE(doc), nil)
	require.Equal(t, http.StatusOK, resp.Status)
	a.stream.WaitFor(t, datapagestest.Func("a redirect to the document list",
		func(ev datapagestest.Event) bool {
			target, ok := ev.Redirect()
			return ok && target == href.PageIndex()
		}))

	require.Equal(t, http.StatusNotFound, s.browser.Get(t, href.PageDoc(doc)).Status)
}

// TestReconnect tests that a reconnecting stream gets the changes after the
// revision the editor reports, its own acknowledged.
func TestReconnect(t *testing.T) {
	s := newServer(t)
	doc := s.create(t)
	a, b := s.open(t, doc), s.open(t, doc)
	a.stream.Close()
	a.submit(t, 1, 0, `[{"insert":"a"}]`)
	b.submit(t, 1, 1, `[{"retain":1},{"insert":"b"}]`)
	b.stream.WaitFor(t,
		receives(`{"rev":2,"delta":{"ops":[{"retain":1},{"insert":"b"}]},"ack":1}`))

	a.reconnect(t, 0)
	a.stream.WaitFor(t, receives(
		`{"rev":1,"delta":{"ops":[{"insert":"a"}]},"ack":1}`,
		`{"rev":2,"delta":{"ops":[{"retain":1},{"insert":"b"}]}}`,
	))
}

// TestRestore tests that the changes a closed tab kept reach the open tabs as
// changes of another editor, without the one the server applied before the tab closed.
func TestRestore(t *testing.T) {
	s := newServer(t)
	doc := s.create(t)
	closed, b := s.open(t, doc), s.open(t, doc)
	closed.submit(t, 1, 0, `[{"insert":"Hello"}]`)
	closed.stream.Close()

	resp := b.restore(t, closed.client, 1, 0,
		`[{"insert":"Hello"}]`, `[{"retain":5},{"insert":" World"}]`, `[{"insert":"Hello World\n"}]`)
	var restored string
	require.NoError(t, json.Unmarshal(callArg(t, resp, "restored"), &restored))
	require.Equal(t, closed.client, restored)
	b.stream.WaitFor(t,
		receives(`{"rev":2,"delta":{"ops":[{"retain":5},{"insert":" World"}]}}`))
	require.Equal(t, "Hello World\n", s.text(t, doc))
}

// TestRestoreCopy tests that changes made at a revision the server no longer
// has go to a new document with the content the closed tab showed.
func TestRestoreCopy(t *testing.T) {
	s := newServer(t)
	doc := s.create(t)
	b := s.open(t, doc)
	other := store.NewClient()
	for i := range 1001 {
		require.NoError(t, s.docs.Submit(doc, store.Submission{
			Client: other,
			Seq:    i + 1,
			Rev:    i,
			Delta:  delta.Delta{Ops: []delta.Op{{Insert: "x"}}},
		}))
	}

	resp := b.restore(t, store.NewClient(), 1, 0,
		`[{"insert":"Mine"}]`, `[]`, `[{"insert":"Mine\n"}]`)
	resp.Event(t, calls("restored"))
	var copied store.Summary
	for _, d := range s.docs.List() {
		if d.ID != doc {
			copied = d
		}
	}
	require.Equal(t, "Notes (offline copy)", copied.Title)
	require.Equal(t, "Mine\n", s.text(t, copied.ID))
	toast := resp.Event(t,
		datapagestest.Patched("error")).Get(t, datapagestest.ID("error"))
	require.Equal(t, []string{href.PageDoc(copied.ID)}, links(toast.Node()))
	require.Equal(t, 1001, len(s.text(t, doc))-1)
}

// TestCreate tests that a document created from a template has its title and
// content and opens in the tab that created it.
func TestCreate(t *testing.T) {
	for _, tmpl := range templates.All {
		t.Run(tmpl.ID, func(t *testing.T) {
			s := newServer(t)
			index := s.browser.Load(t, href.PageIndex())
			resp := index.Do(t, action.PageIndex.Create.POST(tmpl.ID), nil)
			require.Equal(t, http.StatusOK, resp.Status)
			list := s.docs.List()
			require.Len(t, list, 1)
			require.Equal(t, href.PageDoc(list[0].ID), resp.Redirect(t))
			snap, err := s.docs.Get(list[0].ID)
			require.NoError(t, err)
			require.Equal(t, tmpl.Content, snap.Content)
			require.Equal(t, cmp.Or(tmpl.Title, store.DefaultTitle), snap.Title)
		})
	}
}

// TestCreateUnknownTemplate tests that an unknown template creates nothing
// and shows the error toast.
func TestCreateUnknownTemplate(t *testing.T) {
	s := newServer(t)
	index := s.browser.Load(t, href.PageIndex())
	resp := index.Do(t, action.PageIndex.Create.POST("missing"), nil)
	require.Equal(t, http.StatusOK, resp.Status)
	resp.Event(t, datapagestest.TextContains("error", "Something went wrong"))
	require.Empty(t, s.docs.List())
}

// pngData starts with the PNG signature, which is all the server checks.
const pngData = "\x89PNG\r\n\x1a\nimage"

// TestImage tests that an uploaded image reaches the editor that sent it as a
// URL path the server serves and a change can insert.
func TestImage(t *testing.T) {
	s := newServer(t)
	doc := s.create(t)
	a := s.open(t, doc)
	resp := a.Do(t, action.PageDoc.Image.POST(doc), map[string]any{
		"upload": 7, "data": base64.StdEncoding.EncodeToString([]byte(pngData)),
	})
	require.Equal(t, http.StatusOK, resp.Status)
	var upload struct {
		Upload int    `json:"upload"`
		Src    string `json:"src"`
	}
	require.NoError(t, json.Unmarshal(callArg(t, resp, "insertImage"), &upload))
	require.Equal(t, 7, upload.Upload)
	require.Regexp(t, `^/images/[0-9a-f]{64}\.png$`, upload.Src)
	src := upload.Src

	img := s.browser.Get(t, src)
	require.Equal(t, http.StatusOK, img.Status)
	require.Equal(t, "image/png", img.Header.Get("Content-Type"))
	require.Equal(t, pngData, img.Body)

	a.submit(t, 1, 0, `[{"insert":{"image":"`+src+`"}}]`)
	snap, err := s.docs.Get(doc)
	require.NoError(t, err)
	require.Equal(t, delta.Embed{Type: "image", Value: src}, snap.Content.Ops[0].Embed)
}

// TestImageRejected tests that a file the server refuses reaches the editor as
// an empty URL path, with a toast that says why.
func TestImageRejected(t *testing.T) {
	s := newServer(t)
	doc := s.create(t)
	a := s.open(t, doc)
	resp := a.Do(t, action.PageDoc.Image.POST(doc), map[string]any{
		"upload": 3, "data": base64.StdEncoding.EncodeToString([]byte("not an image")),
	})
	require.Equal(t, http.StatusOK, resp.Status)
	resp.Event(t, datapagestest.TextContains("error", "not a PNG, JPEG, GIF or WebP image"))
	require.JSONEq(t, `{"upload":3,"src":""}`, string(callArg(t, resp, "insertImage")))
}

// TestTemplatePicture tests that the server serves the picture of a template
// from the program, with nothing in the image store.
func TestTemplatePicture(t *testing.T) {
	s := newServer(t)
	travel, ok := templates.Get("travel")
	require.True(t, ok)
	src := ""
	for _, op := range travel.Content.Ops {
		if op.IsEmbed() {
			src = op.Embed.Value
		}
	}
	require.NotEmpty(t, src)
	resp := s.browser.Get(t, src)
	require.Equal(t, http.StatusOK, resp.Status)
	require.Equal(t, "image/jpeg", resp.Header.Get("Content-Type"))
	want, _ := templates.Picture(src)
	require.Equal(t, want, []byte(resp.Body))
}

// TestImageMissing tests that the URL path of an image nobody uploaded is a 404.
func TestImageMissing(t *testing.T) {
	s := newServer(t)
	resp := s.browser.Get(t, href.App.Image(strings.Repeat("0", 64)+".png"))
	require.Equal(t, http.StatusNotFound, resp.Status)
}

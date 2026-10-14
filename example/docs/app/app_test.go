// Tests the editing protocol over HTTP: tabs that submit changes and receive
// them on their streams, transformed, acknowledged and in order.

package app_test

import (
	"cmp"
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/example/docs/app"
	"github.com/romshark/datapages/example/docs/app/datapagesgen"
	"github.com/romshark/datapages/example/docs/app/datapagesgen/href"
	"github.com/romshark/datapages/example/docs/delta"
	"github.com/romshark/datapages/example/docs/images"
	"github.com/romshark/datapages/example/docs/store"
	"github.com/romshark/datapages/example/docs/templates"
	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/messaging/inmem"
)

type server struct {
	srv  *httptest.Server
	docs *store.Store
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
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)
	return &server{srv: srv, docs: docs}
}

func (s *server) create(t *testing.T) string {
	t.Helper()
	id, err := s.docs.Create("", templates.Blank.Content)
	require.NoError(t, err)
	return id
}

// tab is a browser tab with a document open: the instance from its page load,
// the client id of its editor and what its stream delivered.
type tab struct {
	s                     *server
	doc, instance, client string
	rev                   string
	close                 context.CancelFunc

	mu     sync.Mutex
	stream strings.Builder
}

var (
	clientSignal = regexp.MustCompile(`data-signals:client="&#34;([A-Za-z0-9_-]+)&#34;"`)
	revSignal    = regexp.MustCompile(`data-signals:rev="(\d+)"`)
)

// open loads the page of a document and connects its stream.
func (s *server) open(t *testing.T, doc string) *tab {
	t.Helper()
	resp, err := s.srv.Client().Get(s.srv.URL + "/doc/" + doc)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	page, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	client := clientSignal.FindSubmatch(page)
	require.NotNil(t, client, "the page sets no client signal")
	rev := revSignal.FindSubmatch(page)
	require.NotNil(t, rev, "the page sets no rev signal")

	tb := &tab{
		s: s, doc: doc, instance: resp.Header.Get("Datapages-Instance"),
		client: string(client[1]), rev: string(rev[1]),
	}
	require.NotEmpty(t, tb.instance)
	tb.connect(t, tb.rev)
	return tb
}

// connect opens the stream with the page signals and rev, the revision the
// editor reports.
func (tb *tab) connect(t *testing.T, rev string) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	tb.close = cancel
	t.Cleanup(cancel)
	signals := `{"docId":"` + tb.doc + `","client":"` + tb.client + `","rev":` + rev + `}`
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		tb.s.srv.URL+"/doc/"+tb.doc+"/_$/?datastar="+url.QueryEscape(signals), nil)
	require.NoError(t, err)
	req.Header.Set("Datastar-Request", "true")
	req.Header.Set("Datapages-Instance", tb.instance)
	req.Header.Set("Accept-Encoding", "identity")
	resp, err := tb.s.srv.Client().Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	t.Cleanup(func() { _ = resp.Body.Close() })
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := resp.Body.Read(buf)
			tb.mu.Lock()
			tb.stream.Write(buf[:n])
			tb.mu.Unlock()
			if err != nil {
				return
			}
		}
	}()
}

// at is how much of the stream arrived so far.
func (tb *tab) at() int {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	return tb.stream.Len()
}

// waitFor returns what the stream delivered after from once it contains want.
func (tb *tab) waitFor(t *testing.T, from int, want string) string {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		tb.mu.Lock()
		got := tb.stream.String()[from:]
		tb.mu.Unlock()
		if strings.Contains(got, want) {
			return got
		}
		if time.Now().After(deadline) {
			require.Contains(t, got, want, "the stream never delivered it")
			return got
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// action sends a request like a Datastar action with a payload.
func (tb *tab) action(t *testing.T, method, path, body string) (int, string) {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), method,
		tb.s.srv.URL+path, strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Datastar-Request", "true")
	if tb.instance != "" {
		req.Header.Set("Datapages-Instance", tb.instance)
	}
	req.Header.Set("Content-Type", "application/json")
	// Keep assertions against the uncompressed handler output.
	req.Header.Set("Accept-Encoding", "identity")
	resp, err := tb.s.srv.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, string(b)
}

// submit sends a change the way the editor does.
func (tb *tab) submit(t *testing.T, seq, rev, ops string) string {
	t.Helper()
	status, body := tb.action(t, http.MethodPost, "/doc/"+tb.doc+"/submit",
		`{"client":"`+tb.client+`","seq":`+seq+`,"rev":`+rev+`,"delta":{"ops":`+ops+`}}`)
	require.Equal(t, http.StatusOK, status, body)
	return body
}

// restore sends what the closed tab of client kept, the way the editor does.
func (tb *tab) restore(
	t *testing.T, client, seq, rev, sent, buffer, content string,
) string {
	t.Helper()
	status, body := tb.action(t, http.MethodPost, "/doc/"+tb.doc+"/restore",
		`{"client":"`+client+`","seq":`+seq+`,"rev":`+rev+
			`,"sent":{"ops":`+sent+`},"buffer":{"ops":`+buffer+`}`+
			`,"title":"Notes","content":{"ops":`+content+`}}`)
	require.Equal(t, http.StatusOK, status, body)
	return body
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

// TestEditTogether tests that 2 tabs typing at once receive each change in
// the order the server applied it, the change of the other transformed, and
// their own as the acknowledgment.
func TestEditTogether(t *testing.T) {
	s := newServer(t)
	doc := s.create(t)
	a, b := s.open(t, doc), s.open(t, doc)

	fromA, fromB := a.at(), b.at()
	a.submit(t, "1", "0", `[{"insert":"Hello"}]`)
	a.waitFor(t, fromA, `{"rev":1,"delta":{"ops":[{"insert":"Hello"}]},"ack":1}`)
	b.waitFor(t, fromB, `{"rev":1,"delta":{"ops":[{"insert":"Hello"}]}}`)

	// b typed at the start before "Hello" reached it.
	fromA, fromB = a.at(), b.at()
	b.submit(t, "1", "0", `[{"insert":"World"}]`)
	change := `{"rev":2,"delta":{"ops":[{"retain":5},{"insert":"World"}]}`
	b.waitFor(t, fromB, change+`,"ack":1}`)
	a.waitFor(t, fromA, change+`}`)
	require.Equal(t, "HelloWorld\n", s.text(t, doc))
}

// TestSubmitRetry tests that a change sent 2 times is applied once.
func TestSubmitRetry(t *testing.T) {
	s := newServer(t)
	doc := s.create(t)
	a := s.open(t, doc)
	a.submit(t, "1", "0", `[{"insert":"x"}]`)
	a.submit(t, "1", "0", `[{"insert":"x"}]`)
	a.waitFor(t, 0, `"ack":1`)
	require.Equal(t, "x\n", s.text(t, doc))
}

// TestRejectedChange tests that a change the server cannot apply stops the
// editor that sent it.
func TestRejectedChange(t *testing.T) {
	s := newServer(t)
	doc := s.create(t)
	a := s.open(t, doc)
	for name, ops := range map[string]string{
		"past the end":   `[{"retain":5},{"insert":"x"}]`,
		"unknown format": `[{"insert":"x","attributes":{"size":"huge"}}]`,
		"embed":          `[{"insert":{"image":"x.png"}}]`,
	} {
		t.Run(name, func(t *testing.T) {
			require.Contains(t, a.submit(t, "1", "0", ops), `.stop(`)
		})
	}
	require.Equal(t, "\n", s.text(t, doc))
}

// TestPresence tests that a tab sees who else has the document open.
func TestPresence(t *testing.T) {
	s := newServer(t)
	doc := s.create(t)
	a := s.open(t, doc)
	from := a.at()
	b := s.open(t, doc)
	name := store.Identity(b.client).Name
	a.waitFor(t, from, `title="`+name+`"`)

	// After b leaves, a is last.
	from = a.at()
	b.close()
	me := store.Identity(a.client)
	initial := me.Name[strings.LastIndexByte(me.Name, ' ')+1:][:1]
	a.waitFor(t, from, `title="`+me.Name+` (you)">`+initial+`</span></div>`)
}

// TestCursor tests that a tab sees the cursor of another tab.
func TestCursor(t *testing.T) {
	s := newServer(t)
	doc := s.create(t)
	a, b := s.open(t, doc), s.open(t, doc)
	a.submit(t, "1", "0", `[{"insert":"Hello"}]`)
	b.waitFor(t, 0, `"rev":1`)

	from := b.at()
	status, _ := a.action(t, http.MethodPost, "/doc/"+doc+"/cursor",
		`{"client":"`+a.client+`","seq":1,"rev":1,"index":2,"length":3}`)
	require.Equal(t, http.StatusOK, status)
	b.waitFor(t, from, `"name":"`+store.Identity(a.client).Name+`"`)
	b.waitFor(t, from, `"index":2,"length":3`)
}

// TestRename tests that a new title reaches every tab.
func TestRename(t *testing.T) {
	s := newServer(t)
	doc := s.create(t)
	a, b := s.open(t, doc), s.open(t, doc)
	from := b.at()
	status, _ := a.action(t, http.MethodPost, "/doc/"+doc+"/rename", `{"title":" Plans "}`)
	require.Equal(t, http.StatusOK, status)
	b.waitFor(t, from, `value="Plans"`)
	b.waitFor(t, from, `Plans - Docs</title>`)
}

// TestDelete tests that the tabs of a deleted document leave it.
func TestDelete(t *testing.T) {
	s := newServer(t)
	doc := s.create(t)
	a := s.open(t, doc)
	from := a.at()
	status, _ := a.action(t, http.MethodDelete, "/delete/"+doc, "")
	require.Equal(t, http.StatusOK, status)
	a.waitFor(t, from, `window.location.href = "/"`)

	resp, err := s.srv.Client().Get(s.srv.URL + "/doc/" + doc)
	require.NoError(t, err)
	_ = resp.Body.Close()
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// TestReconnect tests that a reconnecting stream gets the changes after the
// revision the editor reports, its own acknowledged.
func TestReconnect(t *testing.T) {
	s := newServer(t)
	doc := s.create(t)
	a, b := s.open(t, doc), s.open(t, doc)
	a.close()
	a.submit(t, "1", "0", `[{"insert":"a"}]`)
	b.submit(t, "1", "1", `[{"retain":1},{"insert":"b"}]`)
	b.waitFor(t, 0, `"rev":2`)

	from := a.at()
	a.connect(t, "0")
	got := a.waitFor(t, from, `{"rev":2,"delta":{"ops":[{"retain":1},{"insert":"b"}]}}`)
	require.Contains(t, got, `{"rev":1,"delta":{"ops":[{"insert":"a"}]},"ack":1}`)
}

// TestRestore tests that the changes a closed tab kept reach the open tabs as
// changes of another editor, without the one the server applied before the tab closed.
func TestRestore(t *testing.T) {
	s := newServer(t)
	doc := s.create(t)
	closed, b := s.open(t, doc), s.open(t, doc)
	closed.submit(t, "1", "0", `[{"insert":"Hello"}]`)
	closed.close()

	from := b.at()
	body := b.restore(t, closed.client, "1", "0",
		`[{"insert":"Hello"}]`, `[{"retain":5},{"insert":" World"}]`, `[{"insert":"Hello World\n"}]`)
	require.Contains(t, body, `.restored("`+closed.client+`")`)
	b.waitFor(t, from, `{"rev":2,"delta":{"ops":[{"retain":5},{"insert":" World"}]}}`)
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

	body := b.restore(t, store.NewClient(), "1", "0",
		`[{"insert":"Mine"}]`, `[]`, `[{"insert":"Mine\n"}]`)
	require.Contains(t, body, `.restored(`)
	var copied store.Summary
	for _, d := range s.docs.List() {
		if d.ID != doc {
			copied = d
		}
	}
	require.Equal(t, "Notes (offline copy)", copied.Title)
	require.Equal(t, "Mine\n", s.text(t, copied.ID))
	require.Contains(t, body, href.PageDoc(copied.ID))
	require.Equal(t, 1001, len(s.text(t, doc))-1)
}

// TestCreate tests that a document created from a template has its title and
// content and opens in the tab that created it.
func TestCreate(t *testing.T) {
	for _, tmpl := range templates.All {
		t.Run(tmpl.ID, func(t *testing.T) {
			s := newServer(t)
			tb := &tab{s: s}
			status, body := tb.action(t, http.MethodPost, "/create/"+tmpl.ID, "")
			require.Equal(t, http.StatusOK, status)
			list := s.docs.List()
			require.Len(t, list, 1)
			require.Contains(t, body, "/doc/"+list[0].ID)
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
	tb := &tab{s: s}
	status, body := tb.action(t, http.MethodPost, "/create/missing", "")
	require.Equal(t, http.StatusOK, status)
	require.Contains(t, body, "Something went wrong")
	require.Empty(t, s.docs.List())
}

// pngData starts with the PNG signature, which is all the server checks.
const pngData = "\x89PNG\r\n\x1a\nimage"

var insertImage = regexp.MustCompile(
	`insertImage\(\{"upload":7,"src":"(/images/[0-9a-f]{64}\.png)"\}\)`,
)

// TestImage tests that an uploaded image reaches the editor that sent it as a
// URL path the server serves and a change can insert.
func TestImage(t *testing.T) {
	s := newServer(t)
	doc := s.create(t)
	a := s.open(t, doc)
	data := base64.StdEncoding.EncodeToString([]byte(pngData))
	status, body := a.action(t, http.MethodPost, "/doc/"+doc+"/image",
		`{"upload":7,"data":"`+data+`"}`)
	require.Equal(t, http.StatusOK, status)
	m := insertImage.FindStringSubmatch(body)
	require.NotNil(t, m, body)
	src := m[1]

	resp, err := s.srv.Client().Get(s.srv.URL + src)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "image/png", resp.Header.Get("Content-Type"))
	got, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, pngData, string(got))

	a.submit(t, "1", "0", `[{"insert":{"image":"`+src+`"}}]`)
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
	data := base64.StdEncoding.EncodeToString([]byte("not an image"))
	status, body := a.action(t, http.MethodPost, "/doc/"+doc+"/image",
		`{"upload":3,"data":"`+data+`"}`)
	require.Equal(t, http.StatusOK, status)
	require.Contains(t, body, "not a PNG, JPEG, GIF or WebP image")
	require.Contains(t, body, `insertImage({"upload":3,"src":""})`)
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
	resp, err := s.srv.Client().Get(s.srv.URL + src)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "image/jpeg", resp.Header.Get("Content-Type"))
	got, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	want, _ := templates.Picture(src)
	require.Equal(t, want, got)
}

// TestImageMissing tests that the URL path of an image nobody uploaded is a 404.
func TestImageMissing(t *testing.T) {
	s := newServer(t)
	resp, err := s.srv.Client().Get(s.srv.URL + href.App.Image(strings.Repeat("0", 64)+".png"))
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

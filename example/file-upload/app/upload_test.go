// Tests the upload protocol over HTTP: staging, the chunks, pause and resume,
// picking a file again for a transfer this browser lost, and the download.

package app_test

import (
	"context"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/example/file-upload/app"
	"github.com/romshark/datapages/example/file-upload/app/datapagesgen"
	"github.com/romshark/datapages/example/file-upload/store"
	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/messaging/inmem"
)

// contents is what the test uploads, in chunks the test chooses.
const contents = "0123456789abcdefghij"

// tab is one browser tab: the instance identifier its page load minted,
// the stream that keeps its per-tab state alive and what that stream delivered.
type tab struct {
	srv      *httptest.Server
	files    *store.Store
	app      *app.App
	instance string
	close    context.CancelFunc

	mu     sync.Mutex
	stream strings.Builder
}

func newTab(t *testing.T) *tab {
	t.Helper()
	return newTabWith(t, app.ChunkIdleTimeout, 0)
}

// newTabWith builds a server whose chunk requests end after idle of silence
// and whose requests are read within readTimeout. A zero readTimeout leaves
// the test server without one, as httptest does.
func newTabWith(t *testing.T, idle, readTimeout time.Duration) *tab {
	t.Helper()

	files, err := store.New(t.TempDir())
	require.NoError(t, err, "opening store")
	a := app.NewApp(files)
	s, err := datapages.NewServer[
		app.App,
		datapages.DisableSessions,
		datapages.DisablePrometheus,
		datapagesgen.Server,
	](a, inmem.New(messaging.DefaultBrokerChanBuffer),
		datapages.WithAssets(app.StaticFS, false),
		datapages.WithMiddleware(app.Downloads(a), app.ChunkDeadline(idle)))
	require.NoError(t, err, "building server")

	// Unstarted, because the read timeout belongs to the server that listens
	// and httptest builds its own rather than taking the one of the app.
	srv := httptest.NewUnstartedServer(s)
	srv.Config.ReadTimeout = readTimeout
	srv.Start()
	t.Cleanup(srv.Close)
	return open(t, srv, files, a)
}

// openTab loads the page in another tab of the same server,
// as a second browser window would.
func (tb *tab) openTab(t *testing.T) *tab {
	t.Helper()
	return open(t, tb.srv, tb.files, tb.app)
}

// open loads the page and connects the stream that allocates its per-tab state.
func open(t *testing.T, srv *httptest.Server, files *store.Store, a *app.App) *tab {
	t.Helper()
	page, err := srv.Client().Get(srv.URL + "/")
	require.NoError(t, err, "GET /")
	defer func() { _ = page.Body.Close() }()
	require.Equal(t, http.StatusOK, page.StatusCode, "GET /")
	instance := page.Header.Get("Datapages-Instance")
	require.NotEmpty(t, instance, "GET / minted no instance id")

	tb := &tab{srv: srv, files: files, app: a, instance: instance}
	tb.openStream(t)
	return tb
}

// openStream connects the page stream, which allocates the per-tab state.
// Cancelling it is the tab going away.
func (tb *tab) openStream(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	tb.close = cancel
	t.Cleanup(cancel)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, tb.srv.URL+"/_$/", nil)
	require.NoError(t, err, "building stream request")
	req.Header.Set("Datastar-Request", "true")
	req.Header.Set("Datapages-Instance", tb.instance)
	resp, err := tb.srv.Client().Do(req)
	require.NoError(t, err, "opening stream")
	require.Equal(t, http.StatusOK, resp.StatusCode, "opening stream")
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

// streamAt is how much of the stream arrived so far.
// Waiting from there ignores what an earlier step of the test delivered.
func (tb *tab) streamAt() int {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	return tb.stream.Len()
}

// waitForStream returns what the stream delivered once it contains want.
// Everything a tab receives arrives asynchronously through the broker.
func (tb *tab) waitForStream(t *testing.T, want string) string {
	t.Helper()
	return tb.waitForStreamFrom(t, 0, want)
}

// waitForStreamFrom is waitForStream over what arrived after from.
func (tb *tab) waitForStreamFrom(t *testing.T, from int, want string) string {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		tb.mu.Lock()
		got := tb.stream.String()
		tb.mu.Unlock()
		if len(got) > from {
			got = got[from:]
		} else {
			got = ""
		}
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

// action sends a request the way the Datastar client sends it.
// A failing one answers 200 with the message RecoverError patched into the page.
func (tb *tab) action(
	t *testing.T, method, path, contentType, body string,
) (int, string) {
	t.Helper()
	return tb.request(t, method, path, contentType, body, true)
}

// request sends one request, as Datastar or as
// the plain fetch the uploader uses for the chunks.
func (tb *tab) request(
	t *testing.T, method, path, contentType, body string, datastar bool,
) (int, string) {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req, err := http.NewRequestWithContext(
		context.Background(), method, tb.srv.URL+path, r,
	)
	require.NoError(t, err, "building %s %s", method, path)
	if datastar {
		req.Header.Set("Datastar-Request", "true")
	}
	req.Header.Set("Datapages-Instance", tb.instance)
	req.Header.Set("Accept-Encoding", "identity")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := tb.srv.Client().Do(req)
	require.NoError(t, err, "%s %s", method, path)
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "reading %s %s", method, path)
	return resp.StatusCode, string(b)
}

// chunk sends one chunk the way the uploader does, as a plain fetch.
func (tb *tab) chunk(t *testing.T, id string, offset int64, data string) int {
	t.Helper()
	status, _ := tb.request(t, http.MethodPut,
		"/upload/"+id+"/?offset="+strconv.FormatInt(offset, 10),
		"application/octet-stream", data, false)
	return status
}

// pause sends the action the Pause button sends.
func (tb *tab) pause(t *testing.T, id string) (int, string) {
	t.Helper()
	return tb.action(t, http.MethodPost, "/pause/"+id+"/", "", "")
}

// stage opens the upload dialog on one file and starts it under name.
func (tb *tab) start(t *testing.T, original, name, contentType string, size int) string {
	t.Helper()
	status, _ := tb.action(t, http.MethodPost, "/stage/", "application/json",
		`{"files":[{"name":"`+original+`","size":`+strconv.Itoa(size)+
			`,"type":"`+contentType+`"}]}`)
	require.Equal(t, http.StatusOK, status, "staging %s", original)

	from := tb.streamAt()
	status, _ = tb.action(t, http.MethodPost, "/start/",
		"application/x-www-form-urlencoded", "name-0="+url.QueryEscape(name))
	require.Equal(t, http.StatusOK, status, "starting %s", original)
	return idOfStartScript(t, tb.waitForStreamFrom(t, from, "dpUpload.start(["))
}

// TestUpload covers one file from the dialog to the download,
// interrupted by a pause and resumed, then deleted.
func TestUpload(t *testing.T) {
	tb := newTab(t)
	id := tb.start(t, "notes.txt", "renamed.txt", "text/plain", len(contents))

	f, err := tb.files.Get(id)
	require.NoError(t, err, "reading the created file")
	require.Equal(t, "renamed.txt", f.Name, "the dialog name is not kept")
	require.Equal(t, "notes.txt", f.OriginalName, "the browser name is not kept")
	require.Equal(t, "text/plain", f.ContentType)
	require.Equal(t, store.StatusUploading, f.Status)

	require.Equal(t, http.StatusOK, tb.chunk(t, id, 0, contents[:10]))
	f, err = tb.files.Get(id)
	require.NoError(t, err)
	require.Equal(t, int64(10), f.Received, "half the file did not arrive")
	require.Equal(t, 50, f.Percent())

	// A paused file refuses bytes, which is what stops an uploader that did
	// not see the pause yet.
	status, _ := tb.pause(t, id)
	require.Equal(t, http.StatusOK, status, "pausing")
	require.Equal(t, http.StatusForbidden, tb.chunk(t, id, 10, contents[10:]))

	status, body := tb.action(t, http.MethodPost, "/resume/"+id+"/", "", "")
	require.Equal(t, http.StatusOK, status, "resuming")
	require.Contains(t, body, `dpUpload.go({"id":"`+id+`"`,
		"resuming does not tell the uploader to continue")
	require.Contains(t, body, `"offset":10`,
		"resuming does not continue at the received bytes")

	require.Equal(t, http.StatusOK, tb.chunk(t, id, 10, contents[10:]))
	f, err = tb.files.Get(id)
	require.NoError(t, err)
	require.Equal(t, store.StatusComplete, f.Status, "the file did not complete")

	resp, err := tb.srv.Client().Get(tb.srv.URL + app.DownloadPrefix + id)
	require.NoError(t, err, "downloading")
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode, "downloading")
	require.Equal(t, "text/plain", resp.Header.Get("Content-Type"),
		"the download does not carry the type the browser reported")
	require.Equal(t, `attachment; filename=renamed.txt`,
		resp.Header.Get("Content-Disposition"),
		"the download does not name the file")
	got, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "reading the download")
	require.Equal(t, contents, string(got), "the download differs from what was sent")

	status, _ = tb.action(t, http.MethodDelete, "/files/"+id+"/", "", "")
	require.Equal(t, http.StatusOK, status, "deleting")
	resp, err = tb.srv.Client().Get(tb.srv.URL + app.DownloadPrefix + id)
	require.NoError(t, err, "downloading a deleted file")
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusNotFound, resp.StatusCode,
		"a deleted file is still being served")
}

// TestDownloadIncomplete tests that a file still receiving bytes has nothing
// to download, so that no partial blob can be taken for the whole file.
func TestDownloadIncomplete(t *testing.T) {
	tb := newTab(t)
	id := tb.start(t, "notes.txt", "notes.txt", "text/plain", len(contents))
	require.Equal(t, http.StatusOK, tb.chunk(t, id, 0, contents[:10]))

	resp, err := tb.srv.Client().Get(tb.srv.URL + app.DownloadPrefix + id)
	require.NoError(t, err, "downloading an incomplete file")
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusNotFound, resp.StatusCode)

	// The static files the app embeds keep being served next to the blobs.
	resp, err = tb.srv.Client().Get(tb.srv.URL + "/static/browser.js")
	require.NoError(t, err, "GET /static/browser.js")
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode,
		"the download middleware swallowed a static asset")
}

// TestChunkStatus covers what the uploader reacts to: a repeated chunk it may
// keep going after, a hole it must not, and a file that is gone.
func TestChunkStatus(t *testing.T) {
	tb := newTab(t)
	id := tb.start(t, "notes.txt", "notes.txt", "text/plain", len(contents))

	require.Equal(t, http.StatusConflict, tb.chunk(t, id, 5, contents[5:]),
		"a chunk that leaves a hole was accepted")
	require.Equal(t, http.StatusOK, tb.chunk(t, id, 0, contents[:10]))
	require.Equal(t, http.StatusOK, tb.chunk(t, id, 0, contents[:10]),
		"a repeated chunk was refused")

	f, err := tb.files.Get(id)
	require.NoError(t, err)
	require.Equal(t, int64(10), f.Received, "a repeated chunk was written twice")

	require.Equal(t, http.StatusNotFound, tb.chunk(t, "nosuchfile", 0, "x"))

	// A complete file takes nothing more.
	require.Equal(t, http.StatusOK, tb.chunk(t, id, 10, contents[10:]))
	require.Equal(t, http.StatusForbidden, tb.chunk(t, id, 20, "x"))
}

// TestReattach covers a transfer whose browser lost the file: the visitor
// picks it again and the server checks it before the bytes continue.
func TestReattach(t *testing.T) {
	tb := newTab(t)
	id := tb.start(t, "notes.txt", "renamed.txt", "text/plain", len(contents))
	require.Equal(t, http.StatusOK, tb.chunk(t, id, 0, contents[:10]))

	// A rejected reattachment answers with the message the visitor reads,
	// because RecoverError patches it into the page.
	for name, tc := range map[string]struct {
		body string
		want string
	}{
		"other name": {
			body: `{"id":"` + id + `","name":"other.txt","size":20}`,
			want: "&#34;other.txt&#34; of 20 bytes is not &#34;notes.txt&#34;",
		},
		"other size": {
			body: `{"id":"` + id + `","name":"notes.txt","size":21}`,
			want: "of 21 bytes is not &#34;notes.txt&#34; of 20 bytes",
		},
		"unknown file": {
			body: `{"id":"nosuchfile","name":"notes.txt","size":20}`,
			want: "file not found",
		},
		// The name the dialog chose is not what the browser reports for the
		// file the visitor picks again.
		"renamed file": {
			body: `{"id":"` + id + `","name":"renamed.txt","size":20}`,
			want: "&#34;renamed.txt&#34; of 20 bytes is not &#34;notes.txt&#34;",
		},
	} {
		t.Run(name, func(t *testing.T) {
			status, body := tb.action(t, http.MethodPost, "/reattach/",
				"application/json", tc.body)
			require.Equal(t, http.StatusOK, status)
			require.Contains(t, body, tc.want)
			require.Contains(t, body, "selector #toast",
				"the message did not reach the toaster")
			require.Contains(t, body, "mode append",
				"the message replaces the stack instead of joining it")
			require.Contains(t, body, `<span slot="title">`,
				"the message is in no slot and neo-toast projects none of it")
		})
	}

	status, body := tb.action(t, http.MethodPost, "/reattach/", "application/json",
		`{"id":"`+id+`","name":"notes.txt","size":20}`)
	require.Equal(t, http.StatusOK, status, "reattaching the picked file")
	require.Contains(t, body, `"offset":10`,
		"reattaching does not continue at the received bytes")
	require.Equal(t, http.StatusOK, tb.chunk(t, id, 10, contents[10:]))
}

// TestOwnership tests that only the tab holding the bytes may pause or resume
// a transfer. Every other tab is told so and is offered the take-over control instead,
// which asks for the file rather than posting anything.
func TestOwnership(t *testing.T) {
	tb := newTab(t)
	id := tb.start(t, "notes.txt", "notes.txt", "text/plain", len(contents))
	require.Equal(t, http.StatusOK, tb.chunk(t, id, 0, contents[:10]))

	other := tb.openTab(t)
	status, body := other.pause(t, id)
	require.Equal(t, http.StatusOK, status)
	require.Contains(t, body, "this tab does not hold the bytes of that file")

	f, err := tb.files.Get(id)
	require.NoError(t, err)
	require.Equal(t, store.StatusUploading, f.Status,
		"a tab that holds no bytes paused the transfer")

	status, body = other.action(t, http.MethodPost, "/resume/"+id+"/", "", "")
	require.Equal(t, http.StatusOK, status)
	require.Contains(t, body, "this tab does not hold the bytes of that file")

	// The tab that started it may, and taking the transfer over moves that
	// right to the tab that picked the file again.
	status, _ = tb.pause(t, id)
	require.Equal(t, http.StatusOK, status, "the owning tab could not pause")

	status, _ = other.action(t, http.MethodPost, "/reattach/", "application/json",
		`{"id":"`+id+`","name":"notes.txt","size":20}`)
	require.Equal(t, http.StatusOK, status, "taking the transfer over")
	status, _ = other.pause(t, id)
	require.Equal(t, http.StatusOK, status, "the tab that took over could not pause")

	status, body = tb.pause(t, id)
	require.Equal(t, http.StatusOK, status)
	require.Contains(t, body, "this tab does not hold the bytes of that file",
		"the tab that gave the transfer up can still pause it")
}

// TestSecondUploadLeavesTheFirst tests that picking more files while others
// are being sent changes nothing about those: staging, the dialog and the
// start of a new upload touch only the files they were given.
func TestSecondUploadLeavesTheFirst(t *testing.T) {
	tb := newTab(t)
	first := tb.start(t, "a.txt", "a.txt", "text/plain", len(contents))
	require.Equal(t, http.StatusOK, tb.chunk(t, first, 0, contents[:10]))

	before, err := tb.files.Get(first)
	require.NoError(t, err)

	second := tb.start(t, "b.txt", "b.txt", "text/plain", len(contents))
	require.NotEqual(t, first, second)

	after, err := tb.files.Get(first)
	require.NoError(t, err)
	require.Equal(t, store.StatusUploading, after.Status,
		"starting another upload paused the first")
	require.Equal(t, before.Owner, after.Owner,
		"starting another upload took the first away from its tab")
	require.Equal(t, http.StatusOK, tb.chunk(t, first, 10, contents[10:]),
		"the first upload stopped taking bytes")
}

// TestRefusedChunkKeepsStatus tests that a chunk the store cannot use leaves
// the file as it was. Only the visitor pauses a file.
func TestRefusedChunkKeepsStatus(t *testing.T) {
	tb := newTab(t)
	id := tb.start(t, "a.txt", "a.txt", "text/plain", len(contents))

	require.Equal(t, http.StatusConflict, tb.chunk(t, id, 5, contents[5:]),
		"a chunk that leaves a hole was accepted")
	f, err := tb.files.Get(id)
	require.NoError(t, err)
	require.Equal(t, store.StatusUploading, f.Status,
		"a refused chunk paused the file")
	require.NotEmpty(t, f.Owner, "a refused chunk released the file")
}

// TestEmptyFile tests that a file of no bytes is created complete and downloadable,
// since nothing will ever be appended to finish it.
func TestEmptyFile(t *testing.T) {
	tb := newTab(t)
	id := tb.start(t, "empty.txt", "empty.txt", "text/plain", 0)

	f, err := tb.files.Get(id)
	require.NoError(t, err, "reading the created file")
	require.Equal(t, store.StatusComplete, f.Status)

	resp, err := tb.srv.Client().Get(tb.srv.URL + app.DownloadPrefix + id)
	require.NoError(t, err, "downloading")
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	got, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "reading the download")
	require.Empty(t, got)
}

// TestStreamCloseGivesUp tests that a tab going away releases what it was sending,
// so that another tab can take the transfer over.
// The status stays as it was: only the visitor pauses a file, and the visitor did not.
func TestStreamCloseGivesUp(t *testing.T) {
	tb := newTab(t)
	id := tb.start(t, "notes.txt", "notes.txt", "text/plain", len(contents))
	require.Equal(t, http.StatusOK, tb.chunk(t, id, 0, contents[:10]))

	tb.close()
	require.Eventually(t, func() bool {
		f, err := tb.files.Get(id)
		return err == nil && f.Owner == ""
	}, 3*time.Second, 10*time.Millisecond,
		"the tab went away and still held the transfer")

	f, err := tb.files.Get(id)
	require.NoError(t, err)
	require.Equal(t, store.StatusUploading, f.Status,
		"a tab going away paused a file the visitor did not pause")
}

// TestOrphanedFileOffersTakeOver tests that a page load offers the take-over
// control for a file nobody is sending. A server restart leaves exactly that
// behind: the status is read back from disk and the owner is not.
func TestOrphanedFileOffersTakeOver(t *testing.T) {
	tb := newTab(t)
	id := tb.start(t, "notes.txt", "notes.txt", "text/plain", len(contents))
	require.Equal(t, http.StatusOK, tb.chunk(t, id, 0, contents[:10]))

	tb.close()
	require.Eventually(t, func() bool {
		f, err := tb.files.Get(id)
		return err == nil && f.Owner == ""
	}, 3*time.Second, 10*time.Millisecond, "the transfer was not given up")

	resp, err := tb.srv.Client().Get(tb.srv.URL + "/")
	require.NoError(t, err, "GET /")
	defer func() { _ = resp.Body.Close() }()
	page, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "reading the page")

	require.Contains(t, string(page), "Take over",
		"a file nobody is sending is not offered to be taken over")
	require.NotContains(t, string(page), `class="label">Pause</span>`,
		"a page load offers to pause a file it is not sending")
	require.NotContains(t, string(page), `class="label">Resume</span>`,
		"a page load offers to resume a file it cannot send")
	require.Contains(t, string(page), "status-interrupted")
}

// stage opens the upload dialog on one file and returns what it rendered.
func (tb *tab) stage(t *testing.T, name, contentType string, size int) string {
	t.Helper()
	_, body := tb.action(t, http.MethodPost, "/stage/", "application/json",
		`{"files":[{"name":"`+name+`","size":`+strconv.Itoa(size)+
			`,"type":"`+contentType+`"}]}`)
	return body
}

// TestPickingAnUnfinishedFileAgain tests that a file already half uploaded is
// offered to be continued rather than started again, and that continuing it
// keeps its bytes instead of creating a second file.
func TestPickingAnUnfinishedFileAgain(t *testing.T) {
	tb := newTab(t)
	id := tb.start(t, "notes.txt", "notes.txt", "text/plain", len(contents))
	require.Equal(t, http.StatusOK, tb.chunk(t, id, 0, contents[:10]))

	dialog := tb.stage(t, "notes.txt", "text/plain", len(contents))
	require.Contains(t, dialog, "Continue the unfinished upload (50%)")
	require.Contains(t, dialog, `name="dup-0"`)

	// The dialog defaults to continuing, which is what an unchanged form sends.
	from := tb.streamAt()
	status, _ := tb.action(t, http.MethodPost, "/start/",
		"application/x-www-form-urlencoded", "name-0=notes.txt&dup-0=continue")
	require.Equal(t, http.StatusOK, status, "starting")

	require.Len(t, tb.files.List(), 1, "continuing created a second file")
	again := idOfStartScript(t, tb.waitForStreamFrom(t, from, "dpUpload.start(["))
	require.Equal(t, id, again, "the job does not name the file it continues")
	require.Contains(t, tb.waitForStreamFrom(t, from, `"offset":10`), `"offset":10`,
		"the job does not continue at the received bytes")

	f, err := tb.files.Get(id)
	require.NoError(t, err)
	require.Equal(t, int64(10), f.Received, "continuing threw the bytes away")
	require.Equal(t, store.StatusUploading, f.Status)
}

// TestPickingAnUnfinishedFileAgainSeparately tests the other choice of that
// dialog: a second file under the name the visitor gave it.
func TestPickingAnUnfinishedFileAgainSeparately(t *testing.T) {
	tb := newTab(t)
	id := tb.start(t, "notes.txt", "notes.txt", "text/plain", len(contents))
	require.Equal(t, http.StatusOK, tb.chunk(t, id, 0, contents[:10]))

	tb.stage(t, "notes.txt", "text/plain", len(contents))
	from := tb.streamAt()
	status, _ := tb.action(t, http.MethodPost, "/start/",
		"application/x-www-form-urlencoded", "name-0=copy.txt&dup-0=separate")
	require.Equal(t, http.StatusOK, status, "starting")

	second := idOfStartScript(t, tb.waitForStreamFrom(t, from, "dpUpload.start(["))
	require.NotEqual(t, id, second, "the second pick continued the first upload")
	require.Len(t, tb.files.List(), 2)

	f, err := tb.files.Get(second)
	require.NoError(t, err)
	require.Equal(t, "copy.txt", f.Name)
	require.Zero(t, f.Received)
}

// TestSeparateUploadKeepsNamesApart tests that confirming the dialog unchanged
// for a second copy does not produce two rows a visitor cannot tell apart.
func TestSeparateUploadKeepsNamesApart(t *testing.T) {
	tb := newTab(t)
	id := tb.start(t, "notes.txt", "notes.txt", "text/plain", len(contents))
	require.Equal(t, http.StatusOK, tb.chunk(t, id, 0, contents[:10]))

	tb.stage(t, "notes.txt", "text/plain", len(contents))
	status, _ := tb.action(t, http.MethodPost, "/start/",
		"application/x-www-form-urlencoded", "name-0=notes.txt&dup-0=separate")
	require.Equal(t, http.StatusOK, status, "starting")

	names := make([]string, 0, 2)
	for _, f := range tb.files.List() {
		names = append(names, f.Name)
	}
	require.ElementsMatch(t, []string{"notes.txt", "notes (2).txt"}, names)
}

// TestContinuingKeepsTheName tests that confirming the dialog unchanged for an
// upload that is continued renames nothing, and that editing the name does.
func TestContinuingKeepsTheName(t *testing.T) {
	tb := newTab(t)
	id := tb.start(t, "notes.txt", "holiday.txt", "text/plain", len(contents))
	require.Equal(t, http.StatusOK, tb.chunk(t, id, 0, contents[:10]))

	// The dialog offers the name the file carries, not the one the browser reports,
	// which is what an unchanged confirmation keeps.
	dialog := tb.stage(t, "notes.txt", "text/plain", len(contents))
	require.Contains(t, dialog, `value="holiday.txt"`)

	status, _ := tb.action(t, http.MethodPost, "/start/",
		"application/x-www-form-urlencoded", "name-0=holiday.txt&dup-0=continue")
	require.Equal(t, http.StatusOK, status, "starting")
	f, err := tb.files.Get(id)
	require.NoError(t, err)
	require.Equal(t, "holiday.txt", f.Name)

	tb.stage(t, "notes.txt", "text/plain", len(contents))
	status, _ = tb.action(t, http.MethodPost, "/start/",
		"application/x-www-form-urlencoded", "name-0=trip.txt&dup-0=continue")
	require.Equal(t, http.StatusOK, status, "starting")
	f, err = tb.files.Get(id)
	require.NoError(t, err)
	require.Equal(t, "trip.txt", f.Name, "the name the dialog carried was ignored")
	require.Len(t, tb.files.List(), 1)
}

// TestPickingACompleteFileAgain tests that a file whose bytes are all there is
// not offered to be continued: there is nothing left of it to send.
func TestPickingACompleteFileAgain(t *testing.T) {
	tb := newTab(t)
	id := tb.start(t, "notes.txt", "notes.txt", "text/plain", len(contents))
	require.Equal(t, http.StatusOK, tb.chunk(t, id, 0, contents))

	dialog := tb.stage(t, "notes.txt", "text/plain", len(contents))
	require.NotContains(t, dialog, "Continue the unfinished upload")
	require.NotContains(t, dialog, `name="dup-0"`)
}

// TestContinuingAFileThatWentAway tests that a dialog left open while the
// upload it would continue is deleted still starts the file.
func TestContinuingAFileThatWentAway(t *testing.T) {
	tb := newTab(t)
	id := tb.start(t, "notes.txt", "notes.txt", "text/plain", len(contents))
	require.Equal(t, http.StatusOK, tb.chunk(t, id, 0, contents[:10]))
	tb.stage(t, "notes.txt", "text/plain", len(contents))

	status, _ := tb.action(t, http.MethodDelete, "/files/"+id+"/", "", "")
	require.Equal(t, http.StatusOK, status, "deleting")

	from := tb.streamAt()
	status, _ = tb.action(t, http.MethodPost, "/start/",
		"application/x-www-form-urlencoded", "name-0=notes.txt&dup-0=continue")
	require.Equal(t, http.StatusOK, status, "starting")

	fresh := idOfStartScript(t, tb.waitForStreamFrom(t, from, "dpUpload.start(["))
	require.NotEqual(t, id, fresh)
	require.Len(t, tb.files.List(), 1)
}

// TestSlowChunkOutlivesTheReadTimeout tests that a chunk slower than the read
// timeout of the server still arrives whole. A server times a whole request,
// which an upload held back by a limit outlives: the deadline has to follow
// the bytes instead.
func TestSlowChunkOutlivesTheReadTimeout(t *testing.T) {
	const idle = 400 * time.Millisecond
	tb := newTabWith(t, idle, 200*time.Millisecond)
	id := tb.start(t, "notes.txt", "notes.txt", "text/plain", len(contents))

	// Five pieces, each within the idle deadline, together well past both it
	// and the read timeout of the server.
	pr, pw := io.Pipe()
	go func() {
		for _, b := range []byte(contents) {
			time.Sleep(150 * time.Millisecond)
			if _, err := pw.Write([]byte{b}); err != nil {
				break
			}
		}
		_ = pw.Close()
	}()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPut,
		tb.srv.URL+"/upload/"+id+"/?offset=0", pr)
	require.NoError(t, err, "building the chunk request")
	resp, err := tb.srv.Client().Do(req)
	require.NoError(t, err, "sending a slow chunk")
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode, "a slow chunk was cut off")

	f, err := tb.files.Get(id)
	require.NoError(t, err)
	require.Equal(t, store.StatusComplete, f.Status,
		"the slow chunk did not arrive whole")
}

// TestLimits tests that the numbers the inputs send reach both transfer
// directions and come back to every tab as the values that took effect.
func TestLimits(t *testing.T) {
	payload := strings.Repeat("x", 32<<10)
	tb := newTab(t)

	status, _ := tb.action(t, http.MethodPost, "/limits/", "application/json",
		`{"limitUp":32,"limitDown":32}`)
	require.Equal(t, http.StatusOK, status, "setting the limits")
	tb.waitForStream(t, `"limitUp":32`)
	tb.waitForStream(t, `"limitDown":32`)

	id := tb.start(t, "big.bin", "big.bin", "application/octet-stream", len(payload))

	// 32 KiB at 32 KiB/s is a second of transfer, less the burst the limiter
	// starts with. The bound is loose: a busy machine may take much longer,
	// and only being faster than the limit would be a failure.
	start := time.Now()
	require.Equal(t, http.StatusOK, tb.chunk(t, id, 0, payload))
	require.Greater(t, time.Since(start), 300*time.Millisecond,
		"the upload limit did not slow the chunk down")

	start = time.Now()
	resp, err := tb.srv.Client().Get(tb.srv.URL + app.DownloadPrefix + id)
	require.NoError(t, err, "downloading")
	defer func() { _ = resp.Body.Close() }()
	got, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "reading the download")
	require.Greater(t, time.Since(start), 300*time.Millisecond,
		"the download limit did not slow the response down")
	require.Equal(t, payload, string(got), "the limit changed the content")
}

// TestLimitsAreClamped tests that a number the inputs cannot produce is
// brought into range and reported back rather than refused.
func TestLimitsAreClamped(t *testing.T) {
	tb := newTab(t)
	status, _ := tb.action(t, http.MethodPost, "/limits/", "application/json",
		`{"limitUp":-5,"limitDown":999999999}`)
	require.Equal(t, http.StatusOK, status, "setting the limits")
	tb.waitForStream(t, `"limitUp":`+strconv.Itoa(app.MinLimitKiB))
	tb.waitForStream(t, `"limitDown":`+strconv.Itoa(app.MaxLimitKiB))
}

// TestRateBelowTheMinimumResets tests that every way of reaching zero answers
// with the minimum rather than an error. An emptied number field reports an
// empty string, and refusing it would leave the field holding
// a rate the server never took.
func TestRateBelowTheMinimumResets(t *testing.T) {
	for name, rate := range map[string]string{
		"zero":            "0",
		"quoted zero":     `"0"`,
		"empty":           `""`,
		"blank":           `"  "`,
		"null":            "null",
		"negative":        "-3",
		"quoted negative": `"-3"`,
	} {
		t.Run(name, func(t *testing.T) {
			tb := newTab(t)
			from := tb.streamAt()
			status, _ := tb.action(t, http.MethodPost, "/limits/", "application/json",
				`{"limitUp":`+rate+`,"unlimitedUp":false,"limitDown":1,"unlimitedDown":true}`)
			require.Equal(t, http.StatusOK, status, "setting the limit to %s", rate)
			require.Contains(t,
				tb.waitForStreamFrom(t, from, "limitUp"),
				`"limitUp":`+strconv.Itoa(app.MinLimitKiB),
				"the field was left holding a rate the server never took")
			require.EqualValues(t, app.MinLimitKiB, tb.app.Limits().Upload.KiB)
		})
	}
}

// TestRateArrivesQuoted tests that a rate the browser sends as a string is
// read as the number it holds. Datastar binds a web component by its value
// property, which makes the field report "128" where a native input sent 128.
func TestRateArrivesQuoted(t *testing.T) {
	tb := newTab(t)
	from := tb.streamAt()
	status, _ := tb.action(t, http.MethodPost, "/limits/", "application/json",
		`{"limitUp":"128","unlimitedUp":false,"limitDown":"64","unlimitedDown":false}`)
	require.Equal(t, http.StatusOK, status, "setting the limits")
	require.Contains(t, tb.waitForStreamFrom(t, from, "limitUp"), `"limitUp":128`)
	require.EqualValues(t, 128, tb.app.Limits().Upload.KiB)
	require.EqualValues(t, 64*1024, tb.app.Limits().Download.Rate())
}

// TestLimitStepExprSnapsToTheGrid pins the rates the arrow keys land on.
// Stepping snaps to multiples of the step, so that going down to the floor and
// back up returns a round rate where adding the step would return 65.
//
// The browser evaluates limitStepExpr; [stepped] is its twin in Go, and what
// this asserts is the rate the server ends up holding. It pins the intended
// grid, not the JavaScript.
func TestLimitStepExprSnapsToTheGrid(t *testing.T) {
	for name, tc := range map[string]struct {
		from string
		up   bool
		want string
	}{
		"up from the floor":       {from: "1", up: true, want: "64"},
		"up on the grid":          {from: "64", up: true, want: "128"},
		"up from between":         {from: "100", up: true, want: "128"},
		"down onto the grid":      {from: "128", up: false, want: "64"},
		"down from between":       {from: "100", up: false, want: "64"},
		"down lands on the floor": {from: "64", up: false, want: "1"},
	} {
		t.Run(name, func(t *testing.T) {
			tb := newTab(t)
			limits := func(rate string) string {
				return `{"limitUp":` + rate +
					`,"unlimitedUp":false,"limitDown":1,"unlimitedDown":true}`
			}
			status, _ := tb.action(t, http.MethodPost, "/limits/",
				"application/json", limits(tc.from))
			require.Equal(t, http.StatusOK, status, "seeding the rate")

			from := tb.streamAt()
			status, _ = tb.action(t, http.MethodPost, "/limits/",
				"application/json", limits(stepped(tc.from, tc.up)))
			require.Equal(t, http.StatusOK, status, "stepping the rate")
			tb.waitForStreamFrom(t, from, `"limitUp":`+tc.want)
			require.EqualValues(t, tc.want,
				strconv.FormatInt(tb.app.Limits().Upload.KiB, 10))
		})
	}
}

// stepped mirrors limitStepExpr, the expression the arrow keys evaluate in the
// browser. The server never runs it, and what keeps the two honest is this
// test asserting the rate the server ends up holding.
func stepped(from string, up bool) string {
	v, err := strconv.Atoi(from)
	if err != nil {
		panic(err)
	}
	n := float64(v) / float64(app.LimitStepKiB)
	var next int
	if up {
		next = (int(math.Floor(n)) + 1) * app.LimitStepKiB
	} else {
		next = (int(math.Ceil(n)) - 1) * app.LimitStepKiB
	}
	return strconv.Itoa(min(app.MaxLimitKiB, max(app.MinLimitKiB, next)))
}

// TestUnlimitedKeepsTheRate tests that switching a direction to unlimited
// stops enforcing its rate and still reports the number the field goes back
// to when the switch goes off again.
func TestUnlimitedKeepsTheRate(t *testing.T) {
	tb := newTab(t)
	status, _ := tb.action(t, http.MethodPost, "/limits/", "application/json",
		`{"limitUp":256,"unlimitedUp":true,"limitDown":128,"unlimitedDown":false}`)
	require.Equal(t, http.StatusOK, status, "setting the limits")

	tb.waitForStream(t, `"limitUp":256`)
	tb.waitForStream(t, `"unlimitedUp":true`)

	limits := tb.app.Limits()
	require.True(t, limits.Upload.Unlimited, "the switch did not reach the server")
	require.Zero(t, limits.Upload.Rate(), "an unlimited direction is still paced")
	require.EqualValues(t, 256, limits.Upload.KiB, "the rate was forgotten")
	require.EqualValues(t, 128*1024, limits.Download.Rate(), "the limited direction is not paced")
}

// TestCompleteFileOffersItsLink tests that a finished file carries a copy
// control holding the absolute URL of the download, and that a file still
// receiving bytes carries none: that URL answers 404 until it completes.
func TestCompleteFileOffersItsLink(t *testing.T) {
	tb := newTab(t)
	id := tb.start(t, "notes.txt", "notes.txt", "text/plain", len(contents))

	from := tb.streamAt()
	require.Equal(t, http.StatusOK, tb.chunk(t, id, 0, contents[:10]))
	require.NotContains(t,
		tb.waitForStreamFrom(t, from, "file-list"),
		"neo-clipcopy", "an incomplete file offers a link that 404s")

	from = tb.streamAt()
	require.Equal(t, http.StatusOK, tb.chunk(t, id, 10, contents[10:]))
	require.Contains(t,
		tb.waitForStreamFrom(t, from, "neo-clipcopy"),
		`value="`+tb.srv.URL+app.DownloadPrefix+id+`"`,
		"the copy control does not carry the absolute download URL")
}

// TestCompleteFileShowsNoStatusIcon tests that a file that is all there
// carries no badge: it is the ordinary case, and the row already offers a
// download rather than a transfer.
func TestCompleteFileShowsNoStatusIcon(t *testing.T) {
	tb := newTab(t)
	id := tb.start(t, "notes.txt", "notes.txt", "text/plain", len(contents))

	from := tb.streamAt()
	require.Equal(t, http.StatusOK, tb.chunk(t, id, 0, contents[:10]))
	require.Contains(t, tb.waitForStreamFrom(t, from, "file-list"), "status-uploading",
		"a transfer in flight lost its badge")

	from = tb.streamAt()
	require.Equal(t, http.StatusOK, tb.chunk(t, id, 10, contents[10:]))
	require.NotContains(t,
		lastFileList(t, tb.waitForStreamFrom(t, from, "neo-clipcopy")),
		`class="status`, "a finished file still carries a badge")
}

// lastFileList is the most recent rendering of the list in what the stream
// delivered. The buffer holds every patch since it was marked, and the earlier
// ones describe the file at an earlier point of its transfer.
func lastFileList(t *testing.T, stream string) string {
	t.Helper()
	i := strings.LastIndex(stream, `<ul id="file-list"`)
	require.GreaterOrEqual(t, i, 0, "the stream delivered no file list")
	return stream[i:]
}

// TestRowCarriesAnActionsMenu tests that every row also renders its controls
// as one menu, which is what a phone shows in place of the row of buttons.
func TestRowCarriesAnActionsMenu(t *testing.T) {
	tb := newTab(t)
	id := tb.start(t, "notes.txt", "notes.txt", "text/plain", len(contents))

	from := tb.streamAt()
	require.Equal(t, http.StatusOK, tb.chunk(t, id, 0, contents))
	body := lastFileList(t, tb.waitForStreamFrom(t, from, "Copy link"))
	require.Contains(t, body, `class="actions-menu"`, "no menu button in the row")
	require.Contains(t, body, "<neo-menu", "the menu button opens nothing")
	require.Contains(t, body, "<neo-menuitem data-neo-dialog-trigger>",
		"the menu cannot reach the delete confirmation")
	require.Contains(t, body, "Copy link", "the menu is missing the copy entry")
}

// TestUploadDialogStartsOnClick tests that the Start control carries the action itself.
// neo-button is not form-associated: type="submit" on it submits nothing
// and the dialog would sit there doing nothing when pressed.
func TestUploadDialogStartsOnClick(t *testing.T) {
	tb := newTab(t)
	status, body := tb.action(t, http.MethodPost, "/stage/", "application/json",
		`{"files":[{"name":"notes.txt","size":20,"type":"text/plain"}]}`)
	require.Equal(t, http.StatusOK, status, "staging")
	require.Contains(t, body, "Start upload", "no start control in the dialog")
	require.Contains(t, body, "/start/", "the start control carries no action")
	require.NotContains(t, body, `type="submit"`,
		"a custom element cannot submit a form")
}

// TestDownloadIsALink tests that both download controls can actually navigate.
// neo-button reads no href and neo-menuitem cancels the default of every click it sees:
// neither a button carrying an href nor an anchor nested in a menu row goes anywhere.
func TestDownloadIsALink(t *testing.T) {
	tb := newTab(t)
	id := tb.start(t, "notes.txt", "notes.txt", "text/plain", len(contents))

	from := tb.streamAt()
	require.Equal(t, http.StatusOK, tb.chunk(t, id, 0, contents))
	body := lastFileList(t, tb.waitForStreamFrom(t, from, "Copy link"))

	require.Contains(t, body,
		`<a data-neo-link href="`+app.DownloadPrefix+id+`" variant="button-primary">`,
		"the row's download is not a link")
	require.NotContains(t, body, `<neo-button role="button" href=`,
		"an href on a neo-button navigates nowhere")
	require.Contains(t, body, `<neo-menuitem data-href="`+app.DownloadPrefix+id+`"`,
		"the menu's download row carries no address")
}

// TestProgressCarriesTheBytes tests that the transferred amount rides on the
// progress bar as its label, which the component lays out against the
// percentage, rather than in a paragraph of its own below it.
func TestProgressCarriesTheBytes(t *testing.T) {
	tb := newTab(t)
	id := tb.start(t, "notes.txt", "notes.txt", "text/plain", len(contents))

	from := tb.streamAt()
	require.Equal(t, http.StatusOK, tb.chunk(t, id, 0, contents[:10]))
	body := lastFileList(t, tb.waitForStreamFrom(t, from, "neo-progress"))
	require.Contains(t, body, `label="10 B of 20 B"`, "the bar carries no amount")
	require.Contains(t, body, `value="50"`, "the bar carries no percentage")
	require.NotContains(t, body, "progress-text", "the amount is still a paragraph")
}

// TestBadgeFollowsTheOwner tests that the badge agrees with the control beside
// it. A file paused in a tab that has since gone is one nobody can resume, so
// the row offers to take it over and the badge has to say interrupted rather
// than paused.
func TestBadgeFollowsTheOwner(t *testing.T) {
	tb := newTab(t)
	id := tb.start(t, "notes.txt", "notes.txt", "text/plain", len(contents))
	require.Equal(t, http.StatusOK, tb.chunk(t, id, 0, contents[:10]))

	from := tb.streamAt()
	status, _ := tb.pause(t, id)
	require.Equal(t, http.StatusOK, status, "pausing")
	paused := lastFileList(t, tb.waitForStreamFrom(t, from, "status-paused"))
	require.Contains(t, paused, "You paused this upload",
		"the badge of a held file does not say who paused it")

	// The tab that held the bytes goes away, as a reload does.
	other := tb.openTab(t)
	from = other.streamAt()
	tb.close()
	orphaned := lastFileList(t, other.waitForStreamFrom(t, from, "status-interrupted"))
	require.NotContains(t, orphaned, "status-paused",
		"a file nobody holds still badges as paused")
	require.Contains(t, orphaned, "No tab is sending this file",
		"the badge does not say what happened to the file")
	require.Contains(t, orphaned, "Take over",
		"the badge and the control disagree")
}

// TestDeleteAsksFirst tests that every row carries a confirmation naming its
// file, that the browser owns opening it, and that the file survives until the
// delete itself is sent.
func TestDeleteAsksFirst(t *testing.T) {
	tb := newTab(t)
	id := tb.start(t, "notes.txt", "holiday photos.txt", "text/plain", len(contents))

	from := tb.streamAt()
	require.Equal(t, http.StatusOK, tb.chunk(t, id, 0, contents[:10]))
	body := tb.waitForStreamFrom(t, from, "neo-dialog")
	require.Contains(t, body, "Delete this file?", "the row carries no confirmation")
	require.Contains(t, body, "holiday photos.txt", "the dialog does not name the file")
	require.Contains(t, body, "data-neo-dialog-trigger",
		"nothing opens the dialog: a Morpheus dialog stays inert without a trigger")
	require.Contains(t, body, "data-neo-dialog-close", "nothing cancels the dialog")
	require.Len(t, tb.files.List(), 1, "rendering the dialog deleted the file")

	status, _ := tb.action(t, http.MethodDelete, "/files/"+id+"/", "", "")
	require.Equal(t, http.StatusOK, status, "deleting")
	require.Empty(t, tb.files.List(), "the confirmed delete kept the file")
}

// TestDeleteDialogNamesTheStoredFile tests that the dialog shows the name the
// file is stored under rather than the one the browser reported.
func TestDeleteDialogNamesTheStoredFile(t *testing.T) {
	tb := newTab(t)
	id := tb.start(t, "original.txt", "renamed.txt", "text/plain", len(contents))

	from := tb.streamAt()
	require.Equal(t, http.StatusOK, tb.chunk(t, id, 0, contents[:10]))
	require.Contains(t,
		tb.waitForStreamFrom(t, from, "neo-dialog"),
		`<strong class="delete-name">renamed.txt</strong>`,
		"the dialog names what the browser reported, not what the store holds")
}

// TestChunkSizeFollowsTheLimit tests that the job the uploader receives carries
// a chunk sized from the upload limit.
func TestChunkSizeFollowsTheLimit(t *testing.T) {
	for name, tc := range map[string]struct {
		limitKiB  int64
		unlimited bool
		want      int64
	}{
		"unlimited sends the largest chunk":  {unlimited: true, want: 1 << 20},
		"a high limit is capped":             {limitKiB: 1 << 20, want: 1 << 20},
		"a slow limit sends a small chunk":   {limitKiB: 8, want: 8 * 1024 * 10},
		"the slowest limit stays above zero": {limitKiB: 1, want: 16 << 10},
	} {
		t.Run(name, func(t *testing.T) {
			tb := newTab(t)
			status, _ := tb.action(t, http.MethodPost, "/limits/", "application/json",
				`{"limitUp":`+strconv.FormatInt(tc.limitKiB, 10)+
					`,"unlimitedUp":`+strconv.FormatBool(tc.unlimited)+
					`,"limitDown":1,"unlimitedDown":true}`)
			require.Equal(t, http.StatusOK, status, "setting the limits")

			from := tb.streamAt()
			tb.start(t, "f.bin", "f.bin", "text/plain", 4096)
			require.Contains(t,
				tb.waitForStreamFrom(t, from, "dpUpload.start(["),
				`"chunkSize":`+strconv.FormatInt(tc.want, 10),
				"the job does not carry the chunk size of a %d KiB/s limit", tc.limitKiB)
		})
	}
}

// TestLimitChangeResizesTransfers tests that changing the limit sends every tab
// the chunk size that took effect, which is what reaches a running transfer.
func TestLimitChangeResizesTransfers(t *testing.T) {
	tb := newTab(t)
	from := tb.streamAt()
	status, _ := tb.action(t, http.MethodPost, "/limits/", "application/json",
		`{"limitUp":4,"limitDown":0}`)
	require.Equal(t, http.StatusOK, status, "setting the limits")
	require.Contains(t,
		tb.waitForStreamFrom(t, from, "dpUpload.resize("),
		"dpUpload.resize("+strconv.Itoa(4*1024*10)+")",
		"the limit did not resize the running transfers")
}

// idOfStartScript reads the file identifier out of the dpUpload.start call the
// start action sends back.
func idOfStartScript(t *testing.T, body string) string {
	t.Helper()
	const open = `dpUpload.start([{"id":"`
	_, after, ok := strings.Cut(body, open)
	require.True(t, ok, "no start call in %q", body)
	id, _, ok := strings.Cut(after, `"`)
	require.True(t, ok, "unterminated identifier in %q", body)
	return id
}

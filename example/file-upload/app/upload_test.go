// Tests the upload protocol over HTTP: staging, the chunks, pause and resume,
// picking a file again for a transfer this browser lost, and the download.

package app_test

import (
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/net/html"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/datapagestest"
	"github.com/romshark/datapages/example/file-upload/app"
	"github.com/romshark/datapages/example/file-upload/app/datapagesgen"
	"github.com/romshark/datapages/example/file-upload/app/datapagesgen/href"
	"github.com/romshark/datapages/example/file-upload/store"
	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/messaging/inmem"
)

// contents is what the test uploads, in chunks the test chooses.
const contents = "0123456789abcdefghij"

// tab holds what one browser tab has: the page load that minted its instance
// identifier and the stream that keeps its per-tab state alive. The other fields
// are shared by every tab of the server.
type tab struct {
	*datapagestest.Tab
	stream *datapagestest.Stream

	srv     *datapagestest.Server
	visitor *datapagestest.Visitor // the browser the tabs belong to
	files   *store.Store
	app     *app.App
}

func newTab(t *testing.T) *tab {
	t.Helper()
	return newTabWith(t, app.ChunkIdleTimeout, 0)
}

// newTabWith builds a server whose chunk requests end after idle of silence
// and whose requests are read within readTimeout. A zero readTimeout sets none.
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
		datapages.WithMiddleware(app.ChunkDeadline(idle)))
	require.NoError(t, err, "building server")

	// The read timeout belongs to the net/http server that serves the handler,
	// not to the handler datapages.NewServer returns.
	srv := datapagestest.New(t, s, datapagestest.WithHTTPServer(func(hs *http.Server) {
		hs.ReadTimeout = readTimeout
	}))
	tb := &tab{srv: srv, visitor: srv.Visitor(t), files: files, app: a}
	tb.load(t)
	return tb
}

// openTab loads the page in another tab of the same browser,
// as a second window would.
func (tb *tab) openTab(t *testing.T) *tab {
	t.Helper()
	other := &tab{srv: tb.srv, visitor: tb.visitor, files: tb.files, app: tb.app}
	other.load(t)
	return other
}

// load opens the page and connects the stream that allocates its per-tab state.
// Closing the stream is the tab going away.
func (tb *tab) load(t *testing.T) {
	t.Helper()
	tb.Tab = tb.visitor.Load(t, href.PageIndex())
	require.NotEmpty(t, tb.Instance(), "GET / minted no instance id")
	tb.stream = tb.Connect(t, tb.Page().Signals(t))
}

// chunk sends one chunk the way the uploader does, as a plain fetch. Without the
// Datastar-Request header a refusal arrives as a status code rather than as a
// message RecoverError renders.
func (tb *tab) chunk(t *testing.T, id string, offset int64, data string) int {
	t.Helper()
	req := tb.visitor.NewRequest(t, http.MethodPut,
		"/upload/"+id+"/?offset="+strconv.FormatInt(offset, 10), strings.NewReader(data))
	req.Header.Set("Content-Type", "application/octet-stream")
	return tb.visitor.Send(t, req).Status
}

// pause sends the action the Pause button sends.
func (tb *tab) pause(t *testing.T, id string) *datapagestest.Response {
	t.Helper()
	return tb.Do(t, http.MethodPost, "/pause/"+id+"/", nil)
}

// picked is what the page sends the stage action for one file the visitor picked,
// in the shape dpUpload.files gives it.
func picked(name, contentType string, size int) map[string]any {
	return map[string]any{"files": []map[string]any{
		{"name": name, "size": size, "type": contentType},
	}}
}

// start opens the upload dialog on one file, starts it under name and returns the
// job the uploader of the tab receives for it.
func (tb *tab) start(t *testing.T, original, name, contentType string, size int) job {
	t.Helper()
	resp := tb.Do(t, http.MethodPost, "/stage/", picked(original, contentType, size))
	require.Equal(t, http.StatusOK, resp.Status, "staging %s", original)

	resp = tb.DoForm(t, http.MethodPost, "/start/", url.Values{"name-0": {name}})
	require.Equal(t, http.StatusOK, resp.Status, "starting %s", original)
	return tb.waitForStart(t)
}

// job is an upload job by the names browser.js reads it with.
type job struct {
	ID        string `json:"id"`
	Offset    int64  `json:"offset"`
	ChunkSize int64  `json:"chunkSize"`
}

// waitForStart waits for the job that starting one file sends the uploader of the
// tab. It travels on the tab's stream, not on the response of the start action.
func (tb *tab) waitForStart(t *testing.T) job {
	t.Helper()
	var jobs []job
	decodeCall(t, tb.stream.WaitFor(t, calls("dpUpload.start")), "dpUpload.start", &jobs)
	require.Len(t, jobs, 1, "starting one file")
	return jobs[0]
}

// resumed returns the job that the response of a resume or a reattachment tells the
// uploader to go on with.
func resumed(t *testing.T, resp *datapagestest.Response) job {
	t.Helper()
	evs := resp.Events(t)
	i := slices.IndexFunc(evs, calls("dpUpload.go").Matches)
	require.GreaterOrEqual(t, i, 0,
		"the response tells the uploader nothing:\n%s", resp.Body)
	var j job
	decodeCall(t, evs[i], "dpUpload.go", &j)
	return j
}

// TestUpload covers one file from the dialog to the download,
// interrupted by a pause and resumed, then deleted.
func TestUpload(t *testing.T) {
	tb := newTab(t)
	id := tb.start(t, "notes.txt", "renamed.txt", "text/plain", len(contents)).ID

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
	require.Equal(t, http.StatusOK, tb.pause(t, id).Status, "pausing")
	require.Equal(t, http.StatusForbidden, tb.chunk(t, id, 10, contents[10:]))

	resp := tb.Do(t, http.MethodPost, "/resume/"+id+"/", nil)
	require.Equal(t, http.StatusOK, resp.Status, "resuming")
	resume := resumed(t, resp)
	require.Equal(t, id, resume.ID, "resuming does not tell the uploader to continue")
	require.EqualValues(t, 10, resume.Offset,
		"resuming does not continue at the received bytes")

	require.Equal(t, http.StatusOK, tb.chunk(t, id, 10, contents[10:]))
	f, err = tb.files.Get(id)
	require.NoError(t, err)
	require.Equal(t, store.StatusComplete, f.Status, "the file did not complete")

	download := tb.visitor.Get(t, href.App.File(id))
	require.Equal(t, http.StatusOK, download.Status, "downloading")
	require.Equal(t, "text/plain", download.Header.Get("Content-Type"),
		"the download does not carry the type the browser reported")
	require.Equal(t, `attachment; filename=renamed.txt`,
		download.Header.Get("Content-Disposition"),
		"the download does not name the file")
	require.Equal(t, contents, download.Body, "the download differs from what was sent")

	require.Equal(t, http.StatusOK,
		tb.Do(t, http.MethodDelete, "/files/"+id+"/", nil).Status, "deleting")
	require.Equal(t, http.StatusNotFound, tb.visitor.Get(t, href.App.File(id)).Status,
		"a deleted file is still being served")
}

// TestDownloadIncomplete tests that a file still receiving bytes has nothing
// to download, so that no partial blob can be taken for the whole file.
func TestDownloadIncomplete(t *testing.T) {
	tb := newTab(t)
	id := tb.start(t, "notes.txt", "notes.txt", "text/plain", len(contents)).ID
	require.Equal(t, http.StatusOK, tb.chunk(t, id, 0, contents[:10]))

	require.Equal(t, http.StatusNotFound, tb.visitor.Get(t, href.App.File(id)).Status)
}

// TestChunkStatus covers what the uploader reacts to: a repeated chunk it may
// keep going after, a hole it must not, and a file that is gone.
func TestChunkStatus(t *testing.T) {
	tb := newTab(t)
	id := tb.start(t, "notes.txt", "notes.txt", "text/plain", len(contents)).ID

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

// reattachment is what the page sends the reattach action for a file the visitor
// picked again, in the shape dpUpload.reattachment gives it.
func reattachment(id, name string, size int) map[string]any {
	return map[string]any{"id": id, "name": name, "size": size}
}

// toastTitle returns the text of the title slot of the toast ev appends, "" when
// there is none. neo-toast shows its named slots only.
func toastTitle(ev datapagestest.Event) string {
	if n := find(ev.Root(), element("", "slot", "title")); n != nil {
		return datapagestest.Text(n)
	}
	return ""
}

// toast returns the message RecoverError shows for a refused action, and "" for a
// response without one, such as the empty answer of an action that succeeded
// without writing anything.
func toast(t *testing.T, resp *datapagestest.Response) string {
	t.Helper()
	if resp.Body == "" {
		return ""
	}
	for _, ev := range resp.Events(t) {
		if ev.Data["selector"] == "#toast" {
			return toastTitle(ev)
		}
	}
	return ""
}

// TestReattach covers a transfer whose browser lost the file: the visitor
// picks it again and the server checks it before the bytes continue.
func TestReattach(t *testing.T) {
	tb := newTab(t)
	id := tb.start(t, "notes.txt", "renamed.txt", "text/plain", len(contents)).ID
	require.Equal(t, http.StatusOK, tb.chunk(t, id, 0, contents[:10]))

	// A rejected reattachment answers with the message the visitor reads,
	// because RecoverError patches it into the page.
	for name, tc := range map[string]struct {
		payload map[string]any
		want    string
	}{
		"other name": {
			payload: reattachment(id, "other.txt", 20),
			want:    `"other.txt" of 20 bytes is not "notes.txt"`,
		},
		"other size": {
			payload: reattachment(id, "notes.txt", 21),
			want:    `of 21 bytes is not "notes.txt" of 20 bytes`,
		},
		"unknown file": {
			payload: reattachment("nosuchfile", "notes.txt", 20),
			want:    "file not found",
		},
		// The name the dialog chose is not what the browser reports for the
		// file the visitor picks again.
		"renamed file": {
			payload: reattachment(id, "renamed.txt", 20),
			want:    `"renamed.txt" of 20 bytes is not "notes.txt"`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			resp := tb.Do(t, http.MethodPost, "/reattach/", tc.payload)
			require.Equal(t, http.StatusOK, resp.Status)
			evs := resp.Events(t)
			i := slices.IndexFunc(evs, func(ev datapagestest.Event) bool {
				return strings.Contains(datapagestest.Text(ev.Root()), tc.want)
			})
			require.GreaterOrEqual(t, i, 0, "no event says %q:\n%s", tc.want, resp.Body)
			require.Equal(t, "#toast", evs[i].Data["selector"],
				"the message did not reach the toaster")
			require.Equal(t, "append", evs[i].Data["mode"],
				"the message replaces the stack instead of joining it")
			require.Contains(t, toastTitle(evs[i]), tc.want,
				"the message is in no slot and neo-toast projects none of it")
		})
	}

	resp := tb.Do(t, http.MethodPost, "/reattach/", reattachment(id, "notes.txt", 20))
	require.Equal(t, http.StatusOK, resp.Status, "reattaching the picked file")
	require.EqualValues(t, 10, resumed(t, resp).Offset,
		"reattaching does not continue at the received bytes")
	require.Equal(t, http.StatusOK, tb.chunk(t, id, 10, contents[10:]))
}

// TestOwnership tests that only the tab holding the bytes may pause or resume
// a transfer. Every other tab is told so and is offered the take-over control instead,
// which asks for the file rather than posting anything.
func TestOwnership(t *testing.T) {
	tb := newTab(t)
	id := tb.start(t, "notes.txt", "notes.txt", "text/plain", len(contents)).ID
	require.Equal(t, http.StatusOK, tb.chunk(t, id, 0, contents[:10]))

	other := tb.openTab(t)
	resp := other.pause(t, id)
	require.Equal(t, http.StatusOK, resp.Status)
	require.Contains(t, toast(t, resp), "this tab does not hold the bytes of that file")

	f, err := tb.files.Get(id)
	require.NoError(t, err)
	require.Equal(t, store.StatusUploading, f.Status,
		"a tab that holds no bytes paused the transfer")

	resp = other.Do(t, http.MethodPost, "/resume/"+id+"/", nil)
	require.Equal(t, http.StatusOK, resp.Status)
	require.Contains(t, toast(t, resp), "this tab does not hold the bytes of that file")

	// The tab that started it may, and taking the transfer over moves that
	// right to the tab that picked the file again.
	require.Equal(t, http.StatusOK, tb.pause(t, id).Status,
		"the owning tab could not pause")

	resp = other.Do(t, http.MethodPost, "/reattach/", reattachment(id, "notes.txt", 20))
	require.Equal(t, http.StatusOK, resp.Status, "taking the transfer over")
	require.Equal(t, http.StatusOK, other.pause(t, id).Status,
		"the tab that took over could not pause")

	resp = tb.pause(t, id)
	require.Equal(t, http.StatusOK, resp.Status)
	require.Contains(t, toast(t, resp), "this tab does not hold the bytes of that file",
		"the tab that gave the transfer up can still pause it")
}

// TestSecondUploadLeavesTheFirst tests that picking more files while others
// are being sent changes nothing about those: staging, the dialog and the
// start of a new upload touch only the files they were given.
func TestSecondUploadLeavesTheFirst(t *testing.T) {
	tb := newTab(t)
	first := tb.start(t, "a.txt", "a.txt", "text/plain", len(contents)).ID
	require.Equal(t, http.StatusOK, tb.chunk(t, first, 0, contents[:10]))

	before, err := tb.files.Get(first)
	require.NoError(t, err)

	second := tb.start(t, "b.txt", "b.txt", "text/plain", len(contents)).ID
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
	id := tb.start(t, "a.txt", "a.txt", "text/plain", len(contents)).ID

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
	id := tb.start(t, "empty.txt", "empty.txt", "text/plain", 0).ID

	f, err := tb.files.Get(id)
	require.NoError(t, err, "reading the created file")
	require.Equal(t, store.StatusComplete, f.Status)

	download := tb.visitor.Get(t, href.App.File(id))
	require.Equal(t, http.StatusOK, download.Status)
	require.Empty(t, download.Body)
}

// TestStreamCloseGivesUp tests that a tab going away releases what it was sending,
// so that another tab can take the transfer over.
// The status stays as it was: only the visitor pauses a file, and the visitor did not.
func TestStreamCloseGivesUp(t *testing.T) {
	tb := newTab(t)
	id := tb.start(t, "notes.txt", "notes.txt", "text/plain", len(contents)).ID
	require.Equal(t, http.StatusOK, tb.chunk(t, id, 0, contents[:10]))

	tb.stream.Close()
	tb.srv.WaitFor(t, "the tab that went away to release the transfer", func() bool {
		f, err := tb.files.Get(id)
		return err == nil && f.Owner == ""
	})

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
	id := tb.start(t, "notes.txt", "notes.txt", "text/plain", len(contents)).ID
	require.Equal(t, http.StatusOK, tb.chunk(t, id, 0, contents[:10]))

	tb.stream.Close()
	tb.srv.WaitFor(t, "the transfer to be given up", func() bool {
		f, err := tb.files.Get(id)
		return err == nil && f.Owner == ""
	})

	list := tb.visitor.Get(t, href.PageIndex()).ElementByID(t, "file-list")
	// The buttons of a row show their names in a label span.
	label := func(text string) func(*html.Node) bool {
		return func(n *html.Node) bool {
			return element("span", "class", "label")(n) && datapagestest.Text(n) == text
		}
	}
	require.Contains(t, datapagestest.Text(list), "Take over",
		"a file nobody is sending is not offered to be taken over")
	require.Nil(t, find(list, label("Pause")),
		"a page load offers to pause a file it is not sending")
	require.Nil(t, find(list, label("Resume")),
		"a page load offers to resume a file it cannot send")
	require.NotNil(t, find(list, class("status-interrupted")))
}

// stage opens the upload dialog on one file and returns the dialog it rendered.
func (tb *tab) stage(t *testing.T, name, contentType string, size int) *html.Node {
	t.Helper()
	resp := tb.Do(t, http.MethodPost, "/stage/", picked(name, contentType, size))
	return dialogOf(t, resp)
}

// dialogOf returns the upload dialog that resp patches into the page.
func dialogOf(t *testing.T, resp *datapagestest.Response) *html.Node {
	t.Helper()
	evs := resp.Events(t)
	i := slices.IndexFunc(evs, datapagestest.Element("upload-dialog").Matches)
	require.GreaterOrEqual(t, i, 0, "no upload dialog in the response:\n%s", resp.Body)
	return evs[i].ElementByID(t, "upload-dialog")
}

// TestPickingAnUnfinishedFileAgain tests that a file already half uploaded is
// offered to be continued rather than started again, and that continuing it
// keeps its bytes instead of creating a second file.
func TestPickingAnUnfinishedFileAgain(t *testing.T) {
	tb := newTab(t)
	id := tb.start(t, "notes.txt", "notes.txt", "text/plain", len(contents)).ID
	require.Equal(t, http.StatusOK, tb.chunk(t, id, 0, contents[:10]))

	dialog := tb.stage(t, "notes.txt", "text/plain", len(contents))
	require.Contains(t, datapagestest.Text(dialog),
		"Continue the unfinished upload (50%)")
	require.NotNil(t, find(dialog, element("input", "name", "dup-0")))

	// The dialog defaults to continuing, which is what an unchanged form sends.
	resp := tb.DoForm(t, http.MethodPost, "/start/",
		url.Values{"name-0": {"notes.txt"}, "dup-0": {"continue"}})
	require.Equal(t, http.StatusOK, resp.Status, "starting")

	require.Len(t, tb.files.List(), 1, "continuing created a second file")
	again := tb.waitForStart(t)
	require.Equal(t, id, again.ID, "the job does not name the file it continues")
	require.EqualValues(t, 10, again.Offset,
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
	id := tb.start(t, "notes.txt", "notes.txt", "text/plain", len(contents)).ID
	require.Equal(t, http.StatusOK, tb.chunk(t, id, 0, contents[:10]))

	tb.stage(t, "notes.txt", "text/plain", len(contents))
	resp := tb.DoForm(t, http.MethodPost, "/start/",
		url.Values{"name-0": {"copy.txt"}, "dup-0": {"separate"}})
	require.Equal(t, http.StatusOK, resp.Status, "starting")

	second := tb.waitForStart(t).ID
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
	id := tb.start(t, "notes.txt", "notes.txt", "text/plain", len(contents)).ID
	require.Equal(t, http.StatusOK, tb.chunk(t, id, 0, contents[:10]))

	tb.stage(t, "notes.txt", "text/plain", len(contents))
	resp := tb.DoForm(t, http.MethodPost, "/start/",
		url.Values{"name-0": {"notes.txt"}, "dup-0": {"separate"}})
	require.Equal(t, http.StatusOK, resp.Status, "starting")

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
	id := tb.start(t, "notes.txt", "holiday.txt", "text/plain", len(contents)).ID
	require.Equal(t, http.StatusOK, tb.chunk(t, id, 0, contents[:10]))

	// The dialog offers the name the file carries, not the one the browser reports,
	// which is what an unchanged confirmation keeps.
	field := find(tb.stage(t, "notes.txt", "text/plain", len(contents)),
		element("", "name", "name-0"))
	require.NotNil(t, field, "the dialog has no name field")
	value, _ := datapagestest.Attr(field, "value")
	require.Equal(t, "holiday.txt", value)

	resp := tb.DoForm(t, http.MethodPost, "/start/",
		url.Values{"name-0": {"holiday.txt"}, "dup-0": {"continue"}})
	require.Equal(t, http.StatusOK, resp.Status, "starting")
	f, err := tb.files.Get(id)
	require.NoError(t, err)
	require.Equal(t, "holiday.txt", f.Name)

	tb.stage(t, "notes.txt", "text/plain", len(contents))
	resp = tb.DoForm(t, http.MethodPost, "/start/",
		url.Values{"name-0": {"trip.txt"}, "dup-0": {"continue"}})
	require.Equal(t, http.StatusOK, resp.Status, "starting")
	f, err = tb.files.Get(id)
	require.NoError(t, err)
	require.Equal(t, "trip.txt", f.Name, "the name the dialog carried was ignored")
	require.Len(t, tb.files.List(), 1)
}

// TestPickingACompleteFileAgain tests that a file whose bytes are all there is
// not offered to be continued: there is nothing left of it to send.
func TestPickingACompleteFileAgain(t *testing.T) {
	tb := newTab(t)
	id := tb.start(t, "notes.txt", "notes.txt", "text/plain", len(contents)).ID
	require.Equal(t, http.StatusOK, tb.chunk(t, id, 0, contents))

	dialog := tb.stage(t, "notes.txt", "text/plain", len(contents))
	require.NotContains(t, datapagestest.Text(dialog), "Continue the unfinished upload")
	require.Nil(t, find(dialog, element("input", "name", "dup-0")))
}

// TestContinuingAFileThatWentAway tests that a dialog left open while the
// upload it would continue is deleted still starts the file.
func TestContinuingAFileThatWentAway(t *testing.T) {
	tb := newTab(t)
	id := tb.start(t, "notes.txt", "notes.txt", "text/plain", len(contents)).ID
	require.Equal(t, http.StatusOK, tb.chunk(t, id, 0, contents[:10]))
	tb.stage(t, "notes.txt", "text/plain", len(contents))

	require.Equal(t, http.StatusOK,
		tb.Do(t, http.MethodDelete, "/files/"+id+"/", nil).Status, "deleting")

	resp := tb.DoForm(t, http.MethodPost, "/start/",
		url.Values{"name-0": {"notes.txt"}, "dup-0": {"continue"}})
	require.Equal(t, http.StatusOK, resp.Status, "starting")

	fresh := tb.waitForStart(t).ID
	require.NotEqual(t, id, fresh)
	require.Len(t, tb.files.List(), 1)
}

// TestContinuingUnderAnInvalidNameChangesNothing tests that another tab
// confirming the dialog for an unfinished upload under a name the store
// refuses is told why, and that the tab sending the file keeps it.
func TestContinuingUnderAnInvalidNameChangesNothing(t *testing.T) {
	for name, fileName := range map[string]string{
		"empty":    "",
		"blank":    "   ",
		"too long": strings.Repeat("a", store.MaxNameLen+1),
	} {
		t.Run(name, func(t *testing.T) {
			tb := newTab(t)
			id := tb.start(t, "notes.txt", "notes.txt", "text/plain", len(contents)).ID
			require.Equal(t, http.StatusOK, tb.chunk(t, id, 0, contents[:10]))
			before, err := tb.files.Get(id)
			require.NoError(t, err)

			other := tb.openTab(t)
			require.NotNil(t,
				find(other.stage(t, "notes.txt", "text/plain", len(contents)),
					element("input", "name", "dup-0")),
				"the dialog does not offer to continue the upload")
			resp := other.DoForm(t, http.MethodPost, "/start/",
				url.Values{"name-0": {fileName}, "dup-0": {"continue"}})
			require.Equal(t, http.StatusOK, resp.Status)
			require.Contains(t, toast(t, resp), "invalid file name",
				"the visitor is not told what is wrong with the name")

			after, err := tb.files.Get(id)
			require.NoError(t, err)
			require.Equal(t, before.Owner, after.Owner,
				"a refused dialog took the transfer from the tab sending it")
			require.Equal(t, "notes.txt", after.Name, "a refused dialog renamed the file")

			resp = tb.pause(t, id)
			require.Equal(t, http.StatusOK, resp.Status)
			require.NotContains(t, toast(t, resp),
				"this tab does not hold the bytes of that file",
				"the tab sending the file can no longer pause it")
		})
	}
}

// TestStartRefusesTheWholeDialog tests that a dialog with one name the store
// refuses starts none of its files, not even those listed before that name.
func TestStartRefusesTheWholeDialog(t *testing.T) {
	tb := newTab(t)
	resp := tb.Do(t, http.MethodPost, "/stage/", json.RawMessage(
		`{"files":[{"name":"a.txt","size":20,"type":"text/plain"},`+
			`{"name":"b.txt","size":20,"type":"text/plain"}]}`))
	require.Equal(t, http.StatusOK, resp.Status, "staging")

	resp = tb.DoForm(t, http.MethodPost, "/start/",
		url.Values{"name-0": {"a.txt"}, "name-1": {" "}})
	require.Equal(t, http.StatusOK, resp.Status)
	require.Contains(t, toast(t, resp), "invalid file name",
		"the visitor is not told what is wrong with the name")
	require.Empty(t, tb.files.List(), "a refused dialog stored some of its files")
}

// TestSlowChunkOutlivesTheReadTimeout tests that a chunk slower than the read
// timeout of the server still arrives whole. A server times a whole request,
// which an upload held back by a limit outlives: the deadline has to follow
// the bytes instead.
func TestSlowChunkOutlivesTheReadTimeout(t *testing.T) {
	const idle = 400 * time.Millisecond
	tb := newTabWith(t, idle, 200*time.Millisecond)
	id := tb.start(t, "notes.txt", "notes.txt", "text/plain", len(contents)).ID

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

	req := tb.visitor.NewRequest(t, http.MethodPut, "/upload/"+id+"/?offset=0", pr)
	require.Equal(t, http.StatusOK, tb.visitor.Send(t, req).Status,
		"a slow chunk was cut off")

	f, err := tb.files.Get(id)
	require.NoError(t, err)
	require.Equal(t, store.StatusComplete, f.Status,
		"the slow chunk did not arrive whole")
}

// waitForLimits waits for the patch that writes the limits that took effect back
// into the controls of the tab, and returns its signals.
func (tb *tab) waitForLimits(t *testing.T) map[string]json.RawMessage {
	t.Helper()
	return tb.stream.WaitFor(t, datapagestest.Signal("limitUp")).Signals(t)
}

// TestLimits tests that the numbers the inputs send reach both transfer
// directions and come back to every tab as the values that took effect.
func TestLimits(t *testing.T) {
	payload := strings.Repeat("x", 32<<10)
	tb := newTab(t)

	resp := tb.Do(t, http.MethodPost, "/limits/",
		json.RawMessage(`{"limitUp":32,"limitDown":32}`))
	require.Equal(t, http.StatusOK, resp.Status, "setting the limits")
	signals := tb.waitForLimits(t)
	require.JSONEq(t, "32", string(signals["limitUp"]))
	require.JSONEq(t, "32", string(signals["limitDown"]))

	id := tb.start(t, "big.bin", "big.bin", "application/octet-stream", len(payload)).ID

	// 32 KiB at 32 KiB/s is a second of transfer, less the burst the limiter
	// starts with. The bound is loose: a busy machine may take much longer,
	// and only being faster than the limit would be a failure.
	start := time.Now()
	require.Equal(t, http.StatusOK, tb.chunk(t, id, 0, payload))
	require.Greater(t, time.Since(start), 300*time.Millisecond,
		"the upload limit did not slow the chunk down")

	start = time.Now()
	download := tb.visitor.Get(t, href.App.File(id))
	require.Greater(t, time.Since(start), 300*time.Millisecond,
		"the download limit did not slow the response down")
	require.Equal(t, payload, download.Body, "the limit changed the content")
}

// TestLimitsAreClamped tests that a number the inputs cannot produce is
// brought into range and reported back rather than refused.
func TestLimitsAreClamped(t *testing.T) {
	tb := newTab(t)
	resp := tb.Do(t, http.MethodPost, "/limits/",
		json.RawMessage(`{"limitUp":-5,"limitDown":999999999}`))
	require.Equal(t, http.StatusOK, resp.Status, "setting the limits")
	signals := tb.waitForLimits(t)
	require.JSONEq(t, strconv.Itoa(app.MinLimitKiB), string(signals["limitUp"]))
	require.JSONEq(t, strconv.Itoa(app.MaxLimitKiB), string(signals["limitDown"]))
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
			resp := tb.Do(t, http.MethodPost, "/limits/", json.RawMessage(`{"limitUp":`+
				rate+`,"unlimitedUp":false,"limitDown":1,"unlimitedDown":true}`))
			require.Equal(t, http.StatusOK, resp.Status, "setting the limit to %s", rate)
			require.JSONEq(t, strconv.Itoa(app.MinLimitKiB),
				string(tb.waitForLimits(t)["limitUp"]),
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
	resp := tb.Do(t, http.MethodPost, "/limits/", json.RawMessage(
		`{"limitUp":"128","unlimitedUp":false,"limitDown":"64","unlimitedDown":false}`))
	require.Equal(t, http.StatusOK, resp.Status, "setting the limits")
	require.JSONEq(t, "128", string(tb.waitForLimits(t)["limitUp"]))
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
			limits := func(rate string) json.RawMessage {
				return json.RawMessage(`{"limitUp":` + rate +
					`,"unlimitedUp":false,"limitDown":1,"unlimitedDown":true}`)
			}
			resp := tb.Do(t, http.MethodPost, "/limits/", limits(tc.from))
			require.Equal(t, http.StatusOK, resp.Status, "seeding the rate")
			// Each change sends the tab the limits once. Taking the patch of the seed
			// leaves the next one to the step.
			tb.waitForLimits(t)

			resp = tb.Do(t, http.MethodPost, "/limits/", limits(stepped(tc.from, tc.up)))
			require.Equal(t, http.StatusOK, resp.Status, "stepping the rate")
			require.JSONEq(t, tc.want, string(tb.waitForLimits(t)["limitUp"]))
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
	resp := tb.Do(t, http.MethodPost, "/limits/", json.RawMessage(
		`{"limitUp":256,"unlimitedUp":true,"limitDown":128,"unlimitedDown":false}`))
	require.Equal(t, http.StatusOK, resp.Status, "setting the limits")

	signals := tb.waitForLimits(t)
	require.JSONEq(t, "256", string(signals["limitUp"]))
	require.JSONEq(t, "true", string(signals["unlimitedUp"]))

	limits := tb.app.Limits()
	require.True(t, limits.Upload.Unlimited, "the switch did not reach the server")
	require.Zero(t, limits.Upload.Rate(), "an unlimited direction is still paced")
	require.EqualValues(t, 256, limits.Upload.KiB, "the rate was forgotten")
	require.EqualValues(t, 128*1024, limits.Download.Rate(), "the limited direction is not paced")
}

// halfway matches the progress bar of a file that holds 10 of its 20 bytes, which
// the list shows once the first chunk of contents arrived.
var halfway = element("neo-progress", "label", "10 B of 20 B")

// TestCompleteFileOffersItsLink tests that a finished file carries a copy
// control holding the absolute URL of the download, and that a file still
// receiving bytes carries none: that URL answers 404 until it completes.
func TestCompleteFileOffersItsLink(t *testing.T) {
	tb := newTab(t)
	id := tb.start(t, "notes.txt", "notes.txt", "text/plain", len(contents)).ID

	require.Equal(t, http.StatusOK, tb.chunk(t, id, 0, contents[:10]))
	require.Nil(t,
		find(tb.waitForList(t, "half the file", halfway), element("neo-clipcopy")),
		"an incomplete file offers a link that 404s")

	require.Equal(t, http.StatusOK, tb.chunk(t, id, 10, contents[10:]))
	clip := find(tb.waitForList(t, "a copy control", element("neo-clipcopy")),
		element("neo-clipcopy"))
	value, _ := datapagestest.Attr(clip, "value")
	// The absolute URL of the download is where a request for it goes.
	download := tb.visitor.NewRequest(t, http.MethodGet, href.App.File(id), nil)
	require.Equal(t, download.URL.String(), value,
		"the copy control does not carry the absolute download URL")
}

// TestCompleteFileShowsNoStatusIcon tests that a file that is all there
// carries no badge: it is the ordinary case, and the row already offers a
// download rather than a transfer.
func TestCompleteFileShowsNoStatusIcon(t *testing.T) {
	tb := newTab(t)
	id := tb.start(t, "notes.txt", "notes.txt", "text/plain", len(contents)).ID

	require.Equal(t, http.StatusOK, tb.chunk(t, id, 0, contents[:10]))
	require.NotNil(t, find(tb.waitForList(t, "half the file", halfway),
		class("status-uploading")), "a transfer in flight lost its badge")

	require.Equal(t, http.StatusOK, tb.chunk(t, id, 10, contents[10:]))
	list := tb.waitForList(t, "a copy control", element("neo-clipcopy"))
	// The badge, its icon and its note each carry a class that starts with status.
	badge := func(n *html.Node) bool {
		c, _ := datapagestest.Attr(n, "class")
		return strings.HasPrefix(c, "status")
	}
	require.Nil(t, find(list, badge), "a finished file still carries a badge")
}

// waitForList waits for a patch of the file list with an element that match
// reports true for, which what names, and returns the list. A transfer patches
// the list many times, and the earlier patches describe the file at an earlier
// point of its transfer.
func (tb *tab) waitForList(
	t *testing.T, what string, match func(*html.Node) bool,
) *html.Node {
	t.Helper()
	return tb.stream.WaitFor(t, fileList(what, match)).ElementByID(t, "file-list")
}

// fileList matches a patch of the file list with an element that match reports
// true for, which what names.
func fileList(what string, match func(*html.Node) bool) datapagestest.Match {
	return datapagestest.Func("a file list with "+what,
		func(ev datapagestest.Event) bool {
			list := datapagestest.ElementByID(ev.Root(), "file-list")
			return list != nil && find(list, match) != nil
		})
}

// TestRowCarriesAnActionsMenu tests that every row also renders its controls
// as one menu, which is what a phone shows in place of the row of buttons.
func TestRowCarriesAnActionsMenu(t *testing.T) {
	tb := newTab(t)
	id := tb.start(t, "notes.txt", "notes.txt", "text/plain", len(contents)).ID

	require.Equal(t, http.StatusOK, tb.chunk(t, id, 0, contents))
	menu := find(tb.waitForList(t, "a copy control", element("neo-clipcopy")),
		class("actions-menu"))
	require.NotNil(t, menu, "no menu button in the row")
	require.NotNil(t, find(menu, element("neo-menu")), "the menu button opens nothing")
	require.NotNil(t, find(menu, element("neo-menuitem", "data-neo-dialog-trigger", "")),
		"the menu cannot reach the delete confirmation")
	require.Contains(t, datapagestest.Text(menu), "Copy link",
		"the menu is missing the copy entry")
}

// TestUploadDialogStartsOnClick tests that the Start control carries the action itself.
// neo-button is not form-associated: type="submit" on it submits nothing
// and the dialog would sit there doing nothing when pressed.
func TestUploadDialogStartsOnClick(t *testing.T) {
	tb := newTab(t)
	resp := tb.Do(t, http.MethodPost, "/stage/", picked("notes.txt", "text/plain", 20))
	require.Equal(t, http.StatusOK, resp.Status, "staging")
	dialog := dialogOf(t, resp)

	start := find(dialog, func(n *html.Node) bool {
		return n.Data == "neo-button" && datapagestest.Text(n) == "Start upload"
	})
	require.NotNil(t, start, "no start control in the dialog")
	click, _ := datapagestest.Attr(start, "data-on:click")
	require.Contains(t, click, "@post('/start/'", "the start control carries no action")
	require.Nil(t, find(dialog, element("", "type", "submit")),
		"a custom element cannot submit a form")
}

// TestDownloadIsALink tests that both download controls can actually navigate.
// neo-button reads no href and neo-menuitem cancels the default of every click it sees:
// neither a button carrying an href nor an anchor nested in a menu row goes anywhere.
func TestDownloadIsALink(t *testing.T) {
	tb := newTab(t)
	id := tb.start(t, "notes.txt", "notes.txt", "text/plain", len(contents)).ID

	require.Equal(t, http.StatusOK, tb.chunk(t, id, 0, contents))
	list := tb.waitForList(t, "a copy control", element("neo-clipcopy"))

	require.NotNil(t, find(list, element("a", "data-neo-link", "",
		"href", href.App.File(id), "variant", "button-primary")),
		"the row's download is not a link")
	buttonWithHref := func(n *html.Node) bool {
		_, ok := datapagestest.Attr(n, "href")
		return n.Data == "neo-button" && ok
	}
	require.Nil(t, find(list, buttonWithHref),
		"an href on a neo-button navigates nowhere")
	require.NotNil(t, find(list, element("neo-menuitem", "data-href", href.App.File(id))),
		"the menu's download row carries no address")
}

// TestProgressCarriesTheBytes tests that the transferred amount rides on the
// progress bar as its label, which the component lays out against the
// percentage, rather than in a paragraph of its own below it.
func TestProgressCarriesTheBytes(t *testing.T) {
	tb := newTab(t)
	id := tb.start(t, "notes.txt", "notes.txt", "text/plain", len(contents)).ID

	require.Equal(t, http.StatusOK, tb.chunk(t, id, 0, contents[:10]))
	// The list the start renders comes first and has a bar too.
	list := tb.stream.WaitFor(t, fileList("half the file", halfway),
		"the bar carries no amount").ElementByID(t, "file-list")
	value, _ := datapagestest.Attr(find(list, halfway), "value")
	require.Equal(t, "50", value, "the bar carries no percentage")
	require.Nil(t, find(list, class("progress-text")), "the amount is still a paragraph")
}

// TestBadgeFollowsTheOwner tests that the badge agrees with the control beside
// it. A file paused in a tab that has since gone is one nobody can resume, so
// the row offers to take it over and the badge has to say interrupted rather
// than paused.
func TestBadgeFollowsTheOwner(t *testing.T) {
	tb := newTab(t)
	id := tb.start(t, "notes.txt", "notes.txt", "text/plain", len(contents)).ID
	require.Equal(t, http.StatusOK, tb.chunk(t, id, 0, contents[:10]))

	require.Equal(t, http.StatusOK, tb.pause(t, id).Status, "pausing")
	paused := tb.waitForList(t, "a paused badge", class("status-paused"))
	require.Contains(t, datapagestest.Text(paused), "You paused this upload",
		"the badge of a held file does not say who paused it")

	// The tab that held the bytes goes away, as a reload does.
	other := tb.openTab(t)
	tb.stream.Close()
	orphaned := other.waitForList(t, "an interrupted badge", class("status-interrupted"))
	require.Nil(t, find(orphaned, class("status-paused")),
		"a file nobody holds still badges as paused")
	require.Contains(t, datapagestest.Text(orphaned), "No tab is sending this file",
		"the badge does not say what happened to the file")
	require.Contains(t, datapagestest.Text(orphaned), "Take over",
		"the badge and the control disagree")
}

// TestDeleteAsksFirst tests that every row carries a confirmation naming its
// file, that the browser owns opening it, and that the file survives until the
// delete itself is sent.
func TestDeleteAsksFirst(t *testing.T) {
	tb := newTab(t)
	id := tb.start(t, "notes.txt", "holiday photos.txt", "text/plain", len(contents)).ID

	require.Equal(t, http.StatusOK, tb.chunk(t, id, 0, contents[:10]))
	dialog := find(tb.waitForList(t, "a dialog", element("neo-dialog")),
		element("neo-dialog"))
	require.Contains(t, datapagestest.Text(dialog), "Delete this file?",
		"the row carries no confirmation")
	require.Contains(t, datapagestest.Text(dialog), "holiday photos.txt",
		"the dialog does not name the file")
	require.NotNil(t, find(dialog, element("", "data-neo-dialog-trigger", "")),
		"nothing opens the dialog: a Morpheus dialog stays inert without a trigger")
	require.NotNil(t, find(dialog, element("", "data-neo-dialog-close", "")),
		"nothing cancels the dialog")
	require.Len(t, tb.files.List(), 1, "rendering the dialog deleted the file")

	require.Equal(t, http.StatusOK,
		tb.Do(t, http.MethodDelete, "/files/"+id+"/", nil).Status, "deleting")
	require.Empty(t, tb.files.List(), "the confirmed delete kept the file")
}

// TestDeleteDialogNamesTheStoredFile tests that the dialog shows the name the
// file is stored under rather than the one the browser reported.
func TestDeleteDialogNamesTheStoredFile(t *testing.T) {
	tb := newTab(t)
	id := tb.start(t, "original.txt", "renamed.txt", "text/plain", len(contents)).ID

	require.Equal(t, http.StatusOK, tb.chunk(t, id, 0, contents[:10]))
	name := find(tb.waitForList(t, "a dialog", element("neo-dialog")),
		element("strong", "class", "delete-name"))
	require.NotNil(t, name, "the dialog names no file")
	require.Equal(t, "renamed.txt", datapagestest.Text(name),
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
			resp := tb.Do(t, http.MethodPost, "/limits/", json.RawMessage(
				`{"limitUp":`+strconv.FormatInt(tc.limitKiB, 10)+
					`,"unlimitedUp":`+strconv.FormatBool(tc.unlimited)+
					`,"limitDown":1,"unlimitedDown":true}`))
			require.Equal(t, http.StatusOK, resp.Status, "setting the limits")

			started := tb.start(t, "f.bin", "f.bin", "text/plain", 4096)
			require.Equal(t, tc.want, started.ChunkSize,
				"the job does not carry the chunk size of a %d KiB/s limit", tc.limitKiB)
		})
	}
}

// TestLimitChangeResizesTransfers tests that changing the limit sends every tab
// the chunk size that took effect, which is what reaches a running transfer.
func TestLimitChangeResizesTransfers(t *testing.T) {
	tb := newTab(t)
	resp := tb.Do(t, http.MethodPost, "/limits/",
		json.RawMessage(`{"limitUp":4,"limitDown":0}`))
	require.Equal(t, http.StatusOK, resp.Status, "setting the limits")
	var size int64
	resize := tb.stream.WaitFor(t, calls("dpUpload.resize"))
	decodeCall(t, resize, "dpUpload.resize", &size)
	require.Equal(t, int64(4*1024*10), size,
		"the limit did not resize the running transfers")
}

// calls matches an event that makes the browser call fn. The server sends the
// uploader its commands as a script that calls it with one JSON argument.
func calls(fn string) datapagestest.Match {
	return datapagestest.Func("a script calling "+fn, func(ev datapagestest.Event) bool {
		_, ok := argOf(ev, fn)
		return ok
	})
}

// argOf returns the argument of the call of fn in the script ev appends, and false
// when ev appends no such script.
func argOf(ev datapagestest.Event, fn string) (string, bool) {
	script := find(ev.Root(), element("script"))
	if script == nil || script.FirstChild == nil {
		return "", false
	}
	arg, ok := strings.CutPrefix(script.FirstChild.Data, fn+"(")
	if !ok {
		return "", false
	}
	return strings.CutSuffix(arg, ")")
}

// decodeCall decodes the argument of the call of fn that ev makes into v.
func decodeCall(t *testing.T, ev datapagestest.Event, fn string, v any) {
	t.Helper()
	arg, ok := argOf(ev, fn)
	require.True(t, ok, "no call of %s in %q", fn, ev.Data["elements"])
	require.NoError(t, json.Unmarshal([]byte(arg), v), "decoding the argument of %s", fn)
}

// find returns the first element under root, root included, that match reports
// true for, and nil when there is none. It finds what a patch renders without an
// id, such as the controls of a row.
func find(root *html.Node, match func(*html.Node) bool) *html.Node {
	if root.Type == html.ElementNode && match(root) {
		return root
	}
	for n := range root.Descendants() {
		if n.Type == html.ElementNode && match(n) {
			return n
		}
	}
	return nil
}

// element matches an element named tag, of any name for "", that has the attributes
// attrs lists as key and value pairs.
func element(tag string, attrs ...string) func(*html.Node) bool {
	return func(n *html.Node) bool {
		if tag != "" && n.Data != tag {
			return false
		}
		for i := 0; i+1 < len(attrs); i += 2 {
			if v, ok := datapagestest.Attr(n, attrs[i]); !ok || v != attrs[i+1] {
				return false
			}
		}
		return true
	}
}

// class matches an element whose class attribute lists name.
func class(name string) func(*html.Node) bool {
	return func(n *html.Node) bool {
		v, _ := datapagestest.Attr(n, "class")
		return slices.Contains(strings.Fields(v), name)
	}
}

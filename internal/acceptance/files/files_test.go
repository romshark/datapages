// Asserts what a handler returning datapages.File answers with:
// the file and its headers, and the status of an error.

package acceptance_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/internal/acceptance/client"
	"github.com/romshark/datapages/internal/acceptance/files/app"
	"github.com/romshark/datapages/internal/acceptance/files/app/datapagesgen/href"
	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/messaging/inmem"
)

func newClient(t *testing.T) *client.Client {
	t.Helper()
	return client.New(t, mustNewServer(t, &app.App{},
		inmem.New(messaging.DefaultBrokerChanBuffer)))
}

// get sends a GET with the given request headers, the way a browser loads a
// URL from a link or an img element.
func get(t *testing.T, c *client.Client, path string, header ...string) client.Response {
	t.Helper()
	req := c.Request(t, http.MethodGet, path, "")
	req.Header.Del("Datastar-Request")
	for i := 0; i+1 < len(header); i += 2 {
		req.Header.Set(header[i], header[i+1])
	}
	return c.Do(t, req)
}

// TestFile tests a GET action answering with a file:
// the body, its type and the headers the File sets.
func TestFile(t *testing.T) {
	t.Parallel()
	c := newClient(t)

	link := href.App.File("a.txt", href.App.FileQuery("", false))
	require.Equal(t, "/files/a.txt", link)

	resp := get(t, c, link)
	require.Equal(t, http.StatusOK, resp.Status)
	require.Equal(t, "content of a.txt", resp.Body)
	require.Equal(t, "text/plain; charset=utf-8", resp.Header.Get("Content-Type"))
	require.Equal(t, "nosniff", resp.Header.Get("X-Content-Type-Options"))
	require.Equal(t, "public, max-age=60", resp.Header.Get("Cache-Control"))
	require.Equal(t,
		app.ModTime.Format(http.TimeFormat),
		resp.Header.Get("Last-Modified"))
	require.Empty(t, resp.Header.Get("Content-Disposition"))
}

// TestFileEscapedName tests a path value that needs escaping.
// The href builder escapes it into one segment and the handler reads it back whole.
func TestFileEscapedName(t *testing.T) {
	t.Parallel()
	c := newClient(t)

	link := href.App.File("a b/c.txt", href.App.FileQuery("", false))
	require.Equal(t, "/files/a%20b%2Fc.txt", link)

	resp := get(t, c, link)
	require.Equal(t, http.StatusOK, resp.Status)
	require.Equal(t, "content of a b/c.txt", resp.Body)
}

// TestFileTrailingSlash tests that the URL with a trailing slash reaches the
// same handler. The builder writes the URL without one.
func TestFileTrailingSlash(t *testing.T) {
	t.Parallel()
	c := newClient(t)

	resp := get(t, c, "/files/a.txt/")
	require.Equal(t, http.StatusOK, resp.Status)
	require.Equal(t, "content of a.txt", resp.Body)
}

// TestFileConditional tests the requests net/http.ServeContent answers on its
// own: HEAD, Range and If-Modified-Since.
func TestFileConditional(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		method        string
		header, value string
		wantStatus    int
		wantBody      string
	}{
		"head": {
			http.MethodHead,
			"", "",
			http.StatusOK, "",
		},
		"range": {
			http.MethodGet,
			"Range", "bytes=0-6",
			http.StatusPartialContent, "content",
		},
		"modified": {
			http.MethodGet,
			"If-Modified-Since", app.ModTime.Format(http.TimeFormat),
			http.StatusNotModified, "",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			c := newClient(t)

			req := c.Request(t, tc.method, "/files/a.txt", "")
			req.Header.Del("Datastar-Request")
			if tc.header != "" {
				req.Header.Set(tc.header, tc.value)
			}
			resp := c.Do(t, req)
			require.Equal(t, tc.wantStatus, resp.Status)
			require.Equal(t, tc.wantBody, resp.Body)
		})
	}
}

// TestFileDownload tests a File with a Filename,
// which the browser saves instead of showing.
func TestFileDownload(t *testing.T) {
	t.Parallel()
	c := newClient(t)

	resp := get(t, c, href.App.File("a.txt", href.App.FileQuery("", true)))
	require.Equal(t, http.StatusOK, resp.Status)
	require.Equal(t, "attachment; filename=a.txt", resp.Header.Get("Content-Disposition"))
}

// TestFileError tests the response to a failing handler. Only a document request
// gets the error page of the status. Any other gets the status text,
// which an img element or a fetch reads as the failure it is.
func TestFileError(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		fail       string
		dest       string // Sec-Fetch-Dest
		wantStatus int
		wantBody   string
	}{
		"not found": {
			"missing", "",
			http.StatusNotFound,
			"Not Found\n",
		},
		"not found image": {
			"missing", "image",
			http.StatusNotFound,
			"Not Found\n",
		},
		"not found document": {
			"missing", "document",
			http.StatusNotFound,
			`<p id="page">not found</p>`,
		},
		"error": {
			"error", "",
			http.StatusInternalServerError,
			"Internal Server Error\n",
		},
		"error document": {
			"error", "document",
			http.StatusInternalServerError,
			`<p id="page">internal error</p>`,
		},
		"panic": {
			"panic", "",
			http.StatusInternalServerError,
			"Internal Server Error\n",
		},
		"panic document": {
			"panic", "document",
			http.StatusInternalServerError,
			`<p id="page">internal error</p>`,
		},
		"incomplete file": {
			"type", "",
			http.StatusInternalServerError,
			"Internal Server Error\n",
		},
		"incomplete file document": {
			"type", "document",
			http.StatusInternalServerError,
			`<p id="page">internal error</p>`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			c := newClient(t)

			link := href.App.File("a.txt", href.App.FileQuery(tc.fail, false))
			var resp client.Response
			if tc.dest == "" {
				resp = get(t, c, link)
			} else {
				resp = get(t, c, link, "Sec-Fetch-Dest", tc.dest)
			}
			require.Equal(t, tc.wantStatus, resp.Status)
			if tc.dest == "document" {
				require.Contains(t, resp.Body, tc.wantBody)
				return
			}
			require.Equal(t, tc.wantBody, resp.Body)
			require.NotContains(t, resp.Body, "no type",
				"the body of an incomplete file was sent")
		})
	}
}

// TestPageGETAction tests a GET action of a page and the page builder next to it.
func TestPageGETAction(t *testing.T) {
	t.Parallel()
	c := newClient(t)

	require.Equal(t, "/doc/x/", href.PageDoc("x"))
	require.Equal(t, "/doc/x/export", href.PageDoc.Export("x"))

	page := get(t, c, href.PageDoc("x"))
	require.Equal(t, http.StatusOK, page.Status)
	require.Equal(t, "x", page.Element(t, "doc"))

	file := get(t, c, href.PageDoc.Export("x"))
	require.Equal(t, http.StatusOK, file.Status)
	require.Equal(t, "export of x", file.Body)
}

// TestPOSTFile tests an action of another method answering with a file.
// Its error is the status text, never the event stream RecoverError writes:
// a Datastar action reads no file, and the caller of one reads the status.
func TestPOSTFile(t *testing.T) {
	t.Parallel()
	c := newClient(t)

	resp := c.Action(t, http.MethodPost, "/upper/", "abc")
	require.Equal(t, http.StatusOK, resp.Status)
	require.Equal(t, "ABC", resp.Body)
	require.Equal(t, "nosniff", resp.Header.Get("X-Content-Type-Options"))

	resp = c.Action(t, http.MethodPost, "/upper/", "")
	require.Equal(t, http.StatusBadRequest, resp.Status)
	require.Equal(t, "Bad Request\n", resp.Body)
	require.NotContains(t, resp.Body, "recovered")
}

package httpserve_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/runtime/httpserve"
)

// closingReader records whether ServeFile closed it.
type closingReader struct {
	*strings.Reader
	closed bool
}

func (c *closingReader) Close() error {
	c.closed = true
	return nil
}

func serveFile(
	t *testing.T, r *http.Request, f datapages.File,
) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	require.NoError(t, httpserve.ServeFile(w, r, f))
	return w
}

// TestServeFile tests the headers ServeFile writes and that it closes the body.
func TestServeFile(t *testing.T) {
	t.Parallel()

	body := &closingReader{Reader: strings.NewReader("png bytes")}
	w := serveFile(t, httptest.NewRequest(http.MethodGet, "/", nil), datapages.File{
		Type:         "image/png",
		Body:         body,
		ModTime:      time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		CacheControl: "public, max-age=60",
	})
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "png bytes", w.Body.String())
	require.Equal(t, "image/png", w.Header().Get("Content-Type"))
	require.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
	require.Equal(t, "public, max-age=60", w.Header().Get("Cache-Control"))
	require.Equal(t, "Fri, 02 Jan 2026 03:04:05 GMT", w.Header().Get("Last-Modified"))
	require.Empty(t, w.Header().Get("Content-Disposition"))
	require.True(t, body.closed)
}

// TestServeFileRequests tests the requests net/http.ServeContent answers on
// its own: HEAD, Range and If-Modified-Since.
func TestServeFileRequests(t *testing.T) {
	t.Parallel()

	modTime := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	for name, tc := range map[string]struct {
		method        string
		header, value string
		wantCode      int
		wantBody      string
	}{
		"head": {
			http.MethodHead,
			"", "",
			http.StatusOK, "",
		},
		"range": {
			http.MethodGet,
			"Range", "bytes=2-4",
			http.StatusPartialContent, "cde",
		},
		"modified": {
			http.MethodGet,
			"If-Modified-Since", "Fri, 02 Jan 2026 03:04:05 GMT",
			http.StatusNotModified, "",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := httptest.NewRequest(tc.method, "/", nil)
			if tc.header != "" {
				r.Header.Set(tc.header, tc.value)
			}
			w := serveFile(t, r, datapages.File{
				Type: "text/plain", Body: strings.NewReader("abcdef"), ModTime: modTime,
			})
			require.Equal(t, tc.wantCode, w.Code)
			require.Equal(t, tc.wantBody, w.Body.String())
		})
	}
}

// TestServeFileFilename tests that a Filename turns the response into a
// download, a name with characters outside ASCII included.
func TestServeFileFilename(t *testing.T) {
	t.Parallel()

	for filename, want := range map[string]string{
		"report.pdf":  "attachment; filename=report.pdf",
		"my file.pdf": `attachment; filename="my file.pdf"`,
		"résumé.pdf":  "attachment; filename*=utf-8''r%C3%A9sum%C3%A9.pdf",
	} {
		t.Run(filename, func(t *testing.T) {
			t.Parallel()
			w := serveFile(t,
				httptest.NewRequest(http.MethodGet, "/", nil), datapages.File{
					Type:     "application/pdf",
					Body:     strings.NewReader("%PDF"),
					Filename: filename,
				})
			require.Equal(t, want, w.Header().Get("Content-Disposition"))
		})
	}
}

// TestServeFileIncomplete tests that a File without a Body or a Type is
// refused before anything is written, and that its Body is closed anyway.
func TestServeFileIncomplete(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		file datapages.File
		want error
	}{
		"no body": {datapages.File{Type: "text/plain"}, httpserve.ErrFileNoBody},
		"no type": {
			datapages.File{Body: &closingReader{Reader: strings.NewReader("x")}},
			httpserve.ErrFileNoType,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			w := httptest.NewRecorder()
			err := httpserve.ServeFile(w,
				httptest.NewRequest(http.MethodGet, "/", nil), tc.file)
			require.ErrorIs(t, err, tc.want)
			require.Empty(t, w.Header())
			require.Zero(t, w.Body.Len())
			if c, ok := tc.file.Body.(*closingReader); ok {
				require.True(t, c.closed)
			}
		})
	}
}

// TestIsDocumentRequest tests that only Sec-Fetch-Dest: document counts.
func TestIsDocumentRequest(t *testing.T) {
	t.Parallel()

	for dest, want := range map[string]bool{
		"document": true,
		"image":    false,
		"empty":    false,
		"iframe":   false,
		"":         false,
	} {
		t.Run(dest, func(t *testing.T) {
			t.Parallel()
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			if dest != "" {
				r.Header.Set("Sec-Fetch-Dest", dest)
			}
			require.Equal(t, want, httpserve.IsDocumentRequest(r))
		})
	}
}

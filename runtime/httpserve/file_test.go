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
		Type:    "image/png",
		Body:    body,
		ModTime: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		Cache:   datapages.FileCache{MaxAge: time.Minute},
	})
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "png bytes", w.Body.String())
	require.Equal(t, "image/png", w.Header().Get("Content-Type"))
	require.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
	require.Equal(t, "max-age=60", w.Header().Get("Cache-Control"))
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
				Type:    "text/plain",
				Body:    &closingReader{Reader: strings.NewReader("abcdef")},
				ModTime: modTime,
			})
			require.Equal(t, tc.wantCode, w.Code)
			require.Equal(t, tc.wantBody, w.Body.String())
		})
	}
}

// TestServeFileDisposition tests the Content-Disposition a FileDisposition sets:
// a download or a response the browser shows, with a name or without,
// a name with characters outside ASCII included.
func TestServeFileDisposition(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		datapages.FileDisposition
		want string
	}{
		"zero": {},
		"download": {
			Download: true,
			want:     "attachment",
		},
		"download with name": {
			Download: true,
			Filename: "report.pdf",
			want:     "attachment; filename=report.pdf",
		},
		"shown with name": {
			Filename: "report.pdf",
			want:     "inline; filename=report.pdf",
		},
		"space": {
			Download: true,
			Filename: "my file.pdf",
			want:     `attachment; filename="my file.pdf"`,
		},
		"non-ascii": {
			Download: true,
			Filename: "résumé.pdf",
			want:     "attachment; filename*=utf-8''r%C3%A9sum%C3%A9.pdf",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			w := serveFile(t,
				httptest.NewRequest(http.MethodGet, "/", nil), datapages.File{
					Type:        "application/pdf",
					Body:        &closingReader{Reader: strings.NewReader("%PDF")},
					Disposition: tc.FileDisposition,
				})
			require.Equal(t, tc.want, w.Header().Get("Content-Disposition"))
		})
	}
}

// TestServeFileCache tests the Cache-Control a FileCache sets, and that
// fields which exclude each other are refused before anything is written.
func TestServeFileCache(t *testing.T) {
	t.Parallel()

	year := 365 * 24 * time.Hour
	for name, tc := range map[string]struct {
		datapages.FileCache
		// middleware is the Cache-Control a middleware set before.
		middleware string
		want       string
		wantErr    bool
	}{
		"zero": {
			want: "no-cache",
		},
		"zero after middleware": {
			middleware: "public, max-age=60",
			want:       "public, max-age=60",
		},
		"set after middleware": {
			MaxAge:     time.Hour,
			middleware: "public, max-age=60",
			want:       "max-age=3600",
		},
		"private with no-store": {
			Private: true,
			NoStore: true,
			want:    "private, no-store",
		},
		"no-cache with no-store": {
			NoCache: true,
			NoStore: true,
			want:    "no-cache, no-store",
		},
		"no-cache with max age": {
			NoCache: true,
			MaxAge:  time.Hour,
			wantErr: true,
		},
		"max age": {
			MaxAge: time.Hour,
			want:   "max-age=3600",
		},
		"truncated": {
			MaxAge: 1500 * time.Millisecond,
			want:   "max-age=1",
		},
		"immutable": {
			Private:   true,
			MaxAge:    year,
			Immutable: true,
			want:      "private, max-age=31536000, immutable",
		},
		"no-cache": {
			Private: true,
			NoCache: true,
			want:    "private, no-cache",
		},
		"no-store": {
			NoStore: true,
			want:    "no-store",
		},
		"raw": {
			Raw:  "public, s-maxage=600",
			want: "public, s-maxage=600",
		},
		"negative": {
			MaxAge:  -time.Second,
			wantErr: true,
		},
		"under 1s": {
			MaxAge:  time.Millisecond,
			wantErr: true,
		},
		"immutable without max age": {
			Immutable: true,
			wantErr:   true,
		},
		"immutable with no-cache": {
			MaxAge:    year,
			Immutable: true,
			NoCache:   true,
			wantErr:   true,
		},
		"no-store with max age": {
			NoStore: true,
			MaxAge:  time.Hour,
			wantErr: true,
		},
		"raw with private": {
			Raw:     "no-cache",
			Private: true,
			wantErr: true,
		},
		"raw with no-store": {
			Raw:     "no-cache",
			NoStore: true,
			wantErr: true,
		},
		"raw with newline": {
			Raw:     "no-cache\r\nX-Injected: 1",
			wantErr: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			body := &closingReader{Reader: strings.NewReader("x")}
			w := httptest.NewRecorder()
			if tc.middleware != "" {
				w.Header().Set("Cache-Control", tc.middleware)
			}
			err := httpserve.ServeFile(w, httptest.NewRequest(http.MethodGet, "/", nil),
				datapages.File{Type: "text/plain", Body: body, Cache: tc.FileCache})
			require.True(t, body.closed)
			if tc.wantErr {
				require.ErrorIs(t, err, httpserve.ErrFileInvalidCache)
				require.Empty(t, w.Header())
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, w.Header().Get("Cache-Control"))
		})
	}
}

// TestServeFileETag tests the ETag a File sets, quoted, and the conditional
// requests net/http.ServeContent answers with it.
func TestServeFileETag(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		header   map[string]string
		wantCode int
		wantBody string
	}{
		"unconditional":       {nil, http.StatusOK, "abcdef"},
		"if-none-match":       {map[string]string{"If-None-Match": `"v1"`}, http.StatusNotModified, ""},
		"if-none-match other": {map[string]string{"If-None-Match": `"v2"`}, http.StatusOK, "abcdef"},
		"if-match other":      {map[string]string{"If-Match": `"v2"`}, http.StatusPreconditionFailed, ""},
		"if-range": {
			map[string]string{"Range": "bytes=0-1", "If-Range": `"v1"`},
			http.StatusPartialContent, "ab",
		},
		"if-range other": {
			map[string]string{"Range": "bytes=0-1", "If-Range": `"v2"`},
			http.StatusOK, "abcdef",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			for k, v := range tc.header {
				r.Header.Set(k, v)
			}
			w := serveFile(t, r, datapages.File{
				Type: "text/plain",
				Body: &closingReader{Reader: strings.NewReader("abcdef")},
				ETag: "v1",
			})
			require.Equal(t, `"v1"`, w.Header().Get("ETag"))
			require.Equal(t, tc.wantCode, w.Code)
			require.Equal(t, tc.wantBody, w.Body.String())
		})
	}
}

// TestServeFileRefused tests that a File without a Body or a Type, or with an
// ETag that cannot stand between quotes, is refused before anything is
// written, and that its Body is closed anyway.
func TestServeFileRefused(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		datapages.File
		want error
	}{
		"no body": {datapages.File{Type: "text/plain"}, httpserve.ErrFileNoBody},
		"no type": {
			Body: &closingReader{Reader: strings.NewReader("x")},
			want: httpserve.ErrFileNoType,
		},
		"quoted etag": {
			Type: "text/plain", ETag: `"v1"`,
			Body: &closingReader{Reader: strings.NewReader("x")},
			want: httpserve.ErrFileInvalidETag,
		},
		"etag with space": {
			Type: "text/plain", ETag: "v 1",
			Body: &closingReader{Reader: strings.NewReader("x")},
			want: httpserve.ErrFileInvalidETag,
		},
		"reserved header": {
			Type:   "text/plain",
			Header: http.Header{"Cache-Control": {"no-store"}},
			Body:   &closingReader{Reader: strings.NewReader("x")},
			want:   httpserve.ErrFileReservedHeader,
		},
		"reserved header, not canonical": {
			Type:   "text/plain",
			Header: http.Header{"etag": {`"v1"`}},
			Body:   &closingReader{Reader: strings.NewReader("x")},
			want:   httpserve.ErrFileReservedHeader,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			w := httptest.NewRecorder()
			err := httpserve.ServeFile(w,
				httptest.NewRequest(http.MethodGet, "/", nil), tc.File)
			require.ErrorIs(t, err, tc.want)
			require.Empty(t, w.Header())
			require.Zero(t, w.Body.Len())
			if c, ok := tc.Body.(*closingReader); ok {
				require.True(t, c.closed)
			}
		})
	}
}

// TestServeFileHeader tests the headers File.Header adds, and that one of them
// replaces the value a middleware set before.
func TestServeFileHeader(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	w.Header().Set("Content-Language", "en")
	w.Header().Set("Vary", "Cookie")
	require.NoError(t, httpserve.ServeFile(w,
		httptest.NewRequest(http.MethodGet, "/", nil), datapages.File{
			Type: "image/svg+xml",
			Body: &closingReader{Reader: strings.NewReader("<svg/>")},
			Header: http.Header{
				"Content-Language":        {"de"},
				"Content-Security-Policy": {"sandbox"},
			},
		}))
	require.Equal(t, []string{"de"}, w.Header().Values("Content-Language"))
	require.Equal(t, "sandbox", w.Header().Get("Content-Security-Policy"))
	require.Equal(t, "Cookie", w.Header().Get("Vary"))
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

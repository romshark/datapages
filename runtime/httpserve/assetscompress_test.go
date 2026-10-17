package httpserve_test

import (
	"cmp"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/modules/offline"
	"github.com/romshark/datapages/runtime/httpserve"
)

var assetCSS = strings.Repeat(".card { border: 1px solid #ddd; padding: 1rem; }\n", 200)

func assetFiles() fstest.MapFS {
	return fstest.MapFS{
		"style.css": {
			Data: []byte(assetCSS),
		},
		"page.html": {
			Data: []byte("<!DOCTYPE html>" + strings.Repeat("<p>static</p>", 500)),
		},
		"sub/nested.css": {
			Data: []byte(assetCSS + "/* nested */\n"),
		},
		"docs/index.html": {
			Data: []byte("<!DOCTYPE html>" + strings.Repeat("<p>docs</p>", 500)),
		},
	}
}

func assetsCore(
	t testing.TB, fsys http.FileSystem, conf datapages.ServerConfig,
) *httpserve.Core {
	t.Helper()
	conf.AssetsFS = fsys
	c, err := httpserve.NewCore(conf, "/static/")
	require.NoError(t, err)
	c.Build()
	return c
}

func getAsset(
	t testing.TB, c *httpserve.Core, method, path, accept string, header http.Header,
) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, path, nil)
	r.Header.Set("Accept-Encoding", cmp.Or(accept, "identity"))
	for k, v := range header {
		r.Header[k] = v
	}
	w := httptest.NewRecorder()
	c.ServeHTTP(w, r)
	return w
}

// TestAssetsCompression tests the compressor a server without
// WithAssetsCompression uses, behind the prefix and the directory check.
func TestAssetsCompression(t *testing.T) {
	t.Parallel()

	c := assetsCore(t, http.FS(assetFiles()), datapages.ServerConfig{})
	for name, tc := range map[string]struct {
		path, accept string
		wantStatus   int
		wantCoding   string
		wantBody     string // Decoded; "" for none.
	}{
		"brotli": {
			path: "/static/style.css", accept: browserCodings,
			wantCoding: "br", wantBody: assetCSS,
		},
		"gzip": {
			path: "/static/sub/nested.css", accept: "gzip",
			wantCoding: "gzip", wantBody: assetCSS + "/* nested */\n",
		},
		"directory index": {
			path: "/static/docs/", accept: browserCodings,
			wantCoding: "br",
			wantBody:   "<!DOCTYPE html>" + strings.Repeat("<p>docs</p>", 500),
		},
		"directory": {
			path: "/static/sub/", accept: browserCodings,
			wantStatus: http.StatusNotFound,
		},
		"missing": {
			path: "/static/nope.css", accept: browserCodings,
			wantStatus: http.StatusNotFound,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			w := getAsset(t, c, http.MethodGet, tc.path, tc.accept, nil)
			require.Equal(t, cmp.Or(tc.wantStatus, http.StatusOK), w.Code)
			require.Equal(t, tc.wantCoding, w.Header().Get("Content-Encoding"))
			if tc.wantBody != "" {
				require.True(t, varies(w.Header()))
				require.Equal(t, tc.wantBody, decode(t, tc.wantCoding, w.Body.Bytes()))
			}
		})
	}
}

// TestAssetsCompressionETag tests that WithAssetsCache sets the tag the copies
// extend, and that a conditional request matches the tag of its own copy.
func TestAssetsCompressionETag(t *testing.T) {
	t.Parallel()

	c := assetsCore(t, http.FS(assetFiles()), datapages.ServerConfig{
		AssetsCache: &datapages.AssetsCacheConfig{},
	})
	tag := getAsset(t,
		c, http.MethodGet, "/static/style.css", "", nil).Header().Get("Etag")
	require.True(t, strings.HasPrefix(tag, `"`), "identity tag %q", tag)
	base := strings.TrimSuffix(tag, `"`)

	for coding, want := range map[string]string{
		"br":   base + `.br6"`,
		"gzip": base + `.gzip9"`,
	} {
		w := getAsset(t, c, http.MethodGet, "/static/style.css", coding, nil)
		require.Equal(t, want, w.Header().Get("Etag"), coding)
		require.Equal(t, "public, max-age=0", w.Header().Get("Cache-Control"), coding)

		w = getAsset(t, c, http.MethodGet, "/static/style.css", coding,
			http.Header{"If-None-Match": {want}})
		require.Equal(t, http.StatusNotModified, w.Code, coding)

		w = getAsset(t, c, http.MethodGet, "/static/style.css", coding,
			http.Header{"If-None-Match": {tag}})
		require.Equal(t, http.StatusOK, w.Code,
			"%s: the tag of the uncompressed file matched the copy", coding)
	}
}

// TestAssetsCompressionOff tests WithNoAssetsCompression and dev mode,
// which sends assets uncompressed whatever the compressor.
func TestAssetsCompressionOff(t *testing.T) {
	for name, tc := range map[string]struct {
		devMode bool
		opt     datapages.ServerOption
	}{
		"WithNoAssetsCompression": {opt: datapages.WithNoAssetsCompression()},
		"dev mode":                {devMode: true},
		"dev mode with a compressor that ignores it": {
			devMode: true,
			opt:     datapages.WithAssetsCompression(new(recordingCompressor)),
		},
	} {
		t.Run(name, func(t *testing.T) {
			if tc.devMode {
				t.Setenv(datapages.EnvVarDevMode, "1")
			}
			var cfg datapages.ServerConfig
			if tc.opt != nil {
				require.NoError(t, tc.opt(&cfg))
			}
			c := assetsCore(t, http.FS(assetFiles()), cfg)
			w := getAsset(t, c, http.MethodGet, "/static/style.css", browserCodings, nil)
			require.Empty(t, w.Header().Get("Content-Encoding"))
			require.Empty(t, w.Header().Get("X-Compressor"))
			require.False(t, varies(w.Header()))
			require.Equal(t, assetCSS, w.Body.String())
		})
	}
}

// recordingCompressor marks the responses of its handler and records what the
// server passed it.
type recordingCompressor struct {
	fsys   http.FileSystem
	logger *slog.Logger
}

func (rc *recordingCompressor) Handler(
	fsys http.FileSystem, next http.Handler, logger *slog.Logger,
) http.Handler {
	rc.fsys, rc.logger = fsys, logger
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Compressor", "1")
		next.ServeHTTP(w, r)
	})
}

// TestAssetsCompressor tests that the server sends its asset requests through
// the handler of the compressor of WithAssetsCompression, which it passes the
// file system it serves and its logger.
func TestAssetsCompressor(t *testing.T) {
	t.Parallel()

	rc := new(recordingCompressor)
	var cfg datapages.ServerConfig
	require.NoError(t, datapages.WithAssetsCompression(rc)(&cfg))
	c := assetsCore(t, http.FS(assetFiles()), cfg)
	w := getAsset(t, c, http.MethodGet, "/static/style.css", browserCodings, nil)
	require.Equal(t, "1", w.Header().Get("X-Compressor"))
	require.Equal(t, assetCSS, w.Body.String())
	require.NotNil(t, rc.logger)

	f, err := rc.fsys.Open("/style.css")
	require.NoError(t, err)
	b, err := io.ReadAll(f)
	require.NoError(t, f.Close())
	require.NoError(t, err)
	require.Equal(t, assetCSS, string(b))
}

// TestAssetsPassMiddlewareThrough tests that a middleware that edits HTML,
// as offline.Middleware does with a page, leaves an HTML asset as the bytes
// of its file, uncompressed and compressed.
func TestAssetsPassMiddlewareThrough(t *testing.T) {
	t.Parallel()

	files := assetFiles()
	c := assetsCore(t, http.FS(files), datapages.ServerConfig{
		Middleware: []func(http.Handler) http.Handler{
			offline.Middleware("/offline/", offline.Config{}),
		},
	})
	c.Mux().HandleFunc("GET /page/", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "<!DOCTYPE html><html><head></head><body></body></html>")
	})
	want := string(files["page.html"].Data)

	w := getAsset(t, c, http.MethodGet, "/static/page.html", "", nil)
	require.Equal(t, want, w.Body.String())
	w = getAsset(t, c, http.MethodGet, "/static/page.html", "br", nil)
	require.Equal(t, want, decode(t, "br", w.Body.Bytes()))

	w = getAsset(t, c, http.MethodGet, "/page/", "", nil)
	require.Contains(t, w.Body.String(), "<script", "the page got no offline script")
}

// BenchmarkAssetsServe compares disabled, identity and cached Brotli requests.
func BenchmarkAssetsServe(b *testing.B) {
	for _, tc := range []struct {
		name, accept string
		opt          datapages.ServerOption
	}{
		{"off", "br", datapages.WithNoAssetsCompression()},
		{"identity", "", nil},
		{"brotli", "br", nil},
	} {
		b.Run(tc.name, func(b *testing.B) {
			var cfg datapages.ServerConfig
			if tc.opt != nil {
				require.NoError(b, tc.opt(&cfg))
			}
			c := assetsCore(b, http.FS(assetFiles()), cfg)
			getAsset(b, c, http.MethodGet, "/static/style.css", tc.accept, nil)
			b.ReportAllocs()
			for b.Loop() {
				getAsset(b, c, http.MethodGet, "/static/style.css", tc.accept, nil)
			}
		})
	}
}

// BenchmarkAssetsServeParallel measures cached copies under parallel load.
func BenchmarkAssetsServeParallel(b *testing.B) {
	for _, tc := range []struct{ name, accept string }{
		{"identity", ""},
		{"brotli", "br"},
	} {
		b.Run(tc.name, func(b *testing.B) {
			c := assetsCore(b, http.FS(assetFiles()), datapages.ServerConfig{
				AssetsCache: &datapages.AssetsCacheConfig{},
			})
			getAsset(b, c, http.MethodGet, "/static/style.css", tc.accept, nil)
			b.ReportAllocs()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					getAsset(b, c, http.MethodGet, "/static/style.css", tc.accept, nil)
				}
			})
		})
	}
}

package httpserve_test

import (
	"bytes"
	"cmp"
	"compress/gzip"
	"io"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/a-h/templ"
	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"
	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/runtime/httpserve"
)

const browserCodings = "gzip, deflate, br, zstd"

// listItems returns about n bytes of deterministic, varied list items.
func listItems(n int) []string {
	r := rand.New(rand.NewPCG(3, 4))
	words := strings.Fields("alpha bravo charlie delta echo foxtrot golf " +
		"hotel india juliet kilo lima mike november oscar papa quebec romeo " +
		"sierra tango uniform victor whiskey xray yankee zulu")
	var items []string
	for size := 0; size < n; {
		var b strings.Builder
		b.WriteString(`<li id="item-` + strconv.Itoa(len(items)) +
			`"><input type="checkbox">`)
		for range 3 + r.IntN(4) {
			b.WriteString(" " + words[r.IntN(len(words))])
		}
		b.WriteString("</li>")
		items = append(items, b.String())
		size += b.Len()
	}
	return items
}

// listPage returns a page in the short writes a templ component makes.
func listPage(n int) []string {
	return slices.Concat(
		[]string{`<!DOCTYPE html><html><head><title>List</title></head>` +
			`<body><ul id="list">`},
		listItems(n),
		[]string{`</ul></body></html>`},
	)
}

func writeParts(w http.ResponseWriter, parts []string) {
	for i, p := range parts {
		if i%2 == 0 {
			_, _ = io.WriteString(w, p)
		} else {
			_, _ = w.Write([]byte(p))
		}
	}
}

func decode(t testing.TB, coding string, b []byte) string {
	t.Helper()
	var r io.Reader
	switch coding {
	case "zstd":
		d, err := zstd.NewReader(bytes.NewReader(b))
		require.NoError(t, err)
		defer d.Close()
		r = d
	case "br":
		r = brotli.NewReader(bytes.NewReader(b))
	case "gzip":
		zr, err := gzip.NewReader(bytes.NewReader(b))
		require.NoError(t, err)
		r = zr
	default:
		return string(b)
	}
	out, err := io.ReadAll(r)
	require.NoError(t, err)
	return string(out)
}

func varies(h http.Header) bool {
	for _, v := range h.Values("Vary") {
		for t := range strings.SplitSeq(v, ",") {
			if strings.EqualFold(strings.TrimSpace(t), "Accept-Encoding") {
				return true
			}
		}
	}
	return false
}

func serveCompressed(
	t testing.TB, c *httpserve.Core, method, accept string,
) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, "/page/", nil)
	if accept != "" {
		r.Header.Set("Accept-Encoding", accept)
	}
	w := httptest.NewRecorder()
	c.ServeHTTP(w, r)
	return w
}

// TestResponseCompression tests selection, exclusions and response headers.
func TestResponseCompression(t *testing.T) {
	t.Parallel()

	page := listPage(8 << 10)
	body := strings.Join(page, "")
	small := `<!DOCTYPE html><html><body>` + strings.Repeat("<p>small</p>", 10) +
		`</body></html>`
	require.Less(t, len(small), datapages.DefaultResponseCompressionMinSize)
	pageWith := func(header http.Header, status int) http.HandlerFunc {
		return func(w http.ResponseWriter, _ *http.Request) {
			for k, v := range header {
				w.Header()[k] = v
			}
			if status != 0 {
				w.WriteHeader(status)
			}
			writeParts(w, page)
		}
	}
	lowCPU := datapages.ResponseCompressionLowCPU()
	lowBandwidth := datapages.ResponseCompressionLowBandwidth()

	for name, tc := range map[string]struct {
		conf       *datapages.ResponseCompressionConfig
		method     string
		accept     string
		handler    http.HandlerFunc
		wantBody   string // Defaults to body.
		wantStatus int    // Defaults to 200.
		wantCoding string // The Content-Encoding.
		wantVary   bool
		wantHeader http.Header // Values of the listed keys.
	}{
		"zstd for a browser": {
			accept: browserCodings, handler: pageWith(nil, 0),
			wantCoding: "zstd", wantVary: true,
			wantHeader: http.Header{
				"Content-Type":   {"text/html; charset=utf-8"},
				"Content-Length": nil,
			},
		},
		"brotli without zstd": {
			accept: "gzip, deflate, br", handler: pageWith(nil, 0),
			wantCoding: "br", wantVary: true,
		},
		"gzip only": {
			accept: "gzip", handler: pageWith(nil, 0),
			wantCoding: "gzip", wantVary: true,
		},
		"wildcard": {
			accept: "*", handler: pageWith(nil, 0),
			wantCoding: "gzip", wantVary: true,
		},
		"zstd refused": {
			accept: "zstd;q=0, br", handler: pageWith(nil, 0),
			wantCoding: "br", wantVary: true,
		},
		"no coding accepted": {
			handler:  pageWith(nil, 0),
			wantVary: true,
		},
		"below the minimum size": {
			accept: browserCodings,
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(w, small)
			},
			wantBody: small,
		},
		"minimum size 1": {
			conf:   &datapages.ResponseCompressionConfig{MinSize: 1},
			accept: browserCodings,
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(w, small)
			},
			wantBody:   small,
			wantCoding: "zstd", wantVary: true,
		},
		"error page": {
			accept:     browserCodings,
			handler:    pageWith(nil, http.StatusInternalServerError),
			wantStatus: http.StatusInternalServerError,
			wantCoding: "zstd", wantVary: true,
		},
		"json": {
			accept: browserCodings,
			handler: pageWith(http.Header{
				"Content-Type": {"application/json"},
			}, 0),
			wantCoding: "zstd", wantVary: true,
			wantHeader: http.Header{"Content-Type": {"application/json"}},
		},
		"svg": {
			accept: browserCodings,
			handler: pageWith(http.Header{
				"Content-Type": {"image/svg+xml"},
			}, 0),
			wantCoding: "zstd", wantVary: true,
		},
		"image": {
			accept: browserCodings,
			handler: pageWith(http.Header{
				"Content-Type": {"image/png"},
			}, 0),
		},
		"event stream": {
			accept: browserCodings,
			handler: pageWith(http.Header{
				"Content-Type": {"text/event-stream"},
			}, 0),
		},
		"Content-Type suppressed": {
			accept: browserCodings,
			handler: pageWith(http.Header{
				"Content-Type": nil,
			}, 0),
		},
		"encoded by the handler": {
			accept: browserCodings,
			handler: pageWith(http.Header{
				"Content-Encoding": {"x-custom"},
			}, 0),
			wantCoding: "x-custom",
		},
		"no-transform": {
			accept: browserCodings,
			handler: pageWith(http.Header{
				"Cache-Control": {"private, No-Transform"},
			}, 0),
		},
		"byte ranges": {
			accept: browserCodings,
			handler: pageWith(http.Header{
				"Accept-Ranges": {"bytes"},
			}, 0),
		},
		"no byte ranges": {
			accept: browserCodings,
			handler: pageWith(http.Header{
				"Accept-Ranges": {"none"},
			}, 0),
			wantCoding: "zstd", wantVary: true,
		},
		"partial content": {
			accept: browserCodings,
			handler: pageWith(http.Header{
				"Content-Range": {"bytes 0-9/100"},
			}, http.StatusPartialContent),
			wantStatus: http.StatusPartialContent,
		},
		"no content": {
			accept: browserCodings,
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			},
			wantStatus: http.StatusNoContent,
			wantBody:   "-",
		},
		"not modified": {
			accept: browserCodings,
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNotModified)
			},
			wantStatus: http.StatusNotModified,
			wantBody:   "-",
		},
		"HEAD": {
			method: http.MethodHead, accept: browserCodings,
			handler: pageWith(nil, 0),
		},
		"strong ETag": {
			accept: browserCodings,
			handler: pageWith(http.Header{
				"Etag": {`"v1"`},
			}, 0),
			wantCoding: "zstd", wantVary: true,
			wantHeader: http.Header{"Etag": {`W/"v1"`}},
		},
		"weak ETag": {
			accept: browserCodings,
			handler: pageWith(http.Header{
				"Etag": {`W/"v1"`},
			}, 0),
			wantCoding: "zstd", wantVary: true,
			wantHeader: http.Header{"Etag": {`W/"v1"`}},
		},
		"uncompressed ETag": {
			handler: pageWith(http.Header{
				"Etag": {`"v1"`},
			}, 0),
			wantVary:   true,
			wantHeader: http.Header{"Etag": {`"v1"`}},
		},
		"Content-Length": {
			accept: browserCodings,
			handler: pageWith(http.Header{
				"Content-Length": {strconv.Itoa(len(body))},
			}, 0),
			wantCoding: "zstd", wantVary: true,
			wantHeader: http.Header{"Content-Length": nil},
		},
		"Vary kept": {
			accept: browserCodings,
			handler: pageWith(http.Header{
				"Vary": {"Cookie"},
			}, 0),
			wantCoding: "zstd", wantVary: true,
			wantHeader: http.Header{"Vary": {"Cookie", "Accept-Encoding"}},
		},
		"Vary *": {
			accept: browserCodings,
			handler: pageWith(http.Header{
				"Vary": {"*"},
			}, 0),
			wantCoding: "zstd",
			wantHeader: http.Header{"Vary": {"*"}},
		},
		"status after a write": {
			accept: browserCodings,
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(w, page[0])
				w.WriteHeader(http.StatusInternalServerError)
				writeParts(w, page[1:])
			},
			wantCoding: "zstd", wantVary: true,
		},
		"flush below the minimum size": {
			accept: browserCodings,
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(w, page[0])
				require.NoError(t, http.NewResponseController(w).Flush())
				writeParts(w, page[1:])
			},
			wantCoding: "zstd", wantVary: true,
		},
		"disabled": {
			conf:    &datapages.ResponseCompressionConfig{Disabled: true},
			accept:  browserCodings,
			handler: pageWith(nil, 0),
		},
		"low CPU without zstd": {
			conf:   &lowCPU,
			accept: "gzip, deflate, br", handler: pageWith(nil, 0),
			wantCoding: "gzip", wantVary: true,
		},
		"low bandwidth for a browser": {
			conf:   &lowBandwidth,
			accept: browserCodings, handler: pageWith(nil, 0),
			wantCoding: "br", wantVary: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c := mustCore(t, datapages.ServerConfig{ResponseCompression: tc.conf}, "")
			c.Mux().HandleFunc("/", tc.handler)
			c.Build()

			w := serveCompressed(t, c, cmp.Or(tc.method, http.MethodGet), tc.accept)
			require.Equal(t, cmp.Or(tc.wantStatus, http.StatusOK), w.Code)
			require.Equal(t, tc.wantCoding, w.Header().Get("Content-Encoding"))
			require.Equal(t, tc.wantVary, varies(w.Header()), "Vary: %q",
				w.Header().Values("Vary"))
			for k, v := range tc.wantHeader {
				require.Equal(t, v, w.Header().Values(k), k)
			}
			if want := cmp.Or(tc.wantBody, body); want == "-" {
				require.Empty(t, w.Body.String())
			} else {
				require.Equal(t, want, decode(t, tc.wantCoding, w.Body.Bytes()))
			}
		})
	}
}

// TestResponseCompressionAfterMiddleware tests that middleware edits plain text.
func TestResponseCompressionAfterMiddleware(t *testing.T) {
	t.Parallel()

	page := listPage(8 << 10)
	var seen bytes.Buffer
	c := mustCore(t, datapages.ServerConfig{
		Middleware: []func(http.Handler) http.Handler{
			func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					next.ServeHTTP(teeWriter{w, &seen}, r)
				})
			},
		},
	}, "")
	c.Mux().HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		writeParts(w, page)
	})
	c.Build()

	w := serveCompressed(t, c, http.MethodGet, browserCodings)
	require.Equal(t, "zstd", w.Header().Get("Content-Encoding"))
	require.Equal(t, strings.Join(page, ""), seen.String())
	require.Equal(t, seen.String(), decode(t, "zstd", w.Body.Bytes()))
}

type teeWriter struct {
	http.ResponseWriter
	tee *bytes.Buffer
}

func (w teeWriter) Write(p []byte) (int, error) {
	w.tee.Write(p)
	return w.ResponseWriter.Write(p)
}

// TestResponseCompressionFilesAndAssets tests that range sources pass through.
func TestResponseCompressionFilesAndAssets(t *testing.T) {
	t.Parallel()

	css := strings.Repeat("li { color: red; }\n", 500)
	c := mustCore(t, datapages.ServerConfig{
		AssetsFS:    http.FS(fstest.MapFS{"style.css": {Data: []byte(css)}}),
		AssetsCache: &datapages.AssetsCacheConfig{},
	}, "/static/")
	c.Mux().HandleFunc("/file/", func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, httpserve.ServeFile(w, r, datapages.File{
			Body: nopSeekCloser{strings.NewReader(css)},
			Type: "text/html; charset=utf-8",
			ETag: "v1",
		}))
	})
	c.Build()

	for path, wantType := range map[string]string{
		"/static/style.css": "text/css; charset=utf-8",
		"/file/":            "text/html; charset=utf-8",
	} {
		t.Run(path, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, path, nil)
			r.Header.Set("Accept-Encoding", browserCodings)
			w := httptest.NewRecorder()
			c.ServeHTTP(w, r)
			require.Equal(t, http.StatusOK, w.Code)
			require.Empty(t, w.Header().Get("Content-Encoding"))
			require.False(t, varies(w.Header()))
			require.Equal(t, wantType, w.Header().Get("Content-Type"))
			require.Equal(t, strconv.Itoa(len(css)), w.Header().Get("Content-Length"))
			require.True(t, strings.HasPrefix(w.Header().Get("Etag"), `"`))
			require.Equal(t, css, w.Body.String())
		})
	}
}

type nopSeekCloser struct{ io.ReadSeeker }

func (nopSeekCloser) Close() error { return nil }

// TestResponseCompressionSSE tests that an SSE stream is compressed only once.
func TestResponseCompressionSSE(t *testing.T) {
	t.Parallel()

	el := `<ul id="list">` + strings.Join(listItems(8<<10), "") + `</ul>`
	c := mustCore(t, datapages.ServerConfig{}, "")
	c.Mux().HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		s := c.OpenSSE(w, r)
		defer s.Close()
		require.NoError(t, s.PatchElement(templ.Raw(el)))
	})
	c.Build()

	w := serveCompressed(t, c, http.MethodGet, browserCodings)
	require.Equal(t, "zstd", w.Header().Get("Content-Encoding"))
	require.Equal(t, "text/event-stream", w.Header().Get("Content-Type"))
	require.Contains(t, decode(t, "zstd", w.Body.Bytes()), "data: elements "+el)
}

// TestResponseCompressionBodyWritten tests that buffered bytes count as written.
func TestResponseCompressionBodyWritten(t *testing.T) {
	t.Parallel()

	var written bool
	c := mustCore(t, datapages.ServerConfig{}, "")
	c.Mux().HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "<!DOCTYPE html>")
		written = httpserve.ResponseBodyWritten(w)
	})
	c.Build()

	w := serveCompressed(t, c, http.MethodGet, browserCodings)
	require.True(t, written)
	require.Equal(t, "<!DOCTYPE html>", w.Body.String())
}

// TestResponseCompressionPanic tests panic propagation and encoder reuse.
func TestResponseCompressionPanic(t *testing.T) {
	t.Parallel()

	page := listPage(8 << 10)
	c := mustCore(t, datapages.ServerConfig{}, "")
	c.Mux().HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeParts(w, page)
		if r.URL.Query().Has("panic") {
			panic("handler failed")
		}
	})
	c.Build()

	require.PanicsWithValue(t, "handler failed", func() {
		r := httptest.NewRequest(http.MethodGet, "/page/?panic", nil)
		r.Header.Set("Accept-Encoding", "zstd")
		c.ServeHTTP(httptest.NewRecorder(), r)
	})
	w := serveCompressed(t, c, http.MethodGet, "zstd")
	require.Equal(t, strings.Join(page, ""), decode(t, "zstd", w.Body.Bytes()))
}

// TestResponseCompressionDevMode tests that dev mode disables compression.
func TestResponseCompressionDevMode(t *testing.T) {
	t.Setenv("DATAPAGES_DEV_MODE", "1")

	page := listPage(8 << 10)
	c := mustCore(t, datapages.ServerConfig{}, "")
	c.Mux().HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		writeParts(w, page)
	})
	c.Build()

	w := serveCompressed(t, c, http.MethodGet, browserCodings)
	require.Empty(t, w.Header().Get("Content-Encoding"))
	require.False(t, varies(w.Header()))
	require.Equal(t, strings.Join(page, ""), w.Body.String())
}

// TestResponseCompressionInvalidConfig tests direct ServerConfig validation.
func TestResponseCompressionInvalidConfig(t *testing.T) {
	t.Parallel()

	_, err := httpserve.NewCore(datapages.ServerConfig{
		ResponseCompression: &datapages.ResponseCompressionConfig{ZstdLevel: 99},
	}, "")
	require.EqualError(t, err,
		"response compression: zstd level (99) must be from 1 to 22")
}

type discardWriter struct {
	h http.Header
	n int
}

func (d *discardWriter) Header() http.Header { return d.h }

func (d *discardWriter) Write(b []byte) (int, error) {
	d.n += len(b)
	return len(b), nil
}

func (d *discardWriter) WriteString(s string) (int, error) {
	d.n += len(s)
	return len(s), nil
}

func (d *discardWriter) WriteHeader(int) {}

func (d *discardWriter) Flush() {}

// cardPage returns about n bytes of deterministic, varied listing cards.
func cardPage(n int) []string {
	r := rand.New(rand.NewPCG(5, 6))
	words := strings.Fields("apartment bicycle camera desk electric furniture " +
		"garden guitar heater jacket kitchen lamp laptop mirror monitor nearly new " +
		"oak office phone printer quiet repair rarely used shelf sofa spacious " +
		"table tablet used vintage wardrobe washer window wooden working condition " +
		"city center pickup only delivery possible price negotiable great deal " +
		"original box warranty included moving sale must go this week")
	sentence := func(min, more int) string {
		var b strings.Builder
		for i := range min + r.IntN(more) {
			if i > 0 {
				b.WriteByte(' ')
			}
			b.WriteString(words[r.IntN(len(words))])
		}
		return b.String()
	}
	parts := []string{`<!DOCTYPE html><html lang="en"><head><meta charset="utf-8">` +
		`<meta name="viewport" content="width=device-width, initial-scale=1">` +
		`<title>Listings</title><link rel="stylesheet" href="/static/style.css">` +
		`<script type="module" src="/static/datastar.js"></script></head>` +
		`<body class="bg-gray-50 text-gray-900"><nav class="flex items-center ` +
		`justify-between border-b border-gray-200 bg-white px-6 py-3">` +
		`<a href="/" class="text-xl font-bold">Classifieds</a>` +
		`<a href="/login/" class="text-sm text-blue-700 hover:underline">Sign in</a>` +
		`</nav><main class="mx-auto grid max-w-5xl grid-cols-1 gap-4 p-6 ` +
		`sm:grid-cols-2">`}
	for size := 0; size < n; {
		id := strconv.Itoa(10_000 + r.IntN(90_000))
		card := `<article id="post-` + id +
			`" class="rounded-lg border border-gray-200 ` +
			`bg-white p-4 shadow-sm hover:shadow-md"><a href="/post/` + id +
			`/" class="text-lg font-semibold text-blue-700 hover:underline">` +
			sentence(2, 4) + `</a><p class="mt-1 text-sm text-gray-600">` +
			sentence(8, 16) + `</p><div class="mt-2 flex items-center justify-between">` +
			`<span class="font-bold">$` + strconv.Itoa(5+r.IntN(2_000)) +
			`</span><time class="text-xs text-gray-500" datetime="2026-10-` +
			strconv.Itoa(10+r.IntN(19)) + `">` + strconv.Itoa(1+r.IntN(23)) +
			` hours ago</time></div></article>`
		parts = append(parts, card)
		size += len(card)
	}
	return append(parts, `</main></body></html>`)
}

// BenchmarkResponseCompression measures the presets with and without zstd.
// heapMiB/response is the retained heap of each of 16 flushed responses.
func BenchmarkResponseCompression(b *testing.B) {
	lowCPU := datapages.ResponseCompressionLowCPU()
	lowBandwidth := datapages.ResponseCompressionLowBandwidth()
	for _, size := range []struct {
		name string
		n    int
	}{
		{"8KB", 8_000},
		{"100KB", 100_000},
	} {
		page := cardPage(size.n)
		raw := len(strings.Join(page, ""))
		for _, tc := range []struct {
			name, accept string
			conf         *datapages.ResponseCompressionConfig
		}{
			{"uncompressed", "", nil},
			{"balanced", browserCodings, nil},
			{"low-cpu", browserCodings, &lowCPU},
			{"low-bandwidth", browserCodings, &lowBandwidth},
			{"balanced-without-zstd", "gzip, deflate, br", nil},
			{"low-cpu-without-zstd", "gzip, deflate, br", &lowCPU},
		} {
			b.Run(size.name+"/"+tc.name, func(b *testing.B) {
				var hold chan struct{}
				var wrote sync.WaitGroup
				c, err := httpserve.NewCore(datapages.ServerConfig{
					ResponseCompression: tc.conf,
				}, "")
				require.NoError(b, err)
				c.Mux().HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
					for _, p := range page {
						_, _ = io.WriteString(w, p)
					}
					if hold != nil {
						// The flush has the encoder take all the memory it uses.
						_ = http.NewResponseController(w).Flush()
						wrote.Done()
						<-hold
					}
				})
				c.Build()
				r := httptest.NewRequest(http.MethodGet, "/page/", nil)
				if tc.accept != "" {
					r.Header.Set("Accept-Encoding", tc.accept)
				}

				w := &discardWriter{h: make(http.Header)}
				c.ServeHTTP(w, r)
				sent := w.n
				b.ReportAllocs()
				for b.Loop() {
					w := &discardWriter{h: make(http.Header)}
					c.ServeHTTP(w, r)
				}
				b.ReportMetric(float64(sent)/float64(raw)*100, "%raw")

				const inFlight = 16
				hold = make(chan struct{})
				var before, after runtime.MemStats
				runtime.GC()
				runtime.GC()
				runtime.ReadMemStats(&before)
				var done sync.WaitGroup
				wrote.Add(inFlight)
				for range inFlight {
					done.Go(func() {
						c.ServeHTTP(&discardWriter{h: make(http.Header)}, r)
					})
				}
				wrote.Wait()
				runtime.GC()
				runtime.GC()
				runtime.ReadMemStats(&after)
				close(hold)
				done.Wait()
				b.ReportMetric(
					float64(after.HeapAlloc-before.HeapAlloc)/inFlight/(1<<20),
					"heapMiB/response",
				)
			})
		}
	}
}

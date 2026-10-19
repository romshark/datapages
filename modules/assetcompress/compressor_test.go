package assetcompress_test

import (
	"bytes"
	"cmp"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"
	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/internal/testlog"
	"github.com/romshark/datapages/modules/assetcompress"
	"github.com/romshark/datapages/modules/assetcompress/inmem"
)

const browserCodings = "gzip, deflate, br, zstd"

var (
	assetCSS = strings.Repeat(".card { border: 1px solid #ddd; padding: 1rem; }\n", 200)
	assetJS  = strings.Repeat("export function add(a, b) { return a + b }\n", 200)
)

func assetFiles() fstest.MapFS {
	return fstest.MapFS{
		"style.css": {Data: []byte(assetCSS)},
		"app.js":    {Data: []byte(assetJS)},
		"small.css": {Data: []byte("b { color: red }")},
		"page.html": {
			Data: []byte("<!DOCTYPE html>" + strings.Repeat("<p>static</p>", 500)),
		},
		"image.png": {
			Data: []byte("\x89PNG\r\n\x1a\n" + strings.Repeat("\x00\x01", 4000)),
		},
		"sub/nested.css": {Data: []byte(assetCSS + "/* nested */\n")},
		"sub/copy.css":   {Data: []byte(assetCSS)},
		"docs/index.html": {
			Data: []byte("<!DOCTYPE html>" + strings.Repeat("<p>docs</p>", 500)),
		},
	}
}

// Zero Config creates brotli and gzip copies for 5 unique eligible contents.
const eligibleCopies = 10

// countingStore counts puts and injects errors.
type countingStore struct {
	assetcompress.Store
	puts    atomic.Int64
	openErr error
	putErr  error
}

func newCountingStore() *countingStore { return &countingStore{Store: inmem.New()} }

func (s *countingStore) Open(key string) (io.ReadSeekCloser, error) {
	if s.openErr != nil {
		return nil, s.openErr
	}
	return s.Store.Open(key)
}

func (s *countingStore) Put(key string, r io.Reader) error {
	s.puts.Add(1)
	if s.putErr != nil {
		return s.putErr
	}
	return s.Store.Put(key, r)
}

func newCompressor(t testing.TB, conf assetcompress.Config) *assetcompress.Compressor {
	t.Helper()
	c, err := assetcompress.New(conf)
	require.NoError(t, err)
	return c
}

// newHandler matches the server's middleware order. A nil logger discards logs.
func newHandler(
	c *assetcompress.Compressor, fsys http.FileSystem, logger *slog.Logger,
) http.Handler {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return c.Handler(fsys, http.FileServer(fsys), logger)
}

// withETag sets tag before next runs, as the asset cache of the server does.
func withETag(tag string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Etag", tag)
		next.ServeHTTP(w, r)
	})
}

func get(
	t testing.TB, h http.Handler, method, path, accept string, header http.Header,
) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, path, nil)
	r.Header.Set("Accept-Encoding", cmp.Or(accept, "identity"))
	for k, v := range header {
		r.Header[k] = v
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
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

// TestNew covers valid and invalid configurations.
func TestNew(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		conf    assetcompress.Config
		wantErr string
	}{
		"zero":  {},
		"store": {conf: assetcompress.Config{Store: inmem.New()}},
		"zstd, else gzip": {conf: assetcompress.Config{
			Codings: []datapages.Coding{datapages.CodingZstd, datapages.CodingGzip},
		}},
		"bounds": {conf: assetcompress.Config{
			ZstdLevel: 22, BrotliQuality: 11, GzipLevel: 9,
		}},
		"lower bounds": {conf: assetcompress.Config{
			MinSize: 1, ZstdLevel: 1, BrotliQuality: 1, GzipLevel: 1,
		}},
		"unknown coding": {
			conf:    assetcompress.Config{Codings: []datapages.Coding{"deflate"}},
			wantErr: `assetcompress: unknown coding "deflate"`,
		},
		"coding listed 2 times": {
			conf: assetcompress.Config{
				Codings: []datapages.Coding{
					datapages.CodingBrotli, datapages.CodingBrotli,
				},
			},
			wantErr: `assetcompress: coding "br" listed 2 times`,
		},
		"min size negative": {
			conf:    assetcompress.Config{MinSize: -1},
			wantErr: "assetcompress: min size (-1) must not be negative",
		},
		"zstd level too high": {
			conf:    assetcompress.Config{ZstdLevel: 23},
			wantErr: "assetcompress: zstd level (23) must be from 1 to 22",
		},
		"brotli quality too high": {
			conf:    assetcompress.Config{BrotliQuality: 12},
			wantErr: "assetcompress: brotli quality (12) must be from 1 to 11",
		},
		"gzip level negative": {
			conf:    assetcompress.Config{GzipLevel: -1},
			wantErr: "assetcompress: gzip level (-1) must be from 1 to 9",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c, err := assetcompress.New(tc.conf)
			if tc.wantErr != "" {
				require.EqualError(t, err, tc.wantErr)
				require.Nil(t, c)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, c)
		})
	}
}

// TestHandler covers eligibility, negotiation and headers.
func TestHandler(t *testing.T) {
	t.Parallel()

	h := newHandler(newCompressor(t, assetcompress.Config{}), http.FS(assetFiles()), nil)
	for name, tc := range map[string]struct {
		method, path, accept string
		wantStatus           int
		wantCoding           string
		wantVary             bool
		wantType             string
		wantBody             string // Decoded; "" for none.
	}{
		"brotli": {
			path: "/style.css", accept: browserCodings,
			wantCoding: "br", wantVary: true,
			wantType: "text/css; charset=utf-8", wantBody: assetCSS,
		},
		"gzip only": {
			path: "/style.css", accept: "gzip",
			wantCoding: "gzip", wantVary: true, wantBody: assetCSS,
		},
		"zstd only": {
			path: "/style.css", accept: "zstd",
			wantVary: true, wantBody: assetCSS,
		},
		"no coding": {
			path: "/style.css", wantVary: true, wantBody: assetCSS,
		},
		"javascript": {
			path: "/app.js", accept: browserCodings,
			wantCoding: "br", wantVary: true,
			wantType: "text/javascript; charset=utf-8", wantBody: assetJS,
		},
		"subdirectory": {
			path: "/sub/nested.css", accept: browserCodings,
			wantCoding: "br", wantVary: true, wantBody: assetCSS + "/* nested */\n",
		},
		"below 256 bytes": {
			path: "/small.css", accept: browserCodings,
			wantBody: "b { color: red }",
		},
		"HTML": {
			path: "/page.html", accept: browserCodings,
			wantCoding: "br", wantVary: true, wantType: "text/html; charset=utf-8",
			wantBody: "<!DOCTYPE html>" + strings.Repeat("<p>static</p>", 500),
		},
		"directory index": {
			path: "/docs/", accept: browserCodings,
			wantCoding: "br", wantVary: true, wantType: "text/html; charset=utf-8",
			wantBody: "<!DOCTYPE html>" + strings.Repeat("<p>docs</p>", 500),
		},
		"index.html": {
			path: "/docs/index.html", accept: browserCodings,
			wantStatus: http.StatusMovedPermanently,
		},
		"image": {path: "/image.png", accept: browserCodings},
		"missing": {
			path: "/nope.css", accept: browserCodings,
			wantStatus: http.StatusNotFound,
		},
		"directory": {path: "/sub/", accept: browserCodings},
		"HEAD": {
			method: http.MethodHead, path: "/style.css", accept: browserCodings,
			wantCoding: "br", wantVary: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			w := get(t, h, cmp.Or(tc.method, http.MethodGet), tc.path, tc.accept, nil)
			require.Equal(t, cmp.Or(tc.wantStatus, http.StatusOK), w.Code)
			require.Equal(t, tc.wantCoding, w.Header().Get("Content-Encoding"))
			require.Equal(t, tc.wantVary, varies(w.Header()), "Vary: %q",
				w.Header().Values("Vary"))
			if tc.wantType != "" {
				require.Equal(t, tc.wantType, w.Header().Get("Content-Type"))
			}
			if tc.wantBody != "" {
				require.Equal(t, tc.wantBody, decode(t, tc.wantCoding, w.Body.Bytes()))
				require.Equal(t,
					strconv.Itoa(w.Body.Len()),
					w.Header().Get("Content-Length"))
			}
		})
	}
}

// TestHandlerHEAD verifies that HEAD describes the selected copy.
func TestHandlerHEAD(t *testing.T) {
	t.Parallel()

	h := newHandler(newCompressor(t, assetcompress.Config{}), http.FS(assetFiles()), nil)
	full := get(t, h, http.MethodGet, "/style.css", "br", nil)
	head := get(t, h, http.MethodHead, "/style.css", "br", nil)
	require.Equal(t, "br", head.Header().Get("Content-Encoding"))
	require.Equal(t, strconv.Itoa(full.Body.Len()), head.Header().Get("Content-Length"))
	require.Empty(t, head.Body.String())
}

// TestHandlerRange verifies a range of a copy.
func TestHandlerRange(t *testing.T) {
	t.Parallel()

	h := newHandler(newCompressor(t, assetcompress.Config{}), http.FS(assetFiles()), nil)
	full := get(t, h, http.MethodGet, "/style.css", "br", nil).Body.Bytes()
	w := get(t,
		h, http.MethodGet, "/style.css", "br", http.Header{"Range": {"bytes=0-9"}})
	require.Equal(t, http.StatusPartialContent, w.Code)
	require.Equal(t, "br", w.Header().Get("Content-Encoding"))
	require.Equal(t,
		fmt.Sprintf("bytes 0-9/%d", len(full)),
		w.Header().Get("Content-Range"))
	require.Equal(t, full[:10], w.Body.Bytes())
}

// TestHandlerMultipartRange verifies that multiple ranges use the source file.
func TestHandlerMultipartRange(t *testing.T) {
	t.Parallel()

	h := newHandler(newCompressor(t, assetcompress.Config{}), http.FS(assetFiles()), nil)
	w := get(t, h, http.MethodGet, "/style.css", "br",
		http.Header{"Range": {"bytes=0-9,20-29"}})
	require.Equal(t, http.StatusPartialContent, w.Code)
	require.Empty(t, w.Header().Get("Content-Encoding"))
	require.True(t, strings.HasPrefix(w.Header().Get("Content-Type"),
		"multipart/byteranges"))
	require.Contains(t, w.Body.String(), assetCSS[:10])
}

// TestHandlerMinSize covers the default threshold and MinSize 1.
func TestHandlerMinSize(t *testing.T) {
	t.Parallel()

	for minSize, want := range map[int]string{0: "", 1: "br"} {
		h := newHandler(newCompressor(t, assetcompress.Config{MinSize: minSize}),
			http.FS(assetFiles()), nil)
		w := get(t, h, http.MethodGet, "/small.css", "br", nil)
		require.Equal(t, want, w.Header().Get("Content-Encoding"), "min size %d", minSize)
		require.Equal(t, "b { color: red }", decode(t, want, w.Body.Bytes()))
	}
}

// TestHandlerFontsAndIcons tests that TrueType and OpenType fonts, icons and
// bitmaps get a copy and that WOFF2 doesn't. The files have no extension,
// which makes their types the sniffed ones on every platform.
func TestHandlerFontsAndIcons(t *testing.T) {
	t.Parallel()

	tables := strings.Repeat("glyf loca hmtx cmap ", 50)
	files := fstest.MapFS{
		"truetype":   {Data: []byte("\x00\x01\x00\x00" + tables)},
		"opentype":   {Data: []byte("OTTO" + tables)},
		"collection": {Data: []byte("ttcf" + tables)},
		"icon":       {Data: []byte("\x00\x00\x01\x00" + tables)},
		"bitmap":     {Data: []byte("BM" + tables)},
		"woff2":      {Data: []byte("wOF2" + tables)},
	}
	h := newHandler(newCompressor(t, assetcompress.Config{}), http.FS(files), nil)
	for name, tc := range map[string]struct {
		wantType, wantCoding string
	}{
		"truetype":   {wantType: "font/ttf", wantCoding: "br"},
		"opentype":   {wantType: "font/otf", wantCoding: "br"},
		"collection": {wantType: "font/collection", wantCoding: "br"},
		"icon":       {wantType: "image/x-icon", wantCoding: "br"},
		"bitmap":     {wantType: "image/bmp", wantCoding: "br"},
		"woff2":      {wantType: "font/woff2"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			w := get(t, h, http.MethodGet, "/"+name, browserCodings, nil)
			require.Equal(t, http.StatusOK, w.Code)
			require.Equal(t, tc.wantType, w.Header().Get("Content-Type"))
			require.Equal(t, tc.wantCoding, w.Header().Get("Content-Encoding"))
			require.Equal(t, string(files[name].Data),
				decode(t, tc.wantCoding, w.Body.Bytes()))
		})
	}
}

// TestHandlerNoTransform tests that a response marked no-transform before the
// handler runs passes to next without a coding and without Vary.
func TestHandlerNoTransform(t *testing.T) {
	t.Parallel()

	inner := newHandler(newCompressor(t, assetcompress.Config{}),
		http.FS(assetFiles()), nil)
	for name, tc := range map[string]struct {
		cacheControl string
		wantCoding   string
	}{
		"other directives": {cacheControl: "public, max-age=60", wantCoding: "br"},
		"no-transform":     {cacheControl: "public, no-transform"},
		"mixed case":       {cacheControl: "No-Transform"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Cache-Control", tc.cacheControl)
				inner.ServeHTTP(w, r)
			})
			w := get(t, h, http.MethodGet, "/style.css", "br", nil)
			require.Equal(t, http.StatusOK, w.Code)
			require.Equal(t, tc.wantCoding, w.Header().Get("Content-Encoding"))
			require.Equal(t, tc.wantCoding != "", varies(w.Header()))
			require.Equal(t, tc.cacheControl, w.Header().Get("Cache-Control"))
			require.Equal(t, assetCSS, decode(t, tc.wantCoding, w.Body.Bytes()))
		})
	}
}

// TestHandlerZstdLevels verifies that equivalent levels share copies and ETags.
func TestHandlerZstdLevels(t *testing.T) {
	t.Parallel()

	store := newCountingStore()
	for _, level := range []int{19, 22} {
		h := withETag(`"base"`, newHandler(newCompressor(t, assetcompress.Config{
			Store:     store,
			Codings:   []datapages.Coding{datapages.CodingZstd},
			ZstdLevel: level,
		}), http.FS(assetFiles()), nil))
		w := get(t, h, http.MethodGet, "/style.css", "zstd", nil)
		require.Equal(t, `"base.zstd10"`, w.Header().Get("Etag"), "level %d", level)
	}
	require.Equal(t, int64(1), store.puts.Load(), "copies of levels 19 and 22")
}

// TestHandlerETag covers representation tags and conditional requests.
func TestHandlerETag(t *testing.T) {
	t.Parallel()

	h := withETag(`"base"`,
		newHandler(newCompressor(t, assetcompress.Config{}), http.FS(assetFiles()), nil))
	for coding, want := range map[string]string{
		"":     `"base"`,
		"br":   `"base.br6"`,
		"gzip": `"base.gzip9"`,
	} {
		w := get(t, h, http.MethodGet, "/style.css", coding, nil)
		require.Equal(t, want, w.Header().Get("Etag"), coding)

		w = get(t, h, http.MethodGet, "/style.css", coding,
			http.Header{"If-None-Match": {want}})
		require.Equal(t, http.StatusNotModified, w.Code, coding)
	}
	w := get(t, h, http.MethodGet, "/style.css", "br",
		http.Header{"If-None-Match": {`"base"`}})
	require.Equal(t, http.StatusOK, w.Code,
		"the tag of the uncompressed file matched the copy")
}

// TestHandlerPreconditionFailed verifies that a 412 omits copy headers.
func TestHandlerPreconditionFailed(t *testing.T) {
	t.Parallel()

	h := withETag(`"base"`,
		newHandler(newCompressor(t, assetcompress.Config{}), http.FS(assetFiles()), nil))
	w := get(t, h, http.MethodGet, "/style.css", "br",
		http.Header{"If-Match": {`"other"`}})
	require.Equal(t, http.StatusPreconditionFailed, w.Code)
	require.Empty(t, w.Header().Get("Content-Encoding"))
	require.Empty(t, w.Header().Get("Content-Length"))
	require.Empty(t, w.Body.String())
}

// TestHandlerLazy verifies 1 stored copy per requested coding.
func TestHandlerLazy(t *testing.T) {
	t.Parallel()

	store := newCountingStore()
	h := newHandler(newCompressor(t, assetcompress.Config{Store: store}),
		http.FS(assetFiles()), nil)
	for range 3 {
		w := get(t, h, http.MethodGet, "/style.css", "br", nil)
		require.Equal(t, assetCSS, decode(t, "br", w.Body.Bytes()))
	}
	require.Equal(t, int64(1), store.puts.Load())

	get(t, h, http.MethodGet, "/style.css", "gzip", nil)
	require.Equal(t, int64(2), store.puts.Load(), "the copy of the other coding")
}

// TestHandlerConcurrent verifies that concurrent misses share 1 put.
func TestHandlerConcurrent(t *testing.T) {
	t.Parallel()

	store := newCountingStore()
	h := newHandler(newCompressor(t, assetcompress.Config{Store: store}),
		http.FS(assetFiles()), nil)
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			w := get(t, h, http.MethodGet, "/style.css", "br", nil)
			require.Equal(t, "br", w.Header().Get("Content-Encoding"))
			require.Equal(t, assetCSS, decode(t, "br", w.Body.Bytes()))
		})
	}
	wg.Wait()
	require.Equal(t, int64(1), store.puts.Load())
}

// TestHandlerSharedCompressor covers different content at 1 path on 2 servers.
func TestHandlerSharedCompressor(t *testing.T) {
	t.Parallel()

	c := newCompressor(t, assetcompress.Config{})
	other := strings.ReplaceAll(assetCSS, "#ddd", "#eee")
	a := newHandler(c, http.FS(fstest.MapFS{"style.css": {Data: []byte(assetCSS)}}), nil)
	b := newHandler(c, http.FS(fstest.MapFS{"style.css": {Data: []byte(other)}}), nil)

	w := get(t, a, http.MethodGet, "/style.css", "br", nil)
	require.Equal(t, assetCSS, decode(t, "br", w.Body.Bytes()))
	w = get(t, b, http.MethodGet, "/style.css", "br", nil)
	require.Equal(t, other, decode(t, "br", w.Body.Bytes()))
}

// TestHandlerChangedFile verifies that compressed responses keep cached content
// until a new handler is built. Uncompressed responses read the changed file.
func TestHandlerChangedFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	file := filepath.Join(dir, "style.css")
	require.NoError(t, os.WriteFile(file, []byte(assetCSS), 0o600))
	c := newCompressor(t, assetcompress.Config{})
	h := newHandler(c, http.Dir(dir), nil)
	w := get(t, h, http.MethodGet, "/style.css", "br", nil)
	require.Equal(t, assetCSS, decode(t, "br", w.Body.Bytes()))

	changed := strings.ReplaceAll(assetCSS, "#ddd", "#eee")
	require.NoError(t, os.WriteFile(file, []byte(changed), 0o600))
	w = get(t, h, http.MethodGet, "/style.css", "br", nil)
	require.Equal(t, assetCSS, decode(t, "br", w.Body.Bytes()))
	w = get(t, h, http.MethodGet, "/style.css", "", nil)
	require.Equal(t, changed, w.Body.String())

	w = get(t, newHandler(c, http.Dir(dir), nil), http.MethodGet, "/style.css", "br", nil)
	require.Equal(t, changed, decode(t, "br", w.Body.Bytes()))
}

type changingFS struct {
	opens atomic.Int64
}

func (f *changingFS) Open(name string) (http.File, error) {
	files := assetFiles()
	if name == "/style.css" && f.opens.Add(1) > 1 {
		files["style.css"] = &fstest.MapFile{
			Data: []byte(strings.ReplaceAll(assetCSS, "1rem", "2rem")),
		}
	}
	return http.FS(files).Open(name)
}

// TestHandlerFileChangesDuringCompression rejects a changed source and retries.
func TestHandlerFileChangesDuringCompression(t *testing.T) {
	t.Parallel()

	var log testlog.Buffer
	h := newHandler(
		newCompressor(t, assetcompress.Config{}), &changingFS{}, log.Logger(),
	)
	w := get(t, h, http.MethodGet, "/style.css", "br", nil)
	require.Empty(t, w.Header().Get("Content-Encoding"))
	log.RequireMatch(t,
		`msg="compressing an asset".*file changed while it was compressed`)

	w = get(t, h, http.MethodGet, "/style.css", "br", nil)
	require.Equal(t, "br", w.Header().Get("Content-Encoding"))
	require.Equal(t, strings.ReplaceAll(assetCSS, "1rem", "2rem"),
		decode(t, "br", w.Body.Bytes()))
}

// TestHandlerCodings verifies a configured zstd preference.
func TestHandlerCodings(t *testing.T) {
	t.Parallel()

	h := newHandler(newCompressor(t, assetcompress.Config{
		Codings: []datapages.Coding{datapages.CodingZstd, datapages.CodingBrotli},
	}), http.FS(assetFiles()), nil)
	w := get(t, h, http.MethodGet, "/style.css", browserCodings, nil)
	require.Equal(t, "zstd", w.Header().Get("Content-Encoding"))
	require.Equal(t, assetCSS, decode(t, "zstd", w.Body.Bytes()))

	w = get(t, h, http.MethodGet, "/style.css", "gzip", nil)
	require.Empty(t, w.Header().Get("Content-Encoding"), "gzip is not configured")
}

type openCountingFS struct {
	http.FileSystem
	opens atomic.Int64
}

func (f *openCountingFS) Open(name string) (http.File, error) {
	f.opens.Add(1)
	return f.FileSystem.Open(name)
}

// TestHandlerSkipsUncompressibleTypes verifies that rejected extensions bypass opens.
func TestHandlerSkipsUncompressibleTypes(t *testing.T) {
	t.Parallel()

	fsys := &openCountingFS{FileSystem: http.FS(assetFiles())}
	h := newHandler(newCompressor(t, assetcompress.Config{}), fsys, nil)
	w := get(t, h, http.MethodGet, "/image.png", browserCodings, nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.Empty(t, w.Header().Get("Content-Encoding"))
	require.Equal(t, int64(1), fsys.opens.Load(), "only the file server opens it")
}

// TestStoreFailure covers fallback, logging and failed precompression.
func TestStoreFailure(t *testing.T) {
	t.Parallel()

	errStore := errors.New("store failed")
	for name, store := range map[string]*countingStore{
		"open": {Store: inmem.New(), openErr: errStore},
		"put":  {Store: inmem.New(), putErr: errStore},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c := newCompressor(t, assetcompress.Config{Store: store})
			require.EqualError(t, c.Precompress(context.Background(), assetFiles()),
				"precompressing app.js: store failed")

			var log testlog.Buffer
			w := get(t, newHandler(c, http.FS(assetFiles()), log.Logger()),
				http.MethodGet, "/style.css", "br", nil)
			require.Equal(t, http.StatusOK, w.Code)
			require.Empty(t, w.Header().Get("Content-Encoding"))
			require.True(t, varies(w.Header()))
			require.Equal(t, assetCSS, w.Body.String())
			log.RequireMatch(t, `level=WARN msg="compressing an asset" `+
				`path=/style.css coding=br err="store failed"`)
		})
	}
}

type panickingStore struct {
	assetcompress.Store
	panicked atomic.Bool
}

func (s *panickingStore) Put(key string, r io.Reader) error {
	if s.panicked.CompareAndSwap(false, true) {
		panic("put failed")
	}
	return s.Store.Put(key, r)
}

// panickingFS panics during encoding, which runs on its own goroutine.
type panickingFS struct {
	http.FileSystem
	opens atomic.Int64
}

func (fsys *panickingFS) Open(name string) (http.File, error) {
	f, err := fsys.FileSystem.Open(name)
	if err != nil || name != "/style.css" || fsys.opens.Add(1) != 2 {
		return f, err
	}
	return panickingFile{f}, nil
}

type panickingFile struct{ http.File }

func (panickingFile) Read([]byte) (int, error) { panic("read failed") }

// TestHandlerPanic covers recovery, fallback and a successful retry.
func TestHandlerPanic(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		store assetcompress.Store // Nil for the default.
		fsys  http.FileSystem
		value string
	}{
		"store": {
			store: &panickingStore{Store: inmem.New()},
			fsys:  http.FS(assetFiles()),
			value: "put failed",
		},
		"file system": {
			fsys:  &panickingFS{FileSystem: http.FS(assetFiles())},
			value: "read failed",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var log testlog.Buffer
			h := newHandler(
				newCompressor(t,
					assetcompress.Config{Store: tc.store}), tc.fsys, log.Logger(),
			)
			w := get(t, h, http.MethodGet, "/style.css", "br", nil)
			require.Empty(t, w.Header().Get("Content-Encoding"))
			require.Equal(t, assetCSS, w.Body.String())
			log.RequireMatch(t,
				`level=ERROR msg="recovered panic while compressing an asset" `+
					`path=/style.css coding=br panic="`+tc.value+`" stack="goroutine `)

			// A compressor that the panic left locked never answers.
			next := make(chan *httptest.ResponseRecorder, 1)
			go func() { next <- get(t, h, http.MethodGet, "/style.css", "br", nil) }()
			select {
			case w := <-next:
				require.Equal(t, "br", w.Header().Get("Content-Encoding"))
				require.Equal(t, assetCSS, decode(t, "br", w.Body.Bytes()))
			case <-time.After(5 * time.Second):
				t.Fatal("the request after the panic got no answer")
			}
		})
	}
}

// TestDevMode verifies that dev mode disables compression.
func TestDevMode(t *testing.T) {
	t.Setenv(datapages.EnvVarDevMode, "1")
	store := newCountingStore()
	c := newCompressor(t, assetcompress.Config{Store: store})
	require.NoError(t, c.Precompress(context.Background(), assetFiles()))

	w := get(t, newHandler(c, http.FS(assetFiles()), nil),
		http.MethodGet, "/style.css", browserCodings, nil)
	require.Empty(t, w.Header().Get("Content-Encoding"))
	require.False(t, varies(w.Header()))
	require.Equal(t, assetCSS, w.Body.String())
	require.Zero(t, store.puts.Load())
}

// TestPrecompress verifies 1 copy per content and coding with no lazy puts.
func TestPrecompress(t *testing.T) {
	t.Parallel()

	store := newCountingStore()
	c := newCompressor(t, assetcompress.Config{Store: store})
	require.NoError(t, c.Precompress(context.Background(), assetFiles()))
	require.Equal(t, int64(eligibleCopies), store.puts.Load())

	require.NoError(t, c.Precompress(context.Background(), assetFiles()))
	h := newHandler(c, http.FS(assetFiles()), nil)
	for _, coding := range []string{"br", "gzip"} {
		w := get(t, h, http.MethodGet, "/sub/nested.css", coding, nil)
		require.Equal(t, coding, w.Header().Get("Content-Encoding"))
	}
	require.Equal(t, int64(eligibleCopies), store.puts.Load())
}

// TestPrecompressSubdirectory covers serving a subdirectory of an embed.FS.
func TestPrecompressSubdirectory(t *testing.T) {
	t.Parallel()

	embedded := fstest.MapFS{}
	for name, f := range assetFiles() {
		embedded["static/"+name] = f
	}
	store := newCountingStore()
	c := newCompressor(t, assetcompress.Config{Store: store})
	require.NoError(t, c.Precompress(context.Background(), embedded))
	require.Equal(t, int64(eligibleCopies), store.puts.Load())

	served, err := fs.Sub(embedded, "static")
	require.NoError(t, err)
	h := newHandler(c, http.FS(served), nil)
	for _, p := range []string{
		"/style.css", "/app.js", "/page.html",
		"/sub/nested.css", "/sub/copy.css", "/docs/",
	} {
		w := get(t, h, http.MethodGet, p, "br", nil)
		require.Equal(t, "br", w.Header().Get("Content-Encoding"), p)
	}
	require.Equal(t, int64(eligibleCopies), store.puts.Load(),
		"a request created a copy")
}

// TestPrecompressCanceled stops before the first file.
func TestPrecompressCanceled(t *testing.T) {
	t.Parallel()

	store := newCountingStore()
	c := newCompressor(t, assetcompress.Config{Store: store})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, c.Precompress(ctx, assetFiles()), context.Canceled)
	require.Zero(t, store.puts.Load())
}

var exampleAssets = []string{
	"../../example/docs/app/static/editor.js",
	"../../example/offline-cache/app/static/basecoat.css",
	"../../example/classifieds/app/static/ds.min.js",
}

// BenchmarkPrecompress measures the first copy of example assets.
func BenchmarkPrecompress(b *testing.B) {
	for _, file := range exampleAssets {
		data, err := os.ReadFile(file)
		if errors.Is(err, fs.ErrNotExist) {
			b.Skip("the examples are not checked out")
		}
		require.NoError(b, err)
		base := filepath.Base(file)
		fsys := fstest.MapFS{base: {Data: data}}
		for _, tc := range []struct {
			name string
			conf assetcompress.Config
		}{
			{"br6", assetcompress.Config{
				Codings: []datapages.Coding{datapages.CodingBrotli},
			}},
			{"br11", assetcompress.Config{
				Codings: []datapages.Coding{datapages.CodingBrotli}, BrotliQuality: 11,
			}},
			{"zstd10", assetcompress.Config{
				Codings: []datapages.Coding{datapages.CodingZstd},
			}},
			{"gzip9", assetcompress.Config{
				Codings: []datapages.Coding{datapages.CodingGzip},
			}},
		} {
			b.Run(base+"/"+tc.name, func(b *testing.B) {
				var copySize int
				var allocs uint64
				var ops int
				for b.Loop() {
					b.StopTimer()
					conf := tc.conf
					conf.Store = inmem.New()
					c := newCompressor(b, conf)
					var before, after runtime.MemStats
					runtime.ReadMemStats(&before)
					b.StartTimer()
					require.NoError(b, c.Precompress(context.Background(), fsys))
					b.StopTimer()
					runtime.ReadMemStats(&after)
					allocs += after.TotalAlloc - before.TotalAlloc
					ops++
					w := get(b, newHandler(c, http.FS(fsys), nil),
						http.MethodGet, "/"+base, "br, gzip, zstd", nil)
					copySize = w.Body.Len()
					b.StartTimer()
				}
				b.ReportMetric(float64(copySize)/float64(len(data))*100, "%raw")
				b.ReportMetric(float64(allocs)/float64(ops)/(1<<20), "allocMiB/op")
			})
		}
	}
}

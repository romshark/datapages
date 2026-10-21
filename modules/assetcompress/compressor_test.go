package assetcompress_test

import (
	"bytes"
	"cmp"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	"testing/synctest"
	"time"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"
	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/internal/testlog"
	"github.com/romshark/datapages/modules/assetcompress"
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

func newCountingStore() *countingStore { return &countingStore{Store: new(memStore)} }

// memStore keeps copies in a map and never fails.
type memStore struct{ copies sync.Map }

func (s *memStore) Open(key string) (io.ReadSeekCloser, error) {
	if b, ok := s.copies.Load(key); ok {
		return nopCloser{bytes.NewReader(b.([]byte))}, nil
	}
	return nil, &fs.PathError{Op: "open", Path: key, Err: fs.ErrNotExist}
}

func (s *memStore) Put(key string, r io.Reader) error {
	b, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	s.copies.LoadOrStore(key, b)
	return nil
}

type nopCloser struct{ *bytes.Reader }

func (nopCloser) Close() error { return nil }

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
	return c.Handler(fsys, nil, http.FileServer(fsys), logger)
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
		"store": {conf: assetcompress.Config{Store: new(memStore)}},
		"zstd, else gzip": {conf: assetcompress.Config{
			Codings: []datapages.Coding{datapages.CodingZstd, datapages.CodingGzip},
		}},
		"bounds": {conf: assetcompress.Config{
			Levels: assetcompress.Levels{ZstdLevel: 22, BrotliQuality: 11, GzipLevel: 9},
		}},
		"lower bounds": {conf: assetcompress.Config{
			MinSize: 1,
			Levels:  assetcompress.Levels{ZstdLevel: 1, BrotliQuality: 1, GzipLevel: 1},
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
			conf:    assetcompress.Config{Levels: assetcompress.Levels{ZstdLevel: 23}},
			wantErr: "assetcompress: zstd level (23) must be from 1 to 22",
		},
		"brotli quality too high": {
			conf: assetcompress.Config{
				Levels: assetcompress.Levels{BrotliQuality: 12},
			},
			wantErr: "assetcompress: brotli quality (12) must be from 1 to 11",
		},
		"gzip level negative": {
			conf:    assetcompress.Config{Levels: assetcompress.Levels{GzipLevel: -1}},
			wantErr: "assetcompress: gzip level (-1) must be from 1 to 9",
		},
		"initial levels": {conf: assetcompress.Config{
			Levels: assetcompress.Levels{ZstdLevel: 19, BrotliQuality: 11},
			InitialLevels: assetcompress.Levels{
				ZstdLevel: 3, BrotliQuality: 6, GzipLevel: 9,
			},
		}},
		"initial zstd level negative": {
			conf: assetcompress.Config{InitialLevels: assetcompress.Levels{
				ZstdLevel: -1,
			}},
			wantErr: "assetcompress: initial zstd level (-1) must be from 1 to 22",
		},
		"initial brotli quality too high": {
			conf: assetcompress.Config{InitialLevels: assetcompress.Levels{
				BrotliQuality: 12,
			}},
			wantErr: "assetcompress: initial brotli quality (12) must be from 1 to 11",
		},
		"initial gzip level too high": {
			conf: assetcompress.Config{InitialLevels: assetcompress.Levels{
				GzipLevel: 10,
			}},
			wantErr: "assetcompress: initial gzip level (10) must be from 1 to 9",
		},
		"initial zstd level above default": {
			conf: assetcompress.Config{InitialLevels: assetcompress.Levels{
				ZstdLevel: 19,
			}},
			wantErr: "assetcompress: " +
				"initial zstd level (19) must not exceed zstd level (10)",
		},
		"initial brotli quality above": {
			conf: assetcompress.Config{
				Levels:        assetcompress.Levels{BrotliQuality: 5},
				InitialLevels: assetcompress.Levels{BrotliQuality: 6},
			},
			wantErr: "assetcompress: " +
				"initial brotli quality (6) must not exceed brotli quality (5)",
		},
		"initial gzip level above": {
			conf: assetcompress.Config{
				Levels:        assetcompress.Levels{GzipLevel: 1},
				InitialLevels: assetcompress.Levels{GzipLevel: 2},
			},
			wantErr: "assetcompress: " +
				"initial gzip level (2) must not exceed gzip level (1)",
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

// TestHandlerMultipartRange tests that multiple ranges
// use the source file and carry Vary.
func TestHandlerMultipartRange(t *testing.T) {
	t.Parallel()

	h := newHandler(newCompressor(t, assetcompress.Config{}), http.FS(assetFiles()), nil)
	w := get(t, h, http.MethodGet, "/style.css", "br",
		http.Header{"Range": {"bytes=0-9,20-29"}})
	require.Equal(t, http.StatusPartialContent, w.Code)
	require.Empty(t, w.Header().Get("Content-Encoding"))
	require.True(t, strings.HasPrefix(w.Header().Get("Content-Type"),
		"multipart/byteranges"))
	require.True(t, varies(w.Header()))
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
		c := newCompressor(t, assetcompress.Config{
			Store:   store,
			Codings: []datapages.Coding{datapages.CodingZstd},
			Levels:  assetcompress.Levels{ZstdLevel: level},
		})
		h := withETag(`"base"`, newHandler(c, http.FS(assetFiles()), nil))
		w := get(t, h, http.MethodGet, "/style.css", "zstd", nil)
		require.Equal(t, `"base.zstd10"`, w.Header().Get("Etag"), "level %d", level)
		assetcompress.WaitBackground(c)
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

// readCountingFS counts the bytes read from the files it opens.
type readCountingFS struct {
	http.FileSystem
	read atomic.Int64
}

func (fsys *readCountingFS) Open(name string) (http.File, error) {
	f, err := fsys.FileSystem.Open(name)
	if err != nil {
		return nil, err
	}
	return readCountingFile{File: f, read: &fsys.read}, nil
}

type readCountingFile struct {
	http.File
	read *atomic.Int64
}

func (f readCountingFile) Read(p []byte) (int, error) {
	n, err := f.File.Read(p)
	f.read.Add(int64(n))
	return n, err
}

// styleSum returns the SHA-256 of style.css, as the server does with an ETag.
func styleSum(name string) ([sha256.Size]byte, bool) {
	return sha256.Sum256([]byte(assetCSS)), name == "style.css"
}

// TestHandlerSum tests that the handler stores a copy under the SHA-256 that
// sum returns and reads the file only to compress it.
func TestHandlerSum(t *testing.T) {
	t.Parallel()

	fsys := &readCountingFS{FileSystem: http.FS(assetFiles())}
	store := newCountingStore()
	c := newCompressor(t, assetcompress.Config{Store: store})
	h := c.Handler(fsys, styleSum, http.FileServer(fsys), slog.New(slog.DiscardHandler))
	w := get(t, h, http.MethodGet, "/style.css", "br", nil)
	require.Equal(t, "br", w.Header().Get("Content-Encoding"))
	require.Equal(t, assetCSS, decode(t, "br", w.Body.Bytes()))
	require.Equal(t, int64(len(assetCSS)), fsys.read.Load(), "bytes read")

	assetcompress.WaitBackground(c)
	want, _ := styleSum("style.css")
	copied, err := store.Open(hex.EncodeToString(want[:]) + ".br6")
	require.NoError(t, err, "the store has no copy under the SHA-256 of sum")
	require.NoError(t, copied.Close())
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
	c := newCompressor(t, assetcompress.Config{Store: store})
	h := newHandler(c, http.FS(assetFiles()), nil)
	for range 3 {
		w := get(t, h, http.MethodGet, "/style.css", "br", nil)
		require.Equal(t, assetCSS, decode(t, "br", w.Body.Bytes()))
	}
	assetcompress.WaitBackground(c)
	require.Equal(t, int64(1), store.puts.Load())

	get(t, h, http.MethodGet, "/style.css", "gzip", nil)
	assetcompress.WaitBackground(c)
	require.Equal(t, int64(2), store.puts.Load(), "the copy of the other coding")
}

// TestHandlerConcurrent tests that requests share 1 compression and 1 write.
func TestHandlerConcurrent(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		fsys := newStallingFS(2)
		defer fsys.unstall()
		store := &lateFailingStore{Store: new(memStore)}
		c := newCompressor(t, assetcompress.Config{Store: store})
		h := newHandler(c, fsys, nil)
		responses := make(chan *httptest.ResponseRecorder, 4)
		request := func() {
			responses <- get(t, h, http.MethodGet, "/style.css", "br", nil)
		}
		go request()
		<-fsys.stalled
		for range 3 {
			go request()
		}
		synctest.Wait()
		require.Equal(t, int64(2), fsys.opens.Load(), "opens of style.css")
		fsys.unstall()
		for range 4 {
			w := <-responses
			require.Equal(t, "br", w.Header().Get("Content-Encoding"))
			require.Equal(t, assetCSS, decode(t, "br", w.Body.Bytes()))
		}

		synctest.Wait()
		require.Equal(t, int64(1), store.puts.Load(), "the write waits to retry")
		w := get(t, h, http.MethodGet, "/style.css", "br", nil)
		require.Equal(t, assetCSS, decode(t, "br", w.Body.Bytes()))
		assetcompress.WaitBackground(c)
		require.Equal(t, int64(2), fsys.opens.Load(), "opens of style.css")
		require.Equal(t, int64(5), store.puts.Load(), "attempts of 1 write")
	})
}

// stallingFS stalls the nth open of /style.css, closing stalled, until unstall.
// Requests open it to hash, then to compress. Create it in the synctest bubble.
type stallingFS struct {
	http.FileSystem
	n       int64
	opens   atomic.Int64 // Of /style.css.
	stalled chan struct{}
	release chan struct{}
	once    sync.Once
}

func newStallingFS(n int64) *stallingFS {
	return &stallingFS{
		FileSystem: http.FS(assetFiles()),
		n:          n,
		stalled:    make(chan struct{}),
		release:    make(chan struct{}),
	}
}

func (fsys *stallingFS) Open(name string) (http.File, error) {
	f, err := fsys.FileSystem.Open(name)
	if err != nil || name != "/style.css" || fsys.opens.Add(1) != fsys.n {
		return f, err
	}
	close(fsys.stalled)
	<-fsys.release
	return f, nil
}

func (fsys *stallingFS) unstall() { fsys.once.Do(func() { close(fsys.release) }) }

// TestHandlerCompressesInParallel tests that different files compress in parallel.
func TestHandlerCompressesInParallel(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		fsys := newStallingFS(2)
		defer fsys.unstall()
		h := newHandler(newCompressor(t, assetcompress.Config{}), fsys, nil)
		stalled := make(chan *httptest.ResponseRecorder, 1)
		go func() { stalled <- get(t, h, http.MethodGet, "/style.css", "br", nil) }()
		<-fsys.stalled

		w := get(t, h, http.MethodGet, "/app.js", "br", nil)
		require.Equal(t, "br", w.Header().Get("Content-Encoding"))
		require.Equal(t, assetJS, decode(t, "br", w.Body.Bytes()))
		fsys.unstall()
		w = <-stalled
		require.Equal(t, assetCSS, decode(t, "br", w.Body.Bytes()))
	})
}

// TestHandlerInitialLevel tests the initial copy, its replacement and the store.
func TestHandlerInitialLevel(t *testing.T) {
	t.Parallel()

	store := newCountingStore()
	c := newCompressor(t, assetcompress.Config{
		Store: store, InitialLevels: assetcompress.Levels{BrotliQuality: 1},
	})
	h := withETag(`"base"`, newHandler(c, http.FS(assetFiles()), nil))
	w := get(t, h, http.MethodGet, "/style.css", "br", nil)
	require.Equal(t, `"base.br1"`, w.Header().Get("Etag"))
	require.Equal(t, assetCSS, decode(t, "br", w.Body.Bytes()))
	assetcompress.WaitBackground(c)
	require.Equal(t, int64(1), store.puts.Load())

	w = get(t, h, http.MethodGet, "/style.css", "br", nil)
	require.Equal(t, `"base.br6"`, w.Header().Get("Etag"))
	require.Equal(t, assetCSS, decode(t, "br", w.Body.Bytes()))
	w = get(t, h, http.MethodGet, "/style.css", "gzip", nil)
	require.Equal(t, `"base.gzip9"`, w.Header().Get("Etag"), "no initial gzip level")
}

// TestHandlerWithoutStore tests that a compressor without a store replaces the
// initial copy in memory, writes nothing and serves the copy without reading
// the file again.
func TestHandlerWithoutStore(t *testing.T) {
	t.Parallel()

	fsys := &readCountingFS{FileSystem: http.FS(assetFiles())}
	var log testlog.Buffer
	c := newCompressor(t, assetcompress.Config{
		InitialLevels: assetcompress.Levels{BrotliQuality: 1},
	})
	h := withETag(`"base"`,
		c.Handler(fsys, styleSum, http.FileServer(fsys), log.Logger()))
	w := get(t, h, http.MethodGet, "/style.css", "br", nil)
	require.Equal(t, `"base.br1"`, w.Header().Get("Etag"))
	assetcompress.WaitBackground(c)

	read := fsys.read.Load()
	w = get(t, h, http.MethodGet, "/style.css", "br", nil)
	require.Equal(t, `"base.br6"`, w.Header().Get("Etag"))
	require.Equal(t, assetCSS, decode(t, "br", w.Body.Bytes()))
	require.Equal(t, read, fsys.read.Load(), "bytes read by the 2nd request")
	require.Empty(t, log.Lines())
}

// TestHandlerReplacesOneAtATime tests that replacements run 1 at a time.
func TestHandlerReplacesOneAtATime(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		// The 3rd open of /style.css replaces its copy.
		fsys := newStallingFS(3)
		defer fsys.unstall()
		store := newCountingStore()
		c := newCompressor(t, assetcompress.Config{
			Store: store, InitialLevels: assetcompress.Levels{BrotliQuality: 1},
		})
		h := withETag(`"base"`, newHandler(c, fsys, nil))
		w := get(t, h, http.MethodGet, "/style.css", "br", nil)
		require.Equal(t, `"base.br1"`, w.Header().Get("Etag"))
		<-fsys.stalled
		w = get(t, h, http.MethodGet, "/app.js", "br", nil)
		require.Equal(t, `"base.br1"`, w.Header().Get("Etag"))
		synctest.Wait()
		require.Zero(t, store.puts.Load(), "app.js did not wait for style.css")

		fsys.unstall()
		assetcompress.WaitBackground(c)
		require.Equal(t, int64(2), store.puts.Load())
		for _, name := range []string{"/style.css", "/app.js"} {
			w = get(t, h, http.MethodGet, name, "br", nil)
			require.Equal(t, `"base.br6"`, w.Header().Get("Etag"), name)
		}
	})
}

// failingFS fails the nth open of /style.css.
type failingFS struct {
	http.FileSystem
	n     int64
	opens atomic.Int64
}

func (fsys *failingFS) Open(name string) (http.File, error) {
	if name == "/style.css" && fsys.opens.Add(1) == fsys.n {
		return nil, errors.New("open failed")
	}
	return fsys.FileSystem.Open(name)
}

// TestHandlerKeepsInitialCopy tests the log and the copy after a failed replacement.
func TestHandlerKeepsInitialCopy(t *testing.T) {
	t.Parallel()

	store := newCountingStore()
	c := newCompressor(t, assetcompress.Config{
		Store: store, InitialLevels: assetcompress.Levels{BrotliQuality: 1},
	})
	var log testlog.Buffer
	// The 3rd open of /style.css replaces its copy.
	fsys := &failingFS{FileSystem: http.FS(assetFiles()), n: 3}
	h := withETag(`"base"`, newHandler(c, fsys, log.Logger()))
	get(t, h, http.MethodGet, "/style.css", "br", nil)
	assetcompress.WaitBackground(c)

	w := get(t, h, http.MethodGet, "/style.css", "br", nil)
	require.Equal(t, `"base.br1"`, w.Header().Get("Etag"))
	require.Equal(t, assetCSS, decode(t, "br", w.Body.Bytes()))
	require.Zero(t, store.puts.Load())
	require.Len(t, log.Lines(), 1)
	log.RequireMatch(t, `level=WARN msg="compressing an asset" `+
		`path=/style.css coding=br err="open failed"`)
}

// TestHandlerKeepsContentType tests that a Content-Type set before the handler
// is kept and, when not eligible, rules out compression.
func TestHandlerKeepsContentType(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		contentType string
		wantCoding  string
	}{
		"eligible":     {contentType: "text/plain; charset=utf-8", wantCoding: "br"},
		"not eligible": {contentType: "application/octet-stream"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			h := newHandler(newCompressor(t, assetcompress.Config{}), http.FS(assetFiles()), nil)
			typed := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", tc.contentType)
				h.ServeHTTP(w, r)
			})
			for accept, wantCoding := range map[string]string{"": "", "br": tc.wantCoding} {
				w := get(t, typed, http.MethodGet, "/page.html", accept, nil)
				require.Equal(t, tc.contentType, w.Header().Get("Content-Type"), accept)
				require.Equal(t, wantCoding, w.Header().Get("Content-Encoding"), accept)
			}
		})
	}
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

// TestHandlerFileChangesDuringCompression tests that a changed source is
// rejected and that the next request gets a copy of the new content.
func TestHandlerFileChangesDuringCompression(t *testing.T) {
	t.Parallel()

	var log testlog.Buffer
	fsys := &changingFS{}
	// sum keeps the SHA-256 of the old content, as the server does.
	sum := func(string) ([sha256.Size]byte, bool) {
		return sha256.Sum256([]byte(assetCSS)), true
	}
	c := newCompressor(t, assetcompress.Config{})
	h := c.Handler(fsys, sum, http.FileServer(fsys), log.Logger())
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

// TestStoreFailure tests precompression and requests with a failing store.
func TestStoreFailure(t *testing.T) {
	t.Parallel()

	errStore := errors.New("store failed")
	for name, tc := range map[string]struct {
		store      *countingStore
		wantCoding string
		wantLog    string
	}{
		"open": {
			store: &countingStore{Store: new(memStore), openErr: errStore},
			wantLog: `level=WARN msg="compressing an asset" ` +
				`path=/app.js coding=br err="store failed"`,
		},
		"put": {
			store:      &countingStore{Store: new(memStore), putErr: errStore},
			wantCoding: "br",
			wantLog: `level=WARN msg="storing a compressed asset" ` +
				`key=[0-9a-f]{64}\.br6 attempts=5 err="store failed"`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c := newCompressor(t, assetcompress.Config{Store: tc.store})
			assetcompress.SetWriteRetry(c, time.Millisecond)
			require.EqualError(t, c.Precompress(context.Background(), assetFiles()),
				"precompressing app.js: store failed")

			var log testlog.Buffer
			w := get(t, newHandler(c, http.FS(assetFiles()), log.Logger()),
				http.MethodGet, "/app.js", "br", nil)
			assetcompress.WaitBackground(c)
			require.Equal(t, http.StatusOK, w.Code)
			require.Equal(t, tc.wantCoding, w.Header().Get("Content-Encoding"))
			require.True(t, varies(w.Header()))
			require.Equal(t, assetJS, decode(t, tc.wantCoding, w.Body.Bytes()))
			log.RequireMatch(t, tc.wantLog)
		})
	}
}

// lateFailingStore fails each write after reading it. A nonzero failures
// limits the failed writes.
type lateFailingStore struct {
	assetcompress.Store
	failures int64
	puts     atomic.Int64
}

func (s *lateFailingStore) Put(key string, r io.Reader) error {
	b, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	if n := s.puts.Add(1); s.failures == 0 || n <= s.failures {
		return errors.New("disk full")
	}
	return s.Store.Put(key, bytes.NewReader(b))
}

// TestHandlerServesCopyFromMemory tests a copy that every write attempt fails.
func TestHandlerServesCopyFromMemory(t *testing.T) {
	t.Parallel()

	store := &lateFailingStore{Store: new(memStore)}
	c := newCompressor(t, assetcompress.Config{Store: store})
	assetcompress.SetWriteRetry(c, time.Millisecond)
	var log testlog.Buffer
	h := newHandler(c, http.FS(assetFiles()), log.Logger())
	for range 3 {
		w := get(t, h, http.MethodGet, "/style.css", "br", nil)
		require.Equal(t, "br", w.Header().Get("Content-Encoding"))
		require.Equal(t, assetCSS, decode(t, "br", w.Body.Bytes()))
		assetcompress.WaitBackground(c)
	}
	require.Equal(t, int64(5), store.puts.Load(), "attempts to write the copy")
	require.Len(t, log.Lines(), 1)
	log.RequireMatch(t, `level=WARN msg="storing a compressed asset" `+
		`key=[0-9a-f]{64}\.br6 attempts=5 err="disk full"`)
}

// TestHandlerRetriesWrite tests a copy stored by the 3rd attempt, unlogged.
func TestHandlerRetriesWrite(t *testing.T) {
	t.Parallel()

	store := &lateFailingStore{Store: new(memStore), failures: 2}
	c := newCompressor(t, assetcompress.Config{Store: store})
	assetcompress.SetWriteRetry(c, time.Millisecond)
	var log testlog.Buffer
	w := get(t, newHandler(c, http.FS(assetFiles()), log.Logger()),
		http.MethodGet, "/style.css", "br", nil)
	require.Equal(t, assetCSS, decode(t, "br", w.Body.Bytes()))
	assetcompress.WaitBackground(c)

	require.Equal(t, int64(3), store.puts.Load())
	sum := sha256.Sum256([]byte(assetCSS))
	copied, err := store.Open(hex.EncodeToString(sum[:]) + ".br6")
	require.NoError(t, err, "the store has no copy")
	require.NoError(t, copied.Close())
	require.Empty(t, log.Lines())
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

// panickingFS panics while a copy of /style.css is compressed.
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

// TestHandlerPanic tests that store and file system panics are logged and that
// only the latter sends the file uncompressed.
func TestHandlerPanic(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		store      assetcompress.Store // Nil for the default.
		fsys       http.FileSystem
		wantCoding string
		wantLog    string
	}{
		"store": {
			store:      &panickingStore{Store: new(memStore)},
			fsys:       http.FS(assetFiles()),
			wantCoding: "br",
			wantLog: `level=ERROR msg="recovered panic while storing a compressed asset" ` +
				`key=[0-9a-f]{64}\.br6 panic="put failed" stack="goroutine `,
		},
		"file system": {
			fsys: &panickingFS{FileSystem: http.FS(assetFiles())},
			wantLog: `level=ERROR msg="recovered panic while compressing an asset" ` +
				`path=/style.css coding=br panic="read failed" stack="goroutine `,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var log testlog.Buffer
			c := newCompressor(t, assetcompress.Config{Store: tc.store})
			assetcompress.SetWriteRetry(c, time.Millisecond)
			h := newHandler(c, tc.fsys, log.Logger())
			w := get(t, h, http.MethodGet, "/style.css", "br", nil)
			assetcompress.WaitBackground(c)
			require.Equal(t, tc.wantCoding, w.Header().Get("Content-Encoding"))
			require.Equal(t, assetCSS, decode(t, tc.wantCoding, w.Body.Bytes()))
			log.RequireMatch(t, tc.wantLog)

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

// TestPrecompressWithoutStore tests that requests get the copies that
// Precompress keeps in memory without a store, without reading the file.
func TestPrecompressWithoutStore(t *testing.T) {
	t.Parallel()

	c := newCompressor(t, assetcompress.Config{})
	require.NoError(t, c.Precompress(context.Background(), assetFiles()))
	fsys := &readCountingFS{FileSystem: http.FS(assetFiles())}
	h := c.Handler(fsys, styleSum, http.FileServer(fsys), slog.New(slog.DiscardHandler))
	for _, coding := range []string{"br", "gzip"} {
		w := get(t, h, http.MethodGet, "/style.css", coding, nil)
		require.Equal(t, coding, w.Header().Get("Content-Encoding"))
		require.Equal(t, assetCSS, decode(t, coding, w.Body.Bytes()), coding)
	}
	require.Zero(t, fsys.read.Load(), "bytes read")
}

// readPanickingFS panics on every read of a file.
type readPanickingFS struct{ fstest.MapFS }

func (fsys readPanickingFS) Open(name string) (fs.File, error) {
	f, err := fsys.MapFS.Open(name)
	if err != nil {
		return nil, err
	}
	if st, err := f.Stat(); err == nil && !st.IsDir() {
		return readPanickingFile{f}, nil
	}
	return f, nil
}

type readPanickingFile struct{ fs.File }

func (readPanickingFile) Read([]byte) (int, error) {
	panic("read failed")
}

type openPanickingStore struct{ assetcompress.Store }

func (openPanickingStore) Open(string) (io.ReadSeekCloser, error) {
	panic("open failed")
}

// TestPrecompressPanic tests that a panic of the file system or of
// [assetcompress.Store.Open] returns as a PanicError for the file.
func TestPrecompressPanic(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		fsys  fs.FS
		store assetcompress.Store
		want  string
	}{
		"file system": {fsys: readPanickingFS{assetFiles()}, want: "read failed"},
		"store open": {
			fsys:  assetFiles(),
			store: openPanickingStore{new(memStore)},
			want:  "open failed",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c := newCompressor(t, assetcompress.Config{Store: tc.store})
			err := c.Precompress(context.Background(), tc.fsys)
			var panicked datapages.PanicError
			require.ErrorAs(t, err, &panicked)
			require.Equal(t, tc.want, panicked.Value)
			require.ErrorContains(t, err, "precompressing app.js")
		})
	}
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
	"../../example/offline-cache/app/static/basecoat-1.0.2.css",
	"../../example/classifieds/app/static/datastar-1.0.4.js",
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
				Codings: []datapages.Coding{datapages.CodingBrotli},
				Levels:  assetcompress.Levels{BrotliQuality: 11},
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
					c := newCompressor(b, tc.conf)
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

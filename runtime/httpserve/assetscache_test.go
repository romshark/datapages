package httpserve_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages"
)

// TestAssetsCacheHeaders tests Cache-Control and ETag selection for each configuration.
func TestAssetsCacheHeaders(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		conf             *datapages.AssetsCacheConfig
		wantCacheControl string
		wantETag         bool
	}{
		"no option": {
			conf:             nil,
			wantCacheControl: "public, max-age=0",
		},
		"disabled": {
			conf:             &datapages.AssetsCacheConfig{Disabled: true},
			wantCacheControl: "public, max-age=0",
		},
		"zero value": {
			conf:             &datapages.AssetsCacheConfig{},
			wantCacheControl: "public, max-age=0",
			wantETag:         true,
		},
		"max age": {
			conf:             &datapages.AssetsCacheConfig{MaxAge: time.Hour},
			wantCacheControl: "public, max-age=3600",
			wantETag:         true,
		},
		"immutable": {
			conf: &datapages.AssetsCacheConfig{
				MaxAge: 365 * 24 * time.Hour, Immutable: true,
			},
			wantCacheControl: "public, max-age=31536000, immutable",
			wantETag:         true,
		},
		"custom cache control": {
			conf:             &datapages.AssetsCacheConfig{CacheControl: "private, no-store"},
			wantCacheControl: "private, no-store",
			wantETag:         true,
		},
		"disable etag": {
			conf: &datapages.AssetsCacheConfig{
				MaxAge: time.Hour, DisableETag: true,
			},
			wantCacheControl: "public, max-age=3600",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c := mustCore(t, datapages.ServerConfig{
				AssetsFS:    http.Dir("testdata/static"),
				AssetsCache: tc.conf,
			}, "/static/")
			c.Build()

			w := serve(t, c, "/static/hello.txt")
			require.Equal(t, http.StatusOK, w.Code)
			require.Equal(t, tc.wantCacheControl, w.Header().Get("Cache-Control"))
			if !tc.wantETag {
				require.Empty(t, w.Header().Get("ETag"))
				return
			}
			require.NotEmpty(t, w.Header().Get("ETag"))
		})
	}
}

// openCountingFS counts the files it opens.
type openCountingFS struct {
	http.FileSystem
	opens atomic.Int64
}

func (fsys *openCountingFS) Open(name string) (http.File, error) {
	fsys.opens.Add(1)
	return fsys.FileSystem.Open(name)
}

// TestAssetsCacheWithoutETagOpensOnce tests that without
// ETags the cache opens a file on its first request only.
func TestAssetsCacheWithoutETagOpensOnce(t *testing.T) {
	t.Parallel()

	fsys := &openCountingFS{FileSystem: http.FS(assetFiles())}
	c := assetsCore(t, fsys, datapages.ServerConfig{
		AssetsCache: &datapages.AssetsCacheConfig{MaxAge: time.Hour, DisableETag: true},
	})
	w := getAsset(t, c, http.MethodGet, "/static/style.css", "br", nil)
	require.Equal(t, "br", w.Header().Get("Content-Encoding"))
	opens := fsys.opens.Load()
	for range 3 {
		w := getAsset(t, c, http.MethodGet, "/static/style.css", "br", nil)
		require.Equal(t, "public, max-age=3600", w.Header().Get("Cache-Control"))
		require.Empty(t, w.Header().Get("ETag"))
	}
	require.Equal(t, opens, fsys.opens.Load(), "opens after the first request")
}

// TestAssetsCacheConditionalRequest tests that a matching If-None-Match value
// returns 304 with an empty body.
func TestAssetsCacheConditionalRequest(t *testing.T) {
	t.Parallel()

	c := mustCore(t, datapages.ServerConfig{
		AssetsFS:    http.Dir("testdata/static"),
		AssetsCache: &datapages.AssetsCacheConfig{MaxAge: time.Hour},
	}, "/static/")
	c.Build()

	first := serve(t, c, "/static/hello.txt")
	require.Equal(t, http.StatusOK, first.Code)
	etag := first.Header().Get("ETag")
	require.NotEmpty(t, etag)

	r := httptest.NewRequest(http.MethodGet, "/static/hello.txt", nil)
	r.Header.Set("If-None-Match", etag)
	second := httptest.NewRecorder()
	c.ServeHTTP(second, r)

	require.Equal(t, http.StatusNotModified, second.Code)
	require.Empty(t, second.Body.String())
	require.Equal(t, etag, second.Header().Get("ETag"))
	require.Equal(t, "public, max-age=3600", second.Header().Get("Cache-Control"))
}

// TestAssetsCacheETagPerFile tests that different files have different ETags
// and 1 file keeps the same ETag.
func TestAssetsCacheETagPerFile(t *testing.T) {
	t.Parallel()

	c := mustCore(t, datapages.ServerConfig{
		AssetsFS:    http.Dir("testdata/static"),
		AssetsCache: &datapages.AssetsCacheConfig{},
	}, "/static/")
	c.Build()

	hello := serve(t, c, "/static/hello.txt").Header().Get("ETag")
	nested := serve(t, c, "/static/sub/nested.txt").Header().Get("ETag")

	require.NotEmpty(t, hello)
	require.NotEqual(t, hello, nested)
	require.Equal(t, hello, serve(t, c, "/static/hello.txt").Header().Get("ETag"))
}

// TestAssetsCacheSkipsNonFiles tests that missing files and directory listings
// receive no added cache headers.
func TestAssetsCacheSkipsNonFiles(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		browsable  bool
		url        string
		wantStatus int
	}{
		"missing": {url: "/static/nope.txt", wantStatus: http.StatusNotFound},
		"hidden":  {url: "/static/sub/", wantStatus: http.StatusNotFound},
		"listing": {browsable: true, url: "/static/", wantStatus: http.StatusOK},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c := mustCore(t, datapages.ServerConfig{
				AssetsFS:        http.Dir("testdata/static"),
				AssetsBrowsable: tc.browsable,
				AssetsCache:     &datapages.AssetsCacheConfig{MaxAge: time.Hour},
			}, "/static/")
			c.Build()

			w := serve(t, c, tc.url)
			require.Equal(t, tc.wantStatus, w.Code)
			require.Empty(t, w.Header().Get("Cache-Control"))
			require.Empty(t, w.Header().Get("ETag"))
		})
	}
}

// TestAssetsCacheDevMode tests that dev mode overrides asset cache settings
// with no-cache, no ETag and no Last-Modified, which could answer 304 for a file
// saved 2 times within 1 second.
func TestAssetsCacheDevMode(t *testing.T) {
	t.Setenv("DATAPAGES_DEV_MODE", "1")

	c := mustCore(t, datapages.ServerConfig{
		AssetsFS: http.Dir("testdata/static"),
		AssetsCache: &datapages.AssetsCacheConfig{
			MaxAge: time.Hour, Immutable: true,
		},
	}, "/static/")
	c.Build()

	w := serve(t, c, "/static/hello.txt")
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "no-cache", w.Header().Get("Cache-Control"))
	require.Empty(t, w.Header().Get("ETag"))
	require.Empty(t, w.Header().Get("Last-Modified"))
}

// TestAssetsCacheIndexHTML tests the headers for
// an index.html served from a directory URL.
func TestAssetsCacheIndexHTML(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(dir+"/index.html", []byte("<!doctype html>"), 0o600))

	c := mustCore(t, datapages.ServerConfig{
		AssetsFS:    http.Dir(dir),
		AssetsCache: &datapages.AssetsCacheConfig{MaxAge: time.Hour},
	}, "/static/")
	c.Build()

	w := serve(t, c, "/static/")
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "public, max-age=3600", w.Header().Get("Cache-Control"))
	require.NotEmpty(t, w.Header().Get("ETag"))
}

// TestAssetsCacheConcurrent tests that concurrent
// first requests for 1 file receive the same ETag.
func TestAssetsCacheConcurrent(t *testing.T) {
	t.Parallel()

	c := mustCore(t, datapages.ServerConfig{
		AssetsFS:    http.Dir("testdata/static"),
		AssetsCache: &datapages.AssetsCacheConfig{},
	}, "/static/")
	c.Build()

	const n = 16
	tags := make(chan string, n)
	for range n {
		go func() { tags <- serve(t, c, "/static/hello.txt").Header().Get("ETag") }()
	}
	want := <-tags
	require.NotEmpty(t, want)
	for range n - 1 {
		require.Equal(t, want, <-tags)
	}
}

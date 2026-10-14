package datapages_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages"
)

// TestWithDatastarJS tests the URLs the page shell may load the Datastar
// bundle from: http, https and relative URLs according to [RFC 3986].
//
// [RFC 3986]: https://www.rfc-editor.org/rfc/rfc3986
func TestWithDatastarJS(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		src string
		ok  bool
	}{
		"https":             {"https://cdn.example.com/ds.js", true},
		"http":              {"http://cdn.example.com/ds.js", true},
		"root relative":     {"/static/ds.js", true},
		"relative":          {"static/ds.js", true},
		"protocol relative": {"//cdn.example.com/ds.js", true},
		"query":             {"https://cdn.example.com/ds.js?v=1&min=1", true},
		"single quote":      {"https://cdn.example.com/o'neil/ds.js", true},
		"percent-encoded":   {"https://cdn.example.com/d%20s.js", true},
		"empty":             {"", false},
		"other scheme":      {"javascript:alert(1)", false},
		"double quote":      {`https://x/ds.js"`, false},
		"angle bracket":     {"https://x/ds.js<", false},
		"space":             {"https://x/d s.js", false},
		"control character": {"https://x/ds.js\nfoo", false},
		"brace":             {"https://x/{version}/ds.js", false},
		"non-ASCII":         {"https://x/dätastar.js", false},
		"bad escape":        {"https://x/ds%zz.js", false},
	} {
		t.Run(name, func(t *testing.T) {
			var cfg datapages.ServerConfig
			err := datapages.WithDatastarJS(tc.src)(&cfg)
			if tc.ok {
				require.NoError(t, err)
				require.Equal(t, tc.src, cfg.DatastarJS)
				return
			}
			require.ErrorContains(t, err, "WithDatastarJS")
		})
	}
}

// TestWithAssetsCache tests accepted configurations and rejects incompatible fields,
// invalid durations, and control characters.
func TestWithAssetsCache(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		conf    datapages.AssetsCacheConfig
		wantErr string
	}{
		"zero": {conf: datapages.AssetsCacheConfig{}},
		"disabled": {conf: datapages.AssetsCacheConfig{
			Disabled: true,
		}},
		"max age": {conf: datapages.AssetsCacheConfig{
			MaxAge: time.Hour,
		}},
		"immutable": {conf: datapages.AssetsCacheConfig{
			MaxAge: time.Hour, Immutable: true,
		}},
		"disable etag": {conf: datapages.AssetsCacheConfig{
			DisableETag: true,
		}},
		"cache control": {conf: datapages.AssetsCacheConfig{
			CacheControl: "no-store",
		}},
		"negative max age": {
			conf: datapages.AssetsCacheConfig{
				MaxAge: -time.Second,
			},
			wantErr: `max age (-1s) must not be negative`,
		},
		"sub second max age": {
			conf: datapages.AssetsCacheConfig{
				MaxAge: 500 * time.Millisecond,
			},
			wantErr: `max age (500ms) must be at least 1s`,
		},
		"immutable without max age": {
			conf: datapages.AssetsCacheConfig{
				Immutable: true,
			},
			wantErr: `immutable (true) requires a max age (0s) above 0`,
		},
		"cache control with max age": {
			conf: datapages.AssetsCacheConfig{
				CacheControl: "no-store", MaxAge: time.Hour,
			},
			wantErr: `cache control ("no-store") cannot be combined with ` +
				`max age (1h0m0s) or immutable (false)`,
		},
		"cache control with immutable": {
			conf: datapages.AssetsCacheConfig{
				CacheControl: "no-store",
				Immutable:    true,
			},
			wantErr: `cache control ("no-store") cannot be combined with ` +
				`max age (0s) or immutable (true)`,
		},
		"header injection": {
			conf: datapages.AssetsCacheConfig{
				CacheControl: "no-store\r\nX: 1",
			},
			wantErr: "contains a control character at byte 8",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var cfg datapages.ServerConfig
			err := datapages.WithAssetsCache(tc.conf)(&cfg)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				require.Nil(t, cfg.AssetsCache)
				return
			}
			require.NoError(t, err)
			require.Equal(t, &tc.conf, cfg.AssetsCache)
		})
	}
}

// TestWithSSECompression tests valid and invalid compression settings.
func TestWithSSECompression(t *testing.T) {
	t.Parallel()

	const (
		zstd   = datapages.SSECodingZstd
		brotli = datapages.SSECodingBrotli
		gzip   = datapages.SSECodingGzip
	)
	for name, tc := range map[string]struct {
		conf    datapages.SSECompressionConfig
		wantErr string
	}{
		"zero":            {},
		"balanced":        {conf: datapages.SSECompressionBalanced()},
		"low memory":      {conf: datapages.SSECompressionLowMemory()},
		"low bandwidth":   {conf: datapages.SSECompressionLowBandwidth()},
		"large fragments": {conf: datapages.SSECompressionLargeFragments()},
		"disabled":        {conf: datapages.SSECompressionConfig{Disabled: true}},
		"disabled ignores the rest": {conf: datapages.SSECompressionConfig{
			Disabled: true, ZstdLevel: 99,
		}},
		"codings": {conf: datapages.SSECompressionConfig{
			Codings: []datapages.SSECoding{gzip, brotli, zstd},
		}},
		"gzip only": {conf: datapages.SSECompressionConfig{
			Codings: []datapages.SSECoding{gzip},
		}},
		"zstd only": {conf: datapages.SSECompressionConfig{
			Codings: []datapages.SSECoding{zstd},
		}},
		"zstd, else brotli": {conf: datapages.SSECompressionConfig{
			Codings: []datapages.SSECoding{zstd, brotli},
		}},
		"larger zstd window": {conf: datapages.SSECompressionConfig{
			ZstdLevel: 3, ZstdWindow: 4 << 20,
		}},
		"bounds": {conf: datapages.SSECompressionConfig{
			ZstdLevel: 22, ZstdWindow: 8 << 20,
			BrotliQuality: 11, BrotliWindow: 16 << 20,
			GzipLevel: 9,
		}},
		"lower bounds": {conf: datapages.SSECompressionConfig{
			ZstdLevel: 1, ZstdWindow: 1 << 10,
			BrotliQuality: 1, BrotliWindow: 1 << 10,
			GzipLevel: 1,
		}},
		"unknown coding": {
			conf: datapages.SSECompressionConfig{
				Codings: []datapages.SSECoding{"deflate"},
			},
			wantErr: `unknown coding "deflate"`,
		},
		"coding listed 2 times": {
			conf: datapages.SSECompressionConfig{
				Codings: []datapages.SSECoding{gzip, zstd, gzip},
			},
			wantErr: `coding "gzip" listed 2 times`,
		},
		"zstd level too high": {
			conf:    datapages.SSECompressionConfig{ZstdLevel: 23},
			wantErr: "zstd level (23) must be from 1 to 22",
		},
		"zstd level negative": {
			conf:    datapages.SSECompressionConfig{ZstdLevel: -1},
			wantErr: "zstd level (-1) must be from 1 to 22",
		},
		"zstd window not a power of 2": {
			conf:    datapages.SSECompressionConfig{ZstdWindow: 3 << 10},
			wantErr: "zstd window (3072) must be a power of 2 from 1 KiB to 8 MiB",
		},
		"zstd window above what browsers decode": {
			conf:    datapages.SSECompressionConfig{ZstdWindow: 16 << 20},
			wantErr: "zstd window (16777216) must be a power of 2 from 1 KiB to 8 MiB",
		},
		"zstd window too small": {
			conf:    datapages.SSECompressionConfig{ZstdWindow: 512},
			wantErr: "zstd window (512) must be a power of 2 from 1 KiB to 8 MiB",
		},
		"brotli quality too high": {
			conf:    datapages.SSECompressionConfig{BrotliQuality: 12},
			wantErr: "brotli quality (12) must be from 1 to 11",
		},
		"brotli window too large": {
			conf:    datapages.SSECompressionConfig{BrotliWindow: 32 << 20},
			wantErr: "brotli window (33554432) must be a power of 2 from 1 KiB to 16 MiB",
		},
		"gzip level too high": {
			conf:    datapages.SSECompressionConfig{GzipLevel: 10},
			wantErr: "gzip level (10) must be from 1 to 9",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var cfg datapages.ServerConfig
			err := datapages.WithSSECompression(tc.conf)(&cfg)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, "WithSSECompression: "+tc.wantErr)
				require.Nil(t, cfg.SSECompression)
				return
			}
			require.NoError(t, err)
			require.Equal(t, &tc.conf, cfg.SSECompression)
		})
	}
}

// TestWithSSECompressionCopiesCodings tests that caller mutation has no effect.
func TestWithSSECompressionCopiesCodings(t *testing.T) {
	t.Parallel()

	codings := []datapages.SSECoding{datapages.SSECodingGzip}
	var cfg datapages.ServerConfig
	require.NoError(t, datapages.WithSSECompression(
		datapages.SSECompressionConfig{Codings: codings},
	)(&cfg))
	codings[0] = datapages.SSECodingZstd
	require.Equal(t,
		[]datapages.SSECoding{datapages.SSECodingGzip},
		cfg.SSECompression.Codings)
}

// TestWithBuildID tests that IDs fit unchanged in an HTTP header.
// Fetch rejects other characters, and servers trim surrounding whitespace.
func TestWithBuildID(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		id      string
		wantErr string // empty: the ID is accepted
	}{
		"commit hash": {id: "4f221f8b14b235c4b2f4212c133adf9f"},
		"release tag": {id: "v1.2.3-rc.1+build.7"},
		"punctuation": {id: `a"b<c>&d'e`},
		"longest":     {id: strings.Repeat("x", 128)},
		"empty":       {id: "", wantErr: "WithBuildID: id is empty"},
		"too long": {
			id:      strings.Repeat("x", 129),
			wantErr: "WithBuildID: id length (129) exceeds 128 bytes",
		},
		"space": {
			id:      "v1 v2",
			wantErr: "WithBuildID: id contains a space at byte 2",
		},
		"control character": {
			id:      "v1\r\nX: 1",
			wantErr: `WithBuildID: id contains control character '\r' at byte 2`,
		},
		"delete": {
			id:      "v1\x7f",
			wantErr: `WithBuildID: id contains control character '\x7f' at byte 2`,
		},
		"non-ASCII": {
			id:      "v1-ü",
			wantErr: "WithBuildID: id contains non-ASCII character 'ü' at byte 3",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var cfg datapages.ServerConfig
			err := datapages.WithBuildID(tc.id)(&cfg)
			if tc.wantErr == "" {
				require.NoError(t, err)
				require.Equal(t, tc.id, cfg.BuildID)
				return
			}
			require.EqualError(t, err, tc.wantErr)
			require.Empty(t, cfg.BuildID)
		})
	}
}

// TestWithCSPNonce tests that the option stores its callback and rejects nil.
func TestWithCSPNonce(t *testing.T) {
	t.Parallel()

	var cfg datapages.ServerConfig
	require.ErrorContains(t, datapages.WithCSPNonce(nil)(&cfg), "WithCSPNonce")
	require.Nil(t, cfg.CSPNonce)

	require.NoError(t, datapages.WithCSPNonce(
		func(*http.Request) string { return "n0nce" },
	)(&cfg))
	require.NotNil(t, cfg.CSPNonce)
	require.Equal(t, "n0nce",
		cfg.CSPNonce(httptest.NewRequest(http.MethodGet, "/", nil)))
}

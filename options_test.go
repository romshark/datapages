package datapages_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages"
)

// TestWithDatastarJS tests the URL that goes into the src attribute of a script
// tag in the head of every page. A quote in it closes the attribute and turns
// the rest of the value into further attributes on that tag.
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
		"empty":             {"", false},
		"other scheme":      {"javascript:alert(1)", false},
		"double quote":      {`https://x/ds.js" onload="alert(1)`, false},
		"single quote":      {"https://x/ds.js' onload='alert(1)", false},
		"angle bracket":     {"https://x/ds.js></script><script>", false},
		"space":             {"https://x/d s.js", false},
		"control character": {"https://x/ds.js\nfoo", false},
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
			wantErr: `immutable (true) requires a max age (0s) above zero`,
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

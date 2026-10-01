package httpserve_test

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/runtime/httpserve"
)

// TestRejectStaleBuild tests that only stale Datastar requests are rejected.
// Application middleware still sees rejected requests.
func TestRejectStaleBuild(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		datastar  bool
		build     string
		wantStale bool
	}{
		"this build":    {datastar: true, build: "b1"},
		"another build": {datastar: true, build: "b0", wantStale: true},
		"no build":      {datastar: true},
		// The service worker sends a page fetch with the headers of the
		// Datastar request it answers, minus Datastar-Request.
		"not a datastar request": {build: "b0"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var sawMiddleware, sawHandler bool
			c := mustCore(t, datapages.ServerConfig{
				BuildID: "b1",
				Middleware: []func(http.Handler) http.Handler{
					func(next http.Handler) http.Handler {
						return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
							sawMiddleware = true
							next.ServeHTTP(w, r)
						})
					},
				},
			}, "")
			c.Mux().HandleFunc("/act/", func(w http.ResponseWriter, _ *http.Request) {
				sawHandler = true
				_, _ = io.WriteString(w, "handled")
			})
			c.Build()

			r := httptest.NewRequest(http.MethodPost, "/act/", nil)
			if tc.datastar {
				r.Header.Set("Datastar-Request", "true")
			}
			if tc.build != "" {
				r.Header.Set(datapages.HeaderBuild, tc.build)
			}
			w := httptest.NewRecorder()
			c.ServeHTTP(w, r)

			require.True(t, sawMiddleware)
			require.Equal(t, !tc.wantStale, sawHandler)
			if !tc.wantStale {
				require.Equal(t, http.StatusOK, w.Code)
				require.Equal(t, "handled", w.Body.String())
				return
			}
			require.Equal(t, http.StatusResetContent, w.Code)
			require.Equal(t, "b1", w.Header().Get(datapages.HeaderBuild))
			require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
			require.Empty(t, w.Body.String())
		})
	}
}

// TestWriteHTMLBuild tests the order, nonce, and escaping of the build head.
func TestWriteHTMLBuild(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		nonce      func(*http.Request) string
		wantScript string
	}{
		"no nonce": {wantScript: "<script>(() => {"},
		"nonce": {
			nonce:      func(*http.Request) string { return "n0nce" },
			wantScript: `<script nonce="n0nce">(() => {`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c := mustCore(t, datapages.ServerConfig{
				DatastarJS: "/ds.js", BuildID: `v1"<x>`, CSPNonce: tc.nonce,
			}, "")
			c.Build()
			require.Equal(t, `v1"<x>`, c.BuildID())

			page, err := writeHTML(t, c, httpserve.HTMLDocument{})
			require.NoError(t, err)

			meta := strings.Index(page,
				`<meta name="datapages-build" content="v1&#34;&lt;x&gt;"/>`)
			script := strings.Index(page, tc.wantScript)
			datastar := strings.Index(page, `src="/ds.js"`)
			require.Positive(t, meta, page)
			require.Greater(t, script, meta, page)
			require.Greater(t, datastar, script, page)
			require.Contains(t, page[script:datastar], `"`+datapages.HeaderBuild+`"`)
		})
	}
}

// TestDefaultBuildID tests that all servers use the executable hash by default.
func TestDefaultBuildID(t *testing.T) {
	t.Parallel()

	exe, err := os.Executable()
	require.NoError(t, err)
	b, err := os.ReadFile(exe)
	require.NoError(t, err)
	sum := sha256.Sum256(b)
	want := hex.EncodeToString(sum[:16])

	a := mustCore(t, datapages.ServerConfig{}, "")
	other := mustCore(t, datapages.ServerConfig{}, "")
	require.Equal(t, want, a.BuildID())
	require.Equal(t, want, other.BuildID())
}

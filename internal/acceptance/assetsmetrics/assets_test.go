// Tests the generated asset serving and metrics of ./app.

package acceptance_test

import (
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/datapagestest"
	"github.com/romshark/datapages/internal/acceptance/assetsmetrics/app"
	"github.com/romshark/datapages/internal/acceptance/assetsmetrics/app/datapagesgen"
	"github.com/romshark/datapages/internal/acceptance/assetsmetrics/app/datapagesgen/assets"
	"github.com/romshark/datapages/internal/acceptance/assetsmetrics/app/datapagesgen/href"
	"github.com/romshark/datapages/modules/assetcompress"
	"github.com/romshark/datapages/modules/assetcompress/disk"
	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/messaging/inmem"
)

// registry is shared. The generated code registers its collectors once per
// process and a second registry would come up empty.
var registry = prometheus.NewRegistry()

func serve(t *testing.T, opts ...datapages.ServerOption) *datapagestest.Server {
	t.Helper()
	return datapagestest.New(t, mustNewServer(
		t,
		&app.App{}, inmem.New(messaging.DefaultBrokerChanBuffer),
		append([]datapages.ServerOption{
			datapages.WithAssets(app.StaticFS, false),
			datapages.WithPrometheus(datapages.PrometheusConfig{
				Host:       "127.0.0.1:0",
				Registerer: registry,
				Gatherer:   registry,
			}),
		}, opts...)...,
	))
}

// TestAssetPath tests the asset URL builders against path.Join,
// which is what they fall back to.
func TestAssetPath(t *testing.T) {
	t.Parallel()
	for _, p := range []string{
		"style.css", "sub/nested.js", "", ".", "..", "../secret", "/style.css",
		"sub//nested.js", "sub/./nested.js", "sub/../nested.js", "sub/",
		"..hidden.css",
	} {
		want := path.Join(assets.URLPrefix, p)
		require.Equal(t, want, href.Asset(p), "href.Asset(%q)", p)
		require.Equal(t, want, assets.Path(p), "assets.Path(%q)", p)
	}
}

// TestAssetsAreServed tests the files the app embeds. The prefix comes from
// the configuration and reaches both the URL builder and the route.
// The 2 must agree on it.
func TestAssetsAreServed(t *testing.T) {
	t.Parallel()
	v := serve(t).Visitor(t)

	tests := map[string]struct {
		url        string
		wantStatus int
		wantBody   string
		wantType   string
	}{
		"a file at the root of the assets dir": {
			url:        href.Asset("style.css"),
			wantStatus: http.StatusOK,
			wantBody:   "rebeccapurple",
			wantType:   "text/css",
		},
		"a file in a subdirectory": {
			url:        href.Asset("sub/nested.js"),
			wantStatus: http.StatusOK,
			wantBody:   "nested",
			wantType:   "text/javascript",
		},
		"a file that is not there": {
			url:        href.Asset("nope.css"),
			wantStatus: http.StatusNotFound,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			resp := v.Get(t, tt.url)
			require.Equal(t, tt.wantStatus, resp.Status, "GET %s", tt.url)
			if tt.wantBody != "" {
				require.Contains(t, resp.Body, tt.wantBody, "GET %s", tt.url)
			}
			if tt.wantType != "" {
				require.True(t,
					strings.HasPrefix(resp.Header.Get("Content-Type"), tt.wantType),
					"GET %s: Content-Type = %q, want prefix %q",
					tt.url, resp.Header.Get("Content-Type"), tt.wantType)
			}
		})
	}
}

// TestPageIsCompressed tests that gzip wraps the complete identity response once.
func TestPageIsCompressed(t *testing.T) {
	t.Parallel()
	v := serve(t).Visitor(t)

	plain := v.Get(t, "/")
	require.Empty(t, plain.Header.Get("Content-Encoding"))

	req := v.NewRequest(t, http.MethodGet, "/", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	resp := v.Send(t, req)
	require.Equal(t, http.StatusOK, resp.Status)
	require.Equal(t, "gzip", resp.Header.Get("Content-Encoding"))
	require.Contains(t, resp.Header.Values("Vary"), "Accept-Encoding")
	raw := strings.NewReader(resp.Body)
	zr, err := gzip.NewReader(raw)
	require.NoError(t, err, "reading the gzip header")
	zr.Multistream(false)
	got, err := io.ReadAll(zr)
	require.NoError(t, err, "decoding the page")
	require.Equal(t, plain.Body, string(got))
	require.Zero(t, raw.Len(), "the page has bytes after its gzip stream")
}

// countingStore counts the copies put into it.
type countingStore struct {
	assetcompress.Store
	puts atomic.Int64
}

func (s *countingStore) Put(key string, r io.Reader) error {
	s.puts.Add(1)
	return s.Store.Put(key, r)
}

// TestAssetIsCompressed tests that precompressing the embed.FS passed to
// WithAssets creates the copies the generated server sends from its subdirectory.
func TestAssetIsCompressed(t *testing.T) {
	t.Parallel()
	copies, err := disk.New(t.TempDir(), disk.Config{})
	require.NoError(t, err)
	t.Cleanup(func() { _ = copies.Close() })
	store := &countingStore{Store: copies}
	compressor, err := assetcompress.New(assetcompress.Config{Store: store})
	require.NoError(t, err)
	v := serve(t, datapages.WithAssetsCompression(compressor)).Visitor(t)
	require.NoError(t, compressor.Precompress(context.Background(), app.StaticFS))
	// Brotli and gzip copies of theme.css. The other files are below 256 bytes.
	require.Equal(t, int64(2), store.puts.Load())

	want, err := app.StaticFS.ReadFile("static/theme.css")
	require.NoError(t, err, "reading theme.css")
	plain := v.Get(t, href.Asset("theme.css"))
	require.Empty(t, plain.Header.Get("Content-Encoding"))
	require.Equal(t, string(want), plain.Body)

	req := v.NewRequest(t, http.MethodGet, href.Asset("theme.css"), nil)
	req.Header.Set("Accept-Encoding", "gzip")
	resp := v.Send(t, req)
	require.Equal(t, http.StatusOK, resp.Status)
	require.Equal(t, "gzip", resp.Header.Get("Content-Encoding"))
	require.Contains(t, resp.Header.Values("Vary"), "Accept-Encoding")
	require.Equal(t, strconv.Itoa(len(resp.Body)), resp.Header.Get("Content-Length"))
	zr, err := gzip.NewReader(strings.NewReader(resp.Body))
	require.NoError(t, err, "reading the gzip header")
	got, err := io.ReadAll(zr)
	require.NoError(t, err, "decoding the copy")
	require.Equal(t, string(want), string(got))
	require.Equal(t, int64(2), store.puts.Load(), "the request created a copy")
}

// TestAssetsNotBrowsable tests browsable false: a request for a directory
// holding no index.html is a 404, whichever way it is spelled.
func TestAssetsNotBrowsable(t *testing.T) {
	t.Parallel()
	v := serve(t).Visitor(t)

	for name, url := range map[string]string{
		"root with slash":      "/static/",
		"root without slash":   "/static",
		"subdir with slash":    "/static/sub/",
		"subdir without slash": "/static/sub",
	} {
		t.Run(name, func(t *testing.T) {
			resp := v.Get(t, url)
			require.Equal(t, http.StatusNotFound, resp.Status, "GET %s", url)
			require.NotContains(t, resp.Body, "nested.js", "GET %s listed the tree", url)
		})
	}
}

// TestAssetsBrowsable tests browsable true:
// the application gets the listing http.FileServer generates.
func TestAssetsBrowsable(t *testing.T) {
	t.Parallel()
	srv := datapagestest.New(t, mustNewServer(
		t,
		&app.App{}, inmem.New(messaging.DefaultBrokerChanBuffer),
		datapages.WithAssets(app.StaticFS, true),
		datapages.WithPrometheus(datapages.PrometheusConfig{
			Host:       "127.0.0.1:0",
			Registerer: registry,
			Gatherer:   registry,
		}),
	))

	resp := srv.Visitor(t).Get(t, "/static/")
	require.Equal(t, http.StatusOK, resp.Status)
	require.Contains(t, resp.Body, "style.css")
}

// TestAssetURLs tests the 2 generated ways to name an asset.
// Both are used in templates and both must produce the configured prefix.
func TestAssetURLs(t *testing.T) {
	t.Parallel()
	require.Equal(t, "/static/", assets.URLPrefix)
	require.Equal(t, "/static/style.css", assets.Path("style.css"))
	require.Equal(t, "/static/style.css", href.Asset("style.css"))
}

// TestAssetsEscapeTheirDirectory tests a path that climbs out of the embedded directory.
func TestAssetsEscapeTheirDirectory(t *testing.T) {
	t.Parallel()
	v := serve(t).Visitor(t)

	for name, url := range map[string]string{
		"dot segments":   "/static/../app.go",
		"encoded climb":  "/static/%2e%2e/app.go",
		"absolute climb": "/static//etc/passwd",
	} {
		t.Run(name, func(t *testing.T) {
			resp := v.Get(t, url)
			// ServeMux redirects a path with a dot segment or an empty segment to
			// its clean form, which the browser loads next.
			if resp.Status >= 300 && resp.Status < 400 {
				resp = v.Get(t, resp.Redirect(t))
			}
			require.False(t,
				resp.Status == http.StatusOK &&
					strings.Contains(resp.Body, "package app"),
				"GET %s served a file outside the assets directory", url)
		})
	}
}

// TestDatapagesDevModeServesFromDisk tests that DATAPAGES_DEV_MODE makes
// WithAssets read from disk without setting TEMPL_DEV_MODE.
//
// TestDatapagesDevModeServesFromDisk must not use t.Parallel() because
// [testing.T.Setenv] forbids it.
func TestDatapagesDevModeServesFromDisk(t *testing.T) {
	t.Setenv("TEMPL_DEV_MODE", "")
	t.Setenv(datapages.EnvVarDevMode, "true")
	require.True(t, datapages.IsDevMode(), "the server does not consider this dev mode")

	srv := serve(t)

	require.Empty(t, os.Getenv("TEMPL_DEV_MODE"),
		"NewServer changed TEMPL_DEV_MODE after templ package initialization")

	resp := srv.Visitor(t).Get(t, href.Asset("style.css"))
	require.Equal(t, http.StatusOK, resp.Status)
	require.Contains(t, resp.Body, "rebeccapurple", "the file on disk was not served")
}

// TestDevModeServesFromDisk tests what WithAssets does in development.
//
// In dev mode the files come from the directory on disk rather than from the binary.
// An edit to a stylesheet then shows up without a rebuild.
// This works only if the path the generator wrote into the assets package matches where
// the files are, and only if the browser asks for every response again.
// A stylesheet reused from the cache is a change the developer cannot see.
//
// TestDevModeServesFromDisk must not use t.Parallel() because
// [testing.T.Setenv] forbids it.
func TestDevModeServesFromDisk(t *testing.T) {
	// The generated code reads dev mode from the environment when the option is applied.
	// The variable is set before the server is built.
	t.Setenv("TEMPL_DEV_MODE", "true")
	require.True(t, datapages.IsDevMode(), "the server does not consider this dev mode")

	resp := serve(t).Visitor(t).Get(t, href.Asset("style.css"))
	require.Equal(t, http.StatusOK, resp.Status,
		"the dev directory the generator wrote is %q, relative to the module root",
		assets.DevDir)
	require.Contains(t, resp.Body, "rebeccapurple",
		"the file on disk was not served")
	require.Equal(t, "no-cache", resp.Header.Get("Cache-Control"),
		"the browser may reuse an asset without asking in dev mode")
	require.Empty(t, resp.Header.Get("Last-Modified"),
		"a file saved 2 times within 1 second could get a 304 for its first version")
}

// TestHalfWrittenBodyGetsNoErrorStatus tests an action that fails after
// writing over SSE. The response head is already out, so an error status
// writes its text into the body the client is reading.
// prom.Middleware replaces the response writer with its own,
// which httpserve.ResponseBodyWritten has to see through.
func TestHalfWrittenBodyGetsNoErrorStatus(t *testing.T) {
	index := serve(t).Visitor(t).Load(t, "/")

	resp := index.Do(t, http.MethodPost, "/half-written/", nil)
	require.Contains(t, resp.Body, `<pre id="echo">half</pre>`)
	require.NotContains(t, resp.Body, http.StatusText(http.StatusInternalServerError),
		"the error status was appended to what the client already received")
}

// TestMetrics tests the instrumentation the Prometheus option adds.
// The counters are read from the registry the server was given,
// the same registry a scrape reads.
func TestMetrics(t *testing.T) {
	t.Parallel()
	index := serve(t).Visitor(t).Load(t, "/")
	index.Do(t, http.MethodPost, "/fail/", nil)

	families, err := registry.Gather()
	require.NoError(t, err, "gathering metrics")
	byName := map[string]*dto.MetricFamily{}
	for _, f := range families {
		byName[f.GetName()] = f
	}

	reqs, ok := byName["datapages_http_requests_total"]
	require.True(t, ok, "the request counter was not registered")
	var total float64
	for _, m := range reqs.GetMetric() {
		total += m.GetCounter().GetValue()
	}
	require.GreaterOrEqual(t, total, float64(2),
		"the request counter stands at %v after 2 requests", total)

	require.Contains(t, byName, "datapages_http_request_duration_seconds",
		"the request duration histogram was not registered")
}

// TestMetricsLabelThroughDerivedRequest tests application middleware that
// stores a CSP nonce in the request context, as the WithCSPNonce docs advise.
// The router receives a copy of the request and sets the matched route on the copy.
func TestMetricsLabelThroughDerivedRequest(t *testing.T) {
	t.Parallel()
	type nonceKey struct{}
	v := serve(t, datapages.WithMiddleware(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), nonceKey{}, "nonce")
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})).Visitor(t)

	// PageQuiet's page route, which no other test requests.
	// The registry still holds the requests of earlier runs of this test under -count.
	before := histogramCount(t, "GET /quiet/{$}")
	resp := v.Get(t, "/quiet/")
	require.Equal(t, http.StatusOK, resp.Status, "GET /quiet/")

	require.Equal(t, before+1, histogramCount(t, "GET /quiet/{$}"),
		"the request was not observed under its route")
}

// TestStreamStaysOutOfRequestLatency tests the request metrics a stream leaves behind.
// It lives as long as the browser holds the page: observed as a request,
// an hour-long one lands above the histogram's top bucket.
func TestStreamStaysOutOfRequestLatency(t *testing.T) {
	t.Parallel()
	srv := serve(t)
	v := srv.Visitor(t)

	// PageQuiet's route, which no other test sends anything to:
	// what the histogram holds for it is this stream and nothing else.
	// The page itself is not loaded: TestMetricsLabelThroughDerivedRequest
	// requires its own load of it to be the only one.
	req := v.NewRequest(t, http.MethodGet, "/quiet/_$/", nil)
	req.Header.Set("Datastar-Request", "true")
	stream, err := v.Client().Do(req)
	require.NoError(t, err, "opening stream")
	require.Equal(t, http.StatusOK, stream.StatusCode, "opening stream")

	_ = stream.Body.Close()

	// The counter is written when the handler returns,
	// which the client cannot observe on a stream it closed.
	srv.WaitFor(t, "the stream to be counted", func() bool {
		return strings.Contains(family(t, "datapages_http_requests_total"),
			"GET /quiet/_$/{$}")
	})

	require.NotContains(t, family(t, "datapages_http_request_duration_seconds"),
		"GET /quiet/_$/{$}", "the stream was observed as a request latency")
}

// family renders the metric family name as the registry holds it.
func family(t *testing.T, name string) string {
	t.Helper()
	families, err := registry.Gather()
	require.NoError(t, err, "gathering metrics")
	for _, f := range families {
		if f.GetName() == name {
			return f.String()
		}
	}
	return ""
}

// TestRefusedStreamStaysARequest tests a stream StreamOpen refuses.
// It never reaches the message loop, which leaves it an ordinary request:
// counted, in-flight while it runs and observed in the latency histogram.
func TestRefusedStreamStaysARequest(t *testing.T) {
	t.Parallel()
	v := serve(t).Visitor(t)
	index := v.Load(t, "/")

	before := histogramCount(t, "GET /_$/{$}")

	// Send reads to EOF, not just the head: the middleware records after the
	// handler returns, and the response ends with it.
	resp := v.Send(t, index.NewRequest(t, http.MethodGet, "/_$/?refuse=1", nil))
	// The stream commits 200 before StreamOpen can refuse it.
	require.Equal(t, http.StatusOK, resp.Status)

	require.Greater(t, histogramCount(t, "GET /_$/{$}"), before,
		"the refused stream was not observed as a request")
}

// histogramCount is how many requests the latency histogram observed for path.
func histogramCount(t *testing.T, path string) uint64 {
	t.Helper()
	families, err := registry.Gather()
	require.NoError(t, err, "gathering metrics")
	for _, f := range families {
		if f.GetName() != "datapages_http_request_duration_seconds" {
			continue
		}
		for _, m := range f.GetMetric() {
			if hasLabel(m, "path", path) {
				return m.GetHistogram().GetSampleCount()
			}
		}
	}
	return 0
}

// hasLabel reports whether m carries the label name=value.
func hasLabel(m *dto.Metric, name, value string) bool {
	for _, l := range m.GetLabel() {
		if l.GetName() == name && l.GetValue() == value {
			return true
		}
	}
	return false
}

// TestBrokerMetrics tests the counters the generated code hands the message broker.
// They are what an operator watches to see events flowing,
// and they only move if the generated dispatch passes them along.
func TestBrokerMetrics(t *testing.T) {
	t.Parallel()
	index := serve(t).Visitor(t).Load(t, "/")

	// A subscriber has to exist: a message with nowhere to go is not published,
	// and nothing counts it. The stream subscribes before it answers,
	// which Connect waits for.
	index.Connect(t)

	before := counterTotal(t, "datapages_event_broker_publishes_by_kind_total")

	resp := index.Do(t, http.MethodPost, "/announce/", map[string]string{"text": "hi"})
	require.Equal(t, http.StatusOK, resp.Status, "POST /announce/")

	after := counterTotal(t, "datapages_event_broker_publishes_by_kind_total")
	require.Greater(t, after, before,
		"the publish counter stands at %v after a dispatch, was %v",
		after, before)
}

// counterTotal sums every sample of a counter family in the registry.
func counterTotal(t *testing.T, name string) float64 {
	t.Helper()
	families, err := registry.Gather()
	require.NoError(t, err, "gathering metrics")
	var total float64
	for _, f := range families {
		if f.GetName() != name {
			continue
		}
		for _, m := range f.GetMetric() {
			total += m.GetCounter().GetValue()
		}
	}
	return total
}

// TestErrorCounters tests that every error of a handler counts as not recovered,
// since the app defines no RecoverError. A route that nothing serves is no
// error of a handler and counts as neither.
//
// It doesn't call t.Parallel, because the counters are global to the process.
// The parallel tests of the case start only after it returns.
func TestErrorCounters(t *testing.T) {
	v := serve(t).Visitor(t)
	tests := map[string]struct {
		method, path string
		notRecovered float64
	}{
		"action":                  {http.MethodPost, "/fail/", 1},
		"action after its events": {http.MethodPost, "/half-written/", 1},
		"refused stream":          {http.MethodGet, "/_$/?refuse=1", 1},
		"route miss":              {http.MethodGet, "/nowhere/", 0},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			recovered := counterTotal(t,
				"datapages_internal_errors_recovered_total")
			notRecovered := counterTotal(t,
				"datapages_internal_errors_not_recovered_total")

			req := v.NewRequest(t, tt.method, tt.path, nil)
			req.Header.Set("Datastar-Request", "true")
			// Send reads to EOF, and the response ends after the handler returns.
			v.Send(t, req)

			require.Zero(t,
				counterTotal(t,
					"datapages_internal_errors_recovered_total")-recovered,
				"recovered")
			require.Equal(t, tt.notRecovered,
				counterTotal(t,
					"datapages_internal_errors_not_recovered_total")-notRecovered,
				"not recovered")
		})
	}
}

// TestMetricsWithoutOption tests a server generated with
// datapages.EnablePrometheus and built without datapages.WithPrometheus.
// The metrics it counts have nowhere to go.
func TestMetricsWithoutOption(t *testing.T) {
	t.Parallel()
	s, err := datapages.NewServer[
		app.App,
		datapages.DisableSessions,
		datapages.EnablePrometheus,
		datapagesgen.Server,
	](&app.App{}, inmem.New(messaging.DefaultBrokerChanBuffer),
		datapages.WithAssets(app.StaticFS, false))
	require.Nil(t, s, "server built without WithPrometheus")
	require.ErrorContains(t, err, "missing option WithPrometheus")
}

// TestAssetsCacheHeaders tests the configured headers on embedded assets.
// embed.FS reports a zero modification time, so these responses have no
// Last-Modified header.
func TestAssetsCacheHeaders(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		opts             []datapages.ServerOption
		wantCacheControl string
		wantETag         bool
	}{
		"no option": {wantCacheControl: "public, max-age=0", wantETag: true},
		"default": {
			opts: []datapages.ServerOption{
				datapages.WithAssetsCache(datapages.AssetsCacheConfig{}),
			},
			wantCacheControl: "public, max-age=0",
			wantETag:         true,
		},
		"immutable": {
			opts: []datapages.ServerOption{
				datapages.WithAssetsCache(datapages.AssetsCacheConfig{
					MaxAge:    365 * 24 * time.Hour,
					Immutable: true,
				}),
			},
			wantCacheControl: "public, max-age=31536000, immutable",
			wantETag:         true,
		},
		"disabled": {
			opts: []datapages.ServerOption{
				datapages.WithAssetsCache(datapages.AssetsCacheConfig{Disabled: true}),
			},
			wantCacheControl: "public, max-age=0",
			wantETag:         true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			resp := serve(t, tc.opts...).Visitor(t).Get(t, href.Asset("style.css"))
			require.Equal(t, http.StatusOK, resp.Status)
			require.Empty(t, resp.Header.Get("Last-Modified"))
			require.Equal(t, tc.wantCacheControl, resp.Header.Get("Cache-Control"))
			if !tc.wantETag {
				require.Empty(t, resp.Header.Get("ETag"))
				return
			}
			require.NotEmpty(t, resp.Header.Get("ETag"))
		})
	}
}

// TestAssetsCacheRevalidates tests that a matching If-None-Match value returns
// 304 with an empty body for an embedded file, also without WithAssetsCache.
func TestAssetsCacheRevalidates(t *testing.T) {
	t.Parallel()

	for name, opts := range map[string][]datapages.ServerOption{
		"no option":  nil,
		"zero value": {datapages.WithAssetsCache(datapages.AssetsCacheConfig{})},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			v := serve(t, opts...).Visitor(t)

			first := v.Get(t, href.Asset("style.css"))
			require.Equal(t, http.StatusOK, first.Status)
			require.NotEmpty(t, first.Body)
			etag := first.Header.Get("ETag")
			require.NotEmpty(t, etag)

			req := v.NewRequest(t, http.MethodGet, href.Asset("style.css"), nil)
			req.Header.Set("If-None-Match", etag)
			second := v.Send(t, req)

			require.Equal(t, http.StatusNotModified, second.Status)
			require.Empty(t, second.Body)
			require.Equal(t, etag, second.Header.Get("ETag"))
		})
	}
}

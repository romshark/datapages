// Covers the generated asset serving and metrics of ./app.

package acceptance_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/internal/acceptance/assetsmetrics/app"
	"github.com/romshark/datapages/internal/acceptance/assetsmetrics/app/datapagesgen"
	"github.com/romshark/datapages/internal/acceptance/assetsmetrics/app/datapagesgen/assets"
	"github.com/romshark/datapages/internal/acceptance/assetsmetrics/app/datapagesgen/href"
	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/messaging/inmem"
)

// registry is shared. The generated code registers its collectors once per
// process and a second registry would come up empty.
var registry = prometheus.NewRegistry()

func newServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(mustNewServer(
		t,
		&app.App{}, inmem.New(messaging.DefaultBrokerChanBuffer),
		datapages.WithAssets(app.StaticFS),
		datapages.WithPrometheus(datapages.PrometheusConfig{
			Host:       "127.0.0.1:0",
			Registerer: registry,
			Gatherer:   registry,
		}),
	))
	t.Cleanup(srv.Close)
	return srv
}

func get(t *testing.T, srv *httptest.Server, path string) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(
		context.Background(), http.MethodGet, srv.URL+path, nil,
	)
	require.NoError(t, err, "building GET %s", path)
	req.Header.Set("Accept-Encoding", "identity")
	resp, err := srv.Client().Do(req)
	require.NoError(t, err, "GET %s", path)
	return resp
}

// TestAssetPath covers the asset URL builders against path.Join,
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

// TestAssetsAreServed covers the files the app embeds. The prefix comes from
// the configuration and reaches both the URL builder and the route.
// The two must agree on it.
func TestAssetsAreServed(t *testing.T) {
	t.Parallel()
	srv := newServer(t)

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
			resp := get(t, srv, tt.url)
			defer func() { _ = resp.Body.Close() }()
			b, err := io.ReadAll(resp.Body)
			require.NoError(t, err, "reading %s", tt.url)
			require.Equal(t, tt.wantStatus, resp.StatusCode, "GET %s", tt.url)
			if tt.wantBody != "" {
				require.Contains(t, string(b), tt.wantBody, "GET %s", tt.url)
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

// TestAssetURLs covers the two generated ways to name an asset.
// Both are used in templates and both must produce the configured prefix.
func TestAssetURLs(t *testing.T) {
	t.Parallel()
	require.Equal(t, "/static/", assets.URLPrefix)
	require.Equal(t, "/static/style.css", assets.Path("style.css"))
	require.Equal(t, "/static/style.css", href.Asset("style.css"))
}

// TestAssetsEscapeTheirDirectory covers a path that climbs out of the embedded directory.
func TestAssetsEscapeTheirDirectory(t *testing.T) {
	t.Parallel()
	srv := newServer(t)

	for name, url := range map[string]string{
		"dot segments":   "/static/../app.go",
		"encoded climb":  "/static/%2e%2e/app.go",
		"absolute climb": "/static//etc/passwd",
	} {
		t.Run(name, func(t *testing.T) {
			resp := get(t, srv, url)
			defer func() { _ = resp.Body.Close() }()
			b, _ := io.ReadAll(resp.Body)
			require.False(t,
				resp.StatusCode == http.StatusOK &&
					strings.Contains(string(b), "package app"),
				"GET %s served a file outside the assets directory", url)
		})
	}
}

// TestDevModeServesFromDisk covers what WithAssets does in development.
//
// In dev mode the files come from the directory on disk rather than from the binary.
// An edit to a stylesheet then shows up without a rebuild.
// This works only if the path the generator wrote into the assets package matches where
// the files are, and only if the responses are not cached.
// A cached stylesheet is a change the developer cannot see.
//
// TestDevModeServesFromDisk must not use t.Parallel() because
// [testing.T.Setenv] forbids it.
func TestDevModeServesFromDisk(t *testing.T) {
	// The generated code reads dev mode from the environment when the option is applied.
	// The variable is set before the server is built.
	t.Setenv("TEMPL_DEV_MODE", "true")
	require.True(t, datapages.IsDevMode(), "the server does not consider this dev mode")

	srv := httptest.NewServer(mustNewServer(
		t,
		&app.App{}, inmem.New(messaging.DefaultBrokerChanBuffer),
		datapages.WithAssets(app.StaticFS),
		datapages.WithPrometheus(datapages.PrometheusConfig{
			Host:       "127.0.0.1:0",
			Registerer: registry,
			Gatherer:   registry,
		}),
	))
	t.Cleanup(srv.Close)

	resp := get(t, srv, href.Asset("style.css"))
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "reading the stylesheet")
	require.Equal(t, http.StatusOK, resp.StatusCode,
		"the dev directory the generator wrote is %q, relative to the module root",
		assets.DevDir)
	require.Contains(t, string(b), "rebeccapurple",
		"the file on disk was not served")
	require.Contains(t, resp.Header.Get("Cache-Control"), "no-store",
		"caching is not forbidden in dev mode")
}

// TestMetrics covers the instrumentation the Prometheus option adds.
// The counters are read from the registry the server was given,
// the same registry a scrape reads.
func TestMetrics(t *testing.T) {
	t.Parallel()
	srv := newServer(t)

	resp := get(t, srv, "/")
	_ = resp.Body.Close()

	req, err := http.NewRequestWithContext(
		context.Background(), http.MethodPost, srv.URL+"/fail/", nil,
	)
	require.NoError(t, err, "building request")
	req.Header.Set("Datastar-Request", "true")
	failResp, err := srv.Client().Do(req)
	require.NoError(t, err, "POST /fail/")
	_ = failResp.Body.Close()

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
		"the request counter stands at %v after two requests", total)

	require.Contains(t, byName, "datapages_http_request_duration_seconds",
		"the request duration histogram was not registered")
}

// TestBrokerMetrics covers the counters the generated code hands the message broker.
// They are what an operator watches to see events flowing,
// and they only move if the generated dispatch passes them along.
func TestBrokerMetrics(t *testing.T) {
	t.Parallel()
	srv := newServer(t)

	// A subscriber has to exist: a message with nowhere to go is not published,
	// and nothing counts it.
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/_$/", nil)
	require.NoError(t, err, "building stream request")
	req.Header.Set("Datastar-Request", "true")
	req.Header.Set("Accept-Encoding", "identity")
	stream, err := srv.Client().Do(req)
	require.NoError(t, err, "opening stream")
	t.Cleanup(func() { _ = stream.Body.Close() })
	require.Equal(t, http.StatusOK, stream.StatusCode, "opening stream")
	time.Sleep(200 * time.Millisecond)

	before := counterTotal(t, "datapages_event_broker_publishes_by_kind_total")

	announce, err := http.NewRequestWithContext(context.Background(),
		http.MethodPost, srv.URL+"/announce/", strings.NewReader(`{"text":"hi"}`))
	require.NoError(t, err, "building announce request")
	announce.Header.Set("Datastar-Request", "true")
	announce.Header.Set("Content-Type", "application/json")
	resp, err := srv.Client().Do(announce)
	require.NoError(t, err, "POST /announce/")
	_ = resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, "POST /announce/")

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

// TestMetricsWithoutOption covers a server generated with
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
		datapages.WithAssets(app.StaticFS))
	require.Nil(t, s, "server built without WithPrometheus")
	require.ErrorContains(t, err, "missing option WithPrometheus")
}

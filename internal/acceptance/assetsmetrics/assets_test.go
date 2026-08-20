// Drives the generated asset serving and metrics of ./app.

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

	"github.com/romshark/datapages/internal/acceptance/assetsmetrics/app"
	"github.com/romshark/datapages/internal/acceptance/assetsmetrics/datapagesgen"
	"github.com/romshark/datapages/internal/acceptance/assetsmetrics/datapagesgen/assets"
	"github.com/romshark/datapages/internal/acceptance/assetsmetrics/datapagesgen/href"
	"github.com/romshark/datapages/modules/msgbroker/inmem"
)

// registry is shared. The generated code registers its collectors once per
// process and a second registry would come up empty.
var registry = prometheus.NewRegistry()

func newServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(datapagesgen.NewServer(
		&app.App{}, inmem.New(8),
		datapagesgen.WithAssets(app.StaticFS),
		datapagesgen.WithPrometheus(datapagesgen.PrometheusConfig{
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
	if err != nil {
		t.Fatalf("building GET %s: %v", path, err)
	}
	req.Header.Set("Accept-Encoding", "identity")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	return resp
}

// TestAssetsAreServed covers the files the app embeds. The prefix comes from
// the configuration and reaches both the URL builder and the route.
// The two must agree on it.
// TestAssetPath covers the asset URL builders against path.Join,
// which is what they fall back to.
func TestAssetPath(t *testing.T) {
	for _, p := range []string{
		"style.css", "sub/nested.js", "", ".", "..", "../secret", "/style.css",
		"sub//nested.js", "sub/./nested.js", "sub/../nested.js", "sub/",
		"..hidden.css",
	} {
		want := path.Join(assets.URLPrefix, p)
		if got := href.Asset(p); got != want {
			t.Errorf("href.Asset(%q) = %q, want %q", p, got, want)
		}
		if got := assets.Path(p); got != want {
			t.Errorf("assets.Path(%q) = %q, want %q", p, got, want)
		}
	}
}

func TestAssetsAreServed(t *testing.T) {
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
			if err != nil {
				t.Fatalf("reading %s: %v", tt.url, err)
			}
			if resp.StatusCode != tt.wantStatus {
				t.Fatalf("GET %s: status = %d, want %d",
					tt.url, resp.StatusCode, tt.wantStatus)
			}
			if tt.wantBody != "" && !strings.Contains(string(b), tt.wantBody) {
				t.Errorf("GET %s: body does not carry %q:\n%s",
					tt.url, tt.wantBody, b)
			}
			if tt.wantType != "" {
				if got := resp.Header.Get("Content-Type"); !strings.HasPrefix(got, tt.wantType) {
					t.Errorf("GET %s: Content-Type = %q, want prefix %q",
						tt.url, got, tt.wantType)
				}
			}
		})
	}
}

// TestAssetURLs covers the two generated ways to name an asset.
// Both are used in templates and both must produce the configured prefix.
func TestAssetURLs(t *testing.T) {
	if got, want := assets.URLPrefix, "/static/"; got != want {
		t.Errorf("assets.URLPrefix = %q, want %q", got, want)
	}
	if got, want := assets.Path("style.css"), "/static/style.css"; got != want {
		t.Errorf("assets.Path = %q, want %q", got, want)
	}
	if got, want := href.Asset("style.css"), "/static/style.css"; got != want {
		t.Errorf("href.Asset = %q, want %q", got, want)
	}
}

// TestAssetsEscapeTheirDirectory covers a path that climbs out of the embedded directory.
func TestAssetsEscapeTheirDirectory(t *testing.T) {
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
			if resp.StatusCode == http.StatusOK &&
				strings.Contains(string(b), "package app") {
				t.Errorf("GET %s served a file outside the assets directory", url)
			}
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
func TestDevModeServesFromDisk(t *testing.T) {
	// The generated code reads dev mode from the environment when the option is applied.
	// The variable is set before the server is built.
	t.Setenv("TEMPL_DEV_MODE", "true")
	if !datapagesgen.IsDevMode() {
		t.Fatal("the server does not consider this dev mode")
	}

	srv := httptest.NewServer(datapagesgen.NewServer(
		&app.App{}, inmem.New(8),
		datapagesgen.WithAssets(app.StaticFS),
		datapagesgen.WithPrometheus(datapagesgen.PrometheusConfig{
			Host:       "127.0.0.1:0",
			Registerer: registry,
			Gatherer:   registry,
		}),
	))
	t.Cleanup(srv.Close)

	resp := get(t, srv, href.Asset("style.css"))
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading the stylesheet: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200. The dev directory the generator "+
			"wrote is %q, relative to the module root", resp.StatusCode, assets.DevDir)
	}
	if !strings.Contains(string(b), "rebeccapurple") {
		t.Errorf("the file on disk was not served:\n%s", b)
	}
	if cc := resp.Header.Get("Cache-Control"); !strings.Contains(cc, "no-store") {
		t.Errorf("Cache-Control = %q, want it to forbid caching in dev mode", cc)
	}
}

// TestMetrics covers the instrumentation the Prometheus option adds.
// The counters are read from the registry the server was given,
// the same registry a scrape reads.
func TestMetrics(t *testing.T) {
	srv := newServer(t)

	resp := get(t, srv, "/")
	_ = resp.Body.Close()

	req, err := http.NewRequestWithContext(
		context.Background(), http.MethodPost, srv.URL+"/fail/", nil,
	)
	if err != nil {
		t.Fatalf("building request: %v", err)
	}
	req.Header.Set("Datastar-Request", "true")
	failResp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("POST /fail/: %v", err)
	}
	_ = failResp.Body.Close()

	families, err := registry.Gather()
	if err != nil {
		t.Fatalf("gathering metrics: %v", err)
	}
	byName := map[string]*dto.MetricFamily{}
	for _, f := range families {
		byName[f.GetName()] = f
	}

	reqs, ok := byName["datapages_http_requests_total"]
	if !ok {
		t.Fatal("the request counter was not registered")
	}
	var total float64
	for _, m := range reqs.GetMetric() {
		total += m.GetCounter().GetValue()
	}
	if total < 2 {
		t.Errorf("the request counter stands at %v after two requests", total)
	}

	if _, ok := byName["datapages_http_request_duration_seconds"]; !ok {
		t.Error("the request duration histogram was not registered")
	}
}

// TestBrokerMetrics covers the counters the generated code hands the message broker.
// They are what an operator watches to see events flowing,
// and they only move if the generated dispatch passes them along.
func TestBrokerMetrics(t *testing.T) {
	srv := newServer(t)

	// A subscriber has to exist: a message with nowhere to go is not
	// published, and nothing counts it.
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/_$/", nil)
	if err != nil {
		t.Fatalf("building stream request: %v", err)
	}
	req.Header.Set("Datastar-Request", "true")
	req.Header.Set("Accept-Encoding", "identity")
	stream, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("opening stream: %v", err)
	}
	t.Cleanup(func() { _ = stream.Body.Close() })
	if stream.StatusCode != http.StatusOK {
		t.Fatalf("opening stream: status %d", stream.StatusCode)
	}
	time.Sleep(200 * time.Millisecond)

	before := counterTotal(t, "datapages_event_broker_publishes_by_kind_total")

	announce, err := http.NewRequestWithContext(context.Background(),
		http.MethodPost, srv.URL+"/announce/", strings.NewReader(`{"text":"hi"}`))
	if err != nil {
		t.Fatalf("building announce request: %v", err)
	}
	announce.Header.Set("Datastar-Request", "true")
	announce.Header.Set("Content-Type", "application/json")
	resp, err := srv.Client().Do(announce)
	if err != nil {
		t.Fatalf("POST /announce/: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /announce/: status %d", resp.StatusCode)
	}

	if after := counterTotal(
		t, "datapages_event_broker_publishes_by_kind_total",
	); after <= before {
		t.Errorf("the publish counter stands at %v after a dispatch, was %v",
			after, before)
	}
}

// counterTotal sums every sample of a counter family in the registry.
func counterTotal(t *testing.T, name string) float64 {
	t.Helper()
	families, err := registry.Gather()
	if err != nil {
		t.Fatalf("gathering metrics: %v", err)
	}
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

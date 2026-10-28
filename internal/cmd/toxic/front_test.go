package toxic

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/internal/cmd/config"
	"github.com/romshark/datapages/runtime/httpserve"
)

// bodySize is the size of every response of the upstream of [startFront].
const bodySize = 4096

// startFront serves a [Front] with conf in front of an upstream that answers
// with bodySize bytes and the host and path it received in X-Host and X-Path.
// It returns the base URL of the front.
func startFront(t *testing.T, conf Config) (*Front, string) {
	t.Helper()
	return startFrontWith(t, conf, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Host", r.Host)
		w.Header().Set("X-Path", r.URL.Path)
		_, _ = io.WriteString(w, strings.Repeat("x", bodySize))
	})
}

// startFrontWith serves a [Front] with conf in front of an upstream served by h.
// It returns the base URL of the front.
func startFrontWith(t *testing.T, conf Config, h http.HandlerFunc) (*Front, string) {
	t.Helper()
	upstream := httptest.NewServer(h)
	t.Cleanup(upstream.Close)

	u, err := url.Parse(upstream.URL)
	require.NoError(t, err)
	conf.Upstream = u
	f, err := New(conf)
	require.NoError(t, err)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- f.Serve(ctx, ln) }()
	t.Cleanup(func() {
		cancel()
		require.NoError(t, <-done)
	})
	return f, "http://" + ln.Addr().String()
}

// get reads the whole response to a GET of target, or the error that ended it.
func get(t *testing.T, target string, host string) (*http.Response, string, error) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, target, nil)
	require.NoError(t, err)
	if host != "" {
		req.Host = host
	}
	c := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	return resp, string(b), err
}

// TestFrontRoutesByEndpoint tests that each request passes through the toxics
// of its endpoint, and that a reset of the connection to the app resets the one
// to the browser.
func TestFrontRoutesByEndpoint(t *testing.T) {
	_, base := startFront(t, Config{
		Endpoints: endpoints("GET /", "GET /limited/{$}", "GET /reset/{$}"),
		Settings: Settings{
			Global: Set{latencyDown: toxicWith(0, 0)},
			Endpoints: map[string]Set{
				"GET /limited/{$}": {limitDown: toxicWith(100)},
				"GET /reset/{$}":   {{Type: ResetPeer, Stream: Downstream}: toxicWith(0)},
			},
		},
	})

	resp, body, err := get(t, base+"/other/", "app.test")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Len(t, body, bodySize)
	require.Equal(t, "app.test", resp.Header.Get("X-Host"),
		"the app sees the host of the browser")

	_, body, err = get(t, base+"/limited/", "")
	require.Error(t, err, "the response was cut after %d bytes", len(body))
	require.Less(t, len(body), bodySize)

	_, _, err = get(t, base+"/reset/", "")
	require.Error(t, err)
}

// TestFrontHTTPError tests that the http-error toxic answers as
// [httpserve.WriteErrStatus] does, upstream before the app runs and downstream
// after it finishes.
func TestFrontHTTPError(t *testing.T) {
	want := httptest.NewRecorder()
	httpserve.WriteErrStatus(want, errors.New("boom"))

	for name, tc := range map[string]struct {
		slot    Slot
		wantRan bool
	}{
		"upstream":   {slot: errorUp},
		"downstream": {slot: errorDown, wantRan: true},
	} {
		t.Run(name, func(t *testing.T) {
			var ran atomic.Bool
			_, base := startFrontWith(t, Config{
				Endpoints: endpoints("GET /"),
				Settings:  Settings{Global: Set{tc.slot: toxicWith(500)}},
			}, func(w http.ResponseWriter, _ *http.Request) {
				_ = http.NewResponseController(w).Flush()
				time.Sleep(50 * time.Millisecond)
				ran.Store(true)
				_, _ = io.WriteString(w, "done")
			})

			resp, body, err := get(t, base+"/", "")
			require.NoError(t, err)
			require.Equal(t, want.Code, resp.StatusCode)
			require.Equal(t, want.Body.String(), body)
			require.Equal(t, want.Header().Get("Content-Type"),
				resp.Header.Get("Content-Type"))
			require.Equal(t, tc.wantRan, ran.Load())
		})
	}
}

// TestFrontHTTPErrorScope tests that an http-error override applies to its
// endpoint only, and that a toxicity of 0 passes every request.
func TestFrontHTTPErrorScope(t *testing.T) {
	never := toxicWith(500)
	never.Toxicity = 0
	_, base := startFront(t, Config{
		Endpoints: endpoints("GET /", "GET /fail/{$}", "GET /never/{$}"),
		Settings: Settings{Endpoints: map[string]Set{
			"GET /fail/{$}":  {errorUp: toxicWith(502)},
			"GET /never/{$}": {errorUp: never},
		}},
	})

	resp, _, err := get(t, base+"/fail/", "")
	require.NoError(t, err)
	require.Equal(t, http.StatusBadGateway, resp.StatusCode)
	for _, path := range []string{"/", "/never/"} {
		resp, body, err := get(t, base+path, "")
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode, path)
		require.Len(t, body, bodySize, path)
	}
}

// TestFrontHTTPErrorStream tests that a downstream http-error toxic answers
// the stream of a page at its head and ends the stream on the server.
func TestFrontHTTPErrorStream(t *testing.T) {
	ended := make(chan struct{})
	_, base := startFrontWith(t, Config{
		Endpoints: []Endpoint{
			{Pattern: "GET /_$/{$}", Owner: "PageIndex", Handler: "GETStream"},
		},
		Settings: Settings{Global: Set{errorDown: toxicWith(500)}},
	}, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_ = http.NewResponseController(w).Flush()
		<-r.Context().Done()
		close(ended)
	})

	resp, _, err := get(t, base+"/_$/", "")
	require.NoError(t, err)
	require.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	select {
	case <-ended:
	case <-time.After(5 * time.Second):
		t.Fatal("the stream did not end on the server")
	}
}

// TestFrontOffline tests that Offline fails the requests of every endpoint,
// overrides included, and that the preset applied after it brings its
// overrides back.
func TestFrontOffline(t *testing.T) {
	limited := map[string]Set{"GET /limited/{$}": {limitDown: toxicWith(100)}}
	f, base := startFront(t, Config{
		Endpoints: endpoints("GET /", "GET /limited/{$}"),
		Settings: Settings{Presets: []Preset{
			{Name: "Limited", Toxics: Set{}, Endpoints: limited},
		}},
	})
	apply := func(name string) {
		t.Helper()
		require.NoError(t, f.Update(func(s *Settings) error { return s.ApplyPreset(name) }))
	}
	apply(config.PresetOffline)
	for _, path := range []string{"/", "/limited/"} {
		_, _, err := get(t, base+path, "")
		require.Error(t, err, path)
	}

	apply("Limited")
	_, body, err := get(t, base+"/", "")
	require.NoError(t, err)
	require.Len(t, body, bodySize)
	_, body, err = get(t, base+"/limited/", "")
	require.Error(t, err, "the response was cut after %d bytes", len(body))
}

// TestFrontPassesDevServerEvents tests that the reload connection of the dev
// server skips the toxics.
func TestFrontPassesDevServerEvents(t *testing.T) {
	_, base := startFront(t, Config{
		Endpoints: endpoints("GET /"),
		Settings: Settings{Global: Set{
			{Type: ResetPeer, Stream: Downstream}: toxicWith(0),
		}},
	})
	_, _, err := get(t, base+"/", "")
	require.Error(t, err, "the global toxic applies")

	resp, body, err := get(t, base+config.TemplierEventsPath, "")
	require.NoError(t, err)
	require.Equal(t, config.TemplierEventsPath, resp.Header.Get("X-Path"))
	require.Len(t, body, bodySize)
}

// TestFrontReloadsEndpoints tests that a change of GenFile reloads the
// endpoints and calls EndpointsChanged.
func TestFrontReloadsEndpoints(t *testing.T) {
	genFile := filepath.Join(t.TempDir(), "app_gen.go")
	require.NoError(t, os.WriteFile(genFile, nil, 0o644))
	changed := make(chan struct{}, 1)
	f, _ := startFront(t, Config{
		Endpoints: endpoints("GET /"),
		GenFile:   genFile,
		LoadEndpoints: func() ([]Endpoint, error) {
			return endpoints("GET /", "GET /new/{$}"), nil
		},
		EndpointsChanged: func() { changed <- struct{}{} },
	})
	match := func() string {
		return f.routes.Load().match(httptest.NewRequest("GET", "/new/", nil)).Pattern
	}
	require.Equal(t, "GET /", match())

	later := time.Now().Add(time.Minute)
	require.NoError(t, os.Chtimes(genFile, later, later))
	select {
	case <-changed:
	case <-time.After(5 * time.Second):
		t.Fatal("the endpoints were not reloaded")
	}
	require.Equal(t, "GET /new/{$}", match())
}

// TestFrontSectionHost tests that the saved section holds the addresses only
// where datapages.yaml sets them.
func TestFrontSectionHost(t *testing.T) {
	for name, set := range map[string]bool{"set": true, "default": false} {
		t.Run(name, func(t *testing.T) {
			f, _ := startFront(t, Config{
				Host: "localhost:7332", HostSet: set,
				UIHost: "localhost:7333", UIHostSet: set,
			})
			section := f.section(0)
			require.Equal(t, set,
				strings.Contains(section, "host: \"localhost:7332\"\n"), section)
			require.Equal(t, set,
				strings.Contains(section, "ui-host: \"localhost:7333\"\n"), section)
		})
	}
}

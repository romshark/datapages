package httpserve_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/runtime/httpserve"
)

// echoPath answers with the path it was routed with.
func echoPath() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(r.URL.Path))
	})
	return mux
}

func serve(t *testing.T, c *httpserve.Core, path string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	c.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	return w
}

// TestServeHTTPNormalizesPath tests the trailing slash every page path gets
// before the router sees it. One canonical form per page keeps the route
// patterns and the generated href helpers from having to agree on two.
func TestServeHTTPNormalizesPath(t *testing.T) {
	t.Parallel()

	c := mustCore(t, datapages.ServerConfig{}, "")
	c.Mux().HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(r.URL.Path))
	})
	c.Build()

	for path, want := range map[string]string{
		"/":       "/",
		"/page":   "/page/",
		"/page/":  "/page/",
		"/a/b":    "/a/b/",
		"/page?x": "/page/",
	} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, want, serve(t, c, path).Body.String())
		})
	}
}

// TestServeHTTPKeepsAssetPaths tests the exception: a static file path keeps
// the name it has on disk.
func TestServeHTTPKeepsAssetPaths(t *testing.T) {
	t.Parallel()

	c := mustCore(t, datapages.ServerConfig{}, "/static/")
	c.Mux().HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(r.URL.Path))
	})
	c.Build()

	require.Equal(t, "/static/style.css", serve(t, c, "/static/style.css").Body.String())
	require.Equal(t, "/page/", serve(t, c, "/page").Body.String())
}

// TestMiddlewareOrder tests that middleware runs in the order it was added,
// and that the outermost one wraps all of them.
func TestMiddlewareOrder(t *testing.T) {
	t.Parallel()

	var order []string
	tag := func(name string) func(http.Handler) http.Handler {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, name)
				next.ServeHTTP(w, r)
			})
		}
	}

	c := mustCore(t, datapages.ServerConfig{
		Middleware: []func(http.Handler) http.Handler{
			tag("first"), tag("second"),
		},
		OutermostMiddleware: tag("outermost"),
	}, "")
	c.Mux().HandleFunc("/", func(http.ResponseWriter, *http.Request) {
		order = append(order, "handler")
	})
	c.Build()

	serve(t, c, "/")
	require.Equal(t, []string{"outermost", "first", "second", "handler"}, order)
}

// TestLoggerBeforeBuild tests that a Core logs as soon as NewCore returns,
// since a caller may log before it calls Build.
func TestLoggerBeforeBuild(t *testing.T) {
	t.Parallel()

	c := mustCore(t, datapages.ServerConfig{}, "")
	require.NotNil(t, c.Logger())
	require.NotPanics(t, func() {
		c.LogErr("before build", errors.New("x"))
	})
}

// TestBuildDefaults tests what a core built from an empty config carries: a logger,
// the bundled Datastar script in the HTML prefix, and no TLS, metrics or
// assets until an option asks for them.
func TestBuildDefaults(t *testing.T) {
	t.Parallel()

	c := mustCore(t, datapages.ServerConfig{}, "")
	c.Build()

	require.NotNil(t, c.Logger())
	require.Contains(t, c.HTMLPrefix(), httpserve.DefaultDatastarJSSrc)
	require.True(t, strings.HasPrefix(c.HTMLPrefix(), "<!DOCTYPE html>"))
	require.False(t, c.TLSEnabled())
	require.False(t, c.MetricsEnabled())
	require.Nil(t, c.AssetsFS())
}

// TestDatastarJSOption tests that a custom script URL replaces the bundled one
// in the HTML prefix, which is where every page picks it up.
func TestDatastarJSOption(t *testing.T) {
	t.Parallel()

	c := mustCore(t, datapages.ServerConfig{
		DatastarJS: "/static/ds.js",
	}, "")
	c.Build()

	require.Contains(t, c.HTMLPrefix(), `src="/static/ds.js"`)
}

// TestShutdownEndsListenAndServe tests that the context ends the server
// and that ShutdownCh reports it.
func TestShutdownEndsListenAndServe(t *testing.T) {
	t.Parallel()

	c := mustCore(t, datapages.ServerConfig{}, "")
	c.Mux().Handle("/", echoPath())
	c.Build()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- c.ListenAndServe(ctx, "127.0.0.1:0") }()

	select {
	case <-c.ShutdownCh():
		t.Fatal("the shutdown channel closed before the shutdown")
	case <-time.After(50 * time.Millisecond):
	}

	cancel()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("ListenAndServe did not return")
	}
	<-c.ShutdownCh()
}

// TestShutdownDrainsInFlightRequest tests the request context during a
// graceful shutdown. Shutdown waits for the handler instead of cancelling it.
func TestShutdownDrainsInFlightRequest(t *testing.T) {
	t.Parallel()

	entered := make(chan struct{})
	release := make(chan struct{})
	cancelled := make(chan error, 1)

	c := mustCore(t, datapages.ServerConfig{}, "")
	// Simulate a slow handler: report that the request arrived, then hold it until the
	// test releases it. Report a cancellation instead if the request context ends first,
	// which is what a handler doing a database call or an outbound request sees.
	c.Mux().HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		select {
		case <-r.Context().Done():
			cancelled <- r.Context().Err()
		case <-release:
		}
		_, _ = w.Write([]byte("done"))
	})
	c.Build()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	served := make(chan error, 1)
	go func() { served <- c.ListenAndServe(ctx, "127.0.0.1:0") }()

	// The listener is open once Addr reports the port the kernel chose.
	require.Eventually(t, func() bool { return c.Addr() != "" },
		5*time.Second, time.Millisecond)

	type response struct {
		body string
		err  error
	}
	got := make(chan response, 1)
	go func() {
		resp, err := http.Get("http://" + c.Addr() + "/")
		if err != nil {
			got <- response{err: err}
			return
		}
		defer func() { _ = resp.Body.Close() }()
		b, err := io.ReadAll(resp.Body)
		got <- response{body: string(b), err: err}
	}()

	<-entered
	// Simulate a signal handler cancelling the context while
	// the request is still in the handler.
	cancel()

	// The client must get no answer while the handler still holds the request.
	select {
	case <-time.After(time.Second):
	case r := <-got:
		t.Fatalf("the request ended before it was released: %q, %v", r.body, r.err)
	}
	// The request context must stay alive.
	// A handler that reads it would give up its own work.
	select {
	case err := <-cancelled:
		t.Fatalf("the request context was cancelled: %v", err)
	default:
	}

	// The handler answers, and ListenAndServe returns once Shutdown has drained it.
	close(release)
	r := <-got
	require.NoError(t, r.err)
	require.Equal(t, "done", r.body)
	require.NoError(t, <-served)
}

// TestBuildServesAssets tests that an assets file system plus a URL prefix
// reach the router as a file server.
func TestBuildServesAssets(t *testing.T) {
	t.Parallel()

	c := mustCore(t, datapages.ServerConfig{
		AssetsFS: http.Dir("testdata/static"),
	}, "/static/")
	c.Build()

	w := serve(t, c, "/static/hello.txt")
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "hello", strings.TrimSpace(w.Body.String()))
}

// TestBuildWithoutAssetsPrefix tests an assets file system given without a URL prefix.
// Nothing is routed to it: serving it under a guessed prefix would expose
// files the application never asked to publish.
func TestBuildWithoutAssetsPrefix(t *testing.T) {
	t.Parallel()

	c := mustCore(t, datapages.ServerConfig{
		AssetsFS: http.Dir("testdata/static"),
	}, "")
	c.Build()

	require.Equal(t, http.StatusNotFound, serve(t, c, "/static/hello.txt").Code)
}

// TestWildcardPathValue tests the value a {name...} route wildcard hands a
// handler after [httpserve.Core.ServeHTTP] normalized the path.
func TestWildcardPathValue(t *testing.T) {
	for name, tt := range map[string]struct{ raw, want string }{
		"one segment":       {"a", "a"},
		"trailing slash":    {"a/", "a"},
		"several segments":  {"a/b/c", "a/b/c"},
		"segments slash":    {"a/b/c/", "a/b/c"},
		"empty":             {"", ""},
		"slash only":        {"/", ""},
		"inner slash kept":  {"a//b/", "a//b"},
		"one slash trimmed": {"a//", "a/"},
	} {
		t.Run(name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.SetPathValue("rest", tt.raw)
			require.Equal(t, tt.want, httpserve.WildcardPathValue(r, "rest"))
		})
	}
}

// mustCore builds a core and fails the test on a configuration error,
// which no assertion can carry on from.
func mustCore(
	t *testing.T, cfg datapages.ServerConfig, assetsURLPrefix string,
) *httpserve.Core {
	t.Helper()
	c, err := httpserve.NewCore(cfg, assetsURLPrefix)
	require.NoError(t, err)
	return c
}

package httpserve_test

import (
	"context"
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

func TestServeHTTPNormalizesPath(t *testing.T) {
	t.Parallel()

	c := httpserve.NewCore(datapages.ServerConfig{}, "")
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

// TestServeHTTPKeepsAssetPaths covers the exception: a static file path keeps
// the name it has on disk.
func TestServeHTTPKeepsAssetPaths(t *testing.T) {
	t.Parallel()

	c := httpserve.NewCore(datapages.ServerConfig{}, "/static/")
	c.Mux().HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(r.URL.Path))
	})
	c.Build()

	require.Equal(t, "/static/style.css", serve(t, c, "/static/style.css").Body.String())
	require.Equal(t, "/page/", serve(t, c, "/page").Body.String())
}

// TestMiddlewareOrder covers that middleware runs in the order it was added,
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

	c := httpserve.NewCore(datapages.ServerConfig{
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

func TestBuildDefaults(t *testing.T) {
	t.Parallel()

	c := httpserve.NewCore(datapages.ServerConfig{}, "")
	c.Build()

	require.NotNil(t, c.Logger())
	require.Contains(t, c.HTMLPrefix(), httpserve.DefaultDatastarJSSrc)
	require.True(t, strings.HasPrefix(c.HTMLPrefix(), "<!DOCTYPE html>"))
	require.False(t, c.TLSEnabled())
	require.False(t, c.MetricsEnabled())
	require.Nil(t, c.AssetsFS())
}

func TestDatastarJSOption(t *testing.T) {
	t.Parallel()

	c := httpserve.NewCore(datapages.ServerConfig{
		DatastarJS: "/static/ds.js",
	}, "")
	c.Build()

	require.Contains(t, c.HTMLPrefix(), `src="/static/ds.js"`)
}

// TestShutdownEndsListenAndServe covers that the context ends the server
// and that ShutdownCh reports it.
func TestShutdownEndsListenAndServe(t *testing.T) {
	t.Parallel()

	c := httpserve.NewCore(datapages.ServerConfig{}, "")
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

func TestBuildServesAssets(t *testing.T) {
	t.Parallel()

	c := httpserve.NewCore(datapages.ServerConfig{
		AssetsFS: http.Dir("testdata/static"),
	}, "/static/")
	c.Build()

	w := serve(t, c, "/static/hello.txt")
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "hello", strings.TrimSpace(w.Body.String()))
}

func TestBuildWithoutAssetsPrefix(t *testing.T) {
	t.Parallel()

	c := httpserve.NewCore(datapages.ServerConfig{
		AssetsFS: http.Dir("testdata/static"),
	}, "")
	c.Build()

	require.Equal(t, http.StatusNotFound, serve(t, c, "/static/hello.txt").Code)
}

// TestWildcardPathValue covers the value a {name...} route wildcard hands a
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

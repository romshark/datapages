// Tests the return values of a GET handler other than its body.

package acceptance_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/internal/acceptance/getoptions/app"
	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/messaging/inmem"
)

// reloadAttr is what the server writes on the body so that a tab reloads the
// page when it becomes visible again. The two streaming flags exist to suppress it.
const reloadAttr = "data-on:visibilitychange"

func newServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(mustNewServer(t, &app.App{},
		inmem.New(messaging.DefaultBrokerChanBuffer)))
	t.Cleanup(srv.Close)
	return srv
}

// get performs a page load without following redirects.
func get(t *testing.T, srv *httptest.Server, path string) (*http.Response, string) {
	t.Helper()
	client := *srv.Client()
	client.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	req, err := http.NewRequestWithContext(
		context.Background(), http.MethodGet, srv.URL+path, nil,
	)
	require.NoError(t, err, "building GET %s", path)
	req.Header.Set("Accept-Encoding", "identity")
	resp, err := client.Do(req)
	require.NoError(t, err, "GET %s", path)
	b, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	require.NoError(t, err, "reading %s", path)
	return resp, string(b)
}

// TestRedirectFromPageLoad tests a GET that returns a redirect.
// The visitor is sent elsewhere and the body the handler also returned is not rendered.
func TestRedirectFromPageLoad(t *testing.T) {
	t.Parallel()
	srv := newServer(t)

	resp, body := get(t, srv, "/gone/")
	require.Equal(t, 3, resp.StatusCode/100,
		"status = %d, want a redirect\n%s", resp.StatusCode, body)
	require.Equal(t, "/", resp.Header.Get("Location"))
	require.NotContains(t, body, "never rendered",
		"the body was rendered alongside the redirect")
}

// TestRedirectStatusFromPageLoad tests a handler that chooses the status,
// and the same handler when it decides not to redirect at all.
func TestRedirectStatusFromPageLoad(t *testing.T) {
	t.Parallel()
	srv := newServer(t)

	t.Run("redirecting", func(t *testing.T) {
		resp, _ := get(t, srv, "/maybe/?go=true")
		require.Equal(t, http.StatusMovedPermanently, resp.StatusCode)
		require.Equal(t, "/", resp.Header.Get("Location"))
	})

	t.Run("not redirecting", func(t *testing.T) {
		resp, body := get(t, srv, "/maybe/")
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.Contains(t, body, "stayed", "the page did not render")
		require.Empty(t, resp.Header.Get("Location"),
			"a page that did not redirect sent a Location header")
	})
}

// TestVisibilityReload tests the two flags that decide whether a hidden tab
// reloads the page when it comes back.
//
// Both suppress the same body attribute. A page that keeps its stream running
// in the background must not reload, and a page that asks not to reload must not either.
func TestVisibilityReload(t *testing.T) {
	t.Parallel()
	srv := newServer(t)

	tests := map[string]struct {
		path       string
		wantReload bool
	}{
		"the plain page reloads":             {"/", true},
		"background streaming suppresses it": {"/background/", false},
		"disabled refresh suppresses it":     {"/no-refresh/", false},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			resp, body := get(t, srv, tt.path)
			require.Equal(t, http.StatusOK, resp.StatusCode)
			require.Equal(t, tt.wantReload, strings.Contains(body, reloadAttr),
				"the page carries %s\n%s", reloadAttr, body)
		})
	}
}

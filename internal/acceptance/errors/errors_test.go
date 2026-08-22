// Drives the generated error handling of ./app.

package acceptance_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/internal/acceptance/client"
	"github.com/romshark/datapages/internal/acceptance/errors/app"
	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/messaging/inmem"
)

func newClient(t *testing.T) *client.Client {
	t.Helper()
	return client.New(t, mustNewServer(t, &app.App{},
		inmem.New(messaging.DefaultBrokerChanBuffer)))
}

// TestNotFoundPage covers the page an app supplies for URLs nothing claims,
// reached both ways: by such a URL and by its own route.
func TestNotFoundPage(t *testing.T) {
	tests := map[string]string{
		"unknown url":                "/no-such-page/",
		"the error page's own route": "/not-found/",
	}

	for name, path := range tests {
		t.Run(name, func(t *testing.T) {
			c := newClient(t)
			resp := c.Get(t, path)
			require.Equal(t, "not found: "+path, resp.Element(t, "echo"))
		})
	}
}

// TestServerErrorPageRoute covers the 500 page reached by its own route,
// which is served like any other page. The error500page case covers what a
// failed page load is answered with.
func TestServerErrorPageRoute(t *testing.T) {
	c := newClient(t)

	resp := c.Get(t, "/server-error/")
	require.Equal(t, http.StatusOK, resp.Status)
	require.Equal(t, "server error", resp.Element(t, "echo"))
}

// TestFailedPageLoad covers a page load whose handler fails.
// Whatever the visitor is given, it cannot be the error the handler produced:
// that text is written for the operator's log.
func TestFailedPageLoad(t *testing.T) {
	c := newClient(t)

	resp := c.Get(t, "/boom/")
	require.Equal(t, http.StatusInternalServerError, resp.Status)
	require.NotContains(t, resp.Body, "the page could not be built",
		"the error message reached the visitor")
}

// TestActionErrorStatus covers the status an action's error becomes.
// The sentinels are the only way an application chooses it,
// and each one must arrive as itself.
func TestActionErrorStatus(t *testing.T) {
	tests := map[string]struct {
		path string
		want int
	}{
		"plain error":      {"/boom/plain/", http.StatusInternalServerError},
		"bad request":      {"/boom/bad/", http.StatusBadRequest},
		"forbidden":        {"/boom/forbidden/", http.StatusForbidden},
		"not found":        {"/boom/not-found/", http.StatusNotFound},
		"conflict":         {"/boom/conflict/", http.StatusConflict},
		"wrapped sentinel": {"/boom/wrapped/", http.StatusNotFound},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			c := newClient(t)
			resp := c.Action(t, http.MethodPost, tt.path, "")
			require.Equal(t, tt.want, resp.Status, resp.Body)
			for _, leaked := range []string{"no such item", "something went wrong"} {
				require.NotContains(t, resp.Body, leaked,
					"the error message reached the client")
			}
		})
	}
}

// TestFailedPageLoadIsNotCached covers the response of a failed page load.
// A cached 500 outlives the failure that caused it.
func TestFailedPageLoadIsNotCached(t *testing.T) {
	c := newClient(t)

	resp := c.Get(t, "/boom/")

	cc := resp.Header.Get("Cache-Control")
	if strings.Contains(cc, "max-age") {
		require.Contains(t, cc, "max-age=0",
			"Cache-Control = %q on a failed page load", cc)
	}
}

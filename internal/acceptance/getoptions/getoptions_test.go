// Tests the return values of a GET handler other than its body.

package acceptance_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/datapagestest"
	"github.com/romshark/datapages/internal/acceptance/getoptions/app"
	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/messaging/inmem"
)

// reloadAttr is what the server writes on the body of a page with a stream so
// that a tab reloads it when it becomes visible again. The 2 streaming flags
// exist to suppress it.
const reloadAttr = "data-on:visibilitychange"

func serve(t *testing.T) *datapagestest.Server {
	t.Helper()
	return datapagestest.New(t, mustNewServer(t, &app.App{},
		inmem.New(messaging.DefaultBrokerChanBuffer)))
}

// TestRedirectFromPageLoad tests a GET that returns a redirect.
// The visitor is sent elsewhere and the body the handler also returned is not rendered.
func TestRedirectFromPageLoad(t *testing.T) {
	t.Parallel()
	v := serve(t).Visitor(t)

	resp := v.Get(t, "/gone/")
	require.Equal(t, 3, resp.Status/100,
		"status = %d, want a redirect\n%s", resp.Status, resp.Body)
	require.Equal(t, "/", resp.Redirect(t))
	require.NotContains(t, resp.Body, "never rendered",
		"the body was rendered alongside the redirect")
}

// TestRedirectStatusFromPageLoad tests a handler that chooses the status,
// and the same handler when it decides not to redirect at all.
func TestRedirectStatusFromPageLoad(t *testing.T) {
	t.Parallel()
	v := serve(t).Visitor(t)

	t.Run("redirecting", func(t *testing.T) {
		resp := v.Get(t, "/maybe/?go=true")
		require.Equal(t, http.StatusMovedPermanently, resp.Status)
		require.Equal(t, "/", resp.Redirect(t))
	})

	t.Run("not redirecting", func(t *testing.T) {
		resp := v.Get(t, "/maybe/")
		require.Equal(t, http.StatusOK, resp.Status)
		require.Contains(t, resp.Body, "stayed", "the page did not render")
		require.Empty(t, resp.Header.Get("Location"),
			"a page that did not redirect sent a Location header")
	})
}

// TestVisibilityReload tests what decides whether a hidden tab reloads the
// page when it comes back.
//
// The reload renders the events the closed stream missed. A page without a
// stream misses none. A page that keeps its stream running in the background
// must not reload, and a page that asks not to reload must not either.
func TestVisibilityReload(t *testing.T) {
	t.Parallel()
	v := serve(t).Visitor(t)

	tests := map[string]struct {
		path       string
		wantReload bool
	}{
		"a page with a stream reloads":       {"/live/", true},
		"a page without a stream does not":   {"/", false},
		"background streaming suppresses it": {"/background/", false},
		"disabled refresh suppresses it":     {"/no-refresh/", false},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			resp := v.Get(t, tt.path)
			require.Equal(t, http.StatusOK, resp.Status)
			require.Equal(t, tt.wantReload, strings.Contains(resp.Body, reloadAttr),
				"the page carries %s\n%s", reloadAttr, resp.Body)
		})
	}
}

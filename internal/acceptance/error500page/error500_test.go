// Asserts that a failed page load renders the app's own 500 page.

package acceptance_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/internal/acceptance/client"
	"github.com/romshark/datapages/internal/acceptance/error500page/app"
	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/messaging/inmem"
	"github.com/romshark/datapages/modules/offline"
)

func newClient(t *testing.T) *client.Client {
	t.Helper()
	return client.New(t, mustNewServer(t, &app.App{},
		inmem.New(messaging.DefaultBrokerChanBuffer)))
}

// TestError500PageIsRendered tests a failed page load in an app that supplies
// PageError500 and defines no RecoverError.
func TestError500PageIsRendered(t *testing.T) {
	t.Parallel()
	c := newClient(t)

	// The page renders on its own route, which rules the page itself out as
	// the reason a failed load might not show it.
	own := c.Get(t, "/server-error/")
	require.Contains(t, own.Body, `<p id="msg">`,
		"the 500 page does not render on its own route")

	resp := c.Get(t, "/boom/")
	require.Equal(t, http.StatusInternalServerError, resp.Status)
	require.Contains(t, resp.Body, "something went wrong on our side",
		"a failed page load did not render PageError500")
	require.NotContains(t, resp.Body, "the page could not be built",
		"the error message reached the visitor")
}

// TestSentinelPageLoadSkipsError500Page tests a page load failing with ErrNotFound
// in an app that supplies PageError500 and no PageError404. PageError500 stands
// for 500 only: the visitor gets 404 and its status text.
func TestSentinelPageLoadSkipsError500Page(t *testing.T) {
	t.Parallel()
	c := newClient(t)

	resp := c.Get(t, "/gone/")
	require.Equal(t, http.StatusNotFound, resp.Status, resp.Body)
	require.Equal(t, http.StatusText(http.StatusNotFound)+"\n", resp.Body)
}

// TestPanicAfterThePageStarted tests a page body that panics after it wrote
// part of the page. The visitor gets the truncated page under 200, the status
// already sent, and no 500 page after it. The offline module holds the page
// back until the handler returns, and must answer the same.
func TestPanicAfterThePageStarted(t *testing.T) {
	t.Parallel()
	for name, opts := range map[string][]datapages.ServerOption{
		"plain": nil,
		"offline": {
			offline.WithServiceWorker("", offline.Config{WorkerVersion: 1}),
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			c := client.New(t, mustNewServer(t, &app.App{},
				inmem.New(messaging.DefaultBrokerChanBuffer), opts...))

			resp := c.Get(t, "/half/")
			require.Equal(t, http.StatusOK, resp.Status)
			require.Contains(t, resp.Body, `<p id="half">half</p>`)
			require.Equal(t, 1, strings.Count(resp.Body, "<!DOCTYPE html>"),
				"a second document follows the page:\n%s", resp.Body)
			require.NotContains(t, resp.Body, "something went wrong on our side")
		})
	}
}

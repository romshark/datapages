// Asserts that a failed page load renders the app's own 500 page.

package acceptance_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/internal/acceptance/client"
	"github.com/romshark/datapages/internal/acceptance/error500page/app"
	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/messaging/inmem"
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

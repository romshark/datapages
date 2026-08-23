// Asserts what a failed page load gets when PageError500 fails as well.

package acceptance_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/internal/acceptance/client"
	"github.com/romshark/datapages/internal/acceptance/error500failing/app"
	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/messaging/inmem"
)

func newClient(t *testing.T) *client.Client {
	t.Helper()
	return client.New(t, mustNewServer(t, &app.App{},
		inmem.New(messaging.DefaultBrokerChanBuffer)))
}

// TestFailingError500PageStillAnswers covers a page load that fails and an
// error page that fails reporting it.
//
// The error page reports through the handler that renders the error page.
// Answering the second failure the way the first one was answered runs that
// pair until the stack is gone, which takes the process with it rather than
// the request. The server has to stop at the plain status instead.
func TestFailingError500PageStillAnswers(t *testing.T) {
	c := newClient(t)

	resp := c.Get(t, "/boom/")
	require.Equal(t, http.StatusInternalServerError, resp.Status)
	require.Contains(t, resp.Body, http.StatusText(http.StatusInternalServerError),
		"the response carries no status text to show")
	require.NotContains(t, resp.Body, "could not be built",
		"an error message reached the visitor")
}

// TestFailingError500PageOnItsOwnRoute covers the same page requested
// directly, where nothing failed before it.
func TestFailingError500PageOnItsOwnRoute(t *testing.T) {
	c := newClient(t)

	resp := c.Get(t, "/server-error/")
	require.Equal(t, http.StatusInternalServerError, resp.Status)
	require.NotContains(t, resp.Body, "could not be built",
		"an error message reached the visitor")
}

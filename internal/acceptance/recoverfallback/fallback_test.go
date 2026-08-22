// Asserts what a request gets when RecoverError itself fails.

package acceptance_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/internal/acceptance/client"
	"github.com/romshark/datapages/internal/acceptance/recoverfallback/app"
	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/messaging/inmem"
)

func newClient(t *testing.T) *client.Client {
	t.Helper()
	return client.New(t, mustNewServer(t, &app.App{},
		inmem.New(messaging.DefaultBrokerChanBuffer)))
}

// TestRecoverFallbackWritesNothingIntoTheStream covers an action whose error
// RecoverError could not turn into a patch. The stream ends and carries no status text;
// the response was committed before the failure.
func TestRecoverFallbackWritesNothingIntoTheStream(t *testing.T) {
	c := newClient(t)

	resp := c.Action(t, http.MethodPost, "/bad/", "")

	require.NotContains(t, resp.Body, "Bad Request",
		"the status text was written into the event stream")
	require.NotContains(t, resp.Body, "cannot render a toast for this",
		"the error of RecoverError reached the client")
	require.NotContains(t, resp.Body, "Internal Server Error",
		"a status text was written into the event stream")
}

// TestPageLoadStillGetsAStatus covers the same failure on a page load,
// where nothing is committed yet and the status is the server's to send.
func TestPageLoadStillGetsAStatus(t *testing.T) {
	c := newClient(t)

	resp := c.Get(t, "/bad/")
	require.NotEqual(t, http.StatusOK, resp.Status,
		"a failed page load was answered with 200")
}

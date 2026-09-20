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

// newPanickingClient returns a client of a server whose error page panics
// instead of returning an error.
func newPanickingClient(t *testing.T) *client.Client {
	t.Helper()
	return client.New(t, mustNewServer(t, &app.App{Error500Panics: true},
		inmem.New(messaging.DefaultBrokerChanBuffer)))
}

// TestFailingError500PageStillAnswers tests a page load that fails and an
// error page that fails reporting it.
//
// The error page reports through the handler that renders the error page.
// Answering the second failure the way the first one was answered runs that
// pair until the stack is gone, which takes the process with it rather than
// the request. The server has to stop at the plain status instead.
func TestFailingError500PageStillAnswers(t *testing.T) {
	t.Parallel()
	c := newClient(t)

	resp := c.Get(t, "/boom/")
	require.Equal(t, http.StatusInternalServerError, resp.Status)
	require.Contains(t, resp.Body, http.StatusText(http.StatusInternalServerError),
		"the response carries no status text to show")
	require.NotContains(t, resp.Body, "could not be built",
		"an error message reached the visitor")
}

// TestFailingError500PageOnItsOwnRoute tests the same page requested
// directly, where nothing failed before it.
func TestFailingError500PageOnItsOwnRoute(t *testing.T) {
	t.Parallel()
	c := newClient(t)

	resp := c.Get(t, "/server-error/")
	require.Equal(t, http.StatusInternalServerError, resp.Status)
	require.NotContains(t, resp.Body, "could not be built",
		"an error message reached the visitor")
}

// TestPanickingPage tests a page that panics while the error page returns an
// error, which is the path a recovered panic takes into the error page.
func TestPanickingPage(t *testing.T) {
	t.Parallel()
	c := newClient(t)

	resp := c.Get(t, "/panic/")
	require.Equal(t, http.StatusInternalServerError, resp.Status)
	require.NotContains(t, resp.Body, "the page panicked",
		"the panic value reached the visitor")
}

// TestPanickingError500PageStillAnswers tests a failed page load answered by
// an error page that panics.
//
// A panic in the error page takes the deferred recovery, which hands it to the
// handler that renders the error page. That page panics again, and the pair
// runs until the stack is gone and the process with it. The server has to stop
// at the plain status instead.
func TestPanickingError500PageStillAnswers(t *testing.T) {
	t.Parallel()
	c := newPanickingClient(t)

	resp := c.Get(t, "/boom/")
	require.Equal(t, http.StatusInternalServerError, resp.Status)
	require.Contains(t, resp.Body, http.StatusText(http.StatusInternalServerError),
		"the response carries no status text to show")
	require.NotContains(t, resp.Body, "the error page panicked",
		"the panic value reached the visitor")
}

// TestPanickingError500PageAfterPanickingPage tests the same pair reached from
// a page that panics rather than one that returns an error.
func TestPanickingError500PageAfterPanickingPage(t *testing.T) {
	t.Parallel()
	c := newPanickingClient(t)

	resp := c.Get(t, "/panic/")
	require.Equal(t, http.StatusInternalServerError, resp.Status)
	require.Contains(t, resp.Body, http.StatusText(http.StatusInternalServerError),
		"the response carries no status text to show")
}

// TestPanickingError500PageOnItsOwnRoute tests the panicking page requested
// directly, where nothing failed before it.
func TestPanickingError500PageOnItsOwnRoute(t *testing.T) {
	t.Parallel()
	c := newPanickingClient(t)

	resp := c.Get(t, "/server-error/")
	require.Equal(t, http.StatusInternalServerError, resp.Status)
	require.NotContains(t, resp.Body, "the error page panicked",
		"the panic value reached the visitor")
}

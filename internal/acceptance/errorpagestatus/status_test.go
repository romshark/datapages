// Asserts that a rendered error page carries the status it stands for.

package acceptance_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/internal/acceptance/client"
	"github.com/romshark/datapages/internal/acceptance/errorpagestatus/app"
	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/messaging/inmem"
)

func newClient(t *testing.T) *client.Client {
	t.Helper()
	return client.New(t, mustNewServer(t, &app.App{},
		inmem.New(messaging.DefaultBrokerChanBuffer)))
}

// TestErrorPageStatus covers a URL no page claims in an app that supplies a 404 page:
// the visitor gets that page and the response carries 404.
func TestErrorPageStatus(t *testing.T) {
	t.Parallel()
	c := newClient(t)

	resp := c.Get(t, "/no-such-page/")

	// The page is the app's own. The status is therefore about the response,
	// not about the absence of a page to render.
	require.Contains(t, resp.Body, `<p id="msg">no such page</p>`,
		"the custom 404 page was not rendered")
	require.Equal(t, http.StatusNotFound, resp.Status)
}

// TestRoutedPageIsUnaffected covers a URL a page does claim.
func TestRoutedPageIsUnaffected(t *testing.T) {
	t.Parallel()
	c := newClient(t)

	resp := c.Get(t, "/")
	require.Equal(t, http.StatusOK, resp.Status)
}

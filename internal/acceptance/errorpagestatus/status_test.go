// Asserts that a rendered error page carries the status it stands for.

package acceptance_test

import (
	"context"
	"io"
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

// TestErrorPageStatus tests a URL no page claims in an app that supplies a 404 page:
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

// TestErrorPage404Redirects tests a 404 page that returns a redirect instead of a body.
// The response carries the redirect's own status and Location,
// not the 404 the route would otherwise write.
func TestErrorPage404Redirects(t *testing.T) {
	t.Parallel()
	c := newClient(t)

	// The default client follows the redirect, which hides the status.
	hc := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	req, err := http.NewRequestWithContext(context.Background(),
		http.MethodGet, c.URL()+"/go-home/", nil)
	require.NoError(t, err, "building GET /go-home/")
	resp, err := hc.Do(req)
	require.NoError(t, err, "GET /go-home/")
	defer func() { _ = resp.Body.Close() }()
	_, err = io.ReadAll(resp.Body)
	require.NoError(t, err)

	require.Equal(t, http.StatusFound, resp.StatusCode)
	require.Equal(t, "/", resp.Header.Get("Location"))
}

// TestRoutedPageIsUnaffected tests a URL a page does claim.
func TestRoutedPageIsUnaffected(t *testing.T) {
	t.Parallel()
	c := newClient(t)

	resp := c.Get(t, "/")
	require.Equal(t, http.StatusOK, resp.Status)
}

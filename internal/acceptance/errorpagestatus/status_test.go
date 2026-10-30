// Asserts that a rendered error page carries the status it stands for.

package acceptance_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/datapagestest"
	"github.com/romshark/datapages/internal/acceptance/errorpagestatus/app"
	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/messaging/inmem"
)

func serve(t *testing.T) *datapagestest.Server {
	t.Helper()
	return datapagestest.New(t, mustNewServer(t, &app.App{},
		inmem.New(messaging.DefaultBrokerChanBuffer)))
}

// TestErrorPageStatus tests a URL no page claims in an app that supplies a 404 page:
// the visitor gets that page and the response carries 404.
func TestErrorPageStatus(t *testing.T) {
	t.Parallel()
	v := serve(t).Visitor(t)

	resp := v.Get(t, "/no-such-page/")

	// The page is the app's own. The status is therefore about the response,
	// not about the absence of a page to render.
	require.Equal(t, "no such page", resp.Get(t, datapagestest.ID("msg")).Text(),
		"the custom 404 page was not rendered")
	require.Equal(t, http.StatusNotFound, resp.Status)
}

// TestErrorPage404ReadsQuery tests that a 404 page receives the unmatched URL's query.
func TestErrorPage404ReadsQuery(t *testing.T) {
	t.Parallel()
	v := serve(t).Visitor(t)

	resp := v.Get(t, "/no-such-page/?q=needle")

	require.Equal(t, http.StatusNotFound, resp.Status)
	require.Equal(t, "needle", resp.Get(t, datapagestest.ID("term")).Text())
}

// TestErrorPage404Redirects tests a 404 page that returns a redirect instead of a body.
// The response carries the redirect's own status and Location,
// not the 404 the route would otherwise write.
func TestErrorPage404Redirects(t *testing.T) {
	t.Parallel()
	v := serve(t).Visitor(t)

	resp := v.Get(t, "/go-home/")

	require.Equal(t, http.StatusFound, resp.Status)
	require.Equal(t, "/", resp.Redirect(t))
}

// TestRoutedPageIsUnaffected tests a URL a page does claim.
func TestRoutedPageIsUnaffected(t *testing.T) {
	t.Parallel()
	v := serve(t).Visitor(t)

	resp := v.Get(t, "/")
	require.Equal(t, http.StatusOK, resp.Status)
}

// TestOpenStreamGetsNoStatus tests the same contract as the errors case,
// on an app that defines neither PageError500 nor RecoverError:
// with nothing left to answer with, the stream must end carrying no status text.
func TestOpenStreamGetsNoStatus(t *testing.T) {
	t.Parallel()
	tab := serve(t).Visitor(t).Load(t, "/")

	resp := tab.Do(t, http.MethodPost, "/stream-fail/", nil)

	require.Equal(t, http.StatusOK, resp.Status, "%s", resp.Body)
	require.NotContains(t, resp.Body, "Internal Server Error",
		"a status text was written into the event stream")
	require.NotContains(t, resp.Body, "the action failed with the stream open")
}

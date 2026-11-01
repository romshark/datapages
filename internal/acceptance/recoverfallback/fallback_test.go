// Asserts what a request gets when RecoverError itself fails.

package acceptance_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/datapagestest"
	"github.com/romshark/datapages/internal/acceptance/recoverfallback/app"
	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/messaging/inmem"
)

func serve(t *testing.T) *datapagestest.Server {
	t.Helper()
	return datapagestest.New(t, mustNewServer(t, &app.App{},
		inmem.New(messaging.DefaultBrokerChanBuffer)))
}

// TestRecoverFallbackWritesNothingIntoTheStream tests an action whose error
// RecoverError could not turn into a patch. The stream ends and carries no status text;
// the response was committed before the failure.
func TestRecoverFallbackWritesNothingIntoTheStream(t *testing.T) {
	t.Parallel()
	index := serve(t).Visitor(t).Load(t, "/")

	resp := index.Do(t, "@post('/bad/')", nil)

	require.NotContains(t, resp.Body, "Bad Request",
		"the status text was written into the event stream")
	require.NotContains(t, resp.Body, "cannot render a toast for this",
		"the error of RecoverError reached the client")
	require.NotContains(t, resp.Body, "Internal Server Error",
		"a status text was written into the event stream")
}

// TestPageLoadStillGetsAStatus tests the same failure on a page load,
// where nothing is committed yet and the status is the server's to send.
func TestPageLoadStillGetsAStatus(t *testing.T) {
	t.Parallel()
	v := serve(t).Visitor(t)

	resp := v.Get(t, "/bad/")
	require.NotEqual(t, http.StatusOK, resp.Status,
		"a failed page load was answered with 200")
}

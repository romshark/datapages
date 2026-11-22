// Asserts that RecoverError is called for an app that defines it.

package acceptance_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/datapagestest"
	"github.com/romshark/datapages/internal/acceptance/recovererroralone/app"
	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/messaging/inmem"
)

func serve(t *testing.T) *datapagestest.Server {
	t.Helper()
	return datapagestest.New(t, mustNewServer(t, &app.App{},
		inmem.New(messaging.DefaultBrokerChanBuffer)))
}

// TestRecoverErrorIsCalled tests a failed action in an app that defines
// RecoverError and supplies no PageError500.
func TestRecoverErrorIsCalled(t *testing.T) {
	t.Parallel()
	index := serve(t).Visitor(t).Load(t, "/")

	resp := index.Do(t, "@post('/fail/')", nil)

	require.Contains(t, resp.Body, `<div id="toast">something went wrong</div>`,
		"RecoverError did not answer the failed request")
	require.NotContains(t, resp.Body, "the action failed",
		"the error message reached the client")
}

// TestPageLoadWithoutAn500Page tests that a failed page load returns HTTP 500
// when the app defines RecoverError but no PageError500.
func TestPageLoadWithoutAn500Page(t *testing.T) {
	t.Parallel()
	v := serve(t).Visitor(t)

	resp := v.Get(t, "/boom/")
	require.Equal(t, http.StatusInternalServerError, resp.Status)
	require.NotContains(t, resp.Header.Get("Content-Type"), "text/event-stream")
	require.NotContains(t, resp.Body, "toast")
	require.NotContains(t, resp.Body, "the page could not be built")
}

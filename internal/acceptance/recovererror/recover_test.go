// Tests the generated error recovery of ./app.

package acceptance_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/datapagestest"
	"github.com/romshark/datapages/internal/acceptance/recovererror/app"
	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/messaging/inmem"
)

func serve(t *testing.T) *datapagestest.Server {
	t.Helper()
	return datapagestest.New(t, mustNewServer(t, &app.App{},
		inmem.New(messaging.DefaultBrokerChanBuffer)))
}

func post(t *testing.T, srv *datapagestest.Server, path string) *datapagestest.Response {
	t.Helper()
	return srv.Visitor(t).Load(t, "/").Do(t, http.MethodPost, path, nil)
}

// TestRecoveredActionErrors tests a failed action of a Datastar request.
// The visitor is shown what RecoverError rendered and the request itself succeeds.
// Nothing navigates to a status page here, and nothing would read a status.
func TestRecoveredActionErrors(t *testing.T) {
	t.Parallel()
	tests := map[string]struct{ path, want string }{
		"sentinel the hook knows":  {"/bad/", "bad request"},
		"another sentinel":         {"/missing/", "not found"},
		"error without a sentinel": {"/plain/", "unknown"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			srv := serve(t)
			resp := post(t, srv, tt.path)
			require.Equal(t, http.StatusOK, resp.Status, "%s", resp.Body)
			require.Contains(t, resp.Body, `<div id="toast">`+tt.want+`</div>`)
		})
	}
}

// TestRecoveredPanics tests a handler that panics rather than returning an error.
// It reaches RecoverError as a datapages.PanicError, which keeps a bug
// in 1 handler from dropping the visitor's connection without a word.
func TestRecoveredPanics(t *testing.T) {
	t.Parallel()

	t.Run("action", func(t *testing.T) {
		srv := serve(t)
		resp := post(t, srv, "/panic/")
		require.Equal(t, http.StatusOK, resp.Status, "%s", resp.Body)
		require.Contains(t, resp.Body, `<div id="toast">panic</div>`)
		require.NotContains(t, resp.Body, "the action panicked",
			"the panic value reached the client")
	})

	// A panic while the body is being written cannot be answered any more:
	// the status line and part of the page are on the wire. What the visitor
	// gets is the truncated page, and what the operator gets is the stack.
	t.Run("component render", func(t *testing.T) {
		resp := serve(t).Visitor(t).Get(t, "/render-panic/")

		require.Equal(t, 1, strings.Count(resp.Body, "<!DOCTYPE html>"),
			"the error page was appended to the page:\n%s", resp.Body)
		require.NotContains(t, resp.Body, "server error")
		require.NotContains(t, resp.Body, "the component panicked")
	})

	// A panic in StreamOpen reaches RecoverError instead of aborting the  connection.
	t.Run("stream open", func(t *testing.T) {
		v := serve(t).Visitor(t)
		tab := v.Load(t, "/stream-panic/")
		resp := v.Send(t, tab.NewRequest(t, http.MethodGet, "/stream-panic/_$/", nil))

		require.Contains(t, resp.Body, `<div id="toast">panic</div>`)
		require.NotContains(t, resp.Body, "the stream open hook panicked",
			"the panic value reached the client")
	})

	t.Run("page load", func(t *testing.T) {
		resp := serve(t).Visitor(t).Get(t, "/panic-page/")

		require.Equal(t, http.StatusInternalServerError, resp.Status)
		require.Contains(t, resp.Body, "server error", "the 500 page was not rendered")
		require.NotContains(t, resp.Body, "the page panicked",
			"the panic value reached the visitor")
	})
}

// TestUnrecoveredActionErrorStillAnswers tests the case RecoverError itself
// cannot handle. The request must still be answered rather than left hanging,
// and what it is answered with must say nothing about the error.
//
// The answer is the end of the event stream. Its status line went out with the
// first byte, which leaves the server no status to send. It does not pretend otherwise:
// the failure goes to the operator's log. See the recoverfallback case.
func TestUnrecoveredActionErrorStillAnswers(t *testing.T) {
	t.Parallel()
	srv := serve(t)

	resp := post(t, srv, "/unrecoverable/")
	// The stream was committed before the failure, which leaves 200 as the
	// status the client already received.
	require.Equal(t, http.StatusOK, resp.Status, "want the committed 200")
	for _, leak := range []string{
		"cannot render a toast for this", "unrecoverable", "Internal Server Error",
	} {
		require.NotContains(t, resp.Body, leak, "the failure reached the client")
	}
}

// TestFailedPageLoadRendersTheErrorPage tests an ordinary page load that fails.
// It is not a Datastar request. There is a browser to navigate,
// and it navigates to the app's own 500 page.
func TestFailedPageLoadRendersTheErrorPage(t *testing.T) {
	t.Parallel()
	resp := serve(t).Visitor(t).Get(t, "/boom/")

	require.Contains(t, resp.Body, "server error", "the 500 page was not rendered")
	require.NotContains(t, resp.Body, "the page could not be built",
		"the error message reached the visitor")
}

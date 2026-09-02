// Tests the generated error recovery of ./app.

package acceptance_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/internal/acceptance/recovererror/app"
	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/messaging/inmem"
)

func newServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(mustNewServer(t, &app.App{},
		inmem.New(messaging.DefaultBrokerChanBuffer)))
	t.Cleanup(srv.Close)
	return srv
}

func post(t *testing.T, srv *httptest.Server, path string) (int, string) {
	t.Helper()
	req, err := http.NewRequestWithContext(
		context.Background(), http.MethodPost, srv.URL+path, nil,
	)
	require.NoError(t, err, "building POST %s", path)
	req.Header.Set("Datastar-Request", "true")
	req.Header.Set("Accept-Encoding", "identity")
	resp, err := srv.Client().Do(req)
	require.NoError(t, err, "POST %s", path)
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "reading %s", path)
	return resp.StatusCode, string(b)
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
			srv := newServer(t)
			status, body := post(t, srv, tt.path)
			require.Equal(t, http.StatusOK, status, "%s", body)
			require.Contains(t, body, `<div id="toast">`+tt.want+`</div>`)
		})
	}
}

// TestRecoveredPanics tests a handler that panics rather than returning an error.
// It reaches RecoverError as a datapages.PanicError, which keeps a bug
// in one handler from dropping the visitor's connection without a word.
func TestRecoveredPanics(t *testing.T) {
	t.Parallel()

	t.Run("action", func(t *testing.T) {
		srv := newServer(t)
		status, body := post(t, srv, "/panic/")
		require.Equal(t, http.StatusOK, status, "%s", body)
		require.Contains(t, body, `<div id="toast">panic</div>`)
		require.NotContains(t, body, "the action panicked",
			"the panic value reached the client")
	})

	// A panic while the body is being written cannot be answered any more:
	// the status line and part of the page are on the wire. What the visitor
	// gets is the truncated page, and what the operator gets is the stack.
	t.Run("component render", func(t *testing.T) {
		srv := newServer(t)
		resp, err := srv.Client().Get(srv.URL + "/render-panic/")
		require.NoError(t, err, "GET /render-panic/")
		defer func() { _ = resp.Body.Close() }()
		b, err := io.ReadAll(resp.Body)
		require.NoError(t, err, "reading body")

		body := string(b)
		require.Equal(t, 1, strings.Count(body, "<!DOCTYPE html>"),
			"the error page was appended to the page:\n%s", body)
		require.NotContains(t, body, "server error")
		require.NotContains(t, body, "the component panicked")
	})

	t.Run("page load", func(t *testing.T) {
		srv := newServer(t)
		resp, err := srv.Client().Get(srv.URL + "/panic-page/")
		require.NoError(t, err, "GET /panic-page/")
		defer func() { _ = resp.Body.Close() }()
		b, err := io.ReadAll(resp.Body)
		require.NoError(t, err, "reading body")

		require.Equal(t, http.StatusInternalServerError, resp.StatusCode)
		require.Contains(t, string(b), "server error", "the 500 page was not rendered")
		require.NotContains(t, string(b), "the page panicked",
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
	srv := newServer(t)

	status, body := post(t, srv, "/unrecoverable/")
	// The stream was committed before the failure, which leaves 200 as the
	// status the client already received.
	require.Equal(t, http.StatusOK, status, "want the committed 200")
	for _, leak := range []string{
		"cannot render a toast for this", "unrecoverable", "Internal Server Error",
	} {
		require.NotContains(t, body, leak, "the failure reached the client")
	}
}

// TestFailedPageLoadRendersTheErrorPage tests an ordinary page load that fails.
// It is not a Datastar request. There is a browser to navigate,
// and it navigates to the app's own 500 page.
func TestFailedPageLoadRendersTheErrorPage(t *testing.T) {
	t.Parallel()
	srv := newServer(t)

	resp, err := srv.Client().Get(srv.URL + "/boom/")
	require.NoError(t, err, "GET /boom/")
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "reading body")
	require.Contains(t, string(b), "server error", "the 500 page was not rendered")
	require.NotContains(t, string(b), "the page could not be built",
		"the error message reached the visitor")
}

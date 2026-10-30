// Tests the generated error handling of ./app.

package acceptance_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/datapagestest"
	"github.com/romshark/datapages/internal/acceptance/errors/app"
	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/messaging/inmem"
)

func serve(t *testing.T) *datapagestest.Server {
	t.Helper()
	return datapagestest.New(t, mustNewServer(t, &app.App{},
		inmem.New(messaging.DefaultBrokerChanBuffer)))
}

// serveFailing404 serves the app with a 404 page that fails with ErrNotFound.
func serveFailing404(t *testing.T) *datapagestest.Server {
	t.Helper()
	return datapagestest.New(t, mustNewServer(t, &app.App{Error404Fails: true},
		inmem.New(messaging.DefaultBrokerChanBuffer)))
}

// TestNotFoundPage tests the page an app supplies for URLs nothing claims,
// reached both ways: by such a URL and by its own route.
func TestNotFoundPage(t *testing.T) {
	t.Parallel()
	tests := map[string]string{
		"unknown url":                "/no-such-page/",
		"the error page's own route": "/not-found/",
	}

	for name, path := range tests {
		t.Run(name, func(t *testing.T) {
			v := serve(t).Visitor(t)
			resp := v.Get(t, path)
			require.Equal(t, "not found: "+path,
				resp.Get(t, datapagestest.ID("echo")).Text())
		})
	}
}

// TestServerErrorPageRoute tests the 500 page reached by its own route,
// which is served like any other page. The error500page case tests what a
// failed page load is answered with.
func TestServerErrorPageRoute(t *testing.T) {
	t.Parallel()
	v := serve(t).Visitor(t)

	resp := v.Get(t, "/server-error/")
	require.Equal(t, http.StatusOK, resp.Status)
	require.Equal(t, "server error", resp.Get(t, datapagestest.ID("echo")).Text())
}

// TestFailedPageLoad tests the status and the page a page load gets for the
// error its handler returns. The app supplies pages for 404 and 500, and any
// other status gets its status text. Whatever the visitor is given, it cannot be
// the error the handler produced: that text is written for the operator's log.
func TestFailedPageLoad(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		path   string
		status int
		page   string // the echo of the error page, empty for the status text
	}{
		"plain error": {"/boom/", http.StatusInternalServerError, "server error"},
		"not found":   {"/gone/", http.StatusNotFound, "not found: /gone/"},
		"forbidden":   {"/denied/", http.StatusForbidden, ""},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			v := serve(t).Visitor(t)
			resp := v.Get(t, tt.path)

			require.Equal(t, tt.status, resp.Status, resp.Body)
			if tt.page == "" {
				require.Equal(t, http.StatusText(tt.status)+"\n", resp.Body)
			} else {
				require.Equal(t, tt.page, resp.Get(t, datapagestest.ID("echo")).Text())
			}
			for _, leaked := range []string{
				"the page could not be built", "no such item", "not the owner",
			} {
				require.NotContains(t, resp.Body, leaked,
					"the error message reached the visitor")
			}
		})
	}
}

// TestFailingError404PageStillAnswers tests a 404 page that fails with
// ErrNotFound itself, reached each way the server renders it.
//
// The server answers ErrNotFound on a page load by rendering the 404 page.
// Answering the page's own ErrNotFound the same way renders it again until the
// stack is gone, which takes the process with it rather than the request.
// The server has to stop at the status text instead.
func TestFailingError404PageStillAnswers(t *testing.T) {
	t.Parallel()
	tests := map[string]string{
		"unknown url":                "/no-such-page/",
		"failed page load":           "/gone/",
		"the error page's own route": "/not-found/",
	}

	for name, path := range tests {
		t.Run(name, func(t *testing.T) {
			v := serveFailing404(t).Visitor(t)
			resp := v.Get(t, path)

			require.Equal(t, http.StatusNotFound, resp.Status, resp.Body)
			require.Equal(t, http.StatusText(http.StatusNotFound)+"\n", resp.Body)
		})
	}
}

// TestActionErrorStatus tests the status an action's error becomes.
// The sentinels are the only way an application chooses it,
// and each one must arrive as itself.
func TestActionErrorStatus(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		path string
		want int
	}{
		"plain error":      {"/boom/plain/", http.StatusInternalServerError},
		"bad request":      {"/boom/bad/", http.StatusBadRequest},
		"forbidden":        {"/boom/forbidden/", http.StatusForbidden},
		"not found":        {"/boom/not-found/", http.StatusNotFound},
		"conflict":         {"/boom/conflict/", http.StatusConflict},
		"wrapped sentinel": {"/boom/wrapped/", http.StatusNotFound},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			// PageBoom declares these actions and fails to load, which leaves no tab
			// of its own. A tab of the index page sends the same request:
			// neither page has per-tab state.
			tab := serve(t).Visitor(t).Load(t, "/")
			resp := tab.Do(t, http.MethodPost, tt.path, nil)
			require.Equal(t, tt.want, resp.Status, resp.Body)
			for _, leaked := range []string{"no such item", "something went wrong"} {
				require.NotContains(t, resp.Body, leaked,
					"the error message reached the client")
			}
		})
	}
}

// TestFailedPageLoadIsNotCached tests the response of a failed page load.
// A cached 500 outlives the failure that caused it.
func TestFailedPageLoadIsNotCached(t *testing.T) {
	t.Parallel()
	v := serve(t).Visitor(t)

	resp := v.Get(t, "/boom/")

	cc := resp.Header.Get("Cache-Control")
	if strings.Contains(cc, "max-age") {
		require.Contains(t, cc, "max-age=0",
			"Cache-Control = %q on a failed page load", cc)
	}
}

// TestOpenStreamGetsNoStatus tests an action holding an open event stream in an
// app that defines no RecoverError. The response went out as 200 text/event-stream
// before the handler ran, which leaves no status to send. A status written anyway
// lands in the stream as text and makes net/http log a superfluous WriteHeader call.
func TestOpenStreamGetsNoStatus(t *testing.T) {
	t.Parallel()
	tests := map[string]string{
		"error": "/stream-fail/",
		"panic": "/stream-panic/",
	}

	for name, path := range tests {
		t.Run(name, func(t *testing.T) {
			tab := serve(t).Visitor(t).Load(t, "/")
			resp := tab.Do(t, http.MethodPost, path, nil)

			require.Equal(t, http.StatusOK, resp.Status, "%s", resp.Body)
			require.Equal(t, "text/event-stream",
				strings.Split(resp.Header.Get("Content-Type"), ";")[0])
			require.NotContains(t, resp.Body, "Internal Server Error",
				"a status text was written into the event stream")
			require.NotContains(t, resp.Body, "the action failed with the stream open")
			require.NotContains(t, resp.Body, "the action panicked with the stream open")
		})
	}
}

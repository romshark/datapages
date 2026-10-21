// Tests the generated error recovery of ./app.

package acceptance_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/internal/acceptance/recovererror/app"
	"github.com/romshark/datapages/internal/testlog"
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

func getStream(t *testing.T, srv *httptest.Server, path string) (int, string) {
	t.Helper()
	req, err := http.NewRequestWithContext(
		context.Background(), http.MethodGet, srv.URL+path, nil,
	)
	require.NoError(t, err, "building GET %s", path)
	req.Header.Set("Datastar-Request", "true")
	req.Header.Set("Accept-Encoding", "identity")
	resp, err := srv.Client().Do(req)
	require.NoError(t, err, "GET %s", path)
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "reading %s", path)
	return resp.StatusCode, string(b)
}

// send sends method to path the way the Datastar client sends a request,
// with body as the signals.
func send(t *testing.T, srv *httptest.Server, method, path, body string) (int, string) {
	t.Helper()
	req, err := http.NewRequestWithContext(
		context.Background(), method, srv.URL+path, strings.NewReader(body),
	)
	require.NoError(t, err, "building %s %s", method, path)
	req.Header.Set("Datastar-Request", "true")
	req.Header.Set("Accept-Encoding", "identity")
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := srv.Client().Do(req)
	require.NoError(t, err, "%s %s", method, path)
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
// in 1 handler from dropping the visitor's connection without a word.
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

	// A panic in StreamOpen reaches RecoverError instead of aborting the connection,
	// and logs its stack.
	t.Run("stream open", func(t *testing.T) {
		var log testlog.Buffer
		srv := httptest.NewServer(mustNewServer(t, &app.App{},
			inmem.New(messaging.DefaultBrokerChanBuffer),
			datapages.WithLogger(log.Logger())))
		t.Cleanup(srv.Close)
		_, body := getStream(t, srv, "/stream-panic/_$/")
		require.Contains(t, body, `<div id="toast">panic</div>`)
		require.NotContains(t, body, "the stream open hook panicked",
			"the panic value reached the client")
		log.RequireMatch(t, `^level=ERROR msg="recovered panic" `+
			`handler=PageStreamPanic\.StreamOpen `+
			`panic="the stream open hook panicked" stack=`)
	})

	// A request-scoped StreamConfig panic reaches RecoverError and logs its stack.
	t.Run("stream config", func(t *testing.T) {
		var log testlog.Buffer
		srv := httptest.NewServer(mustNewServer(t, &app.App{},
			inmem.New(messaging.DefaultBrokerChanBuffer),
			datapages.WithLogger(log.Logger())))
		t.Cleanup(srv.Close)
		_, body := getStream(t, srv, "/stream-panic/_$/?config-panic")
		require.Contains(t, body, `<div id="toast">panic</div>`)
		require.NotContains(t, body, "the stream config panicked",
			"the panic value reached the client")
		log.RequireMatch(t, `^level=ERROR msg="recovered panic" `+
			`handler=PageStreamPanic\.StreamConfig `+
			`panic="the stream config panicked" stack=`)
	})

	// A panicking action StreamConfig reaches RecoverError instead of the action.
	t.Run("action stream config", func(t *testing.T) {
		var log testlog.Buffer
		srv := httptest.NewServer(mustNewServer(t, &app.App{},
			inmem.New(messaging.DefaultBrokerChanBuffer),
			datapages.WithLogger(log.Logger())))
		t.Cleanup(srv.Close)
		status, body := post(t, srv, "/config-panic/")
		require.Equal(t, http.StatusOK, status, "%s", body)
		require.Contains(t, body, `<div id="toast">panic</div>`)
		require.NotContains(t, body, ">ran<", "the action ran")
		require.NotContains(t, body, "the action stream config panicked",
			"the panic value reached the client")
		log.RequireMatch(t, `^level=ERROR msg="recovered panic" `+
			`handler=PageIndex\.StreamConfigPOSTConfigPanic `+
			`panic="the action stream config panicked" stack=`)
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

// TestHalfWrittenPageGetsNoEvents tests that recovery appends neither SSE nor a 2nd gzip stream.
func TestHalfWrittenPageGetsNoEvents(t *testing.T) {
	t.Parallel()

	for _, coding := range []string{"identity", "gzip"} {
		t.Run(coding, func(t *testing.T) {
			srv := newServer(t)
			req, err := http.NewRequestWithContext(context.Background(),
				http.MethodGet, srv.URL+"/render-panic/", nil)
			require.NoError(t, err, "building the request")
			req.Header.Set("Datastar-Request", "true")
			req.Header.Set("Accept-Encoding", coding)
			resp, err := srv.Client().Do(req)
			require.NoError(t, err, "GET /render-panic/")
			defer func() { _ = resp.Body.Close() }()
			raw, err := io.ReadAll(resp.Body)
			require.NoError(t, err, "reading body")

			b := raw
			switch ce := resp.Header.Get("Content-Encoding"); ce {
			case "":
				require.NotContains(t, string(raw), "\x1f\x8b",
					"a gzip stream was appended to the page")
			case coding:
				r := bytes.NewReader(raw)
				zr, err := gzip.NewReader(r)
				require.NoError(t, err, "reading the gzip header")
				zr.Multistream(false)
				b, err = io.ReadAll(zr)
				require.NoError(t, err, "decoding the page")
				require.Zero(t, r.Len(), "a stream was appended to the page")
			default:
				t.Fatalf("Content-Encoding %q for Accept-Encoding %q", ce, coding)
			}

			body := string(b)
			require.Contains(t, resp.Header.Get("Content-Type"), "text/html")
			require.Contains(t, body, "<!DOCTYPE html>")
			require.NotContains(t, body, "event:", "RecoverError wrote into the page")
		})
	}
}

// TestPanickingRecoverErrorRunsOnce tests that its panic is logged once with a stack.
func TestPanickingRecoverErrorRunsOnce(t *testing.T) {
	t.Parallel()

	for _, coding := range []string{"identity", "gzip"} {
		t.Run(coding, func(t *testing.T) {
			var log testlog.Buffer
			srv := httptest.NewServer(mustNewServer(t, &app.App{},
				inmem.New(messaging.DefaultBrokerChanBuffer),
				datapages.WithLogger(log.Logger())))
			t.Cleanup(srv.Close)

			req, err := http.NewRequestWithContext(context.Background(),
				http.MethodPost, srv.URL+"/recover-panic/", nil)
			require.NoError(t, err, "building the request")
			req.Header.Set("Datastar-Request", "true")
			req.Header.Set("Accept-Encoding", coding)
			resp, err := srv.Client().Do(req)
			require.NoError(t, err, "POST /recover-panic/")
			defer func() { _ = resp.Body.Close() }()
			raw, err := io.ReadAll(resp.Body)
			require.NoError(t, err, "reading body")
			require.Equal(t, http.StatusOK, resp.StatusCode)

			body := raw
			if coding == "gzip" {
				require.Equal(t, "gzip", resp.Header.Get("Content-Encoding"))
				r := bytes.NewReader(raw)
				zr, err := gzip.NewReader(r)
				require.NoError(t, err, "reading the gzip header")
				zr.Multistream(false)
				body, err = io.ReadAll(zr)
				require.NoError(t, err, "decoding the stream")
				require.Zero(t, r.Len(), "the response holds more than 1 gzip member")
			}
			require.Empty(t, string(body), "RecoverError wrote after its panic")
			log.RequireMatch(t, `^level=ERROR msg="recovered panic" `+
				`handler=App\.RecoverError panic="RecoverError panicked" stack=`)
		})
	}
}

// TestFailedSubscribeDelaysReconnect tests the recovery stream's retry field.
func TestFailedSubscribeDelaysReconnect(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(mustNewServer(t, &app.App{},
		failingBroker{inmem.New(messaging.DefaultBrokerChanBuffer)}))
	t.Cleanup(srv.Close)

	status, body := getStream(t, srv, "/stream-panic/_$/")
	require.Equal(t, http.StatusOK, status)
	i := strings.Index(body, `<div id="toast">unknown</div>`)
	require.GreaterOrEqual(t, i, 0, "RecoverError did not answer:\n%s", body)
	require.Contains(t, body[i:], "retry: 5000",
		"the reconnect delay does not follow the RecoverError event")
}

// failingBroker fails every subscription.
type failingBroker struct{ messaging.Broker }

func (failingBroker) Subscribe(
	context.Context, messaging.Metrics, ...string,
) (messaging.Subscription, error) {
	return nil, errors.New("broker down")
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

// TestRecoveredDecodePanics tests a panic in the application code that parses
// a request input: UnmarshalText of a path or query field and UnmarshalJSON of
// a signal. It takes the error path of the handler that reads the input.
// net/http otherwise closes the connection without a response.
func TestRecoveredDecodePanics(t *testing.T) {
	t.Parallel()
	datastar := "?datastar=" + url.QueryEscape(`{"s":"xbad"}`)

	// A page load gets the 500 page.
	for name, tt := range map[string]struct{ path, handler string }{
		"page path":    {"/thing/xbad/", "PageThing.GET"},
		"page query":   {"/thing/ok/?q=xbad", "PageThing.GET"},
		"page signals": {"/thing/ok/" + datastar, "PageThing.GET"},
		"404 page":     {"/nowhere/?nf=xbad", "PageError404.GET"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var log testlog.Buffer
			srv := httptest.NewServer(mustNewServer(t, &app.App{},
				inmem.New(messaging.DefaultBrokerChanBuffer),
				datapages.WithLogger(log.Logger())))
			t.Cleanup(srv.Close)

			resp, err := srv.Client().Get(srv.URL + tt.path)
			require.NoError(t, err, "GET %s", tt.path)
			defer func() { _ = resp.Body.Close() }()
			b, err := io.ReadAll(resp.Body)
			require.NoError(t, err, "reading body")

			require.Equal(t, http.StatusInternalServerError, resp.StatusCode)
			require.Contains(t, string(b), "server error", "the 500 page was not rendered")
			require.NotContains(t, string(b), "parser panicked",
				"the panic value reached the visitor")
			log.RequireMatch(t, `^level=ERROR msg="recovered panic" handler=`+
				regexp.QuoteMeta(tt.handler)+` panic="the (text|JSON) parser panicked" stack=`)
		})
	}

	// PageError500 answers its own panic with the status alone. Rendering
	// itself again would panic again.
	t.Run("500 page", func(t *testing.T) {
		t.Parallel()
		srv := newServer(t)
		resp, err := srv.Client().Get(srv.URL + "/server-error/?se=xbad")
		require.NoError(t, err, "GET /server-error/")
		defer func() { _ = resp.Body.Close() }()
		b, err := io.ReadAll(resp.Body)
		require.NoError(t, err, "reading body")

		require.Equal(t, http.StatusInternalServerError, resp.StatusCode)
		require.Equal(t, "Internal Server Error\n", string(b))
	})

	// A file goes to an img element or a fetch as often as to a document,
	// which shows nothing of an error page.
	for name, path := range map[string]string{
		"file action":     "/thing/ok/file/?q=xbad",
		"app file action": "/app-file/?q=xbad",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			srv := newServer(t)
			resp, err := srv.Client().Get(srv.URL + path)
			require.NoError(t, err, "GET %s", path)
			defer func() { _ = resp.Body.Close() }()
			b, err := io.ReadAll(resp.Body)
			require.NoError(t, err, "reading body")

			require.Equal(t, http.StatusInternalServerError, resp.StatusCode)
			require.Equal(t, "Internal Server Error\n", string(b))
		})
	}

	// A Datastar request gets what RecoverError renders. An action reads its
	// signals first, which have to be valid for the path and the query to be read.
	// handler is as logged, which quotes a name with a space.
	for name, tt := range map[string]struct{ method, path, body, handler string }{
		"action path":    {http.MethodPost, "/thing/xbad/act/", "{}", "PageThing.Act"},
		"action query":   {http.MethodPost, "/thing/ok/act/?q=xbad", "{}", "PageThing.Act"},
		"action signals": {http.MethodPost, "/thing/ok/act/", `{"s":"xbad"}`, "PageThing.Act"},
		"action without stream": {
			http.MethodPost, "/thing/ok/plain/?q=xbad", "", "PageThing.Plain",
		},
		"app action": {http.MethodPost, "/app-act/?q=xbad", "", "App.AppAct"},
		"stream": {
			http.MethodGet, "/thing/ok/_$/" + datastar, "", `"PageThing stream"`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var log testlog.Buffer
			srv := httptest.NewServer(mustNewServer(t, &app.App{},
				inmem.New(messaging.DefaultBrokerChanBuffer),
				datapages.WithLogger(log.Logger())))
			t.Cleanup(srv.Close)

			status, body := send(t, srv, tt.method, tt.path, tt.body)
			require.Equal(t, http.StatusOK, status, "%s", body)
			require.Contains(t, body, `<div id="toast">panic</div>`)
			require.NotContains(t, body, ">ran<", "the action ran")
			require.NotContains(t, body, "parser panicked",
				"the panic value reached the client")
			log.RequireMatch(t, `^level=ERROR msg="recovered panic" handler=`+
				regexp.QuoteMeta(tt.handler)+` panic="the (text|JSON) parser panicked" stack=`)
		})
	}
}

// TestPointerParameters tests path and query fields that are pointers to a type
// that parses itself. UnmarshalText writes to a value the server allocates,
// and a query parameter that is absent leaves its field nil.
func TestPointerParameters(t *testing.T) {
	t.Parallel()
	for name, tt := range map[string]struct{ path, want string }{
		"query absent":  {"/pointer/abc/", "abc absent"},
		"query present": {"/pointer/abc/?q=def", "abc def"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			srv := newServer(t)
			resp, err := srv.Client().Get(srv.URL + tt.path)
			require.NoError(t, err, "GET %s", tt.path)
			defer func() { _ = resp.Body.Close() }()
			b, err := io.ReadAll(resp.Body)
			require.NoError(t, err, "reading body")

			require.Equal(t, http.StatusOK, resp.StatusCode, "%s", b)
			require.Contains(t, string(b), `<pre id="echo">`+tt.want+`</pre>`)
		})
	}
}

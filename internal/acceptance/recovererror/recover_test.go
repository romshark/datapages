// Tests the generated error recovery of ./app.

package acceptance_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/datapagestest"
	"github.com/romshark/datapages/internal/acceptance/recovererror/app"
	"github.com/romshark/datapages/internal/testlog"
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

// serveLogged is serve with the server's log written to log.
func serveLogged(t *testing.T, log *testlog.Buffer) *datapagestest.Server {
	t.Helper()
	return datapagestest.New(t, mustNewServer(t, &app.App{},
		inmem.New(messaging.DefaultBrokerChanBuffer),
		datapages.WithLogger(log.Logger())))
}

// readStream loads page and reads the response of its stream route,
// followed by query, to the end.
func readStream(
	t *testing.T, srv *datapagestest.Server, page, query string,
) *datapagestest.Response {
	t.Helper()
	v := srv.Visitor(t)
	tab := v.Load(t, page)
	return v.Send(t, tab.NewRequest(t, http.MethodGet, page+"_$/"+query, nil))
}

// send sends method to path the way the Datastar client sends a request,
// with body as the signals. It loads no page first, since a page whose path
// value panics fails to load.
func send(
	t *testing.T, srv *datapagestest.Server, method, path, body string,
) *datapagestest.Response {
	t.Helper()
	v := srv.Visitor(t)
	req := v.NewRequest(t, method, path, strings.NewReader(body))
	req.Header.Set("Datastar-Request", "true")
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	return v.Send(t, req)
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

	// A panic in StreamOpen reaches RecoverError instead of aborting the connection,
	// and logs its stack.
	t.Run("stream open", func(t *testing.T) {
		var log testlog.Buffer
		resp := readStream(t, serveLogged(t, &log), "/stream-panic/", "")
		require.Contains(t, resp.Body, `<div id="toast">panic</div>`)
		require.NotContains(t, resp.Body, "the stream open hook panicked",
			"the panic value reached the client")
		log.RequireMatch(t, `^level=ERROR msg="recovered panic" `+
			`handler=PageStreamPanic\.StreamOpen `+
			`panic="the stream open hook panicked" stack=`)
	})

	// A request-scoped StreamConfig panic reaches RecoverError and logs its stack.
	t.Run("stream config", func(t *testing.T) {
		var log testlog.Buffer
		resp := readStream(t, serveLogged(t, &log), "/stream-panic/", "?config-panic")
		require.Contains(t, resp.Body, `<div id="toast">panic</div>`)
		require.NotContains(t, resp.Body, "the stream config panicked",
			"the panic value reached the client")
		log.RequireMatch(t, `^level=ERROR msg="recovered panic" `+
			`handler=PageStreamPanic\.StreamConfig `+
			`panic="the stream config panicked" stack=`)
	})

	// A panicking action StreamConfig reaches RecoverError instead of the action.
	t.Run("action stream config", func(t *testing.T) {
		var log testlog.Buffer
		resp := post(t, serveLogged(t, &log), "/config-panic/")
		require.Equal(t, http.StatusOK, resp.Status, "%s", resp.Body)
		require.Contains(t, resp.Body, `<div id="toast">panic</div>`)
		require.NotContains(t, resp.Body, ">ran<", "the action ran")
		require.NotContains(t, resp.Body, "the action stream config panicked",
			"the panic value reached the client")
		log.RequireMatch(t, `^level=ERROR msg="recovered panic" `+
			`handler=PageIndex\.StreamConfigPOSTConfigPanic `+
			`panic="the action stream config panicked" stack=`)
	})

	t.Run("page load", func(t *testing.T) {
		resp := serve(t).Visitor(t).Get(t, "/panic-page/")

		require.Equal(t, http.StatusInternalServerError, resp.Status)
		require.Contains(t, resp.Body, "server error", "the 500 page was not rendered")
		require.NotContains(t, resp.Body, "the page panicked",
			"the panic value reached the visitor")
	})
}

// TestHalfWrittenPageGetsNoEvents tests that recovery appends neither SSE nor a 2nd gzip stream.
func TestHalfWrittenPageGetsNoEvents(t *testing.T) {
	t.Parallel()

	for _, coding := range []string{"identity", "gzip"} {
		t.Run(coding, func(t *testing.T) {
			v := serve(t).Visitor(t)
			req := v.NewRequest(t, http.MethodGet, "/render-panic/", nil)
			req.Header.Set("Datastar-Request", "true")
			req.Header.Set("Accept-Encoding", coding)
			resp := v.Send(t, req)

			body := []byte(resp.Body)
			switch ce := resp.Header.Get("Content-Encoding"); ce {
			case "":
				require.NotContains(t, resp.Body, "\x1f\x8b",
					"a gzip stream was appended to the page")
			case coding:
				r := bytes.NewReader(body)
				zr, err := gzip.NewReader(r)
				require.NoError(t, err, "reading the gzip header")
				zr.Multistream(false)
				body, err = io.ReadAll(zr)
				require.NoError(t, err, "decoding the page")
				require.Zero(t, r.Len(), "a stream was appended to the page")
			default:
				t.Fatalf("Content-Encoding %q for Accept-Encoding %q", ce, coding)
			}

			require.Contains(t, resp.Header.Get("Content-Type"), "text/html")
			require.Contains(t, string(body), "<!DOCTYPE html>")
			require.NotContains(t, string(body), "event:", "RecoverError wrote into the page")
		})
	}
}

// TestPanickingRecoverErrorRunsOnce tests that its panic is logged once with a stack.
func TestPanickingRecoverErrorRunsOnce(t *testing.T) {
	t.Parallel()

	for _, coding := range []string{"identity", "gzip"} {
		t.Run(coding, func(t *testing.T) {
			var log testlog.Buffer
			v := serveLogged(t, &log).Visitor(t)
			req := v.Load(t, "/").NewRequest(t, http.MethodPost, "/recover-panic/", nil)
			req.Header.Set("Accept-Encoding", coding)
			resp := v.Send(t, req)
			require.Equal(t, http.StatusOK, resp.Status)

			body := []byte(resp.Body)
			if coding == "gzip" {
				require.Equal(t, "gzip", resp.Header.Get("Content-Encoding"))
				r := bytes.NewReader(body)
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
	srv := datapagestest.New(t, mustNewServer(t, &app.App{},
		failingBroker{inmem.New(messaging.DefaultBrokerChanBuffer)}))

	resp := readStream(t, srv, "/stream-panic/", "")
	require.Equal(t, http.StatusOK, resp.Status)
	i := strings.Index(resp.Body, `<div id="toast">unknown</div>`)
	require.GreaterOrEqual(t, i, 0, "RecoverError did not answer:\n%s", resp.Body)
	require.Contains(t, resp.Body[i:], "retry: 5000",
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
			resp := serveLogged(t, &log).Visitor(t).Get(t, tt.path)

			require.Equal(t, http.StatusInternalServerError, resp.Status)
			require.Contains(t, resp.Body, "server error", "the 500 page was not rendered")
			require.NotContains(t, resp.Body, "parser panicked",
				"the panic value reached the visitor")
			log.RequireMatch(t, `^level=ERROR msg="recovered panic" handler=`+
				regexp.QuoteMeta(tt.handler)+` panic="the (text|JSON) parser panicked" stack=`)
		})
	}

	// PageError500 answers its own panic with the status alone. Rendering
	// itself again would panic again.
	t.Run("500 page", func(t *testing.T) {
		t.Parallel()
		resp := serve(t).Visitor(t).Get(t, "/server-error/?se=xbad")

		require.Equal(t, http.StatusInternalServerError, resp.Status)
		require.Equal(t, "Internal Server Error\n", resp.Body)
	})

	// A file goes to an img element or a fetch as often as to a document,
	// which shows nothing of an error page.
	for name, path := range map[string]string{
		"file action":     "/thing/ok/file/?q=xbad",
		"app file action": "/app-file/?q=xbad",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			resp := serve(t).Visitor(t).Get(t, path)

			require.Equal(t, http.StatusInternalServerError, resp.Status)
			require.Equal(t, "Internal Server Error\n", resp.Body)
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
			resp := send(t, serveLogged(t, &log), tt.method, tt.path, tt.body)
			require.Equal(t, http.StatusOK, resp.Status, "%s", resp.Body)
			require.Contains(t, resp.Body, `<div id="toast">panic</div>`)
			require.NotContains(t, resp.Body, ">ran<", "the action ran")
			require.NotContains(t, resp.Body, "parser panicked",
				"the panic value reached the client")
			log.RequireMatch(t, `^level=ERROR msg="recovered panic" handler=`+
				regexp.QuoteMeta(tt.handler)+` panic="the (text|JSON) parser panicked" stack=`)
		})
	}
}

// TestErrorCounters tests that every error of a handler counts once,
// as recovered when RecoverError handles it and as not recovered otherwise.
// A route that nothing serves is no error of a handler and counts as neither.
//
// It doesn't call t.Parallel, because the counters are global to the process.
// The parallel tests of the case start only after it returns.
func TestErrorCounters(t *testing.T) {
	v := serve(t).Visitor(t)
	tests := map[string]struct {
		method, path            string
		datastar                bool
		recovered, notRecovered float64
	}{
		"handled action": {
			http.MethodPost, "/bad/", true, 1, 0,
		},
		"handled stream": {
			http.MethodGet, "/stream-panic/_$/", true, 1, 0,
		},
		"RecoverError fails": {
			http.MethodPost, "/unrecoverable/", true, 0, 1,
		},
		"RecoverError panics": {
			http.MethodPost, "/recover-panic/", true, 0, 1,
		},
		"page load": {
			http.MethodGet, "/boom/", false, 0, 1,
		},
		"page load after body": {
			http.MethodGet, "/render-panic/", false, 0, 1,
		},
		"Datastar GET after body": {
			http.MethodGet, "/render-panic/", true, 0, 1,
		},
		"failing 500 page": {
			http.MethodGet, "/server-error/?se=xbad", false, 0, 1,
		},
		"failing 404 page": {
			http.MethodGet, "/nowhere/?nf=gone", false, 0, 1,
		},
		"file action": {
			http.MethodGet, "/app-file/?q=xbad", false, 0, 1,
		},
		"route miss": {
			http.MethodGet, "/nowhere/", false, 0, 0,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			recovered := counter(t, "datapages_internal_errors_recovered_total")
			notRecovered := counter(t, "datapages_internal_errors_not_recovered_total")

			req := v.NewRequest(t, tt.method, tt.path, nil)
			if tt.datastar {
				req.Header.Set("Datastar-Request", "true")
			}
			// Send reads to EOF, and the response ends after the handler returns.
			v.Send(t, req)

			require.Equal(t, tt.recovered,
				counter(t, "datapages_internal_errors_recovered_total")-recovered,
				"recovered")
			require.Equal(t, tt.notRecovered,
				counter(t, "datapages_internal_errors_not_recovered_total")-notRecovered,
				"not recovered")
		})
	}
}

// counter reads the counter name from the registry.
func counter(t *testing.T, name string) float64 {
	t.Helper()
	families, err := registry.Gather()
	require.NoError(t, err, "gathering metrics")
	for _, f := range families {
		if f.GetName() == name {
			return f.GetMetric()[0].GetCounter().GetValue()
		}
	}
	t.Fatalf("%s is not registered", name)
	return 0
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
			resp := serve(t).Visitor(t).Get(t, tt.path)

			require.Equal(t, http.StatusOK, resp.Status, "%s", resp.Body)
			require.Contains(t, resp.Body, `<pre id="echo">`+tt.want+`</pre>`)
		})
	}
}

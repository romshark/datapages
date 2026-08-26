// Covers the generated action handlers of ./app.

package acceptance_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/internal/acceptance/actions/app"
	"github.com/romshark/datapages/internal/acceptance/actions/app/datapagesgen/action"
	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/messaging/inmem"
)

type server struct {
	*httptest.Server
}

func newServer(t *testing.T) server {
	t.Helper()
	s := httptest.NewServer(mustNewServer(t, &app.App{},
		inmem.New(messaging.DefaultBrokerChanBuffer)))
	t.Cleanup(s.Close)
	return server{s}
}

// do sends an action request the way the Datastar client sends it.
func (s server) do(t *testing.T, method, url, body string) *http.Response {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req, err := http.NewRequestWithContext(context.Background(), method, s.URL+url, r)
	require.NoError(t, err, "building %s %s", method, url)
	req.Header.Set("Datastar-Request", "true")
	// Responses are compressed by default. Asking for none of it keeps the
	// bytes this test reads the bytes the handler wrote.
	req.Header.Set("Accept-Encoding", "identity")
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := s.Client().Do(req)
	require.NoError(t, err, "%s %s", method, url)
	return resp
}

// call sends an action request and returns its status and body.
func (s server) call(t *testing.T, method, url, body string) (int, string) {
	t.Helper()
	resp := s.do(t, method, url, body)
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "reading body of %s %s", method, url)
	return resp.StatusCode, string(b)
}

// logOf reads back what the actions recorded.
func (s server) logOf(t *testing.T) string {
	t.Helper()
	resp, err := s.Client().Get(s.URL + "/log/")
	require.NoError(t, err, "GET /log/")
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "reading /log/")
	return echoed(t, string(b))
}

func echoed(t *testing.T, body string) string {
	t.Helper()
	const open = `<pre id="echo">`
	_, after, ok := strings.Cut(body, open)
	require.True(t, ok, "no echo element in response:\n%s", body)
	before, _, ok := strings.Cut(after, "</pre>")
	require.True(t, ok, "unterminated echo element in response:\n%s", body)
	return before
}

// TestMethods covers one action per HTTP method, page-level and app-level.
// The action package builds the URL the template would use. The routing of an
// action is therefore asserted through the same expression a page carries.
func TestMethods(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		method string
		expr   string
		body   string
		want   string
	}{
		"post on a page": {
			method: http.MethodPost,
			expr:   action.POSTPageFormSubmit(),
			body:   `{"name":"ada","age":36}`,
			want:   `submit name="ada" age=36`,
		},
		"put on a page": {
			method: http.MethodPut,
			expr:   action.PUTPageFormReplace(),
			want:   "replace",
		},
		"patch on a page": {
			method: http.MethodPatch,
			expr:   action.PATCHPageFormTouch(),
			want:   "touch",
		},
		"delete on a page": {
			method: http.MethodDelete,
			expr:   action.DELETEPageFormRemove(),
			want:   "remove",
		},
		"post on the app": {
			method: http.MethodPost,
			expr:   action.POSTAppPing(),
			want:   "ping",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			srv := newServer(t)
			url := urlOf(t, tt.expr)
			st, body := srv.call(t, tt.method, url, tt.body)
			require.Equal(t, http.StatusOK, st, "%s %s\n%s", tt.method, url, body)
			require.Equal(t, tt.want, srv.logOf(t))
		})
	}
}

// TestParameters covers path variables and query parameters on an action.
func TestParameters(t *testing.T) {
	t.Parallel()
	srv := newServer(t)

	url := urlOf(t, action.POSTPageFormBump(7, action.QueryPOSTPageFormBump{By: 3}))
	status, body := srv.call(t, http.MethodPost, url, "")
	require.Equal(t, http.StatusOK, status, "POST %s\n%s", url, body)
	require.Equal(t, "bump id=7 by=3", srv.logOf(t))
}

// TestSignalsAreRequired covers a request whose body is not the signals the
// handler declares.
func TestSignalsAreRequired(t *testing.T) {
	t.Parallel()
	srv := newServer(t)

	url := urlOf(t, action.POSTPageFormSubmit())
	status, _ := srv.call(t, http.MethodPost, url, `{"age":"not a number"}`)
	require.Equal(t, http.StatusBadRequest, status)
	require.Empty(t, srv.logOf(t), "the handler ran on a body it could not read")
}

// TestBodySizeLimit covers the limit on a signals body, on an action that
// opens an SSE on its own request as well as on one that does not.
// Both read the signals before anything else, hence the limit costs the SSE nothing.
func TestBodySizeLimit(t *testing.T) {
	t.Parallel()
	// datapagesgen.DefaultBodySizeLimit is 1 MiB.
	body := `{"count":1,"pad":"` + strings.Repeat("a", 2*1024*1024) + `"}`
	for name, expr := range map[string]string{
		"signals":     action.POSTPageFormSubmit(),
		"signals+sse": action.POSTPageFormPatch(),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			srv := newServer(t)
			status, _ := srv.call(t, http.MethodPost, urlOf(t, expr), body)
			require.Equal(t, http.StatusBadRequest, status)
			require.Empty(t, srv.logOf(t), "the handler ran on an oversized body")
		})
	}
}

// TestBodySizeLimitOption covers datapages.WithBodySizeLimit, which is what an
// application raises when its signals carry more than the default allows.
func TestBodySizeLimitOption(t *testing.T) {
	t.Parallel()
	const limit = 4096
	srv := server{httptest.NewServer(mustNewServer(t, &app.App{},
		inmem.New(messaging.DefaultBrokerChanBuffer),
		datapages.WithBodySizeLimit(limit)))}
	t.Cleanup(srv.Close)

	pad := func(n int) string {
		return `{"count":1,"pad":"` + strings.Repeat("a", n) + `"}`
	}
	url := urlOf(t, action.POSTPageFormSubmit())

	status, _ := srv.call(t, http.MethodPost, url, pad(limit/2))
	require.Equal(t, http.StatusOK, status, "a body under the limit")

	status, _ = srv.call(t, http.MethodPost, url, pad(2*limit))
	require.Equal(t, http.StatusBadRequest, status, "a body over the limit")
}

// TestBodyOutput covers an action that answers with a document.
func TestBodyOutput(t *testing.T) {
	t.Parallel()
	srv := newServer(t)

	status, body := srv.call(t, http.MethodPost, "/form/render/", "")
	require.Equal(t, http.StatusOK, status, "%s", body)
	require.Equal(t, "rendered by an action", echoed(t, body))
}

// TestGlobalHead covers the head the app declares once for everything it renders,
// on a page load and on a rendering action alike.
func TestGlobalHead(t *testing.T) {
	t.Parallel()
	srv := newServer(t)

	for name, req := range map[string]struct{ method, url string }{
		"page load": {http.MethodGet, "/"},
		"action":    {http.MethodPost, "/form/render/"},
	} {
		t.Run(name, func(t *testing.T) {
			status, body := srv.call(t, req.method, req.url, "")
			require.Equal(t, http.StatusOK, status, "%s", body)
			require.Contains(t, body, "<title>actions</title>",
				"the global head is not in the response")
		})
	}
}

// TestRedirect covers the script a Datastar request is sent and
// the 303 a plain one is sent.
// A Datastar request cannot follow a 303: the browser would replace the fragment,
// not the page. It gets a script instead.
func TestRedirect(t *testing.T) {
	t.Parallel()
	t.Run("datastar request navigates by script", func(t *testing.T) {
		srv := newServer(t)
		resp := srv.do(t, http.MethodPost, "/form/go/", "")
		defer func() { _ = resp.Body.Close() }()

		requireContentType(t, resp, "text/javascript")
		b, err := io.ReadAll(resp.Body)
		require.NoError(t, err, "reading body")
		require.Equal(t, `window.location = "/log/";`, string(b))
	})

	// An action holding the stream of its own request has no response head
	// left to redirect with, hence the navigation is an event of the stream.
	t.Run("stream request navigates through the stream", func(t *testing.T) {
		srv := newServer(t)
		resp := srv.do(t, http.MethodPost, "/form/go-stream/", "")
		defer func() { _ = resp.Body.Close() }()

		requireContentType(t, resp, "text/event-stream")
		b, err := io.ReadAll(resp.Body)
		require.NoError(t, err, "reading stream")
		stream := string(b)
		require.Contains(t, stream, "event: datastar-patch-elements")
		require.Contains(t, stream, `window.location.href = "/log/"`)
	})

	// An action that needs neither signals nor an SSE connection is reachable
	// by a plain form post, and that client can only be moved by a real redirect.
	// The status the handler asked for is the status it must send.
	t.Run("plain request gets the status it asked for", func(t *testing.T) {
		srv := newServer(t)
		client := *srv.Client()
		client.CheckRedirect = func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}
		req, err := http.NewRequestWithContext(
			context.Background(), http.MethodPost, srv.URL+"/form/go/", nil,
		)
		require.NoError(t, err, "building request")
		resp, err := client.Do(req)
		require.NoError(t, err, "POST /form/go/")
		defer func() { _ = resp.Body.Close() }()
		require.Equal(t, http.StatusSeeOther, resp.StatusCode)
		require.Equal(t, "/log/", resp.Header.Get("Location"))
	})
}

// TestDatastarOnlyActions covers the actions that cannot serve a plain request:
// reading signals and answering on an SSE connection both require the Datastar client.
func TestDatastarOnlyActions(t *testing.T) {
	t.Parallel()
	srv := newServer(t)

	for name, url := range map[string]string{
		"action reading signals": "/form/submit/",
		"action writing on sse":  "/form/patch/",
	} {
		t.Run(name, func(t *testing.T) {
			req, err := http.NewRequestWithContext(
				context.Background(), http.MethodPost, srv.URL+url, nil,
			)
			require.NoError(t, err, "building request")
			resp, err := srv.Client().Do(req)
			require.NoError(t, err, "POST %s", url)
			defer func() { _ = resp.Body.Close() }()
			require.Equal(t, http.StatusNotAcceptable, resp.StatusCode)
		})
	}
}

// TestSSEOutput covers an action that writes on the connection of its own request.
// The client sent one request and reads elements and signals back from it.
func TestSSEOutput(t *testing.T) {
	t.Parallel()
	srv := newServer(t)

	resp := srv.do(t, http.MethodPost, "/form/patch/", `{"count":41}`)
	defer func() { _ = resp.Body.Close() }()

	requireContentType(t, resp, "text/event-stream")
	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "reading stream")
	stream := string(b)
	for _, want := range []string{
		"event: datastar-patch-elements",
		`<div id="out">count 42</div>`,
		"event: datastar-patch-signals",
		`{"count":42}`,
	} {
		require.Contains(t, stream, want)
	}
	require.Equal(t, "patch count=41", srv.logOf(t))
}

// TestSSEOutputPatchElementAt covers every shape the generated SSE wrapper
// translates PatchElementAt into: a target, a mode, both, and neither.
func TestSSEOutputPatchElementAt(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		signals string
		want    []string
		unwant  []string
	}{
		"selector and mode": {
			signals: `{"selector":"#target","mode":"append"}`,
			want:    []string{"selector #target", "mode append"},
		},
		"selector only": {
			signals: `{"selector":"#target","mode":""}`,
			want:    []string{"selector #target"},
			unwant:  []string{"mode "},
		},
		"mode only": {
			signals: `{"selector":"","mode":"inner"}`,
			want:    []string{"mode inner"},
			unwant:  []string{"selector "},
		},
		"neither": {
			signals: `{"selector":"","mode":""}`,
			unwant:  []string{"selector ", "mode "},
		},
		"unknown mode is ignored": {
			signals: `{"selector":"","mode":"sideways"}`,
			unwant:  []string{"mode ", "selector "},
		},
	} {
		t.Run(name, func(t *testing.T) {
			srv := newServer(t)

			resp := srv.do(t, http.MethodPost, "/form/patch-at/", tc.signals)
			defer func() { _ = resp.Body.Close() }()

			b, err := io.ReadAll(resp.Body)
			require.NoError(t, err, "reading stream")
			stream := string(b)
			require.Contains(t, stream, `<div id="out">patched</div>`)
			for _, want := range tc.want {
				require.Contains(t, stream, want)
			}
			for _, unwant := range tc.unwant {
				require.NotContains(t, stream, unwant)
			}
		})
	}
}

// TestSSEOutputRemoveElement covers the removal event.
func TestSSEOutputRemoveElement(t *testing.T) {
	t.Parallel()
	srv := newServer(t)

	resp := srv.do(t, http.MethodPost, "/form/remove/", "")
	defer func() { _ = resp.Body.Close() }()

	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "reading stream")
	require.Contains(t, string(b),
		"event: datastar-patch-elements\ndata: selector #gone\ndata: mode remove\n")
	require.Equal(t, "remove", srv.logOf(t))
}

// TestSSEOutputSignals covers the signal patches of the SSE wrapper:
// JSON given as json.RawMessage, the if-missing variant,
// and a value that does not marshal.
func TestSSEOutputSignals(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		path       string
		wantStatus int
		want       []string
		unwant     []string
	}{
		"raw json": {
			path:       "/form/signals-raw/",
			wantStatus: http.StatusOK,
			want: []string{
				"event: datastar-patch-signals",
				`data: signals {"count":7}`,
			},
			unwant: []string{"onlyIfMissing"},
		},
		"if missing": {
			path:       "/form/signals-missing/",
			wantStatus: http.StatusOK,
			want: []string{
				"data: onlyIfMissing true",
				`data: signals {"count":3}`,
			},
		},
		// The response head is out before the handler patches, which leaves
		// the error no status to carry. The request must end without a panic
		// that drops the connection.
		"unmarshalable value": {
			path:       "/form/signals-bad/",
			wantStatus: http.StatusOK,
			unwant:     []string{"datastar-patch-signals"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			srv := newServer(t)

			resp := srv.do(t, http.MethodPost, tc.path, "")
			defer func() { _ = resp.Body.Close() }()

			require.Equal(t, tc.wantStatus, resp.StatusCode)
			b, err := io.ReadAll(resp.Body)
			require.NoError(t, err, "reading stream")
			stream := string(b)
			for _, want := range tc.want {
				require.Contains(t, stream, want)
			}
			for _, unwant := range tc.unwant {
				require.NotContains(t, stream, unwant)
			}
		})
	}
}

// TestSSEOutputCompressed covers the same action as a client that accepts compression,
// which every browser does. The events must survive the encoding.
//
// The compressed body ends without its gzip footer, because the response ends
// when the handler returns rather than when the encoder is closed.
// A reader therefore sees every event and then an unexpected EOF.
// That is tolerated here rather than asserted: the events are what the client acts on.
func TestSSEOutputCompressed(t *testing.T) {
	t.Parallel()
	srv := newServer(t)

	req, err := http.NewRequestWithContext(context.Background(),
		http.MethodPost, srv.URL+"/form/patch/", strings.NewReader(`{"count":1}`))
	require.NoError(t, err, "building request")
	req.Header.Set("Datastar-Request", "true")
	req.Header.Set("Content-Type", "application/json")

	resp, err := srv.Client().Do(req)
	require.NoError(t, err, "POST /form/patch/")
	defer func() { _ = resp.Body.Close() }()

	b, err := io.ReadAll(resp.Body)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
		require.NoError(t, err, "reading stream")
	}
	require.Contains(t, string(b), `<div id="out">count 2</div>`,
		"the compressed stream lost the patched element")
}

// TestWrongMethod covers a route reached with a method no action claims.
func TestWrongMethod(t *testing.T) {
	t.Parallel()
	srv := newServer(t)

	status, _ := srv.call(t, http.MethodPost, "/form/replace/", "")
	require.Contains(t,
		[]int{http.StatusMethodNotAllowed, http.StatusNotFound}, status)
	require.Empty(t, srv.logOf(t),
		"an action ran for a method it does not serve")
}

// TestActionExpressions covers the expressions themselves. They go into
// templates as Datastar attribute values. Their exact text is the contract.
func TestActionExpressions(t *testing.T) {
	t.Parallel()
	tests := map[string]struct{ got, want string }{
		"page action": {
			action.POSTPageFormSubmit(),
			"@post('/form/submit/')",
		},
		"app action": {
			action.POSTAppPing(),
			"@post('/ping/')",
		},
		"delete on the app": {
			action.DELETEAppAll(),
			"@delete('/all/')",
		},
		"path variable": {
			action.POSTPageFormBump(7, action.QueryPOSTPageFormBump{}),
			"@post('/form/7/bump/')",
		},
		"path and query": {
			action.POSTPageFormBump(7, action.QueryPOSTPageFormBump{By: 3}),
			"@post('/form/7/bump/?by=3')",
		},
		"with an option": {
			action.POSTPageFormSubmit(action.WithContentType(action.ContentTypeForm)),
			"@post('/form/submit/', {contentType: 'form'})",
		},
		"with a before expression": {
			action.POSTPageFormSubmit(action.WithBefore("$busy = true")),
			"$busy = true; @post('/form/submit/')",
		},
		"with an after expression": {
			action.POSTPageFormSubmit(action.WithAfter("$busy = false")),
			"@post('/form/submit/'); $busy = false",
		},
		"selector": {
			action.POSTPageFormSubmit(action.WithSelector("#form")),
			"@post('/form/submit/', {selector: '#form'})",
		},
		"selector with a quote in it": {
			action.POSTPageFormSubmit(action.WithSelector(`#it's`)),
			`@post('/form/submit/', {selector: '#it\'s'})`,
		},
		"payload": {
			action.POSTPageFormSubmit(action.WithPayload("{id: $id}")),
			"@post('/form/submit/', {payload: {id: $id}})",
		},
		"headers": {
			action.POSTPageFormSubmit(
				action.WithHeaders(map[string]string{"X-Trace": "abc"}),
			),
			"@post('/form/submit/', {headers: {'X-Trace': 'abc'}})",
		},
		"filtered signals": {
			action.POSTPageFormSubmit(action.WithFilterSignals("name", "secret")),
			"@post('/form/submit/', {filterSignals: {include: /name/, exclude: /secret/}})",
		},
		"open when hidden": {
			action.POSTPageFormSubmit(action.WithOpenWhenHidden(true)),
			"@post('/form/submit/', {openWhenHidden: true})",
		},
		"retry settings": {
			action.POSTPageFormSubmit(
				action.WithRetry(action.RetryAlways),
				action.WithRetryInterval(500),
				action.WithRetryScaler(1.5),
				action.WithRetryMaxWaitMs(30000),
				action.WithRetryMaxCount(3),
			),
			"@post('/form/submit/', {retry: 'always', retryInterval: 500, " +
				"retryScaler: 1.5, retryMaxWaitMs: 30000, retryMaxCount: 3})",
		},
		"request cancellation": {
			action.POSTPageFormSubmit(
				action.WithRequestCancellation(action.RequestCancellationDisabled),
			),
			"@post('/form/submit/', {requestCancellation: 'disabled'})",
		},
		"an option with no typed helper": {
			action.POSTPageFormSubmit(action.WithOption("custom", "$x")),
			"@post('/form/submit/', {custom: $x})",
		},
		// The remaining entry points of the generated action package,
		// with the inputs whose handling is a decision rather than a formatting rule.
		"filtered signals defaulting the include": {
			action.POSTPageFormSubmit(action.WithFilterSignals("", "secret")),
			"@post('/form/submit/', {filterSignals: {include: /.*/, exclude: /secret/}})",
		},
		"filtered signals without an exclude": {
			action.POSTPageFormSubmit(action.WithFilterSignals("name", "")),
			"@post('/form/submit/', {filterSignals: {include: /name/}})",
		},
		"a cancellation controller of the caller's own": {
			action.POSTPageFormSubmit(
				action.WithRequestCancellationController("$myController"),
			),
			"@post('/form/submit/', {requestCancellation: $myController})",
		},
		"retries capped at none": {
			action.POSTPageFormSubmit(action.WithRetryMaxCount(0)),
			"@post('/form/submit/', {retryMaxCount: 0})",
		},
		"before and after around options": {
			action.POSTPageFormSubmit(
				action.WithBefore("$busy = true"),
				action.WithContentType(action.ContentTypeForm),
				action.WithAfter("$busy = false"),
			),
			"$busy = true; @post('/form/submit/', {contentType: 'form'}); $busy = false",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.got)
		})
	}
}

// requireContentType fails unless the response was written with the given media type.
// The header also carries the charset, which is why it is a prefix match.
func requireContentType(t *testing.T, resp *http.Response, want string) {
	t.Helper()
	got := resp.Header.Get("Content-Type")
	require.True(t, strings.HasPrefix(got, want),
		"Content-Type = %q, want prefix %q", got, want)
}

// urlOf takes the URL out of a Datastar action expression such as
// "@post('/form/submit/')". That is the form a template carries.
func urlOf(t *testing.T, expr string) string {
	t.Helper()
	i := strings.Index(expr, "('")
	j := strings.LastIndex(expr, "')")
	require.False(t, i < 0 || j < i, "not an action expression: %s", expr)
	return expr[i+2 : j]
}

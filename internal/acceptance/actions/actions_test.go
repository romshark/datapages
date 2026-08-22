// Drives the generated action handlers of ./app.

package acceptance_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
	if err != nil {
		t.Fatalf("building %s %s: %v", method, url, err)
	}
	req.Header.Set("Datastar-Request", "true")
	// Responses are compressed by default. Asking for none of it keeps the
	// bytes this test reads the bytes the handler wrote.
	req.Header.Set("Accept-Encoding", "identity")
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := s.Client().Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	return resp
}

// call sends an action request and returns its status and body.
func (s server) call(t *testing.T, method, url, body string) (int, string) {
	t.Helper()
	resp := s.do(t, method, url, body)
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading body of %s %s: %v", method, url, err)
	}
	return resp.StatusCode, string(b)
}

// logOf reads back what the actions recorded.
func (s server) logOf(t *testing.T) string {
	t.Helper()
	resp, err := s.Client().Get(s.URL + "/log/")
	if err != nil {
		t.Fatalf("GET /log/: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading /log/: %v", err)
	}
	return echoed(t, string(b))
}

func echoed(t *testing.T, body string) string {
	t.Helper()
	const open = `<pre id="echo">`
	_, after, ok := strings.Cut(body, open)
	if !ok {
		t.Fatalf("no echo element in response:\n%s", body)
	}
	rest := after
	before, _, ok := strings.Cut(rest, "</pre>")
	if !ok {
		t.Fatalf("unterminated echo element in response:\n%s", body)
	}
	return before
}

// TestMethods covers one action per HTTP method, page-level and app-level.
// The action package builds the URL the template would use. The routing of an
// action is therefore asserted through the same expression a page carries.
func TestMethods(t *testing.T) {
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
			if st, body := srv.call(t, tt.method, url, tt.body); st != http.StatusOK {
				t.Fatalf("%s %s: status = %d, want 200\n%s",
					tt.method, url, st, body)
			}
			if got := srv.logOf(t); got != tt.want {
				t.Errorf(" got: %s\nwant: %s", got, tt.want)
			}
		})
	}
}

// TestParameters covers path variables and query parameters on an action.
func TestParameters(t *testing.T) {
	srv := newServer(t)

	url := urlOf(t, action.POSTPageFormBump(7, action.QueryPOSTPageFormBump{By: 3}))
	if status, body := srv.call(t, http.MethodPost, url, ""); status != http.StatusOK {
		t.Fatalf("POST %s: status = %d, want 200\n%s", url, status, body)
	}
	if got, want := srv.logOf(t), "bump id=7 by=3"; got != want {
		t.Errorf(" got: %s\nwant: %s", got, want)
	}
}

// TestSignalsAreRequired covers a request whose body is not the signals the
// handler declares.
func TestSignalsAreRequired(t *testing.T) {
	srv := newServer(t)

	url := urlOf(t, action.POSTPageFormSubmit())
	status, _ := srv.call(t, http.MethodPost, url, `{"age":"not a number"}`)
	if status != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", status, http.StatusBadRequest)
	}
	if got := srv.logOf(t); got != "" {
		t.Errorf("the handler ran on a body it could not read: %s", got)
	}
}

// TestBodyOutput covers an action that answers with a document.
func TestBodyOutput(t *testing.T) {
	srv := newServer(t)

	status, body := srv.call(t, http.MethodPost, "/form/render/", "")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200\n%s", status, body)
	}
	if got, want := echoed(t, body), "rendered by an action"; got != want {
		t.Errorf(" got: %s\nwant: %s", got, want)
	}
}

// TestGlobalHead covers the head the app declares once for everything it renders,
// on a page load and on a rendering action alike.
func TestGlobalHead(t *testing.T) {
	srv := newServer(t)

	for name, req := range map[string]struct{ method, url string }{
		"page load": {http.MethodGet, "/"},
		"action":    {http.MethodPost, "/form/render/"},
	} {
		t.Run(name, func(t *testing.T) {
			status, body := srv.call(t, req.method, req.url, "")
			if status != http.StatusOK {
				t.Fatalf("status = %d, want 200\n%s", status, body)
			}
			if !strings.Contains(body, "<title>actions</title>") {
				t.Errorf("the global head is not in the response:\n%s", body)
			}
		})
	}
}

// TestRedirect covers the two ways a redirect leaves the server.
// A Datastar request cannot follow a 303: the browser would replace the fragment,
// not the page. It gets a script instead.
func TestRedirect(t *testing.T) {
	t.Run("datastar request navigates by script", func(t *testing.T) {
		srv := newServer(t)
		resp := srv.do(t, http.MethodPost, "/form/go/", "")
		defer func() { _ = resp.Body.Close() }()

		if got, want := resp.Header.Get("Content-Type"),
			"text/javascript"; !strings.HasPrefix(got, want) {
			t.Errorf("Content-Type = %q, want prefix %q", got, want)
		}
		b, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("reading body: %v", err)
		}
		if got, want := string(b), `window.location = "/log/";`; got != want {
			t.Errorf(" got: %s\nwant: %s", got, want)
		}
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
		if err != nil {
			t.Fatalf("building request: %v", err)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("POST /form/go/: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusSeeOther {
			t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusSeeOther)
		}
		if got, want := resp.Header.Get("Location"), "/log/"; got != want {
			t.Errorf("Location = %q, want %q", got, want)
		}
	})
}

// TestDatastarOnlyActions covers the actions that cannot serve a plain request:
// reading signals and answering on an SSE connection both require the Datastar client.
func TestDatastarOnlyActions(t *testing.T) {
	srv := newServer(t)

	for name, url := range map[string]string{
		"action reading signals": "/form/submit/",
		"action writing on sse":  "/form/patch/",
	} {
		t.Run(name, func(t *testing.T) {
			req, err := http.NewRequestWithContext(
				context.Background(), http.MethodPost, srv.URL+url, nil,
			)
			if err != nil {
				t.Fatalf("building request: %v", err)
			}
			resp, err := srv.Client().Do(req)
			if err != nil {
				t.Fatalf("POST %s: %v", url, err)
			}
			defer func() { _ = resp.Body.Close() }()
			if resp.StatusCode != http.StatusNotAcceptable {
				t.Errorf("status = %d, want %d",
					resp.StatusCode, http.StatusNotAcceptable)
			}
		})
	}
}

// TestSSEOutput covers an action that writes on the connection of its own request.
// The client sent one request and reads elements and signals back from it.
func TestSSEOutput(t *testing.T) {
	srv := newServer(t)

	resp := srv.do(t, http.MethodPost, "/form/patch/", `{"count":41}`)
	defer func() { _ = resp.Body.Close() }()

	if got, want := resp.Header.Get("Content-Type"),
		"text/event-stream"; !strings.HasPrefix(got, want) {
		t.Errorf("Content-Type = %q, want prefix %q", got, want)
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading stream: %v", err)
	}
	stream := string(b)
	for _, want := range []string{
		"event: datastar-patch-elements",
		`<div id="out">count 42</div>`,
		"event: datastar-patch-signals",
		`{"count":42}`,
	} {
		if !strings.Contains(stream, want) {
			t.Errorf("stream does not carry %q:\n%s", want, stream)
		}
	}
	if got, want := srv.logOf(t), "patch count=41"; got != want {
		t.Errorf(" got: %s\nwant: %s", got, want)
	}
}

// TestSSEOutputPatchElementAt covers every shape the generated SSE wrapper
// translates PatchElementAt into: a target, a mode, both, and neither.
func TestSSEOutputPatchElementAt(t *testing.T) {
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
			if err != nil {
				t.Fatalf("reading stream: %v", err)
			}
			stream := string(b)
			if !strings.Contains(stream, `<div id="out">patched</div>`) {
				t.Errorf("stream carries no patched element:\n%s", stream)
			}
			for _, want := range tc.want {
				if !strings.Contains(stream, want) {
					t.Errorf("stream does not carry %q:\n%s", want, stream)
				}
			}
			for _, unwant := range tc.unwant {
				if strings.Contains(stream, unwant) {
					t.Errorf("stream carries %q:\n%s", unwant, stream)
				}
			}
		})
	}
}

// TestSSEOutputRemoveElement covers the removal event.
func TestSSEOutputRemoveElement(t *testing.T) {
	srv := newServer(t)

	resp := srv.do(t, http.MethodPost, "/form/remove/", "")
	defer func() { _ = resp.Body.Close() }()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading stream: %v", err)
	}
	if got, want := string(b),
		"event: datastar-patch-elements\ndata: selector #gone\ndata: mode remove\n"; !strings.Contains(got, want) {
		t.Errorf("stream does not carry %q:\n%s", want, got)
	}
	if got, want := srv.logOf(t), "remove"; got != want {
		t.Errorf(" got: %s\nwant: %s", got, want)
	}
}

// TestSSEOutputSignals covers the signal patches of the SSE wrapper:
// JSON given as json.RawMessage, the if-missing variant,
// and a value that does not marshal.
func TestSSEOutputSignals(t *testing.T) {
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

			if resp.StatusCode != tc.wantStatus {
				t.Errorf("status = %d, want %d", resp.StatusCode, tc.wantStatus)
			}
			b, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("reading stream: %v", err)
			}
			stream := string(b)
			for _, want := range tc.want {
				if !strings.Contains(stream, want) {
					t.Errorf("stream does not carry %q:\n%s", want, stream)
				}
			}
			for _, unwant := range tc.unwant {
				if strings.Contains(stream, unwant) {
					t.Errorf("stream carries %q:\n%s", unwant, stream)
				}
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
	srv := newServer(t)

	req, err := http.NewRequestWithContext(context.Background(),
		http.MethodPost, srv.URL+"/form/patch/", strings.NewReader(`{"count":1}`))
	if err != nil {
		t.Fatalf("building request: %v", err)
	}
	req.Header.Set("Datastar-Request", "true")
	req.Header.Set("Content-Type", "application/json")

	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("POST /form/patch/: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	b, err := io.ReadAll(resp.Body)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("reading stream: %v", err)
	}
	if got, want := string(b), `<div id="out">count 2</div>`; !strings.Contains(got, want) {
		t.Errorf("compressed stream does not carry %q:\n%s", want, got)
	}
}

// TestWrongMethod covers a route reached with a method no action claims.
func TestWrongMethod(t *testing.T) {
	srv := newServer(t)

	status, _ := srv.call(t, http.MethodPost, "/form/replace/", "")
	if status != http.StatusMethodNotAllowed && status != http.StatusNotFound {
		t.Errorf("status = %d, want %d or %d", status,
			http.StatusMethodNotAllowed, http.StatusNotFound)
	}
	if got := srv.logOf(t); got != "" {
		t.Errorf("an action ran for a method it does not serve: %s", got)
	}
}

// TestActionExpressions covers the expressions themselves. They go into
// templates as Datastar attribute values. Their exact text is the contract.
func TestActionExpressions(t *testing.T) {
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
			if tt.got != tt.want {
				t.Errorf(" got: %s\nwant: %s", tt.got, tt.want)
			}
		})
	}
}

// urlOf takes the URL out of a Datastar action expression such as
// "@post('/form/submit/')". That is the form a template carries.
func urlOf(t *testing.T, expr string) string {
	t.Helper()
	i := strings.Index(expr, "('")
	j := strings.LastIndex(expr, "')")
	if i < 0 || j < i {
		t.Fatalf("not an action expression: %s", expr)
	}
	return expr[i+2 : j]
}

// Tests the generated action handlers of ./app.

package acceptance_test

import (
	"compress/gzip"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/datapagestest"
	"github.com/romshark/datapages/internal/acceptance/actions/app"
	"github.com/romshark/datapages/internal/acceptance/actions/app/datapagesgen/action"
	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/messaging/inmem"
)

func serve(t *testing.T, opts ...datapages.ServerOption) *datapagestest.Server {
	t.Helper()
	return datapagestest.New(t, mustNewServer(t, &app.App{},
		inmem.New(messaging.DefaultBrokerChanBuffer), opts...))
}

// logOf reads back what the actions recorded.
func logOf(t *testing.T, v *datapagestest.Visitor) string {
	t.Helper()
	return datapagestest.Text(v.Get(t, "/log/").ElementByID(t, "echo"))
}

// TestMethods tests one action per HTTP method, page-level and app-level.
// The action package builds the URL the template would use. The routing of an
// action is therefore asserted through the same expression a page carries.
func TestMethods(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		method  string
		expr    string
		signals any
		want    string
	}{
		"post on a page": {
			method:  http.MethodPost,
			expr:    action.PageForm.Submit.POST(),
			signals: map[string]any{"name": "ada", "age": 36},
			want:    `submit name="ada" age=36`,
		},
		"put on a page": {
			method: http.MethodPut,
			expr:   action.PageForm.Replace.PUT(),
			want:   "replace",
		},
		"patch on a page": {
			method: http.MethodPatch,
			expr:   action.PageForm.Touch.PATCH(),
			want:   "touch",
		},
		"delete on a page": {
			method: http.MethodDelete,
			expr:   action.PageForm.Remove.DELETE(),
			want:   "remove",
		},
		"post on the app": {
			method: http.MethodPost,
			expr:   action.App.Ping.POST(),
			want:   "ping",
		},
		"query on a page": {
			method:  "QUERY",
			expr:    action.PageForm.Search.QUERY(),
			signals: map[string]string{"term": "ada"},
			want:    `search term="ada"`,
		},
		"query on the app": {
			method: "QUERY",
			expr:   action.App.Lookup.QUERY(),
			want:   "lookup",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			v := serve(t).Visitor(t)
			url := urlOf(t, tt.expr)
			resp := v.Load(t, "/form/").Do(t, tt.method, url, tt.signals)
			require.Equal(t, http.StatusOK, resp.Status,
				"%s %s\n%s", tt.method, url, resp.Body)
			require.Equal(t, tt.want, logOf(t, v))
		})
	}
}

// TestParameters tests path variables and query parameters on an action.
func TestParameters(t *testing.T) {
	t.Parallel()
	v := serve(t).Visitor(t)

	url := urlOf(t, action.PageForm.Bump.POST(7, action.PageForm.Bump.POSTQuery(3)))
	resp := v.Load(t, "/form/").Do(t, http.MethodPost, url, nil)
	require.Equal(t, http.StatusOK, resp.Status, "POST %s\n%s", url, resp.Body)
	require.Equal(t, "bump id=7 by=3", logOf(t, v))
}

// TestSignalsAreRequired tests a request whose body is not the signals the
// handler declares.
func TestSignalsAreRequired(t *testing.T) {
	t.Parallel()
	v := serve(t).Visitor(t)

	url := urlOf(t, action.PageForm.Submit.POST())
	resp := v.Load(t, "/form/").Do(t, http.MethodPost, url,
		map[string]string{"age": "not a number"})
	require.Equal(t, http.StatusBadRequest, resp.Status)
	require.Empty(t, logOf(t, v), "the handler ran on a body it could not read")
}

// TestBodySizeLimit tests the limit on a signals body, on an action that
// opens an SSE on its own request as well as on one that does not.
// Both read the signals before anything else, hence the limit costs the SSE nothing.
func TestBodySizeLimit(t *testing.T) {
	t.Parallel()
	// datapagesgen.DefaultBodySizeLimit is 1 MiB.
	signals := map[string]any{"count": 1, "pad": strings.Repeat("a", 2*1024*1024)}
	for name, expr := range map[string]string{
		"signals":     action.PageForm.Submit.POST(),
		"signals+sse": action.PageForm.Patch.POST(),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			v := serve(t).Visitor(t)
			resp := v.Load(t, "/form/").Do(t, http.MethodPost, urlOf(t, expr), signals)
			require.Equal(t, http.StatusBadRequest, resp.Status)
			require.Empty(t, logOf(t, v), "the handler ran on an oversized body")
		})
	}
}

// TestBodySizeLimitOption tests datapages.WithBodySizeLimit, which is what an
// application raises when its signals carry more than the default allows.
func TestBodySizeLimitOption(t *testing.T) {
	t.Parallel()
	const limit = 4096
	form := serve(t, datapages.WithBodySizeLimit(limit)).Visitor(t).Load(t, "/form/")

	pad := func(n int) map[string]any {
		return map[string]any{"count": 1, "pad": strings.Repeat("a", n)}
	}
	url := urlOf(t, action.PageForm.Submit.POST())

	resp := form.Do(t, http.MethodPost, url, pad(limit/2))
	require.Equal(t, http.StatusOK, resp.Status, "a body under the limit")

	resp = form.Do(t, http.MethodPost, url, pad(2*limit))
	require.Equal(t, http.StatusBadRequest, resp.Status, "a body over the limit")
}

// TestBodyOutput tests an action that answers with a document.
func TestBodyOutput(t *testing.T) {
	t.Parallel()
	form := serve(t).Visitor(t).Load(t, "/form/")

	resp := form.Do(t, http.MethodPost, "/form/render/", nil)
	require.Equal(t, http.StatusOK, resp.Status, "%s", resp.Body)
	require.Equal(t, "rendered by an action",
		datapagestest.Text(resp.ElementByID(t, "echo")))
}

// TestGlobalHead tests the head the app declares once for everything it renders,
// on a page load and on a rendering action alike.
func TestGlobalHead(t *testing.T) {
	t.Parallel()
	v := serve(t).Visitor(t)

	for name, resp := range map[string]*datapagestest.Response{
		"page load": v.Get(t, "/"),
		"action":    v.Load(t, "/form/").Do(t, http.MethodPost, "/form/render/", nil),
	} {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, http.StatusOK, resp.Status, "%s", resp.Body)
			require.Contains(t, resp.Body, "<title>actions</title>",
				"the global head is not in the response")
		})
	}
}

// TestRedirect tests the script a Datastar request is sent and
// the 303 a plain one is sent.  A Datastar request cannot follow a 303:
// the browser would replace the fragment, not the page. It gets a script instead.
func TestRedirect(t *testing.T) {
	t.Parallel()
	t.Run("datastar request navigates by script", func(t *testing.T) {
		form := serve(t).Visitor(t).Load(t, "/form/")
		resp := form.Do(t, http.MethodPost, "/form/go/", nil)

		requireContentType(t, resp, "text/javascript")
		require.Equal(t, `window.location = "/log/";`, resp.Body)
	})

	// An action holding the stream of its own request has no response head
	// left to redirect with, hence the navigation is an event of the stream.
	t.Run("stream request navigates through the stream", func(t *testing.T) {
		form := serve(t).Visitor(t).Load(t, "/form/")
		resp := form.Do(t, http.MethodPost, "/form/go-stream/", nil)

		requireContentType(t, resp, "text/event-stream")
		require.Equal(t, "/log/", resp.Redirect(t))
	})

	// An action that needs neither signals nor an SSE connection is reachable
	// by a plain form post, and that client can only be moved by a real redirect.
	// The status the handler asked for is the status it must send.
	t.Run("plain request gets the status it asked for", func(t *testing.T) {
		v := serve(t).Visitor(t)
		resp := v.Send(t, v.NewRequest(t, http.MethodPost, "/form/go/", nil))
		require.Equal(t, http.StatusSeeOther, resp.Status)
		require.Equal(t, "/log/", resp.Header.Get("Location"))
	})
}

// TestDatastarOnlyActions tests the actions that cannot serve a plain request:
// reading signals and answering on an SSE connection both require the Datastar client.
func TestDatastarOnlyActions(t *testing.T) {
	t.Parallel()
	v := serve(t).Visitor(t)

	for name, tc := range map[string]struct{ method, url string }{
		"action reading signals":       {http.MethodPost, "/form/submit/"},
		"action writing on sse":        {http.MethodPost, "/form/patch/"},
		"query action reading signals": {"QUERY", "/form/search/"},
	} {
		t.Run(name, func(t *testing.T) {
			resp := v.Send(t, v.NewRequest(t, tc.method, tc.url, nil))
			require.Equal(t, http.StatusNotAcceptable, resp.Status)
		})
	}
}

// TestCrossOriginFormAction tests the action that carries no Datastar check.
// A plain form reaches it, hence so does a form on another site,
// which the browser reports through Sec-Fetch-Site and Origin.
func TestCrossOriginFormAction(t *testing.T) {
	t.Parallel()
	v := serve(t).Visitor(t)

	for name, tc := range map[string]struct {
		headers    map[string]string
		wantStatus int
	}{
		"cross site": {
			headers:    map[string]string{"Sec-Fetch-Site": "cross-site"},
			wantStatus: http.StatusForbidden,
		},
		"foreign origin": {
			headers:    map[string]string{"Origin": "https://evil.example"},
			wantStatus: http.StatusForbidden,
		},
		"same origin": {
			headers:    map[string]string{"Sec-Fetch-Site": "same-origin"},
			wantStatus: http.StatusSeeOther,
		},
		"no origin headers": {wantStatus: http.StatusSeeOther},
	} {
		t.Run(name, func(t *testing.T) {
			req := v.NewRequest(t, http.MethodPost, "/form/go/", nil)
			for k, value := range tc.headers {
				req.Header.Set(k, value)
			}
			require.Equal(t, tc.wantStatus, v.Send(t, req).Status)
		})
	}
}

// TestCrossOriginQuery tests a QUERY action that takes no signals. A request
// without the Datastar header reaches it, and the origin check refuses one from
// another site: [http.CrossOriginProtection] exempts only GET, HEAD and OPTIONS.
// A Go release that also exempts QUERY fails this test.
func TestCrossOriginQuery(t *testing.T) {
	t.Parallel()
	v := serve(t).Visitor(t)

	for name, tc := range map[string]struct {
		site       string
		wantStatus int
	}{
		"cross site":  {site: "cross-site", wantStatus: http.StatusForbidden},
		"same site":   {site: "same-site", wantStatus: http.StatusForbidden},
		"same origin": {site: "same-origin", wantStatus: http.StatusOK},
	} {
		t.Run(name, func(t *testing.T) {
			req := v.NewRequest(t, "QUERY", "/lookup/", nil)
			req.Header.Set("Sec-Fetch-Site", tc.site)
			require.Equal(t, tc.wantStatus, v.Send(t, req).Status)
		})
	}
}

// TestSSEOutput tests an action that writes on the connection of its own request.
// The client sent one request and reads elements and signals back from it.
func TestSSEOutput(t *testing.T) {
	t.Parallel()
	v := serve(t).Visitor(t)

	resp := v.Load(t, "/form/").Do(t, http.MethodPost, "/form/patch/",
		map[string]int{"count": 41})

	evs := resp.Events(t)
	require.Len(t, evs, 2, "want the element patch, then the signal patch")
	require.Equal(t, datapagestest.PatchElements, evs[0].Type)
	require.Equal(t, "count 42", datapagestest.Text(evs[0].ElementByID(t, "out")))
	require.Equal(t, datapagestest.PatchSignals, evs[1].Type)
	require.JSONEq(t, `{"count":42}`, evs[1].Data["signals"])
	require.Equal(t, "patch count=41", logOf(t, v))
}

// TestSSEOutputPatchElementAt tests every shape the generated SSE wrapper
// translates PatchElementAt into: a target, a mode, both, and neither.
func TestSSEOutputPatchElementAt(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		signals map[string]string
		want    map[string]string // the event's data fields by name
		unwant  []string          // data fields the event must not carry
	}{
		"selector and mode": {
			signals: map[string]string{"selector": "#target", "mode": "append"},
			want:    map[string]string{"selector": "#target", "mode": "append"},
		},
		"selector only": {
			signals: map[string]string{"selector": "#target", "mode": ""},
			want:    map[string]string{"selector": "#target"},
			unwant:  []string{"mode"},
		},
		"mode only": {
			signals: map[string]string{"selector": "", "mode": "inner"},
			want:    map[string]string{"mode": "inner"},
			unwant:  []string{"selector"},
		},
		"neither": {
			signals: map[string]string{"selector": "", "mode": ""},
			unwant:  []string{"selector", "mode"},
		},
		"unknown mode is ignored": {
			signals: map[string]string{"selector": "", "mode": "sideways"},
			unwant:  []string{"mode", "selector"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			form := serve(t).Visitor(t).Load(t, "/form/")

			resp := form.Do(t, http.MethodPost, "/form/patch-at/", tc.signals)

			evs := resp.Events(t)
			require.Len(t, evs, 1)
			ev := evs[0]
			require.Equal(t, "patched", datapagestest.Text(ev.ElementByID(t, "out")))
			for field, want := range tc.want {
				require.Equal(t, want, ev.Data[field], "data field %q", field)
			}
			for _, field := range tc.unwant {
				require.NotContains(t, ev.Data, field)
			}
		})
	}
}

// TestSSEOutputRemoveElement tests the removal event.
func TestSSEOutputRemoveElement(t *testing.T) {
	t.Parallel()
	v := serve(t).Visitor(t)

	resp := v.Load(t, "/form/").Do(t, http.MethodPost, "/form/remove/", nil)

	require.Contains(t, resp.Body,
		"event: datastar-patch-elements\ndata: selector #gone\ndata: mode remove\n")
	require.Equal(t, "remove", logOf(t, v))
}

// TestSSEOutputSignals tests the signal patches of the SSE wrapper:
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
			form := serve(t).Visitor(t).Load(t, "/form/")

			resp := form.Do(t, http.MethodPost, tc.path, nil)

			require.Equal(t, tc.wantStatus, resp.Status)
			for _, want := range tc.want {
				require.Contains(t, resp.Body, want)
			}
			for _, unwant := range tc.unwant {
				require.NotContains(t, resp.Body, unwant)
			}
		})
	}
}

// TestSSEOutputCompressed tests the same action as a client that accepts compression,
// which every browser does. The events must survive the encoding.
//
// The compressed body ends without its gzip footer, because the response ends
// when the handler returns rather than when the encoder is closed.
// A reader therefore sees every event and then an unexpected EOF.
// That is tolerated here rather than asserted: the events are what the client acts on.
func TestSSEOutputCompressed(t *testing.T) {
	t.Parallel()
	v := serve(t).Visitor(t)
	form := v.Load(t, "/form/")

	req := form.NewRequest(t, http.MethodPost, "/form/patch/",
		strings.NewReader(`{"count":1}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept-Encoding", "gzip")
	resp := v.Send(t, req)
	require.Equal(t, "gzip", resp.Header.Get("Content-Encoding"),
		"the stream was not compressed")

	zr, err := gzip.NewReader(strings.NewReader(resp.Body))
	require.NoError(t, err, "reading the gzip header")
	b, err := io.ReadAll(zr)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
		require.NoError(t, err, "decoding stream")
	}
	require.Contains(t, string(b), `<div id="out">count 2</div>`,
		"the compressed stream lost the patched element")
}

// TestWrongMethod tests a route reached with a method no action claims.
func TestWrongMethod(t *testing.T) {
	t.Parallel()
	v := serve(t).Visitor(t)

	resp := v.Load(t, "/form/").Do(t, http.MethodPost, "/form/replace/", nil)
	require.Contains(t,
		[]int{http.StatusMethodNotAllowed, http.StatusNotFound}, resp.Status)
	require.Empty(t, logOf(t, v),
		"an action ran for a method it does not serve")
}

// TestActionExpressions tests the expressions themselves.
// They go into templates as Datastar attribute values. Their exact text is the contract.
func TestActionExpressions(t *testing.T) {
	t.Parallel()
	tests := map[string]struct{ got, want string }{
		"page action": {
			action.PageForm.Submit.POST(),
			"@post('/form/submit/')",
		},
		"app action": {
			action.App.Ping.POST(),
			"@post('/ping/')",
		},
		"delete on the app": {
			action.App.All.DELETE(),
			"@delete('/all/')",
		},
		"query on a page": {
			action.PageForm.Search.QUERY(),
			"@query('/form/search/')",
		},
		"path variable": {
			action.PageForm.Bump.POST(7, action.PageForm.Bump.POSTQuery(0)),
			"@post('/form/7/bump/')",
		},
		"path and query": {
			action.PageForm.Bump.POST(7, action.PageForm.Bump.POSTQuery(3)),
			"@post('/form/7/bump/?by=3')",
		},
		"with an option": {
			action.PageForm.Submit.POST(action.WithContentType(action.ContentTypeForm)),
			"@post('/form/submit/', {contentType: 'form'})",
		},
		"with a before expression": {
			action.PageForm.Submit.POST(action.WithBefore("$busy = true")),
			"$busy = true; @post('/form/submit/')",
		},
		"with an after expression": {
			action.PageForm.Submit.POST(action.WithAfter("$busy = false")),
			"@post('/form/submit/'); $busy = false",
		},
		"selector": {
			action.PageForm.Submit.POST(action.WithSelector("#form")),
			"@post('/form/submit/', {selector: '#form'})",
		},
		"selector with a quote in it": {
			action.PageForm.Submit.POST(action.WithSelector(`#it's`)),
			`@post('/form/submit/', {selector: '#it\'s'})`,
		},
		"payload": {
			action.PageForm.Submit.POST(action.WithPayload("{id: $id}")),
			"@post('/form/submit/', {payload: {id: $id}})",
		},
		"headers": {
			action.PageForm.Submit.POST(
				action.WithHeaders(map[string]string{"X-Trace": "abc"}),
			),
			"@post('/form/submit/', {headers: {'X-Trace': 'abc'}})",
		},
		"filtered signals": {
			action.PageForm.Submit.POST(action.WithFilterSignals("name", "secret")),
			"@post('/form/submit/', {filterSignals: {include: /name/, exclude: /secret/}})",
		},
		"open when hidden": {
			action.PageForm.Submit.POST(action.WithOpenWhenHidden(true)),
			"@post('/form/submit/', {openWhenHidden: true})",
		},
		"retry settings": {
			action.PageForm.Submit.POST(
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
			action.PageForm.Submit.POST(
				action.WithRequestCancellation(action.RequestCancellationDisabled),
			),
			"@post('/form/submit/', {requestCancellation: 'disabled'})",
		},
		"an option with no typed helper": {
			action.PageForm.Submit.POST(action.WithOption("custom", "$x")),
			"@post('/form/submit/', {custom: $x})",
		},
		// The remaining entry points of the generated action package,
		// with the inputs whose handling is a decision rather than a formatting rule.
		"filtered signals defaulting the include": {
			action.PageForm.Submit.POST(action.WithFilterSignals("", "secret")),
			"@post('/form/submit/', {filterSignals: {include: /.*/, exclude: /secret/}})",
		},
		"filtered signals without an exclude": {
			action.PageForm.Submit.POST(action.WithFilterSignals("name", "")),
			"@post('/form/submit/', {filterSignals: {include: /name/}})",
		},
		"a cancellation controller of the caller's own": {
			action.PageForm.Submit.POST(
				action.WithRequestCancellationController("$myController"),
			),
			"@post('/form/submit/', {requestCancellation: $myController})",
		},
		"retries capped at none": {
			action.PageForm.Submit.POST(action.WithRetryMaxCount(0)),
			"@post('/form/submit/', {retryMaxCount: 0})",
		},
		"before and after around options": {
			action.PageForm.Submit.POST(
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
func requireContentType(t *testing.T, resp *datapagestest.Response, want string) {
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

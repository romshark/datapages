package datapagestest_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/datapagestest"
	"github.com/romshark/datapages/modules/csrf"
	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/messaging/inmem"
)

// page answers with markup and, when instance is set,
// the Datapages-Instance header of a page with per-tab state.
func page(markup, instance string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		if instance != "" {
			w.Header().Set("Datapages-Instance", instance)
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, markup)
	}
}

// stream answers a Datastar request with the SSE body.
// It keeps the connection open until the client leaves, unless end is set.
func stream(body string, end bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Datastar-Request") != "true" {
			http.Error(w, "Not Acceptable", http.StatusNotAcceptable)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, body)
		w.(http.Flusher).Flush()
		if !end {
			<-r.Context().Done()
		}
	}
}

// patch is a PatchElements event as the Datastar SDK writes it.
func patch(elements string) string {
	return "event: datastar-patch-elements\ndata: elements " + elements + "\n\n\n"
}

// echoed is what echo received.
type echoed struct {
	Method          string
	Query           string
	Datastar        string
	Body            string
	ContentType     string
	DatastarRequest string
	Instance        string
	CSRF            string
	AcceptEncoding  string
}

func echo(w http.ResponseWriter, r *http.Request) {
	b, _ := io.ReadAll(r.Body)
	_ = json.NewEncoder(w).Encode(echoed{
		Method:          r.Method,
		Query:           r.URL.RawQuery,
		Datastar:        r.URL.Query().Get("datastar"),
		Body:            string(b),
		ContentType:     r.Header.Get("Content-Type"),
		DatastarRequest: r.Header.Get("Datastar-Request"),
		Instance:        r.Header.Get("Datapages-Instance"),
		CSRF:            r.Header.Get("X-CSRF-Token"),
		AcceptEncoding:  r.Header.Get("Accept-Encoding"),
	})
}

func decodeEcho(t *testing.T, resp *datapagestest.Response) echoed {
	t.Helper()
	require.Equal(t, http.StatusOK, resp.Status, resp.Body)
	var e echoed
	require.NoError(t, json.Unmarshal([]byte(resp.Body), &e))
	return e
}

// failure runs f with a testing.TB that records the first fatal failure
// instead of failing t, and returns its message, "" when f passed.
func failure(t *testing.T, f func(testing.TB)) string {
	t.Helper()
	r := &recorder{TB: t}
	done := make(chan struct{})
	go func() {
		defer close(done)
		f(r)
	}()
	<-done
	return r.msg
}

// recorder is a testing.TB whose Fatalf records the message and
// ends the goroutine that called it, the way testing.T.Fatalf does.
type recorder struct {
	testing.TB
	msg string
}

func (r *recorder) Fatalf(format string, args ...any) {
	r.msg = fmt.Sprintf(format, args...)
	runtime.Goexit()
}

// TestDoPlacesSignals tests that an action sends its signals where Datastar sends
// them for its method, with the headers of a Datastar request of the tab.
func TestDoPlacesSignals(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("/p/", page(`<p>page</p>`, "inst-1"))
	mux.HandleFunc("/echo/", echo)
	tab := datapagestest.New(t, mux).Visitor(t).Load(t, "/p/")

	tests := map[string]struct {
		method, path string
		signals      any
		body         string
		datastar     string
		contentType  string
	}{
		"post": {
			method: http.MethodPost, path: "/echo/", signals: map[string]int{"n": 1},
			body: `{"n":1}`, contentType: "application/json",
		},
		"nil signals": {
			method: http.MethodPatch, path: "/echo/",
			body: "{}", contentType: "application/json",
		},
		"raw json": {
			method: http.MethodPut, path: "/echo/", signals: json.RawMessage(`{"n":`),
			body: `{"n":`, contentType: "application/json",
		},
		"delete": {
			method: http.MethodDelete, path: "/echo/?keep=1",
			signals: map[string]bool{"confirm": true}, datastar: `{"confirm":true}`,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			e := decodeEcho(t, tab.Do(t, tt.method, tt.path, tt.signals))
			require.Equal(t, tt.method, e.Method)
			require.Equal(t, tt.body, e.Body)
			require.Equal(t, tt.datastar, e.Datastar)
			require.Equal(t, tt.contentType, e.ContentType)
			require.Equal(t, "true", e.DatastarRequest)
			require.Equal(t, "inst-1", e.Instance)
			require.Equal(t, "identity", e.AcceptEncoding)
			require.Empty(t, e.CSRF, "a guest sends no CSRF token")
			if tt.method == http.MethodDelete {
				require.Contains(t, e.Query, "keep=1", "the path's own query is kept")
			}
		})
	}
}

// TestDoFormPlacesFields tests that a form action sends urlencoded fields in the body,
// and in the query of a DELETE.
func TestDoFormPlacesFields(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("/p/", page(`<p>page</p>`, ""))
	mux.HandleFunc("/echo/", echo)
	tab := datapagestest.New(t, mux).Visitor(t).Load(t, "/p/")

	e := decodeEcho(t, tab.DoForm(t, http.MethodPost, "/echo/", map[string][]string{
		"name": {"a b"},
	}))
	require.Equal(t, "name=a+b", e.Body)
	require.Equal(t, "application/x-www-form-urlencoded", e.ContentType)

	e = decodeEcho(t, tab.DoForm(t, http.MethodDelete, "/echo/", map[string][]string{
		"name": {"a"},
	}))
	require.Empty(t, e.Body)
	require.Equal(t, "name=a", e.Query)
}

// TestCSRFToken tests that a signed-in visitor's actions carry the token the default
// CSRF configuration derives from the Secure session cookie, and that a guest's and a
// GET's do not.
func TestCSRFToken(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("/p/", page(`<p>page</p>`, ""))
	mux.HandleFunc("/echo/", echo)
	mux.HandleFunc("POST /login/", func(w http.ResponseWriter, _ *http.Request) {
		http.SetCookie(w, &http.Cookie{
			Name: "sessiontoken", Value: "tok-1", Path: "/",
			Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode,
		})
	})
	v := datapagestest.New(t, mux).Visitor(t)
	tab := v.Load(t, "/p/")

	require.Empty(t, decodeEcho(t, tab.Do(t, http.MethodPost, "/echo/", nil)).CSRF)

	require.Equal(t, http.StatusOK, tab.Do(t, http.MethodPost, "/login/", nil).Status)
	require.NotNil(t, v.Cookie("sessiontoken"), "the jar keeps the Secure cookie")

	token := decodeEcho(t, tab.Do(t, http.MethodPost, "/echo/", nil)).CSRF
	require.True(t, csrf.Tokens{}.ValidateToken("tok-1", token), "token %q", token)
	require.Empty(t, decodeEcho(t, tab.Do(t, http.MethodGet, "/echo/", nil)).CSRF)
}

// prefixTokens is a CSRF token writer a server could be configured with.
type prefixTokens struct{}

func (prefixTokens) WriteToken(w io.Writer, sessionToken string) (int, error) {
	return io.WriteString(w, "t:"+sessionToken)
}

// TestCSRFTokenConfigured tests the options for a server with a renamed session cookie
// and its own token writer, and a session a test creates without signing in.
func TestCSRFTokenConfigured(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("/p/", page(`<p>page</p>`, ""))
	mux.HandleFunc("/echo/", echo)
	srv := datapagestest.New(t, mux,
		datapagestest.WithSessionCookie("sid"),
		datapagestest.WithCSRFTokens(prefixTokens{}))
	v := srv.Visitor(t)
	v.SetSessionToken("tok-2")
	tab := v.Load(t, "/p/")

	require.Equal(t, "t:tok-2", decodeEcho(t, tab.Do(t, http.MethodPost, "/echo/", nil)).CSRF)
	require.Equal(t, "tok-2", v.Cookie("sid").Value)
}

// TestConnect tests the stream request of a tab: its path, headers and signals.
func TestConnect(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("GET /p", page(`<p>page</p>`, "inst-1"))
	mux.HandleFunc("GET /p/_$/", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Datapages-Instance") != "inst-1" ||
			r.URL.Query().Get("datastar") != `{"q":"x"}` ||
			r.Header.Get("Accept-Encoding") != "identity" {
			http.Error(w, "unexpected stream request", http.StatusBadRequest)
			return
		}
		stream(patch(`<p id="a">connected</p>`), false)(w, r)
	})
	// A path without its slash and with a query, as a doc-comment route may be written.
	tab := datapagestest.New(t, mux).Visitor(t).Load(t, "/p?x=1")
	st := tab.Connect(t, map[string]string{"q": "x"})
	ev := st.WaitFor(t, datapagestest.Element("a"))
	require.Equal(t, "connected", datapagestest.Text(ev.ElementByID(t, "a")))
}

// TestConnectRedirects tests that a stream follows the 303 to an anonymous stream,
// which a browser follows, and no other redirect.
func TestConnectRedirects(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("GET /room/", page(`<p>room</p>`, ""))
	mux.HandleFunc("GET /room/_$/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/room/_$/anon/?"+r.URL.RawQuery, http.StatusSeeOther)
	})
	mux.Handle("GET /room/_$/anon/", stream(patch(`<p id="a">anonymous</p>`), false))
	mux.Handle("GET /moved/", page(`<p>moved</p>`, ""))
	mux.HandleFunc("GET /moved/_$/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/elsewhere/", http.StatusFound)
	})
	v := datapagestest.New(t, mux).Visitor(t)

	st := v.Load(t, "/room/").Connect(t, nil)
	st.WaitFor(t, datapagestest.TextContains("a", "anonymous"))

	tab := v.Load(t, "/moved/")
	msg := failure(t, func(tb testing.TB) { tab.Connect(tb, nil) })
	require.Contains(t, msg, "302 Found")
}

// TestConnectRefusesPage tests that a stream request answered with a page,
// which is what a wrong stream path reaches, fails the test at once.
func TestConnectRefusesPage(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("GET /p/", page(`<p>page</p>`, ""))
	tab := datapagestest.New(t, mux).Visitor(t).Load(t, "/p/")
	msg := failure(t, func(tb testing.TB) { tab.Connect(tb, nil) })
	require.Contains(t, msg, `"text/html; charset=utf-8", want 200 text/event-stream`)
}

// TestWaitForCursor tests that each wait consumes the events up to the one it returns.
func TestWaitForCursor(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("GET /p/", page(`<p>page</p>`, ""))
	mux.Handle("GET /p/_$/", stream(patch(`<p id="a">1</p>`)+patch(`<p id="b">2</p>`)+
		patch(`<p id="a">3</p>`), false))
	srv := datapagestest.New(t, mux, datapagestest.WithTimeout(100*time.Millisecond))
	st := srv.Visitor(t).Load(t, "/p/").Connect(t, nil)

	first := st.WaitFor(t, datapagestest.Element("a"))
	require.Equal(t, "1", datapagestest.Text(first.ElementByID(t, "a")))
	second := st.WaitFor(t, datapagestest.Element("a"))
	require.Equal(t, "3", datapagestest.Text(second.ElementByID(t, "a")))
	require.Len(t, st.Events(), 3)

	msg := failure(t, func(tb testing.TB) { st.WaitFor(tb, datapagestest.Element("b")) })
	require.Contains(t, msg, `waited 100ms for an element with id "b"; no events arrived`)
}

// TestWaitForEndedStream tests that a wait fails as soon as the server ends the
// stream, instead of at the timeout.
func TestWaitForEndedStream(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("GET /p/", page(`<p>page</p>`, ""))
	mux.Handle("GET /p/_$/", stream(patch(`<p id="a">1</p>`), true))
	srv := datapagestest.New(t, mux, datapagestest.WithTimeout(time.Minute))
	st := srv.Visitor(t).Load(t, "/p/").Connect(t, nil)

	start := time.Now()
	msg := failure(t, func(tb testing.TB) { st.WaitFor(tb, datapagestest.Element("b")) })
	require.Contains(t, msg, "the server ended the stream before")
	require.Contains(t, msg, `1 event arrived:
datastar-patch-elements elements="<p id=\"a\">1</p>"`)
	require.Less(t, time.Since(start), 10*time.Second)
}

// TestAbsent tests that an event the anchor follows
// fails the test and one after the anchor does not.
func TestAbsent(t *testing.T) {
	tests := map[string]struct {
		body string
		want string
	}{
		"before anchor": {
			body: patch(`<p id="secret">x</p>`) + patch(`<p id="anchor">y</p>`),
			want: `an element with id "secret" arrived before an element with id "anchor"`,
		},
		"after anchor": {
			body: patch(`<p id="anchor">y</p>`) + patch(`<p id="secret">x</p>`),
		},
		"no anchor": {
			body: patch(`<p id="other">y</p>`),
			want: `for an element with id "anchor"`,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			mux := http.NewServeMux()
			mux.Handle("GET /p/", page(`<p>page</p>`, ""))
			mux.Handle("GET /p/_$/", stream(tt.body, false))
			srv := datapagestest.New(t, mux, datapagestest.WithTimeout(100*time.Millisecond))
			st := srv.Visitor(t).Load(t, "/p/").Connect(t, nil)
			msg := failure(t, func(tb testing.TB) {
				st.Absent(tb, datapagestest.Element("secret"), datapagestest.Element("anchor"))
			})
			if tt.want == "" {
				require.Empty(t, msg)
				return
			}
			require.Contains(t, msg, tt.want)
		})
	}
}

// TestSignalPatch tests that a deleted signal reads
// as null and an untouched one as missing.
func TestSignalPatch(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("GET /p/", page(`<p>page</p>`, ""))
	mux.Handle("GET /p/_$/", stream(
		"event: datastar-patch-signals\ndata: signals {\"draft\":null,\"n\":2}\n\n\n", false,
	))
	st := datapagestest.New(t, mux).Visitor(t).Load(t, "/p/").Connect(t, nil)

	signals := st.WaitFor(t, datapagestest.Signal("draft")).Signals(t)
	require.JSONEq(t, "null", string(signals["draft"]))
	require.JSONEq(t, "2", string(signals["n"]))
	require.NotContains(t, signals, "title")
}

// TestPatchRoot tests finding the elements that
// html.Parse drops at the root of a patch.
func TestPatchRoot(t *testing.T) {
	tests := map[string]string{
		"row":  `<tr id="x"><td>1</td></tr>`,
		"cell": `<td id="x">1</td>`,
		"body": `<tbody id="x"><tr><td>1</td></tr></tbody>`,
		"item": `<li id="x">1</li>`,
	}
	for name, elements := range tests {
		t.Run(name, func(t *testing.T) {
			ev := datapagestest.Event{
				Type: datapagestest.PatchElements,
				Data: map[string]string{"elements": elements},
			}
			require.Equal(t, "1", datapagestest.Text(ev.ElementByID(t, "x")))
		})
	}
}

// TestRedirect tests reading the target of each form a redirect takes.
func TestRedirect(t *testing.T) {
	respond := func(contentType, body string) http.HandlerFunc {
		return func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", contentType)
			_, _ = io.WriteString(w, body)
		}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /http/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/a/", http.StatusSeeOther)
	})
	mux.Handle("GET /script/", respond("text/javascript; charset=utf-8",
		`window.location = "/b/";`))
	mux.Handle("GET /cache/", respond("text/javascript; charset=utf-8",
		`window.location="/c/";`))
	mux.Handle("GET /event/", respond("text/event-stream",
		patch(`<script data-effect="el.remove()">`+
			`navigator.serviceWorker&&navigator.serviceWorker.ready</script>`)+
			patch(`<script data-effect="el.remove()">`+
				`setTimeout(() => window.location.href = "/d/")</script>`)))
	mux.Handle("GET /none/", respond("text/plain", "done"))
	v := datapagestest.New(t, mux).Visitor(t)

	tests := map[string]string{
		"/http/":   "/a/",
		"/script/": "/b/",
		"/cache/":  "/c/",
		"/event/":  "/d/",
	}
	for path, want := range tests {
		t.Run(path, func(t *testing.T) {
			require.Equal(t, want, v.Get(t, path).Redirect(t))
		})
	}
	resp := v.Get(t, "/none/")
	msg := failure(t, func(tb testing.TB) { resp.Redirect(tb) })
	require.Contains(t, msg, "GET /none/: 200")
}

// TestResponseSignals tests reading the signals a page declares
// the way Datastar names and decodes them.
func TestResponseSignals(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("GET /p/", page(`<body data-signals:term="'shoes'" data-signals:page="2">`+
		`<div data-signals:new-title="'it\'s'" data-signals:form.done="false"`+
		` data-signals:term__ifmissing="'ignored'" data-signals:raw-key__case.kebab="1"`+
		` data-signals="{&quot;n&quot;:{&quot;a&quot;:1}}"></div></body>`, ""))
	mux.Handle("GET /bad/", page(`<div data-signals:n="$other + 1"></div>`, ""))
	v := datapagestest.New(t, mux).Visitor(t)

	require.Equal(t, map[string]any{
		"term":     "shoes",
		"page":     float64(2),
		"newTitle": "it's",
		"form":     map[string]any{"done": false},
		"raw-key":  float64(1),
		"n":        map[string]any{"a": float64(1)},
	}, v.Get(t, "/p/").Signals(t))

	resp := v.Get(t, "/bad/")
	msg := failure(t, func(tb testing.TB) { resp.Signals(tb) })
	require.Contains(t, msg, `data-signals:n="$other + 1": not a literal`)
}

// TestSubject tests that subject values are escaped the way a dispatcher escapes them.
func TestSubject(t *testing.T) {
	require.Equal(t, "dmed.alice.bob", datapagestest.Subject("dmed.", "alice", "bob"))
	require.Equal(t, "room.said.a%2Eb.c%20d%25",
		datapagestest.Subject("room.said.", "a.b", "c d%"))
}

// TestPublish tests that an event reaches a subscriber of its subject as JSON.
func TestPublish(t *testing.T) {
	broker := inmem.New(8)
	sub, err := broker.Subscribe(t.Context(), messaging.NoopMetrics{}, "rows.added")
	require.NoError(t, err)
	defer sub.Close()

	datapagestest.Publish(t, broker, "rows.added", map[string]int{"n": 1})
	select {
	case msg := <-sub.C():
		require.Equal(t, "rows.added", msg.Subject)
		require.JSONEq(t, `{"n":1}`, string(msg.Data))
	case <-time.After(5 * time.Second):
		t.Fatal("the event did not arrive")
	}
}

// TestSendAsksForIdentity tests that a request built without Accept-Encoding asks for
// identity and one that names an encoding keeps it.
func TestSendAsksForIdentity(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/echo/", echo)
	v := datapagestest.New(t, mux).Visitor(t)

	require.Equal(t, "identity", decodeEcho(t, v.Get(t, "/echo/")).AcceptEncoding)
	req := v.NewRequest(t, http.MethodGet, "/echo/", nil)
	req.Header.Set("Accept-Encoding", "br")
	require.Equal(t, "br", decodeEcho(t, v.Send(t, req)).AcceptEncoding)
}

// TestLoadRefusesRedirect tests that loading a page that redirects fails with the
// redirect's target.
func TestLoadRefusesRedirect(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /old/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/new/", http.StatusFound)
	})
	v := datapagestest.New(t, mux).Visitor(t)
	msg := failure(t, func(tb testing.TB) { v.Load(tb, "/old/") })
	require.Contains(t, msg, "GET /old/: 302, want 200, a redirect to /new/")
}

// TestServerWaitFor tests polling a condition until it holds and failing at the timeout.
func TestServerWaitFor(t *testing.T) {
	srv := datapagestest.New(t, http.NotFoundHandler(),
		datapagestest.WithTimeout(100*time.Millisecond))
	done := make(chan struct{})
	go func() {
		time.Sleep(10 * time.Millisecond)
		close(done)
	}()
	srv.WaitFor(t, "the goroutine", func() bool {
		select {
		case <-done:
			return true
		default:
			return false
		}
	})
	msg := failure(t, func(tb testing.TB) {
		srv.WaitFor(tb, "a condition that never holds", func() bool { return false })
	})
	require.Equal(t, "waited 100ms for a condition that never holds", msg)
}

// TestSynctestBubble tests that a server, a visitor and a stream work inside a
// testing/synctest bubble: a wait covers an hour of the bubble's fake time, and every
// goroutine of the server ends when the test does.
func TestSynctestBubble(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		mux := http.NewServeMux()
		mux.Handle("GET /p/", page(`<p>page</p>`, ""))
		mux.HandleFunc("GET /p/_$/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, patch(`<p id="a">first</p>`))
			w.(http.Flusher).Flush()
			time.Sleep(time.Hour)
			_, _ = io.WriteString(w, patch(`<p id="a">an hour later</p>`))
			w.(http.Flusher).Flush()
			<-r.Context().Done()
		})
		srv := datapagestest.New(t, mux, datapagestest.WithTimeout(2*time.Hour))
		st := srv.Visitor(t).Load(t, "/p/").Connect(t, nil)

		start := time.Now()
		st.WaitFor(t, datapagestest.TextContains("a", "first"))
		st.WaitFor(t, datapagestest.TextContains("a", "an hour later"))
		require.GreaterOrEqual(t, time.Since(start), time.Hour)
	})
}

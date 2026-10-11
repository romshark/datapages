package datapagestest_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"runtime"
	"slices"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/starfederation/datastar-go/datastar"
	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/datapagestest"
	"github.com/romshark/datapages/modules/csrf"
	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/messaging/inmem"
	"github.com/romshark/datapages/runtime/auth"
)

// page serves markup with an optional Datapages-Instance header.
func page(markup, instance string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		if instance != "" {
			w.Header().Set("Datapages-Instance", instance)
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, markup)
	}
}

// quietServer discards auth manager logs.
type quietServer struct{}

func (quietServer) Logger() *slog.Logger { return slog.New(slog.DiscardHandler) }

// csrfPage serves the CSRF script for the named session cookie.
func csrfPage(cookie string) http.HandlerFunc {
	// WriteCSRFScript does not read the session store.
	m := auth.NewManager[struct{}](quietServer{}, nil, datapages.ServerConfig{}, nil)
	return func(w http.ResponseWriter, r *http.Request) {
		var head strings.Builder
		if c, err := r.Cookie(cookie); err == nil {
			_ = m.WriteCSRFScript(&head, c.Value, "")
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, "<!DOCTYPE html><html><head>"+head.String()+
			"</head><body><p>page</p></body></html>")
	}
}

// stream serves an SSE body and stays open unless end is set.
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

// patch returns an SDK-formatted PatchElements event.
func patch(elements string) string {
	return "event: datastar-patch-elements\ndata: elements " + elements + "\n\n\n"
}

// echoed records a request received by echo.
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
	FetchDest       string
	FetchMode       string
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
		FetchDest:       r.Header.Get("Sec-Fetch-Dest"),
		FetchMode:       r.Header.Get("Sec-Fetch-Mode"),
	})
}

func decodeEcho(t *testing.T, resp *datapagestest.Response) echoed {
	t.Helper()
	resp.WantStatus(t, http.StatusOK)
	var e echoed
	require.NoError(t, json.Unmarshal([]byte(resp.Body), &e))
	return e
}

// failure returns f's first fatal message without failing t.
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

// recorder captures Fatalf and exits its goroutine like testing.T.
type recorder struct {
	testing.TB
	msg string
}

func (r *recorder) Fatalf(format string, args ...any) {
	r.msg = fmt.Sprintf(format, args...)
	runtime.Goexit()
}

// TestDoPlacesSignals tests signal placement and Datastar request headers by method.
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
		"get": {
			method: http.MethodGet, path: "/echo/",
			signals: map[string]string{"q": "x"}, datastar: `{"q":"x"}`,
		},
		"query": {
			method: "QUERY", path: "/echo/", signals: map[string]string{"q": "x"},
			body: `{"q":"x"}`, contentType: "application/json",
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

// TestDoFormPlacesFields tests form fields in request bodies and DELETE queries.
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

// TestCSRFToken tests page-scoped CSRF tokens. Signed-in pages send them on unsafe
// methods; guest pages, GET and QUERY do not.
func TestCSRFToken(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("/p/", csrfPage(datapages.DefaultSessionCookieName))
	mux.HandleFunc("/echo/", echo)
	mux.HandleFunc("POST /login/", func(w http.ResponseWriter, _ *http.Request) {
		http.SetCookie(w, &http.Cookie{
			Name: datapages.DefaultSessionCookieName, Value: "tok-1", Path: "/",
			Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode,
		})
	})
	v := datapagestest.New(t, mux).Visitor(t)
	guest := v.Load(t, "/p/")

	require.Empty(t, decodeEcho(t, guest.Do(t, http.MethodPost, "/echo/", nil)).CSRF)

	guest.Do(t, http.MethodPost, "/login/", nil).WantStatus(t, http.StatusOK)
	require.NotNil(t, v.Cookie(datapages.DefaultSessionCookieName),
		"the jar keeps the Secure cookie")
	require.Empty(t, decodeEcho(t, guest.Do(t, http.MethodPost, "/echo/", nil)).CSRF,
		"the page rendered for the guest carries no token after the sign-in")

	tab := v.Load(t, "/p/")
	token := decodeEcho(t, tab.Do(t, http.MethodPost, "/echo/", nil)).CSRF
	require.True(t, csrf.Tokens{}.ValidateToken("tok-1", token), "token %q", token)
	require.Empty(t, decodeEcho(t, tab.Do(t, http.MethodGet, "/echo/", nil)).CSRF)
	require.Empty(t, decodeEcho(t, tab.Do(t, "QUERY", "/echo/", nil)).CSRF)
}

// TestCSRFTokenFromResponse tests that a document response updates the tab's token.
func TestCSRFTokenFromResponse(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("/p/", csrfPage(datapages.DefaultSessionCookieName))
	mux.HandleFunc("/echo/", echo)
	mux.HandleFunc("POST /sign-in/", func(w http.ResponseWriter, r *http.Request) {
		c := &http.Cookie{Name: datapages.DefaultSessionCookieName, Value: "tok-3", Path: "/"}
		http.SetCookie(w, c)
		r.AddCookie(c)
		csrfPage(datapages.DefaultSessionCookieName)(w, r)
	})
	tab := datapagestest.New(t, mux).Visitor(t).Load(t, "/p/")
	tab.Do(t, http.MethodPost, "/sign-in/", nil).WantStatus(t, http.StatusOK)

	token := decodeEcho(t, tab.Do(t, http.MethodPost, "/echo/", nil)).CSRF
	require.True(t, csrf.Tokens{}.ValidateToken("tok-3", token), "token %q", token)
}

// TestSetSessionToken tests direct sign-in with a custom session cookie name.
func TestSetSessionToken(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("/p/", csrfPage("sid"))
	mux.HandleFunc("/echo/", echo)
	v := datapagestest.New(t, mux, datapagestest.WithSessionCookie("sid")).Visitor(t)
	v.SetSessionToken("tok-2")
	require.Equal(t, "tok-2", v.Cookie("sid").Value)

	token := decodeEcho(t, v.Load(t, "/p/").Do(t, http.MethodPost, "/echo/", nil)).CSRF
	require.True(t, csrf.Tokens{}.ValidateToken("tok-2", token), "token %q", token)
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
	// Accept a doc-comment route without its trailing slash and with a query.
	tab := datapagestest.New(t, mux).Visitor(t).Load(t, "/p?x=1")
	st := tab.Connect(t, map[string]string{"q": "x"})
	require.Equal(t, "/p/_$/", st.Path())
	ev := st.WaitFor(t, datapagestest.Element("a"))
	require.Equal(t, "connected", datapagestest.Text(ev.ElementByID(t, "a")))
}

// TestConnectRedirects tests that Connect follows only an anonymous-stream redirect.
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
	require.Equal(t, "/room/_$/anon/", st.Path())
	st.Close()
	msg := failure(t, func(tb testing.TB) { st.WaitFor(tb, datapagestest.Element("x")) })
	require.True(t, strings.HasPrefix(msg, "GET /room/_$/ -> /room/_$/anon/: "), msg)

	tab := v.Load(t, "/moved/")
	msg = failure(t, func(tb testing.TB) { tab.Connect(tb, nil) })
	require.Contains(t, msg, "302 Found")
}

// TestConnectRefusesPage tests that Connect rejects an HTML page immediately.
func TestConnectRefusesPage(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("GET /p/", page(`<p>page</p>`, ""))
	tab := datapagestest.New(t, mux).Visitor(t).Load(t, "/p/")
	msg := failure(t, func(tb testing.TB) { tab.Connect(tb, nil) })
	require.Contains(t, msg, `"text/html; charset=utf-8", want 200 text/event-stream`)
}

// TestWaitForCursor tests that each wait consumes the events up to the one it returns.
func TestWaitForCursor(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
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
	})
}

// TestWaitForEndedStream tests that server termination fails a wait before timeout.
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

// TestAbsent tests that a prohibited event fails only before the anchor.
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
			synctest.Test(t, func(t *testing.T) {
				mux := http.NewServeMux()
				mux.Handle("GET /p/", page(`<p>page</p>`, ""))
				mux.Handle("GET /p/_$/", stream(tt.body, false))
				srv := datapagestest.New(t, mux,
					datapagestest.WithTimeout(100*time.Millisecond))
				st := srv.Visitor(t).Load(t, "/p/").Connect(t, nil)
				msg := failure(t, func(tb testing.TB) {
					st.Absent(tb, datapagestest.Element("secret"),
						datapagestest.Element("anchor"))
				})
				if tt.want == "" {
					require.Empty(t, msg)
					return
				}
				require.Contains(t, msg, tt.want)
			})
		})
	}
}

// TestWaitMessage tests optional prefixes and stream descriptions in failures.
func TestWaitMessage(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		mux := http.NewServeMux()
		mux.Handle("GET /p/", page(`<p>page</p>`, ""))
		mux.Handle("GET /p/_$/",
			stream(patch(`<p id="a">1</p>`)+patch(`<p id="b">2</p>`), false))
		srv := datapagestest.New(t, mux, datapagestest.WithTimeout(50*time.Millisecond))
		tab := srv.Visitor(t).Load(t, "/p/")

		st := tab.Connect(t, map[string]string{"room": "red"})
		msg := failure(t, func(tb testing.TB) {
			st.WaitFor(tb, datapagestest.Element("c"), "stream %d", 2)
		})
		require.True(t, strings.HasPrefix(msg, `stream 2: GET /p/_$/ with signals {"room":"red"}: `+
			`waited 50ms for an element with id "c"`), msg)

		st = tab.Connect(t, nil)
		msg = failure(t, func(tb testing.TB) {
			st.Absent(tb, datapagestest.Element("a"), datapagestest.Element("b"), "the second")
		})
		require.True(t, strings.HasPrefix(msg, `the second: GET /p/_$/: `+
			`an element with id "a" arrived before an element with id "b"`), msg)
	})
}

// TestWaitEnd tests server termination, timeout, test closure and retained events.
func TestWaitEnd(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		mux := http.NewServeMux()
		mux.Handle("GET /ends/", page(`<p>ends</p>`, ""))
		mux.Handle("GET /ends/_$/", stream(patch(`<p id="a">1</p>`), true))
		mux.Handle("GET /open/", page(`<p>open</p>`, ""))
		mux.Handle("GET /open/_$/", stream(patch(`<p id="a">1</p>`), false))
		srv := datapagestest.New(t, mux, datapagestest.WithTimeout(100*time.Millisecond))
		v := srv.Visitor(t)

		ends := v.Load(t, "/ends/").Connect(t, nil)
		ends.WaitEnd(t)
		require.True(t, ends.Ended())
		ends.WaitFor(t, datapagestest.Element("a"))

		open := v.Load(t, "/open/").Connect(t, nil)
		msg := failure(t, func(tb testing.TB) { open.WaitEnd(tb, "the open stream") })
		require.True(t, strings.HasPrefix(msg, "the open stream: GET /open/_$/: "+
			"waited 100ms for the server to end the stream; 1 event arrived:"), msg)
		require.False(t, open.Ended())

		open.Close()
		require.True(t, open.Ended())
		msg = failure(t, func(tb testing.TB) { open.WaitEnd(tb) })
		require.Equal(t,
			"GET /open/_$/: the test closed the stream before the server ended it", msg)
	})
}

// TestCloseAfterServerEnd tests that Close preserves a prior server-ended state.
func TestCloseAfterServerEnd(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("GET /p/", page(`<p>page</p>`, ""))
	mux.Handle("GET /p/_$/", stream(patch(`<p id="a">1</p>`), true))
	srv := datapagestest.New(t, mux)
	st := srv.Visitor(t).Load(t, "/p/").Connect(t, nil)
	srv.WaitFor(t, "the server to end the stream", st.Ended)

	st.Close()
	st.WaitEnd(t)
}

// TestRequestTimeout tests timeouts before headers or a complete body.
func TestRequestTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		mux := http.NewServeMux()
		mux.Handle("GET /p/", page(`<p>page</p>`, ""))
		mux.Handle("GET /p/_$/", stream(patch(`<p id="a">1</p>`), false))
		mux.HandleFunc("GET /held/", func(_ http.ResponseWriter, r *http.Request) {
			<-r.Context().Done()
		})
		v := datapagestest.New(t, mux, datapagestest.WithTimeout(time.Minute)).Visitor(t)

		msg := failure(t, func(tb testing.TB) { v.Get(tb, "/held/") })
		require.True(t, strings.HasPrefix(msg,
			"GET /held/: no complete response within 1m0s;"), msg)

		tab := v.Load(t, "/p/")
		msg = failure(t, func(tb testing.TB) { tab.Do(tb, http.MethodGet, "/p/_$/", nil) })
		require.True(t, strings.HasPrefix(msg,
			"GET /p/_$/?datastar=%7B%7D: no complete response within 1m0s;"), msg)
	})
}

// TestConnectTimeout tests a timeout before stream response headers.
func TestConnectTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		mux := http.NewServeMux()
		mux.Handle("GET /p/", page(`<p>page</p>`, ""))
		mux.HandleFunc("GET /p/_$/", func(_ http.ResponseWriter, r *http.Request) {
			<-r.Context().Done()
		})
		srv := datapagestest.New(t, mux, datapagestest.WithTimeout(time.Minute))
		tab := srv.Visitor(t).Load(t, "/p/")

		msg := failure(t, func(tb testing.TB) { tab.Connect(tb, nil) })
		require.Equal(t, "GET /p/_$/: no response header within 1m0s", msg)
	})
}

// TestOversizedBodyInBubble tests cleanup after net/http's 500ms connection hold.
func TestOversizedBodyInBubble(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		limited := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if _, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 10)); err != nil {
				http.Error(w, "too large", http.StatusRequestEntityTooLarge)
			}
		})
		v := datapagestest.New(t, limited).Visitor(t)
		req := v.NewRequest(t, http.MethodPost, "/", strings.NewReader(strings.Repeat("a", 100)))
		v.Send(t, req).WantStatus(t, http.StatusRequestEntityTooLarge)
	})
}

// TestMatches tests applying a Match to the events of an action's response.
func TestMatches(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("GET /p/", page(`<p>page</p>`, ""))
	mux.Handle("POST /act/", stream(patch(`<p id="a">done</p>`), true))
	tab := datapagestest.New(t, mux).Visitor(t).Load(t, "/p/")

	evs := tab.Do(t, http.MethodPost, "/act/", nil).Events(t)
	require.True(t, slices.ContainsFunc(evs, datapagestest.TextContains("a", "done").Matches))
	require.False(t, slices.ContainsFunc(evs, datapagestest.Element("b").Matches))
}

// TestResponseEvent tests the first match and the no-match failure.
func TestResponseEvent(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("GET /p/", page(`<p>page</p>`, ""))
	mux.Handle("POST /act/", stream(patch(`<p id="a">1</p>`)+patch(`<p id="a">2</p>`), true))
	tab := datapagestest.New(t, mux).Visitor(t).Load(t, "/p/")
	resp := tab.Do(t, http.MethodPost, "/act/", nil)

	ev := resp.Event(t, datapagestest.Element("a"))
	require.Equal(t, "1", datapagestest.Text(ev.ElementByID(t, "a")))
	msg := failure(t, func(tb testing.TB) { resp.Event(tb, datapagestest.Element("b")) })
	require.True(t, strings.HasPrefix(msg,
		`POST /act/: an element with id "b" is not among the events:`), msg)
}

// TestWantStatus tests chaining and failure details.
func TestWantStatus(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /missing/", http.NotFound)
	resp := datapagestest.New(t, mux).Visitor(t).Get(t, "/missing/")

	require.Same(t, resp, resp.WantStatus(t, http.StatusNotFound))
	msg := failure(t, func(tb testing.TB) { resp.WantStatus(tb, http.StatusOK) })
	require.Equal(t, "GET /missing/: 404, want 200:\n404 page not found\n", msg)
}

// TestSignalPatch tests null deleted signals and absent untouched signals.
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

// TestPatchRoot tests table elements that html.Parse drops at a document root.
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

// TestScript distinguishes ExecuteScript events from element patches and matches reloads.
func TestScript(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("GET /p/", page(`<p>page</p>`, ""))
	mux.HandleFunc("POST /act/", func(w http.ResponseWriter, r *http.Request) {
		sse := datastar.NewSSE(w, r)
		_ = sse.PatchElements(`<p id="a"><script>inline()</script></p>`)
		_ = sse.ExecuteScript("go(1 < 2)\nnext()")
		_ = sse.ExecuteScript("setTimeout(() => { location.reload() }, 1000)")
	})
	tab := datapagestest.New(t, mux).Visitor(t).Load(t, "/p/")
	evs := tab.Do(t, http.MethodPost, "/act/", nil).Events(t)
	require.Len(t, evs, 3)

	_, ok := evs[0].Script()
	require.False(t, ok, "an element patch is no script event")
	script, ok := evs[1].Script()
	require.True(t, ok)
	require.Equal(t, "go(1 < 2)\nnext()", script)

	require.False(t, datapagestest.Reload().Matches(evs[0]))
	require.False(t, datapagestest.Reload().Matches(evs[1]))
	require.True(t, datapagestest.Reload().Matches(evs[2]))
}

// TestRedirect tests HTTP, JavaScript and SSE redirect forms.
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

// TestResponseSignals tests Datastar signal names and values.
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

// TestPublicSignals tests underscore-prefixed paths and empty-object filtering.
func TestPublicSignals(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("GET /p/", page(`<div data-signals:_busy="false" data-signals:q="'x'"`+
		` data-signals="{&quot;form&quot;:{&quot;_draft&quot;:1,&quot;n&quot;:2},`+
		`&quot;_hidden&quot;:{&quot;n&quot;:3},&quot;drafts&quot;:{&quot;_a&quot;:1}}">`+
		`</div>`, ""))
	resp := datapagestest.New(t, mux).Visitor(t).Get(t, "/p/")

	require.Equal(t, map[string]any{
		"q":    "x",
		"form": map[string]any{"n": float64(2)},
	}, resp.PublicSignals(t))
	require.Contains(t, resp.Signals(t), "_busy", "Signals keeps the private signals")
}

// TestResponseCookies tests Set-Cookie attributes omitted by the visitor's jar.
func TestResponseCookies(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /set/", func(w http.ResponseWriter, _ *http.Request) {
		http.SetCookie(w, &http.Cookie{
			Name: "c", Value: "v", Path: "/", MaxAge: 60,
			HttpOnly: true, SameSite: http.SameSiteStrictMode,
		})
	})
	v := datapagestest.New(t, mux).Visitor(t)

	cookies := v.Get(t, "/set/").Cookies()
	require.Len(t, cookies, 1)
	require.Equal(t, "c", cookies[0].Name)
	require.Equal(t, 60, cookies[0].MaxAge)
	require.True(t, cookies[0].HttpOnly)
	require.Equal(t, http.SameSiteStrictMode, cookies[0].SameSite)
	require.Equal(t, "v", v.Cookie("c").Value)
}

// TestParseAction tests generated action expressions and missing actions.
func TestParseAction(t *testing.T) {
	tests := map[string]struct {
		expr, method, path string
	}{
		"post":  {`@post('/form/submit/')`, http.MethodPost, "/form/submit/"},
		"get":   {`@get('/files/1/')`, http.MethodGet, "/files/1/"},
		"put":   {`@put('/p/')`, http.MethodPut, "/p/"},
		"patch": {`@patch('/p/')`, http.MethodPatch, "/p/"},
		"query": {
			`@query('/input/?btn=18&paste=true')`, "QUERY", "/input/?btn=18&paste=true",
		},
		"escaped path": {`@delete('/row/a%27b/')`, http.MethodDelete, "/row/a%27b/"},
		"options": {
			`$busy = true; @post('/x/', {headers: {'X-Note': 'it\'s'}}); $busy = false`,
			http.MethodPost, "/x/",
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			method, path := datapagestest.ParseAction(t, tt.expr)
			require.Equal(t, tt.method, method)
			require.Equal(t, tt.path, path)
		})
	}
	msg := failure(t, func(tb testing.TB) { datapagestest.ParseAction(tb, "$count++") })
	require.Equal(t, `"$count++" calls no action`, msg)
}

// TestSubject tests dispatcher-compatible subject escaping.
func TestSubject(t *testing.T) {
	require.Equal(t, "dmed.alice.bob", datapagestest.Subject("dmed.", "alice", "bob"))
	require.Equal(t, "room.said.a%2Eb.c%20d%25",
		datapagestest.Subject("room.said.", "a.b", "c d%"))
}

// TestPublish tests JSON delivery to a subject subscriber.
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

// TestSendAsksForIdentity tests default and explicit content encodings.
func TestSendAsksForIdentity(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/echo/", echo)
	v := datapagestest.New(t, mux).Visitor(t)

	require.Equal(t, "identity", decodeEcho(t, v.Get(t, "/echo/")).AcceptEncoding)
	req := v.NewRequest(t, http.MethodGet, "/echo/", nil)
	req.Header.Set("Accept-Encoding", "br")
	require.Equal(t, "br", decodeEcho(t, v.Send(t, req)).AcceptEncoding)
}

// TestGetIsANavigation tests that only Get sends document-navigation headers.
func TestGetIsANavigation(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/echo/", echo)
	v := datapagestest.New(t, mux).Visitor(t)

	e := decodeEcho(t, v.Get(t, "/echo/"))
	require.Equal(t, "document", e.FetchDest)
	require.Equal(t, "navigate", e.FetchMode)
	e = decodeEcho(t, v.Send(t, v.NewRequest(t, http.MethodGet, "/echo/", nil)))
	require.Empty(t, e.FetchDest)
	require.Empty(t, e.FetchMode)
}

// TestClient tests shared cookies, disabled redirects, partial reads and isolated options.
func TestClient(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /set/", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "c", Value: "v", Path: "/"})
		http.Redirect(w, r, "/new/", http.StatusSeeOther)
	})
	mux.Handle("GET /p/_$/", stream(patch(`<p id="a">open</p>`), false))
	v := datapagestest.New(t, mux).Visitor(t)
	c := v.Client()

	resp, err := c.Do(v.NewRequest(t, http.MethodGet, "/set/", nil))
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusSeeOther, resp.StatusCode)
	require.Equal(t, "v", v.Cookie("c").Value)

	req := v.NewRequest(t, http.MethodGet, "/p/_$/", nil)
	req.Header.Set("Datastar-Request", "true")
	st, err := c.Do(req)
	require.NoError(t, err)
	defer func() { _ = st.Body.Close() }()
	require.Equal(t, http.StatusOK, st.StatusCode)
	head := make([]byte, len("event: "+datapagestest.PatchElements))
	_, err = io.ReadFull(st.Body, head)
	require.NoError(t, err)
	require.Equal(t, "event: "+datapagestest.PatchElements, string(head))

	c.CheckRedirect = nil
	require.Equal(t, http.StatusSeeOther, v.Get(t, "/set/").Status)
}

// TestLoadRefusesRedirect tests that Load reports a redirect target.
func TestLoadRefusesRedirect(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /old/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/new/", http.StatusFound)
	})
	v := datapagestest.New(t, mux).Visitor(t)
	msg := failure(t, func(tb testing.TB) { v.Load(tb, "/old/") })
	require.Contains(t, msg, "GET /old/: 302, want 200, a redirect to /new/")
}

// TestWithHTTPServer tests ReadTimeout on an in-memory connection and synctest clock.
func TestWithHTTPServer(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		read := make(chan error, 1)
		h := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			_, err := io.ReadAll(r.Body)
			read <- err
		})
		srv := datapagestest.New(t, h, datapagestest.WithHTTPServer(func(s *http.Server) {
			s.ReadTimeout = 50 * time.Millisecond
		}))
		v := srv.Visitor(t)

		body, w := io.Pipe()
		t.Cleanup(func() { _ = w.Close() })
		req := v.NewRequest(t, http.MethodPost, "/", body)
		go func() {
			if resp, err := v.Client().Do(req); err == nil {
				_ = resp.Body.Close()
			}
		}()

		// Without the read timeout, the bubble deadlocks here and the test fails.
		err := <-read
		var ne net.Error
		require.True(t, errors.As(err, &ne) && ne.Timeout(), "read: %v", err)
	})
}

// TestServerWaitFor tests success before timeout and timeout failure.
func TestServerWaitFor(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
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
	})
}

// TestSynctestBubble tests fake time and cleanup inside a testing/synctest bubble.
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

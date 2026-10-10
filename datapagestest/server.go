package datapagestest

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/html"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/modules/csrf"
)

// DefaultTimeout bounds each wait of a [Server] created without [WithTimeout].
const DefaultTimeout = 5 * time.Second

// pollInterval is how often a wait checks its condition again.
const pollInterval = 5 * time.Millisecond

type config struct {
	timeout time.Duration
	cookie  string
	tokens  csrf.TokenWriter
}

// Option configures a [Server].
type Option func(*config)

// WithTimeout sets how long the waits of [Stream] and [Server.WaitFor] last before
// they fail the test.
func WithTimeout(d time.Duration) Option {
	return func(c *config) { c.timeout = d }
}

// WithSessionCookie names the session cookie of a server whose
// [github.com/romshark/datapages.SessionsConfig] renames it.
// The CSRF token derives from that cookie's value.
func WithSessionCookie(name string) Option {
	return func(c *config) { c.cookie = name }
}

// WithCSRFTokens derives CSRF tokens with w, for a server whose
// [github.com/romshark/datapages.CSRFConfig] replaces the default [csrf.Tokens].
func WithCSRFTokens(w csrf.TokenWriter) Option {
	return func(c *config) { c.tokens = w }
}

// Server is a Datapages server that its visitors reach over in-memory connections.
type Server struct {
	srv       *httptest.Server
	transport *http.Transport
	base      *url.URL
	conf      config
}

// New serves h on in-memory connections and closes it when the test ends.
// h is the server [github.com/romshark/datapages.NewServer] returned.
//
// The net/http server serves the connections as it serves a socket, and no socket
// is opened, which lets a test run in a testing/synctest bubble.
func New(t testing.TB, h http.Handler, opts ...Option) *Server {
	t.Helper()
	conf := config{
		timeout: DefaultTimeout,
		cookie:  datapages.DefaultSessionCookieName,
		tokens:  csrf.Tokens{},
	}
	for _, opt := range opts {
		opt(&conf)
	}
	l := newPipeListener()
	srv := &httptest.Server{Listener: l, Config: &http.Server{Handler: h}}
	srv.Start()
	t.Cleanup(srv.Close)
	transport := &http.Transport{DialContext: l.dial, DisableCompression: true}
	t.Cleanup(transport.CloseIdleConnections)
	base, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parsing the server URL %q: %v", srv.URL, err)
	}
	return &Server{srv: srv, transport: transport, base: base, conf: conf}
}

// WaitFor polls cond until it holds and fails the test when the timeout passes first.
// It is for effects no stream carries, such as a store update from StreamClose.
func (s *Server) WaitFor(t testing.TB, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(s.conf.timeout)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("waited %v for %s", s.conf.timeout, what)
		}
		time.Sleep(pollInterval)
	}
}

// Visitor is one browser: a cookie jar its tabs share.
type Visitor struct {
	srv    *Server
	jar    http.CookieJar
	client *http.Client // follows no redirect
	stream *http.Client // follows only the redirect to an anonymous stream
}

// Visitor returns a visitor with an empty cookie jar.
func (s *Server) Visitor(t testing.TB) *Visitor {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("creating a cookie jar: %v", err)
	}
	client := http.Client{
		Transport: s.transport,
		Jar:       jar,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	stream := client
	stream.CheckRedirect = followAnonymousStream
	return &Visitor{srv: s, jar: jar, client: &client, stream: &stream}
}

// followAnonymousStream follows the 303 that the stream of a page with public and
// user-addressed events answers a signed-out visitor with.
func followAnonymousStream(req *http.Request, via []*http.Request) error {
	if len(via) == 1 && strings.HasSuffix(req.URL.Path, "/_$/anon/") {
		return nil
	}
	return http.ErrUseLastResponse
}

// Cookie returns the visitor's cookie named name, or nil when it holds none.
func (v *Visitor) Cookie(name string) *http.Cookie {
	for _, c := range v.jar.Cookies(v.srv.base) {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// SetSessionToken stores token as the visitor's session cookie. It signs the visitor
// in with a session the test created through the session manager, for an app whose
// sign-in a test cannot send, such as one that goes through an identity provider.
func (v *Visitor) SetSessionToken(token string) {
	v.jar.SetCookies(v.srv.base, []*http.Cookie{
		{Name: v.srv.conf.cookie, Value: token, Path: "/"},
	})
}

// Get loads path the way a browser navigation does, without following a redirect.
func (v *Visitor) Get(t testing.TB, path string) *Response {
	t.Helper()
	return v.Send(t, v.NewRequest(t, http.MethodGet, path, nil))
}

// Load loads the page at path and returns it as a tab.
// It fails the test unless the page answers 200.
func (v *Visitor) Load(t testing.TB, path string) *Tab {
	t.Helper()
	page := v.Get(t, path)
	if page.Status != http.StatusOK {
		t.Fatalf("%s: %d, want 200%s", page.desc, page.Status, redirectHint(page))
	}
	u, err := url.Parse(path)
	if err != nil {
		t.Fatalf("parsing %q: %v", path, err)
	}
	// The server appends a slash to a path without one. The stream route is that
	// path followed by "_$/".
	p := u.EscapedPath()
	if !strings.HasSuffix(p, "/") {
		p += "/"
	}
	return &Tab{v: v, page: page, path: p, instance: page.Instance()}
}

func redirectHint(r *Response) string {
	if loc := r.Header.Get("Location"); loc != "" {
		return ", a redirect to " + loc
	}
	return ""
}

// NewRequest builds a request for path on the visitor's server, without the headers
// of a Datastar request. [Visitor.Send] sends it.
func (v *Visitor) NewRequest(
	t testing.TB, method, path string, body io.Reader,
) *http.Request {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), method, v.srv.srv.URL+path, body)
	if err != nil {
		t.Fatalf("building %s %s: %v", method, path, err)
	}
	return req
}

// Send sends req with the visitor's cookies and reads the whole response.
// It follows no redirect. A request without an Accept-Encoding header asks for
// identity: Go's client would otherwise ask for gzip and decode it, and reading a
// finished SSE response to its end would then fail with io.ErrUnexpectedEOF.
func (v *Visitor) Send(t testing.TB, req *http.Request) *Response {
	t.Helper()
	if req.Header.Get("Accept-Encoding") == "" {
		req.Header.Set("Accept-Encoding", "identity")
	}
	desc := req.Method + " " + req.URL.RequestURI()
	resp, err := v.client.Do(req)
	if err != nil {
		t.Fatalf("%s: %v", desc, err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("%s: reading the response: %v", desc, err)
	}
	return &Response{
		Status: resp.StatusCode,
		Header: resp.Header,
		Body:   string(b),
		desc:   desc,
	}
}

// Client returns an HTTP client with the visitor's in-memory transport and cookie jar
// that follows no redirect. It sends the requests the other methods don't model,
// such as a stream whose status a test checks or a response it reads in part.
// The caller closes each response body. Changing the client doesn't change the visitor's.
func (v *Visitor) Client() *http.Client {
	c := *v.client
	return &c
}

// csrfToken derives the token the server expects from the visitor's session cookie,
// "" for a visitor without one.
func (v *Visitor) csrfToken(t testing.TB) string {
	t.Helper()
	c := v.Cookie(v.srv.conf.cookie)
	if c == nil || c.Value == "" {
		return ""
	}
	var b strings.Builder
	if _, err := v.srv.conf.tokens.WriteToken(&b, c.Value); err != nil {
		t.Fatalf("deriving the CSRF token: %v", err)
	}
	return b.String()
}

// Tab is one page load in a browser tab. It holds the page and the Datapages-Instance
// header of the page load, and sends the tab's actions and opens its stream.
type Tab struct {
	v        *Visitor
	page     *Response
	path     string // the page's path with a trailing slash and no query
	instance string
}

// Page is the response the tab was loaded with.
func (tb *Tab) Page() *Response { return tb.page }

// Instance is the Datapages-Instance header of the page load, "" for a page without
// per-tab state.
func (tb *Tab) Instance() string { return tb.instance }

// NewRequest builds a Datastar request of this tab: with Datastar-Request, the tab's
// Datapages-Instance and, for a signed-in visitor, the X-CSRF-Token that the page's
// script adds to every method but GET, HEAD, OPTIONS and QUERY.
// Change it and send it with [Visitor.Send], such as to leave out the CSRF token.
func (tb *Tab) NewRequest(
	t testing.TB, method, path string, body io.Reader,
) *http.Request {
	t.Helper()
	req := tb.v.NewRequest(t, method, path, body)
	req.Header.Set("Datastar-Request", "true")
	if tb.instance != "" {
		req.Header.Set("Datapages-Instance", tb.instance)
	}
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, "QUERY":
	default:
		if token := tb.v.csrfToken(t); token != "" {
			req.Header.Set("X-CSRF-Token", token)
		}
	}
	return req
}

// Do sends an action the way Datastar sends one from this tab. The signals, marshaled
// to JSON, go in the body of a POST, PUT, PATCH or QUERY and in the datastar query
// parameter of a GET or DELETE. A nil signals sends "{}", which is what Datastar sends
// for a page without signals. A [json.RawMessage] is sent as it is.
func (tb *Tab) Do(t testing.TB, method, path string, signals any) *Response {
	t.Helper()
	enc := encodeSignals(t, signals)
	if queryCarries(method) {
		req := tb.NewRequest(t, method, withQuery(t, path, url.Values{"datastar": {enc}}), nil)
		return tb.v.Send(t, req)
	}
	req := tb.NewRequest(t, method, path, strings.NewReader(enc))
	req.Header.Set("Content-Type", "application/json")
	return tb.v.Send(t, req)
}

// DoForm sends an action the way Datastar sends one with contentType 'form': the
// fields go in a urlencoded body, and in the query of a GET or DELETE.
func (tb *Tab) DoForm(t testing.TB, method, path string, form url.Values) *Response {
	t.Helper()
	if queryCarries(method) {
		return tb.v.Send(t, tb.NewRequest(t, method, withQuery(t, path, form), nil))
	}
	req := tb.NewRequest(t, method, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return tb.v.Send(t, req)
}

// queryCarries reports whether Datastar puts the payload of a request with method in
// the query instead of the body.
func queryCarries(method string) bool {
	return method == http.MethodGet || method == http.MethodDelete
}

func encodeSignals(t testing.TB, signals any) string {
	t.Helper()
	switch s := signals.(type) {
	case nil:
		return "{}"
	case json.RawMessage:
		return string(s)
	}
	b, err := json.Marshal(signals)
	if err != nil {
		t.Fatalf("encoding signals: %v", err)
	}
	return string(b)
}

// withQuery adds params to the query of path.
func withQuery(t testing.TB, path string, params url.Values) string {
	t.Helper()
	u, err := url.Parse(path)
	if err != nil {
		t.Fatalf("parsing %q: %v", path, err)
	}
	q := u.Query()
	for k, vs := range params {
		for _, v := range vs {
			q.Add(k, v)
		}
	}
	u.RawQuery = q.Encode()
	return u.String()
}

// Response is what the server answered a request with.
type Response struct {
	Status int
	Header http.Header
	Body   string

	desc string // the request, such as "POST /add/", for failure messages
	doc  *html.Node
}

// Instance is the Datapages-Instance header, "" when there is none.
func (r *Response) Instance() string { return r.Header.Get("Datapages-Instance") }

// Retry is the Datapages-Retry header. It is "reconnect" when the server refused a
// stateful request because the tab holds no state, "" when there is none.
func (r *Response) Retry() string { return r.Header.Get("Datapages-Retry") }

// Cookies parses the cookies the response's Set-Cookie headers set, with their
// attributes, as [http.Response.Cookies] does. [Visitor.Cookie] reads the visitor's
// cookie jar, which keeps the name and value of a cookie only.
func (r *Response) Cookies() []*http.Cookie {
	return (&http.Response{Header: r.Header}).Cookies()
}

// Document parses the body as an HTML document.
func (r *Response) Document() *html.Node {
	if r.doc == nil {
		r.doc = parseHTML(r.Body)
	}
	return r.doc
}

// ElementByID returns the element with id in the body and fails the test when there
// is none.
func (r *Response) ElementByID(t testing.TB, id string) *html.Node {
	t.Helper()
	n := ElementByID(r.Document(), id)
	if n == nil {
		t.Fatalf("%s: no element with id %q in:\n%s", r.desc, id, excerpt(r.Body))
	}
	return n
}

// Events parses the body of an SSE response, such as that of an action with an sse
// parameter. It fails the test unless the response is text/event-stream.
func (r *Response) Events(t testing.TB) []Event {
	t.Helper()
	if !isEventStream(r.Header.Get("Content-Type")) {
		t.Fatalf("%s: %d with Content-Type %q, not an event stream:\n%s",
			r.desc, r.Status, r.Header.Get("Content-Type"), excerpt(r.Body))
	}
	return parseEvents(r.Body)
}

// Redirect returns the target of the redirect the response carries, in whichever
// form the request received it: the Location of an HTTP redirect, the script a
// Datastar request receives before SSE starts, or the script event of an SSE
// response. It fails the test when the response carries no redirect.
func (r *Response) Redirect(t testing.TB) string {
	t.Helper()
	ct := r.Header.Get("Content-Type")
	switch {
	case r.Status >= 300 && r.Status < 400:
		if loc := r.Header.Get("Location"); loc != "" {
			return loc
		}
	case strings.HasPrefix(ct, "text/javascript"):
		if target, ok := scriptRedirect(r.Body); ok {
			return target
		}
	case isEventStream(ct):
		for _, ev := range parseEvents(r.Body) {
			if target, ok := ev.Redirect(); ok {
				return target
			}
		}
	}
	t.Fatalf("%s: %d with Content-Type %q carries no redirect:\n%s",
		r.desc, r.Status, ct, excerpt(r.Body))
	return ""
}

// excerpt shortens a body for a failure message.
func excerpt(s string) string {
	const limit = 4 << 10
	if len(s) <= limit {
		return s
	}
	return s[:limit] + fmt.Sprintf("\n... %d more bytes", len(s)-limit)
}

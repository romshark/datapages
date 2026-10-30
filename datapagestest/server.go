package datapagestest

import (
	"context"
	"encoding/json"
	"errors"
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
)

// DefaultTimeout applies to requests and waits when [WithTimeout] is unset.
const DefaultTimeout = 5 * time.Second

const pollInterval = 5 * time.Millisecond

type config struct {
	timeout    time.Duration
	cookie     string
	httpServer func(*http.Server)
}

// Option configures a [Server].
type Option func(*config)

// WithTimeout sets the request and wait timeout.
func WithTimeout(d time.Duration) Option {
	return func(c *config) { c.timeout = d }
}

// WithSessionCookie sets the cookie name used by [Visitor.SetSessionToken].
func WithSessionCookie(name string) Option {
	return func(c *config) { c.cookie = name }
}

// WithHTTPServer configures the HTTP server before it starts. Its timeouts apply to
// the in-memory connections. [New] replaces its Handler.
func WithHTTPServer(configure func(*http.Server)) Option {
	return func(c *config) { c.httpServer = configure }
}

// Server serves a Datapages handler over in-memory connections.
type Server struct {
	srv       *httptest.Server
	transport *http.Transport
	base      *url.URL
	conf      config
}

// New serves h over in-memory connections and closes it during test cleanup.
// Pass the handler returned by [datapages.NewServer].
//
// In-memory connections support testing/synctest. Cleanup waits for all connections;
// net/http may retain one for 500ms after rejecting a body or
// leaving at least 256 KiB unread.
func New(t testing.TB, h http.Handler, opts ...Option) *Server {
	t.Helper()
	conf := config{
		timeout: DefaultTimeout,
		cookie:  datapages.DefaultSessionCookieName,
	}
	for _, opt := range opts {
		opt(&conf)
	}
	l := newPipeListener()
	hs := &http.Server{}
	if conf.httpServer != nil {
		conf.httpServer(hs)
	}
	hs.Handler = h
	srv := &httptest.Server{Listener: l, Config: hs}
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

// WaitFor polls cond until it succeeds or times out. Use it for effects no stream
// carries, such as a store update from StreamClose.
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

// Visitor represents a browser whose tabs share a cookie jar.
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

// followAnonymousStream permits only the redirect to an anonymous stream.
func followAnonymousStream(req *http.Request, via []*http.Request) error {
	if len(via) == 1 && strings.HasSuffix(req.URL.Path, "/_$/anon/") {
		return nil
	}
	return http.ErrUseLastResponse
}

// Cookie returns the named visitor cookie, or nil if absent.
func (v *Visitor) Cookie(name string) *http.Cookie {
	for _, c := range v.jar.Cookies(v.srv.base) {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// SetSessionToken stores token as the cookie for a session created directly by a test.
// Pages loaded afterward receive its CSRF token.
func (v *Visitor) SetSessionToken(token string) {
	v.jar.SetCookies(v.srv.base, []*http.Cookie{
		{Name: v.srv.conf.cookie, Value: token, Path: "/"},
	})
}

// Get loads path as a browser navigation without following redirects. Its Sec-Fetch
// headers make errors from file-returning GET actions render as pages. Use
// [Visitor.NewRequest] for a fetch.
func (v *Visitor) Get(t testing.TB, path string) *Response {
	t.Helper()
	req := v.NewRequest(t, http.MethodGet, path, nil)
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	return v.Send(t, req)
}

// Load loads path as a tab and requires status 200.
func (v *Visitor) Load(t testing.TB, path string) *Tab {
	t.Helper()
	page := v.Get(t, path).RequireStatus(t, http.StatusOK)
	u, err := url.Parse(path)
	if err != nil {
		t.Fatalf("parsing %q: %v", path, err)
	}
	// The server canonicalizes page paths with a slash; stream routes add "_$/".
	p := u.EscapedPath()
	if !strings.HasSuffix(p, "/") {
		p += "/"
	}
	return &Tab{
		v:        v,
		resp:     page,
		path:     p,
		url:      path,
		instance: page.Instance(),
		csrf:     csrfToken(page.Body),
	}
}

// csrfMark begins the token in a server-generated CSRF script.
const csrfMark = `h.set("X-CSRF-Token",'`

// csrfToken extracts the page's CSRF token. Tokens contain no quote.
func csrfToken(page string) string {
	_, rest, ok := strings.Cut(page, csrfMark)
	if !ok {
		return ""
	}
	token, _, ok := strings.Cut(rest, "'")
	if !ok {
		return ""
	}
	return token
}

func redirectHint(r *Response) string {
	if loc := r.Header.Get("Location"); loc != "" {
		return ", a redirect to " + loc
	}
	return ""
}

// NewRequest builds a request without Datastar headers. Send it with [Visitor.Send].
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

// Send sends req with the visitor's cookies, follows no redirects and reads
// the whole response. It fails on timeout.
// Use [Tab.Connect] for streams or [Visitor.Client] for partial reads.
//
// A request without Accept-Encoding uses identity. This prevents compression
// middleware from choosing an encoding the visitor does not decode
// ([RFC 9110 section 12.5.3]).
//
// [RFC 9110 section 12.5.3]: https://www.rfc-editor.org/rfc/rfc9110#section-12.5.3
func (v *Visitor) Send(t testing.TB, req *http.Request) *Response {
	t.Helper()
	if req.Header.Get("Accept-Encoding") == "" {
		req.Header.Set("Accept-Encoding", "identity")
	}
	desc := req.Method + " " + req.URL.RequestURI()
	timeout := v.srv.conf.timeout
	ctx, cancel := context.WithTimeout(req.Context(), timeout)
	defer cancel()
	resp, err := v.client.Do(req.WithContext(ctx))
	var b []byte
	if err == nil {
		b, err = io.ReadAll(resp.Body)
		_ = resp.Body.Close()
	}
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			t.Fatalf("%s: no complete response within %v; open a stream with "+
				"Tab.Connect, read a response in part through Visitor.Client",
				desc, timeout)
		}
		t.Fatalf("%s: %v", desc, err)
	}
	return &Response{
		Status: resp.StatusCode,
		Header: resp.Header,
		Body:   string(b),
		desc:   desc,
	}
}

// Client returns a copy of the visitor's no-redirect HTTP client with
// its shared cookie jar. Use it for unmodeled requests or partial reads,
// and close each response body. Changes to the copy do not affect the visitor.
func (v *Visitor) Client() *http.Client {
	c := *v.client
	return &c
}

// Tab represents one page load. It keeps the page as a browser would, which is
// the loaded document with the events of its streams and action responses applied,
// and the page's signals. It also keeps the Datapages instance and the CSRF token.
// Reload a guest tab after sign-in unless an action response supplies a token.
type Tab struct {
	v        *Visitor
	resp     *Response
	path     string // the page's path with a trailing slash and no query
	url      string // the loaded path and query, which links resolve against
	instance string
	csrf     string

	live     *page // nil until first used
	streams  []*tabStream
	err      error    // the first event the tab could not apply
	warnings []string // the patches Datastar skips
}

// Page is the response the tab was loaded with. Events do not change it.
func (tb *Tab) Page() *Response { return tb.resp }

// Instance is the Datapages-Instance header of the page load,
// "" for a page without per-tab state.
func (tb *Tab) Instance() string { return tb.instance }

// NewRequest builds a Datastar request with the tab's instance and CSRF token.
// It omits the token for GET, HEAD, OPTIONS and QUERY.
// Modify the request and send it with [Visitor.Send] to test other headers.
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
		if tb.csrf != "" {
			req.Header.Set("X-CSRF-Token", tb.csrf)
		}
	}
	return req
}

// Do sends a Datastar action with signals instead of the tab's and applies a
// successful response to the tab. JSON signals go in the datastar query parameter
// for GET and DELETE, and in the body otherwise. Nil sends "{}"; [json.RawMessage]
// is unchanged. Use [Tab.Click] to send the action an element calls
// with the tab's signals.
func (tb *Tab) Do(t testing.TB, method, path string, signals any) *Response {
	t.Helper()
	resp := tb.sendJSON(t, method, path, encodeSignals(t, signals), nil)
	tb.applyResponse(resp)
	return resp
}

// DoForm sends a form action and applies a successful response to the tab.
// Fields go in the query for GET and DELETE, and in a URL-encoded body otherwise.
func (tb *Tab) DoForm(t testing.TB, method, path string, form url.Values) *Response {
	t.Helper()
	resp := tb.sendForm(t, method, path, form, nil)
	tb.applyResponse(resp)
	return resp
}

// request records any CSRF token returned by the action.
func (tb *Tab) request(t testing.TB, req *http.Request) *Response {
	t.Helper()
	resp := tb.v.Send(t, req)
	if token := csrfToken(resp.Body); token != "" {
		tb.csrf = token
	}
	return resp
}

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

// Response records an HTTP response.
type Response struct {
	Status int
	Header http.Header
	Body   string

	desc string // the request, such as "POST /add/", for failure messages
	doc  *html.Node
	pg   *page
}

// RequireStatus requires status and returns r for chaining.
func (r *Response) RequireStatus(t testing.TB, status int) *Response {
	t.Helper()
	if r.Status != status {
		t.Fatalf("%s: %d, want %d%s:\n%s",
			r.desc, r.Status, status, redirectHint(r), excerpt(r.Body))
	}
	return r
}

// Instance is the Datapages-Instance header, "" when there is none.
func (r *Response) Instance() string { return r.Header.Get("Datapages-Instance") }

// Retry returns the Datapages-Retry header. "reconnect" means the tab lacks state.
func (r *Response) Retry() string { return r.Header.Get("Datapages-Retry") }

// Cookies parses Set-Cookie headers with their attributes.
// [Visitor.Cookie] reads the visitor's stored cookie instead.
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

// page returns the document with its signals, without the events of a tab.
func (r *Response) page() *page {
	if r.pg == nil {
		// A copy, since rendering changes the document Document returns.
		r.pg = &page{doc: parseHTML(r.Body)}
		if r.pg.signals, r.pg.sigErr = decodeSignals(r.pg.doc); r.pg.signals != nil {
			r.pg.bindDefaults(r.pg.doc)
		}
		r.pg.render()
	}
	return r.pg
}

// Get returns the element of the HTML document q selects. It fails unless exactly
// 1 element matches. Use [Tab.Get] for a page a tab updates.
func (r *Response) Get(t testing.TB, q Query) *Element {
	t.Helper()
	p := r.page()
	return getOne(t, p, p.doc, q, r.desc, "")
}

// All returns the elements of the HTML document q selects, in document order.
func (r *Response) All(t testing.TB, q Query) []*Element {
	t.Helper()
	p := r.page()
	return getAll(t, p, p.doc, q, r.desc)
}

// Events parses an SSE response body and requires Content-Type text/event-stream.
func (r *Response) Events(t testing.TB) []Event {
	t.Helper()
	if !isEventStream(r.Header.Get("Content-Type")) {
		t.Fatalf("%s: %d with Content-Type %q, not an event stream:\n%s",
			r.desc, r.Status, r.Header.Get("Content-Type"), excerpt(r.Body))
	}
	return parseEvents(r.Body)
}

// Event returns the first SSE event matching m or fails the test.
func (r *Response) Event(t testing.TB, m Match) Event {
	t.Helper()
	for _, ev := range r.Events(t) {
		if m.f(ev) {
			return ev
		}
	}
	t.Fatalf("%s: %s is not among the events:\n%s", r.desc, m, excerpt(r.Body))
	return Event{}
}

// Redirect returns a target from an HTTP Location,
// JavaScript response or SSE script event.
// It fails if the response carries no redirect.
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

func excerpt(s string) string {
	const limit = 4 << 10
	if len(s) <= limit {
		return s
	}
	return s[:limit] + fmt.Sprintf("\n... %d more bytes", len(s)-limit)
}

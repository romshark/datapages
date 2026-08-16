// Package client drives a generated server over HTTP the way the Datastar
// runtime in a browser does.
//
// It holds what every acceptance case does the same way — the headers a
// Datastar request carries, reading an SSE stream in the background,
// the page load that mints a tab's instance id — so that a case is left with the
// requests that are its own and the assertions it makes about them.
//
// Nothing here names generated code. A case hands New its server and keeps
// everything the model decides to itself.
package client

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Settle is how long to wait for the server to catch up with a connection it
// has already answered. A stream's subscription is registered by the handler
// rather than by the response header, and a close hook runs after the client is gone.
// A dispatch sent without this wait can outrun either.
const Settle = 200 * time.Millisecond

// Await is how long a stream is watched for a patch that should arrive.
const Await = time.Second

// Client is a running server and the requests a test sends it.
type Client struct {
	t    *testing.T
	srv  *httptest.Server
	http *http.Client
}

// New starts h on a local address and closes it when the test ends.
func New(t *testing.T, h http.Handler) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &Client{t: t, srv: srv, http: srv.Client()}
}

// WithJar returns a client that keeps cookies across requests,
// the way a browser does. Sessions ride on cookies.
func (c *Client) WithJar(t *testing.T) *Client {
	t.Helper()
	jar := &cookieJar{}
	http := *c.http
	http.Jar = jar
	return &Client{t: t, srv: c.srv, http: &http}
}

// URL is the address the server listens on.
func (c *Client) URL() string { return c.srv.URL }

// Response is what a request was answered with.
type Response struct {
	Status int
	Body   string
	Header http.Header
}

// Retry is the value of the Datapages-Retry header, "" when there is none.
func (r Response) Retry() string { return r.Header.Get("Datapages-Retry") }

// Instance is the value of the Datapages-Instance header, "" when there is
// none.
func (r Response) Instance() string { return r.Header.Get("Datapages-Instance") }

// Element returns the text between the tags of the element with the given id.
// Applications under test echo what a handler received into one.
func (r Response) Element(t *testing.T, id string) string {
	t.Helper()
	open := `id="` + id + `">`
	i := strings.Index(r.Body, open)
	require.GreaterOrEqual(t, i, 0, "no element %q in response:\n%s", id, r.Body)
	rest := r.Body[i+len(open):]
	j := strings.Index(rest, "</")
	require.GreaterOrEqual(t, j, 0,
		"unterminated element %q in response:\n%s", id, r.Body)
	return rest[:j]
}

// Get loads a page without content negotiation.
// The bytes it reads are then the bytes the handler wrote.
func (c *Client) Get(t *testing.T, path string) Response {
	t.Helper()
	req := c.request(t, http.MethodGet, path, "")
	return c.Do(t, req)
}

// Action sends a request the way the Datastar client sends an action.
func (c *Client) Action(t *testing.T, method, path, body string) Response {
	t.Helper()
	req := c.request(t, method, path, body)
	req.Header.Set("Datastar-Request", "true")
	return c.Do(t, req)
}

// Request builds a request against this server, for a case that has to set
// something of its own on it. Pass it to Do.
func (c *Client) Request(
	t *testing.T, method, path, body string,
) *http.Request {
	t.Helper()
	req := c.request(t, method, path, body)
	req.Header.Set("Datastar-Request", "true")
	return req
}

func (c *Client) request(
	t *testing.T, method, path, body string,
) *http.Request {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req, err := http.NewRequestWithContext(
		context.Background(), method, c.srv.URL+path, r,
	)
	require.NoError(t, err, "building %s %s", method, path)
	// The transport would otherwise ask for a compressed response and hand
	// back a decompressed one, which hides what the handler wrote.
	req.Header.Set("Accept-Encoding", "identity")
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	return req
}

// Do sends a request and reads the whole response.
func (c *Client) Do(t *testing.T, req *http.Request) Response {
	t.Helper()
	resp, err := c.http.Do(req)
	require.NoError(t, err, "%s %s", req.Method, req.URL.Path)
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "reading %s %s", req.Method, req.URL.Path)
	return Response{
		Status: resp.StatusCode,
		Body:   string(b),
		Header: resp.Header,
	}
}

// Stream is one open SSE connection, the way a browser tab holds one.
type Stream struct {
	t      *testing.T
	cancel context.CancelFunc

	mu     sync.Mutex
	lines  []string
	failed error // the read ended for a reason other than the test closing it
}

// OpenStream connects to an SSE route. signals are the ones the client sends
// at connect time, which is where subject scoping gets its values; pass nil for none.
// The connection closes when the test ends.
func (c *Client) OpenStream(t *testing.T, path string, signals any) *Stream {
	t.Helper()
	return c.openStream(t, path, signals, "")
}

func (c *Client) openStream(
	t *testing.T, path string, signals any, instanceID string,
) *Stream {
	t.Helper()

	target := c.srv.URL + path
	if signals != nil {
		b, err := json.Marshal(signals)
		require.NoError(t, err, "encoding signals")
		target += "?datastar=" + url.QueryEscape(string(b))
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	require.NoError(t, err, "building the stream request")
	req.Header.Set("Datastar-Request", "true")
	req.Header.Set("Accept-Encoding", "identity")
	if instanceID != "" {
		req.Header.Set("Datapages-Instance", instanceID)
	}

	resp, err := c.http.Do(req)
	require.NoError(t, err, "opening stream %s", path)
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode, "opening stream %s", path)
	}

	s := &Stream{t: t, cancel: cancel}
	go func() {
		defer func() { _ = resp.Body.Close() }()
		sc := bufio.NewScanner(resp.Body)
		// A patch of a whole page is one SSE event and outgrows the default 64KiB token.
		// Scan would stop there, and a test would read that as "the event never arrived".
		sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
		for sc.Scan() {
			s.mu.Lock()
			s.lines = append(s.lines, sc.Text())
			s.mu.Unlock()
		}
		// The read also ends when the test closes the stream or the test itself ends,
		// which is what cancelling the context looks like here.
		if err := sc.Err(); err != nil && !errors.Is(err, context.Canceled) {
			s.mu.Lock()
			s.failed = err
			s.mu.Unlock()
		}
	}()
	time.Sleep(Settle)
	return s
}

// requireHealthy fails the test when the connection ended by itself.
// Without it a stream that died reads as a stream that carried nothing,
// and the assertion blames the server for not sending what the test never read.
//
// It runs on the test's goroutine, from Saw and Never.
func (s *Stream) requireHealthy() {
	s.t.Helper()
	s.mu.Lock()
	err := s.failed
	s.mu.Unlock()
	require.NoError(s.t, err, "the stream ended by itself")
}

// Saw reports whether sub showed up on the stream within a short window.
func (s *Stream) Saw(sub string) bool {
	s.t.Helper()
	got := WaitFor(func() bool { return s.has(sub) }, Await)
	s.requireHealthy()
	return got
}

// Never reports whether sub stayed off the stream for a short window.
// It waits the window out. Only then does a negative mean anything.
func (s *Stream) Never(sub string) bool {
	s.t.Helper()
	time.Sleep(Settle)
	s.requireHealthy()
	return !s.has(sub)
}

// Lines is everything the stream has carried so far.
func (s *Stream) Lines() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.lines...)
}

func (s *Stream) has(sub string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, l := range s.lines {
		if strings.Contains(l, sub) {
			return true
		}
	}
	return false
}

// Close drops the connection and gives the server the moment it needs to run
// the stream's close hooks.
func (s *Stream) Close() {
	s.cancel()
	time.Sleep(Settle)
}

// Tab is one browser tab of a stateful page: the instance id its page load minted,
// and the stream that owns it.
type Tab struct {
	*Stream
	client     *Client
	page       string
	stream     string
	instanceID string
}

// OpenTab loads a stateful page and connects the stream that its instance id belongs to,
// the way a browser does. streamPath defaults to page+"_$/".
func (c *Client) OpenTab(t *testing.T, page, streamPath string) *Tab {
	t.Helper()
	if streamPath == "" {
		streamPath = page + "_$/"
	}
	resp := c.Get(t, page)
	id := resp.Instance()
	require.NotEmpty(t, id, "GET %s mints no instance id", page)

	tb := &Tab{client: c, page: page, stream: streamPath, instanceID: id}
	tb.Stream = c.openStream(t, streamPath, nil, id)
	return tb
}

// InstanceID is what the page load minted for this tab.
func (tb *Tab) InstanceID() string { return tb.instanceID }

// Act sends an action as this tab, which is what gives the handler its state.
func (tb *Tab) Act(t *testing.T, method, path, body string) Response {
	t.Helper()
	req := tb.client.Request(t, method, path, body)
	req.Header.Set("Datapages-Instance", tb.instanceID)
	return tb.client.Do(t, req)
}

// Reopen connects a new stream under the same instance id, the way the client
// does after its connection dropped.
// Within the grace period the tab finds the state it had.
func (tb *Tab) Reopen(t *testing.T) {
	t.Helper()
	tb.Stream = tb.client.openStream(t, tb.stream, nil, tb.instanceID)
}

// WaitFor polls cond until it holds or d elapses, and reports which happened.
func WaitFor(cond func() bool, d time.Duration) bool {
	deadline := time.Now().Add(d)
	for {
		if cond() {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// cookieJar is the smallest jar that keeps a session cookie across requests to
// one server. net/http/cookiejar sorts and filters by domain, which a test
// against 127.0.0.1 does not need.
type cookieJar struct {
	mu      sync.Mutex
	cookies map[string]*http.Cookie
}

func (j *cookieJar) SetCookies(_ *url.URL, cookies []*http.Cookie) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.cookies == nil {
		j.cookies = map[string]*http.Cookie{}
	}
	for _, c := range cookies {
		if c.MaxAge < 0 || (!c.Expires.IsZero() && c.Expires.Before(time.Now())) {
			delete(j.cookies, c.Name)
			continue
		}
		j.cookies[c.Name] = c
	}
}

func (j *cookieJar) Cookies(_ *url.URL) []*http.Cookie {
	j.mu.Lock()
	defer j.mu.Unlock()
	out := make([]*http.Cookie, 0, len(j.cookies))
	for _, c := range j.cookies {
		out = append(out, c)
	}
	return out
}

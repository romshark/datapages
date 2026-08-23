// Drives the generated session handling of ./app.

package acceptance_test

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/internal/acceptance/brokers"
	"github.com/romshark/datapages/internal/acceptance/sessions/app"
	"github.com/romshark/datapages/modules/csrf"
	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/sessions"
	sessinmem "github.com/romshark/datapages/modules/sessions/inmem"
)

const cookieName = "sessiontoken"

type server struct {
	*httptest.Server
	csrf     csrf.Tokens
	sessions *sessinmem.SessionManager[app.SessionData]
}

// newServer starts the generated server.
//
// An app that declares a Session type is CSRF protected without being given
// anything: the token is derived from the session token.
func TestMain(m *testing.M) { os.Exit(brokers.Main(m)) }

func newServer(t *testing.T, broker messaging.Broker) server {
	t.Helper()

	sessions := sessinmem.New[app.SessionData](
		sessions.DefaultTokenGenerator{Length: sessions.DefaultTokenLen},
	)

	s := httptest.NewServer(mustNewServer(t, &app.App{}, broker, sessions))
	t.Cleanup(s.Close)
	return server{Server: s, sessions: sessions}
}

// client is one visitor:
// a cookie jar and the CSRF token that goes with the session in it.
type client struct {
	http  *http.Client
	srv   server
	token string
	// set holds the cookies of the last response. A cookie jar keeps only
	// names and values. The attributes have to be read where they arrive.
	set []*http.Cookie
}

func (s server) client(t *testing.T) *client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("building cookie jar: %v", err)
	}
	return &client{
		http: &http.Client{
			Jar: jar,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		srv: s,
	}
}

func (c *client) get(t *testing.T, path string) (int, string) {
	t.Helper()
	req, err := http.NewRequestWithContext(
		context.Background(), http.MethodGet, c.srv.URL+path, nil,
	)
	if err != nil {
		t.Fatalf("building GET %s: %v", path, err)
	}
	req.Header.Set("Accept-Encoding", "identity")
	resp, err := c.http.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return resp.StatusCode, string(b)
}

// post sends an action request carrying the visitor's CSRF token.
func (c *client) post(t *testing.T, path, body string) (int, string) {
	t.Helper()
	return c.postWithToken(t, path, body, c.token)
}

// postWithToken sends an action request with the given CSRF token,
// or with none when it is empty.
func (c *client) postWithToken(
	t *testing.T, path, body, csrfToken string,
) (int, string) {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(),
		http.MethodPost, c.srv.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatalf("building POST %s: %v", path, err)
	}
	req.Header.Set("Datastar-Request", "true")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept-Encoding", "identity")
	if csrfToken != "" {
		req.Header.Set("X-CSRF-Token", csrfToken)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	c.set = resp.Cookies()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading response of %s: %v", path, err)
	}
	return resp.StatusCode, string(b)
}

// setCookie returns the session cookie of the last response,
// with the attributes the server sent.
func (c *client) setCookie() *http.Cookie {
	for _, ck := range c.set {
		if ck.Name == cookieName {
			return ck
		}
	}
	return nil
}

// signIn creates a session and keeps the CSRF token that goes with it.
// An anonymous request needs no token.
// Signing in is the one action that can be sent without one.
func (c *client) signIn(t *testing.T, user, nickname string) string {
	t.Helper()
	status, body := c.postWithToken(t, "/login/submit/",
		`{"user":"`+user+`","nickname":"`+nickname+`"}`, "")
	if status != http.StatusOK {
		t.Fatalf("signing in: status = %d\n%s", status, body)
	}
	c.token = csrfToken(t, c.srv.csrf, c.sessionToken(t))
	return c.token
}

// csrfToken is the token derived from sessionToken as a string,
// which is what a test sends where the browser would send the header.
func csrfToken(t *testing.T, tokens csrf.Tokens, sessionToken string) string {
	t.Helper()
	var b strings.Builder
	_, err := tokens.WriteToken(&b, sessionToken)
	require.NoError(t, err, "writing the CSRF token")
	return b.String()
}

// sessionToken is the token the visitor's session cookie carries,
// which is what the CSRF token is derived from.
func (c *client) sessionToken(t *testing.T) string {
	t.Helper()
	ck := c.cookie(t)
	if ck == nil {
		t.Fatal("the visitor holds no session cookie")
	}
	return ck.Value
}

// issuedAt is the time the server stamped on the visitor's session.
func (c *client) issuedAt(t *testing.T) time.Time {
	t.Helper()
	rec, err := c.srv.sessions.Session(context.Background(), c.sessionToken(t))
	if err != nil {
		t.Fatalf("reading the session record: %v", err)
	}
	return rec.IssuedAt
}

// cookie returns the session cookie the server set, if any.
// setSessionCookie puts a session token into the visitor's jar,
// which is what a returning visitor's request carries.
func (c *client) setSessionCookie(t *testing.T, token string) {
	t.Helper()
	u, err := url.Parse(c.srv.Server.URL)
	if err != nil {
		t.Fatalf("parsing the server URL: %v", err)
	}
	c.http.Jar.SetCookies(u, []*http.Cookie{{Name: cookieName, Value: token}})
}

func (c *client) cookie(t *testing.T) *http.Cookie {
	t.Helper()
	u := c.srv.Server.URL
	parsed, err := http.NewRequestWithContext(
		context.Background(), http.MethodGet, u, nil,
	)
	if err != nil {
		t.Fatalf("building request: %v", err)
	}
	for _, ck := range c.http.Jar.Cookies(parsed.URL) {
		if ck.Name == cookieName {
			return ck
		}
	}
	return nil
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

// TestAnonymous covers a visitor with no session.
// Every handler that asks for one gets the zero value rather than an error.
func TestAnonymous(t *testing.T) {
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		srv := newServer(t, broker)

		c := srv.client(t)

		status, body := c.get(t, "/")
		if status != http.StatusOK {
			t.Fatalf("status = %d, want 200", status)
		}
		if got, want := echoed(t, body), "anonymous"; got != want {
			t.Errorf(" got: %s\nwant: %s", got, want)
		}
		if !strings.Contains(body, "<title>anonymous</title>") {
			t.Errorf("the head did not see the empty session:\n%s", body)
		}
		if c.cookie(t) != nil {
			t.Error("a visitor with no session was given a session cookie")
		}
	})
}

// TestSignInAndOut covers the whole life of a session: an action creates it,
// later requests carry it, and an action ends it.
// TestCookieHeaderVariants covers the Cookie header shapes the generated
// reader must read the way net/http.Request.Cookie reads them.
// The expectation comes from that method, not from a hand written table.
func TestCookieHeaderVariants(t *testing.T) {
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		srv := newServer(t, broker)
		c := srv.client(t)
		c.signIn(t, "alice", "Al")
		token := c.cookie(t).Value

		for name, header := range map[string]string{
			"plain":               cookieName + "=" + token,
			"among other pairs":   "theme=dark; " + cookieName + "=" + token + "; lang=en",
			"quoted value":        cookieName + `="` + token + `"`,
			"space before name":   " " + cookieName + "=" + token,
			"space after name":    cookieName + " =" + token,
			"invalid value byte":  cookieName + `=a"b`,
			"invalid then valid":  cookieName + `=a"b; ` + cookieName + "=" + token,
			"another cookie only": "theme=dark",
			"empty value":         cookieName + "=",
			"empty header":        "",
			"many other pairs": strings.Repeat("a=b; ", 200) +
				cookieName + "=" + token,
		} {
			t.Run(name, func(t *testing.T) {
				want := "anonymous"
				oracle := &http.Request{
					Header: http.Header{"Cookie": []string{header}},
				}
				if ck, err := oracle.Cookie(cookieName); err == nil &&
					ck.Value == token {
					want = "user=alice"
				}

				req, err := http.NewRequestWithContext(context.Background(),
					http.MethodGet, srv.URL+"/", nil)
				require.NoError(t, err)
				req.Header.Set("Accept-Encoding", "identity")
				if header != "" {
					req.Header.Set("Cookie", header)
				}
				resp, err := (&http.Client{}).Do(req)
				require.NoError(t, err)
				defer func() { _ = resp.Body.Close() }()
				b, err := io.ReadAll(resp.Body)
				require.NoError(t, err)

				require.Contains(t, string(b), want)
			})
		}
	})
}

func TestSignInAndOut(t *testing.T) {
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		srv := newServer(t, broker)

		c := srv.client(t)

		c.signIn(t, "alice", "Al")

		ck := c.setCookie()
		if ck == nil {
			t.Fatal("signing in set no session cookie")
		}
		if !ck.HttpOnly {
			t.Error("the session cookie is readable by scripts")
		}
		if ck.SameSite == http.SameSiteNoneMode {
			t.Error("the session cookie is sent on cross-site requests")
		}
		if c.cookie(t) == nil {
			t.Error("the session cookie was not kept by the client")
		}

		status, body := c.get(t, "/")
		if status != http.StatusOK {
			t.Fatalf("status = %d, want 200", status)
		}
		want := "user=alice nickname=Al issued=" +
			strconv.FormatInt(c.issuedAt(t).Unix(), 10)
		if got := echoed(t, body); got != want {
			t.Errorf(" got: %s\nwant: %s", got, want)
		}
		if !strings.Contains(body, "<title>alice</title>") {
			t.Errorf("the head did not see the session:\n%s", body)
		}

		if status, body := c.post(t, "/sign-out/", ""); status != http.StatusOK {
			t.Fatalf("signing out: status = %d\n%s", status, body)
		}

		status, body = c.get(t, "/")
		if status != http.StatusOK {
			t.Fatalf("status = %d, want 200", status)
		}
		if got, want := echoed(t, body), "anonymous"; got != want {
			t.Errorf("after signing out\n got: %s\nwant: %s", got, want)
		}

		if got, want := logOf(t, c), "login(alice) signout(alice)"; got != want {
			t.Errorf(" got: %s\nwant: %s", got, want)
		}
	})
}

// TestSessionToken covers the handler parameter that asks for the token
// instead of the session.
func TestSessionToken(t *testing.T) {
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		srv := newServer(t, broker)

		c := srv.client(t)

		if status, body := c.get(t, "/token/"); status != http.StatusOK {
			t.Fatalf("status = %d, want 200\n%s", status, body)
		} else if got, want := echoed(t, body), "no token"; got != want {
			t.Errorf(" got: %s\nwant: %s", got, want)
		}

		c.signIn(t, "bob", "Bo")

		status, body := c.get(t, "/token/")
		if status != http.StatusOK {
			t.Fatalf("status = %d, want 200\n%s", status, body)
		}
		if got := echoed(t, body); !strings.HasPrefix(got, "token of length ") {
			t.Errorf("the handler received no token: %s", got)
		}
	})
}

// TestStaleCookie covers a cookie the session manager does not know.
// The visitor continues as anonymous and the cookie is cleared,
// rather than being refused on every later request.
func TestStaleCookie(t *testing.T) {
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		srv := newServer(t, broker)

		req, err := http.NewRequestWithContext(
			context.Background(), http.MethodGet, srv.URL+"/", nil,
		)
		if err != nil {
			t.Fatalf("building request: %v", err)
		}
		req.AddCookie(&http.Cookie{Name: cookieName, Value: "not-a-real-token"})

		resp, err := srv.Client().Do(req)
		if err != nil {
			t.Fatalf("GET /: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()
		b, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("reading body: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
		if got, want := echoed(t, string(b)), "anonymous"; got != want {
			t.Errorf(" got: %s\nwant: %s", got, want)
		}
		var cleared bool
		for _, ck := range resp.Cookies() {
			if ck.Name == cookieName && ck.Value == "" {
				cleared = true
			}
		}
		if !cleared {
			t.Error("the stale cookie was left in place")
		}
	})
}

// TestErrorSentinel covers the status an action or
// page takes from the sentinel it returns.
func TestErrorSentinel(t *testing.T) {
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		srv := newServer(t, broker)

		c := srv.client(t)

		if status, _ := c.get(t, "/secret/"); status != http.StatusForbidden {
			t.Errorf("anonymous: status = %d, want %d", status, http.StatusForbidden)
		}

		status, body := c.postWithToken(t, "/login/submit/", `{"user":""}`, "")
		if status != http.StatusBadRequest {
			t.Errorf("empty user: status = %d, want %d\n%s",
				status, http.StatusBadRequest, body)
		}

		c.signIn(t, "carol", "")
		if status, body := c.get(t, "/secret/"); status != http.StatusOK {
			t.Errorf("signed in: status = %d, want 200\n%s", status, body)
		} else if got, want := echoed(t, body), "secret for carol"; got != want {
			t.Errorf(" got: %s\nwant: %s", got, want)
		}
	})
}

// TestCSRF covers the token a state-changing request must carry once the
// visitor has a session.
// Without a session there is nothing to forge and an anonymous request is let through.
func TestCSRF(t *testing.T) {
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		srv := newServer(t, broker)

		t.Run("anonymous request needs no token", func(t *testing.T) {
			c := srv.client(t)
			if status, body := c.postWithToken(t, "/login/submit/",
				`{"user":"dan"}`, ""); status != http.StatusOK {
				t.Errorf("status = %d, want 200\n%s", status, body)
			}
		})

		t.Run("session request without a token is refused", func(t *testing.T) {
			c := srv.client(t)
			c.signIn(t, "erin", "")
			if status, _ := c.postWithToken(t, "/login/rename/",
				`{"nickname":"E"}`, ""); status != http.StatusForbidden {
				t.Errorf("status = %d, want %d", status, http.StatusForbidden)
			}
		})

		t.Run("session request with a wrong token is refused", func(t *testing.T) {
			c := srv.client(t)
			c.signIn(t, "frank", "")
			wrong := csrfToken(t, srv.csrf, "the-token-of-someone-else")
			if status, _ := c.postWithToken(t, "/login/rename/",
				`{"nickname":"F"}`, wrong); status != http.StatusForbidden {
				t.Errorf("status = %d, want %d", status, http.StatusForbidden)
			}
		})

		t.Run("session request with its token is served", func(t *testing.T) {
			c := srv.client(t)
			token := c.signIn(t, "gina", "")
			if status, body := c.postWithToken(t, "/login/rename/",
				`{"nickname":"G"}`, token); status != http.StatusOK {
				t.Errorf("status = %d, want 200\n%s", status, body)
			}
		})

		t.Run("a page load needs no token", func(t *testing.T) {
			c := srv.client(t)
			c.signIn(t, "hank", "")
			if status, _ := c.get(t, "/"); status != http.StatusOK {
				t.Errorf("status = %d, want 200", status)
			}
		})
	})
}

// TestPrivateEvent covers an event addressed to a user. Two visitors,
// two streams, one dispatch: the user it names sees it and the other does not.
func TestPrivateEvent(t *testing.T) {
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		srv := newServer(t, broker)

		alice := srv.client(t)
		alice.signIn(t, "alice", "")
		bob := srv.client(t)
		bob.signIn(t, "bob", "")

		as := alice.openStream(t)
		bs := bob.openStream(t)

		if status, body := alice.post(t, "/login/notify/",
			`{"user":"alice","text":"for alice"}`); status != http.StatusOK {
			t.Fatalf("dispatching: status = %d\n%s", status, body)
		}

		if !as.saw(`<div id="notice">alice: for alice</div>`) {
			t.Error("the addressed user's stream received nothing")
		}
		if !bs.never("for alice") {
			t.Error("a private event reached another user")
		}
	})
}

// TestSignInRefusesUnsafeUserID covers the ID a session is created with.
// It names the subject every event addressed to that user is published to and
// subscribed by, which makes a wildcard in it a subscription to every user.
func TestSignInRefusesUnsafeUserID(t *testing.T) {
	users := map[string]string{
		"separator": "a.b",
		"star":      "*",
		"gt":        ">",
		"space":     "a b",
	}

	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		for name, user := range users {
			t.Run(name, func(t *testing.T) {
				srv := newServer(t, broker)
				c := srv.client(t)

				status, body := c.postWithToken(t, "/login/submit/",
					`{"user":"`+user+`","nickname":""}`, "")
				if status != http.StatusInternalServerError {
					t.Fatalf("signing in as %q: status = %d, want 500\n%s",
						user, status, body)
				}
				if c.cookie(t) != nil {
					t.Error("a refused sign-in left a session cookie")
				}
			})
		}
	})
}

// TestStreamRefusesUnsafeUserID covers a session that carries such an ID
// regardless, which a store filled before the check existed still holds.
// The stream reads the ID into its subscription subject, so it refuses to open
// rather than subscribe to every user.
func TestStreamRefusesUnsafeUserID(t *testing.T) {
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		srv := newServer(t, broker)

		token, err := srv.sessions.CreateSession(context.Background(),
			sessions.Record[app.SessionData]{
				UserID:    "*",
				IssuedAt:  time.Now(),
				ExpiresAt: time.Now().Add(time.Hour),
			})
		if err != nil {
			t.Fatalf("creating the session: %v", err)
		}

		c := srv.client(t)
		c.setSessionCookie(t, token)

		req, err := http.NewRequestWithContext(
			context.Background(), http.MethodGet, srv.URL+"/_$/", nil,
		)
		if err != nil {
			t.Fatalf("building stream request: %v", err)
		}
		req.Header.Set("Datastar-Request", "true")

		resp, err := c.http.Do(req)
		if err != nil {
			t.Fatalf("opening stream: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusInternalServerError {
			t.Errorf("opening the stream: status = %d, want 500", resp.StatusCode)
		}
	})
}

// TestAnonymousStream covers the stream a page serves to a visitor with no session,
// on a page that handles a public event and a private one.
//
// The server generates a second stream handler for that page.
// An anonymous connection is served by it and subscribes to the public event alone.
// The private event is addressed to a user, and a connection with no user must
// never be given it: that is one visitor reading another's messages.
func TestAnonymousStream(t *testing.T) {
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		srv := newServer(t, broker)

		anon := srv.client(t)
		anonStream := anon.openStream(t)

		alice := srv.client(t)
		alice.signIn(t, "alice", "")
		aliceStream := alice.openStream(t)

		// The two connections are served by two different handlers.
		if anonStream.path == aliceStream.path {
			t.Errorf("both connections landed on %s", anonStream.path)
		}

		// The public event reaches both.
		if status, body := alice.post(t, "/login/broadcast/",
			`{"text":"for everyone"}`); status != http.StatusOK {
			t.Fatalf("broadcasting: status = %d\n%s", status, body)
		}
		if !anonStream.saw(`<div id="broadcast">for everyone</div>`) {
			t.Error("the anonymous stream received no public event")
		}
		if !aliceStream.saw(`<div id="broadcast">for everyone</div>`) {
			t.Error("the signed-in stream received no public event")
		}

		// The private one reaches only the user it names.
		if status, body := alice.post(t, "/login/notify/",
			`{"user":"alice","text":"for alice"}`); status != http.StatusOK {
			t.Fatalf("notifying: status = %d\n%s", status, body)
		}
		if !aliceStream.saw(`<div id="notice">alice: for alice</div>`) {
			t.Error("the addressed user received nothing")
		}
		if !anonStream.never("for alice") {
			t.Error("a private event reached a stream with no session")
		}
	})
}

// --- helpers ---------------------------------------------------------------

type stream struct {
	t      *testing.T
	cancel context.CancelFunc
	// path is where the connection ended up,
	// which differs for a visitor with no session.
	path   string
	mu     sync.Mutex
	lines  []string
	failed error // the read ended for a reason other than the test closing it
}

// requireHealthy fails the test when the connection ended by itself.
// Without it a stream that died reads as a stream that carried nothing.
func (s *stream) requireHealthy() {
	s.t.Helper()
	s.mu.Lock()
	err := s.failed
	s.mu.Unlock()
	require.NoError(s.t, err, "the stream ended by itself")
}

func (c *client) openStream(t *testing.T) *stream {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	req, err := http.NewRequestWithContext(
		ctx, http.MethodGet, c.srv.URL+"/_$/", nil,
	)
	if err != nil {
		t.Fatalf("building stream request: %v", err)
	}
	req.Header.Set("Datastar-Request", "true")
	req.Header.Set("Accept-Encoding", "identity")

	// A visitor with no session is sent to the page's anonymous stream route.
	// The Datastar client follows that, and so does this one.
	follow := *c.http
	follow.CheckRedirect = nil

	resp, err := follow.Do(req)
	if err != nil {
		t.Fatalf("opening stream: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		t.Fatalf("opening stream: status %d", resp.StatusCode)
	}

	s := &stream{t: t, cancel: cancel, path: resp.Request.URL.Path}
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
	time.Sleep(200 * time.Millisecond)
	return s
}

func (s *stream) saw(sub string) bool {
	s.t.Helper()
	defer s.requireHealthy()
	deadline := time.Now().Add(time.Second)
	for {
		s.mu.Lock()
		for _, l := range s.lines {
			if strings.Contains(l, sub) {
				s.mu.Unlock()
				return true
			}
		}
		s.mu.Unlock()
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func (s *stream) never(sub string) bool {
	s.t.Helper()
	time.Sleep(200 * time.Millisecond)
	s.requireHealthy()
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, l := range s.lines {
		if strings.Contains(l, sub) {
			return false
		}
	}
	return true
}

func logOf(t *testing.T, c *client) string {
	t.Helper()
	_, body := c.get(t, "/log/")
	return echoed(t, body)
}

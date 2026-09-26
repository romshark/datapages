// Asserts that CSRF protection covers every state-changing action a
// visitor with a session can reach.

package acceptance_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/internal/acceptance/csrfcoverage/app"
	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/messaging/inmem"
	"github.com/romshark/datapages/modules/sessions"
	sessinmem "github.com/romshark/datapages/modules/sessions/inmem"
)

// countingSessions counts the store reads a request costs.
type countingSessions struct {
	sessions.Manager[struct{}]
	reads atomic.Int64
}

func (c *countingSessions) ReadSessionFromCookie(cookieValue string) (
	sessions.Record[struct{}], string, bool, error,
) {
	c.reads.Add(1)
	return c.Manager.ReadSessionFromCookie(cookieValue)
}

// failingSessions is a store whose reads fail the way a backend outage does.
type failingSessions struct {
	sessions.Manager[struct{}]
}

func (failingSessions) ReadSessionFromCookie(string) (
	sessions.Record[struct{}], string, bool, error,
) {
	return sessions.Record[struct{}]{}, "", false, errStoreDown
}

var errStoreDown = errors.New("session store is down")

// newPost returns a function that sends a Datastar POST and returns the status code.
// An empty token sends no CSRF header.
func newPost(
	t *testing.T, srv *httptest.Server, client *http.Client,
) func(path, body, token string) int {
	return func(path, body, token string) int {
		t.Helper()
		req, err := http.NewRequestWithContext(context.Background(),
			http.MethodPost, srv.URL+path, strings.NewReader(body))
		require.NoError(t, err, "building POST %s", path)
		req.Header.Set("Datastar-Request", "true")
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("X-CSRF-Token", token)
		}
		resp, err := client.Do(req)
		require.NoError(t, err, "POST %s", path)
		defer func() { _ = resp.Body.Close() }()
		_, _ = io.Copy(io.Discard, resp.Body)
		return resp.StatusCode
	}
}

// TestCSRFOnlyActionReadsNoSession tests the check an action runs when it
// takes no session. The CSRF token is derived from the cookie, hence the store
// stays out of the request.
func TestCSRFOnlyActionReadsNoSession(t *testing.T) {
	t.Parallel()
	store := &countingSessions{Manager: sessinmem.New[struct{}](
		sessions.DefaultTokenGenerator{Length: sessions.DefaultTokenLen},
	)}

	srv := httptest.NewServer(mustNewServer(
		t, &app.App{}, inmem.New(messaging.DefaultBrokerChanBuffer), store,
	))
	t.Cleanup(srv.Close)

	jar, err := cookiejar.New(nil)
	require.NoError(t, err, "building cookie jar")
	client := &http.Client{Jar: jar}

	post := newPost(t, srv, client)

	require.Equal(t, http.StatusOK,
		post("/sign-in/", `{"user":"alice"}`, ""), "signing in")

	before := store.reads.Load()
	require.Equal(t, http.StatusForbidden,
		post("/delete/", `{"confirm":true}`, ""),
		"an action without a CSRF token was served")
	require.Equal(t, before, store.reads.Load(),
		"the store was read for an action that takes no session")
}

// TestCSRFCoversEveryAction tests a state-changing action that declares
// neither session nor sessionToken.
//
// A visitor with a session sends it without a CSRF token,
// which is the request a cross-site page can make their browser send.
// The server refuses it and the action does not take effect.
func TestCSRFCoversEveryAction(t *testing.T) {
	t.Parallel()
	sessions := sessinmem.New[struct{}](
		sessions.DefaultTokenGenerator{Length: sessions.DefaultTokenLen},
	)

	srv := httptest.NewServer(mustNewServer(
		t,
		&app.App{}, inmem.New(messaging.DefaultBrokerChanBuffer), sessions,
	))
	t.Cleanup(srv.Close)

	jar, err := cookiejar.New(nil)
	require.NoError(t, err, "building cookie jar")
	client := &http.Client{Jar: jar}

	post := newPost(t, srv, client)

	require.Equal(t, http.StatusOK,
		post("/sign-in/", `{"user":"alice"}`, ""), "signing in")

	require.Equal(t, http.StatusForbidden,
		post("/delete/", `{"confirm":true}`, ""),
		"a state-changing action of a visitor with a session was served "+
			"without a CSRF token")

	req, err := http.NewRequestWithContext(
		context.Background(), http.MethodGet, srv.URL+"/", nil,
	)
	require.NoError(t, err, "building GET /")
	resp, err := client.Do(req)
	require.NoError(t, err, "GET /")
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "reading /")
	require.NotContains(t, string(b), "deleted=1", "the refused action took effect")
}

// TestErrorPagesCarryTheCSRFScript tests the documents the 404 and the 500
// page write for a signed-in visitor. Written from the zero session,
// WriteCSRFScript writes nothing and every action reachable from them answers 403.
func TestErrorPagesCarryTheCSRFScript(t *testing.T) {
	t.Parallel()
	store := sessinmem.New[struct{}](
		sessions.DefaultTokenGenerator{Length: sessions.DefaultTokenLen},
	)

	srv := httptest.NewServer(mustNewServer(
		t, &app.App{}, inmem.New(messaging.DefaultBrokerChanBuffer), store,
	))
	t.Cleanup(srv.Close)

	jar, err := cookiejar.New(nil)
	require.NoError(t, err, "building cookie jar")
	client := &http.Client{Jar: jar}

	require.Equal(t, http.StatusOK,
		newPost(t, srv, client)("/sign-in/", `{"user":"alice"}`, ""), "signing in")

	get := func(path string) (int, string) {
		t.Helper()
		req, err := http.NewRequestWithContext(
			context.Background(), http.MethodGet, srv.URL+path, nil,
		)
		require.NoError(t, err, "building GET %s", path)
		resp, err := client.Do(req)
		require.NoError(t, err, "GET %s", path)
		defer func() { _ = resp.Body.Close() }()
		b, err := io.ReadAll(resp.Body)
		require.NoError(t, err, "reading %s", path)
		return resp.StatusCode, string(b)
	}

	status, page := get("/")
	require.Equal(t, http.StatusOK, status)
	require.Contains(t, page, "X-CSRF-Token", "the page carries no CSRF script")

	for name, tt := range map[string]struct {
		path   string
		status int
		echo   string
	}{
		"404": {"/no-such-path/", http.StatusNotFound, "404 user=alice"},
		"500": {"/boom/", http.StatusInternalServerError, "500 user=alice"},
	} {
		t.Run(name, func(t *testing.T) {
			status, body := get(tt.path)
			require.Equal(t, tt.status, status)
			require.Contains(t, body, tt.echo, "the page did not see the session")
			require.Contains(t, body, "X-CSRF-Token",
				"the document carries no CSRF script:\n%s", body)
		})
	}
}

var csrfTokenInScript = regexp.MustCompile(`"X-CSRF-Token",'([^']+)'`)

// TestPrivateEventPageCarriesTheCSRFScript tests that a signed-in visitor can
// submit an action from a page whose GET omits the session but whose private
// event stream requires it.
func TestPrivateEventPageCarriesTheCSRFScript(t *testing.T) {
	t.Parallel()
	store := sessinmem.New[struct{}](
		sessions.DefaultTokenGenerator{Length: sessions.DefaultTokenLen},
	)

	srv := httptest.NewServer(mustNewServer(
		t, &app.App{}, inmem.New(messaging.DefaultBrokerChanBuffer), store,
	))
	t.Cleanup(srv.Close)

	jar, err := cookiejar.New(nil)
	require.NoError(t, err, "building cookie jar")
	client := &http.Client{Jar: jar}

	post := newPost(t, srv, client)

	require.Equal(t, http.StatusOK,
		post("/sign-in/", `{"user":"alice"}`, ""), "signing in")

	req, err := http.NewRequestWithContext(
		context.Background(), http.MethodGet, srv.URL+"/inbox/", nil,
	)
	require.NoError(t, err, "building GET /inbox/")
	resp, err := client.Do(req)
	require.NoError(t, err, "GET /inbox/")
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "reading /inbox/")
	require.Equal(t, http.StatusOK, resp.StatusCode)

	m := csrfTokenInScript.FindStringSubmatch(string(b))
	require.NotNil(t, m, "the document carries no CSRF script:\n%s", b)

	require.Equal(t, http.StatusOK, post("/inbox/mark-read/", "", m[1]),
		"posting with the page's CSRF token")
}

// TestUnclaimedPathReadsTheSessionOnce tests the store reads a 404 costs.
//
// The index handler serves every path no page claims. It read the session
// before the path check and render404 read it again, which is two store round
// trips and, with a stale cookie, two clearing Set-Cookie headers.
func TestUnclaimedPathReadsTheSessionOnce(t *testing.T) {
	t.Parallel()
	store := &countingSessions{Manager: sessinmem.New[struct{}](
		sessions.DefaultTokenGenerator{Length: sessions.DefaultTokenLen},
	)}

	srv := httptest.NewServer(mustNewServer(
		t, &app.App{}, inmem.New(messaging.DefaultBrokerChanBuffer), store,
	))
	t.Cleanup(srv.Close)

	jar, err := cookiejar.New(nil)
	require.NoError(t, err, "building cookie jar")
	client := &http.Client{Jar: jar}

	require.Equal(t, http.StatusOK,
		newPost(t, srv, client)("/sign-in/", `{"user":"alice"}`, ""), "signing in")

	get := func(path string) *http.Response {
		t.Helper()
		req, err := http.NewRequestWithContext(
			context.Background(), http.MethodGet, srv.URL+path, nil,
		)
		require.NoError(t, err, "building GET %s", path)
		resp, err := client.Do(req)
		require.NoError(t, err, "GET %s", path)
		t.Cleanup(func() { _ = resp.Body.Close() })
		_, _ = io.Copy(io.Discard, resp.Body)
		return resp
	}

	before := store.reads.Load()
	resp := get("/no-such-path/")
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	require.Equal(t, int64(1), store.reads.Load()-before,
		"the 404 read the session store more than once")

	before = store.reads.Load()
	resp = get("/")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, int64(1), store.reads.Load()-before,
		"the index page read the session store more than once")
}

// TestError500PageStatusFollowsItsSessionRead tests what a failed page load
// answers when the 500 page reads the session.
//
// The status is sent with the response head the session read writes into, hence
// it has to be sent after it: a status written first drops the Set-Cookie that
// clears a stale cookie, and overrides the 503 of a store that is down.
func TestError500PageStatusFollowsItsSessionRead(t *testing.T) {
	t.Parallel()

	get := func(t *testing.T, srv *httptest.Server, cookie string) *http.Response {
		t.Helper()
		req, err := http.NewRequestWithContext(
			context.Background(), http.MethodGet, srv.URL+"/boom/", nil,
		)
		require.NoError(t, err, "building GET /boom/")
		req.AddCookie(&http.Cookie{Name: "sessiontoken", Value: cookie})
		resp, err := srv.Client().Do(req)
		require.NoError(t, err, "GET /boom/")
		t.Cleanup(func() { _ = resp.Body.Close() })
		_, _ = io.Copy(io.Discard, resp.Body)
		return resp
	}

	t.Run("stale cookie is cleared", func(t *testing.T) {
		t.Parallel()
		store := sessinmem.New[struct{}](
			sessions.DefaultTokenGenerator{Length: sessions.DefaultTokenLen},
		)
		srv := httptest.NewServer(mustNewServer(
			t, &app.App{}, inmem.New(messaging.DefaultBrokerChanBuffer), store,
		))
		t.Cleanup(srv.Close)

		resp := get(t, srv, "not-a-real-token")
		require.Equal(t, http.StatusInternalServerError, resp.StatusCode)
		var cleared bool
		for _, ck := range resp.Cookies() {
			if ck.Name == "sessiontoken" && ck.Value == "" {
				cleared = true
			}
		}
		require.True(t, cleared, "the stale cookie was left in place")
	})

	t.Run("a store outage answers 503", func(t *testing.T) {
		t.Parallel()
		store := failingSessions{sessinmem.New[struct{}](
			sessions.DefaultTokenGenerator{Length: sessions.DefaultTokenLen},
		)}
		srv := httptest.NewServer(mustNewServer(
			t, &app.App{}, inmem.New(messaging.DefaultBrokerChanBuffer), store,
		))
		t.Cleanup(srv.Close)

		resp := get(t, srv, "any-token")
		require.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
	})
}

// TestCSRFDisabledServesWithoutLogger tests a server with CSRF protection off
// and no WithLogger. It starts, and it serves the action the check refuses.
func TestCSRFDisabledServesWithoutLogger(t *testing.T) {
	t.Parallel()
	store := sessinmem.New[struct{}](
		sessions.DefaultTokenGenerator{Length: sessions.DefaultTokenLen},
	)

	srv := httptest.NewServer(mustNewServer(
		t, &app.App{}, inmem.New(messaging.DefaultBrokerChanBuffer), store,
		datapages.WithCSRFProtection(datapages.CSRFConfig{Disabled: true}),
	))
	t.Cleanup(srv.Close)

	jar, err := cookiejar.New(nil)
	require.NoError(t, err, "building cookie jar")
	client := &http.Client{Jar: jar}
	post := newPost(t, srv, client)

	require.Equal(t, http.StatusOK,
		post("/sign-in/", `{"user":"alice"}`, ""), "signing in")
	require.Equal(t, http.StatusOK,
		post("/delete/", `{"confirm":true}`, ""),
		"an action was refused with CSRF protection disabled")

	req, err := http.NewRequestWithContext(
		context.Background(), http.MethodGet, srv.URL+"/", nil,
	)
	require.NoError(t, err, "building GET /")
	resp, err := client.Do(req)
	require.NoError(t, err, "GET /")
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "reading /")
	require.Contains(t, string(b), "deleted=1", "the action did not take effect")
}

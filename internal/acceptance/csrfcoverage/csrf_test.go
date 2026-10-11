// Asserts that CSRF protection covers every state-changing action a
// visitor with a session can reach.

package acceptance_test

import (
	"errors"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/datapagestest"
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

func newSessions() *sessinmem.SessionManager[struct{}] {
	return sessinmem.New[struct{}](
		sessions.DefaultTokenGenerator{Length: sessions.DefaultTokenLen},
	)
}

func serve(
	t *testing.T, store sessions.Manager[struct{}], opts ...datapages.ServerOption,
) *datapagestest.Server {
	t.Helper()
	return datapagestest.New(t, mustNewServer(
		t, &app.App{}, inmem.New(messaging.DefaultBrokerChanBuffer), store, opts...,
	))
}

// signIn signs v in as alice from the index page and returns that page's tab,
// which was rendered for a guest.
func signIn(t *testing.T, v *datapagestest.Visitor) *datapagestest.Tab {
	t.Helper()
	tab := v.Load(t, "/")
	resp := tab.Do(t, http.MethodPost, "/sign-in/", map[string]string{"user": "alice"})
	require.Equal(t, http.StatusOK, resp.Status, "signing in")
	return tab
}

// sendWithoutToken sends a Datastar request of tab with the JSON signals and
// without a CSRF token, whether or not the page of tab carries one.
func sendWithoutToken(
	t *testing.T, v *datapagestest.Visitor, tab *datapagestest.Tab,
	method, path, signals string,
) *datapagestest.Response {
	t.Helper()
	req := tab.NewRequest(t, method, path, strings.NewReader(signals))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Del("X-CSRF-Token")
	return v.Send(t, req)
}

// postWithToken sends a POST of tab without a body, with token as its CSRF token
// in place of the one the page of tab carries.
func postWithToken(
	t *testing.T, v *datapagestest.Visitor, tab *datapagestest.Tab, path, token string,
) *datapagestest.Response {
	t.Helper()
	req := tab.NewRequest(t, http.MethodPost, path, nil)
	req.Header.Set("X-CSRF-Token", token)
	return v.Send(t, req)
}

// TestCSRFOnlyActionReadsNoSession tests the check an action runs when it
// takes no session. The CSRF token is derived from the cookie, hence the store
// stays out of the request.
func TestCSRFOnlyActionReadsNoSession(t *testing.T) {
	t.Parallel()
	store := &countingSessions{Manager: newSessions()}
	v := serve(t, store).Visitor(t)
	tab := signIn(t, v)

	before := store.reads.Load()
	resp := sendWithoutToken(t, v, tab, http.MethodPost, "/delete/", `{"confirm":true}`)
	require.Equal(t, http.StatusForbidden, resp.Status,
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
	v := serve(t, newSessions()).Visitor(t)
	tab := signIn(t, v)

	resp := sendWithoutToken(t, v, tab, http.MethodPost, "/delete/", `{"confirm":true}`)
	require.Equal(t, http.StatusForbidden, resp.Status,
		"a state-changing action of a visitor with a session was served "+
			"without a CSRF token")

	require.NotContains(t, v.Get(t, "/").Body, "deleted=1",
		"the refused action took effect")
}

// TestQueryNeedsNoCSRFToken tests the QUERY actions of a visitor with a session.
// They carry no CSRF token: the page's script adds none to a QUERY.
// QUERY is safe and both are served:
// the action that reads the session and the action that takes none.
func TestQueryNeedsNoCSRFToken(t *testing.T) {
	t.Parallel()
	v := serve(t, newSessions()).Visitor(t)
	tab := signIn(t, v)

	for name, tc := range map[string]struct{ path, want string }{
		"reading the session": {path: "/count/", want: "user=alice"},
		"taking no session":   {path: "/status/"},
	} {
		t.Run(name, func(t *testing.T) {
			resp := tab.Do(t, "QUERY", tc.path, nil)
			require.Equal(t, http.StatusOK, resp.Status,
				"QUERY %s:\n%s", tc.path, resp.Body)
			require.Contains(t, resp.Body, tc.want)
		})
	}
}

// TestErrorPagesCarryTheCSRFScript tests the documents the 404 and the 500
// page write for a signed-in visitor. Written from the zero session,
// WriteCSRFScript writes nothing and every action reachable from them answers 403.
func TestErrorPagesCarryTheCSRFScript(t *testing.T) {
	t.Parallel()
	v := serve(t, newSessions()).Visitor(t)
	signIn(t, v)

	page := v.Get(t, "/")
	require.Equal(t, http.StatusOK, page.Status)
	require.Contains(t, page.Body, "X-CSRF-Token", "the page carries no CSRF script")

	for name, tt := range map[string]struct {
		path   string
		status int
		echo   string
	}{
		"404": {"/no-such-path/", http.StatusNotFound, "404 user=alice"},
		"500": {"/boom/", http.StatusInternalServerError, "500 user=alice"},
	} {
		t.Run(name, func(t *testing.T) {
			resp := v.Get(t, tt.path)
			require.Equal(t, tt.status, resp.Status)
			require.Contains(t, resp.Body, tt.echo, "the page did not see the session")
			require.Contains(t, resp.Body, "X-CSRF-Token",
				"the document carries no CSRF script:\n%s", resp.Body)
		})
	}
}

var csrfTokenInScript = regexp.MustCompile(`"X-CSRF-Token",'([^']+)'`)

// TestPrivateEventPageCarriesTheCSRFScript tests that a signed-in visitor can
// submit an action from a page whose GET omits the session but whose private
// event stream requires it.
func TestPrivateEventPageCarriesTheCSRFScript(t *testing.T) {
	t.Parallel()
	v := serve(t, newSessions()).Visitor(t)
	signIn(t, v)

	inbox := v.Load(t, "/inbox/")
	m := csrfTokenInScript.FindStringSubmatch(inbox.Page().Body)
	require.NotNil(t, m, "the document carries no CSRF script:\n%s", inbox.Page().Body)

	require.Equal(t, http.StatusOK,
		postWithToken(t, v, inbox, "/inbox/mark-read/", m[1]).Status,
		"posting with the page's CSRF token")
}

// TestHandlersWithoutSessionWriteCSRFScript tests page and action responses
// from handlers with no session parameter. A session cookie produces a CSRF
// script whose token authorizes an app-level action.
// A request without a session cookie produces no script.
func TestHandlersWithoutSessionWriteCSRFScript(t *testing.T) {
	t.Parallel()
	srv := serve(t, newSessions())

	guestPage := srv.Visitor(t).Get(t, "/about/")
	require.Equal(t, http.StatusOK, guestPage.Status)
	require.NotContains(t, guestPage.Body, "X-CSRF-Token",
		"the document contains a CSRF script without a session cookie")

	v := srv.Visitor(t)
	signIn(t, v)

	about := v.Load(t, "/about/")
	pageToken := csrfTokenInScript.FindStringSubmatch(about.Page().Body)
	require.NotNil(t, pageToken,
		"the page contains no CSRF script:\n%s", about.Page().Body)

	doc := postWithToken(t, v, about, "/about/preview/", pageToken[1])
	require.Equal(t, http.StatusOK, doc.Status, "posting with the page's CSRF token")
	docToken := csrfTokenInScript.FindStringSubmatch(doc.Body)
	require.NotNil(t, docToken,
		"the action response contains no CSRF script:\n%s", doc.Body)

	for name, token := range map[string]string{
		"page":            pageToken[1],
		"action response": docToken[1],
	} {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, http.StatusOK,
				postWithToken(t, v, about, "/ping/", token).Status,
				"POST /ping/ with the %s CSRF token", name)
		})
	}
}

// TestUnclaimedPathReadsTheSessionOnce tests the store reads a 404 costs.
//
// The index handler serves every path no page claims. It read the session
// before the path check and render404 read it again, which is 2 store round
// trips and, with a stale cookie, 2 clearing Set-Cookie headers.
func TestUnclaimedPathReadsTheSessionOnce(t *testing.T) {
	t.Parallel()
	store := &countingSessions{Manager: newSessions()}
	v := serve(t, store).Visitor(t)
	signIn(t, v)

	before := store.reads.Load()
	resp := v.Get(t, "/no-such-path/")
	require.Equal(t, http.StatusNotFound, resp.Status)
	require.Equal(t, int64(1), store.reads.Load()-before,
		"the 404 read the session store more than once")

	before = store.reads.Load()
	resp = v.Get(t, "/")
	require.Equal(t, http.StatusOK, resp.Status)
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

	get := func(
		t *testing.T, srv *datapagestest.Server, cookie string,
	) *datapagestest.Response {
		t.Helper()
		v := srv.Visitor(t)
		req := v.NewRequest(t, http.MethodGet, "/boom/", nil)
		req.AddCookie(&http.Cookie{Name: "sessiontoken", Value: cookie})
		return v.Send(t, req)
	}

	t.Run("stale cookie is cleared", func(t *testing.T) {
		t.Parallel()
		resp := get(t, serve(t, newSessions()), "not-a-real-token")
		require.Equal(t, http.StatusInternalServerError, resp.Status)
		cleared := slices.ContainsFunc(resp.Cookies(), func(ck *http.Cookie) bool {
			return ck.Name == "sessiontoken" && ck.Value == ""
		})
		require.True(t, cleared, "the stale cookie was left in place")
	})

	t.Run("a store outage answers 503", func(t *testing.T) {
		t.Parallel()
		resp := get(t, serve(t, failingSessions{newSessions()}), "any-token")
		require.Equal(t, http.StatusServiceUnavailable, resp.Status)
	})
}

// TestCSRFDisabledServesWithoutLogger tests a server with CSRF protection off
// and no WithLogger. It starts, and it serves the action the check refuses.
func TestCSRFDisabledServesWithoutLogger(t *testing.T) {
	t.Parallel()
	v := serve(t, newSessions(),
		datapages.WithCSRFProtection(datapages.CSRFConfig{Disabled: true}),
	).Visitor(t)
	tab := signIn(t, v)

	resp := sendWithoutToken(t, v, tab, http.MethodPost, "/delete/", `{"confirm":true}`)
	require.Equal(t, http.StatusOK, resp.Status,
		"an action was refused with CSRF protection disabled")

	require.Contains(t, v.Get(t, "/").Body, "deleted=1", "the action did not take effect")
}

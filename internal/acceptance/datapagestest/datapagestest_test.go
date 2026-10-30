package acceptance_test

import (
	"net/http"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/datapagestest"
	"github.com/romshark/datapages/internal/acceptance/datapagestest/app"
	"github.com/romshark/datapages/internal/acceptance/datapagestest/app/datapagesgen"
	"github.com/romshark/datapages/internal/acceptance/datapagestest/app/datapagesgen/href"
	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/messaging/inmem"
	"github.com/romshark/datapages/modules/sessions"
	sessinmem "github.com/romshark/datapages/modules/sessions/inmem"
)

// testCase is a server of the case started through datapagestest,
// with the broker and the session store it was built with.
type testCase struct {
	srv      *datapagestest.Server
	broker   *inmem.MessageBroker
	sessions *sessinmem.SessionManager[struct{}]
}

func newCase(t *testing.T) testCase {
	t.Helper()
	broker := inmem.New(messaging.DefaultBrokerChanBuffer)
	store := newSessions()
	s := mustNewServer(t, &app.App{}, broker, store)
	return testCase{srv: datapagestest.New(t, s), broker: broker, sessions: store}
}

func newSessions() *sessinmem.SessionManager[struct{}] {
	return sessinmem.New[struct{}](sessions.DefaultTokenGenerator{})
}

func index(term string) string {
	return href.PageIndex(href.QueryPageIndex{Term: term})
}

// mustDo sends an action and fails the test unless it answers 200.
func mustDo(
	t *testing.T, tab *datapagestest.Tab, method, path string, signals any,
) *datapagestest.Response {
	t.Helper()
	resp := tab.Do(t, method, path, signals)
	require.Equal(t, http.StatusOK, resp.Status, "%s %s: %s", method, path, resp.Body)
	return resp
}

// signIn signs v in as user through the app's sign-in action.
func signIn(t *testing.T, v *datapagestest.Visitor, user string) {
	t.Helper()
	login := v.Load(t, href.PageLogin())
	resp := mustDo(t, login, http.MethodPost, "/login/submit/",
		map[string]string{"user": user})
	require.Equal(t, "/room/", resp.Redirect(t))
}

// TestTabState tests that a stateful action needs its tab's stream, and reads the
// patch whose root is the table body from the action's own event stream.
func TestTabState(t *testing.T) {
	t.Parallel()
	c := newCase(t)
	tab := c.srv.Visitor(t).Load(t, index(""))
	require.NotEmpty(t, tab.Instance())

	resp := tab.Do(t, http.MethodPost, "/filter/", map[string]string{"term": ""})
	require.Equal(t, http.StatusConflict, resp.Status)
	require.Equal(t, "reconnect", resp.Retry())

	tab.Connect(t, tab.Page().Signals(t))
	mustDo(t, tab, http.MethodPost, "/add/", map[string]string{"title": "Buy milk"})
	mustDo(t, tab, http.MethodPost, "/add/", map[string]string{"title": "Buy bread"})
	evs := mustDo(
		t, tab, http.MethodPost, "/filter/",
		map[string]string{"term": "milk"},
	).Events(t)
	require.Len(t, evs, 1)
	require.Equal(t, "Buy milk", datapagestest.Text(evs[0].ElementByID(t, "rows")))
}

// TestRowPatch tests waiting for a patch whose root is a table row.
func TestRowPatch(t *testing.T) {
	t.Parallel()
	c := newCase(t)
	tab := c.srv.Visitor(t).Load(t, index(""))
	st := tab.Connect(t, tab.Page().Signals(t))

	mustDo(t, tab, http.MethodPost, "/add/", map[string]string{"title": "Buy milk"})
	ev := st.WaitFor(t, datapagestest.TextContains("row-1", "Buy milk"))
	require.Equal(t, "#rows", ev.Data["selector"])
	require.Equal(t, "append", ev.Data["mode"])
}

// TestAbsentOnOtherTab tests that a row reaches only the tab whose filter it matches.
// A later row that the tab does match is the anchor.
func TestAbsentOnOtherTab(t *testing.T) {
	t.Parallel()
	c := newCase(t)
	v := c.srv.Visitor(t)
	milk := v.Load(t, index("milk"))
	milkStream := milk.Connect(t, milk.Page().Signals(t))
	bread := v.Load(t, index("bread"))
	breadStream := bread.Connect(t, bread.Page().Signals(t))

	mustDo(t, bread, http.MethodPost, "/add/", map[string]string{"title": "Buy bread"})
	mustDo(t, bread, http.MethodPost, "/add/", map[string]string{"title": "Buy milk"})
	breadStream.WaitFor(t, datapagestest.TextContains("row-1", "Buy bread"))
	milkStream.Absent(t,
		datapagestest.Element("row-1"),
		datapagestest.TextContains("row-2", "Buy milk"))
}

// TestSignalPatch tests reading a signal patch that deletes a signal.
func TestSignalPatch(t *testing.T) {
	t.Parallel()
	c := newCase(t)
	tab := c.srv.Visitor(t).Load(t, index(""))
	require.Equal(t, "it's", tab.Page().Signals(t)["draft"])

	evs := mustDo(t, tab, http.MethodPost, "/clear/", nil).Events(t)
	require.Len(t, evs, 1)
	signals := evs[0].Signals(t)
	require.JSONEq(t, "null", string(signals["draft"]))
	require.JSONEq(t, "0", string(signals["count"]))
}

// TestReflectedSignal tests reading the single-quoted signal the generator writes for
// a query parameter whose value holds a quote.
func TestReflectedSignal(t *testing.T) {
	t.Parallel()
	c := newCase(t)
	signals := c.srv.Visitor(t).Get(t, index("it's")).Signals(t)
	require.Equal(t, "it's", signals["term"])
	require.Equal(t, float64(0), signals["count"])
}

// TestDeleteSignals tests that a DELETE sends its signals in the query.
func TestDeleteSignals(t *testing.T) {
	t.Parallel()
	c := newCase(t)
	tab := c.srv.Visitor(t).Load(t, index(""))
	mustDo(t, tab, http.MethodPost, "/add/", map[string]string{"title": "Buy milk"})

	confirm := func(ok bool) int {
		return tab.Do(
			t, http.MethodDelete, "/row/1/",
			map[string]bool{"confirm": ok},
		).Status
	}
	require.Equal(t, http.StatusBadRequest, confirm(false))
	require.Equal(t, http.StatusOK, confirm(true))
	require.Equal(t, http.StatusNotFound, confirm(true))
}

// TestRedirects tests reading the target of each form a redirect takes.
func TestRedirects(t *testing.T) {
	t.Parallel()
	c := newCase(t)
	v := c.srv.Visitor(t)

	old := v.Get(t, href.PageOld())
	require.Equal(t, http.StatusFound, old.Status)
	require.Equal(t, "/done/", old.Redirect(t))

	tab := v.Load(t, index(""))
	script := mustDo(t, tab, http.MethodPost, "/go/", nil)
	require.Contains(t, script.Header.Get("Content-Type"), "text/javascript")
	require.Equal(t, "/done/", script.Redirect(t))

	event := mustDo(t, tab, http.MethodPost, "/go-stream/", nil)
	require.Len(t, event.Events(t), 1)
	require.Equal(t, "/done/", event.Redirect(t))
}

// TestFailedStreamAction tests that an action whose event stream started answers 200
// with no events when its handler fails with a sentinel.
func TestFailedStreamAction(t *testing.T) {
	t.Parallel()
	c := newCase(t)
	tab := c.srv.Visitor(t).Load(t, index(""))
	resp := tab.Do(t, http.MethodPost, "/fail/", nil)
	require.Equal(t, http.StatusOK, resp.Status)
	require.Empty(t, resp.Events(t))
}

// TestSessions tests signing in through the app's action, the CSRF token the client
// derives, and an event that reaches only the user it addresses.
func TestSessions(t *testing.T) {
	t.Parallel()
	c := newCase(t)

	alice := c.srv.Visitor(t)
	signIn(t, alice, "alice")
	aliceRoom := alice.Load(t, href.PageRoom())
	require.Equal(t,
		"alice", datapagestest.Text(aliceRoom.Page().ElementByID(t, "said")))
	aliceStream := aliceRoom.Connect(t, map[string]string{"room": "lobby"})

	bob := c.srv.Visitor(t)
	signIn(t, bob, "bob")
	bobRoom := bob.Load(t, href.PageRoom())
	bobStream := bobRoom.Connect(t, map[string]string{"room": "lobby"})

	mustDo(t, bobRoom, http.MethodPost, "/room/notify/",
		map[string]string{"user": "alice", "text": "psst"})
	aliceStream.WaitFor(t, datapagestest.TextContains("said", "notice: psst"))

	mustDo(t, bobRoom, http.MethodPost, "/room/say/",
		map[string]string{"room": "lobby", "text": "after"})
	bobStream.Absent(t,
		datapagestest.TextContains("said", "psst"),
		datapagestest.TextContains("said", "lobby: after"))

	req := bobRoom.NewRequest(t, http.MethodPost, "/room/notify/",
		strings.NewReader(`{"user":"alice","text":"forged"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Del("X-CSRF-Token")
	require.Equal(t, http.StatusForbidden, bob.Send(t, req).Status)
}

// TestAnonymousStream tests that a guest's stream follows the redirect to the
// anonymous stream and receives an event published for its room signal, with a room
// name the dispatcher escapes.
func TestAnonymousStream(t *testing.T) {
	t.Parallel()
	c := newCase(t)
	room := c.srv.Visitor(t).Load(t, href.PageRoom())
	require.Equal(t, "guest", datapagestest.Text(room.Page().ElementByID(t, "said")))
	st := room.Connect(t, map[string]string{"room": "lobby.1"})

	datapagestest.Publish(t, c.broker,
		datapagestest.Subject(datapagesgen.EvPrefixRoomSaid, "lobby.1"),
		app.EventRoomSaid{Room: "lobby.1", Text: "hi"})
	st.WaitFor(t, datapagestest.TextContains("said", "lobby.1: hi"))
}

// TestSetSessionToken tests signing a visitor in with a session created through the
// session store.
func TestSetSessionToken(t *testing.T) {
	t.Parallel()
	c := newCase(t)
	token, err := c.sessions.CreateSession(t.Context(),
		sessions.Record[struct{}]{UserID: "carol"})
	require.NoError(t, err)

	v := c.srv.Visitor(t)
	v.SetSessionToken(token)
	room := v.Load(t, href.PageRoom())
	require.Equal(t, "carol", datapagestest.Text(room.Page().ElementByID(t, "said")))
}

// TestSessionExpiryInBubble tests, in a testing/synctest bubble whose clock the test
// advances, that a session the store gave an expiry signs the visitor out once that
// time passes, with the visitor's stream open meanwhile.
func TestSessionExpiryInBubble(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		c := newCase(t)
		token, err := c.sessions.CreateSession(t.Context(), sessions.Record[struct{}]{
			UserID: "carol", ExpiresAt: time.Now().Add(time.Hour),
		})
		require.NoError(t, err)
		v := c.srv.Visitor(t)
		v.SetSessionToken(token)
		room := v.Load(t, href.PageRoom())
		require.Equal(t, "carol", datapagestest.Text(room.Page().ElementByID(t, "said")))
		room.Connect(t, map[string]string{"room": "lobby"})

		time.Sleep(time.Hour + time.Second)
		after := v.Load(t, href.PageRoom())
		require.Equal(t, "guest", datapagestest.Text(after.Page().ElementByID(t, "said")))
	})
}

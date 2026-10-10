// Tests the streams a page serves to a visitor with no session.

package acceptance_test

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/datapagestest"
	"github.com/romshark/datapages/internal/acceptance/anonstreams/app"
	"github.com/romshark/datapages/internal/acceptance/anonstreams/app/datapagesgen"
	"github.com/romshark/datapages/internal/acceptance/brokers"
	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/sessions"
	sessinmem "github.com/romshark/datapages/modules/sessions/inmem"
)

func TestMain(m *testing.M) { os.Exit(brokers.Main(m)) }

func newSessions() *sessinmem.SessionManager[struct{}] {
	return sessinmem.New[struct{}](
		sessions.DefaultTokenGenerator{Length: sessions.DefaultTokenLen},
	)
}

func serve(t *testing.T, broker messaging.Broker) *datapagestest.Server {
	t.Helper()
	return datapagestest.New(t, mustNewServer(t, &app.App{}, broker, newSessions()))
}

// postOK sends an action and requires the server to accept it.
// What the test is after is on the stream, not in this response.
func postOK(t *testing.T, tab *datapagestest.Tab, path string, signals any) {
	t.Helper()
	resp := tab.Do(t, http.MethodPost, path, signals)
	require.Equal(t, http.StatusOK, resp.Status, "POST %s: %s", path, resp.Body)
}

func room(name string) map[string]string { return map[string]string{"room": name} }

func post(room, text string) map[string]string {
	return map[string]string{"room": room, "text": text}
}

func count(n int) datapagestest.Match {
	return datapagestest.TextContains("count", fmt.Sprintf("count %d", n))
}

// TestAnonStreamSubscribesBySignal tests a visitor with no session on a page
// that scopes its events by a signal: the stream receives what is published
// for the value it connected with, and nothing published for another.
func TestAnonStreamSubscribesBySignal(t *testing.T) {
	t.Parallel()
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		v := serve(t, broker).Visitor(t)

		oneTab := v.Load(t, "/rooms/")
		one := oneTab.Connect(t, room("one"))
		two := v.Load(t, "/rooms/").Connect(t, room("two"))

		postOK(t, oneTab, "/rooms/post/", post("one", "hello"))
		one.WaitFor(t, datapagestest.TextContains("out", "room one: hello"))

		// The later post to room two is the anchor: the post to room one would reach
		// the stream before it.
		postOK(t, oneTab, "/rooms/post/", post("two", "after"))
		two.Absent(t,
			datapagestest.TextContains("out", "hello"),
			datapagestest.TextContains("out", "room two: after"))
	})
}

// TestAnonStreamRedirectKeepsThePathValue tests the redirect that sends a
// signed-out visitor from a page's private stream route to its anonymous one.
// The request path arrives decoded: a slug carrying "?" or "#" re-parses in the
// Location header as a query or a fragment and the reconnect lands on the
// page's HTML route.
func TestAnonStreamRedirectKeepsThePathValue(t *testing.T) {
	t.Parallel()
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		for name, slug := range map[string]string{
			"plain":    "plain",
			"question": "a?b",
			"fragment": "a#b",
			"space":    "a b",
		} {
			t.Run(name, func(t *testing.T) {
				v := serve(t, broker).Visitor(t)
				tab := v.Load(t, "/post/"+url.PathEscape(slug)+"/")

				// Connect follows the redirect to the anonymous stream and fails unless
				// it answers 200 with text/event-stream, which the page's HTML route
				// does not.
				st := tab.Connect(t, nil)
				require.Equal(t, "/post/"+slug+"/_$/anon/", st.Path(),
					"the redirect lost the path value")

				// The stream carries the page's live updates.
				datapagestest.Publish(t, broker, datapagesgen.EvSubjTicked,
					app.EventTicked{N: 1})
				st.WaitFor(t, datapagestest.TextContains("out", "tick 1"))
			})
		}
	})
}

// TestAnonStreamCarriesNoPrivateEvent tests the reason the route exists:
// a visitor with no session is nobody, which leaves a private event no way to reach them.
func TestAnonStreamCarriesNoPrivateEvent(t *testing.T) {
	t.Parallel()
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		tab := serve(t, broker).Visitor(t).Load(t, "/rooms/")
		st := tab.Connect(t, room("one"))

		postOK(t, tab, "/rooms/notice/",
			map[string]string{"user": "alice", "text": "for alice"})

		// A post to the stream's room is the anchor: the private event would reach the
		// stream before it.
		postOK(t, tab, "/rooms/post/", post("one", "after"))
		st.Absent(t,
			datapagestest.TextContains("out", "for alice"),
			datapagestest.TextContains("out", "room one: after"))
	})
}

// TestStatefulPageRendersAnonStreamInit covers a page whose GET handler does
// not take Session. The generated HTTP handler still needs the session to
// select the anonymous stream that allocates the page's state.
func TestStatefulPageRendersAnonStreamInit(t *testing.T) {
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		page := serve(t, broker).Visitor(t).Get(t, "/tabs/")
		require.Equal(t, http.StatusOK, page.Status)
		require.Contains(t, page.Body,
			`data-init="@get('/tabs/_$/anon/',{retry:'always',retryMaxCount:Infinity})"`,
			"the rendered page does not start its anonymous state stream")
	})
}

// TestAnonStreamHoldsPerTabState covers a stateful page reached without a session:
// the tab gets an instance of its own, and its handlers are given the
// value that belongs to it.
func TestAnonStreamHoldsPerTabState(t *testing.T) {
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		v := serve(t, broker).Visitor(t)

		aTab := v.Load(t, "/tabs/")
		a := aTab.Connect(t, nil)
		bTab := v.Load(t, "/tabs/")
		b := bTab.Connect(t, nil)

		require.NotEqual(t, aTab.Instance(), bTab.Instance(),
			"two page loads share one instance id")

		for range 2 {
			postOK(t, aTab, "/tabs/bump/", nil)
		}

		// The event is public. Both tabs render, each from its own state.
		a.WaitFor(t, count(2), "the tab that acted does not hold its own count")
		b.WaitFor(t, count(0), "the other tab was not rendered from its own state")

		// The other tab's own bump is the anchor. It renders count 1 from that tab's
		// state, and the count of the first tab would arrive before it.
		postOK(t, bTab, "/tabs/bump/", nil)
		b.Absent(t, count(2), count(1), "one tab sees the count of another")
	})
}

// TestBackgroundStreamingKeepsBothStreamsOpen tests a page with public and
// private events whose GET returns enableBackgroundStreaming.
// A signed-in visitor connects to the page's stream and a guest to its anonymous one.
// Both have to stay open while the tab is hidden: the page does not reload
// when the tab becomes visible again, which would render what a closed stream missed.
func TestBackgroundStreamingKeepsBothStreamsOpen(t *testing.T) {
	t.Parallel()
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		tests := map[string]struct {
			path     string
			signedIn bool
			stream   string
		}{
			"guest":     {"/background/", false, "/background/_$/anon/"},
			"signed in": {"/background/", true, "/background/_$/"},
			"guest, path variable": {
				"/background-post/x/", false, "/background-post/x/_$/anon/",
			},
			"signed in, path variable": {
				"/background-post/x/", true, "/background-post/x/_$/",
			},
		}

		for name, tt := range tests {
			t.Run(name, func(t *testing.T) {
				store := newSessions()
				v := datapagestest.New(t,
					mustNewServer(t, &app.App{}, broker, store)).Visitor(t)
				if tt.signedIn {
					token, err := store.CreateSession(t.Context(),
						sessions.Record[struct{}]{UserID: "alice"})
					require.NoError(t, err)
					v.SetSessionToken(token)
				}
				resp := v.Get(t, tt.path)

				require.Equal(t, http.StatusOK, resp.Status, resp.Body)
				require.Contains(t, resp.Body,
					`data-init="@get('`+tt.stream+`',{openWhenHidden:true,`,
					"the page does not keep its stream open while the tab is hidden")
				require.NotContains(t, resp.Body, "data-on:visibilitychange",
					"the page reloads when the tab becomes visible again")
			})
		}
	})
}

// TestAnonStreamRecoversHandlerPanic tests a public event handler that panics
// on the stream of a visitor with no session. As on the signed-in stream,
// RecoverError receives a datapages.PanicError and StreamClose runs.
func TestAnonStreamRecoversHandlerPanic(t *testing.T) {
	t.Parallel()
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		a := &app.App{}
		srv := datapagestest.New(t, mustNewServer(t, a, broker, newSessions()))

		tab := srv.Visitor(t).Load(t, "/panic/")
		st := tab.Connect(t, nil)
		require.Equal(t, "/panic/_$/anon/", st.Path())
		postOK(t, tab, "/panic/fault/", nil)

		st.WaitFor(t, datapagestest.TextContains("out",
			"recovered: panic: the faulted handler panicked"))
		srv.WaitFor(t, "StreamClose to run", func() bool {
			_, closed := a.PanicStreams()
			return closed > 0
		})
		opened, closed := a.PanicStreams()
		require.Equal(t, 1, opened, "StreamOpen")
		require.Equal(t, 1, closed, "StreamClose")

		recovered := a.Recovered()
		require.Len(t, recovered, 1)
		var pe datapages.PanicError
		require.ErrorAs(t, recovered[0], &pe)
		require.Equal(t, "the faulted handler panicked", pe.Value)
	})
}

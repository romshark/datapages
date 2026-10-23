// Tests the streams a page serves to a visitor with no session.

package acceptance_test

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/internal/acceptance/anonstreams/app"
	"github.com/romshark/datapages/internal/acceptance/brokers"
	"github.com/romshark/datapages/internal/acceptance/client"
	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/sessions"
	sessinmem "github.com/romshark/datapages/modules/sessions/inmem"
)

func TestMain(m *testing.M) { os.Exit(brokers.Main(m)) }

func newClient(t *testing.T, broker messaging.Broker) *client.Client {
	t.Helper()
	sessions := sessinmem.New[struct{}](
		sessions.DefaultTokenGenerator{Length: sessions.DefaultTokenLen},
	)
	return client.New(t, mustNewServer(t, &app.App{}, broker, sessions))
}

// load requests path the way a browser loads a page, signed in as alice or not.
func load(
	t *testing.T, broker messaging.Broker, path string, signedIn bool,
) client.Response {
	t.Helper()
	store := sessinmem.New[struct{}](
		sessions.DefaultTokenGenerator{Length: sessions.DefaultTokenLen},
	)
	c := client.New(t, mustNewServer(t, &app.App{}, broker, store))
	req := c.Request(t, http.MethodGet, path, "")
	// A page load, which Datastar does not send.
	req.Header.Del("Datastar-Request")
	if signedIn {
		token, err := store.CreateSession(context.Background(),
			sessions.Record[struct{}]{UserID: "alice"})
		require.NoError(t, err)
		req.AddCookie(&http.Cookie{
			Name: datapages.DefaultSessionCookieName, Value: token,
		})
	}
	return c.Do(t, req)
}

// TestAnonStreamSubscribesBySignal tests a visitor with no session on a page
// that scopes its events by a signal: the stream receives what is published
// for the value it connected with, and nothing published for another.
func TestAnonStreamSubscribesBySignal(t *testing.T) {
	t.Parallel()
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		c := newClient(t, broker)

		one := c.OpenStream(t, "/rooms/_$/", map[string]string{"room": "one"})
		two := c.OpenStream(t, "/rooms/_$/", map[string]string{"room": "two"})

		resp := c.Action(t, http.MethodPost, "/rooms/post/",
			`{"room":"one","text":"hello"}`)
		require.Equal(t, http.StatusOK, resp.Status)

		require.True(t, one.Saw(`<div id="out">room one: hello</div>`),
			"the anonymous stream received nothing for the room it connected with")
		require.True(t, two.Never("hello"),
			"the anonymous stream received what was published for another room")
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
				c := newClient(t, broker)
				path := "/post/" + url.PathEscape(slug) + "/_$/"

				// OpenStream requires 200: reaching it at all is the claim.
				s := c.OpenStream(t, path, nil)

				resp := c.Action(t, http.MethodPost, "/rooms/post/",
					`{"room":"one","text":"x"}`)
				require.Equal(t, http.StatusOK, resp.Status)
				require.True(t, s.Never("<!DOCTYPE html>"),
					"the reconnect landed on the page's HTML route")
			})
		}
	})
}

// TestAnonStreamCarriesNoPrivateEvent tests the reason the route exists:
// a visitor with no session is nobody, which leaves a private event no way to reach them.
func TestAnonStreamCarriesNoPrivateEvent(t *testing.T) {
	t.Parallel()
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		c := newClient(t, broker)

		s := c.OpenStream(t, "/rooms/_$/", map[string]string{"room": "one"})

		resp := c.Action(t, http.MethodPost, "/rooms/notice/",
			`{"user":"alice","text":"for alice"}`)
		require.Equal(t, http.StatusOK, resp.Status)

		require.True(t, s.Never("for alice"),
			"a private event reached a stream of a visitor with no session")
	})
}

// TestStatefulPageRendersAnonStreamInit covers a page whose GET handler does
// not take Session. The generated HTTP handler still needs the session to
// select the anonymous stream that allocates the page's state.
func TestStatefulPageRendersAnonStreamInit(t *testing.T) {
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		c := newClient(t, broker)

		page := c.Get(t, "/tabs/")
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
		c := newClient(t, broker)

		a := c.OpenTab(t, "/tabs/", "/tabs/_$/")
		b := c.OpenTab(t, "/tabs/", "/tabs/_$/")

		require.NotEqual(t, a.InstanceID(), b.InstanceID(),
			"2 page loads share 1 instance id")

		for range 2 {
			require.Equal(t, http.StatusOK,
				a.Act(t, http.MethodPost, "/tabs/bump/", "").Status, "bumping")
		}

		// The event is public. Both tabs render, each from its own state.
		require.True(t, a.Saw(`<div id="count">count 2</div>`),
			"the tab that acted does not hold its own count")
		require.True(t, b.Saw(`<div id="count">count 0</div>`),
			"the other tab was not rendered from its own state")
		require.True(t, b.Never("count 2"), "one tab sees the count of another")
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
				resp := load(t, broker, tt.path, tt.signedIn)

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

// TestPrivatePageStreamsOnlyWithASession tests that a stateless page with only
// private events gives a guest neither a stream attribute nor the visibility reload,
// also when the stream path holds a path variable.
func TestPrivatePageStreamsOnlyWithASession(t *testing.T) {
	t.Parallel()
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		for name, tt := range map[string]struct {
			path, stream string
			signedIn     bool
		}{
			"guest":     {path: "/inbox/", stream: "/inbox/_$/", signedIn: false},
			"signed in": {path: "/inbox/", stream: "/inbox/_$/", signedIn: true},
			"guest, path variable": {
				path: "/threads/t1/", stream: "/threads/t1/_$/", signedIn: false,
			},
			"signed in, path variable": {
				path: "/threads/t1/", stream: "/threads/t1/_$/", signedIn: true,
			},
		} {
			t.Run(name, func(t *testing.T) {
				resp := load(t, broker, tt.path, tt.signedIn)

				require.Equal(t, http.StatusOK, resp.Status, resp.Body)
				require.Equal(t, tt.signedIn, strings.Contains(resp.Body, "data-init="),
					"the page writes a stream attribute")
				require.Equal(t, tt.signedIn, strings.Contains(resp.Body, tt.stream),
					"the page starts a stream")
				require.Equal(t, tt.signedIn,
					strings.Contains(resp.Body, "data-on:visibilitychange"),
					"the page reloads when the tab becomes visible again")
			})
		}
	})
}

// TestPrivateStatefulPageStreamsForGuests tests a stateful page whose events
// are all private. A guest gets the anonymous stream, which allocates the per-tab
// state the actions of the guest need. Without a stream, each of them answers 409.
func TestPrivateStatefulPageStreamsForGuests(t *testing.T) {
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		for name, tt := range map[string]struct {
			signedIn bool
			stream   string
		}{
			"guest":     {false, "/notes/_$/anon/"},
			"signed in": {true, "/notes/_$/"},
		} {
			t.Run(name, func(t *testing.T) {
				resp := load(t, broker, "/notes/", tt.signedIn)

				require.Equal(t, http.StatusOK, resp.Status, resp.Body)
				require.Contains(t, resp.Body, `data-init="@get('`+tt.stream+`',`,
					"the page does not start the stream of its visitor")
			})
		}

		c := newClient(t, broker)
		tab := c.OpenTab(t, "/notes/", "/notes/_$/anon/")
		resp := tab.Act(t, http.MethodPost, "/notes/count/", "")
		require.Equal(t, http.StatusOK, resp.Status, "a state action of a guest")
		require.Contains(t, resp.Body, `<div id="count">count 1</div>`)
	})
}

// TestPrivateStatefulPageReloadsAGuestStream tests a guest request on the
// stream of a stateful page whose events are all private. The page writes that
// stream only for a session: the request comes from a page whose session ended
// while the stream was disconnected, and the page reloads rather than keep
// showing the old session on the anonymous stream.
func TestPrivateStatefulPageReloadsAGuestStream(t *testing.T) {
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		c := newClient(t, broker)

		s := c.OpenStream(t, "/notes/_$/", nil)
		require.True(t, s.Saw("location.reload()"),
			"the page was not told to reload")
	})
}

// TestAnonStreamRecoversHandlerPanic tests a public event handler that panics
// on the stream of a visitor with no session. As on the signed-in stream,
// RecoverError receives a datapages.PanicError and StreamClose runs.
func TestAnonStreamRecoversHandlerPanic(t *testing.T) {
	t.Parallel()
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		a := &app.App{}
		store := sessinmem.New[struct{}](
			sessions.DefaultTokenGenerator{Length: sessions.DefaultTokenLen},
		)
		c := client.New(t, mustNewServer(t, a, broker, store))

		s := c.OpenStream(t, "/panic/_$/anon/", nil)
		resp := c.Action(t, http.MethodPost, "/panic/fault/", "")
		require.Equal(t, http.StatusOK, resp.Status)

		require.True(t,
			s.Saw(`<div id="out">recovered: panic: the faulted handler panicked</div>`),
			"RecoverError did not answer on the stream")
		require.True(t, client.WaitFor(func() bool {
			_, closed := a.PanicStreams()
			return closed > 0
		}, client.Await), "StreamClose did not run")
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

// TestAnonStreamRecoversDecodePanic tests a StreamOpen signal whose
// UnmarshalJSON panics, on the stream of a visitor with no session.
// RecoverError receives a datapages.PanicError. net/http otherwise
// closes the connection without a response.
func TestAnonStreamRecoversDecodePanic(t *testing.T) {
	t.Parallel()
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		a := &app.App{}
		store := sessinmem.New[struct{}](
			sessions.DefaultTokenGenerator{Length: sessions.DefaultTokenLen},
		)
		c := client.New(t, mustNewServer(t, a, broker, store))

		s := c.OpenStream(t, "/decode/_$/anon/", map[string]string{"v": "xbad"})
		require.True(t,
			s.Saw(`<div id="out">recovered: panic: the JSON parser panicked</div>`),
			"RecoverError did not answer on the stream")

		recovered := a.Recovered()
		require.Len(t, recovered, 1)
		var pe datapages.PanicError
		require.ErrorAs(t, recovered[0], &pe)
		require.Equal(t, "the JSON parser panicked", pe.Value)
	})
}

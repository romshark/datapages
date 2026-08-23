// Drives the streams a page serves to a visitor with no session.

package acceptance_test

import (
	"crypto/sha256"
	"net/http"
	"os"
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
	stateKey := sha256.Sum256([]byte("acceptance-state"))
	return client.New(t, mustNewServer(t, &app.App{}, broker, sessions,
		datapages.WithStateConfig(datapages.StateConfig{
			HMACKey: stateKey[:],
		})))
}

// TestAnonStreamSubscribesBySignal covers a visitor with no session on a page
// that scopes its events by a signal: the stream receives what is published
// for the value it connected with, and nothing published for another.
func TestAnonStreamSubscribesBySignal(t *testing.T) {
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

// TestAnonStreamCarriesNoPrivateEvent covers the reason the route exists:
// a visitor with no session is nobody, so a private event has no way to reach them.
func TestAnonStreamCarriesNoPrivateEvent(t *testing.T) {
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

// TestAnonStreamHoldsPerTabState covers a stateful page reached without a session:
// the tab gets an instance of its own, and its handlers are given the
// value that belongs to it.
func TestAnonStreamHoldsPerTabState(t *testing.T) {
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		c := newClient(t, broker)

		a := c.OpenTab(t, "/tabs/", "/tabs/_$/")
		b := c.OpenTab(t, "/tabs/", "/tabs/_$/")

		require.NotEqual(t, a.InstanceID(), b.InstanceID(),
			"two page loads share one instance id")

		for range 2 {
			require.Equal(t, http.StatusOK,
				a.Act(t, http.MethodPost, "/tabs/bump/", "").Status, "bumping")
		}

		// The event is public, so both tabs render — each from its own state.
		require.True(t, a.Saw(`<div id="count">count 2</div>`),
			"the tab that acted does not hold its own count")
		require.True(t, b.Saw(`<div id="count">count 0</div>`),
			"the other tab was not rendered from its own state")
		require.True(t, b.Never("count 2"), "one tab sees the count of another")
	})
}

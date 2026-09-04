// Tests the streams a page serves to a visitor with no session.

package acceptance_test

import (
	"net/http"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

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

// TestAnonStreamCarriesNoMultiFieldPrivateEvent tests an event whose two
// subject fields share one declaration line. Reading only the first name
// leaves the event public and delivers it to a visitor with no session.
func TestAnonStreamCarriesNoMultiFieldPrivateEvent(t *testing.T) {
	t.Parallel()
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		c := newClient(t, broker)

		s := c.OpenStream(t, "/rooms/_$/", map[string]string{"room": "one"})

		resp := c.Action(t, http.MethodPost, "/rooms/dm/",
			`{"to":"alice","cc":"bob","text":"for alice and bob"}`)
		require.Equal(t, http.StatusOK, resp.Status)

		require.True(t, s.Never("for alice and bob"),
			"a private event reached a stream of a visitor with no session")
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

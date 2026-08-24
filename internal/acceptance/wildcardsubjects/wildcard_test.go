// Asserts that an event dispatched with a subject value reaches a stream
// subscribed to every value of that subject.

package acceptance_test

import (
	"net/http"
	"os"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/internal/acceptance/brokers"
	"github.com/romshark/datapages/internal/acceptance/client"
	"github.com/romshark/datapages/internal/acceptance/wildcardsubjects/app"
	"github.com/romshark/datapages/modules/messaging"
)

func TestMain(m *testing.M) { os.Exit(brokers.Main(m)) }

// TestWildcardSubjectDelivery covers a page subscribed to every value of a subject:
// the event reaches it whatever value it was dispatched with.
func TestWildcardSubjectDelivery(t *testing.T) {
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		c := client.New(t, mustNewServer(t, &app.App{}, broker))

		s := c.OpenStream(t, "/_$/", nil)

		resp := c.Action(t, http.MethodPost, "/note/",
			`{"topic":"anything","text":"delivered"}`)
		require.Equal(t, http.StatusOK, resp.Status, "POST /note/")

		require.True(t, s.Saw(`<div id="noted">delivered</div>`),
			"the stream subscribed to every value of the subject received nothing")
	})
}

// TestSecondValueReachesTheSameStream covers a second value of the same
// subject reaching the same stream.
func TestSecondValueReachesTheSameStream(t *testing.T) {
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		c := client.New(t, mustNewServer(t, &app.App{}, broker))

		s := c.OpenStream(t, "/_$/", nil)

		for _, topic := range []string{"one", "two"} {
			resp := c.Action(t, http.MethodPost, "/note/",
				`{"topic":"`+topic+`","text":"from-`+topic+`"}`)
			require.Equal(t, http.StatusOK, resp.Status, "POST /note/ %s", topic)
		}

		require.True(t, s.Saw(`<div id="noted">from-one</div>`))
		require.True(t, s.Saw(`<div id="noted">from-two</div>`))
	})
}

// TestSubjectValueMustBeOneToken covers the publish side of the guard the
// subscribe side already carries. A dispatched value that is not one subject
// token produces a subject of a different shape, which no subscription matches
// and which therefore loses the event in silence.
// TestSubjectValueIsEscaped covers a subject field value that cannot stand in
// a subject as it is. Escaped it names one segment, which the subscription
// this page opens over every value of the field matches like any other.
func TestSubjectValueIsEscaped(t *testing.T) {
	values := map[string]string{
		"plain":     "plain",
		"separator": "a.b",
		"star":      "*",
		"gt":        ">",
		"space":     "a b",
		"percent":   "100%",
	}

	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		for name, topic := range values {
			t.Run(name, func(t *testing.T) {
				c := client.New(t, mustNewServer(t, &app.App{}, broker))

				s := c.OpenStream(t, "/_$/", nil)

				resp := c.Action(t, http.MethodPost, "/note/",
					`{"topic":`+strconv.Quote(topic)+`,"text":"kept"}`)
				require.Equal(t, http.StatusOK, resp.Status,
					"a dispatch with topic %q was refused", topic)

				require.True(t, s.Saw(`<div id="noted">kept</div>`),
					"the event never reached the stream")
			})
		}
	})
}

// TestSubjectValueMustNotBeEmpty covers the one value escaping cannot save. An empty
// value names no segment, which leaves a subject of a shape no subscription matches.
func TestSubjectValueMustNotBeEmpty(t *testing.T) {
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		c := client.New(t, mustNewServer(t, &app.App{}, broker))

		s := c.OpenStream(t, "/_$/", nil)

		resp := c.Action(t, http.MethodPost, "/note/",
			`{"topic":"","text":"lost"}`)
		require.Equal(t, http.StatusInternalServerError, resp.Status,
			"a dispatch with an empty topic was accepted")

		require.True(t, s.Never(`<div id="noted">lost</div>`),
			"the event reached a stream")
	})
}

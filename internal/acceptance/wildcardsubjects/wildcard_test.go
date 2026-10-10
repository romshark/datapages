// Asserts that an event dispatched with a subject value reaches a stream
// subscribed to every value of that subject.

package acceptance_test

import (
	"net/http"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/datapagestest"
	"github.com/romshark/datapages/internal/acceptance/brokers"
	"github.com/romshark/datapages/internal/acceptance/wildcardsubjects/app"
	"github.com/romshark/datapages/modules/messaging"
)

func TestMain(m *testing.M) { os.Exit(brokers.Main(m)) }

func serve(t *testing.T, broker messaging.Broker) *datapagestest.Server {
	t.Helper()
	return datapagestest.New(t, mustNewServer(t, &app.App{}, broker))
}

// postNote sends the action that dispatches the event with topic as its subject value.
func postNote(
	t *testing.T, tab *datapagestest.Tab, topic, text string,
) *datapagestest.Response {
	t.Helper()
	return tab.Do(t, http.MethodPost, "/note/",
		map[string]string{"topic": topic, "text": text})
}

// noted matches the patch of an event that carried text.
func noted(text string) datapagestest.Match {
	return datapagestest.TextContains("noted", text)
}

// TestWildcardSubjectDelivery tests a page subscribed to every value of a subject:
// the event reaches it whatever value it was dispatched with.
func TestWildcardSubjectDelivery(t *testing.T) {
	t.Parallel()
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		tab := serve(t, broker).Visitor(t).Load(t, "/")
		st := tab.Connect(t, nil)

		resp := postNote(t, tab, "anything", "delivered")
		require.Equal(t, http.StatusOK, resp.Status, "POST /note/")

		st.WaitFor(t, noted("delivered"))
	})
}

// TestSecondValueReachesTheSameStream tests a second value of the same
// subject reaching the same stream.
func TestSecondValueReachesTheSameStream(t *testing.T) {
	t.Parallel()
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		tab := serve(t, broker).Visitor(t).Load(t, "/")
		st := tab.Connect(t, nil)

		for _, topic := range []string{"one", "two"} {
			resp := postNote(t, tab, topic, "from-"+topic)
			require.Equal(t, http.StatusOK, resp.Status, "POST /note/ %s", topic)
		}

		st.WaitFor(t, noted("from-one"))
		st.WaitFor(t, noted("from-two"))
	})
}

// TestSubjectValueIsEscaped tests a subject field value that cannot stand in
// a subject as it is. Escaped it names one segment, which the subscription
// this page opens over every value of the field matches like any other.
func TestSubjectValueIsEscaped(t *testing.T) {
	t.Parallel()
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
				tab := serve(t, broker).Visitor(t).Load(t, "/")
				st := tab.Connect(t, nil)

				resp := postNote(t, tab, topic, "kept")
				require.Equal(t, http.StatusOK, resp.Status,
					"a dispatch with topic %q was refused", topic)

				st.WaitFor(t, noted("kept"))
			})
		}
	})
}

// TestSubjectValueMustNotBeEmpty tests the one value escaping cannot save. An empty
// value names no segment, which leaves a subject of a shape no subscription matches.
func TestSubjectValueMustNotBeEmpty(t *testing.T) {
	t.Parallel()
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		tab := serve(t, broker).Visitor(t).Load(t, "/")
		st := tab.Connect(t, nil)

		resp := postNote(t, tab, "", "lost")
		require.Equal(t, http.StatusInternalServerError, resp.Status,
			"a dispatch with an empty topic was accepted")

		// A dispatch with a topic is the anchor:
		// the refused event would reach the stream before it.
		resp = postNote(t, tab, "anchor", "after")
		require.Equal(t, http.StatusOK, resp.Status, "POST /note/")
		st.Absent(t, noted("lost"), noted("after"))
	})
}

// Tests the generated event routing of ./app.

package acceptance_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/internal/acceptance/brokers"
	"github.com/romshark/datapages/internal/acceptance/client"
	"github.com/romshark/datapages/internal/acceptance/events/app"
	"github.com/romshark/datapages/internal/acceptance/events/app/datapagesgen"
	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/messaging/inmem"
)

// postOK sends an action and requires the server to accept it.
// What the test is after is on the stream, not in this response.
func postOK(t *testing.T, c *client.Client, path, body string) {
	t.Helper()
	resp := c.Action(t, http.MethodPost, path, body)
	require.Equal(t, http.StatusOK, resp.Status, "POST %s", path)
}

// logOf reads what the application recorded while its handlers ran.
func logOf(t *testing.T, c *client.Client) string {
	t.Helper()
	return c.Get(t, "/log/").Element(t, "echo")
}

// TestPublicEventReachesEveryStream tests an event with no subject field:
// every open stream of the page receives the patch,
// not only the one whose request dispatched it.
func TestPublicEventReachesEveryStream(t *testing.T) {
	t.Parallel()
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		c := client.New(t, mustNewServer(t, &app.App{}, broker))

		a := c.OpenStream(t, "/_$/", nil)
		b := c.OpenStream(t, "/_$/", nil)

		postOK(t, c, "/tick/", `{"n":7}`)

		require.True(t, a.Saw(`<div id="out">tick 7</div>`),
			"the first stream received no patch")
		require.True(t, b.Saw(`<div id="out">tick 7</div>`),
			"the second stream received no patch")
	})
}

// TestStreamHooks tests the two hooks that bracket a stream's life.
// Both must run and both must see the same stream id.
// An application uses that id to pair up what it allocated with what it releases.
func TestStreamHooks(t *testing.T) {
	t.Parallel()
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		c := client.New(t, mustNewServer(t, &app.App{}, broker))

		s := c.OpenStream(t, "/_$/", nil)
		opened := logOf(t, c)
		if !strings.HasPrefix(opened, "open(") {
			t.Fatalf("StreamOpen did not run: %q", opened)
		}
		id := strings.TrimSuffix(strings.TrimPrefix(opened, "open("), ")")

		s.Close()

		if got, want := logOf(t, c), "open("+id+") close("+id+")"; got != want {
			t.Errorf(" got: %s\nwant: %s", got, want)
		}
	})
}

// TestStreamIDReachesEventHandler tests the stream id an event handler may ask for.
// It must name the stream the handler is writing on, not some other.
func TestStreamIDReachesEventHandler(t *testing.T) {
	t.Parallel()
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		c := client.New(t, mustNewServer(t, &app.App{}, broker))

		_ = c.OpenStream(t, "/_$/", nil)
		opened := logOf(t, c)
		id := strings.TrimSuffix(strings.TrimPrefix(opened, "open("), ")")

		postOK(t, c, "/tick/", `{"n":3}`)
		time.Sleep(client.Settle)

		if got, want := logOf(t, c), "open("+id+") tick("+id+",3)"; got != want {
			t.Errorf(" got: %s\nwant: %s", got, want)
		}
	})
}

// TestEmbeddedHandler tests a page that declares no event handler of its own
// and embeds one. The page receives the event through the embedded type.
func TestEmbeddedHandler(t *testing.T) {
	t.Parallel()
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		c := client.New(t, mustNewServer(t, &app.App{}, broker))

		other := c.OpenStream(t, "/other/_$/", nil)
		postOK(t, c, "/tick/", `{"n":1}`)

		require.True(t, other.Saw(`<div id="shared">shared 1</div>`),
			"the embedding page received nothing from the embedded handler")
	})
}

// TestTwoDispatchers tests an action that declares one dispatcher per event type.
// Both events must reach the stream, since each dispatcher publishes on
// its own and neither depends on the other.
func TestTwoDispatchers(t *testing.T) {
	t.Parallel()
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		c := client.New(t, mustNewServer(t, &app.App{}, broker))

		s := c.OpenStream(t, "/_$/", nil)

		postOK(t, c, "/both/", `{"n":3}`)

		require.True(t, s.Saw(`<div id="out">tick 3</div>`),
			"the first dispatcher delivered nothing")
		require.True(t, s.Saw(`<div id="pong">pong 3</div>`),
			"the second dispatcher delivered nothing")
	})
}

// TestStreamCloseDispatches tests dispatching from StreamClose. The hook runs
// while the closing stream is torn down, which leaves its request context already done.
// The dispatcher must publish with that cancelation stripped.
// The broker here refuses a dead context, which is what makes the difference visible:
// a stream that stays open must receive what the closing one sent.
func TestStreamCloseDispatches(t *testing.T) {
	t.Parallel()
	broker := &ctxBroker{Broker: inmem.New(messaging.DefaultBrokerChanBuffer)}
	c := client.New(t, mustNewServer(t, &app.App{}, broker))

	watcher := c.OpenStream(t, "/_$/", nil)
	leaving := c.OpenStream(t, "/_$/", nil)

	leaving.Close()

	require.True(t, watcher.Saw(`<div id="gone">gone `),
		"the event dispatched from StreamClose never arrived")
}

// TestEventDecodeIsNotCarriedOver tests a field the JSON of the second event
// leaves out. The stream must show it empty, not what the first event carried.
func TestEventDecodeIsNotCarriedOver(t *testing.T) {
	t.Parallel()
	c := client.New(t, mustNewServer(t, &app.App{},
		inmem.New(messaging.DefaultBrokerChanBuffer)))

	s := c.OpenStream(t, "/_$/", nil)

	postOK(t, c, "/note/", `{"text":"first"}`)
	require.True(t, s.Saw(`<div id="note">note=first</div>`),
		"the first note never arrived")

	postOK(t, c, "/note/", `{"text":""}`)
	require.True(t, s.Saw(`<div id="note">note=</div>`),
		"the empty note kept the text of the one before it")
}

// TestDispatchCtx tests Dispatcher.DispatchCtx.
// The action dispatches with a context that is already done.
// A broker that honors the context refuses to publish and the action fails.
// Dispatch would use the live request context and succeed.
func TestDispatchCtx(t *testing.T) {
	t.Parallel()
	broker := &ctxBroker{Broker: inmem.New(messaging.DefaultBrokerChanBuffer)}
	c := client.New(t, mustNewServer(t, &app.App{}, broker))

	s := c.OpenStream(t, "/_$/", nil)

	postOK(t, c, "/tick/", `{"n":1}`)
	require.True(t, s.Saw(`<div id="out">tick 1</div>`),
		"the handler context was refused by the broker")

	resp := c.Action(t, http.MethodPost, "/canceled/", `{"n":2}`)
	require.Equal(t, http.StatusInternalServerError, resp.Status,
		"the canceled context did not reach the broker")
	require.True(t, s.Never("tick 2"),
		"the event was published with a canceled context")
}

// ctxBroker publishes only while the context it is given is alive.
// The in-memory broker ignores the context, which would hide what
// TestDispatchCtx is after.
type ctxBroker struct {
	messaging.Broker
}

func (b *ctxBroker) Publish(
	ctx context.Context, metrics messaging.Metrics, subject string, data []byte,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return b.Broker.Publish(ctx, metrics, subject, data)
}

// TestSubjectScoping tests an event whose subject carries a value the stream
// chose when it connected. Two streams of one page, two values, one dispatch:
// only the addressed stream may see it.
func TestSubjectScoping(t *testing.T) {
	t.Parallel()
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		c := client.New(t, mustNewServer(t, &app.App{}, broker))

		red := c.OpenStream(t, "/room/_$/", map[string]string{"room": "red"})
		blue := c.OpenStream(t, "/room/_$/", map[string]string{"room": "blue"})

		postOK(t, c, "/room/say/", `{"room":"red","text":"hello red"}`)

		require.True(t, red.Saw(`<div id="said">hello red</div>`),
			"the addressed stream received nothing")
		require.True(t, blue.Never("hello red"),
			"a stream received an event addressed at another")
	})
}

// TestOneCopyPerStream tests how many messages a subscription receives.
// A page subscribes to one subject per event it handles, and the room page handles two,
// which a broker that delivers per matching subscription can turn into more copies
// than the page asked for.
func TestOneCopyPerStream(t *testing.T) {
	t.Parallel()
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		c := client.New(t, mustNewServer(t, &app.App{}, broker))

		red := c.OpenStream(t, "/room/_$/", map[string]string{"room": "red"})

		postOK(t, c, "/room/say/", `{"room":"red","text":"once"}`)
		require.True(t, red.Saw(`<div id="said">once</div>`),
			"the addressed stream received nothing")

		// Never waits the window out, which lets any second copy land before the
		// count below.
		require.True(t, red.Never(`<div id="said">twice</div>`))

		require.Equal(t, 1, countSeen(red, `<div id="said">once</div>`),
			"the stream received the event more than once")
	})
}

// countSeen is how many of the stream's lines carry sub.
func countSeen(s *client.Stream, sub string) int {
	n := 0
	for _, l := range s.Lines() {
		if strings.Contains(l, sub) {
			n++
		}
	}
	return n
}

// TestSubjectFanout tests the plural form of a subject field.
// One dispatch carrying two values is published to both subjects.
// Both streams see it and a third one does not.
func TestSubjectFanout(t *testing.T) {
	t.Parallel()
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		c := client.New(t, mustNewServer(t, &app.App{}, broker))

		red := c.OpenStream(t, "/room/_$/", map[string]string{"room": "red"})
		blue := c.OpenStream(t, "/room/_$/", map[string]string{"room": "blue"})
		green := c.OpenStream(t, "/room/_$/", map[string]string{"room": "green"})

		postOK(t, c, "/room/broadcast/",
			`{"rooms":["red","blue"],"text":"to both"}`)

		require.True(t, red.Saw(`<div id="broadcast">to both</div>`),
			"the first addressed stream received nothing")
		require.True(t, blue.Saw(`<div id="broadcast">to both</div>`),
			"the second addressed stream received nothing")
		require.True(t, green.Never("to both"),
			"a stream outside the dispatch received the event")
	})
}

// TestStreamOpenRefuses tests a StreamOpen that fails.
//
// The hook is where an application acquires what the stream needs and where it
// decides the stream may not run at all. A failure there must end the request
// rather than leave a connection open that no handler is behind,
// and it must not run StreamClose for a stream that never opened.
func TestStreamOpenRefuses(t *testing.T) {
	t.Parallel()
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		c := client.New(t, mustNewServer(t, &app.App{}, broker))

		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		req, err := http.NewRequestWithContext(
			ctx, http.MethodGet, c.URL()+"/_$/?refuse=1", nil,
		)
		require.NoError(t, err, "building stream request")
		req.Header.Set("Datastar-Request", "true")
		req.Header.Set("Accept-Encoding", "identity")

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err, "GET /_$/?refuse=1")
		defer func() { _ = resp.Body.Close() }()

		// Whatever the status, the response has to end rather than hang:
		// the client is waiting on a stream that will never carry anything.
		done := make(chan struct{})
		go func() {
			defer close(done)
			_, _ = io.Copy(io.Discard, resp.Body)
		}()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("a refused stream was left open")
		}

		if got := logOf(t, c); strings.Contains(got, "open(") {
			t.Errorf("the refused stream was recorded as opened: %s", got)
		}
		if got := logOf(t, c); strings.Contains(got, "close(") {
			t.Errorf("StreamClose ran for a stream that never opened: %s", got)
		}

		// The server keeps serving streams that are not refused.
		s := c.OpenStream(t, "/_$/", nil)
		postOK(t, c, "/tick/", `{"n":9}`)
		require.True(t, s.Saw(`<div id="out">tick 9</div>`),
			"a later stream was not served")
	})
}

// TestStreamRequiresDatastar tests a stream route reached by a plain client.
func TestStreamRequiresDatastar(t *testing.T) {
	t.Parallel()
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		c := client.New(t, mustNewServer(t, &app.App{}, broker))

		resp, err := http.Get(c.URL() + "/_$/")
		require.NoError(t, err, "GET /_$/")
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusNotAcceptable {
			t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusNotAcceptable)
		}
	})
}

// TestMissingSubjectSignal tests a stream that connects without the signal
// its subscription is scoped by. There is nothing to subscribe to,
// and the client is told so rather than served a stream that stays silent.
func TestMissingSubjectSignal(t *testing.T) {
	t.Parallel()
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		c := client.New(t, mustNewServer(t, &app.App{}, broker))

		req, err := http.NewRequestWithContext(context.Background(),
			http.MethodGet, c.URL()+"/room/_$/", nil)
		require.NoError(t, err, "building request")
		req.Header.Set("Datastar-Request", "true")

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err, "GET /room/_$/")
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
		}
	})
}

// TestSubjectSignalEscaped tests the value a client puts in the signal that
// scopes its subscription. Escaped it names one segment,
// which keeps a client sending a wildcard subscribed to that value alone.
func TestSubjectSignalEscaped(t *testing.T) {
	t.Parallel()
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		c := client.New(t, mustNewServer(t, &app.App{}, broker))

		star := c.OpenStream(t, "/room/_$/", map[string]string{"room": "*"})
		red := c.OpenStream(t, "/room/_$/", map[string]string{"room": "red"})

		postOK(t, c, "/room/say/", `{"room":"red","text":"hello red"}`)
		require.True(t, red.Saw(`<div id="said">hello red</div>`),
			"the addressed stream received nothing")
		require.True(t, star.Never("hello red"),
			"a wildcard signal widened the subscription to another room")

		postOK(t, c, "/room/say/", `{"room":"*","text":"hello star"}`)
		require.True(t, star.Saw(`<div id="said">hello star</div>`),
			"the stream scoped to a wildcard value received nothing")
	})
}

// TestEmptySubjectSignalRefused tests the one value escaping cannot save.
// An empty signal names no segment, which leaves a subject no subscription matches.
func TestEmptySubjectSignalRefused(t *testing.T) {
	t.Parallel()
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		c := client.New(t, mustNewServer(t, &app.App{}, broker))

		encoded, err := json.Marshal(map[string]string{"room": ""})
		require.NoError(t, err, "encoding signals")

		req, err := http.NewRequestWithContext(context.Background(),
			http.MethodGet,
			c.URL()+"/room/_$/?datastar="+url.QueryEscape(string(encoded)), nil)
		require.NoError(t, err, "building request")
		req.Header.Set("Datastar-Request", "true")

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err, "GET /room/_$/")
		_ = resp.Body.Close()
		require.Equal(t, http.StatusBadRequest, resp.StatusCode,
			"an empty signal was accepted")
	})
}

// TestBrokerStreamInitialization tests the hand-off to a message broker that
// needs its streams created before anything is published to them.
//
// A broker that implements messaging.StreamInitializer is given every subject
// the app can publish, once, at startup. No broker shipped with datapages needs
// it, the hand-off exists for brokers that must declare a topic or a stream
// before a publish to it can succeed. Nothing on the datapages side reports a
// subject that was left out: the publish simply goes nowhere.
func TestBrokerStreamInitialization(t *testing.T) {
	t.Parallel()
	broker := &initializingBroker{MessageBroker: inmem.New(messaging.DefaultBrokerChanBuffer)}
	_ = client.New(t, mustNewServer(t, &app.App{}, broker))

	if broker.calls != 1 {
		t.Fatalf("InitStreams ran %d times, want once", broker.calls)
	}
	want := datapagesgen.MessageBrokerStreamSubjects()
	if len(want) == 0 {
		t.Fatal("an app with events exports no stream subjects")
	}
	if !slices.Equal(broker.subjects, want) {
		t.Errorf("InitStreams received %v, want %v", broker.subjects, want)
	}

	// The subjects have to cover what the app publishes,
	// or the events that use them have nowhere to go.
	for _, base := range []string{"tick", "room.said", "room.broadcast"} {
		var found bool
		for _, s := range broker.subjects {
			if strings.HasPrefix(s, base) {
				found = true
			}
		}
		if !found {
			t.Errorf("no stream subject covers %q: %v", base, broker.subjects)
		}
	}
}

// TestBrokerInitFailureIsFatal tests a broker that cannot create its streams.
// Starting anyway would mean a server that accepts requests and drops every
// event it publishes.
func TestBrokerInitFailureIsFatal(t *testing.T) {
	t.Parallel()
	broker := &initializingBroker{
		MessageBroker: inmem.New(messaging.DefaultBrokerChanBuffer),
		err:           errors.New("the stream could not be created"),
	}

	s, err := datapages.NewServer[
		app.App,
		datapages.DisableSessions,
		datapages.DisablePrometheus,
		datapagesgen.Server,
	](&app.App{}, broker)
	require.Nil(t, s, "a broker that cannot create its streams was accepted")
	require.ErrorContains(t, err, "initializing message broker streams")
}

// initializingBroker is a broker that needs its streams set up.
// The generated server calls InitStreams on such a broker.
type initializingBroker struct {
	*inmem.MessageBroker
	subjects []string
	calls    int
	err      error
}

func (b *initializingBroker) InitStreams(subjects []string) error {
	b.calls++
	b.subjects = append([]string(nil), subjects...)
	return b.err
}

// TestPageWithoutStream tests a page that handles no events.
// It has no stream route at all, and asking for one is a 404 rather than
// a stream that never carries anything.
func TestPageWithoutStream(t *testing.T) {
	t.Parallel()
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		c := client.New(t, mustNewServer(t, &app.App{}, broker))

		req, err := http.NewRequestWithContext(context.Background(),
			http.MethodGet, c.URL()+"/log/_$/", nil)
		require.NoError(t, err, "building request")
		req.Header.Set("Datastar-Request", "true")

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err, "GET /log/_$/")
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
		}
	})
}

// TestStreamClosePanicDoesNotEndTheProcess tests a StreamClose that panics.
// Unrecovered it ends the process, taking every other stream with it,
// which the requests after the disconnect are here to catch.
func TestStreamClosePanicDoesNotEndTheProcess(t *testing.T) {
	t.Parallel()
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		c := client.New(t, mustNewServer(t, &app.App{}, broker))

		panicking := c.OpenStream(t, "/panic-on-close/_$/", nil)
		survivor := c.OpenStream(t, "/_$/", nil)

		// Disconnecting is what runs StreamClose.
		panicking.Close()

		resp := c.Get(t, "/")
		require.Equal(t, http.StatusOK, resp.Status,
			"the server stopped answering after the panic")

		// The streams the panic did not belong to are still delivered to.
		postOK(t, c, "/tick/", `{"n":7}`)
		require.True(t, survivor.Saw(`<div id="out">tick 7</div>`),
			"an unrelated stream stopped receiving after the panic")
	})
}

// TestStreamClosePanicIsReported tests what the recovery leaves behind.
// A panic nothing reports is one nobody fixes. The hook has to have run.
func TestStreamClosePanicIsReported(t *testing.T) {
	t.Parallel()
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		c := client.New(t, mustNewServer(t, &app.App{}, broker))

		s := c.OpenStream(t, "/panic-on-close/_$/", nil)
		s.Close()

		require.Contains(t, logOf(t, c), "streamclose(",
			"StreamClose never ran")
	})
}

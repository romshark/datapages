// Tests the generated event routing of ./app.

package acceptance_test

import (
	"context"
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
	"github.com/romshark/datapages/datapagestest"
	"github.com/romshark/datapages/internal/acceptance/brokers"
	"github.com/romshark/datapages/internal/acceptance/events/app"
	"github.com/romshark/datapages/internal/acceptance/events/app/datapagesgen"
	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/messaging/inmem"
)

// settle is how long a test lets the server reach a state that no response and no
// event shows, such as a handler held with events queued behind it.
const settle = 200 * time.Millisecond

func serve(t *testing.T, broker messaging.Broker) *datapagestest.Server {
	t.Helper()
	return datapagestest.New(t, mustNewServer(t, &app.App{}, broker))
}

func tick(n int) map[string]int { return map[string]int{"n": n} }

func room(name string) map[string]string { return map[string]string{"room": name} }

func say(room, text string) map[string]string {
	return map[string]string{"room": room, "text": text}
}

// logOf reads what the application recorded while its handlers ran.
func logOf(t *testing.T, v *datapagestest.Visitor) string {
	t.Helper()
	return datapagestest.Text(v.Get(t, "/log/").ElementByID(t, "echo"))
}

// openedID waits for StreamOpen of the one stream the test opened, which runs after
// the response head, and returns the stream id it recorded.
func openedID(t *testing.T, srv *datapagestest.Server, v *datapagestest.Visitor) string {
	t.Helper()
	var opened string
	srv.WaitFor(t, "StreamOpen to run", func() bool {
		opened = logOf(t, v)
		return strings.HasPrefix(opened, "open(")
	})
	id, _, _ := strings.Cut(strings.TrimPrefix(opened, "open("), ")")
	return id
}

// TestPublicEventReachesEveryStream tests an event with no subject field:
// every open stream of the page receives the patch,
// not only the one whose request dispatched it.
func TestPublicEventReachesEveryStream(t *testing.T) {
	t.Parallel()
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		v := serve(t, broker).Visitor(t)

		a := v.Load(t, "/").Connect(t, nil)
		b := v.Load(t, "/").Connect(t, nil)

		v.Load(t, "/").Do(t, http.MethodPost, "/tick/", tick(7)).
			WantStatus(t, http.StatusOK)

		a.WaitFor(t, datapagestest.TextContains("out", "tick 7"),
			"the first stream received no patch")
		b.WaitFor(t, datapagestest.TextContains("out", "tick 7"),
			"the second stream received no patch")
	})
}

// TestStreamHooks tests the 2 hooks that bracket a stream's life.
// Both must run and both must see the same stream id.
// An application uses that id to pair up what it allocated with what it releases.
func TestStreamHooks(t *testing.T) {
	t.Parallel()
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		srv := serve(t, broker)
		v := srv.Visitor(t)

		s := v.Load(t, "/").Connect(t, nil)
		id := openedID(t, srv, v)

		s.Close()

		srv.WaitFor(t, "StreamClose to run", func() bool {
			return strings.Contains(logOf(t, v), "close(")
		})
		require.Equal(t, "open("+id+") close("+id+")", logOf(t, v))
	})
}

// TestStreamCloseRunsAfterTheEventHandlers tests a disconnect with events still queued.
// StreamClose may free what those handlers read.
func TestStreamCloseRunsAfterTheEventHandlers(t *testing.T) {
	t.Parallel()
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		srv := serve(t, broker)
		v := srv.Visitor(t)

		index := v.Load(t, "/")
		s := index.Connect(t, nil)
		id := openedID(t, srv, v)

		// Hold the first tick in its handler and queue 3 behind it.
		// Unheld, the stream drains each tick before the next arrives.
		index.Do(t, http.MethodPost, "/hold/", nil).WantStatus(t, http.StatusOK)
		for n := range 4 {
			index.Do(t, http.MethodPost, "/tick/", tick(n)).WantStatus(t, http.StatusOK)
		}
		time.Sleep(settle)
		s.Close()
		time.Sleep(settle)
		index.Do(t, http.MethodPost, "/release/", nil).WantStatus(t, http.StatusOK)

		closed := "close(" + id + ")"
		srv.WaitFor(t, "StreamClose to run", func() bool {
			return strings.Contains(logOf(t, v), closed)
		})
		// A handler that runs after StreamClose records right after it.
		time.Sleep(settle)
		got := logOf(t, v)
		require.Equal(t, closed, got[strings.LastIndex(got, " ")+1:],
			"a handler of the stream ran after StreamClose:\n%s", got)
	})
}

// TestStreamIDReachesEventHandler tests the stream id an event handler may ask for.
// It must name the stream the handler is writing on, not some other.
func TestStreamIDReachesEventHandler(t *testing.T) {
	t.Parallel()
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		srv := serve(t, broker)
		v := srv.Visitor(t)

		index := v.Load(t, "/")
		s := index.Connect(t, nil)
		id := openedID(t, srv, v)

		index.Do(t, http.MethodPost, "/tick/", tick(3)).WantStatus(t, http.StatusOK)
		// The handler records before it patches.
		s.WaitFor(t, datapagestest.TextContains("out", "tick 3"))

		require.Equal(t, "open("+id+") tick("+id+",3)", logOf(t, v))
	})
}

// TestEmbeddedHandler tests a page that declares no event handler of its own
// and embeds one. The page receives the event through the embedded type.
func TestEmbeddedHandler(t *testing.T) {
	t.Parallel()
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		v := serve(t, broker).Visitor(t)

		other := v.Load(t, "/other/").Connect(t, nil)
		v.Load(t, "/").Do(t, http.MethodPost, "/tick/", tick(1)).
			WantStatus(t, http.StatusOK)

		other.WaitFor(t, datapagestest.TextContains("shared", "shared 1"))
	})
}

// Test2Dispatchers tests an action that declares 1 dispatcher per event type.
// Both events must reach the stream, since each dispatcher publishes on
// its own and neither depends on the other.
func Test2Dispatchers(t *testing.T) {
	t.Parallel()
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		v := serve(t, broker).Visitor(t)

		index := v.Load(t, "/")
		s := index.Connect(t, nil)

		index.Do(t, http.MethodPost, "/both/", tick(3)).WantStatus(t, http.StatusOK)

		// The handler dispatches the tick first, and the broker delivers in
		// publication order.
		s.WaitFor(t, datapagestest.TextContains("out", "tick 3"))
		s.WaitFor(t, datapagestest.TextContains("pong", "pong 3"))
	})
}

// TestShutdownWaitsForStreamClose tests a shutdown while a StreamClose is still running.
// The hook releases leases, presence rows and remote subscriptions:
// a process exiting with it half-done leaves them held.
//
// It serves over ListenAndServe rather than datapagestest:
// only the server's own http.Server tracks the request the hook belongs to.
func TestShutdownWaitsForStreamClose(t *testing.T) {
	t.Parallel()
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		a := &app.App{}
		srv := mustNewServer(t, a, broker)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		served := make(chan error, 1)
		go func() { served <- srv.ListenAndServe(ctx, "127.0.0.1:0") }()

		addr := waitForAddr(t, srv)
		body := openStream(t, "http://"+addr+"/_$/")
		defer func() { _ = body.Close() }()

		a.HoldStreamClose()

		done := make(chan error, 1)
		go func() { done <- srv.Shutdown(context.Background()) }()

		select {
		case err := <-done:
			t.Fatalf("Shutdown returned while StreamClose was running: %v", err)
		case <-time.After(settle):
		}

		a.ReleaseStreamClose()
		require.NoError(t, <-done)
		require.Contains(t, a.Entries(), "close(", "StreamClose never finished")
		require.NoError(t, <-served)
	})
}

// waitForAddr blocks until the server bound its port.
func waitForAddr(t *testing.T, srv datapages.Server) string {
	t.Helper()
	for range 100 {
		if addr := srv.Addr(); addr != "" {
			return addr
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the server never bound a port")
	return ""
}

// openStream opens an SSE stream the way the Datastar runtime does.
func openStream(t *testing.T, url string) io.ReadCloser {
	t.Helper()
	req, err := http.NewRequestWithContext(
		context.Background(), http.MethodGet, url, nil,
	)
	require.NoError(t, err)
	req.Header.Set("Datastar-Request", "true")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	return resp.Body
}

// TestStreamCloseDispatches tests dispatching from StreamClose. The hook runs
// while the closing stream is torn down, which leaves its request context already done.
// The dispatcher must publish with that cancelation stripped.
// The broker here refuses a dead context, which is what makes the difference visible:
// a stream that stays open must receive what the closing one sent.
func TestStreamCloseDispatches(t *testing.T) {
	t.Parallel()
	broker := &ctxBroker{Broker: inmem.New(messaging.DefaultBrokerChanBuffer)}
	v := serve(t, broker).Visitor(t)

	watcher := v.Load(t, "/").Connect(t, nil)
	leaving := v.Load(t, "/").Connect(t, nil)

	leaving.Close()

	watcher.WaitFor(t, datapagestest.TextContains("gone", "gone "))
}

// TestEventDecodeIsNotCarriedOver tests a field the JSON of the second event
// leaves out. The stream must show it empty, not what the first event carried.
func TestEventDecodeIsNotCarriedOver(t *testing.T) {
	t.Parallel()
	v := serve(t, inmem.New(messaging.DefaultBrokerChanBuffer)).Visitor(t)

	index := v.Load(t, "/")
	s := index.Connect(t, nil)

	index.Do(t, http.MethodPost, "/note/", map[string]string{"text": "first"}).
		WantStatus(t, http.StatusOK)
	s.WaitFor(t, datapagestest.TextContains("note", "note=first"))

	index.Do(t, http.MethodPost, "/note/", map[string]string{"text": ""}).
		WantStatus(t, http.StatusOK)
	ev := s.WaitFor(t, datapagestest.Element("note"))
	require.Equal(t, "note=", datapagestest.Text(ev.ElementByID(t, "note")),
		"the empty note kept the text of the one before it")
}

// TestDispatchCtx tests Dispatcher.DispatchCtx.
// The action dispatches with a context that is already done.
// A broker that honors the context refuses to publish and the action fails.
// Dispatch would use the live request context and succeed.
func TestDispatchCtx(t *testing.T) {
	t.Parallel()
	broker := &ctxBroker{Broker: inmem.New(messaging.DefaultBrokerChanBuffer)}
	v := serve(t, broker).Visitor(t)

	index := v.Load(t, "/")
	s := index.Connect(t, nil)

	index.Do(t, http.MethodPost, "/tick/", tick(1)).WantStatus(t, http.StatusOK)
	s.WaitFor(t, datapagestest.TextContains("out", "tick 1"))

	resp := index.Do(t, http.MethodPost, "/canceled/", tick(2))
	require.Equal(t, http.StatusInternalServerError, resp.Status,
		"the canceled context did not reach the broker")
	index.Do(t, http.MethodPost, "/tick/", tick(3)).WantStatus(t, http.StatusOK)
	s.Absent(t,
		datapagestest.TextContains("out", "tick 2"),
		datapagestest.TextContains("out", "tick 3"))
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
// chose when it connected. 2 streams of 1 page, 2 values, 1 dispatch:
// only the addressed stream may see it. The later dispatch to the other value
// is what shows the first one did not reach it.
func TestSubjectScoping(t *testing.T) {
	t.Parallel()
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		v := serve(t, broker).Visitor(t)

		redTab := v.Load(t, "/room/")
		red := redTab.Connect(t, room("red"))
		blue := v.Load(t, "/room/").Connect(t, room("blue"))

		redTab.Do(t, http.MethodPost, "/room/say/", say("red", "hello red")).
			WantStatus(t, http.StatusOK)
		red.WaitFor(t, datapagestest.TextContains("said", "hello red"))

		redTab.Do(t, http.MethodPost, "/room/say/", say("blue", "hello blue")).
			WantStatus(t, http.StatusOK)
		blue.Absent(t,
			datapagestest.TextContains("said", "hello red"),
			datapagestest.TextContains("said", "hello blue"))
	})
}

// TestNestedSignalScoping tests subject fields bound to the signals chat.room
// and chat.thread. Datastar sends them nested, as
// {"chat":{"room":"red","thread":"a"}}, and a stream subscribes by both values.
func TestNestedSignalScoping(t *testing.T) {
	t.Parallel()
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		v := serve(t, broker).Visitor(t)

		type chat struct {
			Room   string `json:"room"`
			Thread string `json:"thread"`
		}
		type said struct {
			Chat chat   `json:"chat"`
			Text string `json:"text"`
		}
		chatA := v.Load(t, "/chat/")
		threadA := chatA.Connect(t, map[string]chat{"chat": {Room: "red", Thread: "a"}})
		threadB := v.Load(t, "/chat/").Connect(t,
			map[string]chat{"chat": {Room: "red", Thread: "b"}})

		chatA.Do(t, http.MethodPost, "/chat/say/",
			said{Chat: chat{Room: "red", Thread: "a"}, Text: "hello a"}).
			WantStatus(t, http.StatusOK)
		threadA.WaitFor(t, datapagestest.TextContains("said", "hello a"))

		chatA.Do(t, http.MethodPost, "/chat/say/",
			said{Chat: chat{Room: "red", Thread: "b"}, Text: "hello b"}).
			WantStatus(t, http.StatusOK)
		threadB.Absent(t,
			datapagestest.TextContains("said", "hello a"),
			datapagestest.TextContains("said", "hello b"))
	})
}

// Test1CopyPerStream tests how many messages a subscription receives.
// A page subscribes to 1 subject per event it handles, and the room page handles 2,
// which a broker that delivers per matching subscription can turn into more copies
// than the page asked for. A second copy would arrive before the next dispatch.
func Test1CopyPerStream(t *testing.T) {
	t.Parallel()
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		v := serve(t, broker).Visitor(t)

		redTab := v.Load(t, "/room/")
		red := redTab.Connect(t, room("red"))

		redTab.Do(t, http.MethodPost, "/room/say/", say("red", "once")).
			WantStatus(t, http.StatusOK)
		red.WaitFor(t, datapagestest.TextContains("said", "once"))

		redTab.Do(t, http.MethodPost, "/room/say/", say("red", "after")).
			WantStatus(t, http.StatusOK)
		red.Absent(t,
			datapagestest.TextContains("said", "once"),
			datapagestest.TextContains("said", "after"))
	})
}

// TestSubjectFanout tests the plural form of a subject field.
// 1 dispatch carrying 2 values is published to both subjects.
// Both streams see it and a third one does not.
func TestSubjectFanout(t *testing.T) {
	t.Parallel()
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		v := serve(t, broker).Visitor(t)

		redTab := v.Load(t, "/room/")
		red := redTab.Connect(t, room("red"))
		blue := v.Load(t, "/room/").Connect(t, room("blue"))
		green := v.Load(t, "/room/").Connect(t, room("green"))

		broadcast := func(text string, rooms ...string) {
			redTab.Do(t, http.MethodPost, "/room/broadcast/", map[string]any{
				"rooms": rooms, "text": text,
			}).WantStatus(t, http.StatusOK)
		}
		broadcast("to both", "red", "blue")
		red.WaitFor(t, datapagestest.TextContains("broadcast", "to both"))
		blue.WaitFor(t, datapagestest.TextContains("broadcast", "to both"))

		broadcast("to green", "green")
		green.Absent(t,
			datapagestest.TextContains("broadcast", "to both"),
			datapagestest.TextContains("broadcast", "to green"))
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
		v := serve(t, broker).Visitor(t)
		index := v.Load(t, "/")

		// Whatever the status, the response has to end rather than hang:
		// the client is waiting on a stream that will never carry anything.
		// Reading it fails the test once the deadline passes.
		ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
		defer cancel()
		v.Send(t, index.NewRequest(t, http.MethodGet, "/_$/?refuse=1", nil).WithContext(ctx))

		if got := logOf(t, v); strings.Contains(got, "open(") {
			t.Errorf("the refused stream was recorded as opened: %s", got)
		}
		if got := logOf(t, v); strings.Contains(got, "close(") {
			t.Errorf("StreamClose ran for a stream that never opened: %s", got)
		}

		// The server keeps serving streams that are not refused.
		s := index.Connect(t, nil)
		index.Do(t, http.MethodPost, "/tick/", tick(9)).WantStatus(t, http.StatusOK)
		s.WaitFor(t, datapagestest.TextContains("out", "tick 9"))
	})
}

// TestStreamOpenPanicRunsNoStreamClose tests a StreamOpen that panics after it
// registered the stream.
//
// A panic in the hook is an error like any other: the stream never opens, so it
// gets no StreamClose. Whatever the hook registered before the panic is the
// application's to drop, under a defer of its own.
func TestStreamOpenPanicRunsNoStreamClose(t *testing.T) {
	t.Parallel()
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		v := serve(t, broker).Visitor(t)
		index := v.Load(t, "/")

		// Reading a stream left open fails the test once the deadline passes.
		ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
		defer cancel()
		v.Send(t, index.NewRequest(t, http.MethodGet, "/_$/?panic=1", nil).WithContext(ctx))

		require.Contains(t, logOf(t, v), "open(",
			"StreamOpen did not run")
		if got := logOf(t, v); strings.Contains(got, "close(") {
			t.Errorf("StreamClose ran for a stream that never opened: %s", got)
		}

		// The panic did not take the server down.
		s := index.Connect(t, nil)
		index.Do(t, http.MethodPost, "/tick/", tick(7)).WantStatus(t, http.StatusOK)
		s.WaitFor(t, datapagestest.TextContains("out", "tick 7"))
	})
}

// TestStreamRequiresDatastar tests a stream route reached by a plain client.
func TestStreamRequiresDatastar(t *testing.T) {
	t.Parallel()
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		v := serve(t, broker).Visitor(t)

		require.Equal(t, http.StatusNotAcceptable, v.Get(t, "/_$/").Status)
	})
}

// TestMissingSubjectSignal tests a stream that connects without the signal
// its subscription is scoped by. There is nothing to subscribe to,
// and the client is told so rather than served a stream that stays silent.
func TestMissingSubjectSignal(t *testing.T) {
	t.Parallel()
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		v := serve(t, broker).Visitor(t)
		roomTab := v.Load(t, "/room/")

		resp := v.Send(t, roomTab.NewRequest(t, http.MethodGet, "/room/_$/", nil))
		require.Equal(t, http.StatusBadRequest, resp.Status)
	})
}

// TestSubjectSignalEscaped tests the value a client puts in the signal that
// scopes its subscription. Escaped it names 1 segment,
// which keeps a client sending a wildcard subscribed to that value alone.
func TestSubjectSignalEscaped(t *testing.T) {
	t.Parallel()
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		v := serve(t, broker).Visitor(t)

		starTab := v.Load(t, "/room/")
		star := starTab.Connect(t, room("*"))
		red := v.Load(t, "/room/").Connect(t, room("red"))

		starTab.Do(t, http.MethodPost, "/room/say/", say("red", "hello red")).
			WantStatus(t, http.StatusOK)
		red.WaitFor(t, datapagestest.TextContains("said", "hello red"))

		// The stream scoped to the wildcard value receives what is published for
		// that value, and nothing of another room before it.
		starTab.Do(t, http.MethodPost, "/room/say/", say("*", "hello star")).
			WantStatus(t, http.StatusOK)
		star.Absent(t,
			datapagestest.TextContains("said", "hello red"),
			datapagestest.TextContains("said", "hello star"))
	})
}

// TestEmptySubjectSignalRefused tests the only value escaping cannot save.
// An empty signal names no segment, which leaves a subject no subscription matches.
func TestEmptySubjectSignalRefused(t *testing.T) {
	t.Parallel()
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		v := serve(t, broker).Visitor(t)
		roomTab := v.Load(t, "/room/")

		target := "/room/_$/?datastar=" + url.QueryEscape(`{"room":""}`)
		resp := v.Send(t, roomTab.NewRequest(t, http.MethodGet, target, nil))
		require.Equal(t, http.StatusBadRequest, resp.Status,
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
	_ = mustNewServer(t, &app.App{}, broker)

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
		v := serve(t, broker).Visitor(t)
		logTab := v.Load(t, "/log/")

		resp := v.Send(t, logTab.NewRequest(t, http.MethodGet, "/log/_$/", nil))
		require.Equal(t, http.StatusNotFound, resp.Status)
	})
}

// TestStreamClosePanicDoesNotEndTheProcess tests a StreamClose that panics.
// Unrecovered it ends the process, taking every other stream with it,
// which the requests after the disconnect are here to catch.
func TestStreamClosePanicDoesNotEndTheProcess(t *testing.T) {
	t.Parallel()
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		srv := serve(t, broker)
		v := srv.Visitor(t)

		panicking := v.Load(t, "/panic-on-close/").Connect(t, nil)
		index := v.Load(t, "/")
		survivor := index.Connect(t, nil)

		// Disconnecting is what runs StreamClose. It records before it panics.
		panicking.Close()
		srv.WaitFor(t, "StreamClose to run", func() bool {
			return strings.Contains(logOf(t, v), "streamclose(")
		})

		resp := v.Get(t, "/")
		require.Equal(t, http.StatusOK, resp.Status,
			"the server stopped answering after the panic")

		// The streams the panic did not belong to are still delivered to.
		index.Do(t, http.MethodPost, "/tick/", tick(7)).WantStatus(t, http.StatusOK)
		survivor.WaitFor(t, datapagestest.TextContains("out", "tick 7"))
	})
}

// TestStreamClosePanicIsReported tests what the recovery leaves behind.
// A panic nothing reports is one nobody fixes. The hook has to have run.
func TestStreamClosePanicIsReported(t *testing.T) {
	t.Parallel()
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		srv := serve(t, broker)
		v := srv.Visitor(t)

		s := v.Load(t, "/panic-on-close/").Connect(t, nil)
		s.Close()

		srv.WaitFor(t, "StreamClose to run", func() bool {
			return strings.Contains(logOf(t, v), "streamclose(")
		})
	})
}

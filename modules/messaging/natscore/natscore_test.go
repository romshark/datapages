package natscore_test

import (
	"context"
	"fmt"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
	natsctr "github.com/testcontainers/testcontainers-go/modules/nats"

	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/messaging/natscore"
)

// testConn is shared by all tests.
var testConn *nats.Conn

func TestMain(m *testing.M) { os.Exit(runSuite(m)) }

func runSuite(m *testing.M) int {
	ctx := context.Background()

	// The module defaults to "-DV -js", the later option wins.
	ctr, err := natsctr.Run(ctx, "nats:latest")
	if err != nil {
		fmt.Fprintf(os.Stderr, "starting NATS container: %v\n", err)
		return 1
	}
	defer func() { _ = ctr.Terminate(ctx) }()

	url, err := ctr.ConnectionString(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "reading NATS connection string: %v\n", err)
		return 1
	}

	// The testcontainers NATS module only waits for the port to be open,
	// not for the server to be fully initialized. Use nats.RetryOnFailedConnect
	// so the client keeps retrying until NATS is ready.
	conn, err := nats.Connect(
		url,
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(50),
		nats.ReconnectWait(200*time.Millisecond),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connecting to NATS: %v\n", err)
		return 1
	}
	defer conn.Close()

	for deadline := time.Now().Add(10 * time.Second); !conn.IsConnected(); {
		if time.Now().After(deadline) {
			fmt.Fprintln(os.Stderr, "NATS never became ready")
			return 1
		}
		time.Sleep(100 * time.Millisecond)
	}

	testConn = conn
	return m.Run()
}

// testMetrics counts the broker instrumentation callbacks.
// NATS runs the delivery callback on its own goroutine, hence the atomics.
type testMetrics struct {
	published atomic.Int64
	dropped   atomic.Int64
}

func (m *testMetrics) OnPublish(string)   { m.published.Add(1) }
func (m *testMetrics) OnDeliveryDropped() { m.dropped.Add(1) }

func subscribe(
	t *testing.T, b *natscore.MessageBroker, m messaging.Metrics, subjects ...string,
) messaging.Subscription {
	t.Helper()
	sub, err := b.Subscribe(context.Background(), m, subjects...)
	require.NoError(t, err)
	t.Cleanup(sub.Close)
	// A subscription only takes effect once the server has processed it.
	require.NoError(t, testConn.Flush())
	return sub
}

func publish(
	t *testing.T, b *natscore.MessageBroker,
	m messaging.Metrics, subject, data string,
) {
	t.Helper()
	require.NoError(t, b.Publish(context.Background(), m, subject, []byte(data)))
	require.NoError(t, testConn.Flush())
}

func receive(
	t *testing.T, sub messaging.Subscription,
) messaging.Message {
	t.Helper()
	select {
	case msg, ok := <-sub.C():
		require.True(t, ok, "the subscription channel was closed")
		return msg
	case <-time.After(3 * time.Second):
		t.Fatal("no message arrived")
		return messaging.Message{}
	}
}

// TestPublishSubscribe covers the delivery path against a server that has no
// JetStream enabled at all.
func TestPublishSubscribe(t *testing.T) {
	b := natscore.New(testConn, natscore.Config{})
	m := new(testMetrics)
	sub := subscribe(t, b, m, "plain.one")

	publish(t, b, m, "plain.one", "payload")

	msg := receive(t, sub)
	require.Equal(t, "plain.one", msg.Subject)
	require.Equal(t, "payload", string(msg.Data))
	require.Equal(t, int64(1), m.published.Load())
	require.Zero(t, m.dropped.Load())
}

// TestFanOut covers two pages subscribed to the same subject.
// Both receive, which is what the SSE fan-out relies on.
func TestFanOut(t *testing.T) {
	b := natscore.New(testConn, natscore.Config{})
	m := new(testMetrics)
	first := subscribe(t, b, m, "fanout.one")
	second := subscribe(t, b, m, "fanout.one")

	publish(t, b, m, "fanout.one", "payload")

	for _, sub := range []messaging.Subscription{first, second} {
		require.Equal(t, "payload", string(receive(t, sub).Data))
	}
}

// TestWildcardDelivery covers what the generated code expects of a broker:
// an event with a subject field and no signal to fill it in subscribes to
// "topic.*" and has to receive every value of it.
func TestWildcardDelivery(t *testing.T) {
	b := natscore.New(testConn, natscore.Config{})
	m := new(testMetrics)
	sub := subscribe(t, b, m, "wildcard.*")

	publish(t, b, m, "wildcard.anything", "payload")

	msg := receive(t, sub)
	require.Equal(t, "wildcard.anything", msg.Subject)
	require.Equal(t, "payload", string(msg.Data))
}

// TestMultipleSubjects covers a page streaming more than one event type.
// One subscription carries all of them.
func TestMultipleSubjects(t *testing.T) {
	b := natscore.New(testConn, natscore.Config{})
	m := new(testMetrics)
	sub := subscribe(t, b, m, "multi.one", "multi.two")

	publish(t, b, m, "multi.one", "first")
	require.Equal(t, "multi.one", receive(t, sub).Subject)

	publish(t, b, m, "multi.two", "second")
	require.Equal(t, "multi.two", receive(t, sub).Subject)
}

// TestUnrelatedSubjectIsNotDelivered covers the negative case next to it.
func TestUnrelatedSubjectIsNotDelivered(t *testing.T) {
	b := natscore.New(testConn, natscore.Config{})
	m := new(testMetrics)
	sub := subscribe(t, b, m, "unrelated.one")

	publish(t, b, m, "unrelated.two", "wrong")
	publish(t, b, m, "unrelated.one", "right")

	require.Equal(t, "unrelated.one", receive(t, sub).Subject)
}

// TestDefaultBrokerChanBuffer covers a broker created without a buffer size.
// Its subscriptions must buffer all the same.
func TestDefaultBrokerChanBuffer(t *testing.T) {
	b := natscore.New(testConn, natscore.Config{})
	m := new(testMetrics)
	sub := subscribe(t, b, m, "buffered.one")

	// Nothing reads the subscription while these are published.
	messages := messaging.DefaultBrokerChanBuffer
	for i := range messages {
		publish(t, b, m, "buffered.one", fmt.Sprintf("payload %d", i))
	}

	require.Eventually(t, func() bool {
		return len(sub.C()) == messages
	}, 3*time.Second, 10*time.Millisecond,
		"a buffered subscription received %d of %d messages",
		len(sub.C()), messages)
	require.Zero(t, m.dropped.Load(), "a buffered subscription dropped messages")

	for i := range messages {
		require.Equal(t, fmt.Sprintf("payload %d", i), string(receive(t, sub).Data))
	}
}

// TestSlowSubscriberDrops covers the bound on the buffer.
// A subscription that is not read must drop instead of backpressuring the publisher.
func TestSlowSubscriberDrops(t *testing.T) {
	b := natscore.New(testConn, natscore.Config{ChanBuffer: 1})
	m := new(testMetrics)
	_ = subscribe(t, b, m, "slow.one")

	for range 5 {
		publish(t, b, m, "slow.one", "payload")
	}

	require.Eventually(t, func() bool {
		return m.dropped.Load() > 0
	}, 3*time.Second, 10*time.Millisecond,
		"an unread subscription dropped nothing")
	require.Equal(t, int64(5), m.published.Load(),
		"a slow subscriber blocked the publisher")
}

// TestNoStreamIsCreated covers what separates this broker from a JetStream one:
// it publishes and delivers without a stream backing the subject.
// The container has JetStream enabled, so an accidental dependency on it would
// pass every other test in this file and only fail on a deployment without it.
func TestNoStreamIsCreated(t *testing.T) {
	b := natscore.New(testConn, natscore.Config{})
	m := new(testMetrics)
	sub := subscribe(t, b, m, "nostream.one")

	publish(t, b, m, "nostream.one", "payload")
	require.Equal(t, "payload", string(receive(t, sub).Data))

	js, err := testConn.JetStream()
	require.NoError(t, err)
	for name := range js.StreamNames() {
		t.Errorf("the broker created stream %q", name)
	}
}

// TestCloseIsIdempotent covers the cleanup path.
// Close closes the channel and a second call is a no-op.
func TestCloseIsIdempotent(t *testing.T) {
	b := natscore.New(testConn, natscore.Config{})
	m := new(testMetrics)
	sub, err := b.Subscribe(context.Background(), m, "closing.one")
	require.NoError(t, err)

	sub.Close()
	sub.Close()

	_, ok := <-sub.C()
	require.False(t, ok, "the subscription channel stayed open after Close")
}

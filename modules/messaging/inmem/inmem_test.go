package inmem_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/messaging/inmem"
)

// noMetrics is a Metrics that records nothing.
type noMetrics struct{}

func (noMetrics) OnPublish(string)   {}
func (noMetrics) OnDeliveryDropped() {}

func TestMatches(t *testing.T) {
	tests := map[string]struct {
		pattern string
		subject string
		want    bool
	}{
		"literal":                   {"a.b", "a.b", true},
		"literal mismatch":          {"a.b", "a.c", false},
		"star matches one token":    {"a.*", "a.b", true},
		"star matches any value":    {"a.*", "a.zzz", true},
		"star is exactly one token": {"a.*", "a.b.c", false},
		"star in the middle":        {"a.*.c", "a.b.c", true},
		"star needs a token":        {"a.*", "a", false},
		"gt matches the rest":       {"a.>", "a.b.c", true},
		"gt matches one token":      {"a.>", "a.b", true},
		"gt needs a token":          {"a.>", "a", false},
		"pattern longer":            {"a.b.c", "a.b", false},
		"subject longer":            {"a.b", "a.b.c", false},
		"star and gt":               {"a.*.>", "a.b.c.d", true},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tt.want, inmem.Matches(tt.pattern, tt.subject))
		})
	}
}

// countingMetrics counts publishes and dropped deliveries.
type countingMetrics struct{ published, dropped int }

func (m *countingMetrics) OnPublish(string)   { m.published++ }
func (m *countingMetrics) OnDeliveryDropped() { m.dropped++ }

// TestPublishIsCountedWithoutSubscribers covers parity with natscore, which
// counts every publish: core NATS knows nothing about subscribers.
func TestPublishIsCountedWithoutSubscribers(t *testing.T) {
	ctx := context.Background()

	for name, subscribeTo := range map[string]string{
		"no subscription at all":          "",
		"subscription on another subject": "other.subject",
		"pattern matching nothing":        "other.*",
	} {
		t.Run(name, func(t *testing.T) {
			b := inmem.New(messaging.DefaultBrokerChanBuffer)
			t.Cleanup(func() { require.NoError(t, b.Close()) })

			metrics := new(countingMetrics)
			if subscribeTo != "" {
				sub, err := b.Subscribe(ctx, metrics, subscribeTo)
				require.NoError(t, err)
				t.Cleanup(sub.Close)
			}

			require.NoError(t, b.Publish(ctx, metrics, "note.one", []byte("x")))

			require.Equal(t, 1, metrics.published,
				"a publish nobody is subscribed to must still be counted")
			require.Zero(t, metrics.dropped,
				"nothing was delivered, so nothing was dropped")
		})
	}
}

// TestPublishIsCountedOnce covers a publish that reaches subscribers:
// it is one publish however many of them there are.
func TestPublishIsCountedOnce(t *testing.T) {
	ctx := context.Background()
	b := inmem.New(messaging.DefaultBrokerChanBuffer)
	t.Cleanup(func() { require.NoError(t, b.Close()) })

	metrics := new(countingMetrics)
	for _, subject := range []string{"note.one", "note.*", "note.>"} {
		sub, err := b.Subscribe(ctx, metrics, subject)
		require.NoError(t, err)
		t.Cleanup(sub.Close)
	}

	require.NoError(t, b.Publish(ctx, metrics, "note.one", []byte("x")))

	require.Equal(t, 1, metrics.published)
	require.Zero(t, metrics.dropped)
}

// TestDefaultBrokerChanBuffer covers a broker created without a buffer size.
// Its subscriptions must buffer all the same.
func TestDefaultBrokerChanBuffer(t *testing.T) {
	b := inmem.New(0)
	t.Cleanup(func() { require.NoError(t, b.Close()) })

	ctx := context.Background()
	metrics := new(countingMetrics)
	sub, err := b.Subscribe(ctx, metrics, "note.one")
	require.NoError(t, err)
	t.Cleanup(sub.Close)

	// Nothing reads the subscription while these are published.
	const messages = 4
	for i := range messages {
		require.NoError(t, b.Publish(ctx, metrics, "note.one", []byte{byte(i)}))
	}
	require.Zero(t, metrics.dropped, "a buffered subscription dropped messages")

	for i := range messages {
		select {
		case msg := <-sub.C():
			require.Equal(t, []byte{byte(i)}, msg.Data)
		case <-time.After(time.Second):
			t.Fatalf("message %d never arrived", i)
		}
	}
}

// TestWildcardDelivery covers what the generated code expects of a broker: an
// event with a subject field and no signal to fill it in subscribes to
// "topic.*" and has to receive every value of it.
func TestWildcardDelivery(t *testing.T) {
	b := inmem.New(messaging.DefaultBrokerChanBuffer)
	t.Cleanup(func() { require.NoError(t, b.Close()) })

	ctx := context.Background()
	sub, err := b.Subscribe(ctx, noMetrics{}, "note.*")
	require.NoError(t, err)
	t.Cleanup(sub.Close)

	require.NoError(t,
		b.Publish(ctx, noMetrics{}, "note.anything", []byte("x")))

	select {
	case msg := <-sub.C():
		require.Equal(t, "note.anything", msg.Subject)
		require.Equal(t, "x", string(msg.Data))
	case <-time.After(time.Second):
		t.Fatal("a subscription to every value of the subject received nothing")
	}
}

// TestLiteralSubscriptionIsUnaffected covers the common case next to it.
func TestLiteralSubscriptionIsUnaffected(t *testing.T) {
	b := inmem.New(messaging.DefaultBrokerChanBuffer)
	t.Cleanup(func() { require.NoError(t, b.Close()) })

	ctx := context.Background()
	sub, err := b.Subscribe(ctx, noMetrics{}, "note.one")
	require.NoError(t, err)
	t.Cleanup(sub.Close)

	require.NoError(t,
		b.Publish(ctx, noMetrics{}, "note.two", []byte("x")))
	require.NoError(t,
		b.Publish(ctx, noMetrics{}, "note.one", []byte("y")))

	select {
	case msg := <-sub.C():
		require.Equal(t, "note.one", msg.Subject, "a subject it did not ask for")
	case <-time.After(time.Second):
		t.Fatal("the literal subscription received nothing")
	}
}

// TestOneMessagePerSubscription covers a subscription whose patterns overlap.
// Two matches are still one message.
func TestOneMessagePerSubscription(t *testing.T) {
	b := inmem.New(messaging.DefaultBrokerChanBuffer)
	t.Cleanup(func() { require.NoError(t, b.Close()) })

	ctx := context.Background()
	sub, err := b.Subscribe(ctx, noMetrics{}, "note.*", "note.>")
	require.NoError(t, err)
	t.Cleanup(sub.Close)

	require.NoError(t,
		b.Publish(ctx, noMetrics{}, "note.one", []byte("x")))

	// Publish delivers into the subscription before it returns,
	// hence whatever the channel holds now is everything it will ever hold.
	// Waiting for a second message would only be waiting.
	<-sub.C()
	select {
	case msg := <-sub.C():
		t.Fatalf("the message arrived twice: %s", msg.Subject)
	default:
	}
}

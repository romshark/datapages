package inmem_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/modules/msgbroker/inmem"
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

// TestWildcardDelivery covers what the generated code expects of a broker: an
// event with a subject field and no signal to fill it in subscribes to
// "topic.*" and has to receive every value of it.
func TestWildcardDelivery(t *testing.T) {
	b := inmem.New(8)
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
	b := inmem.New(8)
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
	b := inmem.New(8)
	t.Cleanup(func() { require.NoError(t, b.Close()) })

	ctx := context.Background()
	sub, err := b.Subscribe(ctx, noMetrics{}, "note.*", "note.>")
	require.NoError(t, err)
	t.Cleanup(sub.Close)

	require.NoError(t,
		b.Publish(ctx, noMetrics{}, "note.one", []byte("x")))

	<-sub.C()
	select {
	case msg := <-sub.C():
		t.Fatalf("the message arrived twice: %s", msg.Subject)
	case <-time.After(100 * time.Millisecond):
	}
}

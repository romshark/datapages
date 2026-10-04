package inmem_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/messaging/inmem"
)

// BenchmarkPublish measures a publish to the open streams of one page.
// Every stream subscribes to the same subjects, as the generated code does,
// and nothing reads them: after the first few publishes, each delivery is a drop.
func BenchmarkPublish(b *testing.B) {
	ctx := context.Background()
	for name, subjects := range map[string][]string{
		"literal":     {"note.one"},
		"wildcard":    {"note.*"},
		"overlapping": {"note.*", "note.>"},
	} {
		for _, streams := range []int{100, 10_000} {
			b.Run(name+"/"+strconv.Itoa(streams), func(b *testing.B) {
				broker := inmem.New(messaging.DefaultBrokerChanBuffer)
				for range streams {
					sub, err := broker.Subscribe(ctx, messaging.NoopMetrics{}, subjects...)
					if err != nil {
						b.Fatal(err)
					}
					b.Cleanup(sub.Close)
				}
				data := []byte("x")
				b.ReportAllocs()
				for b.Loop() {
					if err := broker.Publish(
						ctx, messaging.NoopMetrics{}, "note.one", data,
					); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}

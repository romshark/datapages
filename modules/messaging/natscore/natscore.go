// Package natscore provides a message broker backed by core NATS
// with fan-out delivery semantics.
//
// Delivery is at-most-once: a message reaches only the subscribers that are
// connected when it's published and there's no replay. Datapages events carry
// live UI updates. A lost event means a stale UI until the next render,
// which is why the durability and the ack round trip of JetStream are not worth
// their cost here.
package natscore

import (
	"bytes"
	"context"
	"sync"

	"github.com/nats-io/nats.go"

	"github.com/romshark/datapages/modules/messaging"
)

var _ messaging.Broker = (*MessageBroker)(nil)

type MessageBroker struct {
	nc   *nats.Conn
	conf Config
}

type Config struct {
	// ChanBuffer is how many messages a subscription buffers.
	// Non-positive selects messaging.DefaultBrokerChanBuffer.
	ChanBuffer int
}

type natsSub struct {
	ch    chan messaging.Message
	subs  []*nats.Subscription
	close func()
}

func New(nc *nats.Conn, conf Config) *MessageBroker {
	if conf.ChanBuffer <= 0 {
		conf.ChanBuffer = messaging.DefaultBrokerChanBuffer
	}
	return &MessageBroker{nc: nc, conf: conf}
}

// Publish implements messaging.Broker.
//
// ctx is ignored: a core NATS publish appends to the connection's local write
// buffer and returns, there's no round trip to cancel.
func (b *MessageBroker) Publish(
	_ context.Context,
	metrics messaging.Metrics,
	subject string,
	data []byte,
) error {
	if err := b.nc.Publish(subject, data); err != nil {
		return err
	}
	metrics.OnPublish(subject)
	return nil
}

func (b *MessageBroker) Subscribe(
	_ context.Context, metrics messaging.Metrics, subjects ...string,
) (messaging.Subscription, error) {
	ch := make(chan messaging.Message, b.conf.ChanBuffer)
	subs := make([]*nats.Subscription, 0, len(subjects))

	var (
		lock     sync.Mutex
		closing  bool
		inflight sync.WaitGroup
		once     sync.Once
	)

	closeAll := func() {
		once.Do(func() {
			// After this, no callback can call wg.Add(1).
			lock.Lock()
			closing = true
			lock.Unlock()
			// Stop NATS deliveries.
			for _, s := range subs {
				_ = s.Unsubscribe()
			}
			// Wait until all callbacks that already registered complete.
			inflight.Wait()
			close(ch)
		})
	}

	for _, subject := range subjects {
		sub, err := b.nc.Subscribe(subject, func(m *nats.Msg) {
			// Registration is serialized with closeAll() so Add never races with Wait.
			lock.Lock()
			if closing {
				lock.Unlock()
				return
			}
			// Add must be done under lock to prevent it from racing with wg.Wait.
			// WaitGroup requires that no new Add happens once Wait may be running.
			inflight.Add(1)
			lock.Unlock()

			defer inflight.Done()

			select {
			case ch <- messaging.Message{
				Subject: m.Subject,
				Data:    bytes.Clone(m.Data),
			}:
			default: // drop if subscriber is slow
				metrics.OnDeliveryDropped()
			}
		})
		if err != nil {
			// Undo already-created subscriptions safely (no send-to-closed-ch races).
			closeAll()
			return nil, err
		}
		subs = append(subs, sub)
	}

	ns := &natsSub{
		ch:   ch,
		subs: subs,
	}
	ns.close = closeAll
	return ns, nil
}

func (s *natsSub) C() <-chan messaging.Message {
	return s.ch
}

func (s *natsSub) Close() {
	// closeAll runs under a sync.Once, which is what makes the second call a
	// no-op and lets two goroutines call this at the same time.
	s.close()
}

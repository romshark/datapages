// Package natsjs provides a NATS JetStream  backed message broker
// with fan-out delivery semantics.
package natsjs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/nats-io/nats.go"
	"github.com/romshark/datapages/modules/msgbroker"
)

var _ msgbroker.MessageBroker = (*MessageBroker)(nil)

type MessageBroker struct {
	nc   *nats.Conn
	js   nats.JetStreamContext
	conf Config
}

var ErrNoStreamSubjects = errors.New("missing stream subjects")

type Config struct {
	StreamConfig   *nats.StreamConfig
	StreamSubjects []string
	ChanBuffer     int
}

type natsSub struct {
	ch    chan msgbroker.Message
	subs  []*nats.Subscription
	close func()
}

func New(nc *nats.Conn, conf Config) (*MessageBroker, error) {
	if len(conf.StreamSubjects) == 0 {
		return nil, ErrNoStreamSubjects
	}
	conf.ChanBuffer = min(conf.ChanBuffer, msgbroker.DefaultBrokerChanBuffer)

	js, err := nc.JetStream()
	if err != nil {
		return nil, fmt.Errorf("initializing jetstream: %w", err)
	}

	if conf.StreamConfig == nil {
		conf.StreamConfig = new(nats.StreamConfig)
	}
	if conf.StreamConfig.Description == "" {
		conf.StreamConfig.Description = "stream was automatically created by datapages"
	}
	conf.StreamConfig.Subjects = conf.StreamSubjects

	_, err = js.AddStream(conf.StreamConfig)
	if err != nil && !errors.Is(err, nats.ErrStreamNameAlreadyInUse) {
		return nil, fmt.Errorf("adding stream: %w", err)
	}

	return &MessageBroker{nc: nc, js: js}, nil
}

func (b *MessageBroker) Publish(
	ctx context.Context,
	metrics msgbroker.Metrics,
	subject string,
	data []byte,
) error {
	_, err := b.js.Publish(subject, data, nats.Context(ctx))
	if err == nil {
		metrics.OnPublish(subject)
	}
	return err
}

func (b *MessageBroker) Subscribe(
	_ context.Context, metrics msgbroker.Metrics, subjects ...string,
) (msgbroker.MessageBrokerSubscription, error) {
	ch := make(chan msgbroker.Message, b.conf.ChanBuffer)
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
			case ch <- msgbroker.Message{
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

func (s *natsSub) C() <-chan msgbroker.Message {
	return s.ch
}

func (s *natsSub) Close() {
	if s.close == nil {
		return
	}
	s.close()
	s.close = nil // Prevent double-close
}

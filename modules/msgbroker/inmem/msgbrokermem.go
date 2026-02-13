// Package inmem provides an in-memory message broker with fan-out
// delivery semantics. Slow subscribers are dropped (matching NATS core behavior).
//
// WARNING: Do not use this in multi-instance deployments;
// messages are not shared across process boundaries.
// In production deployments prefer using a networked broker instead
// (e.g. github.com/romshark/datapages/msgbrokernats).
package inmem

import (
	"bytes"
	"context"
	"sync"

	"github.com/romshark/datapages/modules/msgbroker"
)

var _ msgbroker.MessageBroker = (*MessageBroker)(nil)

// MessageBroker is an in-memory message broker.
type MessageBroker struct {
	chanBuffer int
	lock       sync.RWMutex
	subs       map[string]map[*memSub]struct{}
}

type memSub struct {
	ch      chan msgbroker.Message
	topics  []string
	broker  *MessageBroker
	closed  bool
	closeMu sync.Mutex
}

func New(chanBuffer int) *MessageBroker {
	chanBuffer = min(chanBuffer, msgbroker.DefaultBrokerChanBuffer)
	return &MessageBroker{
		chanBuffer: chanBuffer,
		subs:       make(map[string]map[*memSub]struct{}),
	}
}

func (b *MessageBroker) Close() error {
	return nil
}

func (b *MessageBroker) Publish(
	ctx context.Context,
	metrics msgbroker.Metrics,
	subject string,
	data []byte,
) error {
	b.lock.RLock()
	defer b.lock.RUnlock()
	subs := b.subs[subject]

	if len(subs) == 0 {
		return nil
	}

	msg := msgbroker.Message{
		Subject: subject,
		Data:    bytes.Clone(data),
	}
	metrics.OnPublish(subject)

	for sub := range subs {
		select {
		case sub.ch <- msg:
		default: // Drop if subscriber is slow (matches NATS core semantics).
			metrics.OnDeliveryDropped()
		}
	}

	return nil
}

func (b *MessageBroker) Subscribe(
	ctx context.Context, metrics msgbroker.Metrics, subjects ...string,
) (msgbroker.MessageBrokerSubscription, error) {
	sub := &memSub{
		ch:     make(chan msgbroker.Message, b.chanBuffer),
		topics: subjects,
		broker: b,
	}

	b.lock.Lock()
	for _, subject := range subjects {
		m, ok := b.subs[subject]
		if !ok {
			m = make(map[*memSub]struct{})
			b.subs[subject] = m
		}
		m[sub] = struct{}{}
	}
	b.lock.Unlock()

	return sub, nil
}

func (s *memSub) C() <-chan msgbroker.Message {
	return s.ch
}

func (s *memSub) Close() {
	s.closeMu.Lock()
	defer s.closeMu.Unlock()

	if s.closed {
		return
	}
	s.closed = true

	b := s.broker
	b.lock.Lock()
	for _, subject := range s.topics {
		if m, ok := b.subs[subject]; ok {
			delete(m, s)
			if len(m) == 0 {
				delete(b.subs, subject)
			}
		}
	}
	b.lock.Unlock()

	close(s.ch)
}

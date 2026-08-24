// Package inmem provides an in-memory message broker with fan-out delivery semantics.
// Slow subscribers are dropped (matching NATS core behavior).
//
// WARNING: Do not use this in multi-instance deployments;
// messages are not shared across process boundaries.
// In production deployments prefer a networked broker, such as
// [github.com/romshark/datapages/modules/messaging/natscore].
package inmem

import (
	"bytes"
	"context"
	"slices"
	"strings"
	"sync"

	"github.com/romshark/datapages/modules/messaging"
)

var _ messaging.Broker = (*MessageBroker)(nil)

// MessageBroker is an in-memory message broker.
type MessageBroker struct {
	chanBuffer int
	lock       sync.RWMutex
	// subs holds subscriptions by literal subject, which is what most of them are.
	subs map[string]map[*memSub]struct{}
	// wildcards holds the subscriptions whose subject carries "*" or ">".
	// A publish walks these; there are few, and only patterns are here.
	wildcards map[string]map[*memSub]struct{}
}

type memSub struct {
	ch      chan messaging.Message
	topics  []string
	broker  *MessageBroker
	closed  bool
	closeMu sync.Mutex
}

// New creates an in-memory message broker whose subscriptions buffer
// chanBuffer messages each. A non-positive chanBuffer selects
// [github.com/romshark/datapages/modules/messaging.DefaultBrokerChanBuffer].
func New(chanBuffer int) *MessageBroker {
	if chanBuffer <= 0 {
		chanBuffer = messaging.DefaultBrokerChanBuffer
	}
	return &MessageBroker{
		chanBuffer: chanBuffer,
		subs:       make(map[string]map[*memSub]struct{}),
		wildcards:  make(map[string]map[*memSub]struct{}),
	}
}

func (b *MessageBroker) Close() error {
	return nil
}

func (b *MessageBroker) Publish(
	ctx context.Context,
	metrics messaging.Metrics,
	subject string,
	data []byte,
) error {
	// Count every publish to mirror natscore implementation.
	// Core NATS cannot tell whether anyone is listening.
	metrics.OnPublish(subject)

	b.lock.RLock()
	defer b.lock.RUnlock()

	var matched []*memSub
	for sub := range b.subs[subject] {
		matched = append(matched, sub)
	}
	for pattern, subs := range b.wildcards {
		if !Matches(pattern, subject) {
			continue
		}
		for sub := range subs {
			// A subscription may hold several patterns that match the same subject.
			// It receives the message once.
			if !slices.Contains(matched, sub) {
				matched = append(matched, sub)
			}
		}
	}

	if len(matched) == 0 {
		return nil
	}

	msg := messaging.Message{
		Subject: subject,
		Data:    bytes.Clone(data),
	}

	for _, sub := range matched {
		select {
		case sub.ch <- msg:
		default: // Drop if subscriber is slow (matches NATS core semantics).
			metrics.OnDeliveryDropped()
		}
	}

	return nil
}

// Matches reports whether a NATS subject pattern matches a subject.
//
// Tokens are separated by ".". A "*" matches exactly one token and a ">"
// matches every remaining token, of which there must be at least one.
// A pattern with neither is matched literally.
//
// The generated code subscribes with patterns — an event with a subject field
// and no signal to fill it in subscribes to "topic.*" — and expects a broker
// to deliver by them. A broker that matches subjects as map keys drops those
// messages without an error: the publish returns nil and the handler never runs.
func Matches(pattern, subject string) bool {
	if !strings.ContainsAny(pattern, "*>") {
		return pattern == subject
	}
	for {
		p, pRest, pMore := strings.Cut(pattern, ".")
		s, sRest, sMore := strings.Cut(subject, ".")
		switch {
		case p == ">":
			// Matches the rest, which must not be empty.
			return s != ""
		case p != "*" && p != s:
			return false
		case !pMore || !sMore:
			return pMore == sMore
		}
		pattern, subject = pRest, sRest
	}
}

// isPattern reports whether a subject carries a wildcard.
func isPattern(subject string) bool {
	return strings.ContainsAny(subject, "*>")
}

func (b *MessageBroker) Subscribe(
	ctx context.Context, metrics messaging.Metrics, subjects ...string,
) (messaging.Subscription, error) {
	sub := &memSub{
		ch:     make(chan messaging.Message, b.chanBuffer),
		topics: subjects,
		broker: b,
	}

	b.lock.Lock()
	for _, subject := range subjects {
		into := b.subs
		if isPattern(subject) {
			into = b.wildcards
		}
		m, ok := into[subject]
		if !ok {
			m = make(map[*memSub]struct{})
			into[subject] = m
		}
		m[sub] = struct{}{}
	}
	b.lock.Unlock()

	return sub, nil
}

func (s *memSub) C() <-chan messaging.Message {
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
		from := b.subs
		if isPattern(subject) {
			from = b.wildcards
		}
		if m, ok := from[subject]; ok {
			delete(m, s)
			if len(m) == 0 {
				delete(from, subject)
			}
		}
	}
	b.lock.Unlock()

	close(s.ch)
}

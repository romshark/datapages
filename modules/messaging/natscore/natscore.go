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
	"context"
	"sync"

	"github.com/nats-io/nats.go"

	"github.com/romshark/datapages/modules/messaging"
)

var _ messaging.Broker = (*MessageBroker)(nil)

// readLoopBuffer is the least capacity of the channel the nats.go read loop
// delivers a subscription into. The read loop parses up to 32 KiB per read,
// hundreds of small messages, before forward gets to run. It drops a message
// the channel has no room for and prints a slow consumer error.
const readLoopBuffer = 256

type MessageBroker struct {
	nc   *nats.Conn
	conf Config
}

type Config struct {
	// ChanBuffer is the capacity of the channel a subscription delivers on.
	// While it is full, up to ChanBuffer more messages wait, and the
	// subscription drops what arrives beyond that.
	// Non-positive selects messaging.DefaultBrokerChanBuffer.
	ChanBuffer int
}

type natsSub struct {
	ch   chan messaging.Message
	subs []*nats.Subscription
	once sync.Once
	stop chan struct{} // closed by Close
	done chan struct{} // closed by forward after it closed ch

	// natsDropped is how many of the drops nats.go counted on subs forward has reported.
	// Only forward uses it.
	natsDropped int
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

// Subscribe implements messaging.Broker.
//
// Every subject delivers into one channel, which nats.go fills from the read
// loop of the connection in the order the messages arrive. A callback
// subscription per subject would lose that order: nats.go runs the callbacks
// of each subscription on a goroutine of its own.
func (b *MessageBroker) Subscribe(
	_ context.Context, metrics messaging.Metrics, subjects ...string,
) (messaging.Subscription, error) {
	msgs := make(chan *nats.Msg, max(readLoopBuffer, b.conf.ChanBuffer))
	subs := make([]*nats.Subscription, 0, len(subjects))
	for _, subject := range subjects {
		sub, err := b.nc.ChanSubscribe(subject, msgs)
		if err != nil {
			for _, s := range subs {
				_ = s.Unsubscribe()
			}
			return nil, err
		}
		subs = append(subs, sub)
	}

	ns := &natsSub{
		ch:   make(chan messaging.Message, b.conf.ChanBuffer),
		subs: subs,
		stop: make(chan struct{}),
		done: make(chan struct{}),
	}
	go ns.forward(msgs, metrics)
	return ns, nil
}

// forward moves the messages of msgs to the subscription channel in their order
// until Close stops it. It is the only sender on ch, which is why it closes ch.
//
// forward never blocks while msgs holds messages. It keeps up to cap(ch)
// messages in queue, offers the oldest to ch in the same select and drops a
// message only while both are full. Dropping what ch cannot take at once
// would lose most of a burst that the page reads in time.
//
// nats.go gives every message a payload of its own, which forward passes on
// without a copy.
func (s *natsSub) forward(msgs <-chan *nats.Msg, metrics messaging.Metrics) {
	defer close(s.done)
	defer close(s.ch)

	// queue holds n messages from index first on, wrapping around.
	// It has a slot even for an unbuffered ch: select evaluates queue[first] on every
	// pass, also while out is nil, and the slot keeps a message until a reader waits.
	queue := make([]messaging.Message, max(1, cap(s.ch)))
	first, n := 0, 0
	pop := func() {
		queue[first] = messaging.Message{}
		first = (first + 1) % len(queue)
		n--
	}
	for {
		var out chan<- messaging.Message
		if n > 0 {
			out = s.ch
		}
		select {
		case <-s.stop:
			return
		case m := <-msgs:
			if len(msgs) >= cap(msgs)-1 {
				// msgs was full, which is when nats.go drops a message.
				s.reportNATSDrops(metrics)
			}
			if n == len(queue) {
				// select picks a ready case at random. It may have taken from
				// msgs while ch has room for the oldest message.
				select {
				case s.ch <- queue[first]:
					pop()
				default: // drop if subscriber is slow
					metrics.OnDeliveryDropped()
					continue
				}
			}
			queue[(first+n)%len(queue)] = messaging.Message{
				Subject: m.Subject,
				Data:    m.Data,
			}
			n++
		case out <- queue[first]:
			pop()
		}
	}
}

// reportNATSDrops reports the drops nats.go counted on subs since the last call.
// nats.go drops a message when msgs is full and counts it only on the
// subscription the message came in on.
func (s *natsSub) reportNATSDrops(metrics messaging.Metrics) {
	dropped := 0
	for _, sub := range s.subs {
		if n, err := sub.Dropped(); err == nil {
			dropped += n
		}
	}
	for ; s.natsDropped < dropped; s.natsDropped++ {
		metrics.OnDeliveryDropped()
	}
}

func (s *natsSub) C() <-chan messaging.Message {
	return s.ch
}

// Close stops the deliveries and waits for forward to close the channel.
// nats.go never closes msgs: a message it delivers after forward returned
// stays in msgs, which nothing reads.
func (s *natsSub) Close() {
	s.once.Do(func() {
		for _, sub := range s.subs {
			_ = sub.Unsubscribe()
		}
		close(s.stop)
	})
	<-s.done
}

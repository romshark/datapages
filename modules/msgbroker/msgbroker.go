package msgbroker

import "context"

// DefaultBrokerChanBuffer allows to decouple publisher/NATS callback from the consumer.
// Buffer size should be enough to absorb short bursts without blocking delivery,
// while bounding memory and ensuring slow consumers drop messages instead of
// backpressuring producers.
//
// A broker configured with a non-positive buffer size uses it.
var DefaultBrokerChanBuffer = 16

// MessageBroker is a common interface for message brokers.
type MessageBroker interface {
	// Subscribe creates a new subscription to a subject/stream.
	Subscribe(
		ctx context.Context, metrics Metrics, subjects ...string,
	) (MessageBrokerSubscription, error)

	// Publish sends a message to a subject (non-blocking).
	//
	// ctx carries cancelation and a deadline, nothing else.
	// An implementation must not take publish parameters from it.
	// Another implementation ignores what it doesn't know without error,
	// so behavior the application relies on disappears silently.
	// Take such parameters in the implementation's own configuration instead.
	Publish(ctx context.Context, metrics Metrics, subject string, data []byte) error
}

// Metrics receives broker instrumentation callbacks.
type Metrics interface {
	OnPublish(subject string)
	OnDeliveryDropped()
}

// StreamInitializer is an optional interface that message brokers can implement
// to receive the set of stream subjects during server initialization.
// Brokers that require the destination of a message to be declared up front,
// such as a topic or a stream, should implement this. Brokers that route on the
// subject alone, which is what core NATS and the in-memory broker do, can skip it.
type StreamInitializer interface {
	InitStreams(subjects []string) error
}

// MessageBrokerSubscription represents an active message broker subscription.
type MessageBrokerSubscription interface {
	// C returns the channel to receive messages.
	C() <-chan Message

	// Close closes and removes the subscription.
	Close()
}

// Message represents a received message
type Message struct {
	Subject string
	Data    []byte
}

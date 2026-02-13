package msgbroker

import "context"

// DefaultBrokerChanBuffer allows to decouple publisher/NATS callback from the consumer.
// Buffer size should be enough to absorb short bursts without blocking delivery,
// while bounding memory and ensuring slow consumers drop messages instead of
// backpressuring producers.
var DefaultBrokerChanBuffer = 16

// MessageBroker is a common interface for message brokers.
type MessageBroker interface {
	// Subscribe creates a new subscription to a subject/stream.
	Subscribe(
		ctx context.Context, metrics Metrics, subjects ...string,
	) (MessageBrokerSubscription, error)

	// Publish sends a message to a subject (non-blocking)
	Publish(ctx context.Context, metrics Metrics, subject string, data []byte) error
}

// Metrics receives broker instrumentation callbacks.
type Metrics interface {
	OnPublish(subject string)
	OnDeliveryDropped()
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

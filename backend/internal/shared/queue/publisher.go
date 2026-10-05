package queue

import (
	"context"
	"fmt"
)

// EventPublisher defines the contract to dispatch events to message brokers.
type EventPublisher interface {
	Publish(ctx context.Context, exchange string, routingKey string, event interface{}) error
}

// NoOpPublisher is a mock publisher that discards published messages.
type NoOpPublisher struct{}

// NewNoOpPublisher returns a new instance of NoOpPublisher.
func NewNoOpPublisher() EventPublisher {
	return &NoOpPublisher{}
}

// Publish prints log output and returns nil.
func (n *NoOpPublisher) Publish(ctx context.Context, exchange string, routingKey string, event interface{}) error {
	fmt.Printf("[NoOpPublisher] Publishing to Exchange: %s, RoutingKey: %s, Event: %+v\n", exchange, routingKey, event)
	return nil
}

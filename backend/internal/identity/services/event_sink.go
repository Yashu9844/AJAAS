package services

import (
	"context"

	"github.com/jaas/jaas/internal/identity/events"
	"github.com/jaas/jaas/internal/shared/outbox"
	"github.com/jaas/jaas/internal/shared/queue"
	"gorm.io/gorm"
)

// EventSink records an identity event. The production sink writes it to the outbox inside the caller's transaction,
// so the event exists if and only if the change committed (docs/ARCHITECTURE.md §3.7).
type EventSink interface {
	Emit(ctx context.Context, tx *gorm.DB, evt events.Event) error
}

type outboxSink struct{ store outbox.Store }

// NewOutboxSink returns the transactional sink over the identity outbox table.
func NewOutboxSink(store outbox.Store) EventSink { return outboxSink{store: store} }

func (s outboxSink) Emit(ctx context.Context, tx *gorm.DB, evt events.Event) error {
	return s.store.Add(ctx, tx, outbox.Message{TenantID: evt.TenantID, EventID: evt.EventID, Exchange: events.Exchange,
		EventType: evt.EventType, RoutingKey: evt.RoutingKey, Envelope: evt})
}

// publisherSink publishes directly, best effort; used only where no outbox is wired (unit tests, tools).
type publisherSink struct{ pub queue.EventPublisher }

func (s publisherSink) Emit(ctx context.Context, _ *gorm.DB, evt events.Event) error {
	if s.pub != nil {
		_ = s.pub.Publish(ctx, events.Exchange, evt.RoutingKey, evt)
	}
	return nil
}

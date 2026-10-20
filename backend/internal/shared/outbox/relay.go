package outbox

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jaas/jaas/internal/shared/queue"
	"gorm.io/gorm"
)

// Relay defaults: poll often enough that events feel immediate, give up on a row after MaxAttempts.
const (
	DefaultInterval    = 2 * time.Second
	DefaultBatchSize   = 100
	DefaultMaxAttempts = 10
)

// Relay publishes committed outbox rows.
type Relay struct {
	DB          *gorm.DB
	Store       Store
	Publisher   queue.EventPublisher
	Interval    time.Duration
	BatchSize   int
	MaxAttempts int
	Now         func() time.Time
}

func (r *Relay) defaults() {
	if r.Interval <= 0 {
		r.Interval = DefaultInterval
	}
	if r.BatchSize <= 0 {
		r.BatchSize = DefaultBatchSize
	}
	if r.MaxAttempts <= 0 {
		r.MaxAttempts = DefaultMaxAttempts
	}
	if r.Now == nil {
		r.Now = time.Now
	}
}

// RelayOnce publishes one batch and returns how many rows were published.
func (r *Relay) RelayOnce(ctx context.Context) (int, error) {
	r.defaults()
	rows, err := r.Store.Pending(ctx, r.DB, r.BatchSize, r.MaxAttempts)
	if err != nil {
		return 0, err
	}
	published := 0
	for i := range rows {
		row := &rows[i]
		if err := r.Publisher.Publish(ctx, row.Exchange, row.RoutingKey, json.RawMessage(row.Payload)); err != nil {
			_ = r.Store.RecordFailure(ctx, r.DB, row.ID, err.Error())
			continue
		}
		if err := r.Store.MarkPublished(ctx, r.DB, row.ID, r.Now().UTC()); err == nil {
			published++
		}
	}
	return published, nil
}

// Run relays until ctx is cancelled.
func (r *Relay) Run(ctx context.Context) {
	r.defaults()
	ticker := time.NewTicker(r.Interval)
	defer ticker.Stop()
	for {
		_, _ = r.RelayOnce(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

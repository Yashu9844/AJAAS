package services

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jaas/jaas/internal/attendance/events"
)

// Relay tuning (FR-EV002).
const (
	relayInterval    = 30 * time.Second
	relayBatchSize   = 100
	relayMaxAttempts = 10
)

// OutboxRelay republishes outbox rows the request path could not publish (D3-06).
type OutboxRelay struct {
	base
	interval time.Duration
}

// NewOutboxRelay builds a relay over the module dependencies.
func NewOutboxRelay(d Deps) *OutboxRelay { return &OutboxRelay{base: base{d}, interval: relayInterval} }

// RelayOnce publishes one batch and returns how many rows were published.
func (r *OutboxRelay) RelayOnce(ctx context.Context) (int, error) {
	rows, err := r.Repos.Outbox.FetchUnpublished(ctx, r.Tx.DB(), relayBatchSize, relayMaxAttempts)
	if err != nil {
		return 0, err
	}
	published := 0
	for i := range rows {
		row := &rows[i]
		if err := r.Publisher.Publish(ctx, events.Exchange, row.RoutingKey, json.RawMessage(row.Payload)); err != nil {
			_ = r.Repos.Outbox.RecordFailure(ctx, r.Tx.DB(), row.ID, err.Error())
			continue
		}
		if err := r.Repos.Outbox.MarkPublished(ctx, r.Tx.DB(), row.ID, r.Now().UTC()); err == nil {
			published++
		}
	}
	return published, nil
}

// Run ticks RelayOnce until ctx is cancelled (started by cmd/main.go, stopped on shutdown).
func (r *OutboxRelay) Run(ctx context.Context) {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_, _ = r.RelayOnce(ctx)
		}
	}
}

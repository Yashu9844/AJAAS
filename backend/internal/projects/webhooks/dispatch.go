package webhooks

import (
	"context"
	"strconv"
	"time"

	"gorm.io/gorm"
)

// Dispatcher windows. Outbox rows carry the transaction start time as created_at, so a row can become visible after a
// later one; each pass therefore re-reads a short lookback behind its cursor and relies on the unique (webhook, event)
// index to ignore what was already queued.
const (
	dispatchLookback = 30 * time.Second
	dispatchMaxSpan  = 10 * time.Minute
)

// DispatchOnce queues a delivery for every active webhook that matches a new event of its project and returns how many it
// queued. It reads projects_events_outbox through its own cursor (projects_dispatch_cursor), so it does not depend on the
// broker relay's published flag. Events without a project_id are skipped.
func (s *Service) DispatchOnce(ctx context.Context) (int, error) {
	var queued int64
	err := s.inTx(ctx, func(tx *gorm.DB) error {
		var cur struct{ From, To time.Time }
		if err := tx.Raw(`SELECT last_created_at AS "from", LEAST(now(), last_created_at + ?::interval) AS "to"
			FROM projects_dispatch_cursor WHERE id = 1 FOR UPDATE`, intervalLiteral(dispatchMaxSpan)).Scan(&cur).Error; err != nil {
			return err
		}
		n, err := s.queueMatches(tx, cur.From.Add(-dispatchLookback), cur.To)
		if err != nil {
			return err
		}
		queued = n
		return tx.Exec(`UPDATE projects_dispatch_cursor SET last_created_at = GREATEST(last_created_at, ?), updated_at = now() WHERE id = 1`, cur.To).Error
	})
	return int(queued), err
}

func intervalLiteral(d time.Duration) string { return strconv.Itoa(int(d.Seconds())) + " seconds" }

// queueMatches inserts one pending delivery per (matching webhook, event) in the window. Webhooks only see events created
// after they were; "*" matches every event type.
func (s *Service) queueMatches(tx *gorm.DB, from, to time.Time) (int64, error) {
	now := s.stamp()
	res := tx.Exec(`INSERT INTO webhook_deliveries (id, tenant_id, webhook_id, event_id, event_type, status, attempts, next_attempt_at, created_at)
		SELECT gen_random_uuid(), o.tenant_id, w.id, o.event_id, o.routing_key, 'pending', 0, ?::timestamptz, ?::timestamptz
		FROM projects_events_outbox o
		JOIN webhooks w ON w.tenant_id = o.tenant_id AND w.project_id::text = o.payload->'payload'->>'project_id'
		WHERE o.created_at >= ? AND o.created_at <= ? AND o.created_at >= w.created_at AND w.active AND w.disabled_at IS NULL
		  AND (jsonb_exists(w.event_types, '*') OR jsonb_exists(w.event_types, o.routing_key))
		ON CONFLICT (webhook_id, event_id) DO NOTHING`, now, now, from, to)
	return res.RowsAffected, res.Error
}

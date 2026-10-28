package outbox

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"time"

	"github.com/jaas/jaas/internal/shared/queue"
	"gorm.io/gorm"
)

// TableRelay defaults, the same as the older per-context relays.
const (
	TableRelayInterval    = 30 * time.Second
	TableRelayBatchSize   = 100
	TableRelayMaxAttempts = 10
)

var tableName = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// TableRelay publishes the rows of one context's `<prefix>_events_outbox` table (id, routing_key, payload, published,
// published_at, attempts, last_error) to the message bus (HR-009). Rows are claimed with FOR UPDATE SKIP LOCKED, so
// several instances can run side by side; a row that fails TableRelayMaxAttempts times is left for an operator.
type TableRelay struct {
	db        *gorm.DB
	table     string
	exchange  string
	publisher queue.EventPublisher
	interval  time.Duration
	now       func() time.Time
}

// NewTableRelay builds a relay for table (a trusted constant, validated) publishing to exchange.
func NewTableRelay(db *gorm.DB, table, exchange string, publisher queue.EventPublisher) *TableRelay {
	if !tableName.MatchString(table) {
		panic(fmt.Sprintf("kernel: invalid outbox table name %q", table))
	}
	return &TableRelay{db: db, table: table, exchange: exchange, publisher: publisher, interval: TableRelayInterval, now: time.Now}
}

type outboxRow struct {
	ID         string
	RoutingKey string
	Payload    []byte
}

// RelayOnce publishes one batch and returns how many rows were published.
func (r *TableRelay) RelayOnce(ctx context.Context) (int, error) {
	published := 0
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var rows []outboxRow
		err := tx.Raw(`SELECT id::text AS id, routing_key, payload::text AS payload FROM `+r.table+
			` WHERE published = FALSE AND attempts < ? ORDER BY created_at LIMIT ? FOR UPDATE SKIP LOCKED`, TableRelayMaxAttempts, TableRelayBatchSize).Scan(&rows).Error
		if err != nil {
			return err
		}
		for _, row := range rows {
			if perr := r.publisher.Publish(ctx, r.exchange, row.RoutingKey, json.RawMessage(row.Payload)); perr != nil {
				if err := tx.Exec(`UPDATE `+r.table+` SET attempts = attempts + 1, last_error = ? WHERE id = ?::uuid`, perr.Error(), row.ID).Error; err != nil {
					return err
				}
				continue
			}
			if err := tx.Exec(`UPDATE `+r.table+` SET published = TRUE, published_at = ?, attempts = attempts + 1, last_error = NULL WHERE id = ?::uuid`,
				r.now().UTC(), row.ID).Error; err != nil {
				return err
			}
			published++
		}
		return nil
	})
	return published, err
}

// Run ticks RelayOnce until ctx is cancelled. A failed tick is reported through onErr (may be nil) and retried.
func (r *TableRelay) Run(ctx context.Context, onErr func(error)) {
	t := time.NewTicker(r.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if _, err := r.RelayOnce(ctx); err != nil && onErr != nil {
				onErr(err)
			}
		}
	}
}

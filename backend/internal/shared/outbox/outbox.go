// Package outbox is the transactional outbox shared by every service (docs/ARCHITECTURE.md §3.7): an event row is
// written in the same transaction as the business change and a relay publishes it after commit, retrying while the
// broker is unavailable. Delivery is at least once; consumers deduplicate by event_id.
package outbox

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Row is one stored event. Payload is the full JSON envelope sent to the broker.
type Row struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey"`
	TenantID    uuid.UUID `gorm:"type:uuid;not null"`
	EventID     uuid.UUID `gorm:"type:uuid;not null"`
	Exchange    string    `gorm:"type:varchar(100);not null"`
	EventType   string    `gorm:"type:varchar(100);not null"`
	RoutingKey  string    `gorm:"type:varchar(100);not null"`
	Payload     string    `gorm:"type:jsonb;not null"`
	Published   bool      `gorm:"not null"`
	PublishedAt *time.Time
	Attempts    int     `gorm:"not null"`
	LastError   *string `gorm:"type:text"`
	CreatedAt   time.Time
}

// Message is what a service emits.
type Message struct {
	TenantID   uuid.UUID
	EventID    uuid.UUID
	Exchange   string
	EventType  string
	RoutingKey string
	Envelope   interface{} // marshalled to JSON as the broker payload
}

// Store reads and writes one outbox table.
type Store interface {
	Add(ctx context.Context, tx *gorm.DB, m Message) error
	Pending(ctx context.Context, db *gorm.DB, limit, maxAttempts int) ([]Row, error)
	MarkPublished(ctx context.Context, db *gorm.DB, id uuid.UUID, at time.Time) error
	RecordFailure(ctx context.Context, db *gorm.DB, id uuid.UUID, reason string) error
}

type store struct{ table string }

// NewStore returns a Store over the given table (each service owns its own outbox table).
func NewStore(table string) Store { return store{table: table} }

func (s store) Add(ctx context.Context, tx *gorm.DB, m Message) error {
	raw, err := json.Marshal(m.Envelope)
	if err != nil {
		return err
	}
	row := Row{ID: uuid.New(), TenantID: m.TenantID, EventID: m.EventID, Exchange: m.Exchange, EventType: m.EventType,
		RoutingKey: m.RoutingKey, Payload: string(raw), CreatedAt: time.Now().UTC()}
	return tx.WithContext(ctx).Table(s.table).Create(&row).Error
}

func (s store) Pending(ctx context.Context, db *gorm.DB, limit, maxAttempts int) ([]Row, error) {
	var rows []Row
	err := db.WithContext(ctx).Table(s.table).Where("published = ? AND attempts < ?", false, maxAttempts).
		Order("created_at").Limit(limit).Find(&rows).Error
	return rows, err
}

func (s store) MarkPublished(ctx context.Context, db *gorm.DB, id uuid.UUID, at time.Time) error {
	return db.WithContext(ctx).Table(s.table).Where("id = ?", id).
		Updates(map[string]interface{}{"published": true, "published_at": at, "attempts": gorm.Expr("attempts + 1")}).Error
}

func (s store) RecordFailure(ctx context.Context, db *gorm.DB, id uuid.UUID, reason string) error {
	if len(reason) > 500 {
		reason = reason[:500]
	}
	return db.WithContext(ctx).Table(s.table).Where("id = ?", id).
		Updates(map[string]interface{}{"attempts": gorm.Expr("attempts + 1"), "last_error": reason}).Error
}

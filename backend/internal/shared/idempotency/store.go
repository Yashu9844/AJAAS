// Package idempotency makes retried commands safe (docs/ARCHITECTURE.md §3.6): a client sends Idempotency-Key with
// a write; the first request claims the key, later requests with the same key and payload replay the stored outcome,
// and a reused key with a different payload is rejected. The claim is a database unique constraint, never Redis.
package idempotency

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Record states.
const (
	StatusProcessing = "processing"
	StatusCompleted  = "completed"
)

// Record is one claimed key.
type Record struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey"`
	Scope          string    `gorm:"type:varchar(200);not null;uniqueIndex:uq_idempotency_key,priority:1"`
	Method         string    `gorm:"type:varchar(10);not null;uniqueIndex:uq_idempotency_key,priority:2"`
	Path           string    `gorm:"type:varchar(500);not null;uniqueIndex:uq_idempotency_key,priority:3"`
	Key            string    `gorm:"type:varchar(255);not null;uniqueIndex:uq_idempotency_key,priority:4"`
	RequestHash    string    `gorm:"type:char(64);not null"`
	Status         string    `gorm:"type:varchar(20);not null"`
	ResponseStatus int       `gorm:"type:integer;not null;default:0"`
	ResponseBody   []byte    `gorm:"type:bytea"`
	ContentType    string    `gorm:"type:varchar(100);not null;default:''"`
	CreatedAt      time.Time `gorm:"not null"`
	ExpiresAt      time.Time `gorm:"not null;index:idx_idempotency_expires"`
}

// TableName maps to idempotency_records (migration 000060).
func (Record) TableName() string { return "idempotency_records" }

// Store persists claims and outcomes.
type Store interface {
	// Claim inserts r; when the key already exists it returns the existing record and claimed=false.
	Claim(ctx context.Context, r *Record) (existing *Record, claimed bool, err error)
	Complete(ctx context.Context, id uuid.UUID, status int, body []byte, contentType string) error
	Release(ctx context.Context, id uuid.UUID) error
	PurgeExpired(ctx context.Context, now time.Time) (int64, error)
}

type gormStore struct{ db *gorm.DB }

// NewStore returns the database-backed store.
func NewStore(db *gorm.DB) Store { return gormStore{db: db} }

func (s gormStore) Claim(ctx context.Context, r *Record) (*Record, bool, error) {
	for attempt := 0; attempt < 3; attempt++ {
		res := s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(r)
		if res.Error != nil {
			return nil, false, res.Error
		}
		if res.RowsAffected == 1 {
			return nil, true, nil
		}
		var existing Record
		err := s.db.WithContext(ctx).Where("scope = ? AND method = ? AND path = ? AND key = ?", r.Scope, r.Method, r.Path, r.Key).
			First(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) { // released between the insert and the read: claim again
			continue
		}
		return &existing, false, err
	}
	return nil, false, errors.New("idempotency key could not be claimed")
}

func (s gormStore) Complete(ctx context.Context, id uuid.UUID, status int, body []byte, contentType string) error {
	return s.db.WithContext(ctx).Model(&Record{}).Where("id = ?", id).Updates(map[string]interface{}{
		"status": StatusCompleted, "response_status": status, "response_body": body, "content_type": contentType}).Error
}

func (s gormStore) Release(ctx context.Context, id uuid.UUID) error {
	return s.db.WithContext(ctx).Delete(&Record{}, "id = ?", id).Error
}

func (s gormStore) PurgeExpired(ctx context.Context, now time.Time) (int64, error) {
	res := s.db.WithContext(ctx).Where("expires_at < ?", now).Delete(&Record{})
	return res.RowsAffected, res.Error
}

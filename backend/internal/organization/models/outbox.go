package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// OrgEventOutbox buffers produced events when the broker is down; a retry worker
// republishes them later. Guarantees queue downtime never blocks the API (FR-E005).
type OrgEventOutbox struct {
	ID            uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	TenantID      uuid.UUID  `gorm:"type:uuid;not null;index:idx_org_outbox_tenant_id" json:"tenant_id"`
	EventType     string     `gorm:"type:varchar(100);not null" json:"event_type"`
	RoutingKey    string     `gorm:"type:varchar(200);not null" json:"routing_key"`
	Payload       string     `gorm:"type:jsonb;not null" json:"payload"`
	CorrelationID *uuid.UUID `gorm:"type:uuid" json:"correlation_id,omitempty"`
	Attempts      int        `gorm:"not null;default:0" json:"attempts"`
	NextRetryAt   *time.Time `gorm:"index:idx_org_outbox_next_retry" json:"next_retry_at,omitempty"`
	CreatedAt     time.Time  `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
}

// TableName maps to the org_events_outbox table (migration 000017).
func (OrgEventOutbox) TableName() string { return "org_events_outbox" }

// BeforeCreate generates a UUID v4 key when unset.
func (o *OrgEventOutbox) BeforeCreate(tx *gorm.DB) error {
	if o.ID == uuid.Nil {
		o.ID = uuid.New()
	}
	return nil
}

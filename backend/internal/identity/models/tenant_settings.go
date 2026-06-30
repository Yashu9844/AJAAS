package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// TenantSettings represents a key-value configuration scoped to a tenant.
type TenantSettings struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	TenantID  uuid.UUID `gorm:"type:uuid;not null;index:idx_tenant_settings_tenant_id;uniqueIndex:uq_tenant_settings_tenant_key,priority:1" json:"tenant_id"`
	Key       string    `gorm:"type:varchar(255);not null;uniqueIndex:uq_tenant_settings_tenant_key,priority:2" json:"key"`
	Value     string    `gorm:"type:text;not null" json:"value"`
	CreatedAt time.Time `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt time.Time `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
}

// BeforeCreate is a GORM hook that auto-generates a UUID v4 key if not already set.
func (t *TenantSettings) BeforeCreate(tx *gorm.DB) error {
	if t.ID == uuid.Nil {
		t.ID = uuid.New()
	}
	return nil
}

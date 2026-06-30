package database

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// BaseModel defines the common columns for persistent entities using UUID keys.
type BaseModel struct {
	ID        uuid.UUID      `gorm:"type:uuid;primaryKey" json:"id"`
	CreatedAt time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
}

// TenantBaseModel extends BaseModel by adding a tenant scoping ID.
type TenantBaseModel struct {
	BaseModel
	TenantID uuid.UUID `gorm:"type:uuid;not null;index" json:"tenant_id"`
}

// BeforeCreate is a GORM hook that auto-generates a UUID v4 key if not already set.
func (b *BaseModel) BeforeCreate(tx *gorm.DB) error {
	if b.ID == uuid.Nil {
		b.ID = uuid.New()
	}
	return nil
}

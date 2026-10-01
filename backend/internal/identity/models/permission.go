package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Permission represents a global capability definition.
type Permission struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	Resource    string    `gorm:"type:varchar(100);not null;uniqueIndex:uq_permissions_resource_action,priority:1;index:idx_permissions_resource" json:"resource"`
	Action      string    `gorm:"type:varchar(50);not null;uniqueIndex:uq_permissions_resource_action,priority:2" json:"action"`
	Description *string   `gorm:"type:varchar(500)" json:"description,omitempty"`
	CreatedAt   time.Time `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt   time.Time `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
}

// BeforeCreate is a GORM hook that auto-generates a UUID v4 key if not already set.
func (p *Permission) BeforeCreate(tx *gorm.DB) error {
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	return nil
}

package models

import (
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/shared/database"
	"gorm.io/gorm"
)

// Designation represents a job title/level scoped to a tenant. FR-DG001..FR-DG004.
type Designation struct {
	database.BaseModel
	TenantID    uuid.UUID `gorm:"type:uuid;not null;index:idx_designations_tenant_id;uniqueIndex:uq_designations_tenant_title,priority:1;uniqueIndex:uq_designations_tenant_code,priority:1" json:"tenant_id"`
	Title       string    `gorm:"type:varchar(100);not null;uniqueIndex:uq_designations_tenant_title,priority:2" json:"title"`
	Code        *string   `gorm:"type:varchar(32);uniqueIndex:uq_designations_tenant_code,priority:2" json:"code,omitempty"`
	Level       *int      `gorm:"index:idx_designations_level" json:"level,omitempty"`
	Description *string   `gorm:"type:varchar(500)" json:"description,omitempty"`
	Status      string    `gorm:"type:varchar(20);not null;default:'active';index:idx_designations_status" json:"status"`
}

// BeforeCreate generates a UUID v4 key when unset.
func (d *Designation) BeforeCreate(tx *gorm.DB) error {
	if d.ID == uuid.Nil {
		d.ID = uuid.New()
	}
	return nil
}

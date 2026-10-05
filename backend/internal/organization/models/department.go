package models

import (
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/shared/database"
	"gorm.io/gorm"
)

// Department represents a company department scoped to a tenant. FR-D001..FR-D008.
type Department struct {
	database.BaseModel
	TenantID           uuid.UUID  `gorm:"type:uuid;not null;index:idx_departments_tenant_id;uniqueIndex:uq_departments_tenant_name,priority:1" json:"tenant_id"`
	Name               string     `gorm:"type:varchar(100);not null;uniqueIndex:uq_departments_tenant_name,priority:2" json:"name"`
	Code               *string    `gorm:"type:varchar(32);uniqueIndex:uq_departments_tenant_code" json:"code,omitempty"`
	Description        *string    `gorm:"type:varchar(500)" json:"description,omitempty"`
	ParentDepartmentID *uuid.UUID `gorm:"type:uuid;index:idx_departments_parent_id" json:"parent_department_id,omitempty"`
	Status             string     `gorm:"type:varchar(20);not null;default:'active';index:idx_departments_status" json:"status"`

	// Relationships
	Children []Department `gorm:"foreignKey:ParentDepartmentID" json:"children,omitempty"`
	Teams    []Team       `gorm:"foreignKey:DepartmentID" json:"teams,omitempty"`
}

// BeforeCreate generates a UUID v4 key when unset.
func (d *Department) BeforeCreate(tx *gorm.DB) error {
	if d.ID == uuid.Nil {
		d.ID = uuid.New()
	}
	return nil
}

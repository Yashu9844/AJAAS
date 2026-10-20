package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/shared/database"
	"gorm.io/gorm"
)

// Mapping links a user to org structure. Append-only: deactivate, never hard delete. FR-M001..FR-M007.
type Mapping struct {
	database.BaseModel
	TenantID      uuid.UUID  `gorm:"type:uuid;not null;index:idx_mappings_tenant_id;index:idx_mappings_tenant_user,priority:1" json:"tenant_id"`
	UserID        uuid.UUID  `gorm:"type:uuid;not null;index:idx_mappings_user_id;index:idx_mappings_tenant_user,priority:2" json:"user_id"`
	DepartmentID  *uuid.UUID `gorm:"type:uuid;index:idx_mappings_department_id" json:"department_id,omitempty"`
	TeamID        *uuid.UUID `gorm:"type:uuid;index:idx_mappings_team_id" json:"team_id,omitempty"`
	DesignationID *uuid.UUID `gorm:"type:uuid;index:idx_mappings_designation_id" json:"designation_id,omitempty"`
	IsPrimary     bool       `gorm:"not null;default:false;index:idx_mappings_primary" json:"is_primary"`
	ManagerUserID *uuid.UUID `gorm:"type:uuid;index:idx_mappings_manager_user_id" json:"manager_user_id,omitempty"`
	Status        string     `gorm:"type:varchar(20);not null;default:'active';index:idx_mappings_status" json:"status"`
	Reason        *string    `gorm:"type:varchar(500)" json:"reason,omitempty"`

	// Relationships
	Department  *Department  `gorm:"foreignKey:DepartmentID" json:"department,omitempty"`
	Team        *Team        `gorm:"foreignKey:TeamID" json:"team,omitempty"`
	Designation *Designation `gorm:"foreignKey:DesignationID" json:"designation,omitempty"`
}

// BeforeCreate generates a UUID v4 key when unset.
func (m *Mapping) BeforeCreate(tx *gorm.DB) error {
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}
	if m.Status == "" {
		m.Status = "active"
	}
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now()
		m.UpdatedAt = m.CreatedAt
	}
	return nil
}

package models

import (
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/shared/database"
	"gorm.io/gorm"
)

// Team represents a team inside a department, scoped to a tenant. FR-T001..FR-T007.
type Team struct {
	database.BaseModel
	TenantID     uuid.UUID  `gorm:"type:uuid;not null;index:idx_teams_tenant_id;uniqueIndex:uq_teams_tenant_code,priority:1" json:"tenant_id"`
	DepartmentID uuid.UUID  `gorm:"type:uuid;not null;index:idx_teams_department_id;uniqueIndex:uq_teams_dept_name,priority:1" json:"department_id"`
	Name         string     `gorm:"type:varchar(100);not null;uniqueIndex:uq_teams_dept_name,priority:2" json:"name"`
	Code         *string    `gorm:"type:varchar(32);uniqueIndex:uq_teams_tenant_code" json:"code,omitempty"`
	Description  *string    `gorm:"type:varchar(500)" json:"description,omitempty"`
	LeadUserID   *uuid.UUID `gorm:"type:uuid;index:idx_teams_lead_user_id" json:"lead_user_id,omitempty"`
	Status       string     `gorm:"type:varchar(20);not null;default:'active';index:idx_teams_status" json:"status"`
}

// BeforeCreate generates a UUID v4 key when unset.
func (t *Team) BeforeCreate(tx *gorm.DB) error {
	if t.ID == uuid.Nil {
		t.ID = uuid.New()
	}
	return nil
}

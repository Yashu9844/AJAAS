package models

import (
	"time"

	"github.com/google/uuid"
)

// ResourceRoleAssignment gives a user a role on one resource of another service (today a project). The resource has no
// foreign key: it belongs to another service schema. A user holds at most one role per resource.
type ResourceRoleAssignment struct {
	ID           uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	TenantID     uuid.UUID  `gorm:"type:uuid;not null" json:"-"`
	UserID       uuid.UUID  `gorm:"type:uuid;not null" json:"user_id"`
	RoleID       uuid.UUID  `gorm:"type:uuid;not null" json:"role_id"`
	ResourceType string     `gorm:"type:varchar(20);not null" json:"resource_type"`
	ResourceID   uuid.UUID  `gorm:"type:uuid;not null" json:"resource_id"`
	Source       string     `gorm:"type:varchar(20);not null;default:manual" json:"source"`
	AssignedBy   *uuid.UUID `gorm:"type:uuid" json:"assigned_by,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	Role         *Role      `gorm:"foreignKey:RoleID" json:"role,omitempty"`
}

// TableName maps to resource_role_assignments.
func (ResourceRoleAssignment) TableName() string { return "resource_role_assignments" }

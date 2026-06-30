package models

import (
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/shared/database"
)

// Role represents a Role-Based Access Control role scoped to a tenant.
type Role struct {
	database.BaseModel
	TenantID    uuid.UUID `gorm:"type:uuid;not null;index:idx_roles_tenant_id;uniqueIndex:uq_roles_tenant_name,priority:1" json:"tenant_id"`
	Name        string    `gorm:"type:varchar(100);not null;uniqueIndex:uq_roles_tenant_name,priority:2" json:"name"`
	Description *string   `gorm:"type:varchar(500)" json:"description,omitempty"`
	IsSystem    bool      `gorm:"not null;default:false" json:"is_system"`

	// Relationships
	Permissions []RolePermission `gorm:"foreignKey:RoleID" json:"permissions,omitempty"`
	Users       []UserRole       `gorm:"foreignKey:RoleID" json:"users,omitempty"`
}

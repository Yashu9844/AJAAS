package models

import (
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/shared/database"
)

// Role represents a Role-Based Access Control role scoped to a tenant.
type Role struct {
	database.BaseModel
	TenantID    uuid.UUID `gorm:"type:uuid;not null;index:idx_roles_tenant_id;uniqueIndex:uq_roles_tenant_name,priority:1;uniqueIndex:uq_roles_tenant_template_key,priority:1" json:"tenant_id"`
	Name        string    `gorm:"type:varchar(100);not null;uniqueIndex:uq_roles_tenant_name,priority:2" json:"name"`
	Description *string   `gorm:"type:varchar(500)" json:"description,omitempty"`
	IsSystem    bool      `gorm:"not null;default:false" json:"is_system"`

	// Access model (docs/ARCHITECTURE.md §17): inheritance and predefined-role bookkeeping.
	ParentRoleID    *uuid.UUID `gorm:"type:uuid;index:idx_roles_parent_role_id" json:"parent_role_id,omitempty"`
	TemplateKey     *string    `gorm:"type:varchar(50);uniqueIndex:uq_roles_tenant_template_key,priority:2,where:template_key IS NOT NULL AND deleted_at IS NULL" json:"template_key,omitempty"`
	IsAutomatic     bool       `gorm:"not null;default:false" json:"is_automatic"`
	TemplateManaged bool       `gorm:"not null;default:false" json:"template_managed"`

	// Relationships
	Permissions []RolePermission `gorm:"foreignKey:RoleID" json:"permissions,omitempty"`
	Users       []UserRole       `gorm:"foreignKey:RoleID" json:"users,omitempty"`
}

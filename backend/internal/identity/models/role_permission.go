package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// RolePermission represents the association of permissions to roles scoped to a tenant.
type RolePermission struct {
	ID           uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	RoleID       uuid.UUID `gorm:"type:uuid;not null;index:idx_role_permissions_role_id;uniqueIndex:uq_role_permissions_role_perm_tenant,priority:1" json:"role_id"`
	PermissionID uuid.UUID `gorm:"type:uuid;not null;index:idx_role_permissions_permission_id;uniqueIndex:uq_role_permissions_role_perm_tenant,priority:2" json:"permission_id"`
	TenantID     uuid.UUID `gorm:"type:uuid;not null;index:idx_role_permissions_tenant_id;uniqueIndex:uq_role_permissions_role_perm_tenant,priority:3" json:"tenant_id"`
	CreatedAt    time.Time `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt    time.Time `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`

	// Associations for preloading
	Role       *Role       `gorm:"foreignKey:RoleID" json:"role,omitempty"`
	Permission *Permission `gorm:"foreignKey:PermissionID" json:"permission,omitempty"`
}

// BeforeCreate is a GORM hook that auto-generates a UUID v4 key if not already set.
func (rp *RolePermission) BeforeCreate(tx *gorm.DB) error {
	if rp.ID == uuid.Nil {
		rp.ID = uuid.New()
	}
	return nil
}

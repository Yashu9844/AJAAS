package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// UserRole represents a many-to-many relationship between users and roles scoped to a tenant.
type UserRole struct {
	ID         uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	UserID     uuid.UUID  `gorm:"type:uuid;not null;index:idx_user_roles_user_id;uniqueIndex:uq_user_roles_user_role_tenant,priority:1" json:"user_id"`
	RoleID     uuid.UUID  `gorm:"type:uuid;not null;index:idx_user_roles_role_id;uniqueIndex:uq_user_roles_user_role_tenant,priority:2" json:"role_id"`
	TenantID   uuid.UUID  `gorm:"type:uuid;not null;index:idx_user_roles_tenant_id;uniqueIndex:uq_user_roles_user_role_tenant,priority:3" json:"tenant_id"`
	AssignedAt time.Time  `gorm:"not null;default:CURRENT_TIMESTAMP" json:"assigned_at"`
	AssignedBy *uuid.UUID `gorm:"type:uuid" json:"assigned_by,omitempty"`
	CreatedAt  time.Time  `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt  time.Time  `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`

	// Associations for preloading
	User *User `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Role *Role `gorm:"foreignKey:RoleID" json:"role,omitempty"`
}

// BeforeCreate is a GORM hook that auto-generates a UUID v4 key if not already set.
func (ur *UserRole) BeforeCreate(tx *gorm.DB) error {
	if ur.ID == uuid.Nil {
		ur.ID = uuid.New()
	}
	if ur.AssignedAt.IsZero() {
		ur.AssignedAt = time.Now()
	}
	return nil
}

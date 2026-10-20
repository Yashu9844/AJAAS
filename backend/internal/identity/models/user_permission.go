package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// UserPermissionGrant gives one user a permission at a scope, optionally until ExpiresAt (docs/ARCHITECTURE.md §17).
type UserPermissionGrant struct {
	ID           uuid.UUID   `gorm:"type:uuid;primaryKey" json:"id"`
	TenantID     uuid.UUID   `gorm:"type:uuid;not null;uniqueIndex:uq_user_permission_grants,priority:1" json:"tenant_id"`
	UserID       uuid.UUID   `gorm:"type:uuid;not null;uniqueIndex:uq_user_permission_grants,priority:2" json:"user_id"`
	PermissionID uuid.UUID   `gorm:"type:uuid;not null;uniqueIndex:uq_user_permission_grants,priority:3" json:"permission_id"`
	Scope        string      `gorm:"type:varchar(20);not null" json:"scope"`
	ExpiresAt    *time.Time  `gorm:"type:timestamptz" json:"expires_at,omitempty"`
	Reason       *string     `gorm:"type:varchar(500)" json:"reason,omitempty"`
	GrantedBy    *uuid.UUID  `gorm:"type:uuid" json:"granted_by,omitempty"`
	CreatedAt    time.Time   `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt    time.Time   `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
	Permission   *Permission `gorm:"foreignKey:PermissionID" json:"permission,omitempty"`
}

// BeforeCreate assigns a UUID.
func (g *UserPermissionGrant) BeforeCreate(tx *gorm.DB) error {
	if g.ID == uuid.Nil {
		g.ID = uuid.New()
	}
	return nil
}

// UserPermissionBlock removes a permission from one user whatever their roles grant; blocks always win.
type UserPermissionBlock struct {
	ID           uuid.UUID   `gorm:"type:uuid;primaryKey" json:"id"`
	TenantID     uuid.UUID   `gorm:"type:uuid;not null;uniqueIndex:uq_user_permission_blocks,priority:1" json:"tenant_id"`
	UserID       uuid.UUID   `gorm:"type:uuid;not null;uniqueIndex:uq_user_permission_blocks,priority:2" json:"user_id"`
	PermissionID uuid.UUID   `gorm:"type:uuid;not null;uniqueIndex:uq_user_permission_blocks,priority:3" json:"permission_id"`
	Reason       *string     `gorm:"type:varchar(500)" json:"reason,omitempty"`
	CreatedBy    *uuid.UUID  `gorm:"type:uuid" json:"created_by,omitempty"`
	CreatedAt    time.Time   `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt    time.Time   `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
	Permission   *Permission `gorm:"foreignKey:PermissionID" json:"permission,omitempty"`
}

// BeforeCreate assigns a UUID.
func (b *UserPermissionBlock) BeforeCreate(tx *gorm.DB) error {
	if b.ID == uuid.Nil {
		b.ID = uuid.New()
	}
	return nil
}

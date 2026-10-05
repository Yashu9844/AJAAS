package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/shared/database"
)

// User represents a user identity within a tenant.
type User struct {
	database.BaseModel
	TenantID        uuid.UUID  `gorm:"type:uuid;not null;index:idx_users_tenant_id;uniqueIndex:uq_users_tenant_email,priority:1" json:"tenant_id"`
	Email           string     `gorm:"type:varchar(255);not null;uniqueIndex:uq_users_tenant_email,priority:2;index:idx_users_email" json:"email"`
	PasswordHash    string     `gorm:"type:varchar(255);not null" json:"-"`
	FirstName       string     `gorm:"type:varchar(100);not null" json:"first_name"`
	LastName        string     `gorm:"type:varchar(100);not null" json:"last_name"`
	Phone           *string    `gorm:"type:varchar(20)" json:"phone,omitempty"`
	AvatarURL       *string    `gorm:"type:varchar(500)" json:"avatar_url,omitempty"`
	Status          string     `gorm:"type:varchar(20);not null;default:'active';index:idx_users_status" json:"status"`
	EmailVerifiedAt *time.Time `json:"email_verified_at,omitempty"`
	LastLoginAt     *time.Time `json:"last_login_at,omitempty"`

	// Relationships
	Roles         []UserRole     `gorm:"foreignKey:UserID" json:"roles,omitempty"`
	Sessions      []Session      `gorm:"foreignKey:UserID" json:"sessions,omitempty"`
	RefreshTokens []RefreshToken `gorm:"foreignKey:UserID" json:"refresh_tokens,omitempty"`
}

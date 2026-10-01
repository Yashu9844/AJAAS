package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// PasswordResetToken represents a verification token generated for password reset requests.
type PasswordResetToken struct {
	ID        uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	UserID    uuid.UUID  `gorm:"type:uuid;not null;index:idx_password_reset_tokens_user_id" json:"user_id"`
	TenantID  uuid.UUID  `gorm:"type:uuid;not null;index:idx_password_reset_tokens_tenant_id" json:"tenant_id"`
	TokenHash string     `gorm:"type:varchar(64);uniqueIndex:uq_password_reset_tokens_token_hash;not null" json:"token_hash"`
	ExpiresAt time.Time  `gorm:"not null" json:"expires_at"`
	UsedAt    *time.Time `json:"used_at,omitempty"`
	CreatedAt time.Time  `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt time.Time  `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
}

// BeforeCreate is a GORM hook that auto-generates a UUID v4 key if not already set.
func (pr *PasswordResetToken) BeforeCreate(tx *gorm.DB) error {
	if pr.ID == uuid.Nil {
		pr.ID = uuid.New()
	}
	return nil
}

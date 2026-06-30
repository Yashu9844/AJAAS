package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// RefreshToken represents a refresh token used for JWT renewal.
type RefreshToken struct {
	ID        uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	UserID    uuid.UUID  `gorm:"type:uuid;not null;index:idx_refresh_tokens_user_id;index:idx_refresh_tokens_user_tenant,priority:1" json:"user_id"`
	TenantID  uuid.UUID  `gorm:"type:uuid;not null;index:idx_refresh_tokens_tenant_id;index:idx_refresh_tokens_user_tenant,priority:2" json:"tenant_id"`
	TokenHash string     `gorm:"type:varchar(64);uniqueIndex:uq_refresh_tokens_token_hash;not null" json:"token_hash"`
	ExpiresAt time.Time  `gorm:"not null" json:"expires_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
	CreatedAt time.Time  `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt time.Time  `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
}

// BeforeCreate is a GORM hook that auto-generates a UUID v4 key if not already set.
func (rt *RefreshToken) BeforeCreate(tx *gorm.DB) error {
	if rt.ID == uuid.Nil {
		rt.ID = uuid.New()
	}
	return nil
}

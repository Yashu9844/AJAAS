package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// MFAConfig represents a multi-factor authentication setup for a user.
type MFAConfig struct {
	ID         uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	UserID     uuid.UUID  `gorm:"type:uuid;not null;index:idx_mfa_configs_user_id;uniqueIndex:uq_mfa_configs_user_type,priority:1" json:"user_id"`
	TenantID   uuid.UUID  `gorm:"type:uuid;not null;index:idx_mfa_configs_tenant_id" json:"tenant_id"`
	Type       string     `gorm:"type:varchar(20);not null;uniqueIndex:uq_mfa_configs_user_type,priority:2" json:"type"` // e.g., totp, sms, email
	Secret     string     `gorm:"type:varchar(500);not null" json:"-"`                                                   // Encrypted secret key
	IsEnabled  bool       `gorm:"not null;default:false" json:"is_enabled"`
	VerifiedAt *time.Time `json:"verified_at,omitempty"`
	CreatedAt  time.Time  `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt  time.Time  `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
}

// BeforeCreate is a GORM hook that auto-generates a UUID v4 key if not already set.
func (m *MFAConfig) BeforeCreate(tx *gorm.DB) error {
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}
	return nil
}

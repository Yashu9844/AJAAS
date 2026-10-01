package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Session represents an active login session.
type Session struct {
	ID        uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	UserID    uuid.UUID  `gorm:"type:uuid;not null;index:idx_sessions_user_id;index:idx_sessions_user_tenant,priority:1" json:"user_id"`
	TenantID  uuid.UUID  `gorm:"type:uuid;not null;index:idx_sessions_tenant_id;index:idx_sessions_user_tenant,priority:2" json:"tenant_id"`
	IPAddress string     `gorm:"type:varchar(45);not null" json:"ip_address"`
	UserAgent string     `gorm:"type:varchar(500);not null" json:"user_agent"`
	ExpiresAt time.Time  `gorm:"not null;index:idx_sessions_expires_at" json:"expires_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
	CreatedAt time.Time  `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt time.Time  `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
}

// BeforeCreate is a GORM hook that auto-generates a UUID v4 key if not already set.
func (s *Session) BeforeCreate(tx *gorm.DB) error {
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	return nil
}

package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Invitation lets an invited user activate their account with a single-use token (only its hash is stored).
type Invitation struct {
	ID         uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	TenantID   uuid.UUID  `gorm:"type:uuid;not null;index:idx_invitations_tenant_user,priority:1" json:"tenant_id"`
	UserID     uuid.UUID  `gorm:"type:uuid;not null;index:idx_invitations_tenant_user,priority:2" json:"user_id"`
	Email      string     `gorm:"type:varchar(255);not null" json:"email"`
	TokenHash  string     `gorm:"type:char(64);not null;uniqueIndex:uq_invitations_token_hash" json:"-"`
	ExpiresAt  time.Time  `gorm:"not null" json:"expires_at"`
	AcceptedAt *time.Time `json:"accepted_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	CreatedBy  *uuid.UUID `gorm:"type:uuid" json:"created_by,omitempty"`
	CreatedAt  time.Time  `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
}

// BeforeCreate assigns a UUID.
func (i *Invitation) BeforeCreate(tx *gorm.DB) error {
	if i.ID == uuid.Nil {
		i.ID = uuid.New()
	}
	return nil
}

// Pending reports whether the invitation can still be accepted at now.
func (i *Invitation) Pending(now time.Time) bool {
	return i.AcceptedAt == nil && i.RevokedAt == nil && now.Before(i.ExpiresAt)
}

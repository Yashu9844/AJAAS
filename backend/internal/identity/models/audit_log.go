package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// AuditLog represents an immutable append-only record of system actions.
type AuditLog struct {
	ID         uuid.UUID       `gorm:"type:uuid;primaryKey" json:"id"`
	TenantID   uuid.UUID       `gorm:"type:uuid;not null;index:idx_audit_logs_tenant_id;index:idx_audit_logs_tenant_created,priority:1;index:idx_audit_logs_tenant_action,priority:1" json:"tenant_id"`
	UserID     *uuid.UUID      `gorm:"type:uuid;index:idx_audit_logs_user_id" json:"user_id,omitempty"`
	Action     string          `gorm:"type:varchar(100);not null;index:idx_audit_logs_action;index:idx_audit_logs_tenant_action,priority:2" json:"action"`
	Resource   string          `gorm:"type:varchar(100);not null;index:idx_audit_logs_resource" json:"resource"`
	ResourceID *string         `gorm:"type:varchar(36)" json:"resource_id,omitempty"`
	Metadata   json.RawMessage `gorm:"type:jsonb" json:"metadata,omitempty"`
	IPAddress  *string         `gorm:"type:varchar(45)" json:"ip_address,omitempty"`
	UserAgent  *string         `gorm:"type:varchar(500)" json:"user_agent,omitempty"`
	CreatedAt  time.Time       `gorm:"not null;default:CURRENT_TIMESTAMP;index:idx_audit_logs_created_at;index:idx_audit_logs_tenant_created,priority:2" json:"created_at"`
}

// BeforeCreate is a GORM hook that auto-generates a UUID v4 key if not already set.
func (al *AuditLog) BeforeCreate(tx *gorm.DB) error {
	if al.ID == uuid.Nil {
		al.ID = uuid.New()
	}
	return nil
}

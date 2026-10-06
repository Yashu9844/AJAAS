package dto

import (
	"encoding/json"
	"time"
)

// AuditLogResponse is a read-only view of an audit record.
type AuditLogResponse struct {
	ID         string          `json:"id"`
	UserID     *string         `json:"user_id,omitempty"`
	Action     string          `json:"action"`
	Resource   string          `json:"resource"`
	ResourceID *string         `json:"resource_id,omitempty"`
	Metadata   json.RawMessage `json:"metadata,omitempty"`
	IPAddress  *string         `json:"ip_address,omitempty"`
	CreatedAt  time.Time       `json:"created_at"`
}

// AuditLogListResponse holds paginated audit records.
type AuditLogListResponse struct {
	Data []AuditLogResponse `json:"data"`
	Meta PaginationMeta     `json:"meta"`
}

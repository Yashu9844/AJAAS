package dto

import "time"

// SessionResponse represents serializable details of an active session.
type SessionResponse struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	TenantID  string    `json:"tenant_id"`
	IPAddress string    `json:"ip_address"`
	UserAgent string    `json:"user_agent"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

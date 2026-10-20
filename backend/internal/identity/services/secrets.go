package services

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Secret kinds delivered to users.
const (
	SecretPasswordReset = "password_reset"
	SecretInvitation    = "invitation"
)

// SecretMessage is a one-time secret for one user. It never enters events, audit rows or logs.
type SecretMessage struct {
	Kind      string    `json:"kind"`
	TenantID  uuid.UUID `json:"tenant_id"`
	UserID    uuid.UUID `json:"user_id"`
	Email     string    `json:"email"`
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	SentAt    time.Time `json:"sent_at"`
}

// SecretDelivery sends one-time secrets (email in production via M10 Notifications; the dev mailbox in development).
type SecretDelivery interface {
	Deliver(ctx context.Context, m SecretMessage) error
}

// devMailboxSize bounds the in-memory development mailbox.
const devMailboxSize = 200

// DevMailbox keeps the latest secrets in memory so developers and the test console can complete reset and
// invitation flows without email. It must never be enabled outside development.
type DevMailbox struct {
	mu    sync.Mutex
	items []SecretMessage
}

// NewDevMailbox returns an empty mailbox.
func NewDevMailbox() *DevMailbox { return &DevMailbox{} }

// Deliver stores the message, dropping the oldest when full.
func (d *DevMailbox) Deliver(_ context.Context, m SecretMessage) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	m.SentAt = time.Now().UTC()
	d.items = append(d.items, m)
	if len(d.items) > devMailboxSize {
		d.items = d.items[len(d.items)-devMailboxSize:]
	}
	return nil
}

// For returns the messages for an email address in a tenant, newest first.
func (d *DevMailbox) For(tenantID uuid.UUID, email string) []SecretMessage {
	d.mu.Lock()
	defer d.mu.Unlock()
	var out []SecretMessage
	for i := len(d.items) - 1; i >= 0; i-- {
		m := d.items[i]
		if m.TenantID == tenantID && strings.EqualFold(m.Email, email) {
			out = append(out, m)
		}
	}
	return out
}

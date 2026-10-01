package events

import (
	"time"

	"github.com/google/uuid"
)

// Event type string constants.
const (
	TypeTenantCreated      = "TenantCreated"
	TypeTenantActivated    = "TenantActivated"
	TypeTenantSuspended    = "TenantSuspended"
	TypeUserCreated        = "UserCreated"
	TypeUserInvited        = "UserInvited"
	TypeUserDeactivated    = "UserDeactivated"
	TypeRoleAssigned        = "RoleAssigned"
	TypeRoleRevoked         = "RoleRevoked"
	TypePermissionAssigned  = "PermissionAssigned"
	TypePermissionRevoked   = "PermissionRevoked"
	TypePasswordReset      = "PasswordReset"
	TypeSessionRevoked      = "SessionRevoked"
)

// Routing key constants.
const (
	RoutingKeyTenantCreated      = "identity.tenant.created"
	RoutingKeyTenantActivated    = "identity.tenant.activated"
	RoutingKeyTenantSuspended    = "identity.tenant.suspended"
	RoutingKeyUserCreated        = "identity.user.created"
	RoutingKeyUserInvited        = "identity.user.invited"
	RoutingKeyUserDeactivated    = "identity.user.deactivated"
	RoutingKeyRoleAssigned        = "identity.role.assigned"
	RoutingKeyRoleRevoked         = "identity.role.revoked"
	RoutingKeyPermissionAssigned  = "identity.permission.assigned"
	RoutingKeyPermissionRevoked   = "identity.permission.revoked"
	RoutingKeyPasswordReset      = "identity.password.reset"
	RoutingKeySessionRevoked      = "identity.session.revoked"
)

// Event is the generic envelope for publishing domain events.
type Event struct {
	EventID       uuid.UUID   `json:"event_id"`
	EventType     string      `json:"event_type"`
	RoutingKey    string      `json:"routing_key"`
	Version       string      `json:"version"`
	OccurredAt    time.Time   `json:"occurred_at"`
	Producer      string      `json:"producer"`
	TenantID      uuid.UUID   `json:"tenant_id"`
	CorrelationID uuid.UUID   `json:"correlation_id"`
	Payload       interface{} `json:"payload"`
}

// NewEvent wraps a domain payload inside the standard event envelope.
func NewEvent(eventType string, routingKey string, tenantID uuid.UUID, correlationID uuid.UUID, payload interface{}) Event {
	return Event{
		EventID:       uuid.New(),
		EventType:     eventType,
		RoutingKey:    routingKey,
		Version:       "1.0",
		OccurredAt:    time.Now(),
		Producer:      "identity-service",
		TenantID:      tenantID,
		CorrelationID: correlationID,
		Payload:       payload,
	}
}

// --- Payload Structs ---

type TenantCreatedPayload struct {
	TenantID  uuid.UUID `json:"tenant_id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	Plan      string    `json:"plan"`
	CreatedAt time.Time `json:"created_at"`
}

type TenantActivatedPayload struct {
	TenantID       uuid.UUID `json:"tenant_id"`
	PreviousStatus string    `json:"previous_status"`
	NewStatus      string    `json:"new_status"`
	ActivatedAt    time.Time `json:"activated_at"`
}

type TenantSuspendedPayload struct {
	TenantID       uuid.UUID `json:"tenant_id"`
	PreviousStatus string    `json:"previous_status"`
	NewStatus      string    `json:"new_status"`
	SuspendedAt    time.Time `json:"suspended_at"`
}

type UserCreatedPayload struct {
	UserID    uuid.UUID   `json:"user_id"`
	TenantID  uuid.UUID   `json:"tenant_id"`
	Email     string      `json:"email"`
	FirstName string      `json:"first_name"`
	LastName  string      `json:"last_name"`
	Status    string      `json:"status"`
	RoleIDs   []uuid.UUID `json:"role_ids"`
	CreatedBy uuid.UUID   `json:"created_by"`
	CreatedAt time.Time   `json:"created_at"`
}

type UserInvitedPayload struct {
	UserID    uuid.UUID   `json:"user_id"`
	TenantID  uuid.UUID   `json:"tenant_id"`
	Email     string      `json:"email"`
	FirstName string      `json:"first_name"`
	LastName  string      `json:"last_name"`
	InvitedBy uuid.UUID   `json:"invited_by"`
	InvitedAt time.Time   `json:"invited_at"`
}

type UserDeactivatedPayload struct {
	UserID        uuid.UUID `json:"user_id"`
	TenantID      uuid.UUID `json:"tenant_id"`
	DeactivatedBy uuid.UUID `json:"deactivated_by"`
	DeactivatedAt time.Time `json:"deactivated_at"`
}

type RoleAssignedPayload struct {
	UserID     uuid.UUID   `json:"user_id"`
	TenantID   uuid.UUID   `json:"tenant_id"`
	RoleIDs    []uuid.UUID `json:"role_ids"`
	AssignedBy uuid.UUID   `json:"assigned_by"`
	AssignedAt time.Time   `json:"assigned_at"`
}

type RoleRevokedPayload struct {
	UserID    uuid.UUID   `json:"user_id"`
	TenantID  uuid.UUID   `json:"tenant_id"`
	RoleIDs   []uuid.UUID `json:"role_ids"`
	RevokedBy uuid.UUID   `json:"revoked_by"`
	RevokedAt time.Time   `json:"revoked_at"`
}

type PermissionAssignedPayload struct {
	RoleID        uuid.UUID   `json:"role_id"`
	TenantID      uuid.UUID   `json:"tenant_id"`
	PermissionIDs []uuid.UUID `json:"permission_ids"`
	AssignedAt    time.Time   `json:"assigned_at"`
}

type PermissionRevokedPayload struct {
	RoleID        uuid.UUID   `json:"role_id"`
	TenantID      uuid.UUID   `json:"tenant_id"`
	PermissionIDs []uuid.UUID `json:"permission_ids"`
	RevokedAt     time.Time   `json:"revoked_at"`
}

type PasswordResetPayload struct {
	UserID   uuid.UUID `json:"user_id"`
	TenantID uuid.UUID `json:"tenant_id"`
	ResetAt  time.Time `json:"reset_at"`
}

type SessionRevokedPayload struct {
	SessionID uuid.UUID `json:"session_id"`
	UserID    uuid.UUID `json:"user_id"`
	TenantID  uuid.UUID `json:"tenant_id"`
	Reason    string    `json:"reason"`
	RevokedAt time.Time `json:"revoked_at"`
}

package events

import (
	"time"

	"github.com/google/uuid"
)

// Exchange for organization domain events.
const Exchange = "jaas.organization.events"

// Event type constants.
const (
	TypeDepartmentCreated     = "DepartmentCreated"
	TypeDepartmentUpdated     = "DepartmentUpdated"
	TypeDepartmentDeactivated = "DepartmentDeactivated"
	TypeTeamCreated           = "TeamCreated"
	TypeTeamMoved             = "TeamMoved"
	TypeTeamDeactivated       = "TeamDeactivated"
	TypeMappingCreated        = "MappingCreated"
	TypeMappingUpdated        = "MappingUpdated"
	TypeMappingDeactivated    = "MappingDeactivated"
	TypeHierarchyMoved        = "HierarchyMoved"
	TypeLocationCreated       = "LocationCreated"
	TypeLocationUpdated       = "LocationUpdated"
	TypeLocationDeactivated   = "LocationDeactivated"
	TypeLocationActivated     = "LocationActivated"
)

// Routing keys.
const (
	RoutingKeyDepartmentCreated     = "organization.department.created"
	RoutingKeyDepartmentUpdated     = "organization.department.updated"
	RoutingKeyDepartmentDeactivated = "organization.department.deactivated"
	RoutingKeyTeamCreated           = "organization.team.created"
	RoutingKeyTeamMoved             = "organization.team.moved"
	RoutingKeyTeamDeactivated       = "organization.team.deactivated"
	RoutingKeyMappingCreated        = "organization.mapping.created"
	RoutingKeyMappingUpdated        = "organization.mapping.updated"
	RoutingKeyMappingDeactivated    = "organization.mapping.deactivated"
	RoutingKeyHierarchyMoved        = "organization.hierarchy.moved"
	RoutingKeyLocationCreated       = "organization.location.created"
	RoutingKeyLocationUpdated       = "organization.location.updated"
	RoutingKeyLocationDeactivated   = "organization.location.deactivated"
	RoutingKeyLocationActivated     = "organization.location.activated"
)

// Consumed identity routing keys (FR-E001..FR-E004, connections C7).
const (
	ConsumedTenantCreated   = "identity.tenant.created"
	ConsumedTenantSuspended = "identity.tenant.suspended"
	ConsumedUserCreated     = "identity.user.created"
	ConsumedUserDeactivated = "identity.user.deactivated"
)

// ConsumedUserPayload is the identity user lifecycle payload consumed by org.
type ConsumedUserPayload struct {
	UserID   uuid.UUID `json:"user_id"`
	TenantID uuid.UUID `json:"tenant_id"`
}

// ConsumedTenantPayload is the identity tenant lifecycle payload consumed by org.
type ConsumedTenantPayload struct {
	TenantID uuid.UUID `json:"tenant_id"`
}

// Event is the standard envelope (mirrors identity events).
type Event struct {
	EventID       uuid.UUID   `json:"event_id"`
	EventType     string      `json:"event_type"`
	RoutingKey    string      `json:"routing_key"`
	Version       string      `json:"version"`
	EventVersion  int         `json:"event_version"` // envelope schema version (docs/ARCHITECTURE.md §3.7)
	CausationID   *uuid.UUID  `json:"causation_id,omitempty"`
	OccurredAt    time.Time   `json:"occurred_at"`
	Producer      string      `json:"producer"`
	TenantID      uuid.UUID   `json:"tenant_id"`
	CorrelationID uuid.UUID   `json:"correlation_id"`
	Payload       interface{} `json:"payload"`
}

// NewEvent wraps a payload in the standard envelope.
func NewEvent(eventType, routingKey string, tenantID, correlationID uuid.UUID, payload interface{}) Event {
	return Event{
		EventID: uuid.New(), EventType: eventType, RoutingKey: routingKey,
		Version: "1.0", OccurredAt: time.Now().UTC(), Producer: "organization-service",
		EventVersion: 1,
		TenantID:     tenantID, CorrelationID: correlationID, Payload: payload,
	}
}

// DepartmentPayload carries department lifecycle data.
type DepartmentPayload struct {
	DepartmentID uuid.UUID `json:"department_id"`
	Name         string    `json:"name"`
}

// TeamPayload carries team lifecycle data.
type TeamPayload struct {
	TeamID       uuid.UUID `json:"team_id"`
	DepartmentID uuid.UUID `json:"department_id"`
	Name         string    `json:"name"`
}

// MappingPayload carries mapping lifecycle data.
type MappingPayload struct {
	MappingID uuid.UUID `json:"mapping_id"`
	UserID    uuid.UUID `json:"user_id"`
}

// LocationPayload carries the location id, code, state and country — no personal data (FR-L007).
type LocationPayload struct {
	LocationID  uuid.UUID `json:"location_id"`
	Code        string    `json:"code"`
	StateCode   string    `json:"state_code"`
	CountryCode string    `json:"country_code"`
	Status      string    `json:"status"`
}

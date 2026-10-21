// Package events defines HR/Core event types and the shared envelope (specification.md §6).
package events

import (
	"time"

	"github.com/google/uuid"
)

// Exchange and routing keys.
const (
	Exchange        = "jaas.core.events"
	ActionSubmitted = "hr.core.action_submitted"
	ActionCompleted = "hr.core.action_completed"
	ActionRescinded = "hr.core.action_rescinded"
	PositionChanged = "hr.core.position_changed"

	envelopeVersion = "1.0"
	producer        = "core-service"
)

// Envelope is the platform event envelope (same shape as the other HR contexts).
type Envelope struct {
	EventID       uuid.UUID   `json:"event_id"`
	EventType     string      `json:"event_type"`
	RoutingKey    string      `json:"routing_key"`
	Version       string      `json:"version"`
	EventVersion  int         `json:"event_version"`
	CausationID   *uuid.UUID  `json:"causation_id,omitempty"`
	OccurredAt    time.Time   `json:"occurred_at"`
	Producer      string      `json:"producer"`
	TenantID      uuid.UUID   `json:"tenant_id"`
	CorrelationID uuid.UUID   `json:"correlation_id"`
	Payload       interface{} `json:"payload"`
}

// Source identifies who and when an event came from.
type Source struct {
	TenantID, CorrelationID uuid.UUID
	OccurredAt              time.Time
}

// New wraps a payload in the envelope.
func New(routingKey string, src Source, payload interface{}) Envelope {
	return Envelope{EventID: uuid.New(), EventType: routingKey, RoutingKey: routingKey, Version: envelopeVersion, EventVersion: 1,
		OccurredAt: src.OccurredAt.UTC(), Producer: producer, TenantID: src.TenantID, CorrelationID: src.CorrelationID, Payload: payload}
}

// ActionPayload carries ids, type, status and the effective date only — never comments or reasons (HR-009).
type ActionPayload struct {
	ActionID      uuid.UUID  `json:"action_id"`
	EmployeeID    uuid.UUID  `json:"employee_id"`
	Type          string     `json:"type"`
	Status        string     `json:"status"`
	EffectiveDate string     `json:"effective_date"`
	AssignmentID  *uuid.UUID `json:"assignment_id,omitempty"`
}

// PositionPayload carries a position's id, status and department.
type PositionPayload struct {
	PositionID   uuid.UUID `json:"position_id"`
	DepartmentID uuid.UUID `json:"department_id"`
	Status       string    `json:"status"`
}

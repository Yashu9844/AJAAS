// Package events defines Module 4 event types and the shared envelope (FR-EV001).
package events

import (
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/leave/calc"
)

// Exchange and routing keys (FR-EV001).
const (
	Exchange  = "jaas.leave.events"
	Applied   = "leave.applied"
	Approved  = "leave.approved"
	Rejected  = "leave.rejected"
	Cancelled = "leave.cancelled"
	Accrued   = "leave.accrued"

	envelopeVersion = "1.0"
	producer        = "leave-service"
)

// Envelope is the platform event envelope (same shape as Module 3).
type Envelope struct {
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

// Source identifies who and when an event came from.
type Source struct {
	TenantID, CorrelationID uuid.UUID
	OccurredAt              time.Time
}

// New wraps a payload in the envelope.
func New(routingKey string, src Source, payload interface{}) Envelope {
	return Envelope{
		EventID: uuid.New(), EventType: routingKey, RoutingKey: routingKey, Version: envelopeVersion,
		OccurredAt: src.OccurredAt.UTC(), Producer: producer, TenantID: src.TenantID, CorrelationID: src.CorrelationID,
		Payload: payload,
	}
}

// RequestPayload is published for leave.applied|approved|rejected|cancelled; no reason/comment (FR-EV003).
type RequestPayload struct {
	RequestID     uuid.UUID `json:"leave_request_id"`
	EmployeeID    uuid.UUID `json:"employee_id"`
	LeaveTypeID   uuid.UUID `json:"leave_type_id"`
	LeaveTypeCode string    `json:"leave_type_code"`
	IsPaid        bool      `json:"is_paid"`
	StartDate     string    `json:"start_date"`
	EndDate       string    `json:"end_date"`
	HalfDay       *string   `json:"half_day,omitempty"`
	TotalDays     calc.Days `json:"total_days"`
	Status        string    `json:"status"`
}

// AccruedPayload is published for leave.accrued.
type AccruedPayload struct {
	EmployeeID   uuid.UUID `json:"employee_id"`
	LeaveTypeID  uuid.UUID `json:"leave_type_id"`
	Year         int       `json:"year"`
	Days         calc.Days `json:"days"`
	AccruedTotal calc.Days `json:"accrued_total"`
}

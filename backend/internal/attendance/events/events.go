// Package events defines Module 3 produced events (FR-EV001). Payloads carry no IP/device/coordinates (FR-EV003).
package events

import (
	"time"

	"github.com/google/uuid"
)

// Exchange is the topic exchange for attendance events.
const Exchange = "jaas.attendance.events"

// Routing keys (also used as event_type).
const (
	PunchIn                 = "attendance.punch.in"
	PunchOut                = "attendance.punch.out"
	RegularizationRequested = "attendance.regularization.requested"
	RegularizationApproved  = "attendance.regularization.approved"
	RegularizationRejected  = "attendance.regularization.rejected"
	envelopeVersion         = "1.0"
	producer                = "attendance-service"
)

// Envelope is the standard JAAS event envelope.
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

// Source identifies who and when an event came from; OccurredAt comes from the service clock.
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

// PunchPayload is published for attendance.punch.in/out.
type PunchPayload struct {
	PunchID        uuid.UUID `json:"punch_id"`
	RecordID       uuid.UUID `json:"record_id"`
	EmployeeID     uuid.UUID `json:"employee_id"`
	AttendanceDate string    `json:"attendance_date"`
	PunchType      string    `json:"punch_type"`
	PunchTime      time.Time `json:"punch_time"`
	Status         string    `json:"status"`
	WorkMinutes    int       `json:"total_work_minutes"`
	LateMinutes    int       `json:"late_minutes"`
}

// RegularizationPayload is published for attendance.regularization.*.
type RegularizationPayload struct {
	RegularizationID  uuid.UUID  `json:"regularization_id"`
	EmployeeID        uuid.UUID  `json:"employee_id"`
	AttendanceDate    string     `json:"attendance_date"`
	Status            string     `json:"status"`
	RequestedPunchIn  time.Time  `json:"requested_punch_in"`
	RequestedPunchOut time.Time  `json:"requested_punch_out"`
	ReviewerUserID    *uuid.UUID `json:"reviewer_user_id,omitempty"`
}

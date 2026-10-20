// Package events defines Module 6 event types and the shared envelope (FR-EV001).
package events

import (
	"time"

	"github.com/google/uuid"
)

// Exchange and routing keys (FR-EV001).
const (
	Exchange           = "jaas.recruitment.events"
	JobPublished       = "recruitment.job_published"
	CandidateApplied   = "recruitment.candidate_applied"
	InterviewScheduled = "recruitment.interview_scheduled"
	CandidateHired     = "recruitment.candidate_hired"

	envelopeVersion = "1.0"
	producer        = "recruitment-service"
)

// Envelope is the platform event envelope (same shape as Modules 3–5).
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
	return Envelope{EventID: uuid.New(), EventType: routingKey, RoutingKey: routingKey, Version: envelopeVersion,
		OccurredAt: src.OccurredAt.UTC(), Producer: producer, TenantID: src.TenantID, CorrelationID: src.CorrelationID, Payload: payload}
}

// Payload carries ids and stages only — never candidate PII, CTC or feedback (FR-EV002, RC-011).
type Payload struct {
	JobID       uuid.UUID  `json:"job_id"`
	CandidateID *uuid.UUID `json:"candidate_id,omitempty"`
	InterviewID *uuid.UUID `json:"interview_id,omitempty"`
	EmployeeID  *uuid.UUID `json:"employee_id,omitempty"`
	Stage       string     `json:"stage,omitempty"`
	JobStatus   string     `json:"job_status,omitempty"`
}

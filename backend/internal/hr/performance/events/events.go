// Package events defines HR/Performance event types and the shared envelope (FR-EV001).
package events

import (
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/hr/performance/score"
)

// Exchange and routing keys (FR-EV001).
const (
	Exchange        = "jaas.performance.events"
	CycleStarted    = "performance.cycle_started"
	GoalUpdated     = "performance.goal_updated"
	ReviewSubmitted = "performance.review_submitted"
	CycleCompleted  = "performance.cycle_completed"

	envelopeVersion = "1.0"
	producer        = "performance-service"
)

// Envelope is the platform event envelope (same shape as the other HR contexts).
type Envelope struct {
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

// Payload carries ids, statuses and progress only — never ratings, scores, potential or review text (FR-EV002, PM-017).
type Payload struct {
	CycleID    uuid.UUID    `json:"cycle_id"`
	GoalID     *uuid.UUID   `json:"goal_id,omitempty"`
	ReviewID   *uuid.UUID   `json:"review_id,omitempty"`
	EmployeeID *uuid.UUID   `json:"employee_id,omitempty"`
	Status     string       `json:"status,omitempty"`
	ReviewType string       `json:"review_type,omitempty"`
	Progress   *score.Fixed `json:"progress,omitempty"`
}

// Package events defines Module 5 event types and the shared envelope (FR-EV001).
package events

import (
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/payroll/calc"
)

// Exchange and routing keys (FR-EV001).
const (
	Exchange      = "jaas.payroll.events"
	RunInitiated  = "payroll.run_initiated"
	RunCalculated = "payroll.calculated"
	RunApproved   = "payroll.approved"
	RunFinalized  = "payroll.finalized"

	envelopeVersion = "1.0"
	producer        = "payroll-service"
)

// Envelope is the platform event envelope (same shape as Modules 3/4).
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

// RunPayload carries run identity, period, counts and totals only — never per-employee amounts (FR-EV002).
type RunPayload struct {
	RunID          uuid.UUID  `json:"run_id"`
	Year           int        `json:"year"`
	Month          int        `json:"month"`
	Status         string     `json:"status"`
	EmployeeCount  int        `json:"employee_count"`
	GrossTotal     calc.Money `json:"gross_total"`
	DeductionTotal calc.Money `json:"deduction_total"`
	NetTotal       calc.Money `json:"net_total"`
}

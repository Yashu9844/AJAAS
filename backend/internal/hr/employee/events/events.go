package events

import (
	"time"

	"github.com/google/uuid"
)

const (
	ExchangeName = "jaas.employee.events"

	EventTypeEmployeeCreated       = "employee.created"
	EventTypeEmployeeUpdated       = "employee.updated"
	EventTypeEmployeeStatusChanged = "employee.status.changed"
	EventTypeEmployeeExited        = "employee.exited"
	EventTypeEmployeeDocVerified   = "employee.document.verified"
)

type EventEnvelope struct {
	EventID       uuid.UUID   `json:"event_id"`
	EventType     string      `json:"event_type"`
	RoutingKey    string      `json:"routing_key"`
	Version       string      `json:"version"`
	OccurredAt    time.Time   `json:"occurred_at"`
	Producer      string      `json:"producer"`
	TenantID      uuid.UUID   `json:"tenant_id"`
	CorrelationID string      `json:"correlation_id,omitempty"`
	Payload       interface{} `json:"payload"`
}

func NewEventEnvelope(eventType, routingKey string, tenantID uuid.UUID, payload interface{}) EventEnvelope {
	return EventEnvelope{
		EventID:    uuid.New(),
		EventType:  eventType,
		RoutingKey: routingKey,
		Version:    "1.0",
		OccurredAt: time.Now().UTC(),
		Producer:   "jaas-employee-service",
		TenantID:   tenantID,
		Payload:    payload,
	}
}

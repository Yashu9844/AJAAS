package events_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/employee/events"
)

func TestNewEventEnvelope(t *testing.T) {
	tenantID := uuid.New()
	payload := map[string]string{"employee_code": "EMP-001"}
	env := events.NewEventEnvelope(events.EventTypeEmployeeCreated, "employee.created", tenantID, payload)

	if env.EventID == uuid.Nil {
		t.Errorf("expected generated event ID")
	}
	if env.EventType != events.EventTypeEmployeeCreated {
		t.Errorf("expected %s, got %s", events.EventTypeEmployeeCreated, env.EventType)
	}
	if env.TenantID != tenantID {
		t.Errorf("expected %s, got %s", tenantID, env.TenantID)
	}
	if env.Producer != "jaas-employee-service" {
		t.Errorf("expected jaas-employee-service, got %s", env.Producer)
	}
}

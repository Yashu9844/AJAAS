package events

import (
	"testing"

	"github.com/google/uuid"
)

func TestNewEvent_Envelope(t *testing.T) {
	tenantID := uuid.New()
	correlationID := uuid.New()
	evt := NewEvent(TypeDepartmentCreated, RoutingKeyDepartmentCreated, tenantID, correlationID,
		DepartmentPayload{DepartmentID: uuid.New(), Name: "Engineering"})

	if evt.EventID == uuid.Nil {
		t.Error("expected event_id to be set")
	}
	if evt.Version != "1.0" || evt.Producer != "organization-service" {
		t.Errorf("unexpected envelope: %+v", evt)
	}
	if evt.TenantID != tenantID || evt.CorrelationID != correlationID {
		t.Error("expected tenant/correlation IDs to propagate")
	}
	if evt.RoutingKey != "organization.department.created" {
		t.Errorf("unexpected routing key: %s", evt.RoutingKey)
	}
}

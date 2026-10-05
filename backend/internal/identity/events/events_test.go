package events

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestNewEvent(t *testing.T) {
	tenantID := uuid.New()
	correlationID := uuid.New()
	payload := TenantCreatedPayload{
		TenantID:  tenantID,
		Name:      "Test Tenant",
		Slug:      "test-tenant",
		Plan:      "free",
		CreatedAt: time.Now(),
	}

	event := NewEvent(
		TypeTenantCreated,
		RoutingKeyTenantCreated,
		tenantID,
		correlationID,
		payload,
	)

	if event.EventID == uuid.Nil {
		t.Error("expected generated EventID to be non-nil UUID")
	}

	if event.EventType != TypeTenantCreated {
		t.Errorf("expected EventType %q, got %q", TypeTenantCreated, event.EventType)
	}

	if event.RoutingKey != RoutingKeyTenantCreated {
		t.Errorf("expected RoutingKey %q, got %q", RoutingKeyTenantCreated, event.RoutingKey)
	}

	if event.TenantID != tenantID {
		t.Errorf("expected TenantID %s, got %s", tenantID, event.TenantID)
	}

	if event.CorrelationID != correlationID {
		t.Errorf("expected CorrelationID %s, got %s", correlationID, event.CorrelationID)
	}

	p, ok := event.Payload.(TenantCreatedPayload)
	if !ok {
		t.Fatalf("expected payload type TenantCreatedPayload, got %T", event.Payload)
	}

	if p.Slug != "test-tenant" {
		t.Errorf("expected payload slug 'test-tenant', got %s", p.Slug)
	}
}

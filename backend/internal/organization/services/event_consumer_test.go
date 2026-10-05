package services

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/organization/dto"
	"github.com/jaas/jaas/internal/organization/events"
	"gorm.io/gorm"
)

// spyMappingSvc records DeactivateUserMappings calls for consumer tests.
type spyMappingSvc struct {
	deactivated []uuid.UUID
	tenantSeen  []uuid.UUID
	err         error
}

func (s *spyMappingSvc) CreateMapping(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, req dto.CreateMappingRequest, correlationID uuid.UUID) (*dto.MappingResponse, error) {
	return nil, nil
}
func (s *spyMappingSvc) GetMapping(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*dto.MappingResponse, error) {
	return nil, nil
}
func (s *spyMappingSvc) ListUserMappings(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID, page, perPage int) (*dto.MappingListResponse, error) {
	return nil, nil
}
func (s *spyMappingSvc) UpdateMapping(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID, req dto.UpdateMappingRequest) (*dto.MappingResponse, error) {
	return nil, nil
}
func (s *spyMappingSvc) DeactivateMapping(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID, req dto.DeactivateMappingRequest) (*dto.MappingResponse, error) {
	return nil, nil
}
func (s *spyMappingSvc) DeactivateUserMappings(ctx context.Context, tx *gorm.DB, tenantID, userID uuid.UUID, correlationID uuid.UUID) error {
	s.tenantSeen = append(s.tenantSeen, tenantID)
	s.deactivated = append(s.deactivated, userID)
	return s.err
}

func TestEventConsumer_UserDeactivatedConverges(t *testing.T) {
	spy := &spyMappingSvc{}
	consumer := NewEventConsumer(spy)

	tenantID := uuid.New()
	userID := uuid.New()
	payload, _ := json.Marshal(events.ConsumedUserPayload{UserID: userID, TenantID: tenantID})

	if err := consumer.HandleIdentityEvent(context.Background(), nil, events.ConsumedUserDeactivated, payload, uuid.New()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(spy.deactivated) != 1 || spy.deactivated[0] != userID || spy.tenantSeen[0] != tenantID {
		t.Errorf("expected DeactivateUserMappings(%s, %s), got %v/%v", tenantID, userID, spy.tenantSeen, spy.deactivated)
	}

	// Redelivery is idempotent-safe: handler delegates again without error (C7).
	if err := consumer.HandleIdentityEvent(context.Background(), nil, events.ConsumedUserDeactivated, payload, uuid.New()); err != nil {
		t.Fatalf("redelivery must not error: %v", err)
	}
}

func TestEventConsumer_NoopHandlers(t *testing.T) {
	spy := &spyMappingSvc{}
	consumer := NewEventConsumer(spy)
	payload, _ := json.Marshal(events.ConsumedTenantPayload{TenantID: uuid.New()})

	for _, key := range []string{events.ConsumedTenantCreated, events.ConsumedTenantSuspended, events.ConsumedUserCreated} {
		if err := consumer.HandleIdentityEvent(context.Background(), nil, key, payload, uuid.New()); err != nil {
			t.Errorf("%s must be a no-op, got: %v", key, err)
		}
	}
	if len(spy.deactivated) != 0 {
		t.Errorf("no-op events must not touch mappings, got %v", spy.deactivated)
	}
}

func TestEventConsumer_UnknownKeyAndBadPayload(t *testing.T) {
	consumer := NewEventConsumer(&spyMappingSvc{})
	if err := consumer.HandleIdentityEvent(context.Background(), nil, "identity.unknown", json.RawMessage(`{}`), uuid.New()); err == nil {
		t.Error("expected error on unknown routing key")
	}
	if err := consumer.HandleIdentityEvent(context.Background(), nil, events.ConsumedUserDeactivated, json.RawMessage(`{bad`), uuid.New()); err == nil {
		t.Error("expected error on malformed payload")
	}
}

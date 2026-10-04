package services

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/organization/events"
	"gorm.io/gorm"
)

// EventConsumer handles identity domain events consumed by Module 1 (FR-E001..FR-E004).
type EventConsumer interface {
	// HandleIdentityEvent dispatches one consumed identity event by routing key.
	// Handlers are idempotent: redelivery must converge to the same state (C7).
	HandleIdentityEvent(ctx context.Context, db *gorm.DB, routingKey string, payload json.RawMessage, correlationID uuid.UUID) error
}

type eventConsumer struct {
	mappingSvc MappingService
}

// NewEventConsumer creates the consumer over org services.
func NewEventConsumer(mappingSvc MappingService) EventConsumer {
	return &eventConsumer{mappingSvc: mappingSvc}
}

func (c *eventConsumer) HandleIdentityEvent(ctx context.Context, db *gorm.DB, routingKey string, payload json.RawMessage, correlationID uuid.UUID) error {
	switch routingKey {
	case events.ConsumedUserDeactivated:
		// FR-E004 → FR-M005: deactivate the user's mappings, clear manager refs.
		var p events.ConsumedUserPayload
		if err := json.Unmarshal(payload, &p); err != nil {
			return fmt.Errorf("decode user.deactivated payload: %w", err)
		}
		return c.mappingSvc.DeactivateUserMappings(ctx, db, p.TenantID, p.UserID, correlationID)
	case events.ConsumedTenantCreated, events.ConsumedTenantSuspended, events.ConsumedUserCreated:
		// No-ops by contract: org starts empty (FR-E001), write freeze is enforced
		// at the Module 0 tenant resolver edge (FR-E002), no auto-mapping (FR-E003/FR-M006).
		return nil
	default:
		return fmt.Errorf("unknown identity routing key: %s", routingKey)
	}
}

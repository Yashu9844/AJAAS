package kernel

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// OutboxTable is M2's transactional outbox (published by shared/outbox.TableRelay).
const OutboxTable = "projects_events_outbox"

// Producer names M2 in event envelopes.
const Producer = "projects-service"

// Emit writes one event, version 1, in the caller transaction. Payloads hold ids and versions only (EV, PG12).
func Emit(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, key string, payload map[string]any) error {
	id := uuid.New()
	env := map[string]any{"event_id": id, "event_type": key, "routing_key": key, "event_version": 1,
		"tenant_id": tenantID, "occurred_at": time.Now().UTC(), "producer": Producer, "payload": payload}
	if agent := AgentFrom(ctx); agent != "" {
		env["agent"] = agent // the action was taken by an AI agent acting for a user
	}
	raw, err := json.Marshal(env)
	if err != nil {
		return err
	}
	return tx.WithContext(ctx).Exec(`INSERT INTO `+OutboxTable+` (id, tenant_id, event_id, routing_key, payload) VALUES (?, ?, ?, ?, ?)`,
		uuid.New(), tenantID, id, key, string(raw)).Error
}

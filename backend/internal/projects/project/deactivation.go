package project

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/kernel"
	"github.com/jaas/jaas/internal/shared/inbox"
	"gorm.io/gorm"
)

// consumerName identifies this handler in the inbox ledger.
const consumerName = "projects.identity"

// DeactivateUserMappings marks the user inactive in every project and on every assignment, keeping history. It is the
// identityport DeactivationHook, so it runs in the same transaction that deactivates the login (specification section 7).
func (s *Service) DeactivateUserMappings(ctx context.Context, tx *gorm.DB, tenantID, userID, _ uuid.UUID) error {
	db := tx.WithContext(ctx)
	var projectIDs []uuid.UUID
	if err := db.Raw(`UPDATE project_members SET active = FALSE WHERE tenant_id = ? AND user_id = ? AND active RETURNING project_id`,
		tenantID, userID).Scan(&projectIDs).Error; err != nil {
		return err
	}
	if err := db.Exec(`UPDATE task_assignees SET active = FALSE WHERE tenant_id = ? AND user_id = ? AND active`, tenantID, userID).Error; err != nil {
		return err
	}
	if err := dropWatches(tx, tenantID, uuid.Nil, userID); err != nil {
		return err
	}
	for _, pid := range projectIDs {
		if err := kernel.LogActivity(ctx, tx, kernel.Activity{TenantID: tenantID, ProjectID: pid, ActorID: &userID, Type: "member.deactivated",
			Data: map[string]any{"user_id": userID}, Source: "system"}); err != nil {
			return err
		}
		if err := kernel.Emit(ctx, tx, tenantID, "project.member.removed", map[string]any{"project_id": pid, "user_id": userID, "reason": "deactivated"}); err != nil {
			return err
		}
	}
	return nil
}

type identityEnvelope struct {
	EventID    uuid.UUID `json:"event_id"`
	RoutingKey string    `json:"routing_key"`
	TenantID   uuid.UUID `json:"tenant_id"`
	Payload    struct {
		UserID uuid.UUID `json:"user_id"`
	} `json:"payload"`
}

// HandleIdentityEvent applies an identity or HR lifecycle event delivered by a broker consumer. Redeliveries are
// recognised through the inbox ledger and ignored (PG10). Unknown event types are ignored.
func (s *Service) HandleIdentityEvent(ctx context.Context, raw []byte) error {
	var ev identityEnvelope
	if err := json.Unmarshal(raw, &ev); err != nil {
		return kernel.Invalid("event", "is not valid JSON")
	}
	if ev.RoutingKey != "identity.user.deactivated" && ev.RoutingKey != "hr.employee.terminated" {
		return nil
	}
	if ev.EventID == uuid.Nil || ev.TenantID == uuid.Nil || ev.Payload.UserID == uuid.Nil {
		return kernel.Invalid("event", "needs event_id, tenant_id and payload.user_id")
	}
	return s.inTx(ctx, func(tx *gorm.DB) error {
		first, err := inbox.Claim(tx, "projects_events_inbox", consumerName, ev.TenantID, ev.EventID, ev.RoutingKey)
		if err != nil || !first {
			return err
		}
		return s.DeactivateUserMappings(ctx, tx, ev.TenantID, ev.Payload.UserID, uuid.Nil)
	})
}

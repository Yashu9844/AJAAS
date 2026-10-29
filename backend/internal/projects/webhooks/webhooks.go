package webhooks

import (
	"context"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
	"gorm.io/gorm"
)

// Webhooks lists the webhooks of a project (project.update: a chat webhook URL is itself a secret).
func (s *Service) Webhooks(ctx context.Context, a api.Actor, projectID uuid.UUID) ([]Webhook, error) {
	if _, err := s.require(ctx, a, projectID, api.ProjectUpdate); err != nil {
		return nil, err
	}
	var rows []Webhook
	err := s.db.WithContext(ctx).Where("project_id = ? AND tenant_id = ?", projectID, a.TenantID).Order("created_at, id").Find(&rows).Error
	return rows, err
}

// CreateWebhook adds a webhook (project.update); a project has at most 20. The signing secret is returned once.
func (s *Service) CreateWebhook(ctx context.Context, a api.Actor, projectID uuid.UUID, in WebhookInput) (*WebhookWithSecret, error) {
	if _, err := s.require(ctx, a, projectID, api.ProjectUpdate); err != nil {
		return nil, err
	}
	w, err := in.build()
	if err != nil {
		return nil, err
	}
	secret, sealed, err := s.newSealedSecret()
	if err != nil {
		return nil, err
	}
	now := s.stamp()
	w.ID, w.TenantID, w.ProjectID, w.CreatedBy = uuid.New(), a.TenantID, projectID, a.UserID
	w.SecretEnc, w.SecretHint, w.CreatedAt, w.UpdatedAt = sealed, Hint(secret), now, now
	err = s.inTx(ctx, func(tx *gorm.DB) error { return s.insertWebhook(ctx, tx, a, &w) })
	if isDup(err) {
		return nil, kernel.Conflict("WEBHOOK_NAME_TAKEN", "a webhook with this name already exists")
	}
	if err != nil {
		return nil, err
	}
	return &WebhookWithSecret{Webhook: w, Secret: secret}, nil
}

func (s *Service) insertWebhook(ctx context.Context, tx *gorm.DB, a api.Actor, w *Webhook) error {
	if err := lockProject(tx, w.ProjectID); err != nil {
		return err
	}
	n, err := countRows(tx, "webhooks", "project_id = ? AND tenant_id = ?", w.ProjectID, a.TenantID)
	if err != nil {
		return err
	}
	if n >= maxWebhooksPerProject {
		return kernel.Conflict("WEBHOOK_LIMIT", "a project has at most 20 webhooks")
	}
	// created_at comes from the database clock, the same clock that stamps outbox rows, so "events after the webhook" is exact.
	if err := tx.Omit("CreatedAt", "UpdatedAt").Create(w).Error; err != nil {
		return err
	}
	if err := tx.Where("id = ? AND tenant_id = ?", w.ID, a.TenantID).Take(w).Error; err != nil {
		return err
	}
	s.audit(ctx, tx, a, "webhook.created", ref{"webhook", w.ID, map[string]any{"project_id": w.ProjectID, "format": w.Format}})
	return nil
}

// lockProject serialises creations per project so the per-project limits hold under concurrency.
func lockProject(tx *gorm.DB, projectID uuid.UUID) error {
	return tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended(?, 0))`, projectID.String()).Error
}

func (s *Service) loadWebhook(ctx context.Context, db *gorm.DB, a api.Actor, projectID, id uuid.UUID) (Webhook, error) {
	var w Webhook
	err := db.WithContext(ctx).Where("id = ? AND project_id = ? AND tenant_id = ?", id, projectID, a.TenantID).Take(&w).Error
	return w, mapMissing(err, "webhook")
}

// UpdateWebhook edits a webhook (project.update). Setting active clears the automatic disable and the failure count.
func (s *Service) UpdateWebhook(ctx context.Context, a api.Actor, projectID, id uuid.UUID, in WebhookUpdate) (*Webhook, error) {
	if _, err := s.require(ctx, a, projectID, api.ProjectUpdate); err != nil {
		return nil, err
	}
	var w Webhook
	err := s.inTx(ctx, func(tx *gorm.DB) error {
		var err error
		if w, err = s.loadWebhook(ctx, tx, a, projectID, id); err != nil {
			return err
		}
		if err = in.apply(&w); err != nil {
			return err
		}
		w.UpdatedAt = s.stamp()
		if err = tx.Save(&w).Error; err != nil {
			return err
		}
		s.audit(ctx, tx, a, "webhook.updated", ref{"webhook", w.ID, map[string]any{"project_id": projectID, "active": w.Active}})
		return nil
	})
	if isDup(err) {
		return nil, kernel.Conflict("WEBHOOK_NAME_TAKEN", "a webhook with this name already exists")
	}
	if err != nil {
		return nil, err
	}
	return &w, nil
}

// DeleteWebhook removes a webhook and its deliveries (project.update).
func (s *Service) DeleteWebhook(ctx context.Context, a api.Actor, projectID, id uuid.UUID) error {
	if _, err := s.require(ctx, a, projectID, api.ProjectUpdate); err != nil {
		return err
	}
	return s.inTx(ctx, func(tx *gorm.DB) error {
		res := tx.Where("id = ? AND project_id = ? AND tenant_id = ?", id, projectID, a.TenantID).Delete(&Webhook{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return kernel.NotFound("webhook")
		}
		s.audit(ctx, tx, a, "webhook.deleted", ref{"webhook", id, map[string]any{"project_id": projectID}})
		return nil
	})
}

// RotateWebhookSecret replaces the signing secret (project.update) and returns the new one once.
func (s *Service) RotateWebhookSecret(ctx context.Context, a api.Actor, projectID, id uuid.UUID) (*WebhookWithSecret, error) {
	if _, err := s.require(ctx, a, projectID, api.ProjectUpdate); err != nil {
		return nil, err
	}
	secret, sealed, err := s.newSealedSecret()
	if err != nil {
		return nil, err
	}
	var w Webhook
	err = s.inTx(ctx, func(tx *gorm.DB) error {
		var err error
		if w, err = s.loadWebhook(ctx, tx, a, projectID, id); err != nil {
			return err
		}
		w.SecretEnc, w.SecretHint, w.UpdatedAt = sealed, Hint(secret), s.stamp()
		if err = tx.Save(&w).Error; err != nil {
			return err
		}
		s.audit(ctx, tx, a, "webhook.secret_rotated", ref{"webhook", w.ID, map[string]any{"project_id": projectID}})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &WebhookWithSecret{Webhook: w, Secret: secret}, nil
}

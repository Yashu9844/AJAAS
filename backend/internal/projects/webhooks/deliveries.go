package webhooks

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
	"gorm.io/gorm"
)

// pingEvent is the event type of a test delivery; test deliveries are never retried and never count as failures.
const pingEvent = "webhook.test"

// maxPingsPerMinute bounds test pings per webhook, because each one makes an outgoing request.
const maxPingsPerMinute = 10

// DeliveryFilter selects and pages deliveries.
type DeliveryFilter struct {
	Status string
	Cursor string
	Limit  int
}

type deliveryCursor struct {
	At string `json:"at"`
	ID string `json:"id"`
}

// Deliveries lists the deliveries of a webhook, newest first (project.read).
func (s *Service) Deliveries(ctx context.Context, a api.Actor, projectID, webhookID uuid.UUID, f DeliveryFilter) ([]Delivery, string, error) {
	if _, err := s.require(ctx, a, projectID, api.ProjectRead); err != nil {
		return nil, "", err
	}
	if _, err := s.loadWebhook(ctx, s.db, a, projectID, webhookID); err != nil {
		return nil, "", err
	}
	var cur deliveryCursor
	if err := kernel.DecodeCursor(f.Cursor, &cur); err != nil {
		return nil, "", err
	}
	q := s.db.WithContext(ctx).Where("tenant_id = ? AND webhook_id = ?", a.TenantID, webhookID)
	if f.Status != "" {
		if !validStatus(f.Status) {
			return nil, "", kernel.Invalid("status", "must be pending, delivered, failed or dead")
		}
		q = q.Where("status = ?", f.Status)
	}
	if cur.ID != "" {
		q = q.Where("(created_at, id) < (?::timestamptz, ?::uuid)", cur.At, cur.ID)
	}
	var rows []Delivery
	if err := q.Order("created_at DESC, id DESC").Limit(f.Limit + 1).Find(&rows).Error; err != nil {
		return nil, "", err
	}
	rows, more := kernel.Trim(rows, f.Limit)
	next := ""
	if more {
		last := rows[len(rows)-1]
		next = kernel.EncodeCursor(deliveryCursor{At: last.CreatedAt.UTC().Format(time.RFC3339Nano), ID: last.ID.String()})
	}
	return rows, next, nil
}

func validStatus(s string) bool {
	return s == StatusPending || s == StatusDelivered || s == StatusFailed || s == StatusDead
}

// Redeliver queues a settled delivery for another full round of attempts (project.update).
func (s *Service) Redeliver(ctx context.Context, a api.Actor, projectID, webhookID, deliveryID uuid.UUID) (*Delivery, error) {
	if _, err := s.require(ctx, a, projectID, api.ProjectUpdate); err != nil {
		return nil, err
	}
	var d Delivery
	err := s.inTx(ctx, func(tx *gorm.DB) error {
		w, err := s.loadWebhook(ctx, tx, a, projectID, webhookID)
		if err != nil {
			return err
		}
		if !w.Active || w.DisabledAt != nil {
			return kernel.Conflict("WEBHOOK_DISABLED", "the webhook is disabled; enable it first")
		}
		if err = tx.Where("id = ? AND webhook_id = ? AND tenant_id = ?", deliveryID, webhookID, a.TenantID).Take(&d).Error; err != nil {
			return mapMissing(err, "delivery")
		}
		if d.Status == StatusPending {
			return kernel.Conflict("DELIVERY_PENDING", "the delivery is already queued")
		}
		now := s.stamp()
		d.Status, d.Attempts, d.NextAttemptAt, d.ResponseStatus, d.LastError, d.DeliveredAt = StatusPending, 0, &now, nil, "", nil
		if err = tx.Save(&d).Error; err != nil {
			return err
		}
		s.audit(ctx, tx, a, "webhook.redelivered", ref{"webhook_delivery", d.ID, map[string]any{"project_id": projectID, "webhook_id": webhookID}})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// TestWebhook sends a ping now and returns the recorded delivery (project.update). The ping goes through the same
// guard as every delivery, so a destination that is refused or unreachable is recorded as failed.
func (s *Service) TestWebhook(ctx context.Context, a api.Actor, projectID, webhookID uuid.UUID) (*Delivery, error) {
	if _, err := s.require(ctx, a, projectID, api.ProjectUpdate); err != nil {
		return nil, err
	}
	d := Delivery{ID: uuid.New(), TenantID: a.TenantID, WebhookID: webhookID, EventID: uuid.New(), EventType: pingEvent,
		Status: StatusPending, CreatedAt: s.stamp()}
	err := s.inTx(ctx, func(tx *gorm.DB) error {
		if _, err := s.loadWebhook(ctx, tx, a, projectID, webhookID); err != nil {
			return err
		}
		n, err := countRows(tx, "webhook_deliveries", "webhook_id = ? AND tenant_id = ? AND event_type = ? AND created_at > ?",
			webhookID, a.TenantID, pingEvent, d.CreatedAt.Add(-time.Minute))
		if err != nil {
			return err
		}
		if n >= maxPingsPerMinute {
			return kernel.Err(http.StatusTooManyRequests, "RATE_LIMITED", "too many test pings, retry later")
		}
		if err = tx.Create(&d).Error; err != nil {
			return err
		}
		s.audit(ctx, tx, a, "webhook.tested", ref{"webhook", webhookID, map[string]any{"project_id": projectID}})
		return nil
	})
	if err != nil {
		return nil, err
	}
	if err := s.deliver(ctx, claimOf(d)); err != nil {
		return nil, err
	}
	err = s.db.WithContext(ctx).Where("id = ? AND tenant_id = ?", d.ID, a.TenantID).Take(&d).Error
	return &d, err
}

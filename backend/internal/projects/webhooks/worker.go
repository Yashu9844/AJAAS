package webhooks

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Delivery policy: 6 attempts with exponential backoff and jitter, then the delivery is dead. After 20 consecutive
// failed attempts the webhook is disabled.
const (
	maxAttempts   = 6
	claimBatch    = 20
	leaseFor      = 3 * time.Minute
	sendWorkers   = 5
	jitterSpread  = 0.4 // the delay is scaled by 0.8 .. 1.2
	jitterFloor   = 0.8
	retentionDays = 30
)

// backoffSchedule is the wait after attempt 1..5.
var backoffSchedule = []time.Duration{time.Minute, 5 * time.Minute, 30 * time.Minute, 2 * time.Hour, 12 * time.Hour}

// backoff is the wait after the given failed attempt (1-based); jitter is a number in [0,1).
func backoff(attempt int, jitter float64) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if attempt > len(backoffSchedule) {
		attempt = len(backoffSchedule)
	}
	return time.Duration(float64(backoffSchedule[attempt-1]) * (jitterFloor + jitterSpread*jitter))
}

// claim is a delivery leased to this worker.
type claim struct {
	ID        uuid.UUID
	TenantID  uuid.UUID
	WebhookID uuid.UUID
	EventID   uuid.UUID
	EventType string
	Attempts  int
}

func claimOf(d Delivery) claim {
	return claim{ID: d.ID, TenantID: d.TenantID, WebhookID: d.WebhookID, EventID: d.EventID, EventType: d.EventType, Attempts: d.Attempts}
}

// claimDue leases due deliveries of enabled webhooks by pushing their next attempt out; a crashed worker's lease simply
// expires and the delivery is picked up again.
func (s *Service) claimDue(ctx context.Context, limit int) ([]claim, error) {
	now := s.stamp()
	var out []claim
	err := s.db.WithContext(ctx).Raw(`UPDATE webhook_deliveries d SET next_attempt_at = ?
		WHERE d.id IN (SELECT d2.id FROM webhook_deliveries d2
		    JOIN webhooks w ON w.id = d2.webhook_id AND w.tenant_id = d2.tenant_id
		  WHERE d2.status IN ('pending', 'failed') AND d2.next_attempt_at <= ? AND w.active AND w.disabled_at IS NULL
		  ORDER BY d2.next_attempt_at LIMIT ? FOR UPDATE OF d2 SKIP LOCKED)
		RETURNING d.id, d.tenant_id, d.webhook_id, d.event_id, d.event_type, d.attempts`, now.Add(leaseFor), now, limit).Scan(&out).Error
	return out, err
}

// SendOnce sends the deliveries that are due and returns how many it handled.
func (s *Service) SendOnce(ctx context.Context) (int, error) {
	cs, err := s.claimDue(ctx, claimBatch)
	if err != nil {
		return 0, err
	}
	var wg sync.WaitGroup
	errs := make(chan error, len(cs))
	sem := make(chan struct{}, sendWorkers)
	for _, c := range cs {
		wg.Add(1)
		sem <- struct{}{}
		go func(c claim) {
			defer wg.Done()
			defer func() { <-sem }()
			if err := s.deliver(ctx, c); err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				errs <- err
			}
		}(c)
	}
	wg.Wait()
	close(errs)
	return len(cs), <-errs
}

// deliver sends one claimed delivery (no transaction is open while the network is used) and records the result.
func (s *Service) deliver(ctx context.Context, c claim) error {
	res, err := s.sendOne(ctx, c)
	if err != nil {
		return err
	}
	return s.settle(ctx, c, res)
}

func (s *Service) sendOne(ctx context.Context, c claim) (Result, error) {
	var w Webhook
	if err := s.db.WithContext(ctx).Where("id = ? AND tenant_id = ?", c.WebhookID, c.TenantID).Take(&w).Error; err != nil {
		return Result{}, err
	}
	secret, ok := s.openSecret(w.SecretEnc)
	if !ok {
		return Result{Err: errSecret}, nil
	}
	env := s.pingEnvelope(c, w.ProjectID)
	if c.EventType != pingEvent {
		var err error
		if env, err = s.loadEnvelope(ctx, c.TenantID, c.EventID); errors.Is(err, errNoEnvelope) {
			return Result{Err: errNoEvent}, nil
		} else if err != nil {
			return Result{}, err
		}
	}
	body, err := s.body(ctx, w, env, c.ID)
	if err != nil {
		return Result{}, err
	}
	return s.sender.Do(ctx, Request{URL: w.URL, Body: body, Headers: signedHeaders(signing{secret: secret, at: s.now(), event: c.EventType, delivery: c.ID}, body)}), nil
}

// outcome decides the delivery state after an attempt.
func (s *Service) outcome(c claim, res Result, now time.Time) (status string, next, delivered *time.Time) {
	attempts := c.Attempts + 1
	switch {
	case res.OK():
		return StatusDelivered, nil, &now
	case c.EventType == pingEvent:
		return StatusFailed, nil, nil
	case attempts >= maxAttempts:
		return StatusDead, nil, nil
	}
	at := now.Add(backoff(attempts, s.jitter()))
	return StatusFailed, &at, nil
}

// settle stores the attempt result and updates the webhook failure counter in one short transaction.
func (s *Service) settle(ctx context.Context, c claim, res Result) error {
	now := s.stamp()
	status, next, delivered := s.outcome(c, res, now)
	var code *int
	if res.Status != 0 {
		code = &res.Status
	}
	return s.inTx(ctx, func(tx *gorm.DB) error {
		err := tx.Exec(`UPDATE webhook_deliveries SET status = ?, attempts = ?, next_attempt_at = ?, response_status = ?, last_error = ?, delivered_at = ?
			WHERE id = ? AND tenant_id = ?`, status, c.Attempts+1, next, code, res.Err, delivered, c.ID, c.TenantID).Error
		if err != nil || c.EventType == pingEvent {
			return err
		}
		return s.countOutcome(ctx, tx, c, res.OK(), now)
	})
}

// countOutcome resets the failure counter on success, or counts the failure and disables the webhook at the threshold.
func (s *Service) countOutcome(ctx context.Context, tx *gorm.DB, c claim, ok bool, now time.Time) error {
	if ok {
		return tx.Exec(`UPDATE webhooks SET failure_count = 0 WHERE id = ? AND tenant_id = ? AND failure_count <> 0`, c.WebhookID, c.TenantID).Error
	}
	var r struct {
		FailureCount int
		JustDisabled bool
	}
	err := tx.Raw(`UPDATE webhooks SET failure_count = failure_count + 1,
		disabled_at = CASE WHEN disabled_at IS NULL AND failure_count + 1 >= ? THEN ?::timestamptz ELSE disabled_at END
		WHERE id = ? AND tenant_id = ? RETURNING failure_count, (disabled_at = ?::timestamptz) AS just_disabled`,
		disableAfterFailures, now, c.WebhookID, c.TenantID, now).Scan(&r).Error
	if err == nil && r.JustDisabled {
		s.auditSystem(ctx, tx, c.TenantID, "webhook.auto_disabled", ref{"webhook", c.WebhookID, map[string]any{"failures": r.FailureCount}})
	}
	return err
}

// auditSystem records an action no person took.
func (s *Service) auditSystem(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, action string, r ref) {
	if s.p.Audit != nil {
		_ = s.p.Audit.Log(ctx, tx, tenantID.String(), "", action, r.resource, r.id.String(), r.meta, "", "")
	}
}

// Purge deletes deliveries older than the retention window.
func (s *Service) Purge(ctx context.Context) error {
	cutoff := s.stamp().AddDate(0, 0, -retentionDays)
	return s.db.WithContext(ctx).Exec(`DELETE FROM webhook_deliveries WHERE created_at < ?`, cutoff).Error
}

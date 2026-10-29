package webhooks

import (
	"context"
	"encoding/json"
	"regexp"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/kernel"
)

var (
	eventNameOK = regexp.MustCompile(`^[a-z][a-z0-9_.]{0,99}$`)
	keyOK       = regexp.MustCompile(`^[A-Z0-9]{1,10}$`)
)

// loadEnvelope reads the stored event envelope (ids and versions only; an AI agent label may ride along).
func (s *Service) loadEnvelope(ctx context.Context, tenantID, eventID uuid.UUID) (map[string]any, error) {
	var raws []string
	err := s.db.WithContext(ctx).Raw(`SELECT payload::text FROM `+kernel.OutboxTable+` WHERE tenant_id = ? AND event_id = ?`, tenantID, eventID).Scan(&raws).Error
	if err != nil {
		return nil, err
	}
	if len(raws) == 0 {
		return nil, errNoEnvelope
	}
	env := map[string]any{}
	if err := json.Unmarshal([]byte(raws[0]), &env); err != nil {
		return nil, errNoEnvelope
	}
	return env, nil
}

type envelopeError string

// Error implements error.
func (e envelopeError) Error() string { return string(e) }

const errNoEnvelope = envelopeError("event envelope unavailable")

// pingEnvelope is the synthetic event of a test delivery.
func (s *Service) pingEnvelope(c claim, projectID uuid.UUID) map[string]any {
	return map[string]any{"event_id": c.EventID, "event_type": pingEvent, "routing_key": pingEvent, "event_version": 1,
		"tenant_id": c.TenantID, "occurred_at": s.stamp(), "producer": kernel.Producer,
		"payload": map[string]any{"project_id": projectID, "webhook_id": c.WebhookID}}
}

// body renders the request body for the webhook format. Generic sends the envelope plus the delivery id; slack and
// discord send one short line built only from the event name, the project key and the ticket key.
func (s *Service) body(ctx context.Context, w Webhook, env map[string]any, deliveryID uuid.UUID) ([]byte, error) {
	if w.Format == FormatGeneric {
		out := make(map[string]any, len(env)+1)
		for k, v := range env {
			out[k] = v
		}
		out["delivery_id"] = deliveryID
		return json.Marshal(out)
	}
	text, err := s.chatText(ctx, w, env)
	if err != nil {
		return nil, err
	}
	if w.Format == FormatDiscord {
		return json.Marshal(map[string]string{"content": text})
	}
	return json.Marshal(map[string]string{"text": text})
}

// chatText is "[JAAS] <event> in <KEY>" with " (<KEY>-<n>)" when the event names a task. No titles or other free text.
func (s *Service) chatText(ctx context.Context, w Webhook, env map[string]any) (string, error) {
	event, _ := env["event_type"].(string)
	if !eventNameOK.MatchString(event) {
		event = "event"
	}
	text := "[JAAS] " + event
	var keys []string
	if err := s.db.WithContext(ctx).Raw(`SELECT key FROM projects WHERE id = ? AND tenant_id = ?`, w.ProjectID, w.TenantID).Scan(&keys).Error; err != nil {
		return "", err
	}
	if len(keys) == 0 || !keyOK.MatchString(keys[0]) {
		return text, nil
	}
	text += " in " + keys[0]
	payload, _ := env["payload"].(map[string]any)
	tid, _ := payload["task_id"].(string)
	taskID, err := uuid.Parse(tid)
	if err != nil {
		return text, nil
	}
	var numbers []int
	err = s.db.WithContext(ctx).Raw(`SELECT number FROM tasks WHERE id = ? AND project_id = ? AND tenant_id = ?`, taskID, w.ProjectID, w.TenantID).Scan(&numbers).Error
	if err == nil && len(numbers) > 0 {
		text += " (" + keys[0] + "-" + strconv.Itoa(numbers[0]) + ")"
	}
	return text, err
}

// signing is what a delivery signature covers: the secret, the time, the event and the delivery id.
type signing struct {
	secret   []byte
	at       time.Time
	event    string
	delivery uuid.UUID
}

// signedHeaders are the headers every delivery carries.
func signedHeaders(g signing, body []byte) map[string]string {
	return map[string]string{"X-JAAS-Event": g.event, "X-JAAS-Delivery": g.delivery.String(), "X-JAAS-Signature": Sign(g.secret, g.at.Unix(), body)}
}

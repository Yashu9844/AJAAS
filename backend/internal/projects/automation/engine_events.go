package automation

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
)

// outboxRow is one M2 event as stored in projects_events_outbox.
type outboxRow struct {
	ID         uuid.UUID
	TenantID   uuid.UUID
	EventID    uuid.UUID
	RoutingKey string
	Payload    string
	CreatedAt  time.Time
}

// envelope is the part of the event envelope the engine reads (ids only; see kernel.Emit).
type envelope struct {
	Agent   string `json:"agent"`
	Payload struct {
		TaskID       uuid.UUID   `json:"task_id"`
		ProjectID    uuid.UUID   `json:"project_id"`
		FromColumnID *uuid.UUID  `json:"from_column_id"`
		ToColumnID   *uuid.UUID  `json:"to_column_id"`
		UserIDs      []uuid.UUID `json:"user_ids"`
	} `json:"payload"`
}

// events reads the next batch after pos that is older than the horizon and that some rule can react to.
func (e *Engine) events(ctx context.Context, pos position, horizon time.Time) ([]outboxRow, error) {
	var rows []outboxRow
	err := e.s.db.WithContext(ctx).Raw(`SELECT id, tenant_id, event_id, routing_key, payload::text AS payload, created_at FROM projects_events_outbox
		WHERE (created_at, id) > (?, ?) AND created_at < ? AND routing_key IN ? ORDER BY created_at, id LIMIT ?`,
		pos.At, pos.ID, horizon, Triggers, e.batch).Scan(&rows).Error
	return rows, err
}

// handleAll applies the rules to each event in order and returns how many events were handled before a failure.
func (e *Engine) handleAll(ctx context.Context, rows []outboxRow) (int, error) {
	for i, row := range rows {
		if err := e.handle(ctx, row); err != nil {
			return i, err
		}
	}
	return len(rows), nil
}

// handle runs every active rule of the event project and trigger. Events caused by a rule carry an agent label that
// starts with "automation" and are never handled: rules do not cascade (loop protection).
func (e *Engine) handle(ctx context.Context, row outboxRow) error {
	var env envelope
	if err := json.Unmarshal([]byte(row.Payload), &env); err != nil {
		return nil // a malformed envelope can never match; skipping it keeps the cursor moving
	}
	if strings.HasPrefix(env.Agent, AgentPrefix) || env.Payload.TaskID == uuid.Nil || env.Payload.ProjectID == uuid.Nil {
		return nil
	}
	var rules []Rule
	err := e.s.db.WithContext(ctx).Raw(`SELECT r.* FROM workflow_rules r JOIN projects p ON p.id = r.project_id
		WHERE r.tenant_id = ? AND r.project_id = ? AND r.trigger = ? AND r.active AND p.deleted_at IS NULL AND p.status <> 'archived'
		ORDER BY r.created_at, r.id`, row.TenantID, env.Payload.ProjectID, row.RoutingKey).Scan(&rules).Error
	if err != nil {
		return err
	}
	ev := event{row: row, task: env.Payload.TaskID, facts: eventFacts{fromColumn: env.Payload.FromColumnID, toColumn: env.Payload.ToColumnID, users: env.Payload.UserIDs}}
	for _, r := range rules {
		if err := e.apply(ctx, r, ev); err != nil {
			return err
		}
	}
	return nil
}

// event is one triggering event as the rules see it.
type event struct {
	row   outboxRow
	task  uuid.UUID
	facts eventFacts
}

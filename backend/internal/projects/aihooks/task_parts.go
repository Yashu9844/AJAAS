package aihooks

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
)

type commentRow struct {
	Author  string    `json:"author_user_id"`
	At      time.Time `json:"created_at"`
	Excerpt string    `json:"excerpt"`
}

type activityRow struct {
	Type   string    `json:"type"`
	Source string    `json:"source"`
	Actor  *string   `json:"actor_user_id,omitempty"`
	At     time.Time `json:"at"`
}

type fieldRow struct {
	Key   string `json:"key"`
	Type  string `json:"type"`
	Value string `json:"value"`
}

// taskParts adds the related sections to the pack: people, labels, subtasks, relations, comments, activity, fields, time.
func (s *Service) taskParts(ctx context.Context, a api.Actor, t api.TaskRef, data map[string]any, cut *[]string) error {
	db := s.db.WithContext(ctx)
	var assignees, labels []string
	if err := db.Raw(`SELECT user_id::text FROM task_assignees WHERE task_id = ? AND active`, t.ID).Scan(&assignees).Error; err != nil {
		return err
	}
	if err := db.Raw(`SELECT l.name FROM task_labels tl JOIN labels l ON l.id = tl.label_id WHERE tl.task_id = ? ORDER BY lower(l.name)`, t.ID).Scan(&labels).Error; err != nil {
		return err
	}
	data["assignee_ids"], data["labels"] = assignees, labels
	var subs []miniTask
	if err := db.Raw(`SELECT p.key || '-' || s.number AS key, s.title, c.category AS category FROM tasks s JOIN projects p ON p.id = s.project_id
		JOIN board_columns c ON c.id = s.column_id WHERE s.parent_task_id = ? AND s.tenant_id = ? AND s.deleted_at IS NULL ORDER BY s.number LIMIT ?`,
		t.ID, a.TenantID, capSubtasks+1).Scan(&subs).Error; err != nil {
		return err
	}
	data["subtasks"] = capList(subs, capSubtasks, "subtasks", cut)
	rels, err := s.relations(ctx, a, t.ID)
	if err != nil {
		return err
	}
	data["relations"] = capList(rels, capRelations, "relations", cut)
	return s.taskHistory(ctx, a, t.ID, data)
}

func (s *Service) relations(ctx context.Context, a api.Actor, id uuid.UUID) ([]relationRow, error) {
	var rows []relationRow
	err := s.db.WithContext(ctx).Raw(`SELECT r.type, CASE WHEN r.source_task_id = @id THEN 'outgoing' ELSE 'incoming' END AS direction,
		p.key || '-' || o.number AS key, o.title, c.category AS category, o.project_id::text AS project_id
		FROM task_relations r JOIN tasks o ON o.id = CASE WHEN r.source_task_id = @id THEN r.target_task_id ELSE r.source_task_id END
		JOIN projects p ON p.id = o.project_id JOIN board_columns c ON c.id = o.column_id
		WHERE (r.source_task_id = @id OR r.target_task_id = @id) AND r.tenant_id = @tenant AND o.deleted_at IS NULL LIMIT @n`,
		map[string]any{"id": id, "tenant": a.TenantID, "n": relationQuery}).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	readable, err := s.authz.ReadableProjectIDs(ctx, nil, a)
	if err != nil {
		return nil, err
	}
	ok := map[string]bool{}
	for _, p := range readable {
		ok[p.String()] = true
	}
	out := rows[:0]
	for _, r := range rows {
		if ok[r.ProjectID] { // a relation to a project the caller cannot read is invisible to them (and so to their AI)
			out = append(out, r)
		}
	}
	return out, nil
}

func (s *Service) taskHistory(ctx context.Context, a api.Actor, id uuid.UUID, data map[string]any) error {
	db := s.db.WithContext(ctx)
	var comments []commentRow
	if err := db.Raw(`SELECT author_user_id::text AS author, created_at AS at, left(body, ?) AS excerpt FROM comments
		WHERE task_id = ? AND tenant_id = ? AND deleted_at IS NULL ORDER BY created_at DESC LIMIT ?`, commentMax, id, a.TenantID, capComments).Scan(&comments).Error; err != nil {
		return err
	}
	var acts []activityRow
	if err := db.Raw(`SELECT type, source, actor_user_id::text AS actor, occurred_at AS at FROM activity WHERE task_id = ? AND tenant_id = ?
		ORDER BY seq DESC LIMIT ?`, id, a.TenantID, capActivity).Scan(&acts).Error; err != nil {
		return err
	}
	var fields []fieldRow
	if err := db.Raw(`SELECT d.key, d.type, v.value::text AS value FROM custom_field_values v JOIN custom_field_definitions d ON d.id = v.field_id
		WHERE v.task_id = ? ORDER BY d.position`, id).Scan(&fields).Error; err != nil {
		return err
	}
	var minutes int64
	if err := db.Raw(`SELECT COALESCE(sum(minutes), 0) FROM time_entries WHERE task_id = ? AND tenant_id = ?`, id, a.TenantID).Scan(&minutes).Error; err != nil {
		return err
	}
	data["latest_comments"], data["recent_activity"], data["custom_fields"], data["logged_minutes"] = comments, acts, fields, minutes
	return nil
}

package task

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
	"gorm.io/gorm"
)

// addAssignees inserts the assignees that are not yet on the task and returns the new ones.
func addAssignees(tx *gorm.DB, a api.Actor, taskID uuid.UUID, ids []uuid.UUID) ([]uuid.UUID, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	args := make([]any, 0, len(ids)*4)
	for _, u := range ids {
		args = append(args, taskID, u, a.TenantID, a.UserID)
	}
	var added []uuid.UUID
	err := tx.Raw(`INSERT INTO task_assignees (task_id, user_id, tenant_id, assigned_by) VALUES `+
		strings.TrimSuffix(strings.Repeat("(?, ?, ?, ?),", len(ids)), ",")+` ON CONFLICT (task_id, user_id) DO UPDATE SET active = TRUE, assigned_by = EXCLUDED.assigned_by, assigned_at = now()
		WHERE task_assignees.active = FALSE RETURNING user_id`, args...).Scan(&added).Error
	return added, err
}

// addLabels attaches the labels that are not yet on the task and returns the new ones.
func addLabels(tx *gorm.DB, tenantID, taskID uuid.UUID, ids []uuid.UUID) ([]uuid.UUID, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	args := make([]any, 0, len(ids)*3)
	for _, l := range ids {
		args = append(args, taskID, l, tenantID)
	}
	var added []uuid.UUID
	err := tx.Raw(`INSERT INTO task_labels (task_id, label_id, tenant_id) VALUES `+
		strings.TrimSuffix(strings.Repeat("(?, ?, ?),", len(ids)), ",")+` ON CONFLICT DO NOTHING RETURNING label_id`, args...).Scan(&added).Error
	return added, err
}

// assign adds assignees who are project members (task.assign).
func (m *mut) assign(ids []uuid.UUID) error {
	ids, err := uniqueIDs(ids, maxAssignees, "user_ids")
	if err != nil {
		return err
	}
	if err := m.acc.Require(api.TaskAssign); err != nil {
		return err
	}
	if err := checkAssignees(m.tx, m.a.TenantID, m.t.ProjectID, ids); err != nil {
		return err
	}
	added, err := addAssignees(m.tx, m.a, m.t.ID, ids)
	if err != nil || len(added) == 0 {
		return err
	}
	var total int64
	m.tx.Raw(`SELECT count(*) FROM task_assignees WHERE task_id = ? AND active`, m.t.ID).Scan(&total)
	if total > maxAssignees {
		return kernel.Invalid("user_ids", "a task has at most 20 assignees")
	}
	m.touch()
	m.add(note{event: "task.assigned", activity: "assigned", data: map[string]any{"user_ids": added}, payload: map[string]any{"user_ids": added}})
	return m.s.watch(m.ctx, m.tx, api.WatchRequest{TenantID: m.a.TenantID, TaskID: m.t.ID, Reason: "assignee", UserIDs: added})
}

// unassign removes assignees (task.assign).
func (m *mut) unassign(ids []uuid.UUID) error {
	if err := m.acc.Require(api.TaskAssign); err != nil {
		return err
	}
	var removed []uuid.UUID
	if len(ids) == 0 {
		return nil
	}
	err := m.tx.Raw(`DELETE FROM task_assignees WHERE task_id = ? AND tenant_id = ? AND user_id IN ? RETURNING user_id`,
		m.t.ID, m.a.TenantID, ids).Scan(&removed).Error
	if err != nil || len(removed) == 0 {
		return err
	}
	m.touch()
	m.add(note{event: "task.unassigned", activity: "unassigned", data: map[string]any{"user_ids": removed}, payload: map[string]any{"user_ids": removed}})
	return nil
}

// attachLabels adds labels of the tenant or project (task.update).
func (m *mut) attachLabels(ids []uuid.UUID) error {
	ids, err := uniqueIDs(ids, maxLabels, "label_ids")
	if err != nil {
		return err
	}
	if err := checkLabels(m.tx, m.a.TenantID, m.t.ProjectID, ids); err != nil {
		return err
	}
	added, err := addLabels(m.tx, m.a.TenantID, m.t.ID, ids)
	if err != nil || len(added) == 0 {
		return err
	}
	m.touch()
	m.add(note{event: "task.updated", activity: "labels_added", data: map[string]any{"label_ids": added}, payload: map[string]any{"label_ids": added}})
	return nil
}

// detachLabels removes labels from the task.
func (m *mut) detachLabels(ids []uuid.UUID) error {
	if len(ids) == 0 {
		return nil
	}
	var removed []uuid.UUID
	err := m.tx.Raw(`DELETE FROM task_labels WHERE task_id = ? AND tenant_id = ? AND label_id IN ? RETURNING label_id`,
		m.t.ID, m.a.TenantID, ids).Scan(&removed).Error
	if err != nil || len(removed) == 0 {
		return err
	}
	m.touch()
	m.add(note{event: "task.updated", activity: "labels_removed", data: map[string]any{"label_ids": removed}, payload: map[string]any{"label_ids": removed}})
	return nil
}

// AddAssignees assigns users to a task (task.assign).
func (s *Service) AddAssignees(ctx context.Context, a api.Actor, id uuid.UUID, in UserIDsInput) (*View, error) {
	return s.mutate(ctx, a, id, mutSpec{Perm: api.TaskAssign, NoColumnLock: true}, func(m *mut) error { return m.assign(in.UserIDs) })
}

// RemoveAssignee removes one assignee (task.assign).
func (s *Service) RemoveAssignee(ctx context.Context, a api.Actor, id, userID uuid.UUID) (*View, error) {
	return s.mutate(ctx, a, id, mutSpec{Perm: api.TaskAssign, NoColumnLock: true}, func(m *mut) error { return m.unassign([]uuid.UUID{userID}) })
}

// AddLabels labels a task (task.update).
func (s *Service) AddLabels(ctx context.Context, a api.Actor, id uuid.UUID, in LabelIDsInput) (*View, error) {
	return s.mutate(ctx, a, id, mutSpec{Perm: api.TaskUpdate, NoColumnLock: true}, func(m *mut) error { return m.attachLabels(in.LabelIDs) })
}

// RemoveLabel removes one label from a task (task.update).
func (s *Service) RemoveLabel(ctx context.Context, a api.Actor, id, labelID uuid.UUID) (*View, error) {
	return s.mutate(ctx, a, id, mutSpec{Perm: api.TaskUpdate, NoColumnLock: true}, func(m *mut) error { return m.detachLabels([]uuid.UUID{labelID}) })
}

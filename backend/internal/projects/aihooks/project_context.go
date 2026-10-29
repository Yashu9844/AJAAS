package aihooks

import (
	"context"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
)

// Caps keep a context pack small; anything cut is reported in Pack.Truncated.
const (
	capColumns    = 20
	capMilestones = 5
	capLabels     = 30
	capFields     = 30
)

type columnRow struct {
	Name     string `json:"name"`
	Category string `json:"category"`
	WipLimit *int   `json:"wip_limit,omitempty"`
	Tasks    int    `json:"tasks"`
}

type countsRow struct {
	Open       int `json:"open"`
	Overdue    int `json:"overdue"`
	Blocked    int `json:"blocked"`
	Unassigned int `json:"unassigned"`
}

// ProjectContext returns a compact overview of a project for a model: shape, workload, planning and the caller permissions.
func (s *Service) ProjectContext(ctx context.Context, a api.Actor, projectID uuid.UUID) (Pack, error) {
	acc, err := s.authz.Access(ctx, nil, a, projectID)
	if err != nil {
		return Pack{}, err
	}
	if err := acc.Require(api.ProjectRead); err != nil {
		return Pack{}, err
	}
	var cut []string
	cols, err := s.columns(ctx, a, projectID)
	if err != nil {
		return Pack{}, err
	}
	if len(cols) > capColumns {
		cols, cut = cols[:capColumns], append(cut, "columns")
	}
	counts, err := s.counts(ctx, a, projectID)
	if err != nil {
		return Pack{}, err
	}
	plan, err := s.planning(ctx, a, projectID)
	if err != nil {
		return Pack{}, err
	}
	setup, err := s.setup(ctx, a, projectID, &cut)
	if err != nil {
		return Pack{}, err
	}
	data := map[string]any{
		"project":     map[string]any{"id": acc.Project.ID, "key": acc.Project.Key, "name": acc.Project.Name, "status": acc.Project.Status, "mode": acc.Project.Mode, "my_role": acc.Role},
		"permissions": api.SortedPerms(acc.Perms), "columns": cols, "counts": counts, "planning": plan, "setup": setup,
	}
	return s.seal("project", a, data, cut), nil
}

func (s *Service) columns(ctx context.Context, a api.Actor, pid uuid.UUID) ([]columnRow, error) {
	var rows []columnRow
	err := s.db.WithContext(ctx).Raw(`SELECT c.name, c.category, c.wip_limit, count(t.id) AS tasks FROM board_columns c
		LEFT JOIN tasks t ON t.column_id = c.id AND t.deleted_at IS NULL AND t.archived_at IS NULL
		WHERE c.project_id = ? AND c.tenant_id = ? GROUP BY c.id ORDER BY c.position`, pid, a.TenantID).Scan(&rows).Error
	return rows, err
}

func (s *Service) counts(ctx context.Context, a api.Actor, pid uuid.UUID) (countsRow, error) {
	var c countsRow
	err := s.db.WithContext(ctx).Raw(`SELECT
		count(*) FILTER (WHERE col.category NOT IN ('done','canceled')) AS open,
		count(*) FILTER (WHERE col.category NOT IN ('done','canceled') AND t.due_at < now()) AS overdue,
		count(*) FILTER (WHERE col.category NOT IN ('done','canceled') AND EXISTS (
			SELECT 1 FROM task_relations r JOIN tasks b ON b.id = r.source_task_id JOIN board_columns bc ON bc.id = b.column_id
			WHERE r.target_task_id = t.id AND r.type = 'blocks' AND b.deleted_at IS NULL AND bc.category NOT IN ('done','canceled'))) AS blocked,
		count(*) FILTER (WHERE col.category NOT IN ('done','canceled') AND NOT EXISTS (
			SELECT 1 FROM task_assignees ta WHERE ta.task_id = t.id AND ta.active)) AS unassigned
		FROM tasks t JOIN board_columns col ON col.id = t.column_id
		WHERE t.project_id = ? AND t.tenant_id = ? AND t.deleted_at IS NULL AND t.archived_at IS NULL`, pid, a.TenantID).Scan(&c).Error
	return c, err
}

func (s *Service) planning(ctx context.Context, a api.Actor, pid uuid.UUID) (map[string]any, error) {
	var ms []struct {
		Name  string       `json:"name"`
		DueOn *kernel.Date `json:"due_on,omitempty"`
		Total int          `json:"total"`
		Done  int          `json:"done"`
	}
	err := s.db.WithContext(ctx).Raw(`SELECT m.name, m.due_on,
		(SELECT count(*) FROM tasks t WHERE t.milestone_id = m.id AND t.deleted_at IS NULL) AS total,
		(SELECT count(*) FROM tasks t JOIN board_columns c ON c.id = t.column_id WHERE t.milestone_id = m.id AND t.deleted_at IS NULL AND c.category = 'done') AS done
		FROM milestones m WHERE m.project_id = ? AND m.tenant_id = ? AND m.status = 'open' ORDER BY m.due_on NULLS LAST LIMIT ?`, pid, a.TenantID, capMilestones).Scan(&ms).Error
	if err != nil {
		return nil, err
	}
	var sprint []struct {
		Name  string       `json:"name"`
		EndOn *kernel.Date `json:"end_on,omitempty"`
	}
	if err := s.db.WithContext(ctx).Raw(`SELECT name, end_on FROM sprints WHERE project_id = ? AND tenant_id = ? AND status = 'active' LIMIT 1`, pid, a.TenantID).Scan(&sprint).Error; err != nil {
		return nil, err
	}
	out := map[string]any{"open_milestones": ms}
	if len(sprint) == 1 {
		out["active_sprint"] = sprint[0]
	}
	return out, nil
}

func (s *Service) setup(ctx context.Context, a api.Actor, pid uuid.UUID, cut *[]string) (map[string]any, error) {
	var labels []string
	if err := s.db.WithContext(ctx).Raw(`SELECT name FROM labels WHERE tenant_id = ? AND (project_id = ? OR project_id IS NULL) ORDER BY lower(name) LIMIT ?`,
		a.TenantID, pid, capLabels+1).Scan(&labels).Error; err != nil {
		return nil, err
	}
	if len(labels) > capLabels {
		labels, *cut = labels[:capLabels], append(*cut, "labels")
	}
	var fields []struct {
		Key     string            `json:"key"`
		Name    string            `json:"name"`
		Type    string            `json:"type"`
		Options kernel.StringList `json:"options,omitempty"`
	}
	if err := s.db.WithContext(ctx).Raw(`SELECT key, name, type, options FROM custom_field_definitions WHERE project_id = ? AND tenant_id = ? ORDER BY position LIMIT ?`,
		pid, a.TenantID, capFields).Scan(&fields).Error; err != nil {
		return nil, err
	}
	var members int64
	if err := s.db.WithContext(ctx).Raw(`SELECT count(*) FROM project_members WHERE project_id = ? AND tenant_id = ? AND active`, pid, a.TenantID).Scan(&members).Error; err != nil {
		return nil, err
	}
	return map[string]any{"labels": labels, "custom_fields": fields, "members": members}, nil
}

package task

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
	"gorm.io/gorm"
)

// projectKey names a project inside its tenant.
type projectKey struct{ tenantID, projectID uuid.UUID }

// pickColumn chooses the column a moved task lands in: the requested one, the one with the same key, the first one of
// the same category, or the default column of the target project.
func pickColumn(db *gorm.DB, target projectKey, want *uuid.UUID, from *colRow) (*colRow, error) {
	tenantID, targetProject := target.tenantID, target.projectID
	if want != nil {
		return column(db, tenantID, targetProject, *want)
	}
	var c colRow
	err := db.Where("tenant_id = ? AND project_id = ? AND key = ?", tenantID, targetProject, from.Key).Take(&c).Error
	if err == nil {
		return &c, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	err = db.Where("tenant_id = ? AND project_id = ? AND category = ?", tenantID, targetProject, from.Category).Order("position").Take(&c).Error
	if err == nil {
		return &c, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	return defaultColumn(db, tenantID, targetProject)
}

// MoveProject moves a task to another project (TK-08): a new number there, columns, labels and milestone mapped or
// cleared, the sprint cleared, and an alias so the old ticket id keeps resolving.
func (s *Service) MoveProject(ctx context.Context, a api.Actor, id uuid.UUID, in MoveProjectInput) (*View, error) {
	src, acc, err := s.locate(ctx, a, id)
	if err != nil {
		return nil, err
	}
	if err := acc.Require(api.TaskUpdate); err != nil {
		return nil, err
	}
	if in.ProjectID == src.ProjectID {
		return nil, kernel.Invalid("project_id", "the task is already in this project")
	}
	dstAcc, err := s.access(ctx, a, in.ProjectID)
	if err != nil {
		return nil, kernel.NotFound("target project")
	}
	if err := dstAcc.Require(api.TaskCreate); err != nil {
		return nil, err
	}
	from, err := column(s.db, a.TenantID, src.ProjectID, src.ColumnID)
	if err != nil {
		return nil, err
	}
	to, err := pickColumn(s.db, projectKey{a.TenantID, in.ProjectID}, in.ColumnID, from)
	if err != nil {
		return nil, err
	}
	spec := mutSpec{Perm: api.TaskUpdate, Version: in.Version, LockColumn: func(*Task) uuid.UUID { return to.ID }}
	return s.mutate(ctx, a, id, spec, func(m *mut) error {
		return m.moveProject(moveTarget{col: to, from: from, key: acc.Project.Key, project: in.ProjectID})
	})
}

type moveTarget struct {
	col     *colRow
	from    *colRow
	key     string
	project uuid.UUID
}

func (m *mut) moveProject(tg moveTarget) error {
	if m.t.ParentTaskID != nil {
		return kernel.Conflict("TASK_HAS_HIERARCHY", "a subtask cannot move to another project; detach it first")
	}
	n, err := count(m.tx, `SELECT count(*) FROM tasks WHERE parent_task_id = ? AND tenant_id = ? AND deleted_at IS NULL`, m.t.ID, m.a.TenantID)
	if err != nil {
		return err
	}
	if n > 0 {
		return kernel.Conflict("TASK_HAS_HIERARCHY", "a task with subtasks cannot move to another project")
	}
	if tg.col.Category == api.ColumnDone {
		if err := m.doneRules(false); err != nil {
			return err
		}
	}
	warns, err := wipCheck(m.tx, m.a.TenantID, tg.col, m.t.ID)
	if err != nil {
		return err
	}
	m.warn = append(m.warn, warns...)
	number, err := nextNumber(m.tx, m.a.TenantID, tg.project)
	if err != nil {
		return err
	}
	rank, err := m.s.allocRank(m.tx, colPlace{m.a.TenantID, tg.col.ID, m.t.ID}, slot{})
	if err != nil {
		return err
	}
	if err := m.remapLinks(tg.project); err != nil {
		return err
	}
	return m.applyMove(tg, number, rank)
}

func (m *mut) applyMove(tg moveTarget, number int, rank string) error {
	err := m.tx.Exec(`INSERT INTO task_ticket_alias (tenant_id, old_key, old_number, task_id) VALUES (?, ?, ?, ?) ON CONFLICT DO NOTHING`,
		m.a.TenantID, tg.key, m.t.Number, m.t.ID).Error
	if err != nil {
		return err
	}
	ids := map[string]any{"from_project_id": m.t.ProjectID, "to_project_id": tg.project, "number": number}
	if err := m.s.record(m.unit(), m.t, note{activity: "moved_out", data: ids}); err != nil {
		return err
	}
	m.change("project_id", tg.project)
	m.change("number", number)
	m.change("column_id", tg.col.ID)
	m.change("rank", rank)
	m.change("sprint_id", nil)
	switch {
	case tg.col.Category == api.ColumnDone && m.t.CompletedAt == nil:
		now := m.s.now().UTC()
		m.change("completed_at", &now)
	case tg.col.Category != api.ColumnDone:
		m.change("completed_at", nil)
	}
	m.t.ProjectID, m.t.Number = tg.project, number
	m.add(note{event: "task.moved", activity: "moved_in", data: ids, payload: ids})
	return nil
}

// remapLinks maps milestone and project labels to the target project by name and drops what has no counterpart; the
// assignees who cannot read the target project are removed.
func (m *mut) remapLinks(target uuid.UUID) error {
	tenant := m.a.TenantID
	if err := m.tx.Exec(`UPDATE tasks SET milestone_id = (SELECT tm.id FROM milestones sm JOIN milestones tm ON tm.project_id = ?
			AND lower(tm.name) = lower(sm.name) AND tm.tenant_id = ? WHERE sm.id = tasks.milestone_id) WHERE id = ? AND tenant_id = ?`,
		target, tenant, m.t.ID, tenant).Error; err != nil {
		return err
	}
	if err := m.tx.Exec(`UPDATE task_labels tl SET label_id = tgt.id FROM labels src, labels tgt WHERE tl.task_id = ? AND tl.tenant_id = ?
			AND tl.label_id = src.id AND src.project_id = ? AND tgt.project_id = ? AND tgt.tenant_id = ? AND lower(tgt.name) = lower(src.name)`,
		m.t.ID, tenant, m.t.ProjectID, target, tenant).Error; err != nil {
		return err
	}
	if err := m.tx.Exec(`DELETE FROM task_labels tl USING labels l WHERE tl.task_id = ? AND tl.tenant_id = ? AND tl.label_id = l.id
			AND l.project_id = ?`, m.t.ID, tenant, m.t.ProjectID).Error; err != nil {
		return err
	}
	return m.tx.Exec(`DELETE FROM task_assignees ta WHERE ta.task_id = ? AND ta.tenant_id = ? AND NOT EXISTS (
			SELECT 1 FROM project_members pm LEFT JOIN project_roles r ON r.id = pm.custom_role_id
			WHERE pm.project_id = ? AND pm.user_id = ta.user_id AND pm.active
			AND (pm.role_key IS NOT NULL OR r.permissions @> '["task.read"]'::jsonb))`, m.t.ID, tenant, target).Error
}

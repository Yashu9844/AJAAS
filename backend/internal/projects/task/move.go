package task

import (
	"context"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
	"gorm.io/gorm"
)

// wipCheck enforces the WIP limit of the target column: a hard limit refuses (409 WIP_LIMIT), a soft one warns (TK-04).
func wipCheck(tx *gorm.DB, tenantID uuid.UUID, col *colRow, exclude uuid.UUID) ([]string, error) {
	if col.WipLimit == nil {
		return nil, nil
	}
	n, err := count(tx, `SELECT count(*) FROM tasks WHERE `+liveInColumn+` AND archived_at IS NULL`, tenantID, col.ID, exclude)
	if err != nil {
		return nil, err
	}
	if int(n)+1 <= *col.WipLimit {
		return nil, nil
	}
	if col.WipHard {
		return nil, kernel.Conflict("WIP_LIMIT", "the column is at its work-in-progress limit")
	}
	return []string{"WIP_LIMIT_EXCEEDED"}, nil
}

func targetOf(in MoveInput) func(t *Task) uuid.UUID {
	return func(t *Task) uuid.UUID {
		if in.ColumnID != nil {
			return *in.ColumnID
		}
		return t.ColumnID
	}
}

// Move puts a task in a column, at a position or at the end (TK-04, TK-05).
func (s *Service) Move(ctx context.Context, a api.Actor, id uuid.UUID, in MoveInput) (*View, error) {
	return s.mutate(ctx, a, id, mutSpec{Perm: api.TaskUpdate, Version: in.Version, LockColumn: targetOf(in)}, func(m *mut) error {
		return m.place(in)
	})
}

// Reorder moves a task next to a neighbour in its column (or the named column).
func (s *Service) Reorder(ctx context.Context, a api.Actor, id uuid.UUID, in MoveInput) (*View, error) {
	if (in.BeforeID == nil) == (in.AfterID == nil) {
		return nil, kernel.Invalid("before_id", "give exactly one of before_id and after_id")
	}
	return s.Move(ctx, a, id, in)
}

func (m *mut) place(in MoveInput) error {
	if in.BeforeID != nil && in.AfterID != nil {
		return kernel.Invalid("before_id", "give at most one of before_id and after_id")
	}
	if (in.BeforeID != nil && *in.BeforeID == m.t.ID) || (in.AfterID != nil && *in.AfterID == m.t.ID) {
		return kernel.Invalid("before_id", "a task cannot be placed next to itself")
	}
	target := m.t.ColumnID
	if in.ColumnID != nil {
		target = *in.ColumnID
	} else if m.t.ColumnID != m.col0 {
		return kernel.Conflict("TASK_MOVED", "the task moved to another column; retry")
	}
	to, err := column(m.tx, m.a.TenantID, m.t.ProjectID, target)
	if err != nil {
		return err
	}
	rank, err := m.s.allocRank(m.tx, colPlace{m.a.TenantID, to.ID, m.t.ID}, slot{before: in.BeforeID, after: in.AfterID})
	if err != nil {
		return err
	}
	if to.ID == m.t.ColumnID {
		m.quiet("rank", rank)
		m.add(note{event: "task.moved", payload: map[string]any{"from_column_id": to.ID, "to_column_id": to.ID}})
		return nil
	}
	return m.changeColumn(to, rank, in.Force)
}

// changeColumn applies a column change: rules for done, WIP, completion time and the completed/reopened events.
func (m *mut) changeColumn(to *colRow, rank string, force bool) error {
	from, err := column(m.tx, m.a.TenantID, m.t.ProjectID, m.t.ColumnID)
	if err != nil {
		return err
	}
	if to.Category == api.ColumnDone {
		if err := m.doneRules(force); err != nil {
			return err
		}
	}
	warns, err := wipCheck(m.tx, m.a.TenantID, to, m.t.ID)
	if err != nil {
		return err
	}
	m.warn = append(m.warn, warns...)
	m.change("column_id", to.ID)
	m.change("rank", rank)
	ids := map[string]any{"from_column_id": from.ID, "to_column_id": to.ID}
	m.add(note{event: "task.moved", activity: "moved", data: ids, payload: ids})
	switch {
	case to.Category == api.ColumnDone && from.Category != api.ColumnDone:
		now := m.s.now().UTC()
		m.change("completed_at", &now)
		m.add(note{event: "task.completed"})
	case from.Category == api.ColumnDone && to.Category != api.ColumnDone:
		m.change("completed_at", nil)
	}
	if from.closed() && !to.closed() {
		m.add(note{event: "task.reopened"})
	}
	return nil
}

// doneRules refuses completion while blockers or subtasks are open unless a manager forces it (TK-06, TK-07).
func (m *mut) doneRules(force bool) error {
	tid, tenant := m.t.ID, m.a.TenantID
	blockers, err := count(m.tx, `SELECT count(*) FROM task_relations r JOIN tasks b ON b.id = r.source_task_id
		JOIN board_columns bc ON bc.id = b.column_id WHERE r.target_task_id = ? AND r.tenant_id = ? AND r.type = 'blocks'
		AND b.deleted_at IS NULL AND bc.category NOT IN ('done', 'canceled')`, tid, tenant)
	if err != nil {
		return err
	}
	open, err := count(m.tx, `SELECT count(*) FROM tasks s JOIN board_columns sc ON sc.id = s.column_id WHERE s.parent_task_id = ?
		AND s.tenant_id = ? AND s.deleted_at IS NULL AND sc.category NOT IN ('done', 'canceled')`, tid, tenant)
	if err != nil {
		return err
	}
	if blockers == 0 && open == 0 {
		return nil
	}
	if !force {
		if blockers > 0 {
			return kernel.Conflict("TASK_BLOCKED", "the task has open blockers; resolve them or force the move")
		}
		return kernel.Conflict("OPEN_SUBTASKS", "the task has open subtasks; finish them or force the move")
	}
	return m.acc.Require(api.TaskDelete)
}

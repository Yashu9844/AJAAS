package task

import (
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/kernel"
)

var categories = map[string]bool{"backlog": true, "todo": true, "in_progress": true, "done": true, "canceled": true}

// where builds the WHERE clause and its arguments; every user value is a bound parameter.
func (f ListFilter) where(tenantID, projectID uuid.UUID) (string, []any, error) {
	w := `t.tenant_id = ? AND t.project_id = ? AND t.deleted_at IS NULL`
	args := []any{tenantID, projectID}
	if f.Archived {
		w += ` AND t.archived_at IS NOT NULL`
	} else {
		w += ` AND t.archived_at IS NULL`
	}
	if f.Category != "" && !categories[f.Category] {
		return "", nil, kernel.Invalid("category", "must be backlog, todo, in_progress, done or canceled")
	}
	if f.Priority != "" && !priorities[f.Priority] {
		return "", nil, kernel.Invalid("priority", "must be none, low, medium, high or urgent")
	}
	for _, c := range f.equalities() {
		if c.set {
			w += ` AND ` + c.sql
			args = append(args, c.arg)
		}
	}
	w, args = f.dueRange(w, args)
	w = f.flags(w)
	if f.Query != "" {
		w += ` AND (t.search @@ plainto_tsquery('simple', ?) OR t.title ILIKE ?)`
		args = append(args, f.Query, "%"+escapeLike(f.Query)+"%")
	}
	return w, args, nil
}

type cond struct {
	set bool
	sql string
	arg any
}

func ptr[T any](p *T) any {
	if p == nil {
		return nil
	}
	return *p
}

// equalities lists the single-value filters with fixed SQL fragments.
func (f ListFilter) equalities() []cond {
	return []cond{
		{f.ColumnID != nil, `t.column_id = ?`, ptr(f.ColumnID)},
		{f.Category != "", `c.category = ?`, f.Category},
		{f.Priority != "", `t.priority = ?`, f.Priority},
		{f.MilestoneID != nil, `t.milestone_id = ?`, ptr(f.MilestoneID)},
		{f.SprintID != nil, `t.sprint_id = ?`, ptr(f.SprintID)},
		{f.ParentID != nil, `t.parent_task_id = ?`, ptr(f.ParentID)},
		{f.AssigneeID != nil, `EXISTS (SELECT 1 FROM task_assignees ta WHERE ta.task_id = t.id AND ta.user_id = ? AND ta.active)`, ptr(f.AssigneeID)},
		{f.LabelID != nil, `EXISTS (SELECT 1 FROM task_labels tl WHERE tl.task_id = t.id AND tl.label_id = ?)`, ptr(f.LabelID)},
	}
}

func (f ListFilter) dueRange(w string, args []any) (string, []any) {
	if f.DueFrom != nil {
		w += ` AND t.due_at >= ?`
		args = append(args, *f.DueFrom)
	}
	if f.DueTo != nil {
		w += ` AND t.due_at <= ?`
		args = append(args, *f.DueTo)
	}
	return w, args
}

func (f ListFilter) flags(w string) string {
	if f.TopLevel {
		w += ` AND t.parent_task_id IS NULL`
	}
	for _, fl := range []struct {
		v    *bool
		expr string
	}{{f.Overdue, overdueExpr}, {f.Blocked, blockedExpr}} {
		switch {
		case fl.v == nil:
		case *fl.v:
			w += ` AND ` + fl.expr
		default:
			w += ` AND NOT ` + fl.expr
		}
	}
	return w
}

func escapeLike(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if r == '%' || r == '_' || r == '\\' {
			out = append(out, '\\')
		}
		out = append(out, r)
	}
	return string(out)
}

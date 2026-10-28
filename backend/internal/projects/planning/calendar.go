package planning

import (
	"context"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
)

// CalendarView lists the tasks whose due_at or start_on falls in the window. Truncated is true when more than 500 tasks
// matched and the rest was cut (narrow the window or filter).
type CalendarView struct {
	From      string `json:"from"`
	To        string `json:"to"`
	Tasks     []Card `json:"tasks"`
	Truncated bool   `json:"truncated"`
}

// calendarWhere matches tasks due inside the window instants or starting inside the window days (all UTC).
const calendarWhere = ` AND ((t.due_at >= ? AND t.due_at < ?) OR (t.start_on >= ?::date AND t.start_on <= ?::date))`

// calendarOrder sorts by the first relevant instant; a start date counts as midnight UTC.
const calendarOrder = `COALESCE(t.due_at, t.start_on::timestamp AT TIME ZONE 'UTC'), t.id`

// ProjectCalendar returns one project calendar window of at most 62 days (VW-03, task.read).
func (s *Service) ProjectCalendar(ctx context.Context, a api.Actor, projectID uuid.UUID, w Window) (*CalendarView, error) {
	acc, err := s.access(ctx, a, projectID)
	if err != nil {
		return nil, err
	}
	if err := acc.Require(api.TaskRead); err != nil {
		return nil, err
	}
	where := `t.tenant_id = ? AND t.project_id = ? AND ` + liveTask
	return s.calendar(ctx, where, []any{a.TenantID, projectID}, w)
}

// MyCalendar returns the tasks assigned to the caller across the projects the caller can read (VW-03).
func (s *Service) MyCalendar(ctx context.Context, a api.Actor, w Window) (*CalendarView, error) {
	ids, err := s.taskProjects(ctx, a)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return emptyCalendar(w), nil
	}
	where := `t.tenant_id = ? AND t.project_id = ANY(?::uuid[]) AND ` + liveTask + ` AND ` + assignedTo
	return s.calendar(ctx, where, []any{a.TenantID, idArray(ids), a.UserID}, w)
}

// assignedTo restricts to tasks assigned to one user (bind the user id as the next argument).
const assignedTo = `EXISTS (SELECT 1 FROM task_assignees ta WHERE ta.task_id = t.id AND ta.tenant_id = t.tenant_id AND ta.user_id = ?)`

func emptyCalendar(w Window) *CalendarView {
	return &CalendarView{From: w.From.String(), To: w.To.String(), Tasks: []Card{}}
}

func (s *Service) calendar(ctx context.Context, where string, args []any, w Window) (*CalendarView, error) {
	start, end := w.bounds()
	rows, err := cardQuery{where: where + calendarWhere, args: append(args, start, end, w.From, w.To), order: calendarOrder,
		limit: maxPageTasks + 1}.run(ctx, s.db)
	if err != nil {
		return nil, err
	}
	out := emptyCalendar(w)
	if len(rows) > maxPageTasks {
		rows, out.Truncated = rows[:maxPageTasks], true
	}
	if rows != nil {
		out.Tasks = rows
	}
	return out, nil
}

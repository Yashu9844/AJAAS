package planning

import (
	"context"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
)

// GanttParams selects a Gantt page. Window is nil for every dated task of the project.
type GanttParams struct {
	Window *Window
	Cursor string
	Limit  int
}

// GanttTask is a dated task with the number of tasks it depends on.
type GanttTask struct {
	Card
	DependencyCount int `json:"dependency_count"`
}

// GanttMilestone is a milestone marker on the timeline.
type GanttMilestone struct {
	ID    uuid.UUID    `json:"id"`
	Name  string       `json:"name"`
	DueOn *kernel.Date `json:"due_on"`
	State string       `json:"status" gorm:"column:status"`
}

// Dependency is a directed blocks relation: Source blocks Target.
type Dependency struct {
	SourceTaskID uuid.UUID `json:"source_task_id"`
	TargetTaskID uuid.UUID `json:"target_task_id"`
	Type         string    `json:"type"`
}

// GanttView is one page of the Gantt read model (VW-04).
type GanttView struct {
	Tasks        []GanttTask      `json:"tasks"`
	Milestones   []GanttMilestone `json:"milestones"`
	Dependencies []Dependency     `json:"dependencies"`
	NextCursor   *string          `json:"next_cursor"`
}

// ganttStart is the day a task begins on the timeline: start_on, else the due day (UTC).
const ganttStart = `COALESCE(t.start_on, (t.due_at AT TIME ZONE 'UTC')::date)`

// ganttEnd is the day it ends: the due day, else start_on.
const ganttEnd = `COALESCE((t.due_at AT TIME ZONE 'UTC')::date, t.start_on)`

type ganttCursor struct {
	Day string `json:"d"`
	ID  string `json:"id"`
}

// ClampGanttLimit applies the Gantt page default (200) and maximum (500).
func ClampGanttLimit(raw string) int {
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || n < 1 {
		return 200
	}
	return min(n, maxPageTasks)
}

// Gantt returns the dated tasks, the milestones and the blocks relations of a project, paged by start day, at most 500
// tasks per request (VW-04, task.read).
func (s *Service) Gantt(ctx context.Context, a api.Actor, projectID uuid.UUID, p GanttParams) (*GanttView, error) {
	acc, err := s.access(ctx, a, projectID)
	if err != nil {
		return nil, err
	}
	if err := acc.Require(api.TaskRead); err != nil {
		return nil, err
	}
	var cur ganttCursor
	if err := kernel.DecodeCursor(p.Cursor, &cur); err != nil {
		return nil, err
	}
	where, args := ganttWhere(a, projectID, p.Window, cur)
	rows, err := cardQuery{where: where, args: args, order: ganttStart + `, t.id`, limit: p.Limit + 1}.run(ctx, s.db)
	if err != nil {
		return nil, err
	}
	rows, more := kernel.Trim(rows, p.Limit)
	out := &GanttView{Tasks: make([]GanttTask, len(rows)), Dependencies: []Dependency{}}
	for i, c := range rows {
		out.Tasks[i] = GanttTask{Card: c}
	}
	if more {
		next := kernel.EncodeCursor(ganttCursor{Day: ganttDay(rows[len(rows)-1]), ID: rows[len(rows)-1].ID.String()})
		out.NextCursor = &next
	}
	if out.Milestones, err = s.ganttMilestones(ctx, a.TenantID, projectID, p.Window); err != nil {
		return nil, err
	}
	return out, s.attachDependencies(ctx, a, out)
}

func ganttWhere(a api.Actor, projectID uuid.UUID, w *Window, cur ganttCursor) (string, []any) {
	where := `t.tenant_id = ? AND t.project_id = ? AND ` + liveTask + ` AND (t.start_on IS NOT NULL OR t.due_at IS NOT NULL)`
	args := []any{a.TenantID, projectID}
	if w != nil {
		where += ` AND ` + ganttStart + ` <= ?::date AND ` + ganttEnd + ` >= ?::date`
		args = append(args, w.To, w.From)
	}
	if cur.ID != "" {
		where += ` AND (` + ganttStart + `, t.id) > (?::date, ?::uuid)`
		args = append(args, cur.Day, cur.ID)
	}
	return where, args
}

// ganttDay is the sort key of a card as used by ganttStart.
func ganttDay(c Card) string {
	if c.StartOn != nil {
		return c.StartOn.String()
	}
	if c.DueAt == nil {
		return ""
	}
	return c.DueAt.UTC().Format("2006-01-02")
}

func (s *Service) ganttMilestones(ctx context.Context, tenantID, projectID uuid.UUID, w *Window) ([]GanttMilestone, error) {
	q, args := `SELECT id, name, due_on, status FROM milestones WHERE tenant_id = ? AND project_id = ?`, []any{tenantID, projectID}
	if w != nil {
		q += ` AND due_on >= ?::date AND due_on <= ?::date`
		args = append(args, w.From, w.To)
	}
	var rows []GanttMilestone
	err := s.db.WithContext(ctx).Raw(q+` ORDER BY due_on NULLS LAST, name LIMIT 200`, args...).Scan(&rows).Error
	if rows == nil {
		rows = []GanttMilestone{}
	}
	return rows, err
}

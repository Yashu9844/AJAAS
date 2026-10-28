package planning

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
)

// My work groups (VW-05).
const (
	GroupOverdue  = "overdue"
	GroupToday    = "today"
	GroupThisWeek = "this_week"
	GroupLater    = "later"
	GroupNoDate   = "no_date"
)

// WorkGroups lists the groups in display order.
var WorkGroups = []string{GroupOverdue, GroupToday, GroupThisWeek, GroupLater, GroupNoDate}

// WorkCounts are the per-group totals for badges. Total counts assigned open tasks; Watching and Mentions count the
// open tasks the caller watches or is mentioned in (read from the collaboration tables task_watchers and mentions).
type WorkCounts struct {
	Overdue  int `json:"overdue"`
	Today    int `json:"today"`
	ThisWeek int `json:"this_week"`
	Later    int `json:"later"`
	NoDate   int `json:"no_date"`
	Total    int `json:"total"`
	Watching int `json:"watching"`
	Mentions int `json:"mentions"`
}

// WorkView is My work: per-group counts and, per group, the first page of tasks (or one full group page).
type WorkView struct {
	Counts     WorkCounts        `json:"counts"`
	Groups     map[string][]Card `json:"groups"`
	NextCursor *string           `json:"next_cursor,omitempty"`
}

// WorkParams selects My work. Group, when set, returns that group only (a due-date group, watching or mentions), paged
// with Cursor; otherwise every group returns its first PerGroup tasks.
type WorkParams struct {
	Group    string
	Cursor   string
	PerGroup int
}

// dayBounds are the instants that split the groups, from the service clock (UTC): today starts at Today, this week runs
// to the next Monday.
type dayBounds struct{ today, tomorrow, nextWeek time.Time }

func boundsAt(now time.Time) dayBounds {
	d := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	back := (int(d.Weekday()) + 6) % 7 // days since Monday
	return dayBounds{today: d, tomorrow: d.AddDate(0, 0, 1), nextWeek: d.AddDate(0, 0, 7-back)}
}

// workFilter is the shared filter: open tasks of active, readable projects (bind tenant and project ids).
const workFilter = `t.tenant_id = ? AND t.project_id = ANY(?::uuid[]) AND p.status = 'active' AND ` + liveTask + ` AND ` + openCategories

// workBase narrows the scope to tasks assigned to the caller (bind the user id next).
const workBase = workFilter + ` AND ` + assignedTo

// groupWhere returns the due_at predicate of a group and its bind values.
func groupWhere(group string, b dayBounds) (string, []any, bool) {
	switch group {
	case GroupOverdue:
		return `t.due_at < ?`, []any{b.today}, true
	case GroupToday:
		return `t.due_at >= ? AND t.due_at < ?`, []any{b.today, b.tomorrow}, true
	case GroupThisWeek:
		return `t.due_at >= ? AND t.due_at < ?`, []any{b.tomorrow, b.nextWeek}, true
	case GroupLater:
		return `t.due_at >= ?`, []any{b.nextWeek}, true
	case GroupNoDate:
		return `t.due_at IS NULL`, nil, true
	}
	return "", nil, false
}

// MyWork returns the caller tasks grouped by due date plus the counts (VW-05). Only open tasks of active projects the
// caller can read are included.
func (s *Service) MyWork(ctx context.Context, a api.Actor, p WorkParams) (*WorkView, error) {
	if p.Group != "" && !validGroup(p.Group) {
		return nil, kernel.Invalid("group", "must be overdue, today, this_week, later, no_date, watching or mentions")
	}
	ids, err := s.taskProjects(ctx, a)
	if err != nil {
		return nil, err
	}
	out := &WorkView{Groups: map[string][]Card{}}
	for _, g := range allGroups {
		out.Groups[g] = []Card{}
	}
	if len(ids) == 0 {
		return out, nil
	}
	b := boundsAt(s.now())
	if out.Counts, err = s.workCounts(ctx, a, ids, b); err != nil {
		return nil, err
	}
	if err = s.linkedCounts(ctx, a, ids, &out.Counts); err != nil {
		return nil, err
	}
	groups := allGroups
	if p.Group != "" {
		groups = []string{p.Group}
	}
	for _, g := range groups {
		rows, next, err := s.workGroup(ctx, workScope{a: a, ids: ids, b: b, group: g}, p)
		if err != nil {
			return nil, err
		}
		out.Groups[g] = rows
		if p.Group != "" && next != "" {
			out.NextCursor = &next
		}
	}
	return out, nil
}

// MyWorkCounts returns the badge counts only.
func (s *Service) MyWorkCounts(ctx context.Context, a api.Actor) (WorkCounts, error) {
	ids, err := s.taskProjects(ctx, a)
	if err != nil || len(ids) == 0 {
		return WorkCounts{}, err
	}
	c, err := s.workCounts(ctx, a, ids, boundsAt(s.now()))
	if err != nil {
		return c, err
	}
	return c, s.linkedCounts(ctx, a, ids, &c)
}

func (s *Service) workCounts(ctx context.Context, a api.Actor, ids []uuid.UUID, b dayBounds) (WorkCounts, error) {
	var c WorkCounts
	err := s.db.WithContext(ctx).Raw(`SELECT
		count(*) FILTER (WHERE t.due_at < ?) AS overdue,
		count(*) FILTER (WHERE t.due_at >= ? AND t.due_at < ?) AS today,
		count(*) FILTER (WHERE t.due_at >= ? AND t.due_at < ?) AS this_week,
		count(*) FILTER (WHERE t.due_at >= ?) AS later,
		count(*) FILTER (WHERE t.due_at IS NULL) AS no_date,
		count(*) AS total
		FROM tasks t JOIN projects p ON p.id = t.project_id AND p.tenant_id = t.tenant_id JOIN board_columns c ON c.id = t.column_id
		WHERE `+workBase, b.today, b.today, b.tomorrow, b.tomorrow, b.nextWeek, b.nextWeek, a.TenantID, idArray(ids), a.UserID).Scan(&c).Error
	return c, err
}

type workScope struct {
	a     api.Actor
	ids   []uuid.UUID
	b     dayBounds
	group string
}

type workCursor struct {
	At string `json:"at"`
	ID string `json:"id"`
}

// workGroup loads one group page ordered by due date then id (keyset).
func (s *Service) workGroup(ctx context.Context, w workScope, p WorkParams) ([]Card, string, error) {
	var cur workCursor
	if err := kernel.DecodeCursor(p.Cursor, &cur); err != nil {
		return nil, "", err
	}
	if isLinked(w.group) {
		return s.linkedGroup(ctx, w, p)
	}
	cond, cargs, _ := groupWhere(w.group, w.b)
	where, args := workBase+` AND `+cond, append([]any{w.a.TenantID, idArray(w.ids), w.a.UserID}, cargs...)
	if cur.ID != "" {
		if cur.At == "" {
			where, args = where+` AND t.id > ?::uuid`, append(args, cur.ID)
		} else {
			where, args = where+` AND (t.due_at, t.id) > (?::timestamptz, ?::uuid)`, append(args, cur.At, cur.ID)
		}
	}
	limit := ClampWorkLimit(p.PerGroup)
	rows, err := cardQuery{where: where, args: args, order: `t.due_at NULLS LAST, t.id`, limit: limit + 1}.run(ctx, s.db)
	if err != nil {
		return nil, "", err
	}
	rows, more := kernel.Trim(rows, limit)
	next := ""
	if more {
		last := rows[len(rows)-1]
		c := workCursor{ID: last.ID.String()}
		if last.DueAt != nil {
			c.At = last.DueAt.UTC().Format(time.RFC3339Nano)
		}
		next = kernel.EncodeCursor(c)
	}
	return rows, next, nil
}

// ClampWorkLimit applies the My work group page default (20) and maximum (100).
func ClampWorkLimit(n int) int {
	if n < 1 {
		return 20
	}
	return min(n, kernel.MaxLimit)
}

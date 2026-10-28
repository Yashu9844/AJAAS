package task

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
)

// ListFilter narrows and orders a project task list (TK-13). Nil pointers and empty strings mean "no filter".
type ListFilter struct {
	ColumnID    *uuid.UUID
	Category    string
	Priority    string
	AssigneeID  *uuid.UUID
	LabelID     *uuid.UUID
	MilestoneID *uuid.UUID
	SprintID    *uuid.UUID
	ParentID    *uuid.UUID
	DueFrom     *time.Time
	DueTo       *time.Time
	Overdue     *bool
	Blocked     *bool
	TopLevel    bool
	Archived    bool
	Query       string
	Sort        string
	Dir         string
	Cursor      string
	Limit       int
}

type sortSpec struct {
	expr, cast, dir string
}

// sorts is the allow-list of orderings; user input only selects a key, never an expression.
var sorts = map[string]sortSpec{
	"rank":     {"t.rank", `text COLLATE "C"`, "asc"},
	"created":  {"t.created_at", "timestamptz", "desc"},
	"updated":  {"t.updated_at", "timestamptz", "desc"},
	"due":      {"COALESCE(t.due_at, 'infinity'::timestamptz)", "timestamptz", "asc"},
	"priority": {"t.priority_rank", "int", "desc"},
	"number":   {"t.number", "int", "desc"},
}

type listCursor struct {
	Sort string `json:"s"`
	Dir  string `json:"d"`
	Val  string `json:"v"`
	ID   string `json:"id"`
}

// resolveSort validates the sort key and direction and returns the effective pair.
func (f ListFilter) resolveSort() (string, sortSpec, string, error) {
	key := f.Sort
	if key == "" {
		key = "rank"
	}
	spec, ok := sorts[key]
	if !ok {
		return "", sortSpec{}, "", kernel.Invalid("sort", "must be rank, created, updated, due, priority or number")
	}
	dir := spec.dir
	switch strings.ToLower(f.Dir) {
	case "":
	case "asc", "desc":
		dir = strings.ToLower(f.Dir)
	default:
		return "", sortSpec{}, "", kernel.Invalid("dir", "must be asc or desc")
	}
	return key, spec, dir, nil
}

// List pages the tasks of a project with filters and a keyset cursor (TK-13). Order is stable: the sort key, then id.
func (s *Service) List(ctx context.Context, a api.Actor, projectID uuid.UUID, f ListFilter) ([]View, string, error) {
	acc, err := s.access(ctx, a, projectID)
	if err != nil {
		return nil, "", err
	}
	if err := acc.Require(api.TaskRead); err != nil {
		return nil, "", err
	}
	key, spec, dir, err := f.resolveSort()
	if err != nil {
		return nil, "", err
	}
	if f.Limit < 1 || f.Limit > kernel.MaxLimit {
		f.Limit = kernel.DefaultLimit
	}
	var cur listCursor
	if err := kernel.DecodeCursor(f.Cursor, &cur); err != nil {
		return nil, "", err
	}
	if cur.ID != "" && (cur.Sort != key || cur.Dir != dir) {
		return nil, "", kernel.Invalid("cursor", "does not belong to this sort order")
	}
	where, args, err := f.where(a.TenantID, projectID)
	if err != nil {
		return nil, "", err
	}
	if cur.ID != "" {
		op := map[string]string{"asc": ">", "desc": "<"}[dir]
		where += ` AND (` + spec.expr + `, t.id) ` + op + ` (?::` + spec.cast + `, ?::uuid)`
		args = append(args, cur.Val, cur.ID)
	}
	order := ` ORDER BY ` + spec.expr + ` ` + dir + `, t.id ` + dir + ` LIMIT ?`
	sql := `SELECT ` + viewCols + `, (` + spec.expr + `)::text AS sort_key` + viewFrom + `WHERE ` + where + order
	var rows []View
	if err := s.db.WithContext(ctx).Raw(sql, append(args, f.Limit+1)...).Scan(&rows).Error; err != nil {
		return nil, "", err
	}
	rows, more := kernel.Trim(rows, f.Limit)
	if err := s.fill(ctx, s.db, a.TenantID, rows); err != nil {
		return nil, "", err
	}
	next := ""
	if more {
		last := rows[len(rows)-1]
		next = kernel.EncodeCursor(listCursor{Sort: key, Dir: dir, Val: last.SortKey, ID: last.ID.String()})
	}
	return rows, next, nil
}

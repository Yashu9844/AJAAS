package task

import (
	"context"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
)

// ActivityFilter pages an activity read by its monotonic sequence (CM-04).
type ActivityFilter struct {
	Cursor string
	Limit  int
	Asc    bool
}

type seqCursor struct {
	Seq int64 `json:"seq"`
}

const activityCols = `seq, id, project_id, task_id, actor_user_id, type, data, source, occurred_at`

// TaskActivity returns the history of one task, newest first unless Asc is set.
func (s *Service) TaskActivity(ctx context.Context, a api.Actor, taskID uuid.UUID, f ActivityFilter) ([]ActivityRow, string, error) {
	if _, _, err := s.locate(ctx, a, taskID); err != nil {
		return nil, "", err
	}
	return s.activity(ctx, a, `task_id = ?`, taskID, f)
}

// ProjectActivity returns the history of a project, newest first unless Asc is set.
func (s *Service) ProjectActivity(ctx context.Context, a api.Actor, projectID uuid.UUID, f ActivityFilter) ([]ActivityRow, string, error) {
	acc, err := s.access(ctx, a, projectID)
	if err != nil {
		return nil, "", err
	}
	if err := acc.Require(api.TaskRead); err != nil {
		return nil, "", err
	}
	return s.activity(ctx, a, `project_id = ?`, projectID, f)
}

func (s *Service) activity(ctx context.Context, a api.Actor, scope string, id uuid.UUID, f ActivityFilter) ([]ActivityRow, string, error) {
	if f.Limit < 1 || f.Limit > kernel.MaxLimit {
		f.Limit = kernel.DefaultLimit
	}
	var cur seqCursor
	if err := kernel.DecodeCursor(f.Cursor, &cur); err != nil {
		return nil, "", err
	}
	op, dir := "<", "DESC"
	if f.Asc {
		op, dir = ">", "ASC"
	}
	where, args := `tenant_id = ? AND `+scope, []any{a.TenantID, id}
	if f.Cursor != "" {
		where += ` AND seq ` + op + ` ?`
		args = append(args, cur.Seq)
	}
	var rows []ActivityRow
	err := s.db.WithContext(ctx).Raw(`SELECT `+activityCols+` FROM activity WHERE `+where+` ORDER BY seq `+dir+` LIMIT ?`,
		append(args, f.Limit+1)...).Scan(&rows).Error
	if err != nil {
		return nil, "", err
	}
	rows, more := kernel.Trim(rows, f.Limit)
	next := ""
	if more {
		next = kernel.EncodeCursor(seqCursor{Seq: rows[len(rows)-1].Seq})
	}
	return rows, next, nil
}

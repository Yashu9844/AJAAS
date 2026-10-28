package task

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
	"gorm.io/gorm"
)

// Derived-column SQL shared by the view query and the list filters.
const (
	blockedExpr = `EXISTS (SELECT 1 FROM task_relations r JOIN tasks b ON b.id = r.source_task_id
		JOIN board_columns bc ON bc.id = b.column_id WHERE r.target_task_id = t.id AND r.tenant_id = t.tenant_id
		AND r.type = 'blocks' AND b.deleted_at IS NULL AND bc.category NOT IN ('done', 'canceled'))`
	overdueExpr = `(t.due_at IS NOT NULL AND t.due_at < now() AND c.category NOT IN ('done', 'canceled'))`
	viewCols    = `t.*, p.key || '-' || t.number AS key, c.category AS column_category, ` + blockedExpr + ` AS blocked, ` +
		overdueExpr + ` AS overdue,
		(SELECT count(*) FROM tasks s WHERE s.parent_task_id = t.id AND s.deleted_at IS NULL) AS subtask_total,
		(SELECT count(*) FROM tasks s JOIN board_columns sc ON sc.id = s.column_id WHERE s.parent_task_id = t.id
			AND s.deleted_at IS NULL AND sc.category = 'done') AS subtask_done`
	viewFrom   = ` FROM tasks t JOIN board_columns c ON c.id = t.column_id JOIN projects p ON p.id = t.project_id `
	viewSelect = `SELECT ` + viewCols + viewFrom
)

// row reads one task without access checks (internal use after the caller was authorised).
func (s *Service) row(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*Task, error) {
	var t Task
	err := db.WithContext(ctx).Where("id = ? AND tenant_id = ? AND deleted_at IS NULL", id, tenantID).Take(&t).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, kernel.NotFound("task")
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// locate finds a live task and the caller access to its project; unknown, foreign and unreadable ids are all 404 (PG4).
func (s *Service) locate(ctx context.Context, a api.Actor, id uuid.UUID) (*Task, api.Access, error) {
	t, err := s.row(ctx, s.db, a.TenantID, id)
	if err != nil {
		return nil, api.Access{}, err
	}
	acc, err := s.access(ctx, a, t.ProjectID)
	if err != nil {
		return nil, api.Access{}, kernel.NotFound("task")
	}
	if !acc.Can(api.TaskRead) {
		return nil, api.Access{}, kernel.NotFound("task")
	}
	return t, acc, nil
}

// Task implements api.TaskResolver: the task reference and the caller access to its project.
func (s *Service) Task(ctx context.Context, db *gorm.DB, a api.Actor, taskID uuid.UUID) (api.TaskRef, api.Access, error) {
	t, err := s.row(ctx, s.conn(db), a.TenantID, taskID)
	if err != nil {
		return api.TaskRef{}, api.Access{}, err
	}
	acc, err := s.auth.Access(ctx, s.conn(db), a, t.ProjectID)
	if err != nil || !acc.Can(api.TaskRead) {
		return api.TaskRef{}, api.Access{}, kernel.NotFound("task")
	}
	return api.TaskRef{ID: t.ID, ProjectID: t.ProjectID, Number: t.Number, Title: t.Title, ColumnID: t.ColumnID, Version: t.Version,
		Archived: t.ArchivedAt != nil}, acc, nil
}

func (s *Service) conn(db *gorm.DB) *gorm.DB {
	if db != nil {
		return db
	}
	return s.db
}

// view loads one task as the API shape.
func (s *Service) view(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*View, error) {
	var rows []View
	err := db.WithContext(ctx).Raw(viewSelect+`WHERE t.id = ? AND t.tenant_id = ? AND t.deleted_at IS NULL`, id, tenantID).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, kernel.NotFound("task")
	}
	if err := s.fill(ctx, db, tenantID, rows); err != nil {
		return nil, err
	}
	return &rows[0], nil
}

type pair struct {
	TaskID uuid.UUID
	Other  uuid.UUID
	Active bool
}

// fill adds assignees, labels and the progress roll-up to a page of views with two batched queries.
func (s *Service) fill(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, rows []View) error {
	if len(rows) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, len(rows))
	idx := make(map[uuid.UUID]int, len(rows))
	for i := range rows {
		ids[i], idx[rows[i].ID] = rows[i].ID, i
		rows[i].AssigneeIDs, rows[i].LabelIDs, rows[i].Assignees = []uuid.UUID{}, []uuid.UUID{}, []Assignee{}
		if rows[i].SubtaskTotal > 0 {
			rows[i].Progress = rows[i].SubtaskDone * 100 / rows[i].SubtaskTotal
		}
	}
	var as, ls []pair
	q := db.WithContext(ctx)
	if err := q.Raw(`SELECT task_id, user_id AS other, active FROM task_assignees WHERE tenant_id = ? AND task_id IN ? ORDER BY assigned_at, user_id`,
		tenantID, ids).Scan(&as).Error; err != nil {
		return err
	}
	if err := q.Raw(`SELECT task_id, label_id AS other FROM task_labels WHERE tenant_id = ? AND task_id IN ? ORDER BY label_id`,
		tenantID, ids).Scan(&ls).Error; err != nil {
		return err
	}
	for _, p := range as {
		r := &rows[idx[p.TaskID]]
		r.Assignees = append(r.Assignees, Assignee{UserID: p.Other, Active: p.Active})
		if p.Active {
			r.AssigneeIDs = append(r.AssigneeIDs, p.Other)
		}
	}
	for _, p := range ls {
		rows[idx[p.TaskID]].LabelIDs = append(rows[idx[p.TaskID]].LabelIDs, p.Other)
	}
	return nil
}

// Get returns a task the caller can read.
func (s *Service) Get(ctx context.Context, a api.Actor, id uuid.UUID) (*View, error) {
	if _, _, err := s.locate(ctx, a, id); err != nil {
		return nil, err
	}
	return s.view(ctx, s.db, a.TenantID, id)
}

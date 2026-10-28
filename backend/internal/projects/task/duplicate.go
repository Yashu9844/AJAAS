package task

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"gorm.io/gorm"
)

const copySuffix = " (copy)"

// Duplicate copies a task into the same column right after the original (TK-11): a new number, the same fields, no
// comments, and the labels and assignees only when asked.
func (s *Service) Duplicate(ctx context.Context, a api.Actor, id uuid.UUID, in DuplicateInput) (*View, error) {
	src, acc, err := s.locate(ctx, a, id)
	if err != nil {
		return nil, err
	}
	if err := acc.Require(api.TaskCreate); err != nil {
		return nil, err
	}
	var newID uuid.UUID
	var warns []string
	err = s.inTx(ctx, func(tx *gorm.DB) error {
		var e error
		newID, warns, e = s.copyTask(unit{ctx, tx, a}, src, in)
		return e
	})
	if err != nil {
		return nil, err
	}
	v, err := s.view(ctx, s.db, a.TenantID, newID)
	if v != nil {
		v.Warnings = warns
	}
	return v, err
}

func copyTitle(title string) string {
	r := []rune(title)
	if max := maxTitle - runes(copySuffix); len(r) > max {
		r = r[:max]
	}
	return strings.TrimSpace(string(r)) + copySuffix
}

func (s *Service) copyTask(u unit, src *Task, in DuplicateInput) (uuid.UUID, []string, error) {
	tx, a := u.tx, u.a
	if err := lockColumn(tx, src.ColumnID); err != nil {
		return uuid.Nil, nil, err
	}
	col, err := column(tx, a.TenantID, src.ProjectID, src.ColumnID)
	if err != nil {
		return uuid.Nil, nil, err
	}
	warns, err := wipCheck(tx, a.TenantID, col, uuid.Nil)
	if err != nil {
		return uuid.Nil, nil, err
	}
	number, err := nextNumber(tx, a.TenantID, src.ProjectID)
	if err != nil {
		return uuid.Nil, nil, err
	}
	at := slot{}
	if src.ArchivedAt == nil {
		at.after = &src.ID
	}
	rank, err := s.allocRank(tx, colPlace{a.TenantID, col.ID, uuid.Nil}, at)
	if err != nil {
		return uuid.Nil, nil, err
	}
	now := s.now().UTC()
	t := *src
	t.ID, t.Number, t.Title, t.Rank, t.Version = uuid.New(), number, copyTitle(src.Title), rank, 1
	t.ReporterUserID, t.CreatedAt, t.UpdatedAt, t.ArchivedAt, t.CompletedAt = a.UserID, now, now, nil, nil
	if col.Category == api.ColumnDone {
		t.CompletedAt = &now
	}
	if err := tx.Create(&t).Error; err != nil {
		return uuid.Nil, nil, err
	}
	if err := copyLinks(u, [2]uuid.UUID{src.ID, t.ID}, in); err != nil {
		return uuid.Nil, nil, err
	}
	err = s.record(u, &t, note{event: "task.created", activity: "created", data: map[string]any{"number": number, "duplicate_of": src.ID},
		payload: map[string]any{"number": number, "column_id": t.ColumnID, "duplicate_of": src.ID}})
	return t.ID, warns, err
}

// copyLinks copies labels and the assignees who are still active project members.
// copyLinks copies labels and the assignees who are still active project members; ids is {from, to}.
func copyLinks(u unit, ids [2]uuid.UUID, in DuplicateInput) error {
	tx, a, from, to := u.tx, u.a, ids[0], ids[1]
	if in.CopyLabels {
		err := tx.Exec(`INSERT INTO task_labels (task_id, label_id, tenant_id) SELECT ?, label_id, tenant_id FROM task_labels
			WHERE task_id = ? AND tenant_id = ?`, to, from, a.TenantID).Error
		if err != nil {
			return err
		}
	}
	if !in.CopyAssignees {
		return nil
	}
	err := tx.Exec(`INSERT INTO task_assignees (task_id, user_id, tenant_id, assigned_by) SELECT ?, ta.user_id, ta.tenant_id, ?
		FROM task_assignees ta JOIN tasks t ON t.id = ta.task_id JOIN project_members m ON m.project_id = t.project_id
		AND m.user_id = ta.user_id AND m.active WHERE ta.task_id = ? AND ta.tenant_id = ? AND ta.active`, to, a.UserID, from, a.TenantID).Error
	return err
}

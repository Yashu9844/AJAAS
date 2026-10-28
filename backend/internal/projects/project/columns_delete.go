package project

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
	"gorm.io/gorm"
)

// DeleteColumn removes a column. A column that still has tasks needs moveTo, the column that receives them (PR-06).
func (s *Service) DeleteColumn(ctx context.Context, a api.Actor, projectID, id uuid.UUID, moveTo *uuid.UUID) error {
	if err := s.require(ctx, a, projectID, api.ColumnManage); err != nil {
		return err
	}
	return s.inTx(ctx, func(tx *gorm.DB) error {
		if _, err := lockProject(tx, a.TenantID, projectID); err != nil {
			return err
		}
		var col Column
		if err := tx.Where("id = ? AND project_id = ? AND tenant_id = ?", id, projectID, a.TenantID).Take(&col).Error; err != nil {
			return mapMissing(err, "column")
		}
		if col.IsDefault {
			return kernel.Conflict("DEFAULT_COLUMN", "choose another default column before deleting this one")
		}
		if err := s.moveTasksOut(tx, col, moveTo); err != nil {
			return err
		}
		if err := tx.Delete(&Column{}, "id = ?", id).Error; err != nil {
			return err
		}
		if err := ensureCategories(tx, projectID); err != nil {
			return err
		}
		s.audit(ctx, tx, a, "column.deleted", ref{"column", id, map[string]any{"project_id": projectID}})
		return nil
	})
}

func mapMissing(err error, what string) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return kernel.NotFound(what)
	}
	return err
}

// moveTasksOut moves every task of col into target, keeping completion state consistent with the target category.
func (s *Service) moveTasksOut(tx *gorm.DB, col Column, moveTo *uuid.UUID) error {
	n, err := countRows(tx, "tasks", "column_id = ?", col.ID)
	if err != nil {
		return err
	}
	if n == 0 {
		return nil
	}
	if moveTo == nil {
		return kernel.Conflict("COLUMN_NOT_EMPTY", "the column has tasks; pass move_to with the column that should receive them")
	}
	var target Column
	if err := tx.Where("id = ? AND project_id = ? AND tenant_id = ?", *moveTo, col.ProjectID, col.TenantID).Take(&target).Error; err != nil {
		return mapMissing(err, "target column")
	}
	if target.ID == col.ID {
		return kernel.Invalid("move_to", "must be a different column")
	}
	now := s.now().UTC()
	return tx.Exec(`UPDATE tasks SET column_id = ?, version = version + 1, updated_at = ?,
		completed_at = CASE WHEN ? = 'done' THEN COALESCE(completed_at, ?::timestamptz) ELSE NULL END WHERE column_id = ?`,
		target.ID, now, target.Category, now, col.ID).Error
}

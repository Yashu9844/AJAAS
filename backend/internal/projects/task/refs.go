package task

import (
	"errors"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/kernel"
	"gorm.io/gorm"
)

// colRow is the part of a board column the task context needs.
type colRow struct {
	ID        uuid.UUID
	ProjectID uuid.UUID
	Key       string
	Position  int
	Category  string
	WipLimit  *int
	WipHard   bool
	IsDefault bool
}

func (colRow) TableName() string { return "board_columns" }

func (c colRow) closed() bool { return c.Category == "done" || c.Category == "canceled" }

// column loads a column of the project; a foreign or unknown id is a validation error naming the field.
func column(tx *gorm.DB, tenantID, projectID, id uuid.UUID) (*colRow, error) {
	var c colRow
	err := tx.Where("id = ? AND tenant_id = ? AND project_id = ?", id, tenantID, projectID).Take(&c).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, kernel.Invalid("column_id", "unknown column")
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func defaultColumn(tx *gorm.DB, tenantID, projectID uuid.UUID) (*colRow, error) {
	var c colRow
	err := tx.Where("tenant_id = ? AND project_id = ? AND is_default", tenantID, projectID).Take(&c).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, kernel.Conflict("NO_DEFAULT_COLUMN", "the project has no default column")
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func count(tx *gorm.DB, sql string, args ...any) (int64, error) {
	var n int64
	err := tx.Raw(sql, args...).Scan(&n).Error
	return n, err
}

func checkMilestone(tx *gorm.DB, tenantID, projectID uuid.UUID, id *uuid.UUID) error {
	if id == nil {
		return nil
	}
	n, err := count(tx, `SELECT count(*) FROM milestones WHERE id = ? AND tenant_id = ? AND project_id = ?`, *id, tenantID, projectID)
	if err != nil {
		return err
	}
	if n == 0 {
		return kernel.Invalid("milestone_id", "unknown milestone")
	}
	return nil
}

func checkSprint(tx *gorm.DB, tenantID, projectID uuid.UUID, id *uuid.UUID) error {
	if id == nil {
		return nil
	}
	n, err := count(tx, `SELECT count(*) FROM sprints WHERE id = ? AND tenant_id = ? AND project_id = ?`, *id, tenantID, projectID)
	if err != nil {
		return err
	}
	if n == 0 {
		return kernel.Invalid("sprint_id", "unknown sprint")
	}
	return nil
}

// checkLabels accepts tenant-wide labels and labels of this project only.
func checkLabels(tx *gorm.DB, tenantID, projectID uuid.UUID, ids []uuid.UUID) error {
	if len(ids) == 0 {
		return nil
	}
	n, err := count(tx, `SELECT count(*) FROM labels WHERE tenant_id = ? AND id IN ? AND (project_id IS NULL OR project_id = ?)`,
		tenantID, ids, projectID)
	if err != nil {
		return err
	}
	if int(n) != len(ids) {
		return kernel.Invalid("label_ids", "contains an unknown label")
	}
	return nil
}

// checkAssignees requires every user to be an active member who can read tasks in the project.
func checkAssignees(tx *gorm.DB, tenantID, projectID uuid.UUID, ids []uuid.UUID) error {
	if len(ids) == 0 {
		return nil
	}
	n, err := count(tx, `SELECT count(*) FROM project_members m LEFT JOIN project_roles r ON r.id = m.custom_role_id
		WHERE m.tenant_id = ? AND m.project_id = ? AND m.active AND m.user_id IN ?
		AND (m.role_key IS NOT NULL OR r.permissions @> '["task.read"]'::jsonb)`, tenantID, projectID, ids)
	if err != nil {
		return err
	}
	if int(n) != len(ids) {
		return kernel.Invalid("assignee_ids", "every assignee must be a project member who can read tasks")
	}
	return nil
}

// checkParent returns the depth a child of parentID gets; the parent must be a live task of the project.
func checkParent(tx *gorm.DB, tenantID, projectID uuid.UUID, parentID *uuid.UUID) (int, error) {
	if parentID == nil {
		return 0, nil
	}
	var p Task
	err := tx.Where("id = ? AND tenant_id = ? AND project_id = ? AND deleted_at IS NULL", *parentID, tenantID, projectID).Take(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, kernel.Invalid("parent_task_id", "unknown task in this project")
	}
	if err != nil {
		return 0, err
	}
	if p.Depth+1 > maxDepth {
		return 0, kernel.Conflict("SUBTASK_DEPTH", "subtasks nest at most 3 levels")
	}
	return p.Depth + 1, nil
}

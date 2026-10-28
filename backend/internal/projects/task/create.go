package task

import (
	"context"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
	"gorm.io/gorm"
)

func (in *CreateInput) normalise() error {
	var err error
	if in.Title, err = cleanTitle(in.Title); err != nil {
		return err
	}
	if err = checkDescription(in.Description); err != nil {
		return err
	}
	if in.Priority == "" {
		in.Priority = "none"
	}
	if err = checkPriority(in.Priority); err != nil {
		return err
	}
	if err = checkEstimate(in.EstimateMinutes); err != nil {
		return err
	}
	if err = checkDates(in.StartOn, in.DueAt); err != nil {
		return err
	}
	if in.AssigneeIDs, err = uniqueIDs(in.AssigneeIDs, maxAssignees, "assignee_ids"); err != nil {
		return err
	}
	in.LabelIDs, err = uniqueIDs(in.LabelIDs, maxLabels, "label_ids")
	return err
}

// Create adds a task to a project with the next ticket number, appended to its column (TK-01, TK-02).
func (s *Service) Create(ctx context.Context, a api.Actor, projectID uuid.UUID, in CreateInput) (*View, error) {
	acc, err := s.access(ctx, a, projectID)
	if err != nil {
		return nil, err
	}
	if err := acc.Require(api.TaskCreate); err != nil {
		return nil, err
	}
	if err := in.normalise(); err != nil {
		return nil, err
	}
	var id uuid.UUID
	var warns []string
	err = s.inTx(ctx, func(tx *gorm.DB) error {
		var e error
		id, warns, e = s.insertTask(unit{ctx, tx, a}, projectID, in)
		return e
	})
	if isCheck(err) {
		return nil, kernel.Invalid("due_at", "must not be before start_on")
	}
	if err != nil {
		return nil, err
	}
	v, err := s.view(ctx, s.db, a.TenantID, id)
	if v != nil {
		v.Warnings = warns
	}
	return v, err
}

func (s *Service) targetColumn(tx *gorm.DB, a api.Actor, projectID uuid.UUID, id *uuid.UUID) (*colRow, error) {
	if id == nil {
		return defaultColumn(tx, a.TenantID, projectID)
	}
	return column(tx, a.TenantID, projectID, *id)
}

// checkRefs validates every id the input names against the project and tenant.
func checkRefs(tx *gorm.DB, a api.Actor, projectID uuid.UUID, in CreateInput) (int, error) {
	if err := checkMilestone(tx, a.TenantID, projectID, in.MilestoneID); err != nil {
		return 0, err
	}
	if err := checkSprint(tx, a.TenantID, projectID, in.SprintID); err != nil {
		return 0, err
	}
	if err := checkLabels(tx, a.TenantID, projectID, in.LabelIDs); err != nil {
		return 0, err
	}
	if err := checkAssignees(tx, a.TenantID, projectID, in.AssigneeIDs); err != nil {
		return 0, err
	}
	return checkParent(tx, a.TenantID, projectID, in.ParentTaskID)
}

func (s *Service) insertTask(u unit, projectID uuid.UUID, in CreateInput) (uuid.UUID, []string, error) {
	tx, a := u.tx, u.a
	col, err := s.targetColumn(tx, a, projectID, in.ColumnID)
	if err != nil {
		return uuid.Nil, nil, err
	}
	if err := lockColumn(tx, col.ID); err != nil {
		return uuid.Nil, nil, err
	}
	depth, err := checkRefs(tx, a, projectID, in)
	if err != nil {
		return uuid.Nil, nil, err
	}
	warns, err := wipCheck(tx, a.TenantID, col, uuid.Nil)
	if err != nil {
		return uuid.Nil, nil, err
	}
	number, err := nextNumber(tx, a.TenantID, projectID)
	if err != nil {
		return uuid.Nil, nil, err
	}
	rank, err := s.allocRank(tx, colPlace{a.TenantID, col.ID, uuid.Nil}, slot{})
	if err != nil {
		return uuid.Nil, nil, err
	}
	now := s.now().UTC()
	t := Task{ID: uuid.New(), TenantID: a.TenantID, ProjectID: projectID, Number: number, ParentTaskID: in.ParentTaskID, Depth: depth,
		Title: in.Title, Description: in.Description, ColumnID: col.ID, Priority: in.Priority, ReporterUserID: a.UserID, StartOn: in.StartOn,
		DueAt: in.DueAt, EstimateMinutes: in.EstimateMinutes, MilestoneID: in.MilestoneID, SprintID: in.SprintID, Rank: rank, Version: 1,
		CreatedAt: now, UpdatedAt: now}
	if col.Category == api.ColumnDone {
		t.CompletedAt = &now
	}
	if err := tx.Create(&t).Error; err != nil {
		return uuid.Nil, nil, err
	}
	if err := s.writeCreated(u, &t, in); err != nil {
		return uuid.Nil, nil, err
	}
	return t.ID, warns, nil
}

// writeCreated stores assignees and labels and records the created (and assigned) notes.
func (s *Service) writeCreated(u unit, t *Task, in CreateInput) error {
	tx, a := u.tx, u.a
	if _, err := addAssignees(tx, a, t.ID, in.AssigneeIDs); err != nil {
		return err
	}
	if _, err := addLabels(tx, a.TenantID, t.ID, in.LabelIDs); err != nil {
		return err
	}
	if err := s.afterCreate(u, t, in.AssigneeIDs); err != nil {
		return err
	}
	notes := []note{{event: "task.created", activity: "created", data: map[string]any{"number": t.Number, "column_id": t.ColumnID,
		"priority": t.Priority}, payload: map[string]any{"number": t.Number, "column_id": t.ColumnID}}}
	if len(in.AssigneeIDs) > 0 {
		notes = append(notes, note{event: "task.assigned", activity: "assigned", data: map[string]any{"user_ids": in.AssigneeIDs},
			payload: map[string]any{"user_ids": in.AssigneeIDs}})
	}
	for _, n := range notes {
		if err := s.record(u, t, n); err != nil {
			return err
		}
	}
	return nil
}

// nextNumber allocates the project-scoped ticket number atomically; a rollback releases it, so there are no gaps (PG5).
func nextNumber(tx *gorm.DB, tenantID, projectID uuid.UUID) (int, error) {
	var n int
	err := tx.Raw(`UPDATE projects SET task_counter = task_counter + 1 WHERE id = ? AND tenant_id = ? AND deleted_at IS NULL
		RETURNING task_counter`, projectID, tenantID).Scan(&n).Error
	if err != nil {
		return 0, err
	}
	if n == 0 {
		return 0, kernel.NotFound("project")
	}
	return n, nil
}

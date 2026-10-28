package planning

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type sprintCursor struct {
	At string `json:"at"`
	ID string `json:"id"`
}

// loadSprint finds a sprint in the caller tenant, resolves the project access (404 when the caller cannot read the
// project, PG4), then checks perm (403/409) and the scrum mode (409). An empty perm skips the permission check.
func (s *Service) loadSprint(ctx context.Context, a api.Actor, id uuid.UUID, perm api.Perm) (*Sprint, api.Access, error) {
	var sp Sprint
	err := s.db.WithContext(ctx).Where("id = ? AND tenant_id = ?", id, a.TenantID).Take(&sp).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, api.Access{}, kernel.NotFound("sprint")
	}
	if err != nil {
		return nil, api.Access{}, err
	}
	acc, err := s.access(ctx, a, sp.ProjectID)
	if err != nil {
		return nil, api.Access{}, err
	}
	if perm != "" {
		if err := acc.Require(perm); err != nil {
			return nil, api.Access{}, err
		}
	}
	return &sp, acc, requireScrum(acc)
}

// lockSprint re-reads the sprint with a row lock inside tx so state transitions serialise.
func lockSprint(tx *gorm.DB, tenantID, id uuid.UUID) (*Sprint, error) {
	var sp Sprint
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND tenant_id = ?", id, tenantID).Take(&sp).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, kernel.NotFound("sprint")
	}
	return &sp, err
}

func (in SprintInput) validateNew() (Sprint, error) {
	var sp Sprint
	if in.Name == nil {
		return sp, kernel.Invalid("name", "is required")
	}
	return sp, in.apply(&sp)
}

// apply copies the set fields onto sp and checks the resulting shape.
func (in SprintInput) apply(sp *Sprint) error {
	var err error
	if in.Name != nil {
		if sp.Name, err = text("name", *in.Name, 1, maxName); err != nil {
			return err
		}
	}
	if in.Goal != nil {
		if tooLong(*in.Goal, maxGoal) {
			return kernel.Invalid("goal", "is too long")
		}
		sp.Goal = *in.Goal
	}
	if in.StartOn != nil {
		sp.StartOn = in.StartOn
	}
	if in.EndOn != nil {
		sp.EndOn = in.EndOn
	}
	if sp.StartOn != nil && sp.EndOn != nil && sp.EndOn.Before(*sp.StartOn) {
		return kernel.Invalid("end_on", "must not be before start_on")
	}
	return nil
}

// CreateSprint adds a planned sprint to a scrum project (SP-01, sprint.manage).
func (s *Service) CreateSprint(ctx context.Context, a api.Actor, projectID uuid.UUID, in SprintInput) (*Sprint, error) {
	acc, err := s.access(ctx, a, projectID)
	if err != nil {
		return nil, err
	}
	if err := acc.Require(api.SprintManage); err != nil {
		return nil, err
	}
	if err := requireScrum(acc); err != nil {
		return nil, err
	}
	sp, err := in.validateNew()
	if err != nil {
		return nil, err
	}
	now := s.now().UTC()
	sp.ID, sp.TenantID, sp.ProjectID, sp.Status, sp.Version = uuid.New(), a.TenantID, projectID, SprintPlanned, 1
	sp.CreatedBy, sp.CreatedAt, sp.UpdatedAt = a.UserID, now, now
	err = s.inTx(ctx, func(tx *gorm.DB) error {
		if err := tx.Create(&sp).Error; err != nil {
			return err
		}
		s.audit(ctx, tx, a, "sprint.created", ref{"sprint", sp.ID})
		return nil
	})
	if isDup(err) {
		return nil, kernel.Conflict("SPRINT_NAME_TAKEN", "a sprint with this name already exists in the project")
	}
	return &sp, err
}

// ListSprints returns the sprints of a project, newest first, with their task counts (project.read).
func (s *Service) ListSprints(ctx context.Context, a api.Actor, projectID uuid.UUID, f SprintFilter) ([]Sprint, string, error) {
	acc, err := s.access(ctx, a, projectID)
	if err != nil {
		return nil, "", err
	}
	if err := requireScrum(acc); err != nil {
		return nil, "", err
	}
	var cur sprintCursor
	if err := kernel.DecodeCursor(f.Cursor, &cur); err != nil {
		return nil, "", err
	}
	if f.Status != "" && f.Status != SprintPlanned && f.Status != SprintActive && f.Status != SprintCompleted {
		return nil, "", kernel.Invalid("status", "must be planned, active or completed")
	}
	where, args := sprintWhere(a, projectID, f, cur)
	var rows []Sprint
	err = s.db.WithContext(ctx).Raw(`SELECT s.*, (SELECT count(*) FROM tasks t WHERE t.tenant_id = s.tenant_id AND t.sprint_id = s.id
		AND t.deleted_at IS NULL) AS task_count FROM sprints s WHERE `+where+` ORDER BY s.created_at DESC, s.id DESC LIMIT ?`,
		append(args, f.Limit+1)...).Scan(&rows).Error
	if err != nil {
		return nil, "", err
	}
	rows, more := kernel.Trim(rows, f.Limit)
	next := ""
	if more {
		last := rows[len(rows)-1]
		next = kernel.EncodeCursor(sprintCursor{At: last.CreatedAt.UTC().Format(time.RFC3339Nano), ID: last.ID.String()})
	}
	return rows, next, nil
}

func sprintWhere(a api.Actor, projectID uuid.UUID, f SprintFilter, cur sprintCursor) (string, []any) {
	where, args := `s.tenant_id = ? AND s.project_id = ?`, []any{a.TenantID, projectID}
	if f.Status != "" {
		where += ` AND s.status = ?`
		args = append(args, f.Status)
	}
	if cur.ID != "" {
		where += ` AND (s.created_at, s.id) < (?::timestamptz, ?::uuid)`
		args = append(args, cur.At, cur.ID)
	}
	return where, args
}

// GetSprint returns one sprint with its task count (project.read).
func (s *Service) GetSprint(ctx context.Context, a api.Actor, id uuid.UUID) (*Sprint, error) {
	sp, _, err := s.loadSprint(ctx, a, id, "")
	if err != nil {
		return nil, err
	}
	var n int64
	err = s.db.WithContext(ctx).Raw(`SELECT count(*) FROM tasks WHERE tenant_id = ? AND sprint_id = ? AND deleted_at IS NULL`, a.TenantID, id).Scan(&n).Error
	sp.TaskCount = int(n)
	return sp, err
}

// UpdateSprint edits a planned or active sprint; completed sprints are frozen (sprint.manage, optimistic version).
func (s *Service) UpdateSprint(ctx context.Context, a api.Actor, id uuid.UUID, in SprintInput) (*Sprint, error) {
	if _, _, err := s.loadSprint(ctx, a, id, api.SprintManage); err != nil {
		return nil, err
	}
	var out *Sprint
	err := s.inTx(ctx, func(tx *gorm.DB) error {
		sp, err := lockSprint(tx, a.TenantID, id)
		if err != nil {
			return err
		}
		if in.Version != nil && *in.Version != sp.Version {
			return &kernel.VersionConflict{Current: sp.Version}
		}
		if sp.Status == SprintCompleted {
			return kernel.Conflict("SPRINT_STATE", "a completed sprint cannot be edited")
		}
		if err := in.apply(sp); err != nil {
			return err
		}
		sp.Version++
		sp.UpdatedAt = s.now().UTC()
		if err := tx.Save(sp).Error; err != nil {
			return err
		}
		s.audit(ctx, tx, a, "sprint.updated", ref{"sprint", id})
		out = sp
		return nil
	})
	if isDup(err) {
		return nil, kernel.Conflict("SPRINT_NAME_TAKEN", "a sprint with this name already exists in the project")
	}
	return out, err
}

// DeleteSprint removes a planned sprint; its tasks return to the backlog (sprint.manage). Only the sprint_id column of
// those tasks changes.
func (s *Service) DeleteSprint(ctx context.Context, a api.Actor, id uuid.UUID) (int, error) {
	if _, _, err := s.loadSprint(ctx, a, id, api.SprintManage); err != nil {
		return 0, err
	}
	released := 0
	err := s.inTx(ctx, func(tx *gorm.DB) error {
		sp, err := lockSprint(tx, a.TenantID, id)
		if err != nil {
			return err
		}
		if sp.Status != SprintPlanned {
			return kernel.Conflict("SPRINT_STATE", "only a planned sprint can be deleted")
		}
		res := tx.Exec(`UPDATE tasks SET sprint_id = NULL, version = version + 1, updated_at = ? WHERE tenant_id = ? AND sprint_id = ?`,
			s.now().UTC(), a.TenantID, id)
		if res.Error != nil {
			return res.Error
		}
		released = int(res.RowsAffected)
		if err := tx.Exec(`DELETE FROM sprints WHERE id = ? AND tenant_id = ?`, id, a.TenantID).Error; err != nil {
			return err
		}
		s.audit(ctx, tx, a, "sprint.deleted", ref{"sprint", id})
		return kernel.LogActivity(ctx, tx, kernel.Activity{TenantID: a.TenantID, ProjectID: sp.ProjectID, ActorID: &a.UserID,
			Type: "sprint.deleted", Data: map[string]any{"sprint_id": id, "released_tasks": released}})
	})
	return released, err
}

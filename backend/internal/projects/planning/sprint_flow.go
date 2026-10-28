package planning

import (
	"context"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
	"gorm.io/gorm"
)

// StartSprint makes a planned sprint with dates the active one. A second active sprint in the project is refused by the
// partial unique index, so concurrent starts cannot both win (SP-01, SP-02, PG6).
func (s *Service) StartSprint(ctx context.Context, a api.Actor, id uuid.UUID) (*Sprint, error) {
	if _, _, err := s.loadSprint(ctx, a, id, api.SprintManage); err != nil {
		return nil, err
	}
	var out *Sprint
	err := s.inTx(ctx, func(tx *gorm.DB) error {
		sp, err := lockSprint(tx, a.TenantID, id)
		if err != nil {
			return err
		}
		if sp.Status != SprintPlanned {
			return kernel.Conflict("SPRINT_STATE", "only a planned sprint can be started")
		}
		if sp.StartOn == nil || sp.EndOn == nil {
			return kernel.Conflict("SPRINT_DATES", "set start_on and end_on before starting the sprint")
		}
		now := s.now().UTC()
		sp.Status, sp.StartedAt, sp.Version, sp.UpdatedAt = SprintActive, &now, sp.Version+1, now
		if err := tx.Save(sp).Error; err != nil {
			return err
		}
		s.audit(ctx, tx, a, "sprint.started", ref{"sprint", id})
		out = sp
		if err := kernel.LogActivity(ctx, tx, kernel.Activity{TenantID: a.TenantID, ProjectID: sp.ProjectID, ActorID: &a.UserID,
			Type: "sprint.started", Data: map[string]any{"sprint_id": id}}); err != nil {
			return err
		}
		return kernel.Emit(ctx, tx, a.TenantID, "sprint.started", map[string]any{"project_id": sp.ProjectID, "sprint_id": id, "version": sp.Version})
	})
	if isDup(err) {
		return nil, kernel.Conflict("SPRINT_ACTIVE", "the project already has an active sprint")
	}
	return out, err
}

// CompleteSprint closes an active sprint: unfinished tasks (column category not done or canceled) move to the next
// planned sprint or the backlog, and the commitment report is stored (SP-02). Only tasks.sprint_id changes.
func (s *Service) CompleteSprint(ctx context.Context, a api.Actor, id uuid.UUID, in CompleteInput) (*Sprint, error) {
	if _, _, err := s.loadSprint(ctx, a, id, api.SprintManage); err != nil {
		return nil, err
	}
	var out *Sprint
	err := s.inTx(ctx, func(tx *gorm.DB) error {
		sp, err := lockSprint(tx, a.TenantID, id)
		if err != nil {
			return err
		}
		if sp.Status != SprintActive {
			return kernel.Conflict("SPRINT_STATE", "only an active sprint can be completed")
		}
		target, err := s.resolveTarget(tx, a, sp, in.MoveUnfinishedTo)
		if err != nil {
			return err
		}
		rep, err := sprintStats(tx, a.TenantID, id)
		if err != nil {
			return err
		}
		if rep.MovedUnfinishedCount, err = s.moveUnfinished(tx, a.TenantID, id, target); err != nil {
			return err
		}
		rep.MovedTo = in.MoveUnfinishedTo
		now := s.now().UTC()
		sp.Status, sp.CompletedAt, sp.Report, sp.Version, sp.UpdatedAt = SprintCompleted, &now, &rep, sp.Version+1, now
		if err := tx.Save(sp).Error; err != nil {
			return err
		}
		out = sp
		return s.finishCompletion(ctx, tx, a, sp)
	})
	return out, err
}

func (s *Service) finishCompletion(ctx context.Context, tx *gorm.DB, a api.Actor, sp *Sprint) error {
	s.audit(ctx, tx, a, "sprint.completed", ref{"sprint", sp.ID})
	if err := kernel.LogActivity(ctx, tx, kernel.Activity{TenantID: a.TenantID, ProjectID: sp.ProjectID, ActorID: &a.UserID,
		Type: "sprint.completed", Data: map[string]any{"sprint_id": sp.ID, "moved": sp.Report.MovedUnfinishedCount}}); err != nil {
		return err
	}
	return kernel.Emit(ctx, tx, a.TenantID, "sprint.completed", map[string]any{"project_id": sp.ProjectID, "sprint_id": sp.ID, "version": sp.Version})
}

// resolveTarget validates move_unfinished_to: "backlog" (nil) or a planned sprint of the same project.
func (s *Service) resolveTarget(tx *gorm.DB, a api.Actor, from *Sprint, raw string) (*uuid.UUID, error) {
	if raw == "backlog" {
		return nil, nil
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return nil, kernel.Invalid("move_unfinished_to", "must be backlog or a sprint id")
	}
	var n int64
	err = tx.Raw(`SELECT count(*) FROM sprints WHERE id = ? AND tenant_id = ? AND project_id = ? AND status = ? AND id <> ?`,
		id, a.TenantID, from.ProjectID, SprintPlanned, from.ID).Scan(&n).Error
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, kernel.Conflict("SPRINT_TARGET", "the target must be another planned sprint of the same project")
	}
	return &id, nil
}

// sprintStats computes the commitment numbers from the tasks currently in the sprint (SP-02).
func sprintStats(tx *gorm.DB, tenantID, sprintID uuid.UUID) (Report, error) {
	var rep Report
	err := tx.Raw(`SELECT count(*) AS committed_count,
			count(*) FILTER (WHERE c.category = 'done') AS completed_count,
			COALESCE(sum(t.estimate_minutes), 0) AS committed_estimate_minutes,
			COALESCE(sum(t.estimate_minutes) FILTER (WHERE c.category = 'done'), 0) AS completed_estimate_minutes
		FROM tasks t JOIN board_columns c ON c.id = t.column_id
		WHERE t.tenant_id = ? AND t.sprint_id = ? AND t.deleted_at IS NULL`, tenantID, sprintID).Scan(&rep).Error
	return rep, err
}

// moveUnfinished re-points unfinished tasks to target (nil = backlog) and bumps their version.
func (s *Service) moveUnfinished(tx *gorm.DB, tenantID, from uuid.UUID, target *uuid.UUID) (int, error) {
	res := tx.Exec(`UPDATE tasks t SET sprint_id = ?, version = t.version + 1, updated_at = ? FROM board_columns c
		WHERE c.id = t.column_id AND t.tenant_id = ? AND t.sprint_id = ? AND t.deleted_at IS NULL
		AND c.category NOT IN ('done', 'canceled')`, target, s.now().UTC(), tenantID, from)
	return int(res.RowsAffected), res.Error
}

// SprintReport returns the stored report of a completed sprint, or the live numbers for a planned or active one.
func (s *Service) SprintReport(ctx context.Context, a api.Actor, id uuid.UUID) (*ReportView, error) {
	sp, _, err := s.loadSprint(ctx, a, id, "")
	if err != nil {
		return nil, err
	}
	if sp.Status == SprintCompleted && sp.Report != nil {
		return &ReportView{SprintID: id, Status: sp.Status, Report: *sp.Report}, nil
	}
	rep, err := sprintStats(s.db.WithContext(ctx), a.TenantID, id)
	return &ReportView{SprintID: id, Status: sp.Status, Live: true, Report: rep}, err
}

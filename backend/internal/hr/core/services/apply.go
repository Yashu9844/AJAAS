package services

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/hr/core/dto"
	"github.com/jaas/jaas/internal/hr/core/events"
	"github.com/jaas/jaas/internal/hr/core/lifecycle"
	"github.com/jaas/jaas/internal/hr/core/models"
	employeeAPI "github.com/jaas/jaas/internal/hr/employee/api"
	"gorm.io/gorm"
)

// sweepBatch bounds one effective-date sweep (CO-041).
const sweepBatch = 500

// Approve completes a submitted action (CO-030, CO-035): re-plan under the worker lock, close and open assignments,
// and apply the effects at once when they are due (DC-06). The initiator cannot approve (DC-02).
func (s *ActionService) Approve(ctx context.Context, a Actor, id uuid.UUID, req dto.DecisionRequest) (*dto.ActionResponse, error) {
	comment, err := optionalText("comment", req.Comment, maxDecision)
	if err != nil {
		return nil, err
	}
	var act *models.Action
	var row *models.OutboxEvent
	err = s.Tx.InTx(ctx, func(tx *gorm.DB) error {
		if act, err = s.Repo.LockAction(ctx, tx, a.TenantID, id); err != nil || act == nil {
			return orNotFound(err)
		}
		if !lifecycle.CanMove(act.Status, lifecycle.Completed) {
			return ErrInvalidState
		}
		if act.InitiatedByUserID == a.UserID {
			return ErrSelfApproval
		}
		if err := s.Repo.LockWorker(ctx, tx, act.EmployeeID); err != nil {
			return err
		}
		out, err := s.plan(ctx, tx, act)
		if err != nil {
			return err
		}
		if err := s.write(ctx, tx, act, out); err != nil {
			return err
		}
		now := s.Now().UTC()
		act.Status, act.CompletedAt, act.DecidedByUserID, act.DecisionComment, act.UpdatedAt = lifecycle.Completed, &now, &a.UserID, comment, now
		if !out.effectsDate.After(s.today()) {
			if err := s.applyEffects(ctx, tx, a, act); err != nil {
				return err
			}
		}
		if err := s.Repo.Save(ctx, tx, act); err != nil {
			return err
		}
		row, err = s.actionEvent(ctx, tx, a, act, events.ActionCompleted)
		return err
	})
	if err != nil {
		return nil, err
	}
	s.afterCommit(ctx, a, s.audit(act, "completed"), row)
	return actionResponse(act), nil
}

// write closes the current assignment and opens the next one (CO-035, CO-037), remembering what to restore.
func (b base) write(ctx context.Context, tx *gorm.DB, act *models.Action, out *outcome) error {
	now := b.Now().UTC()
	if out.current != nil {
		act.PreviousAssignmentID, act.PreviousEffectiveTo = &out.current.ID, out.current.EffectiveTo
		closeAt := out.closeAt
		out.current.EffectiveTo, out.current.UpdatedAt = &closeAt, now
		if err := b.Repo.Save(ctx, tx, out.current); err != nil {
			return err
		}
	}
	if out.next != nil {
		out.next.ActionID, out.next.CreatedAt, out.next.UpdatedAt = act.ID, now, now
		if err := b.Repo.Create(ctx, tx, out.next); err != nil {
			return err
		}
		act.NewAssignmentID = &out.next.ID
	}
	return nil
}

// Rescind is CO-039: only the worker's latest completed action; assignments and applied effects are restored.
func (s *ActionService) Rescind(ctx context.Context, a Actor, id uuid.UUID, req dto.DecisionRequest) (*dto.ActionResponse, error) {
	if req.Reason == nil {
		return nil, invalid("reason", "is required")
	}
	reason, err := text("reason", *req.Reason, maxDecision)
	if err != nil {
		return nil, err
	}
	var act *models.Action
	var row *models.OutboxEvent
	err = s.Tx.InTx(ctx, func(tx *gorm.DB) error {
		if act, err = s.Repo.LockAction(ctx, tx, a.TenantID, id); err != nil || act == nil {
			return orNotFound(err)
		}
		if !lifecycle.CanMove(act.Status, lifecycle.Rescinded) {
			return ErrInvalidState
		}
		if err := s.Repo.LockWorker(ctx, tx, act.EmployeeID); err != nil {
			return err
		}
		done, err := s.Repo.CompletedActions(ctx, tx, a.TenantID, act.EmployeeID)
		if err != nil {
			return err
		}
		if len(done) == 0 || done[0].ID != act.ID {
			return ErrNotLatest
		}
		if err := s.restore(ctx, tx, act); err != nil {
			return err
		}
		if act.EffectsAppliedAt != nil {
			if err := s.reverseEffects(ctx, tx, a, act); err != nil {
				return err
			}
		}
		now := s.Now().UTC()
		act.Status, act.RescindedAt, act.RescindReason, act.UpdatedAt = lifecycle.Rescinded, &now, &reason, now
		if err := s.Repo.Save(ctx, tx, act); err != nil {
			return err
		}
		row, err = s.actionEvent(ctx, tx, a, act, events.ActionRescinded)
		return err
	})
	if err != nil {
		return nil, err
	}
	s.afterCommit(ctx, a, s.audit(act, "rescinded"), row)
	return actionResponse(act), nil
}

// restore rescinds the action's new assignment and gives the previous one back its old end, checking that its seat is
// still free for the restored period (a rescinded termination must not double-fill the position, CO-014).
func (b base) restore(ctx context.Context, tx *gorm.DB, act *models.Action) error {
	now := b.Now().UTC()
	if act.NewAssignmentID != nil {
		next, err := b.Repo.Assignment(ctx, tx, act.TenantID, *act.NewAssignmentID)
		if err != nil || next == nil {
			return orNotFound(err)
		}
		next.Status, next.UpdatedAt = models.AssignmentRescinded, now
		if err := b.Repo.Save(ctx, tx, next); err != nil {
			return err
		}
	}
	if act.PreviousAssignmentID == nil {
		return nil
	}
	prev, err := b.Repo.Assignment(ctx, tx, act.TenantID, *act.PreviousAssignmentID)
	if err != nil || prev == nil {
		return orNotFound(err)
	}
	if prev.EffectiveTo != nil {
		reopened := *prev
		reopened.EffectiveFrom, reopened.EffectiveTo = *prev.EffectiveTo, act.PreviousEffectiveTo
		if err := b.checkSeat(ctx, tx, act.TenantID, &reopened, prev.ID); err != nil {
			return err
		}
	}
	prev.EffectiveTo, prev.UpdatedAt = act.PreviousEffectiveTo, now
	return b.Repo.Save(ctx, tx, prev)
}

// applyEffects projects a completed action onto the worker's status, organization mapping and login (CO-040).
func (b base) applyEffects(ctx context.Context, tx *gorm.DB, a Actor, act *models.Action) error {
	worker, err := b.employee(ctx, act.TenantID, act.EmployeeID)
	if err != nil {
		return err
	}
	var next *models.Assignment
	if act.NewAssignmentID != nil {
		if next, err = b.Repo.Assignment(ctx, tx, act.TenantID, *act.NewAssignmentID); err != nil || next == nil {
			return orNotFound(err)
		}
	}
	setStatus := func(change employeeAPI.StatusChange) error {
		prev, err := b.Lifecycle.ApplyCoreStatus(ctx, tx, act.TenantID, act.EmployeeID, change)
		if err == nil {
			act.PreviousEmployeeStatus = &prev
		}
		return err
	}
	switch act.Type {
	case lifecycle.Hire, lifecycle.Rehire:
		status := "active"
		if next.ProbationEndDate != nil && next.ProbationEndDate.After(b.today()) {
			status = "probation"
		}
		if err := setStatus(employeeAPI.StatusChange{Status: status, Note: "HR action " + act.Type}); err != nil {
			return err
		}
		if act.Type == lifecycle.Rehire {
			if err := conflictOK(b.Logins.ActivateUser(ctx, tx, act.TenantID, worker.UserID)); err != nil {
				return err
			}
		}
	case lifecycle.Terminate:
		lwd, _ := parseDate("last_working_day", *act.Changes.LastWorkingDay)
		if err := setStatus(employeeAPI.StatusChange{Status: "terminated", ExitDate: &lwd, ExitReason: act.ReasonCode, Note: "HR action terminate"}); err != nil {
			return err
		}
		if err := conflictOK(b.Logins.DeactivateUser(ctx, tx, act.TenantID, worker.UserID, a.CorrelationID)); err != nil {
			return err
		}
	case lifecycle.ProbationConfirm:
		if next.ProbationEndDate == nil {
			d := act.EffectiveDate
			if err := setStatus(employeeAPI.StatusChange{Status: "active", ConfirmationDate: &d, Note: "Probation confirmed"}); err != nil {
				return err
			}
		}
	}
	if next != nil {
		if err := b.project(ctx, tx, act.TenantID, worker.UserID, next); err != nil {
			return err
		}
	}
	now := b.Now().UTC()
	act.EffectsAppliedAt = &now
	return nil
}

// reverseEffects undoes applied effects when an action is rescinded (CO-039, CG9).
func (b base) reverseEffects(ctx context.Context, tx *gorm.DB, a Actor, act *models.Action) error {
	worker, err := b.employee(ctx, act.TenantID, act.EmployeeID)
	if err != nil {
		return err
	}
	restoreStatus := func(fallback string) error {
		st := fallback
		if act.PreviousEmployeeStatus != nil && *act.PreviousEmployeeStatus != "" {
			st = *act.PreviousEmployeeStatus
		}
		_, err := b.Lifecycle.ApplyCoreStatus(ctx, tx, act.TenantID, act.EmployeeID, employeeAPI.StatusChange{Status: st, Note: "HR action " + act.Type + " rescinded"})
		return err
	}
	switch act.Type {
	case lifecycle.Terminate:
		if err := restoreStatus("active"); err != nil {
			return err
		}
		if err := conflictOK(b.Logins.ActivateUser(ctx, tx, act.TenantID, worker.UserID)); err != nil {
			return err
		}
	case lifecycle.Rehire:
		if err := restoreStatus("terminated"); err != nil {
			return err
		}
		if err := conflictOK(b.Logins.DeactivateUser(ctx, tx, act.TenantID, worker.UserID, a.CorrelationID)); err != nil {
			return err
		}
	case lifecycle.Hire, lifecycle.ProbationConfirm:
		if err := restoreStatus("probation"); err != nil {
			return err
		}
	}
	if act.PreviousAssignmentID != nil {
		prev, err := b.Repo.Assignment(ctx, tx, act.TenantID, *act.PreviousAssignmentID)
		if err != nil || prev == nil {
			return orNotFound(err)
		}
		if err := b.project(ctx, tx, act.TenantID, worker.UserID, prev); err != nil {
			return err
		}
	}
	now := b.Now().UTC()
	act.EffectsReversedAt = &now
	return nil
}

// project makes the worker's primary organization mapping match an assignment (CO-040, DC-09).
func (b base) project(ctx context.Context, tx *gorm.DB, tenantID, userID uuid.UUID, as *models.Assignment) error {
	if userID == uuid.Nil {
		return nil
	}
	var managerUser *uuid.UUID
	if as.ManagerEmployeeID != nil {
		m, err := b.employee(ctx, tenantID, *as.ManagerEmployeeID)
		if err != nil {
			return err
		}
		managerUser = &m.UserID
	}
	return b.Org.ProjectPrimaryMapping(ctx, tx, tenantID, userID, as.DepartmentID, as.LocationID, managerUser)
}

// EffectsService runs the effective-date sweep (CO-041).
type EffectsService struct{ base }

// NewEffectsService builds the service.
func NewEffectsService(d Deps) *EffectsService { return &EffectsService{base{d}} }

// effectsDate is when an action's projections are due: a termination after the last working day, others on their date.
func effectsDate(act *models.Action) time.Time {
	if act.Type == lifecycle.Terminate && act.Changes.LastWorkingDay != nil {
		if lwd, err := parseDate("last_working_day", *act.Changes.LastWorkingDay); err == nil {
			return lwd.AddDate(0, 0, 1)
		}
	}
	return act.EffectiveDate
}

// Sweep applies due effects of completed actions; tenant nil sweeps every tenant (background job). It is idempotent.
func (s *EffectsService) Sweep(ctx context.Context, tenant *uuid.UUID) (*dto.RunResponse, error) {
	today := s.today()
	due, err := s.Repo.PendingEffects(ctx, s.Tx.DB(), today, sweepBatch)
	if err != nil {
		return nil, err
	}
	out := &dto.RunResponse{}
	for i := range due {
		act := due[i]
		if (tenant != nil && act.TenantID != *tenant) || effectsDate(&act).After(today) {
			out.Skipped++
			continue
		}
		a := Actor{TenantID: act.TenantID, CorrelationID: uuid.New()}
		applied := false
		err := s.Tx.InTx(ctx, func(tx *gorm.DB) error {
			cur, err := s.Repo.LockAction(ctx, tx, act.TenantID, act.ID)
			if err != nil || cur == nil || cur.Status != lifecycle.Completed || cur.EffectsAppliedAt != nil {
				return err
			}
			if err := s.Repo.LockWorker(ctx, tx, cur.EmployeeID); err != nil {
				return err
			}
			if err := s.applyEffects(ctx, tx, a, cur); err != nil {
				return err
			}
			applied = true
			return s.Repo.Save(ctx, tx, cur)
		})
		if err != nil {
			return out, err
		}
		if applied {
			out.Processed++
			s.afterCommit(ctx, a, auditEntry{"core.action_effects_applied", "core_hr_action", act.ID.String(),
				map[string]string{"type": act.Type, "employee_id": act.EmployeeID.String()}})
		}
	}
	return out, nil
}

// Run ticks Sweep every interval until ctx is cancelled (also once at start).
func (s *EffectsService) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		_, _ = s.Sweep(ctx, nil)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

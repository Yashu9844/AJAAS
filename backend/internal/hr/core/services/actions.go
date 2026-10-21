package services

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/hr/core/dto"
	"github.com/jaas/jaas/internal/hr/core/events"
	"github.com/jaas/jaas/internal/hr/core/lifecycle"
	"github.com/jaas/jaas/internal/hr/core/models"
	"github.com/jaas/jaas/internal/hr/core/repositories"
	"gorm.io/gorm"
)

// Action text limits.
const (
	maxReasonCode = 50
	maxComment    = 2000
	maxDecision   = 1000
)

// ActionService implements HR actions (CO-030..CO-039).
type ActionService struct{ base }

// NewActionService builds the service.
func NewActionService(d Deps) *ActionService { return &ActionService{base{d}} }

// Create is CO-031: a draft action.
func (s *ActionService) Create(ctx context.Context, a Actor, req dto.CreateActionRequest) (*dto.ActionResponse, error) {
	if !lifecycle.ValidType(req.Type) {
		return nil, invalid("type", "is not a known HR action type")
	}
	emp, err := parseUUIDField("employee_id", req.EmployeeID)
	if err != nil {
		return nil, err
	}
	if _, err := s.employee(ctx, a.TenantID, emp); err != nil {
		return nil, err
	}
	act := &models.Action{TenantID: a.TenantID, Type: req.Type, EmployeeID: emp, Status: lifecycle.Draft, Changes: req.Changes,
		InitiatedByUserID: a.UserID}
	if err := s.edit(act, &req.EffectiveDate, &req.ReasonCode, req.Comment, nil); err != nil {
		return nil, err
	}
	now := s.Now().UTC()
	act.CreatedAt, act.UpdatedAt = now, now
	if err := s.Tx.InTx(ctx, func(tx *gorm.DB) error { return s.Repo.Create(ctx, tx, act) }); err != nil {
		return nil, err
	}
	s.afterCommit(ctx, a, s.audit(act, "created"))
	return actionResponse(act), nil
}

// edit applies the editable fields of a draft.
func (s *ActionService) edit(act *models.Action, date, reason, comment *string, changes *models.Changes) error {
	if date != nil {
		d, err := parseDate("effective_date", *date)
		if err != nil {
			return err
		}
		act.EffectiveDate = d
	}
	if reason != nil {
		r, err := text("reason_code", *reason, maxReasonCode)
		if err != nil {
			return err
		}
		act.ReasonCode = strings.ToLower(r)
	}
	if comment != nil {
		c, err := optionalText("comment", comment, maxComment)
		if err != nil {
			return err
		}
		act.Comment = c
	}
	if changes != nil {
		act.Changes = *changes
	}
	return nil
}

// Update is CO-031: only drafts are edited.
func (s *ActionService) Update(ctx context.Context, a Actor, id uuid.UUID, req dto.UpdateActionRequest) (*dto.ActionResponse, error) {
	act, err := s.transition(ctx, a, id, "", func(tx *gorm.DB, act *models.Action) error {
		if act.Status != lifecycle.Draft {
			return ErrInvalidState
		}
		return s.edit(act, req.EffectiveDate, req.ReasonCode, req.Comment, req.Changes)
	})
	if err != nil {
		return nil, err
	}
	return actionResponse(act), nil
}

// Submit is CO-031: full validation, then submitted.
func (s *ActionService) Submit(ctx context.Context, a Actor, id uuid.UUID) (*dto.ActionResponse, error) {
	act, err := s.transition(ctx, a, id, lifecycle.Submitted, func(tx *gorm.DB, act *models.Action) error {
		if _, err := s.plan(ctx, tx, act); err != nil {
			return err
		}
		now := s.Now().UTC()
		act.SubmittedAt = &now
		return nil
	})
	if err != nil {
		return nil, err
	}
	return actionResponse(act), nil
}

// Cancel moves a draft or submitted action to cancelled.
func (s *ActionService) Cancel(ctx context.Context, a Actor, id uuid.UUID) (*dto.ActionResponse, error) {
	act, err := s.transition(ctx, a, id, lifecycle.Cancelled, nil)
	if err != nil {
		return nil, err
	}
	return actionResponse(act), nil
}

// Reject is a decision on a submitted action; the initiator cannot decide (CORE_SELF_APPROVAL).
func (s *ActionService) Reject(ctx context.Context, a Actor, id uuid.UUID, req dto.DecisionRequest) (*dto.ActionResponse, error) {
	comment, err := optionalText("comment", req.Comment, maxDecision)
	if err != nil {
		return nil, err
	}
	act, err := s.transition(ctx, a, id, lifecycle.Rejected, func(tx *gorm.DB, act *models.Action) error {
		if act.InitiatedByUserID == a.UserID {
			return ErrSelfApproval
		}
		act.DecidedByUserID, act.DecisionComment = &a.UserID, comment
		return nil
	})
	if err != nil {
		return nil, err
	}
	return actionResponse(act), nil
}

// transition locks the action, checks the move (to "" = stay), runs fn, saves and audits (plus the submitted event).
func (s *ActionService) transition(ctx context.Context, a Actor, id uuid.UUID, to string, fn func(*gorm.DB, *models.Action) error) (*models.Action, error) {
	var act *models.Action
	var row *models.OutboxEvent
	err := s.Tx.InTx(ctx, func(tx *gorm.DB) error {
		var err error
		if act, err = s.Repo.LockAction(ctx, tx, a.TenantID, id); err != nil || act == nil {
			return orNotFound(err)
		}
		if to != "" && !lifecycle.CanMove(act.Status, to) {
			return ErrInvalidState
		}
		if fn != nil {
			if err := fn(tx, act); err != nil {
				return err
			}
		}
		if to != "" {
			act.Status = to
		}
		act.UpdatedAt = s.Now().UTC()
		if err := s.Repo.Save(ctx, tx, act); err != nil {
			return err
		}
		if to == lifecycle.Submitted {
			row, err = s.actionEvent(ctx, tx, a, act, events.ActionSubmitted)
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	verb := to
	if verb == "" {
		verb = "updated"
	}
	s.afterCommit(ctx, a, s.audit(act, verb), row)
	return act, nil
}

func (b base) actionEvent(ctx context.Context, tx *gorm.DB, a Actor, act *models.Action, key string) (*models.OutboxEvent, error) {
	return b.writeOutbox(ctx, tx, a, key, events.ActionPayload{ActionID: act.ID, EmployeeID: act.EmployeeID, Type: act.Type, Status: act.Status,
		EffectiveDate: fmtDate(act.EffectiveDate), AssignmentID: act.NewAssignmentID})
}

// audit is the action's audit entry: ids, type, status and date only (HR-009).
func (b base) audit(act *models.Action, verb string) auditEntry {
	return auditEntry{"core.action_" + verb, "core_hr_action", act.ID.String(), map[string]string{"type": act.Type, "status": act.Status,
		"employee_id": act.EmployeeID.String(), "effective_date": fmtDate(act.EffectiveDate)}}
}

// Get returns one action.
func (s *ActionService) Get(ctx context.Context, tenantID, id uuid.UUID) (*dto.ActionResponse, error) {
	act, err := s.Repo.Action(ctx, s.Tx.DB(), tenantID, id)
	if err != nil || act == nil {
		return nil, orNotFound(err)
	}
	return actionResponse(act), nil
}

// List filters actions by worker, status and type.
func (s *ActionService) List(ctx context.Context, tenantID uuid.UUID, f repositories.ActionFilter, page dto.Page) ([]dto.ActionResponse, dto.PageMeta, error) {
	if f.Status != "" && !lifecycle.ValidStatus(f.Status) {
		return nil, dto.PageMeta{}, invalid("status", "is not a known action status")
	}
	if f.Type != "" && !lifecycle.ValidType(f.Type) {
		return nil, dto.PageMeta{}, invalid("type", "is not a known HR action type")
	}
	rows, total, err := s.Repo.Actions(ctx, s.Tx.DB(), tenantID, f, page)
	if err != nil {
		return nil, dto.PageMeta{}, err
	}
	out := make([]dto.ActionResponse, len(rows))
	for i := range rows {
		out[i] = *actionResponse(&rows[i])
	}
	return out, page.Meta(total), nil
}

func actionResponse(a *models.Action) *dto.ActionResponse {
	return &dto.ActionResponse{ID: a.ID, Type: a.Type, EmployeeID: a.EmployeeID, EffectiveDate: fmtDate(a.EffectiveDate), Status: a.Status,
		ReasonCode: a.ReasonCode, Comment: a.Comment, Changes: a.Changes, InitiatedByUserID: a.InitiatedByUserID, DecidedByUserID: a.DecidedByUserID,
		DecisionComment: a.DecisionComment, SubmittedAt: a.SubmittedAt, CompletedAt: a.CompletedAt, RescindedAt: a.RescindedAt,
		RescindReason: a.RescindReason, NewAssignmentID: a.NewAssignmentID, EffectsApplied: a.EffectsAppliedAt != nil, CreatedAt: a.CreatedAt}
}

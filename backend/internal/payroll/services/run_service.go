package services

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/payroll/dto"
	"github.com/jaas/jaas/internal/payroll/events"
	"github.com/jaas/jaas/internal/payroll/models"
	"github.com/jaas/jaas/internal/payroll/validators"
	"gorm.io/gorm"
)

// RunService runs the monthly payroll lifecycle (FR-RN001..RN006, PY-010..PY-012).
type RunService interface {
	Create(ctx context.Context, a Actor, req dto.CreateRunRequest) (*dto.RunResponse, error)
	Get(ctx context.Context, tenantID, id uuid.UUID) (*dto.RunResponse, error)
	List(ctx context.Context, tenantID uuid.UUID, status string, page dto.Page) ([]dto.RunResponse, dto.PageMeta, error)
	Calculate(ctx context.Context, a Actor, id uuid.UUID) (*dto.RunResponse, error)
	Approve(ctx context.Context, a Actor, id uuid.UUID) (*dto.RunResponse, error)
	Finalize(ctx context.Context, a Actor, id uuid.UUID) (*dto.RunResponse, error)
}

type runService struct{ base }

// NewRunService builds a RunService.
func NewRunService(d Deps) RunService { return &runService{base{d}} }

func (s *runService) Create(ctx context.Context, a Actor, req dto.CreateRunRequest) (*dto.RunResponse, error) {
	if validators.ValidatePeriod(req.Year, req.Month, s.today()) != nil {
		return nil, ErrPeriod
	}
	run := &models.Run{TenantID: a.TenantID, Year: req.Year, Month: req.Month, Status: models.RunDraft, Warnings: "[]", CreatedByUserID: a.UserID}
	var row *models.OutboxEvent
	err := s.Tx.InTx(ctx, func(tx *gorm.DB) error {
		existing, err := s.Repos.Runs.FindByPeriod(ctx, tx, a.TenantID, req.Year, req.Month)
		if err != nil {
			return err
		}
		if existing != nil {
			return ErrRunExists
		}
		if err := s.Repos.Runs.Create(ctx, tx, run); err != nil {
			return err
		}
		row, err = s.writeOutbox(ctx, tx, a, events.RunInitiated, runPayload(run))
		return err
	})
	if err != nil {
		return nil, err
	}
	s.afterCommit(ctx, a, auditEntry{"payroll.run_created", "payroll_run", run.ID.String(), periodMeta(run)}, row)
	res := mapRun(run)
	return &res, nil
}

func (s *runService) Get(ctx context.Context, tenantID, id uuid.UUID) (*dto.RunResponse, error) {
	run, err := s.Repos.Runs.FindByID(ctx, s.Tx.DB(), tenantID, id)
	if err != nil || run == nil {
		return nil, orNotFound(err)
	}
	res := mapRun(run)
	return &res, nil
}

func (s *runService) List(ctx context.Context, tenantID uuid.UUID, status string, page dto.Page) ([]dto.RunResponse, dto.PageMeta, error) {
	rows, total, err := s.Repos.Runs.List(ctx, s.Tx.DB(), tenantID, status, page)
	if err != nil {
		return nil, dto.PageMeta{}, err
	}
	out := make([]dto.RunResponse, len(rows))
	for i := range rows {
		out[i] = mapRun(&rows[i])
	}
	return out, page.Meta(total), nil
}

// Calculate (re)builds every payslip of a draft or calculated run (FR-RN002, PY-011).
func (s *runService) Calculate(ctx context.Context, a Actor, id uuid.UUID) (*dto.RunResponse, error) {
	return s.transition(ctx, a, id, transition{
		from: []string{models.RunDraft, models.RunCalculated}, key: events.RunCalculated, action: "payroll.run_calculated",
		apply: func(tx *gorm.DB, run *models.Run) error { return s.calculate(ctx, tx, a, run) },
	})
}

// Approve applies maker-checker (PY-012).
func (s *runService) Approve(ctx context.Context, a Actor, id uuid.UUID) (*dto.RunResponse, error) {
	return s.transition(ctx, a, id, transition{
		from: []string{models.RunCalculated}, key: events.RunApproved, action: "payroll.run_approved",
		apply: func(_ *gorm.DB, run *models.Run) error {
			if run.CalculatedByUserID != nil && *run.CalculatedByUserID == a.UserID {
				return ErrSelfApproval
			}
			now, approver := s.Now(), a.UserID
			run.Status, run.ApprovedByUserID, run.ApprovedAt = models.RunApproved, &approver, &now
			return nil
		},
	})
}

// Finalize locks the run and releases payslips to employees (FR-RN004).
func (s *runService) Finalize(ctx context.Context, a Actor, id uuid.UUID) (*dto.RunResponse, error) {
	return s.transition(ctx, a, id, transition{
		from: []string{models.RunApproved}, key: events.RunFinalized, action: "payroll.run_finalized",
		apply: func(_ *gorm.DB, run *models.Run) error {
			now := s.Now()
			run.Status, run.FinalizedAt = models.RunFinalized, &now
			return nil
		},
	})
}

type transition struct {
	from        []string
	key, action string
	apply       func(tx *gorm.DB, run *models.Run) error
}

// transition locks the run row, checks the source state, applies, saves and emits the event (PY-010).
func (s *runService) transition(ctx context.Context, a Actor, id uuid.UUID, tr transition) (*dto.RunResponse, error) {
	var run *models.Run
	var row *models.OutboxEvent
	err := s.Tx.InTx(ctx, func(tx *gorm.DB) error {
		var err error
		if run, err = s.Repos.Runs.LockByID(ctx, tx, a.TenantID, id); err != nil || run == nil {
			return orNotFound(err)
		}
		if !contains(tr.from, run.Status) {
			return ErrRunState
		}
		if err := tr.apply(tx, run); err != nil {
			return err
		}
		if err := s.Repos.Runs.Update(ctx, tx, run); err != nil {
			return err
		}
		row, err = s.writeOutbox(ctx, tx, a, tr.key, runPayload(run))
		return err
	})
	if err != nil {
		return nil, err
	}
	meta := periodMeta(run)
	meta["employee_count"] = fmt.Sprint(run.EmployeeCount)
	s.afterCommit(ctx, a, auditEntry{tr.action, "payroll_run", run.ID.String(), meta}, row)
	res := mapRun(run)
	return &res, nil
}

func periodMeta(r *models.Run) map[string]string {
	return map[string]string{"year": fmt.Sprint(r.Year), "month": fmt.Sprint(r.Month)}
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

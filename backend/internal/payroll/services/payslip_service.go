package services

import (
	"bytes"
	"context"
	"encoding/csv"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/payroll/dto"
	"github.com/jaas/jaas/internal/payroll/models"
	"github.com/jaas/jaas/internal/payroll/validators"
)

// PayslipService serves payslips and the payout export (FR-PS, FR-RN005, PY-013).
type PayslipService interface {
	ByRun(ctx context.Context, tenantID, runID uuid.UUID, page dto.Page) ([]dto.PayslipResponse, dto.PageMeta, error)
	Get(ctx context.Context, tenantID, id uuid.UUID) (*dto.PayslipResponse, error)
	Mine(ctx context.Context, a Actor, page dto.Page) ([]dto.PayslipResponse, dto.PageMeta, error)
	MineOne(ctx context.Context, a Actor, id uuid.UUID) (*dto.PayslipResponse, error)
	PayoutCSV(ctx context.Context, tenantID, runID uuid.UUID) ([]byte, error)
}

type payslipService struct{ base }

// NewPayslipService builds a PayslipService.
func NewPayslipService(d Deps) PayslipService { return &payslipService{base{d}} }

func (s *payslipService) run(ctx context.Context, tenantID, id uuid.UUID) (*models.Run, error) {
	r, err := s.Repos.Runs.FindByID(ctx, s.Tx.DB(), tenantID, id)
	if err != nil || r == nil {
		return nil, orNotFound(err)
	}
	return r, nil
}

func (s *payslipService) ByRun(ctx context.Context, tenantID, runID uuid.UUID, page dto.Page) ([]dto.PayslipResponse, dto.PageMeta, error) {
	run, err := s.run(ctx, tenantID, runID)
	if err != nil {
		return nil, dto.PageMeta{}, err
	}
	rows, total, err := s.Repos.Payslips.ListByRun(ctx, s.Tx.DB(), tenantID, runID, page)
	if err != nil {
		return nil, dto.PageMeta{}, err
	}
	out := make([]dto.PayslipResponse, len(rows))
	for i := range rows {
		out[i] = mapPayslip(&rows[i], run)
	}
	return out, page.Meta(total), nil
}

func (s *payslipService) Get(ctx context.Context, tenantID, id uuid.UUID) (*dto.PayslipResponse, error) {
	p, err := s.Repos.Payslips.FindByID(ctx, s.Tx.DB(), tenantID, id)
	if err != nil || p == nil {
		return nil, orNotFound(err)
	}
	run, err := s.run(ctx, tenantID, p.RunID)
	if err != nil {
		return nil, err
	}
	res := mapPayslip(p, run)
	return &res, nil
}

// Mine lists the caller's payslips of finalized runs (PY-013).
func (s *payslipService) Mine(ctx context.Context, a Actor, page dto.Page) ([]dto.PayslipResponse, dto.PageMeta, error) {
	emp, err := s.employeeFor(ctx, a)
	if err != nil {
		return nil, dto.PageMeta{}, err
	}
	rows, total, err := s.Repos.Payslips.ListFinalizedForEmployee(ctx, s.Tx.DB(), a.TenantID, emp.ID, page)
	if err != nil {
		return nil, dto.PageMeta{}, err
	}
	out := make([]dto.PayslipResponse, len(rows))
	runs := map[uuid.UUID]*models.Run{}
	for i := range rows {
		run, ok := runs[rows[i].RunID]
		if !ok {
			if run, err = s.run(ctx, a.TenantID, rows[i].RunID); err != nil {
				return nil, dto.PageMeta{}, err
			}
			runs[rows[i].RunID] = run
		}
		out[i] = mapPayslip(&rows[i], run)
	}
	return out, page.Meta(total), nil
}

// MineOne returns one of the caller's finalized payslips; anything else is 404 (PS-T1, PS-T2).
func (s *payslipService) MineOne(ctx context.Context, a Actor, id uuid.UUID) (*dto.PayslipResponse, error) {
	emp, err := s.employeeFor(ctx, a)
	if err != nil {
		return nil, err
	}
	p, err := s.Repos.Payslips.FindByID(ctx, s.Tx.DB(), a.TenantID, id)
	if err != nil || p == nil || p.EmployeeProfileID != emp.ID {
		return nil, orNotFound(err)
	}
	run, err := s.run(ctx, a.TenantID, p.RunID)
	if err != nil || run.Status != models.RunFinalized {
		return nil, orNotFound(err)
	}
	res := mapPayslip(p, run)
	return &res, nil
}

// PayoutCSV exports employee_code, employee_name, net_pay for a finalized run (FR-RN005, PS-T8).
func (s *payslipService) PayoutCSV(ctx context.Context, tenantID, runID uuid.UUID) ([]byte, error) {
	run, err := s.run(ctx, tenantID, runID)
	if err != nil {
		return nil, err
	}
	if run.Status != models.RunFinalized {
		return nil, ErrRunState
	}
	rows, err := s.Repos.Payslips.AllByRun(ctx, s.Tx.DB(), tenantID, runID)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	_ = w.Write([]string{"employee_code", "employee_name", "net_pay"})
	for _, p := range rows {
		_ = w.Write([]string{validators.CSVSafe(p.EmployeeCode), validators.CSVSafe(p.EmployeeName), p.Net.String()})
	}
	w.Flush()
	return buf.Bytes(), w.Error()
}

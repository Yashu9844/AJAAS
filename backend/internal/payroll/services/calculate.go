package services

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	employeeDTO "github.com/jaas/jaas/internal/employee/dto"
	leavecalc "github.com/jaas/jaas/internal/leave/calc"
	"github.com/jaas/jaas/internal/payroll/calc"
	"github.com/jaas/jaas/internal/payroll/models"
	"github.com/jaas/jaas/internal/payroll/repositories"
	"gorm.io/gorm"
)

// period is the run month as dates.
type monthPeriod struct {
	from, to time.Time
	days     int
}

func periodOf(run *models.Run) monthPeriod {
	from := time.Date(run.Year, time.Month(run.Month), 1, 0, 0, 0, 0, time.UTC)
	days := calc.DaysInMonth(run.Year, run.Month)
	return monthPeriod{from: from, to: from.AddDate(0, 0, days-1), days: days}
}

// calculate replaces the run's payslips (PY-011) and stores totals and warnings.
func (s *runService) calculate(ctx context.Context, tx *gorm.DB, a Actor, run *models.Run) error {
	p := periodOf(run)
	assigns, err := s.Repos.Assignments.EligibleFor(ctx, tx, run.TenantID, repositories.Period{From: p.from, To: p.to})
	if err != nil {
		return err
	}
	specs := map[uuid.UUID]calc.StructureSpec{}
	var slips []models.Payslip
	warnings := []string{}
	for _, as := range assigns {
		slip, warn, err := s.payslipFor(ctx, tx, run, p, as, specs)
		if err != nil {
			return err
		}
		if warn != "" {
			warnings = append(warnings, warn)
		}
		if slip != nil {
			slips = append(slips, *slip)
		}
	}
	if err := s.Repos.Payslips.DeleteByRun(ctx, tx, run.TenantID, run.ID); err != nil {
		return err
	}
	if err := s.Repos.Payslips.CreateBatch(ctx, tx, slips); err != nil {
		return err
	}
	s.totals(run, slips, warnings, a.UserID)
	return nil
}

func (s *runService) totals(run *models.Run, slips []models.Payslip, warnings []string, calculator uuid.UUID) {
	run.EmployeeCount, run.GrossTotal, run.DeductionTotal, run.NetTotal = len(slips), 0, 0, 0
	for _, sl := range slips {
		run.GrossTotal += sl.Gross
		run.DeductionTotal += sl.Deductions
		run.NetTotal += sl.Net
	}
	raw, _ := json.Marshal(warnings)
	now := s.Now()
	run.Warnings, run.Status, run.CalculatedByUserID, run.CalculatedAt = string(raw), models.RunCalculated, &calculator, &now
}

// payslipFor computes one employee (PY-001..PY-009); a nil payslip with a warning means "skipped".
func (s *runService) payslipFor(ctx context.Context, tx *gorm.DB, run *models.Run, p monthPeriod, as models.Assignment, specs map[uuid.UUID]calc.StructureSpec) (*models.Payslip, string, error) {
	emp, err := s.employeeByID(ctx, run.TenantID, as.EmployeeProfileID)
	if errors.Is(err, ErrEmployeeNotFound) {
		return nil, "EMPLOYEE_MISSING:" + as.EmployeeProfileID.String(), nil
	}
	if err != nil {
		return nil, "", err
	}
	from, to, ok := window(p, emp)
	if !ok {
		return nil, "", nil
	}
	spec, err := s.specCached(ctx, tx, run.TenantID, as.StructureID, specs)
	if err != nil {
		return nil, "", err
	}
	lop, err := s.Leave.UnpaidDays(ctx, run.TenantID, emp.ID, from, to)
	if err != nil {
		return nil, "", err
	}
	payable := int64(to.Sub(from).Hours()/24+1)*100 - int64(lop)
	if payable < 0 {
		payable = 0
	}
	r, err := calc.Compute(calc.Input{Spec: spec, AnnualCTC: as.AnnualCTC, DaysInMonth: p.days, PayableHundredths: payable})
	if errors.Is(err, calc.ErrOverflow) || errors.Is(err, calc.ErrInvalidStructure) {
		return nil, "STRUCTURE_OVERFLOW:" + emp.EmployeeCode, nil
	}
	if err != nil {
		return nil, "", err
	}
	warn := ""
	if r.Capped {
		warn = "NET_CAPPED:" + emp.EmployeeCode
	}
	return buildPayslip(run, as, emp, slipFigures{p.days, payable, lop, r}), warn, nil
}

func (s *runService) specCached(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID, cache map[uuid.UUID]calc.StructureSpec) (calc.StructureSpec, error) {
	if spec, ok := cache[id]; ok {
		return spec, nil
	}
	_, spec, err := s.loadSpec(ctx, tx, tenantID, id)
	if err == nil {
		cache[id] = spec
	}
	return spec, err
}

// window clips the month to the employment dates (PY-003); ok=false means no payslip.
func window(p monthPeriod, emp *employeeDTO.EmployeeResponse) (time.Time, time.Time, bool) {
	from, to := p.from, p.to
	if e := emp.Employment; e != nil {
		if !e.JoiningDate.IsZero() && dateOnly(e.JoiningDate).After(from) {
			from = dateOnly(e.JoiningDate)
		}
		if e.ExitDate != nil && dateOnly(*e.ExitDate).Before(to) {
			to = dateOnly(*e.ExitDate)
		}
	}
	return from, to, !from.After(to)
}

func dateOnly(t time.Time) time.Time {
	u := t.UTC()
	return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
}

type slipFigures struct {
	days    int
	payable int64
	lop     leavecalc.Days
	result  calc.Result
}

func buildPayslip(run *models.Run, as models.Assignment, emp *employeeDTO.EmployeeResponse, f slipFigures) *models.Payslip {
	slip := &models.Payslip{TenantID: run.TenantID, RunID: run.ID, EmployeeProfileID: emp.ID, EmployeeCode: emp.EmployeeCode,
		EmployeeName: strings.TrimSpace(emp.FirstName + " " + emp.LastName), StructureID: as.StructureID, AnnualCTC: as.AnnualCTC,
		DaysInMonth: f.days, PayableDays: leavecalc.Days(f.payable), LopDays: f.lop, Gross: f.result.Gross,
		Deductions: f.result.Deductions, Net: f.result.Net}
	for i, l := range f.result.Lines {
		slip.Lines = append(slip.Lines, models.PayslipLine{Code: l.Code, Name: l.Name, Kind: l.Kind, Amount: l.Amount, Position: i})
	}
	return slip
}

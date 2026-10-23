package services

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	employeeDTO "github.com/jaas/jaas/internal/hr/employee/dto"
	"github.com/jaas/jaas/internal/hr/kernel"
	"github.com/jaas/jaas/internal/hr/payroll/calc"
	"github.com/jaas/jaas/internal/hr/payroll/models"
	"github.com/jaas/jaas/internal/hr/payroll/repositories"
	"gorm.io/gorm"
)

// arrearsMonths is how far back a run looks for payslips to settle (PP-04).
const arrearsMonths = 12

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

// runCache holds per-run lookups so a large tenant does not repeat them per employee.
type runCache struct {
	specs map[uuid.UUID]calc.StructureSpec
	slabs map[string][]calc.PTSlab
}

// calculate replaces the run's payslips (PY-011) and stores totals and warnings.
func (s *runService) calculate(ctx context.Context, tx *gorm.DB, a Actor, run *models.Run) error {
	p := periodOf(run)
	assigns, err := s.Repos.Assignments.PeriodAssignments(ctx, tx, run.TenantID, repositories.Period{From: p.from, To: p.to})
	if err != nil {
		return err
	}
	cache := &runCache{specs: map[uuid.UUID]calc.StructureSpec{}, slabs: map[string][]calc.PTSlab{}}
	var slips []models.Payslip
	var arrears []models.Arrear
	warnings := []string{}
	for _, group := range groupByEmployee(assigns) {
		slip, rows, warn, err := s.payslipFor(ctx, tx, run, p, group, cache)
		if err != nil {
			return err
		}
		warnings = append(warnings, warn...)
		arrears = append(arrears, rows...)
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
	if err := s.Repos.Arrears.ReplaceForRun(ctx, tx, run.TenantID, run.ID, arrears); err != nil {
		return err
	}
	s.totals(run, slips, warnings, a.UserID)
	return nil
}

// groupByEmployee splits assignments (ordered by employee, then start) into one slice per employee.
func groupByEmployee(all []models.Assignment) [][]models.Assignment {
	var out [][]models.Assignment
	for _, a := range all {
		if n := len(out); n > 0 && out[n-1][0].EmployeeProfileID == a.EmployeeProfileID {
			out[n-1] = append(out[n-1], a)
			continue
		}
		out = append(out, []models.Assignment{a})
	}
	return out
}

func (s *runService) totals(run *models.Run, slips []models.Payslip, warnings []string, calculator uuid.UUID) {
	run.EmployeeCount, run.GrossTotal, run.DeductionTotal, run.NetTotal, run.EmployerTotal = len(slips), 0, 0, 0, 0
	for _, sl := range slips {
		run.GrossTotal += sl.Gross
		run.DeductionTotal += sl.Deductions
		run.NetTotal += sl.Net
		run.EmployerTotal += sl.EmployerCost
	}
	raw, _ := json.Marshal(warnings)
	now := s.Now()
	run.Warnings, run.Status, run.CalculatedByUserID, run.CalculatedAt = string(raw), models.RunCalculated, &calculator, &now
}

// segment is one slice of the pay window under one assignment (PP-03).
type segment struct {
	as       models.Assignment
	from, to time.Time
}

func (g segment) days() int { return int(g.to.Sub(g.from).Hours()/24) + 1 }

// segmentsOf clips the employee assignments (ordered by start) to the window.
func segmentsOf(assigns []models.Assignment, from, to time.Time) []segment {
	var out []segment
	for _, as := range assigns {
		f, t := from, to
		if d := dateOnly(as.EffectiveFrom); d.After(f) {
			f = d
		}
		if as.EffectiveTo != nil {
			if d := dateOnly(*as.EffectiveTo); d.Before(t) {
				t = d
			}
		}
		if !f.After(t) {
			out = append(out, segment{as: as, from: f, to: t})
		}
	}
	return out
}

// payslipFor computes one employee (PY-001..PY-009, PP-01..PP-05); a nil payslip with warnings means "skipped".
func (s *runService) payslipFor(ctx context.Context, tx *gorm.DB, run *models.Run, p monthPeriod, assigns []models.Assignment, cache *runCache) (*models.Payslip, []models.Arrear, []string, error) {
	emp, err := s.employeeByID(ctx, run.TenantID, assigns[0].EmployeeProfileID)
	if errors.Is(err, ErrEmployeeNotFound) {
		return nil, nil, []string{"EMPLOYEE_MISSING:" + assigns[0].EmployeeProfileID.String()}, nil
	}
	if err != nil {
		return nil, nil, nil, err
	}
	from, to, ok := window(p, emp)
	if !ok {
		return nil, nil, nil, nil
	}
	segs := segmentsOf(assigns, from, to)
	if len(segs) == 0 {
		return nil, nil, nil, nil
	}
	in := calc.SegmentsInput{DaysInMonth: p.days, Month: run.Month}
	var records models.SegmentRecords
	var payableTotal int64
	var lopTotal kernel.Days
	for _, g := range segs {
		spec, err := s.specCached(ctx, tx, run.TenantID, g.as.StructureID, cache.specs)
		if err != nil {
			return nil, nil, nil, err
		}
		lop, err := s.Leave.UnpaidDays(ctx, run.TenantID, emp.ID, g.from, g.to)
		if err != nil {
			return nil, nil, nil, err
		}
		payable := int64(g.days())*100 - int64(lop)
		if payable < 0 {
			payable = 0
		}
		in.Segments = append(in.Segments, calc.Segment{Spec: spec, AnnualCTC: g.as.AnnualCTC, PayableHundredths: payable})
		records = append(records, models.SegmentRecord{From: g.from.Format("2006-01-02"), To: g.to.Format("2006-01-02"), StructureID: g.as.StructureID,
			AnnualCTC: g.as.AnnualCTC, Payable: payable})
		payableTotal += payable
		lopTotal += lop
	}
	if in.PT, err = s.slabsFor(ctx, tx, run.TenantID, emp.ID, p.to, cache); err != nil {
		return nil, nil, nil, err
	}
	if in.Benefits, err = s.benefitLines(ctx, run.TenantID, emp.ID, from, to); err != nil {
		return nil, nil, nil, err
	}
	var warnings []string
	var rows []models.Arrear
	if in.Arrears, rows, warnings, err = s.arrearsFor(ctx, tx, run, emp, cache); err != nil {
		return nil, nil, nil, err
	}
	r, err := calc.ComputeSegments(in)
	if errors.Is(err, calc.ErrOverflow) || errors.Is(err, calc.ErrInvalidStructure) {
		return nil, nil, []string{"STRUCTURE_OVERFLOW:" + emp.EmployeeCode}, nil
	}
	if err != nil {
		return nil, nil, nil, err
	}
	if r.Capped {
		warnings = append(warnings, "NET_CAPPED:"+emp.EmployeeCode)
	}
	last := segs[len(segs)-1].as
	slip := buildPayslip(run, last, emp, slipFigures{p.days, payableTotal, lopTotal, r})
	slip.Segments = records
	for i := range rows {
		rows[i].EmployeeProfileID = emp.ID
	}
	return slip, rows, warnings, nil
}

// slabsFor resolves the professional-tax slabs of the worker state (PP-02); nil keeps the default slab.
func (s *runService) slabsFor(ctx context.Context, tx *gorm.DB, tenantID, employeeID uuid.UUID, on time.Time, cache *runCache) ([]calc.PTSlab, error) {
	if s.PT == nil {
		return nil, nil
	}
	state, err := s.PT.StateOf(ctx, tenantID, employeeID, on)
	if err != nil || state == "" {
		return nil, err
	}
	state = strings.ToUpper(state)
	if slabs, ok := cache.slabs[state]; ok {
		return slabs, nil
	}
	slabs, err := s.PT.Slabs(ctx, tx, tenantID, state)
	if err == nil {
		cache.slabs[state] = slabs
	}
	return slabs, err
}

// benefitLines turns the benefits port output into deduction lines BEN_<code> (PP-05).
func (s *runService) benefitLines(ctx context.Context, tenantID, employeeID uuid.UUID, from, to time.Time) ([]calc.Line, error) {
	if s.Benefits == nil {
		return nil, nil
	}
	rows, err := s.Benefits.Deductions(ctx, tenantID, employeeID, from, to)
	if err != nil {
		return nil, err
	}
	var out []calc.Line
	for _, b := range rows {
		if b.Amount > 0 {
			out = append(out, calc.Line{Code: "BEN_" + strings.ToUpper(b.Code), Name: b.Name, Kind: calc.Deduction, Amount: b.Amount})
		}
	}
	return out, nil
}

// arrearsFor settles earlier finalized payslips whose month would price differently under the current assignment
// history (PP-04). It returns the signed total and one row per source payslip with a non-zero difference.
func (s *runService) arrearsFor(ctx context.Context, tx *gorm.DB, run *models.Run, emp *employeeDTO.EmployeeResponse, cache *runCache) (calc.Money, []models.Arrear, []string, error) {
	past, err := s.Repos.Payslips.FinalizedBefore(ctx, tx, run.TenantID, emp.ID, run.Year, run.Month, arrearsMonths)
	if err != nil || len(past) == 0 {
		return 0, nil, nil, err
	}
	ids := make([]uuid.UUID, len(past))
	for i, ps := range past {
		ids[i] = ps.ID
	}
	paid, err := s.Repos.Arrears.PaidFor(ctx, tx, run.TenantID, ids, run.ID)
	if err != nil {
		return 0, nil, nil, err
	}
	var total calc.Money
	var rows []models.Arrear
	var warnings []string
	for _, ps := range past {
		due, ok, err := s.arrearsDue(ctx, tx, run, emp, ps, paid[ps.ID], cache)
		if err != nil {
			return 0, nil, nil, err
		}
		if !ok {
			warnings = append(warnings, "ARREARS_SKIPPED:"+emp.EmployeeCode)
			continue
		}
		if due != 0 {
			total += due
			rows = append(rows, models.Arrear{TenantID: run.TenantID, RunID: run.ID, SourcePayslipID: ps.ID, Amount: due})
		}
	}
	return total, rows, warnings, nil
}

// arrearsDue is one source payslip: unchanged history owes nothing; otherwise the recomputed earned gross minus what was
// paid for the month (ARREARS lines excluded) and what later runs already settled.
func (s *runService) arrearsDue(ctx context.Context, tx *gorm.DB, run *models.Run, emp *employeeDTO.EmployeeResponse, ps repositories.PastPayslip, settled calc.Money, cache *runCache) (calc.Money, bool, error) {
	pp := periodOf(&models.Run{Year: ps.RunYear, Month: ps.RunMonth})
	from, to, ok := window(pp, emp)
	if !ok {
		return 0, true, nil
	}
	assigns, err := s.Repos.Assignments.ForEmployee(ctx, tx, run.TenantID, emp.ID, repositories.Period{From: from, To: to})
	if err != nil {
		return 0, false, err
	}
	segs := segmentsOf(assigns, from, to)
	if len(segs) == 0 || sameHistory(ps.Segments, segs) {
		return 0, true, nil
	}
	windowDays := int(to.Sub(from).Hours()/24) + 1
	lop, remaining := int64(ps.LopDays), int64(ps.LopDays)
	in := calc.SegmentsInput{DaysInMonth: pp.days, Month: ps.RunMonth}
	for i, g := range segs {
		share := lop * int64(g.days()) / int64(windowDays)
		if i == len(segs)-1 {
			share = remaining
		}
		remaining -= share
		spec, err := s.specCached(ctx, tx, run.TenantID, g.as.StructureID, cache.specs)
		if err != nil {
			return 0, false, err
		}
		payable := int64(g.days())*100 - share
		if payable < 0 {
			payable = 0
		}
		in.Segments = append(in.Segments, calc.Segment{Spec: spec, AnnualCTC: g.as.AnnualCTC, PayableHundredths: payable})
	}
	r, err := calc.ComputeSegments(in)
	if errors.Is(err, calc.ErrOverflow) || errors.Is(err, calc.ErrInvalidStructure) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return r.Gross - (ps.Gross - ps.ArrearsLine) - settled, true, nil
}

// sameHistory reports whether the stored segments equal the current ones (dates, structure and CTC).
func sameHistory(stored models.SegmentRecords, segs []segment) bool {
	if len(stored) != len(segs) {
		return false
	}
	for i, g := range segs {
		rec := stored[i]
		if rec.From != g.from.Format("2006-01-02") || rec.To != g.to.Format("2006-01-02") || rec.StructureID != g.as.StructureID || rec.AnnualCTC != g.as.AnnualCTC {
			return false
		}
	}
	return true
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
	lop     kernel.Days
	result  calc.Result
}

func buildPayslip(run *models.Run, as models.Assignment, emp *employeeDTO.EmployeeResponse, f slipFigures) *models.Payslip {
	slip := &models.Payslip{TenantID: run.TenantID, RunID: run.ID, EmployeeProfileID: emp.ID, EmployeeCode: emp.EmployeeCode,
		EmployeeName: strings.TrimSpace(emp.FirstName + " " + emp.LastName), StructureID: as.StructureID, AnnualCTC: as.AnnualCTC,
		DaysInMonth: f.days, PayableDays: kernel.Days(f.payable), LopDays: f.lop, Gross: f.result.Gross,
		Deductions: f.result.Deductions, Net: f.result.Net, EmployerCost: f.result.EmployerCost}
	for i, l := range f.result.Lines {
		slip.Lines = append(slip.Lines, models.PayslipLine{Code: l.Code, Name: l.Name, Kind: l.Kind, Amount: l.Amount, Position: i})
	}
	return slip
}

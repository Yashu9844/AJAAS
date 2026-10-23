package services

import (
	"context"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/hr/payroll/calc"
	"github.com/jaas/jaas/internal/hr/payroll/models"
	"github.com/jaas/jaas/internal/hr/payroll/repositories"
	"gorm.io/gorm"
)

// PeriodAssignments and ForEmployee extend fakeAssignments for PP-03 and PP-04.
func (f fakeAssignments) PeriodAssignments(_ context.Context, _ *gorm.DB, t uuid.UUID, p repositories.Period) ([]models.Assignment, error) {
	var out []models.Assignment
	for _, a := range f.st.assignments {
		if a.TenantID == t && !a.EffectiveFrom.After(p.To) && (a.EffectiveTo == nil || !a.EffectiveTo.Before(p.From)) {
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].EmployeeProfileID != out[j].EmployeeProfileID {
			return out[i].EmployeeProfileID.String() < out[j].EmployeeProfileID.String()
		}
		return out[i].EffectiveFrom.Before(out[j].EffectiveFrom)
	})
	return out, f.st.err("assignment.period")
}

func (f fakeAssignments) ForEmployee(ctx context.Context, db *gorm.DB, t, e uuid.UUID, p repositories.Period) ([]models.Assignment, error) {
	all, err := f.PeriodAssignments(ctx, db, t, p)
	var out []models.Assignment
	for _, a := range all {
		if a.EmployeeProfileID == e {
			out = append(out, a)
		}
	}
	if err == nil {
		err = f.st.err("assignment.foremployee")
	}
	return out, err
}

// FinalizedBefore extends fakePayslips for PP-04.
func (f fakePayslips) FinalizedBefore(_ context.Context, _ *gorm.DB, t, emp uuid.UUID, year, month, months int) ([]repositories.PastPayslip, error) {
	var out []repositories.PastPayslip
	current := year*12 + month
	for _, p := range f.st.payslips {
		run := f.st.runs[p.RunID]
		at := run.Year*12 + run.Month
		if p.TenantID != t || p.EmployeeProfileID != emp || run.Status != models.RunFinalized || at >= current || at < current-months {
			continue
		}
		pp := repositories.PastPayslip{Payslip: p, RunYear: run.Year, RunMonth: run.Month}
		for _, l := range p.Lines {
			if l.Code == calc.ArrearsCode {
				pp.ArrearsLine += l.Amount
			}
		}
		out = append(out, pp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RunYear*12+out[i].RunMonth < out[j].RunYear*12+out[j].RunMonth })
	return out, f.st.err("payslip.before")
}

// fakeArrears implements repositories.ArrearsRepository over the shared store.
type fakeArrears struct{ st *store }

func (f fakeArrears) PaidFor(_ context.Context, _ *gorm.DB, t uuid.UUID, ids []uuid.UUID, except uuid.UUID) (map[uuid.UUID]calc.Money, error) {
	out := map[uuid.UUID]calc.Money{}
	for _, a := range f.st.arrears {
		if a.TenantID != t || a.RunID == except {
			continue
		}
		for _, id := range ids {
			if a.SourcePayslipID == id {
				out[id] += a.Amount
			}
		}
	}
	return out, f.st.err("arrears.paid")
}

func (f fakeArrears) ReplaceForRun(_ context.Context, _ *gorm.DB, t, run uuid.UUID, rows []models.Arrear) error {
	if err := f.st.err("arrears.replace"); err != nil {
		return err
	}
	kept := f.st.arrears[:0:0]
	for _, a := range f.st.arrears {
		if a.TenantID != t || a.RunID != run {
			kept = append(kept, a)
		}
	}
	for i := range rows {
		ensureID(&rows[i].ID)
		rows[i].CreatedAt = time.Now()
		kept = append(kept, rows[i])
	}
	f.st.arrears = kept
	return nil
}

// fakePT is the professional-tax source: a state per employee and optional tenant slabs.
type fakePT struct {
	state map[uuid.UUID]string
	slabs map[string][]calc.PTSlab
	err   error
}

func (f *fakePT) StateOf(_ context.Context, _, e uuid.UUID, _ time.Time) (string, error) {
	return f.state[e], f.err
}

func (f *fakePT) Slabs(_ context.Context, _ *gorm.DB, _ uuid.UUID, state string) ([]calc.PTSlab, error) {
	if s, ok := f.slabs[state]; ok {
		return s, f.err
	}
	return calc.DefaultPTSlabs(state), f.err
}

// fakeBenefits returns fixed deductions per employee.
type fakeBenefits struct {
	lines map[uuid.UUID][]BenefitLine
	err   error
}

func (f *fakeBenefits) Deductions(_ context.Context, _, e uuid.UUID, _, _ time.Time) ([]BenefitLine, error) {
	return f.lines[e], f.err
}

package services

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/payroll/dto"
	"github.com/jaas/jaas/internal/payroll/models"
	"gorm.io/gorm"
)

type fakeRuns struct{ st *store }

func (f fakeRuns) Create(_ context.Context, _ *gorm.DB, r *models.Run) error {
	if err := f.st.err("run.create"); err != nil {
		return err
	}
	ensureID(&r.ID)
	f.st.runs[r.ID] = *r
	return nil
}
func (f fakeRuns) Update(_ context.Context, _ *gorm.DB, r *models.Run) error {
	if err := f.st.err("run.update"); err != nil {
		return err
	}
	f.st.runs[r.ID] = *r
	return nil
}
func (f fakeRuns) FindByID(_ context.Context, _ *gorm.DB, t, id uuid.UUID) (*models.Run, error) {
	if err := f.st.err("run.find"); err != nil {
		return nil, err
	}
	if r, ok := f.st.runs[id]; ok && r.TenantID == t {
		return &r, nil
	}
	return nil, nil
}
func (f fakeRuns) LockByID(ctx context.Context, db *gorm.DB, t, id uuid.UUID) (*models.Run, error) {
	f.st.locks++
	if err := f.st.err("run.lock"); err != nil {
		return nil, err
	}
	return f.FindByID(ctx, db, t, id)
}
func (f fakeRuns) FindByPeriod(_ context.Context, _ *gorm.DB, t uuid.UUID, y, m int) (*models.Run, error) {
	for _, r := range f.st.runs {
		if r.TenantID == t && r.Year == y && r.Month == m {
			c := r
			return &c, nil
		}
	}
	return nil, f.st.err("run.period")
}
func (f fakeRuns) List(_ context.Context, _ *gorm.DB, t uuid.UUID, status string, p dto.Page) ([]models.Run, int64, error) {
	var rows []models.Run
	for _, r := range f.st.runs {
		if r.TenantID == t && (status == "" || r.Status == status) {
			rows = append(rows, r)
		}
	}
	out, n := pageOf(rows, p)
	return out, n, f.st.err("run.list")
}

type fakePayslips struct{ st *store }

func (f fakePayslips) DeleteByRun(_ context.Context, _ *gorm.DB, t, run uuid.UUID) error {
	for id, p := range f.st.payslips {
		if p.TenantID == t && p.RunID == run {
			delete(f.st.payslips, id)
		}
	}
	return f.st.err("payslip.delete")
}
func (f fakePayslips) CreateBatch(_ context.Context, _ *gorm.DB, slips []models.Payslip) error {
	if err := f.st.err("payslip.create"); err != nil {
		return err
	}
	for i := range slips {
		ensureID(&slips[i].ID)
		slips[i].CreatedAt = time.Now()
		f.st.payslips[slips[i].ID] = slips[i]
	}
	return nil
}
func (f fakePayslips) FindByID(_ context.Context, _ *gorm.DB, t, id uuid.UUID) (*models.Payslip, error) {
	if p, ok := f.st.payslips[id]; ok && p.TenantID == t {
		return &p, nil
	}
	return nil, f.st.err("payslip.find")
}
func (f fakePayslips) filter(t uuid.UUID, keep func(models.Payslip) bool) []models.Payslip {
	var rows []models.Payslip
	for _, p := range f.st.payslips {
		if p.TenantID == t && keep(p) {
			rows = append(rows, p)
		}
	}
	return rows
}
func (f fakePayslips) ListByRun(_ context.Context, _ *gorm.DB, t, run uuid.UUID, p dto.Page) ([]models.Payslip, int64, error) {
	out, n := pageOf(f.filter(t, func(s models.Payslip) bool { return s.RunID == run }), p)
	return out, n, f.st.err("payslip.list")
}
func (f fakePayslips) AllByRun(_ context.Context, _ *gorm.DB, t, run uuid.UUID) ([]models.Payslip, error) {
	return f.filter(t, func(s models.Payslip) bool { return s.RunID == run }), f.st.err("payslip.all")
}
func (f fakePayslips) ListFinalizedForEmployee(_ context.Context, _ *gorm.DB, t, emp uuid.UUID, p dto.Page) ([]models.Payslip, int64, error) {
	out, n := pageOf(f.filter(t, func(s models.Payslip) bool {
		return s.EmployeeProfileID == emp && f.st.runs[s.RunID].Status == models.RunFinalized
	}), p)
	return out, n, f.st.err("payslip.mine")
}

type fakeOutbox struct{ st *store }

func (f fakeOutbox) Create(_ context.Context, _ *gorm.DB, e *models.OutboxEvent) error {
	if err := f.st.err("outbox.create"); err != nil {
		return err
	}
	ensureID(&e.ID)
	f.st.outbox[e.ID] = *e
	return nil
}
func (f fakeOutbox) MarkPublished(_ context.Context, _ *gorm.DB, id uuid.UUID, at time.Time) error {
	if e, ok := f.st.outbox[id]; ok {
		e.Published, e.PublishedAt = true, &at
		f.st.outbox[id] = e
	}
	return nil
}
func (f fakeOutbox) RecordFailure(_ context.Context, _ *gorm.DB, id uuid.UUID, reason string) error {
	if e, ok := f.st.outbox[id]; ok {
		e.Attempts++
		e.LastError = &reason
		f.st.outbox[id] = e
	}
	return nil
}
func (f fakeOutbox) FetchUnpublished(_ context.Context, _ *gorm.DB, limit, max int) ([]models.OutboxEvent, error) {
	var out []models.OutboxEvent
	for _, e := range f.st.outbox {
		if !e.Published && e.Attempts < max && len(out) < limit {
			out = append(out, e)
		}
	}
	return out, f.st.err("outbox.fetch")
}

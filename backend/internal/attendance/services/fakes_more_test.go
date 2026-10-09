package services

import (
	"context"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/attendance/dto"
	"github.com/jaas/jaas/internal/attendance/models"
	"github.com/jaas/jaas/internal/attendance/repositories"
	"gorm.io/gorm"
)

// --- punches ---
type fakePunches struct{ st *store }

func (f fakePunches) Create(_ context.Context, _ *gorm.DB, p *models.AttendancePunch) error {
	if err := f.st.err("punch.create"); err != nil {
		return err
	}
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	f.st.punches[p.ID] = *p
	return nil
}
func (f fakePunches) filter(pred func(models.AttendancePunch) bool) []models.AttendancePunch {
	var out []models.AttendancePunch
	for _, p := range f.st.punches {
		if pred(p) {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PunchTime.Before(out[j].PunchTime) })
	return out
}
func (f fakePunches) LastForEmployee(_ context.Context, _ *gorm.DB, t, e uuid.UUID) (*models.AttendancePunch, error) {
	ps := f.filter(func(p models.AttendancePunch) bool {
		return p.TenantID == t && p.EmployeeProfileID == e && !p.IsSuperseded
	})
	if len(ps) == 0 {
		return nil, f.st.err("punch.last")
	}
	return &ps[len(ps)-1], f.st.err("punch.last")
}
func (f fakePunches) ActiveForRecord(_ context.Context, _ *gorm.DB, t, r uuid.UUID) ([]models.AttendancePunch, error) {
	return f.filter(func(p models.AttendancePunch) bool {
		return p.TenantID == t && p.AttendanceRecordID == r && !p.IsSuperseded
	}), nil
}
func (f fakePunches) AllForRecord(_ context.Context, _ *gorm.DB, t, r uuid.UUID) ([]models.AttendancePunch, error) {
	return f.filter(func(p models.AttendancePunch) bool { return p.TenantID == t && p.AttendanceRecordID == r }), f.st.err("punch.all")
}
func (f fakePunches) SupersedeForRecord(_ context.Context, _ *gorm.DB, t, r uuid.UUID) error {
	for id, p := range f.st.punches {
		if p.TenantID == t && p.AttendanceRecordID == r {
			p.IsSuperseded = true
			f.st.punches[id] = p
		}
	}
	return f.st.err("punch.supersede")
}

// --- regularizations ---
type fakeRegs struct{ st *store }

func (f fakeRegs) Create(_ context.Context, _ *gorm.DB, g *models.Regularization) error {
	if g.ID == uuid.Nil {
		g.ID = uuid.New()
	}
	f.st.regs[g.ID] = *g
	return f.st.err("reg.create")
}
func (f fakeRegs) Update(_ context.Context, _ *gorm.DB, g *models.Regularization) error {
	f.st.regs[g.ID] = *g
	return nil
}
func (f fakeRegs) FindByID(_ context.Context, _ *gorm.DB, t, id uuid.UUID) (*models.Regularization, error) {
	if g, ok := f.st.regs[id]; ok && g.TenantID == t {
		return &g, nil
	}
	return nil, f.st.err("reg.find")
}
func (f fakeRegs) FindPending(_ context.Context, _ *gorm.DB, t, e uuid.UUID, d time.Time) (*models.Regularization, error) {
	for _, g := range f.st.regs {
		if g.TenantID == t && g.EmployeeProfileID == e && g.AttendanceDate.Equal(d) && g.Status == models.RegPending {
			c := g
			return &c, nil
		}
	}
	return nil, nil
}
func (f fakeRegs) List(_ context.Context, _ *gorm.DB, t uuid.UUID, fl repositories.RegularizationFilter, p dto.Page) ([]models.Regularization, int64, error) {
	var out []models.Regularization
	for _, g := range f.st.regs {
		if g.TenantID == t && (fl.EmployeeID == nil || *fl.EmployeeID == g.EmployeeProfileID) && (fl.Status == "" || fl.Status == g.Status) {
			out = append(out, g)
		}
	}
	items, total := page(out, p)
	return items, total, f.st.err("reg.list")
}

// --- outbox ---
type fakeOutbox struct{ st *store }

func (f fakeOutbox) Create(_ context.Context, _ *gorm.DB, e *models.OutboxEvent) error {
	if e.ID == uuid.Nil {
		e.ID = uuid.New()
	}
	f.st.outbox[e.ID] = *e
	return f.st.err("outbox.create")
}
func (f fakeOutbox) MarkPublished(_ context.Context, _ *gorm.DB, id uuid.UUID, at time.Time) error {
	e := f.st.outbox[id]
	e.Published, e.PublishedAt = true, &at
	f.st.outbox[id] = e
	return nil
}
func (f fakeOutbox) RecordFailure(_ context.Context, _ *gorm.DB, id uuid.UUID, reason string) error {
	e := f.st.outbox[id]
	e.Attempts++
	e.LastError = &reason
	f.st.outbox[id] = e
	return nil
}
func (f fakeOutbox) FetchUnpublished(_ context.Context, _ *gorm.DB, limit, maxAttempts int) ([]models.OutboxEvent, error) {
	var out []models.OutboxEvent
	for _, e := range f.st.outbox {
		if !e.Published && e.Attempts < maxAttempts && len(out) < limit {
			out = append(out, e)
		}
	}
	return out, f.st.err("outbox.fetch")
}

func (f fakeRecords) Delete(_ context.Context, _ *gorm.DB, t, id uuid.UUID) error {
	if err := f.st.err("record.delete"); err != nil {
		return err
	}
	if r, ok := f.st.records[id]; ok && r.TenantID == t {
		delete(f.st.records, id)
	}
	return nil
}

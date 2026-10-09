package services

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/attendance/dto"
	"github.com/jaas/jaas/internal/attendance/models"
	"github.com/jaas/jaas/internal/attendance/repositories"
	"gorm.io/gorm"
)

// store is an in-memory database honoring repository contract semantics, with tx rollback.
type store struct {
	shifts  map[uuid.UUID]models.Shift
	assigns map[uuid.UUID]models.ShiftAssignment
	records map[uuid.UUID]models.AttendanceRecord
	punches map[uuid.UUID]models.AttendancePunch
	regs    map[uuid.UUID]models.Regularization
	outbox  map[uuid.UUID]models.OutboxEvent
	locks   int
	fail    map[string]error // op name → injected error
}

func newStore() *store {
	return &store{
		shifts: map[uuid.UUID]models.Shift{}, assigns: map[uuid.UUID]models.ShiftAssignment{},
		records: map[uuid.UUID]models.AttendanceRecord{}, punches: map[uuid.UUID]models.AttendancePunch{},
		regs: map[uuid.UUID]models.Regularization{}, outbox: map[uuid.UUID]models.OutboxEvent{},
		fail: map[string]error{},
	}
}

func cloneMap[K comparable, V any](m map[K]V) map[K]V {
	out := make(map[K]V, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func (s *store) snapshot() store {
	return store{shifts: cloneMap(s.shifts), assigns: cloneMap(s.assigns), records: cloneMap(s.records),
		punches: cloneMap(s.punches), regs: cloneMap(s.regs), outbox: cloneMap(s.outbox), locks: s.locks, fail: s.fail}
}

func (s *store) err(op string) error { return s.fail[op] }

type fakeRunner struct{ st *store }

func (f fakeRunner) InTx(ctx context.Context, fn func(tx *gorm.DB) error) error {
	snap := f.st.snapshot()
	if err := fn(nil); err != nil {
		*f.st = snap
		return err
	}
	return nil
}

func (f fakeRunner) DB() *gorm.DB { return nil }

func page[T any](items []T, p dto.Page) ([]T, int64) {
	total := int64(len(items))
	start := p.Offset()
	if start > len(items) {
		start = len(items)
	}
	end := start + p.PerPage
	if end > len(items) {
		end = len(items)
	}
	return items[start:end], total
}

// --- shifts ---
type fakeShifts struct{ st *store }

func (f fakeShifts) Create(_ context.Context, _ *gorm.DB, s *models.Shift) error {
	if err := f.st.err("shift.create"); err != nil {
		return err
	}
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	f.st.shifts[s.ID] = *s
	return nil
}
func (f fakeShifts) Update(_ context.Context, _ *gorm.DB, s *models.Shift) error {
	if err := f.st.err("shift.update"); err != nil {
		return err
	}
	f.st.shifts[s.ID] = *s
	return nil
}
func (f fakeShifts) find(pred func(models.Shift) bool) (*models.Shift, error) {
	for _, s := range f.st.shifts {
		if pred(s) {
			c := s
			return &c, nil
		}
	}
	return nil, nil
}
func (f fakeShifts) FindByID(_ context.Context, _ *gorm.DB, t, id uuid.UUID) (*models.Shift, error) {
	if err := f.st.err("shift.find"); err != nil {
		return nil, err
	}
	return f.find(func(s models.Shift) bool { return s.TenantID == t && s.ID == id })
}
func (f fakeShifts) FindByName(_ context.Context, _ *gorm.DB, t uuid.UUID, n string) (*models.Shift, error) {
	return f.find(func(s models.Shift) bool { return s.TenantID == t && strings.EqualFold(s.Name, strings.TrimSpace(n)) })
}
func (f fakeShifts) FindByCode(_ context.Context, _ *gorm.DB, t uuid.UUID, c string) (*models.Shift, error) {
	return f.find(func(s models.Shift) bool { return s.TenantID == t && s.Code != nil && *s.Code == c })
}
func (f fakeShifts) List(_ context.Context, _ *gorm.DB, t uuid.UUID, status string, p dto.Page) ([]models.Shift, int64, error) {
	if err := f.st.err("shift.list"); err != nil {
		return nil, 0, err
	}
	var out []models.Shift
	for _, s := range f.st.shifts {
		if s.TenantID == t && (status == "" || s.Status == status) {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	items, total := page(out, p)
	return items, total, nil
}

// --- assignments ---
type fakeAssigns struct{ st *store }

func (f fakeAssigns) Create(_ context.Context, _ *gorm.DB, a *models.ShiftAssignment) error {
	if a.ID == uuid.Nil {
		a.ID = uuid.New()
	}
	f.st.assigns[a.ID] = *a
	return nil
}
func (f fakeAssigns) Update(_ context.Context, _ *gorm.DB, a *models.ShiftAssignment) error {
	f.st.assigns[a.ID] = *a
	return nil
}
func covers(a models.ShiftAssignment, d time.Time) bool {
	return !a.EffectiveFrom.After(d) && (a.EffectiveTo == nil || !a.EffectiveTo.Before(d))
}
func (f fakeAssigns) Overlapping(_ context.Context, _ *gorm.DB, t, e uuid.UUID, from time.Time, to *time.Time) ([]models.ShiftAssignment, error) {
	var out []models.ShiftAssignment
	for _, a := range f.st.assigns {
		if a.TenantID != t || a.EmployeeProfileID != e {
			continue
		}
		endsAfter := a.EffectiveTo == nil || !a.EffectiveTo.Before(from)
		startsBefore := to == nil || !a.EffectiveFrom.After(*to)
		if endsAfter && startsBefore {
			out = append(out, a)
		}
	}
	return out, nil
}
func (f fakeAssigns) ActiveFor(_ context.Context, _ *gorm.DB, t, e uuid.UUID, d time.Time) (*models.ShiftAssignment, error) {
	if err := f.st.err("assign.active"); err != nil {
		return nil, err
	}
	for _, a := range f.st.assigns {
		if a.TenantID == t && a.EmployeeProfileID == e && covers(a, d) {
			c := a
			return &c, nil
		}
	}
	return nil, nil
}
func (f fakeAssigns) CountCoveringFrom(_ context.Context, _ *gorm.DB, t, shiftID uuid.UUID, d time.Time) (int64, error) {
	var n int64
	for _, a := range f.st.assigns {
		if a.TenantID == t && a.ShiftID == shiftID && (a.EffectiveTo == nil || !a.EffectiveTo.Before(d)) {
			n++
		}
	}
	return n, nil
}
func (f fakeAssigns) ListByEmployee(_ context.Context, _ *gorm.DB, t, e uuid.UUID, p dto.Page) ([]models.ShiftAssignment, int64, error) {
	var out []models.ShiftAssignment
	for _, a := range f.st.assigns {
		if a.TenantID == t && a.EmployeeProfileID == e {
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].EffectiveFrom.After(out[j].EffectiveFrom) })
	items, total := page(out, p)
	return items, total, nil
}

// --- records ---
type fakeRecords struct{ st *store }

func (f fakeRecords) LockEmployee(_ context.Context, _ *gorm.DB, _, _ uuid.UUID) error {
	f.st.locks++
	return f.st.err("record.lock")
}
func (f fakeRecords) FindOrCreate(ctx context.Context, db *gorm.DB, seed *models.AttendanceRecord) (*models.AttendanceRecord, error) {
	if err := f.st.err("record.create"); err != nil {
		return nil, err
	}
	if r, _ := f.FindByDate(ctx, db, seed.TenantID, seed.EmployeeProfileID, seed.AttendanceDate); r != nil {
		return r, nil
	}
	if seed.ID == uuid.Nil {
		seed.ID = uuid.New()
	}
	f.st.records[seed.ID] = *seed
	c := *seed
	return &c, nil
}
func (f fakeRecords) Update(_ context.Context, _ *gorm.DB, r *models.AttendanceRecord) error {
	if err := f.st.err("record.update"); err != nil {
		return err
	}
	f.st.records[r.ID] = *r
	return nil
}
func (f fakeRecords) FindByID(_ context.Context, _ *gorm.DB, t, id uuid.UUID) (*models.AttendanceRecord, error) {
	if r, ok := f.st.records[id]; ok && r.TenantID == t {
		return &r, nil
	}
	return nil, nil
}
func (f fakeRecords) FindByDate(_ context.Context, _ *gorm.DB, t, e uuid.UUID, d time.Time) (*models.AttendanceRecord, error) {
	for _, r := range f.st.records {
		if r.TenantID == t && r.EmployeeProfileID == e && r.AttendanceDate.Equal(d) {
			c := r
			return &c, nil
		}
	}
	return nil, nil
}
func (f fakeRecords) ListForEmployee(_ context.Context, _ *gorm.DB, t, e uuid.UUID, from, to time.Time) ([]models.AttendanceRecord, error) {
	var out []models.AttendanceRecord
	for _, r := range f.st.records {
		if r.TenantID == t && r.EmployeeProfileID == e && !r.AttendanceDate.Before(from) && !r.AttendanceDate.After(to) {
			out = append(out, r)
		}
	}
	return out, f.st.err("record.list")
}
func (f fakeRecords) List(_ context.Context, _ *gorm.DB, t uuid.UUID, fl repositories.RecordFilter, p dto.Page) ([]models.AttendanceRecord, int64, error) {
	var out []models.AttendanceRecord
	for _, r := range f.st.records {
		if r.TenantID != t || r.AttendanceDate.Before(fl.From) || r.AttendanceDate.After(fl.To) {
			continue
		}
		if (fl.EmployeeID == nil || *fl.EmployeeID == r.EmployeeProfileID) && (fl.Status == "" || fl.Status == r.Status) {
			out = append(out, r)
		}
	}
	items, total := page(out, p)
	return items, total, f.st.err("record.list")
}
func (f fakeRecords) CountByStatus(_ context.Context, _ *gorm.DB, t uuid.UUID, d time.Time) (repositories.StatusCounts, error) {
	c := repositories.StatusCounts{ByStatus: map[string]int64{}}
	for _, r := range f.st.records {
		if r.TenantID == t && r.AttendanceDate.Equal(d) {
			c.ByStatus[r.Status]++
			if r.LateMinutes > 0 {
				c.Late++
			}
		}
	}
	return c, f.st.err("record.count")
}

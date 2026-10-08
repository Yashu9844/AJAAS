package services

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/leave/dto"
	"github.com/jaas/jaas/internal/leave/models"
	"gorm.io/gorm"
)

// store is an in-memory database honoring repository contract semantics, with tx rollback.
type store struct {
	types    map[uuid.UUID]models.LeaveType
	holidays map[uuid.UUID]models.Holiday
	balances map[uuid.UUID]models.Balance
	requests map[uuid.UUID]models.Request
	ledger   []models.LedgerEntry
	outbox   map[uuid.UUID]models.OutboxEvent
	locks    int
	fail     map[string]error
}

func newStore() *store {
	return &store{types: map[uuid.UUID]models.LeaveType{}, holidays: map[uuid.UUID]models.Holiday{},
		balances: map[uuid.UUID]models.Balance{}, requests: map[uuid.UUID]models.Request{},
		outbox: map[uuid.UUID]models.OutboxEvent{}, fail: map[string]error{}}
}

func cloneMap[K comparable, V any](m map[K]V) map[K]V {
	out := make(map[K]V, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func (s *store) snapshot() store {
	return store{types: cloneMap(s.types), holidays: cloneMap(s.holidays), balances: cloneMap(s.balances),
		requests: cloneMap(s.requests), ledger: append([]models.LedgerEntry(nil), s.ledger...),
		outbox: cloneMap(s.outbox), locks: s.locks, fail: s.fail}
}

func (s *store) err(op string) error { return s.fail[op] }

type fakeRunner struct{ st *store }

func (f fakeRunner) InTx(_ context.Context, fn func(tx *gorm.DB) error) error {
	snap := f.st.snapshot()
	if err := fn(nil); err != nil {
		*f.st = snap
		return err
	}
	return nil
}

func (f fakeRunner) DB() *gorm.DB { return nil }

func pageOf[T any](rows []T, p dto.Page) ([]T, int64) {
	total := int64(len(rows))
	start := p.Offset()
	if start > len(rows) {
		start = len(rows)
	}
	end := start + p.PerPage
	if end > len(rows) {
		end = len(rows)
	}
	return rows[start:end], total
}

// --- types ---

type fakeTypes struct{ st *store }

func (f fakeTypes) Create(_ context.Context, _ *gorm.DB, t *models.LeaveType) error {
	if err := f.st.err("type.create"); err != nil {
		return err
	}
	if t.ID == uuid.Nil {
		t.ID = uuid.New()
	}
	f.st.types[t.ID] = *t
	return nil
}
func (f fakeTypes) Update(_ context.Context, _ *gorm.DB, t *models.LeaveType) error {
	if err := f.st.err("type.update"); err != nil {
		return err
	}
	f.st.types[t.ID] = *t
	return nil
}
func (f fakeTypes) FindByID(_ context.Context, _ *gorm.DB, tenant, id uuid.UUID) (*models.LeaveType, error) {
	if err := f.st.err("type.find"); err != nil {
		return nil, err
	}
	if t, ok := f.st.types[id]; ok && t.TenantID == tenant {
		return &t, nil
	}
	return nil, nil
}
func (f fakeTypes) find(tenant uuid.UUID, match func(models.LeaveType) bool) (*models.LeaveType, error) {
	if err := f.st.err("type.lookup"); err != nil {
		return nil, err
	}
	for _, t := range f.st.types {
		if t.TenantID == tenant && match(t) {
			c := t
			return &c, nil
		}
	}
	return nil, nil
}
func (f fakeTypes) FindByName(_ context.Context, _ *gorm.DB, tenant uuid.UUID, name string) (*models.LeaveType, error) {
	return f.find(tenant, func(t models.LeaveType) bool { return strings.EqualFold(t.Name, name) })
}
func (f fakeTypes) FindByCode(_ context.Context, _ *gorm.DB, tenant uuid.UUID, code string) (*models.LeaveType, error) {
	return f.find(tenant, func(t models.LeaveType) bool { return t.Code == code })
}
func (f fakeTypes) List(_ context.Context, _ *gorm.DB, tenant uuid.UUID, status string, p dto.Page) ([]models.LeaveType, int64, error) {
	if err := f.st.err("type.list"); err != nil {
		return nil, 0, err
	}
	var rows []models.LeaveType
	for _, t := range f.st.types {
		if t.TenantID == tenant && (status == "" || t.Status == status) {
			rows = append(rows, t)
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Code < rows[j].Code })
	out, total := pageOf(rows, p)
	return out, total, nil
}

// --- holidays ---

type fakeHolidays struct{ st *store }

func (f fakeHolidays) Create(_ context.Context, _ *gorm.DB, h *models.Holiday) error {
	if err := f.st.err("holiday.create"); err != nil {
		return err
	}
	if h.ID == uuid.Nil {
		h.ID = uuid.New()
	}
	f.st.holidays[h.ID] = *h
	return nil
}
func (f fakeHolidays) Delete(_ context.Context, _ *gorm.DB, h *models.Holiday) error {
	if err := f.st.err("holiday.delete"); err != nil {
		return err
	}
	delete(f.st.holidays, h.ID)
	return nil
}
func (f fakeHolidays) FindByID(_ context.Context, _ *gorm.DB, tenant, id uuid.UUID) (*models.Holiday, error) {
	if err := f.st.err("holiday.find"); err != nil {
		return nil, err
	}
	if h, ok := f.st.holidays[id]; ok && h.TenantID == tenant {
		return &h, nil
	}
	return nil, nil
}
func (f fakeHolidays) FindByDate(_ context.Context, _ *gorm.DB, tenant uuid.UUID, d time.Time) (*models.Holiday, error) {
	if err := f.st.err("holiday.find"); err != nil {
		return nil, err
	}
	for _, h := range f.st.holidays {
		if h.TenantID == tenant && h.HolidayDate.Equal(d) {
			c := h
			return &c, nil
		}
	}
	return nil, nil
}
func (f fakeHolidays) ListRange(_ context.Context, _ *gorm.DB, tenant uuid.UUID, from, to time.Time) ([]models.Holiday, error) {
	if err := f.st.err("holiday.list"); err != nil {
		return nil, err
	}
	var out []models.Holiday
	for _, h := range f.st.holidays {
		if h.TenantID == tenant && !h.HolidayDate.Before(from) && !h.HolidayDate.After(to) {
			out = append(out, h)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].HolidayDate.Before(out[j].HolidayDate) })
	return out, nil
}

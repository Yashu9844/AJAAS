package services

import (
	"context"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/payroll/dto"
	"github.com/jaas/jaas/internal/payroll/models"
	"github.com/jaas/jaas/internal/payroll/repositories"
	"gorm.io/gorm"
)

type store struct {
	structures  map[uuid.UUID]models.Structure
	components  []models.Component
	assignments map[uuid.UUID]models.Assignment
	runs        map[uuid.UUID]models.Run
	payslips    map[uuid.UUID]models.Payslip
	outbox      map[uuid.UUID]models.OutboxEvent
	locks       int
	fail        map[string]error
}

func newStore() *store {
	return &store{structures: map[uuid.UUID]models.Structure{}, assignments: map[uuid.UUID]models.Assignment{},
		runs: map[uuid.UUID]models.Run{}, payslips: map[uuid.UUID]models.Payslip{}, outbox: map[uuid.UUID]models.OutboxEvent{}, fail: map[string]error{}}
}

func cloneMap[K comparable, V any](m map[K]V) map[K]V {
	out := make(map[K]V, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func (s *store) err(op string) error { return s.fail[op] }

type fakeRunner struct{ st *store }

func (f fakeRunner) InTx(_ context.Context, fn func(*gorm.DB) error) error {
	snap := store{structures: cloneMap(f.st.structures), components: append([]models.Component(nil), f.st.components...),
		assignments: cloneMap(f.st.assignments), runs: cloneMap(f.st.runs), payslips: cloneMap(f.st.payslips),
		outbox: cloneMap(f.st.outbox), locks: f.st.locks, fail: f.st.fail}
	if err := fn(nil); err != nil {
		*f.st = snap
		return err
	}
	return nil
}
func (f fakeRunner) DB() *gorm.DB { return nil }

func pageOf[T any](rows []T, p dto.Page) ([]T, int64) {
	start, end := p.Offset(), p.Offset()+p.PerPage
	if start > len(rows) {
		start = len(rows)
	}
	if end > len(rows) {
		end = len(rows)
	}
	return rows[start:end], int64(len(rows))
}

func ensureID(id *uuid.UUID) {
	if *id == uuid.Nil {
		*id = uuid.New()
	}
}

type fakeStructures struct{ st *store }

func (f fakeStructures) Create(_ context.Context, _ *gorm.DB, s *models.Structure, comps []models.Component) error {
	if err := f.st.err("structure.create"); err != nil {
		return err
	}
	ensureID(&s.ID)
	f.st.structures[s.ID] = *s
	for i := range comps {
		comps[i].TenantID, comps[i].StructureID, comps[i].Position = s.TenantID, s.ID, i
		ensureID(&comps[i].ID)
		f.st.components = append(f.st.components, comps[i])
	}
	return nil
}
func (f fakeStructures) Update(_ context.Context, _ *gorm.DB, s *models.Structure) error {
	if err := f.st.err("structure.update"); err != nil {
		return err
	}
	f.st.structures[s.ID] = *s
	return nil
}
func (f fakeStructures) FindByID(_ context.Context, _ *gorm.DB, t, id uuid.UUID) (*models.Structure, error) {
	if err := f.st.err("structure.find"); err != nil {
		return nil, err
	}
	if s, ok := f.st.structures[id]; ok && s.TenantID == t {
		return &s, nil
	}
	return nil, nil
}
func (f fakeStructures) FindByName(_ context.Context, _ *gorm.DB, t uuid.UUID, name string) (*models.Structure, error) {
	for _, s := range f.st.structures {
		if s.TenantID == t && strings.EqualFold(s.Name, name) {
			c := s
			return &c, nil
		}
	}
	return nil, f.st.err("structure.name")
}
func (f fakeStructures) Components(_ context.Context, _ *gorm.DB, t, id uuid.UUID) ([]models.Component, error) {
	var out []models.Component
	for _, c := range f.st.components {
		if c.TenantID == t && c.StructureID == id {
			out = append(out, c)
		}
	}
	return out, f.st.err("structure.components")
}
func (f fakeStructures) List(_ context.Context, _ *gorm.DB, t uuid.UUID, status string, p dto.Page) ([]models.Structure, int64, error) {
	var rows []models.Structure
	for _, s := range f.st.structures {
		if s.TenantID == t && (status == "" || s.Status == status) {
			rows = append(rows, s)
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Name < rows[j].Name })
	out, n := pageOf(rows, p)
	return out, n, f.st.err("structure.list")
}

type fakeAssignments struct{ st *store }

func (f fakeAssignments) LockEmployee(context.Context, *gorm.DB, uuid.UUID, uuid.UUID) error {
	f.st.locks++
	return f.st.err("assignment.lock")
}
func (f fakeAssignments) Create(_ context.Context, _ *gorm.DB, a *models.Assignment) error {
	if err := f.st.err("assignment.create"); err != nil {
		return err
	}
	ensureID(&a.ID)
	f.st.assignments[a.ID] = *a
	return nil
}
func (f fakeAssignments) Update(_ context.Context, _ *gorm.DB, a *models.Assignment) error {
	f.st.assignments[a.ID] = *a
	return f.st.err("assignment.update")
}
func (f fakeAssignments) Current(_ context.Context, _ *gorm.DB, t, e uuid.UUID) (*models.Assignment, error) {
	for _, a := range f.st.assignments {
		if a.TenantID == t && a.EmployeeProfileID == e && a.EffectiveTo == nil {
			c := a
			return &c, nil
		}
	}
	return nil, f.st.err("assignment.current")
}
func (f fakeAssignments) ListByEmployee(_ context.Context, _ *gorm.DB, t, e uuid.UUID, p dto.Page) ([]models.Assignment, int64, error) {
	var rows []models.Assignment
	for _, a := range f.st.assignments {
		if a.TenantID == t && a.EmployeeProfileID == e {
			rows = append(rows, a)
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].EffectiveFrom.After(rows[j].EffectiveFrom) })
	out, n := pageOf(rows, p)
	return out, n, f.st.err("assignment.list")
}
func (f fakeAssignments) EligibleFor(_ context.Context, _ *gorm.DB, t uuid.UUID, p repositories.Period) ([]models.Assignment, error) {
	latest := map[uuid.UUID]models.Assignment{}
	for _, a := range f.st.assignments {
		overlaps := a.TenantID == t && !a.EffectiveFrom.After(p.To) && (a.EffectiveTo == nil || !a.EffectiveTo.Before(p.From))
		if cur, ok := latest[a.EmployeeProfileID]; overlaps && (!ok || a.EffectiveFrom.After(cur.EffectiveFrom)) {
			latest[a.EmployeeProfileID] = a
		}
	}
	var out []models.Assignment
	for _, a := range latest {
		out = append(out, a)
	}
	return out, f.st.err("assignment.eligible")
}

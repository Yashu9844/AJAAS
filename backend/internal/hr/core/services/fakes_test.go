package services

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/hr/core/dto"
	"github.com/jaas/jaas/internal/hr/core/models"
	"github.com/jaas/jaas/internal/hr/core/repositories"
	employeeAPI "github.com/jaas/jaas/internal/hr/employee/api"
	employeeDTO "github.com/jaas/jaas/internal/hr/employee/dto"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"gorm.io/gorm"
)

var errBoom = errors.New("boom")

// store is an in-memory twin of the core tables; fail injects an error into the named repository method.
type store struct {
	families    map[uuid.UUID]models.JobFamily
	grades      map[uuid.UUID]models.Grade
	profiles    map[uuid.UUID]models.JobProfile
	positions   map[uuid.UUID]models.Position
	assignments map[uuid.UUID]models.Assignment
	actions     map[uuid.UUID]models.Action
	plans       map[uuid.UUID]models.HeadcountPlan
	outbox      map[uuid.UUID]models.OutboxEvent
	fail        map[string]error
	tick        time.Duration
}

func newStore() *store {
	return &store{families: map[uuid.UUID]models.JobFamily{}, grades: map[uuid.UUID]models.Grade{}, profiles: map[uuid.UUID]models.JobProfile{},
		positions: map[uuid.UUID]models.Position{}, assignments: map[uuid.UUID]models.Assignment{}, actions: map[uuid.UUID]models.Action{},
		plans: map[uuid.UUID]models.HeadcountPlan{}, outbox: map[uuid.UUID]models.OutboxEvent{}, fail: map[string]error{}}
}

func cloneMap[K comparable, V any](m map[K]V) map[K]V {
	out := make(map[K]V, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func (s *store) snapshot() store {
	return store{families: cloneMap(s.families), grades: cloneMap(s.grades), profiles: cloneMap(s.profiles), positions: cloneMap(s.positions),
		assignments: cloneMap(s.assignments), actions: cloneMap(s.actions), plans: cloneMap(s.plans), outbox: cloneMap(s.outbox), fail: s.fail, tick: s.tick}
}

func (s *store) stamp() time.Time {
	s.tick += time.Second
	return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).Add(s.tick)
}

type fakeRunner struct{ st *store }

func (f fakeRunner) InTx(_ context.Context, fn func(*gorm.DB) error) error {
	snap := f.st.snapshot()
	if err := fn(nil); err != nil {
		*f.st = snap
		return err
	}
	return nil
}
func (f fakeRunner) DB() *gorm.DB { return nil }

type fakeRepo struct{ st *store }

func (f fakeRepo) err(op string) error { return f.st.fail[op] }

func pageOf[T any](rows []T, p dto.Page) ([]T, int64) {
	start, end := min(p.Offset(), len(rows)), min(p.Offset()+p.PerPage, len(rows))
	return rows[start:end], int64(len(rows))
}

func (f fakeRepo) Create(_ context.Context, _ *gorm.DB, row interface{}) error {
	return f.put("create", row)
}
func (f fakeRepo) Save(_ context.Context, _ *gorm.DB, row interface{}) error {
	return f.put("save", row)
}

func (f fakeRepo) put(op string, row interface{}) error {
	if err := f.err(op); err != nil {
		return err
	}
	switch v := row.(type) {
	case *models.JobFamily:
		ensure(&v.ID)
		f.st.families[v.ID] = *v
	case *models.Grade:
		ensure(&v.ID)
		f.st.grades[v.ID] = *v
	case *models.JobProfile:
		ensure(&v.ID)
		f.st.profiles[v.ID] = *v
	case *models.Position:
		ensure(&v.ID)
		f.st.positions[v.ID] = *v
	case *models.Assignment:
		if err := f.err(op + ".assignment"); err != nil {
			return err
		}
		ensure(&v.ID)
		if v.CreatedAt.IsZero() {
			v.CreatedAt = f.st.stamp()
		}
		f.st.assignments[v.ID] = *v
	case *models.Action:
		if err := f.err(op + ".action"); err != nil {
			return err
		}
		ensure(&v.ID)
		if v.CreatedAt.IsZero() {
			v.CreatedAt = f.st.stamp()
		}
		f.st.actions[v.ID] = *v
	case *models.HeadcountPlan:
		ensure(&v.ID)
		f.st.plans[v.ID] = *v
	case *models.OutboxEvent:
		if err := f.err("outbox"); err != nil {
			return err
		}
		ensure(&v.ID)
		f.st.outbox[v.ID] = *v
	default:
		panic("fake repo: unknown row type")
	}
	return nil
}

func ensure(id *uuid.UUID) {
	if *id == uuid.Nil {
		*id = uuid.New()
	}
}

func (f fakeRepo) CodeTaken(_ context.Context, _ *gorm.DB, model interface{}, tenantID uuid.UUID, code string, except uuid.UUID) (bool, error) {
	if err := f.err("code"); err != nil {
		return false, err
	}
	match := func(t uuid.UUID, c string, id uuid.UUID) bool { return t == tenantID && c == code && id != except }
	switch model.(type) {
	case *models.JobFamily:
		for _, v := range f.st.families {
			if match(v.TenantID, v.Code, v.ID) {
				return true, nil
			}
		}
	case *models.Grade:
		for _, v := range f.st.grades {
			if match(v.TenantID, v.Code, v.ID) {
				return true, nil
			}
		}
	case *models.JobProfile:
		for _, v := range f.st.profiles {
			if match(v.TenantID, v.Code, v.ID) {
				return true, nil
			}
		}
	case *models.Position:
		for _, v := range f.st.positions {
			if match(v.TenantID, v.Code, v.ID) {
				return true, nil
			}
		}
	}
	return false, nil
}

func get[T any](f fakeRepo, op string, m map[uuid.UUID]T, tenant func(T) uuid.UUID, tenantID, id uuid.UUID) (*T, error) {
	if err := f.err(op); err != nil {
		return nil, err
	}
	if v, ok := m[id]; ok && tenant(v) == tenantID {
		return &v, nil
	}
	return nil, nil
}

func (f fakeRepo) Family(_ context.Context, _ *gorm.DB, t, id uuid.UUID) (*models.JobFamily, error) {
	return get(f, "family", f.st.families, func(v models.JobFamily) uuid.UUID { return v.TenantID }, t, id)
}
func (f fakeRepo) Grade(_ context.Context, _ *gorm.DB, t, id uuid.UUID) (*models.Grade, error) {
	return get(f, "grade", f.st.grades, func(v models.Grade) uuid.UUID { return v.TenantID }, t, id)
}
func (f fakeRepo) Profile(_ context.Context, _ *gorm.DB, t, id uuid.UUID) (*models.JobProfile, error) {
	return get(f, "profile", f.st.profiles, func(v models.JobProfile) uuid.UUID { return v.TenantID }, t, id)
}
func (f fakeRepo) Position(_ context.Context, _ *gorm.DB, t, id uuid.UUID) (*models.Position, error) {
	return get(f, "position", f.st.positions, func(v models.Position) uuid.UUID { return v.TenantID }, t, id)
}
func (f fakeRepo) Assignment(_ context.Context, _ *gorm.DB, t, id uuid.UUID) (*models.Assignment, error) {
	return get(f, "assignment", f.st.assignments, func(v models.Assignment) uuid.UUID { return v.TenantID }, t, id)
}
func (f fakeRepo) Action(_ context.Context, _ *gorm.DB, t, id uuid.UUID) (*models.Action, error) {
	return get(f, "action", f.st.actions, func(v models.Action) uuid.UUID { return v.TenantID }, t, id)
}
func (f fakeRepo) LockAction(ctx context.Context, db *gorm.DB, t, id uuid.UUID) (*models.Action, error) {
	if err := f.err("lockAction"); err != nil {
		return nil, err
	}
	return f.Action(ctx, db, t, id)
}

func list[T any](f fakeRepo, op string, m map[uuid.UUID]T, keep func(T) bool, less func(a, b T) bool, p dto.Page) ([]T, int64, error) {
	if err := f.err(op); err != nil {
		return nil, 0, err
	}
	var out []T
	for _, v := range m {
		if keep(v) {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return less(out[i], out[j]) })
	rows, n := pageOf(out, p)
	return rows, n, nil
}

func (f fakeRepo) Families(_ context.Context, _ *gorm.DB, t uuid.UUID, status string, p dto.Page) ([]models.JobFamily, int64, error) {
	return list(f, "families", f.st.families, func(v models.JobFamily) bool { return v.TenantID == t && (status == "" || v.Status == status) },
		func(a, b models.JobFamily) bool { return a.Name < b.Name }, p)
}
func (f fakeRepo) Grades(_ context.Context, _ *gorm.DB, t uuid.UUID, status string, p dto.Page) ([]models.Grade, int64, error) {
	return list(f, "grades", f.st.grades, func(v models.Grade) bool { return v.TenantID == t && (status == "" || v.Status == status) },
		func(a, b models.Grade) bool { return a.Rank < b.Rank }, p)
}
func (f fakeRepo) RankTaken(_ context.Context, _ *gorm.DB, t uuid.UUID, rank int, except uuid.UUID) (bool, error) {
	if err := f.err("rank"); err != nil {
		return false, err
	}
	for _, g := range f.st.grades {
		if g.TenantID == t && g.Rank == rank && g.ID != except {
			return true, nil
		}
	}
	return false, nil
}
func (f fakeRepo) Profiles(_ context.Context, _ *gorm.DB, t uuid.UUID, flt repositories.ProfileFilter, p dto.Page) ([]models.JobProfile, int64, error) {
	return list(f, "profiles", f.st.profiles, func(v models.JobProfile) bool {
		return v.TenantID == t && (flt.Status == "" || v.Status == flt.Status) && (flt.FamilyID == nil || v.FamilyID == *flt.FamilyID) &&
			(flt.Category == "" || v.Category == flt.Category)
	}, func(a, b models.JobProfile) bool { return a.Title < b.Title }, p)
}
func (f fakeRepo) Positions(_ context.Context, _ *gorm.DB, t uuid.UUID, flt repositories.PositionFilter, p dto.Page) ([]models.Position, int64, error) {
	return list(f, "positions", f.st.positions, func(v models.Position) bool {
		return v.TenantID == t && (flt.Status == "" || v.Status == flt.Status) && (flt.DepartmentID == nil || v.DepartmentID == *flt.DepartmentID) &&
			(flt.JobProfileID == nil || v.JobProfileID == *flt.JobProfileID)
	}, func(a, b models.Position) bool { return a.Code < b.Code }, p)
}
func (f fakeRepo) LockWorker(context.Context, *gorm.DB, uuid.UUID) error { return f.err("lockWorker") }
func (f fakeRepo) LockPosition(context.Context, *gorm.DB, uuid.UUID) error {
	return f.err("lockPosition")
}
func (f fakeRepo) WorkerAssignments(_ context.Context, _ *gorm.DB, t, emp uuid.UUID) ([]models.Assignment, error) {
	rows, _, err := list(f, "workerAssignments", f.st.assignments, func(v models.Assignment) bool { return v.TenantID == t && v.EmployeeID == emp },
		func(a, b models.Assignment) bool {
			return a.EffectiveFrom.Before(b.EffectiveFrom) || a.EffectiveFrom.Equal(b.EffectiveFrom) && a.CreatedAt.Before(b.CreatedAt)
		}, everything)
	return rows, err
}
func (f fakeRepo) PositionAssignments(_ context.Context, _ *gorm.DB, t, pos uuid.UUID, from time.Time, to *time.Time) ([]models.Assignment, error) {
	rows, _, err := list(f, "positionAssignments", f.st.assignments, func(v models.Assignment) bool {
		return v.TenantID == t && v.PositionID != nil && *v.PositionID == pos && v.Status == models.AssignmentActive &&
			(v.EffectiveTo == nil || v.EffectiveTo.After(from)) && (to == nil || v.EffectiveFrom.Before(*to))
	}, func(a, b models.Assignment) bool { return a.EffectiveFrom.Before(b.EffectiveFrom) }, everything)
	return rows, err
}
func (f fakeRepo) AsOf(_ context.Context, _ *gorm.DB, t uuid.UUID, d time.Time, flt repositories.AssignmentFilter, p dto.Page) ([]models.Assignment, int64, error) {
	return list(f, "asOf", f.st.assignments, func(v models.Assignment) bool {
		return v.TenantID == t && v.CoversDate(d) && (flt.DepartmentID == nil || sameID(v.DepartmentID, flt.DepartmentID)) &&
			(flt.PositionID == nil || sameID(v.PositionID, flt.PositionID))
	}, func(a, b models.Assignment) bool { return a.EffectiveFrom.Before(b.EffectiveFrom) }, p)
}
func (f fakeRepo) AssignedWorkers(_ context.Context, _ *gorm.DB, t uuid.UUID) (map[uuid.UUID]bool, error) {
	if err := f.err("assignedWorkers"); err != nil {
		return nil, err
	}
	out := map[uuid.UUID]bool{}
	for _, a := range f.st.assignments {
		if a.TenantID == t && a.Status == models.AssignmentActive {
			out[a.EmployeeID] = true
		}
	}
	return out, nil
}
func (f fakeRepo) Actions(_ context.Context, _ *gorm.DB, t uuid.UUID, flt repositories.ActionFilter, p dto.Page) ([]models.Action, int64, error) {
	return list(f, "actions", f.st.actions, func(v models.Action) bool {
		return v.TenantID == t && (flt.EmployeeID == nil || v.EmployeeID == *flt.EmployeeID) && (flt.Status == "" || v.Status == flt.Status) &&
			(flt.Type == "" || v.Type == flt.Type)
	}, func(a, b models.Action) bool { return a.CreatedAt.After(b.CreatedAt) }, p)
}
func (f fakeRepo) CompletedActions(_ context.Context, _ *gorm.DB, t, emp uuid.UUID) ([]models.Action, error) {
	rows, _, err := list(f, "completedActions", f.st.actions, func(v models.Action) bool {
		return v.TenantID == t && v.EmployeeID == emp && v.Status == "completed"
	}, func(a, b models.Action) bool {
		return a.EffectiveDate.After(b.EffectiveDate) || a.EffectiveDate.Equal(b.EffectiveDate) && a.CreatedAt.After(b.CreatedAt)
	}, everything)
	return rows, err
}
func (f fakeRepo) PendingEffects(_ context.Context, _ *gorm.DB, today time.Time, limit int) ([]models.Action, error) {
	rows, _, err := list(f, "pending", f.st.actions, func(v models.Action) bool {
		return v.Status == "completed" && v.EffectsAppliedAt == nil && !v.EffectiveDate.After(today)
	}, func(a, b models.Action) bool { return a.EffectiveDate.Before(b.EffectiveDate) }, dto.Page{Page: 1, PerPage: limit})
	return rows, err
}
func (f fakeRepo) Plans(_ context.Context, _ *gorm.DB, t uuid.UUID, dept *uuid.UUID, p dto.Page) ([]models.HeadcountPlan, int64, error) {
	return list(f, "plans", f.st.plans, func(v models.HeadcountPlan) bool { return v.TenantID == t && (dept == nil || v.DepartmentID == *dept) },
		func(a, b models.HeadcountPlan) bool { return a.PeriodStart.After(b.PeriodStart) }, p)
}
func (f fakeRepo) PlanOverlaps(_ context.Context, _ *gorm.DB, t, dept uuid.UUID, start, end time.Time) (bool, error) {
	if err := f.err("planOverlaps"); err != nil {
		return false, err
	}
	for _, p := range f.st.plans {
		if p.TenantID == t && p.DepartmentID == dept && !p.PeriodStart.After(end) && !p.PeriodEnd.Before(start) {
			return true, nil
		}
	}
	return false, nil
}
func (f fakeRepo) MarkPublished(_ context.Context, _ *gorm.DB, id uuid.UUID, at time.Time) error {
	e := f.st.outbox[id]
	e.Published, e.PublishedAt = true, &at
	f.st.outbox[id] = e
	return nil
}
func (f fakeRepo) RecordFailure(_ context.Context, _ *gorm.DB, id uuid.UUID, reason string) error {
	e := f.st.outbox[id]
	e.Attempts++
	e.LastError = &reason
	f.st.outbox[id] = e
	return nil
}
func (f fakeRepo) FetchUnpublished(_ context.Context, _ *gorm.DB, limit, maxAttempts int) ([]models.OutboxEvent, error) {
	if err := f.err("fetch"); err != nil {
		return nil, err
	}
	var out []models.OutboxEvent
	for _, e := range f.st.outbox {
		if !e.Published && e.Attempts < maxAttempts && len(out) < limit {
			out = append(out, e)
		}
	}
	return out, nil
}

// --- ports ---

// world is the fake HR/Employee, HR/Organization and M0 seen by core.
type world struct {
	emps     map[uuid.UUID]*employeeDTO.EmployeeResponse
	depts    map[uuid.UUID]bool // id → active
	locs     map[uuid.UUID]bool
	mappings map[uuid.UUID][3]*uuid.UUID // user → dept, location, manager user
	logins   map[uuid.UUID]string        // user → active|inactive
	fail     map[string]error
}

func newWorld() *world {
	return &world{emps: map[uuid.UUID]*employeeDTO.EmployeeResponse{}, depts: map[uuid.UUID]bool{}, locs: map[uuid.UUID]bool{},
		mappings: map[uuid.UUID][3]*uuid.UUID{}, logins: map[uuid.UUID]string{}, fail: map[string]error{}}
}

func (w *world) add(tenant uuid.UUID, status string) *employeeDTO.EmployeeResponse {
	e := &employeeDTO.EmployeeResponse{ID: uuid.New(), TenantID: tenant, UserID: uuid.New(), Status: status,
		CreatedAt:  time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		Employment: &employeeDTO.EmploymentDetail{EmploymentType: "part_time", JoiningDate: time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC)}}
	w.emps[e.ID] = e
	w.logins[e.UserID] = "active"
	return e
}

func (w *world) GetByID(_ context.Context, t, id uuid.UUID) (*employeeDTO.EmployeeResponse, error) {
	if err := w.fail["employees"]; err != nil {
		return nil, err
	}
	if e, ok := w.emps[id]; ok && e.TenantID == t {
		cp := *e
		return &cp, nil
	}
	return nil, sharedErrors.ErrNotFound
}
func (w *world) GetByUserID(_ context.Context, t, u uuid.UUID) (*employeeDTO.EmployeeResponse, error) {
	if err := w.fail["byUser"]; err != nil {
		return nil, err
	}
	for _, e := range w.emps {
		if e.UserID == u && e.TenantID == t {
			cp := *e
			return &cp, nil
		}
	}
	return nil, sharedErrors.ErrNotFound
}
func (w *world) List(_ context.Context, t uuid.UUID, f employeeDTO.EmployeeFilter) ([]employeeDTO.EmployeeResponse, int64, error) {
	if err := w.fail["list"]; err != nil {
		return nil, 0, err
	}
	var out []employeeDTO.EmployeeResponse
	for _, e := range w.emps {
		if e.TenantID == t {
			out = append(out, *e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID.String() < out[j].ID.String() })
	rows, n := pageOf(out, dto.Page{Page: f.Page, PerPage: f.PerPage})
	return rows, n, nil
}
func (w *world) ApplyCoreStatus(_ context.Context, _ *gorm.DB, _, id uuid.UUID, ch employeeAPI.StatusChange) (string, error) {
	if err := w.fail["lifecycle"]; err != nil {
		return "", err
	}
	e := w.emps[id]
	prev := e.Status
	e.Status = ch.Status
	return prev, nil
}
func (w *world) DepartmentState(_ context.Context, _, id uuid.UUID) (bool, bool, error) {
	active, ok := w.depts[id]
	return ok, active, w.fail["dept"]
}
func (w *world) LocationState(_ context.Context, _, id uuid.UUID) (bool, bool, error) {
	active, ok := w.locs[id]
	return ok, active, w.fail["loc"]
}
func (w *world) PrimaryPlacement(_ context.Context, _, user uuid.UUID) (*uuid.UUID, *uuid.UUID, *uuid.UUID, error) {
	m := w.mappings[user]
	return m[0], m[1], m[2], w.fail["placement"]
}
func (w *world) ProjectPrimaryMapping(_ context.Context, _ *gorm.DB, _, user uuid.UUID, dept, loc, mgr *uuid.UUID) error {
	if err := w.fail["project"]; err != nil {
		return err
	}
	w.mappings[user] = [3]*uuid.UUID{dept, loc, mgr}
	return nil
}
func (w *world) DeactivateUser(_ context.Context, _ *gorm.DB, _, id, _ uuid.UUID) error {
	if err := w.fail["logins"]; err != nil {
		return err
	}
	if w.logins[id] == "inactive" {
		return &sharedErrors.AppError{Code: "CONFLICT", Message: "already inactive", StatusCode: 409}
	}
	w.logins[id] = "inactive"
	return nil
}
func (w *world) ActivateUser(_ context.Context, _ *gorm.DB, _, id uuid.UUID) error {
	if err := w.fail["logins"]; err != nil {
		return err
	}
	w.logins[id] = "active"
	return nil
}

type audit struct{ rows []string }

func (a *audit) Log(_ context.Context, _ *gorm.DB, _, _ string, action, _, _ string, meta interface{}, _, _ string) error {
	a.rows = append(a.rows, action)
	_ = meta
	return nil
}

type publisher struct{ err error }

func (p *publisher) Publish(context.Context, string, string, interface{}) error { return p.err }

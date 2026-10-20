package services

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/google/uuid"
	employeeDTO "github.com/jaas/jaas/internal/hr/employee/dto"
	"github.com/jaas/jaas/internal/hr/performance/dto"
	"github.com/jaas/jaas/internal/hr/performance/models"
	"github.com/jaas/jaas/internal/hr/performance/repositories"
	"github.com/jaas/jaas/internal/hr/performance/score"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"gorm.io/gorm"
)

var errBoom = errors.New("boom")

// store is an in-memory twin of the HR/Performance tables; fail injects an error per operation name.
type store struct {
	cycles   map[uuid.UUID]models.Cycle
	goals    map[uuid.UUID]models.Goal
	krs      map[uuid.UUID]models.KeyResult
	checkins []models.CheckIn
	reviews  map[uuid.UUID]models.Review
	outbox   map[uuid.UUID]models.OutboxEvent
	fail     map[string]error
	tick     time.Duration
}

func newStore() *store {
	return &store{cycles: map[uuid.UUID]models.Cycle{}, goals: map[uuid.UUID]models.Goal{}, krs: map[uuid.UUID]models.KeyResult{},
		reviews: map[uuid.UUID]models.Review{}, outbox: map[uuid.UUID]models.OutboxEvent{}, fail: map[string]error{}}
}

func cloneMap[K comparable, V any](m map[K]V) map[K]V {
	out := make(map[K]V, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func (s *store) err(op string) error { return s.fail[op] }

// stamp gives rows strictly increasing timestamps so orderings are deterministic.
func (s *store) stamp() time.Time {
	s.tick += time.Second
	return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).Add(s.tick)
}

type fakeRunner struct{ st *store }

func (f fakeRunner) InTx(_ context.Context, fn func(*gorm.DB) error) error {
	snap := store{cycles: cloneMap(f.st.cycles), goals: cloneMap(f.st.goals), krs: cloneMap(f.st.krs), checkins: append([]models.CheckIn(nil), f.st.checkins...),
		reviews: cloneMap(f.st.reviews), outbox: cloneMap(f.st.outbox), fail: f.st.fail, tick: f.st.tick}
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

// --- cycles ---

type fakeCycles struct{ st *store }

func (f fakeCycles) Create(_ context.Context, _ *gorm.DB, c *models.Cycle) error {
	if err := f.st.err("cycles.create"); err != nil {
		return err
	}
	ensureID(&c.ID)
	f.st.cycles[c.ID] = *c
	return nil
}
func (f fakeCycles) Update(_ context.Context, _ *gorm.DB, c *models.Cycle) error {
	if err := f.st.err("cycles.update"); err != nil {
		return err
	}
	f.st.cycles[c.ID] = *c
	return nil
}
func (f fakeCycles) FindByID(_ context.Context, _ *gorm.DB, tenantID, id uuid.UUID) (*models.Cycle, error) {
	if err := f.st.err("cycles.find"); err != nil {
		return nil, err
	}
	if c, ok := f.st.cycles[id]; ok && c.TenantID == tenantID {
		return &c, nil
	}
	return nil, nil
}
func (f fakeCycles) LockByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Cycle, error) {
	if err := f.st.err("cycles.lock"); err != nil {
		return nil, err
	}
	return f.FindByID(ctx, db, tenantID, id)
}
func (f fakeCycles) List(_ context.Context, _ *gorm.DB, tenantID uuid.UUID, status string, p dto.Page) ([]models.Cycle, int64, error) {
	if err := f.st.err("cycles.list"); err != nil {
		return nil, 0, err
	}
	var out []models.Cycle
	for _, c := range f.st.cycles {
		if c.TenantID == tenantID && (status == "" || c.Status == status) {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	rows, n := pageOf(out, p)
	return rows, n, nil
}

// --- goals ---

type fakeGoals struct{ st *store }

func (f fakeGoals) Create(_ context.Context, _ *gorm.DB, g *models.Goal) error {
	if err := f.st.err("goals.create"); err != nil {
		return err
	}
	ensureID(&g.ID)
	g.CreatedAt = f.st.stamp()
	f.st.goals[g.ID] = *g
	return nil
}
func (f fakeGoals) Update(_ context.Context, _ *gorm.DB, g *models.Goal) error {
	if err := f.st.err("goals.update"); err != nil {
		return err
	}
	f.st.goals[g.ID] = *g
	return nil
}
func (f fakeGoals) FindByID(_ context.Context, _ *gorm.DB, tenantID, id uuid.UUID) (*models.Goal, error) {
	if err := f.st.err("goals.find"); err != nil {
		return nil, err
	}
	if g, ok := f.st.goals[id]; ok && g.TenantID == tenantID {
		return &g, nil
	}
	return nil, nil
}
func (f fakeGoals) LockByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Goal, error) {
	if err := f.st.err("goals.lock"); err != nil {
		return nil, err
	}
	return f.FindByID(ctx, db, tenantID, id)
}
func (f fakeGoals) List(_ context.Context, _ *gorm.DB, tenantID uuid.UUID, flt repositories.GoalFilter, p dto.Page) ([]models.Goal, int64, error) {
	if err := f.st.err("goals.list"); err != nil {
		return nil, 0, err
	}
	var out []models.Goal
	for _, g := range f.st.goals {
		if g.TenantID == tenantID && (flt.CycleID == nil || g.CycleID == *flt.CycleID) && (flt.EmployeeID == nil || g.EmployeeID == *flt.EmployeeID) &&
			(flt.Status == "" || g.Status == flt.Status) {
			out = append(out, g)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	rows, n := pageOf(out, p)
	return rows, n, nil
}
func (f fakeGoals) LockOwner(context.Context, *gorm.DB, uuid.UUID, uuid.UUID) error { return f.st.err("goals.lockowner") }
func (f fakeGoals) WeightSum(_ context.Context, _ *gorm.DB, tenantID, cycleID, employeeID, exclude uuid.UUID) (score.Fixed, error) {
	if err := f.st.err("goals.weightsum"); err != nil {
		return 0, err
	}
	var sum score.Fixed
	for _, g := range f.st.goals {
		if g.TenantID == tenantID && g.CycleID == cycleID && g.EmployeeID == employeeID && g.Status != "cancelled" && g.ID != exclude {
			sum += g.Weight
		}
	}
	return sum, nil
}
func (f fakeGoals) Open(_ context.Context, _ *gorm.DB, tenantID, cycleID uuid.UUID) ([]models.Goal, error) {
	if err := f.st.err("goals.open"); err != nil {
		return nil, err
	}
	var out []models.Goal
	for _, g := range f.st.goals {
		if g.TenantID == tenantID && g.CycleID == cycleID && g.Status != "cancelled" {
			out = append(out, g)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}
func (f fakeGoals) AddKeyResult(_ context.Context, _ *gorm.DB, k *models.KeyResult) error {
	if err := f.st.err("krs.create"); err != nil {
		return err
	}
	ensureID(&k.ID)
	k.CreatedAt = f.st.stamp()
	f.st.krs[k.ID] = *k
	return nil
}
func (f fakeGoals) UpdateKeyResult(_ context.Context, _ *gorm.DB, k *models.KeyResult) error {
	if err := f.st.err("krs.update"); err != nil {
		return err
	}
	f.st.krs[k.ID] = *k
	return nil
}
func (f fakeGoals) KeyResults(_ context.Context, _ *gorm.DB, tenantID, goalID uuid.UUID) ([]models.KeyResult, error) {
	if err := f.st.err("krs.list"); err != nil {
		return nil, err
	}
	var out []models.KeyResult
	for _, k := range f.st.krs {
		if k.TenantID == tenantID && k.GoalID == goalID {
			out = append(out, k)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}
func (f fakeGoals) AddCheckIn(_ context.Context, _ *gorm.DB, c *models.CheckIn) error {
	if err := f.st.err("checkins.create"); err != nil {
		return err
	}
	ensureID(&c.ID)
	c.CreatedAt = f.st.stamp()
	f.st.checkins = append(f.st.checkins, *c)
	return nil
}
func (f fakeGoals) CheckIns(_ context.Context, _ *gorm.DB, tenantID, goalID uuid.UUID, limit int) ([]models.CheckIn, error) {
	if err := f.st.err("checkins.list"); err != nil {
		return nil, err
	}
	var out []models.CheckIn
	for i := len(f.st.checkins) - 1; i >= 0 && len(out) < limit; i-- {
		if c := f.st.checkins[i]; c.TenantID == tenantID && c.GoalID == goalID {
			out = append(out, c)
		}
	}
	return out, nil
}

// --- reviews ---

type fakeReviews struct{ st *store }

func (f fakeReviews) Create(_ context.Context, _ *gorm.DB, r *models.Review) error {
	if err := f.st.err("reviews.create"); err != nil {
		return err
	}
	ensureID(&r.ID)
	r.CreatedAt = f.st.stamp()
	f.st.reviews[r.ID] = *r
	return nil
}
func (f fakeReviews) Update(_ context.Context, _ *gorm.DB, r *models.Review) error {
	if err := f.st.err("reviews.update"); err != nil {
		return err
	}
	f.st.reviews[r.ID] = *r
	return nil
}
func (f fakeReviews) FindByID(_ context.Context, _ *gorm.DB, tenantID, id uuid.UUID) (*models.Review, error) {
	if err := f.st.err("reviews.find"); err != nil {
		return nil, err
	}
	if r, ok := f.st.reviews[id]; ok && r.TenantID == tenantID {
		return &r, nil
	}
	return nil, nil
}
func (f fakeReviews) LockByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Review, error) {
	if err := f.st.err("reviews.lock"); err != nil {
		return nil, err
	}
	return f.FindByID(ctx, db, tenantID, id)
}
func (f fakeReviews) Exists(_ context.Context, _ *gorm.DB, tenantID, cycleID, employeeID, reviewerID uuid.UUID, typ string) (bool, error) {
	if err := f.st.err("reviews.exists"); err != nil {
		return false, err
	}
	for _, r := range f.st.reviews {
		if r.TenantID == tenantID && r.CycleID == cycleID && r.EmployeeID == employeeID && r.ReviewerEmployeeID == reviewerID && r.Type == typ {
			return true, nil
		}
	}
	return false, nil
}
func (f fakeReviews) List(_ context.Context, _ *gorm.DB, tenantID uuid.UUID, flt repositories.ReviewFilter, p dto.Page) ([]models.Review, int64, error) {
	if err := f.st.err("reviews.list"); err != nil {
		return nil, 0, err
	}
	var out []models.Review
	for _, r := range f.st.reviews {
		if r.TenantID == tenantID && (flt.CycleID == nil || r.CycleID == *flt.CycleID) && (flt.EmployeeID == nil || r.EmployeeID == *flt.EmployeeID) &&
			(flt.ReviewerID == nil || r.ReviewerEmployeeID == *flt.ReviewerID) && (flt.Status == "" || r.Status == flt.Status) && (flt.Type == "" || r.Type == flt.Type) {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	rows, n := pageOf(out, p)
	return rows, n, nil
}
func (f fakeReviews) ExpirePending(_ context.Context, _ *gorm.DB, tenantID, cycleID uuid.UUID, at time.Time) (int64, error) {
	if err := f.st.err("reviews.expire"); err != nil {
		return 0, err
	}
	var n int64
	for id, r := range f.st.reviews {
		if r.TenantID == tenantID && r.CycleID == cycleID && r.Status == models.ReviewPending {
			r.Status, r.UpdatedAt = models.ReviewExpired, at
			f.st.reviews[id] = r
			n++
		}
	}
	return n, nil
}

// --- outbox ---

type fakeOutbox struct{ st *store }

func (f fakeOutbox) Create(_ context.Context, _ *gorm.DB, e *models.OutboxEvent) error {
	if err := f.st.err("outbox.create"); err != nil {
		return err
	}
	ensureID(&e.ID)
	e.CreatedAt = f.st.stamp()
	f.st.outbox[e.ID] = *e
	return nil
}
func (f fakeOutbox) MarkPublished(_ context.Context, _ *gorm.DB, id uuid.UUID, at time.Time) error {
	if err := f.st.err("outbox.mark"); err != nil {
		return err
	}
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
	if err := f.st.err("outbox.fetch"); err != nil {
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

// --- collaborators ---

// people is the fake HR/Employee directory plus the HR/Organization manager map (user → manager user).
type people struct {
	byID     map[uuid.UUID]*employeeDTO.EmployeeResponse
	managers map[uuid.UUID]uuid.UUID
	fail     map[string]error
}

func newPeople() *people {
	return &people{byID: map[uuid.UUID]*employeeDTO.EmployeeResponse{}, managers: map[uuid.UUID]uuid.UUID{}, fail: map[string]error{}}
}

func (p *people) add(tenantID uuid.UUID, status string) *employeeDTO.EmployeeResponse {
	e := &employeeDTO.EmployeeResponse{ID: uuid.New(), TenantID: tenantID, UserID: uuid.New(), Status: status}
	p.byID[e.ID] = e
	return e
}

func (p *people) GetByID(_ context.Context, tenantID, id uuid.UUID) (*employeeDTO.EmployeeResponse, error) {
	if err := p.fail["employees.get"]; err != nil {
		return nil, err
	}
	if e, ok := p.byID[id]; ok && e.TenantID == tenantID {
		return e, nil
	}
	return nil, sharedErrors.ErrNotFound
}

func (p *people) GetByUserID(_ context.Context, tenantID, userID uuid.UUID) (*employeeDTO.EmployeeResponse, error) {
	if err := p.fail["employees.byuser"]; err != nil {
		return nil, err
	}
	for _, e := range p.byID {
		if e.UserID == userID && e.TenantID == tenantID {
			return e, nil
		}
	}
	return nil, sharedErrors.ErrNotFound
}

func (p *people) DirectManager(_ context.Context, _ uuid.UUID, userID uuid.UUID) (*uuid.UUID, error) {
	if err := p.fail["managers"]; err != nil {
		return nil, err
	}
	if m, ok := p.managers[userID]; ok {
		return &m, nil
	}
	return nil, nil
}

type audit struct{ rows []interface{} }

func (a *audit) Log(_ context.Context, _ *gorm.DB, _, _ string, action, _, _ string, metadata interface{}, _, _ string) error {
	a.rows = append(a.rows, []interface{}{action, metadata})
	return nil
}

type publisher struct {
	err  error
	sent []string
}

func (p *publisher) Publish(_ context.Context, _, routingKey string, _ interface{}) error {
	if p.err != nil {
		return p.err
	}
	p.sent = append(p.sent, routingKey)
	return nil
}

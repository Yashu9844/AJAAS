package services

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/recruitment/dto"
	"github.com/jaas/jaas/internal/recruitment/models"
	"github.com/jaas/jaas/internal/recruitment/repositories"
	"gorm.io/gorm"
)

type store struct {
	jobs       map[uuid.UUID]models.Job
	candidates map[uuid.UUID]models.Candidate
	events     []models.StageEvent
	interviews map[uuid.UUID]models.Interview
	offers     map[uuid.UUID]models.Offer
	outbox     map[uuid.UUID]models.OutboxEvent
	fail       map[string]error
	tick       time.Duration
}

func newStore() *store {
	return &store{jobs: map[uuid.UUID]models.Job{}, candidates: map[uuid.UUID]models.Candidate{}, interviews: map[uuid.UUID]models.Interview{},
		offers: map[uuid.UUID]models.Offer{}, outbox: map[uuid.UUID]models.OutboxEvent{}, fail: map[string]error{}}
}

func cloneMap[K comparable, V any](m map[K]V) map[K]V {
	out := make(map[K]V, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func (s *store) err(op string) error { return s.fail[op] }

// stamp gives rows strictly increasing CreatedAt so "latest" ordering is deterministic.
func (s *store) stamp() time.Time {
	s.tick += time.Second
	return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).Add(s.tick)
}

type fakeRunner struct{ st *store }

func (f fakeRunner) InTx(_ context.Context, fn func(*gorm.DB) error) error {
	snap := store{jobs: cloneMap(f.st.jobs), candidates: cloneMap(f.st.candidates), events: append([]models.StageEvent(nil), f.st.events...),
		interviews: cloneMap(f.st.interviews), offers: cloneMap(f.st.offers), outbox: cloneMap(f.st.outbox), fail: f.st.fail, tick: f.st.tick}
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

func scoped[T any](m map[uuid.UUID]T, id uuid.UUID, tenant func(T) uuid.UUID, t uuid.UUID) *T {
	if v, ok := m[id]; ok && tenant(v) == t {
		return &v
	}
	return nil
}

type fakeJobs struct{ st *store }

func (f fakeJobs) Create(_ context.Context, _ *gorm.DB, j *models.Job) error {
	ensureID(&j.ID)
	j.CreatedAt = f.st.stamp()
	f.st.jobs[j.ID] = *j
	return f.st.err("job.create")
}
func (f fakeJobs) Update(_ context.Context, _ *gorm.DB, j *models.Job) error {
	f.st.jobs[j.ID] = *j
	return f.st.err("job.update")
}
func (f fakeJobs) FindByID(_ context.Context, _ *gorm.DB, t, id uuid.UUID) (*models.Job, error) {
	return scoped(f.st.jobs, id, func(j models.Job) uuid.UUID { return j.TenantID }, t), f.st.err("job.find")
}
func (f fakeJobs) LockByID(_ context.Context, _ *gorm.DB, t, id uuid.UUID) (*models.Job, error) {
	return scoped(f.st.jobs, id, func(j models.Job) uuid.UUID { return j.TenantID }, t), f.st.err("job.lock")
}
func (f fakeJobs) List(_ context.Context, _ *gorm.DB, t uuid.UUID, flt repositories.JobFilter, p dto.Page) ([]models.Job, int64, error) {
	var rows []models.Job
	for _, j := range f.st.jobs {
		if j.TenantID == t && (flt.Status == "" || j.Status == flt.Status) && (flt.DepartmentID == nil || (j.DepartmentID != nil && *j.DepartmentID == *flt.DepartmentID)) {
			rows = append(rows, j)
		}
	}
	sort.Slice(rows, func(i, k int) bool { return rows[i].CreatedAt.After(rows[k].CreatedAt) })
	out, n := pageOf(rows, p)
	return out, n, f.st.err("job.list")
}

type fakeCandidates struct{ st *store }

func (f fakeCandidates) Create(_ context.Context, _ *gorm.DB, c *models.Candidate) error {
	ensureID(&c.ID)
	c.CreatedAt = f.st.stamp()
	f.st.candidates[c.ID] = *c
	return f.st.err("candidate.create")
}
func (f fakeCandidates) Update(_ context.Context, _ *gorm.DB, c *models.Candidate) error {
	f.st.candidates[c.ID] = *c
	return f.st.err("candidate.update")
}
func (f fakeCandidates) FindByID(_ context.Context, _ *gorm.DB, t, id uuid.UUID) (*models.Candidate, error) {
	return scoped(f.st.candidates, id, func(c models.Candidate) uuid.UUID { return c.TenantID }, t), f.st.err("candidate.find")
}
func (f fakeCandidates) LockByID(_ context.Context, _ *gorm.DB, t, id uuid.UUID) (*models.Candidate, error) {
	return scoped(f.st.candidates, id, func(c models.Candidate) uuid.UUID { return c.TenantID }, t), f.st.err("candidate.lock")
}
func (f fakeCandidates) FindByJobEmail(_ context.Context, _ *gorm.DB, job uuid.UUID, email string) (*models.Candidate, error) {
	for _, c := range f.st.candidates {
		if c.JobID == job && strings.EqualFold(c.Email, email) {
			return &c, nil
		}
	}
	return nil, f.st.err("candidate.email")
}
func (f fakeCandidates) List(_ context.Context, _ *gorm.DB, t uuid.UUID, flt repositories.CandidateFilter, p dto.Page) ([]models.Candidate, int64, error) {
	var rows []models.Candidate
	for _, c := range f.st.candidates {
		if c.TenantID == t && (flt.JobID == nil || c.JobID == *flt.JobID) && (flt.Stage == "" || c.Stage == flt.Stage) {
			rows = append(rows, c)
		}
	}
	sort.Slice(rows, func(i, k int) bool { return rows[i].CreatedAt.After(rows[k].CreatedAt) })
	out, n := pageOf(rows, p)
	return out, n, f.st.err("candidate.list")
}
func (f fakeCandidates) AddEvent(_ context.Context, _ *gorm.DB, e *models.StageEvent) error {
	ensureID(&e.ID)
	e.CreatedAt = f.st.stamp()
	f.st.events = append(f.st.events, *e)
	return f.st.err("candidate.event")
}
func (f fakeCandidates) Events(_ context.Context, _ *gorm.DB, t, id uuid.UUID) ([]models.StageEvent, error) {
	var out []models.StageEvent
	for _, e := range f.st.events {
		if e.TenantID == t && e.CandidateID == id {
			out = append(out, e)
		}
	}
	return out, f.st.err("candidate.events")
}

type fakeInterviews struct{ st *store }

func (f fakeInterviews) Create(_ context.Context, _ *gorm.DB, i *models.Interview) error {
	ensureID(&i.ID)
	f.st.interviews[i.ID] = *i
	return f.st.err("interview.create")
}
func (f fakeInterviews) Update(_ context.Context, _ *gorm.DB, i *models.Interview) error {
	f.st.interviews[i.ID] = *i
	return f.st.err("interview.update")
}
func (f fakeInterviews) FindByID(_ context.Context, _ *gorm.DB, t, id uuid.UUID) (*models.Interview, error) {
	return scoped(f.st.interviews, id, func(i models.Interview) uuid.UUID { return i.TenantID }, t), f.st.err("interview.find")
}
func (f fakeInterviews) list(t uuid.UUID, keep func(models.Interview) bool, p dto.Page) ([]models.Interview, int64, error) {
	var rows []models.Interview
	for _, i := range f.st.interviews {
		if i.TenantID == t && keep(i) {
			rows = append(rows, i)
		}
	}
	sort.Slice(rows, func(a, b int) bool { return rows[a].ScheduledAt.Before(rows[b].ScheduledAt) })
	out, n := pageOf(rows, p)
	return out, n, f.st.err("interview.list")
}
func (f fakeInterviews) ListByCandidate(_ context.Context, _ *gorm.DB, t, c uuid.UUID, p dto.Page) ([]models.Interview, int64, error) {
	return f.list(t, func(i models.Interview) bool { return i.CandidateID == c }, p)
}
func (f fakeInterviews) ListByInterviewer(_ context.Context, _ *gorm.DB, t, e uuid.UUID, status string, p dto.Page) ([]models.Interview, int64, error) {
	return f.list(t, func(i models.Interview) bool {
		return i.InterviewerEmployeeID == e && (status == "" || i.Status == status)
	}, p)
}

type fakeOffers struct{ st *store }

func (f fakeOffers) Create(_ context.Context, _ *gorm.DB, o *models.Offer) error {
	ensureID(&o.ID)
	o.CreatedAt = f.st.stamp()
	f.st.offers[o.ID] = *o
	return f.st.err("offer.create")
}
func (f fakeOffers) Update(_ context.Context, _ *gorm.DB, o *models.Offer) error {
	f.st.offers[o.ID] = *o
	return f.st.err("offer.update")
}
func (f fakeOffers) FindByID(_ context.Context, _ *gorm.DB, t, id uuid.UUID) (*models.Offer, error) {
	return scoped(f.st.offers, id, func(o models.Offer) uuid.UUID { return o.TenantID }, t), f.st.err("offer.find")
}
func (f fakeOffers) byCandidate(t, c uuid.UUID) []models.Offer {
	var rows []models.Offer
	for _, o := range f.st.offers {
		if o.TenantID == t && o.CandidateID == c {
			rows = append(rows, o)
		}
	}
	sort.Slice(rows, func(i, k int) bool { return rows[i].CreatedAt.After(rows[k].CreatedAt) })
	return rows
}
func (f fakeOffers) LatestByStatus(_ context.Context, _ *gorm.DB, t, c uuid.UUID, status string) (*models.Offer, error) {
	if err := f.st.err("offer.latest"); err != nil {
		return nil, err
	}
	for _, o := range f.byCandidate(t, c) {
		if o.Status == status {
			return &o, nil
		}
	}
	return nil, f.st.err("offer.latest")
}
func (f fakeOffers) ListByCandidate(_ context.Context, _ *gorm.DB, t, c uuid.UUID, p dto.Page) ([]models.Offer, int64, error) {
	out, n := pageOf(f.byCandidate(t, c), p)
	return out, n, f.st.err("offer.list")
}

type fakeOutbox struct{ st *store }

func (f fakeOutbox) Create(_ context.Context, _ *gorm.DB, e *models.OutboxEvent) error {
	ensureID(&e.ID)
	e.CreatedAt = f.st.stamp()
	f.st.outbox[e.ID] = *e
	return f.st.err("outbox.create")
}
func (f fakeOutbox) MarkPublished(_ context.Context, _ *gorm.DB, id uuid.UUID, at time.Time) error {
	e := f.st.outbox[id]
	e.Published, e.PublishedAt = true, &at
	f.st.outbox[id] = e
	return f.st.err("outbox.mark")
}
func (f fakeOutbox) RecordFailure(_ context.Context, _ *gorm.DB, id uuid.UUID, reason string) error {
	e := f.st.outbox[id]
	e.Attempts++
	e.LastError = &reason
	f.st.outbox[id] = e
	return nil
}
func (f fakeOutbox) FetchUnpublished(_ context.Context, _ *gorm.DB, limit, max int) ([]models.OutboxEvent, error) {
	var rows []models.OutboxEvent
	for _, e := range f.st.outbox {
		if !e.Published && e.Attempts < max {
			rows = append(rows, e)
		}
	}
	sort.Slice(rows, func(i, k int) bool { return rows[i].CreatedAt.Before(rows[k].CreatedAt) })
	if len(rows) > limit {
		rows = rows[:limit]
	}
	return rows, f.st.err("outbox.fetch")
}

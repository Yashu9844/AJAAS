package compensation

import (
	"context"
	"errors"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	coreAPI "github.com/jaas/jaas/internal/hr/core/api"
	employeeDTO "github.com/jaas/jaas/internal/hr/employee/dto"
	"github.com/jaas/jaas/internal/hr/kernel"
	"github.com/jaas/jaas/internal/identity/access"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"github.com/jaas/jaas/internal/shared/identityport"
	"gorm.io/gorm"
)

var errBoom = errors.New("boom")

// --- fakes ---

type fakeStore struct {
	ranges    map[uuid.UUID]PayRange
	records   map[uuid.UUID]Record
	changes   map[uuid.UUID]Change
	cycles    map[uuid.UUID]Cycle
	budgets   map[uuid.UUID][]Budget
	proposals map[uuid.UUID]Proposal
	events    []string
	fail      map[string]error
}

func newStore() *fakeStore {
	return &fakeStore{ranges: map[uuid.UUID]PayRange{}, records: map[uuid.UUID]Record{}, changes: map[uuid.UUID]Change{},
		cycles: map[uuid.UUID]Cycle{}, budgets: map[uuid.UUID][]Budget{}, proposals: map[uuid.UUID]Proposal{}, fail: map[string]error{}}
}

func clone[K comparable, V any](m map[K]V) map[K]V {
	out := make(map[K]V, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func (f *fakeStore) InTx(_ context.Context, fn func(*gorm.DB) error) error {
	r, rec, c, cy, b, p, ev := clone(f.ranges), clone(f.records), clone(f.changes), clone(f.cycles), clone(f.budgets), clone(f.proposals), len(f.events)
	if err := fn(nil); err != nil {
		f.ranges, f.records, f.changes, f.cycles, f.budgets, f.proposals, f.events = r, rec, c, cy, b, p, f.events[:ev]
		return err
	}
	return nil
}
func (f *fakeStore) DB() *gorm.DB                                 { return nil }
func (f *fakeStore) Lock(context.Context, *gorm.DB, string) error { return f.fail["lock"] }
func (f *fakeStore) Create(_ context.Context, _ *gorm.DB, row interface{}) error {
	return f.put("create", row)
}
func (f *fakeStore) Save(_ context.Context, _ *gorm.DB, row interface{}) error {
	return f.put("save", row)
}
func (f *fakeStore) put(op string, row interface{}) error {
	if err := f.fail[op]; err != nil {
		return err
	}
	switch v := row.(type) {
	case *PayRange:
		_ = v.BeforeCreate(nil)
		f.ranges[v.ID] = *v
	case *Record:
		_ = v.BeforeCreate(nil)
		f.records[v.ID] = *v
	case *Change:
		_ = v.BeforeCreate(nil)
		f.changes[v.ID] = *v
	case *Cycle:
		_ = v.BeforeCreate(nil)
		f.cycles[v.ID] = *v
	case *Proposal:
		_ = v.BeforeCreate(nil)
		f.proposals[v.ID] = *v
	}
	return nil
}
func (f *fakeStore) Ranges(_ context.Context, _ *gorm.DB, _ uuid.UUID, g *uuid.UUID) ([]PayRange, error) {
	var out []PayRange
	for _, r := range f.ranges {
		if g == nil || r.GradeID == *g {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].EffectiveFrom.After(out[j].EffectiveFrom.Time) })
	return out, f.fail["ranges"]
}
func (f *fakeStore) Records(_ context.Context, _ *gorm.DB, _, e uuid.UUID) ([]Record, error) {
	var out []Record
	for _, r := range f.records {
		if r.EmployeeProfileID == e {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].EffectiveFrom.Before(out[j].EffectiveFrom.Time) })
	return out, f.fail["records"]
}
func (f *fakeStore) Change(_ context.Context, _ *gorm.DB, _, id uuid.UUID, _ bool) (*Change, error) {
	if c, ok := f.changes[id]; ok {
		return &c, f.fail["change"]
	}
	return nil, f.fail["change"]
}
func (f *fakeStore) Changes(_ context.Context, _ *gorm.DB, _ uuid.UUID, flt ChangeFilter, _, _ int) ([]Change, int64, error) {
	var out []Change
	for _, c := range f.changes {
		if (flt.EmployeeID == nil || c.EmployeeProfileID == *flt.EmployeeID) && (flt.Status == "" || c.Status == flt.Status) {
			out = append(out, c)
		}
	}
	return out, int64(len(out)), f.fail["changes"]
}
func (f *fakeStore) Cycle(_ context.Context, _ *gorm.DB, _, id uuid.UUID, _ bool) (*Cycle, error) {
	if c, ok := f.cycles[id]; ok {
		return &c, f.fail["cycle"]
	}
	return nil, f.fail["cycle"]
}
func (f *fakeStore) Cycles(_ context.Context, _ *gorm.DB, _ uuid.UUID, status string, _, _ int) ([]Cycle, int64, error) {
	var out []Cycle
	for _, c := range f.cycles {
		if status == "" || c.Status == status {
			out = append(out, c)
		}
	}
	return out, int64(len(out)), f.fail["cycles"]
}
func (f *fakeStore) Budgets(_ context.Context, _ *gorm.DB, _, c uuid.UUID) ([]Budget, error) {
	return append([]Budget{}, f.budgets[c]...), f.fail["budgets"]
}
func (f *fakeStore) ReplaceBudgets(_ context.Context, _ *gorm.DB, _, c uuid.UUID, rows []Budget) error {
	f.budgets[c] = rows
	return f.fail["replace"]
}
func (f *fakeStore) Proposals(_ context.Context, _ *gorm.DB, _, c uuid.UUID, m *uuid.UUID) ([]Proposal, error) {
	var out []Proposal
	for _, p := range f.proposals {
		if p.CycleID == c && (m == nil || (p.ManagerEmployeeID != nil && *p.ManagerEmployeeID == *m)) {
			out = append(out, p)
		}
	}
	return out, f.fail["proposals"]
}
func (f *fakeStore) Proposal(_ context.Context, _ *gorm.DB, _, id uuid.UUID, _ bool) (*Proposal, error) {
	if p, ok := f.proposals[id]; ok {
		return &p, f.fail["proposal"]
	}
	return nil, f.fail["proposal"]
}
func (f *fakeStore) Event(_ context.Context, _ *gorm.DB, _ uuid.UUID, key string, _ map[string]string) error {
	if err := f.fail["event"]; err != nil {
		return err
	}
	f.events = append(f.events, key)
	return nil
}

type fakeEmployees struct {
	byID map[uuid.UUID]employeeDTO.EmployeeResponse
	err  error
}

func (e *fakeEmployees) GetByID(_ context.Context, _, id uuid.UUID) (*employeeDTO.EmployeeResponse, error) {
	if r, ok := e.byID[id]; ok {
		return &r, e.err
	}
	return nil, sharedErrors.ErrNotFound
}
func (e *fakeEmployees) GetByUserID(_ context.Context, _, u uuid.UUID) (*employeeDTO.EmployeeResponse, error) {
	for _, r := range e.byID {
		if r.UserID == u {
			r := r
			return &r, e.err
		}
	}
	return nil, sharedErrors.ErrNotFound
}
func (e *fakeEmployees) List(_ context.Context, _ uuid.UUID, f employeeDTO.EmployeeFilter) ([]employeeDTO.EmployeeResponse, int64, error) {
	var all []employeeDTO.EmployeeResponse
	for _, r := range e.byID {
		all = append(all, r)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].ID.String() < all[j].ID.String() })
	lo := (f.Page - 1) * f.PerPage
	if lo > len(all) {
		lo = len(all)
	}
	return all[lo:min(len(all), lo+f.PerPage)], int64(len(all)), e.err
}

type fakeCore struct {
	place  map[uuid.UUID]*coreAPI.Placement
	grades map[uuid.UUID]bool
	err    error
}

func (c *fakeCore) PlacementOn(_ context.Context, _, e uuid.UUID, _ time.Time) (*coreAPI.Placement, error) {
	return c.place[e], c.err
}
func (c *fakeCore) GradeState(_ context.Context, _, g uuid.UUID) (bool, bool, error) {
	active, ok := c.grades[g]
	return ok, active, c.err
}

type fakeRatings struct {
	bands map[uuid.UUID]string
	err   error
}

func (r *fakeRatings) FinalBands(context.Context, uuid.UUID, uuid.UUID) (map[uuid.UUID]string, error) {
	return r.bands, r.err
}

type fakePayroll struct {
	calls  int
	synced bool
	err    error
}

func (p *fakePayroll) SetCTC(context.Context, *gorm.DB, uuid.UUID, uuid.UUID, kernel.Money, time.Time, uuid.UUID) (bool, error) {
	p.calls++
	return p.synced, p.err
}

type noAudit struct{}

func (noAudit) Log(context.Context, *gorm.DB, string, string, string, string, string, interface{}, string, string) error {
	return nil
}

var _ identityport.Auditor = noAudit{}

// --- environment ---

type env struct {
	svc                     *Service
	store                   *fakeStore
	emps                    *fakeEmployees
	core                    *fakeCore
	ratings                 *fakeRatings
	payroll                 *fakePayroll
	tenant, grade, dept     uuid.UUID
	hr, approver, mgrUser   Actor
	mgr, alice, bob, nobody uuid.UUID
}

var today = time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)

func setup(t *testing.T) *env {
	t.Helper()
	e := &env{store: newStore(), tenant: uuid.New(), grade: uuid.New(), dept: uuid.New(), mgr: uuid.New(), alice: uuid.New(), bob: uuid.New(), nobody: uuid.New()}
	mgrUser := uuid.New()
	e.emps = &fakeEmployees{byID: map[uuid.UUID]employeeDTO.EmployeeResponse{
		e.mgr: {ID: e.mgr, UserID: mgrUser}, e.alice: {ID: e.alice, UserID: uuid.New()}, e.bob: {ID: e.bob, UserID: uuid.New()},
		e.nobody: {ID: e.nobody, UserID: uuid.New()}}}
	g, d, m := e.grade, e.dept, e.mgr
	e.core = &fakeCore{grades: map[uuid.UUID]bool{e.grade: true}, place: map[uuid.UUID]*coreAPI.Placement{
		e.alice: {GradeID: &g, DepartmentID: &d, ManagerEmployeeID: &m}, e.bob: {GradeID: &g, DepartmentID: &d, ManagerEmployeeID: &m},
		e.mgr: {DepartmentID: &d}}}
	e.ratings = &fakeRatings{bands: map[uuid.UUID]string{e.alice: "high"}}
	e.payroll = &fakePayroll{synced: true}
	e.svc = NewService(e.store, Ports{Employees: e.emps, Core: e.core, Ratings: e.ratings, Payroll: e.payroll, Audit: noAudit{}},
		func() time.Time { return today })
	e.hr = Actor{TenantID: e.tenant, UserID: uuid.New(), CanRead: true, CanManage: true}
	e.approver = Actor{TenantID: e.tenant, UserID: uuid.New(), CanRead: true}
	e.mgrUser = Actor{TenantID: e.tenant, UserID: mgrUser}
	return e
}

func ctx() context.Context { return context.Background() }

func code(t *testing.T, err error, want string) {
	t.Helper()
	var app *sharedErrors.AppError
	if !errors.As(err, &app) || app.Code != want {
		t.Fatalf("want %s, got %v", want, err)
	}
}

func ok(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func str(s string) *string { return &s }

func (e *env) rangeIn(from string) RangeInput {
	return RangeInput{GradeID: e.grade.String(), MinAmount: kernel.Rupees(800000), MidAmount: kernel.Rupees(1000000), MaxAmount: kernel.Rupees(1200000),
		EffectiveFrom: from}
}

// --- pay ranges and position (CO-C01, CO-C02) ---

func TestPayRanges(t *testing.T) {
	e := setup(t)
	bad := []RangeInput{{GradeID: "x"}, {GradeID: e.grade.String(), EffectiveFrom: "nope"},
		{GradeID: e.grade.String(), EffectiveFrom: "2026-01-01", MinAmount: 5, MidAmount: 4, MaxAmount: 6}}
	for _, in := range bad {
		code(t, mustErr(e.svc.CreateRange(ctx(), e.hr, in)), "VALIDATION_ERROR")
	}
	in := e.rangeIn("2026-01-01")
	in.GradeID = uuid.NewString()
	code(t, mustErr(e.svc.CreateRange(ctx(), e.hr, in)), "NOT_FOUND")
	inactive := uuid.New()
	e.core.grades[inactive] = false
	in.GradeID = inactive.String()
	code(t, mustErr(e.svc.CreateRange(ctx(), e.hr, in)), "INACTIVE_REFERENCE")
	e.core.err = errBoom
	if _, err := e.svc.CreateRange(ctx(), e.hr, e.rangeIn("2026-01-01")); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	e.core.err = nil

	first, err := e.svc.CreateRange(ctx(), e.hr, e.rangeIn("2026-01-01"))
	ok(t, err)
	code(t, mustErr(e.svc.CreateRange(ctx(), e.hr, e.rangeIn("2026-01-01"))), "BACKDATED")
	_, err = e.svc.CreateRange(ctx(), e.hr, e.rangeIn("2027-01-01"))
	ok(t, err)
	if r := e.store.ranges[first.ID]; r.EffectiveTo == nil || r.EffectiveTo.Format(DateLayout) != "2027-01-01" {
		t.Fatal("previous range not closed")
	}
	rows, err := e.svc.Ranges(ctx(), e.hr, &e.grade)
	if err != nil || len(rows) != 2 {
		t.Fatal(rows, err)
	}
	for _, k := range []string{"lock", "ranges", "save", "create"} {
		e.store.fail[k] = errBoom
		if _, err := e.svc.CreateRange(ctx(), e.hr, e.rangeIn("2028-01-01")); !errors.Is(err, errBoom) {
			t.Fatal(k, err)
		}
		delete(e.store.fail, k)
	}
}

func mustErr(_ interface{}, err error) error { return err }

func TestPositionAndBands(t *testing.T) {
	e := setup(t)
	_, err := e.svc.CreateRange(ctx(), e.hr, e.rangeIn("2026-01-01"))
	ok(t, err)
	p, err := e.svc.position(ctx(), nil, e.tenant, e.alice, kernel.Rupees(1000000), today)
	ok(t, err)
	if p.CompaRatio == nil || *p.CompaRatio != 100 || *p.Penetration != 5000 || p.CompaBand != "within" {
		t.Fatalf("%+v", p)
	}
	p, _ = e.svc.position(ctx(), nil, e.tenant, e.alice, kernel.Rupees(850000), today)
	if *p.CompaRatio != 85 || p.CompaBand != "below" {
		t.Fatalf("%+v", p)
	}
	p, _ = e.svc.position(ctx(), nil, e.tenant, e.alice, kernel.Rupees(1150000), today)
	if p.CompaBand != "above" {
		t.Fatal(p.CompaBand)
	}
	if p, _ = e.svc.position(ctx(), nil, e.tenant, e.mgr, kernel.Rupees(1), today); p.CompaBand != "unknown" || p.GradeID != nil {
		t.Fatal("no grade must be unknown")
	}
	if p, _ = e.svc.position(ctx(), nil, e.tenant, e.alice, kernel.Rupees(1), time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)); p.CompaBand != "unknown" {
		t.Fatal("no range in effect must be unknown")
	}
	e.store.fail["ranges"] = errBoom
	if _, err := e.svc.position(ctx(), nil, e.tenant, e.alice, 1, today); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
}

// --- off-cycle changes (CO-C10, CO-C11, CO-C13) ---

func (e *env) change(base int64, from string) ChangeInput {
	return ChangeInput{EmployeeID: e.alice.String(), AnnualBase: kernel.Rupees(base), EffectiveFrom: from, Reason: "market", Comment: str("  note ")}
}

func TestChangesLifecycle(t *testing.T) {
	e := setup(t)
	for _, in := range []ChangeInput{{EmployeeID: "x"}, {EmployeeID: e.alice.String()}, {EmployeeID: e.alice.String(), AnnualBase: 1, EffectiveFrom: "x"},
		{EmployeeID: e.alice.String(), AnnualBase: 1, EffectiveFrom: "2026-01-01", Reason: "bonus"},
		{EmployeeID: e.alice.String(), AnnualBase: 1, EffectiveFrom: "2026-01-01", Reason: "hire", Comment: str(strings.Repeat("x", 1001))}} {
		code(t, mustErr(e.svc.CreateChange(ctx(), e.hr, in)), "VALIDATION_ERROR")
	}
	missing := e.change(1, "2026-01-01")
	missing.EmployeeID = uuid.NewString()
	code(t, mustErr(e.svc.CreateChange(ctx(), e.hr, missing)), "EMPLOYEE_NOT_FOUND")

	// First record applies at once.
	first, err := e.svc.CreateChange(ctx(), e.hr, e.change(1000000, "2026-01-01"))
	ok(t, err)
	if first.Status != ChangeApproved || len(e.store.records) != 1 || e.payroll.calls != 1 || e.store.events[0] != "hr.compensation.changed" {
		t.Fatalf("first change: %+v", first)
	}
	code(t, mustErr(e.svc.CreateChange(ctx(), e.hr, e.change(1000000, "2026-03-01"))), "NO_CHANGE")
	code(t, mustErr(e.svc.CreateChange(ctx(), e.hr, e.change(1050000, "2026-01-01"))), "BACKDATED")
	// A raise up to 10% applies at once and closes the open record.
	_, err = e.svc.CreateChange(ctx(), e.hr, e.change(1100000, "2026-03-01"))
	ok(t, err)
	if len(e.store.records) != 2 {
		t.Fatal("raise not applied")
	}
	// More than 10% (or a cut) waits for approval.
	big, err := e.svc.CreateChange(ctx(), e.hr, e.change(1500000, "2026-05-01"))
	ok(t, err)
	if big.Status != ChangePending || *big.Comment != "note" {
		t.Fatalf("%+v", big)
	}
	code(t, mustErr(e.svc.CreateChange(ctx(), e.hr, e.change(900000, "2026-05-01"))), "CONFLICT")
	code(t, mustErr(e.svc.DecideChange(ctx(), e.hr, big.ID, ChangeApproved, nil)), "COMP_SELF_APPROVAL")
	code(t, mustErr(e.svc.DecideChange(ctx(), e.approver, big.ID, ChangeCancelled, nil)), "NOT_FOUND")
	code(t, mustErr(e.svc.DecideChange(ctx(), e.approver, uuid.New(), ChangeApproved, nil)), "NOT_FOUND")
	code(t, mustErr(e.svc.DecideChange(ctx(), e.approver, big.ID, ChangeApproved, str(strings.Repeat("x", 1001)))), "VALIDATION_ERROR")
	done, err := e.svc.DecideChange(ctx(), e.approver, big.ID, ChangeApproved, str("ok"))
	ok(t, err)
	if done.Status != ChangeApproved || *done.DecidedByUserID != e.approver.UserID || len(e.store.records) != 3 {
		t.Fatalf("%+v", done)
	}
	code(t, mustErr(e.svc.DecideChange(ctx(), e.approver, big.ID, ChangeRejected, nil)), "CHANGE_STATE")

	// Worker view: current record, position, history and pending changes.
	_, err = e.svc.CreateRange(ctx(), e.hr, e.rangeIn("2026-01-01"))
	ok(t, err)
	v, err := e.svc.Worker(ctx(), e.hr, e.alice)
	ok(t, err)
	if v.Current == nil || v.Current.AnnualBase != kernel.Rupees(1500000) || len(v.History) != 3 || v.Position.CompaBand != "above" {
		t.Fatalf("%+v", v)
	}
	code(t, mustErr(e.svc.Worker(ctx(), e.hr, uuid.New())), "EMPLOYEE_NOT_FOUND")
	code(t, mustErr(e.svc.Me(ctx(), Actor{TenantID: e.tenant, UserID: uuid.New()})), "EMPLOYEE_NOT_FOUND")
	me, err := e.svc.Me(ctx(), e.mgrUser)
	if err != nil || me.Current != nil {
		t.Fatal(me, err)
	}

	// Reject and cancel.
	cut, err := e.svc.CreateChange(ctx(), e.hr, e.change(1200000, "2026-07-01"))
	ok(t, err)
	if _, err := e.svc.DecideChange(ctx(), e.approver, cut.ID, ChangeRejected, nil); err != nil {
		t.Fatal(err)
	}
	again, err := e.svc.CreateChange(ctx(), e.hr, e.change(1200000, "2026-07-01"))
	ok(t, err)
	c, err := e.svc.DecideChange(ctx(), e.hr, again.ID, ChangeCancelled, nil)
	if err != nil || c.Status != ChangeCancelled || c.DecidedByUserID != nil {
		t.Fatal(c, err)
	}

	got, err := e.svc.GetChange(ctx(), e.hr, again.ID)
	if err != nil || got.ID != again.ID {
		t.Fatal(err)
	}
	code(t, mustErr(e.svc.GetChange(ctx(), e.hr, uuid.New())), "NOT_FOUND")
	rows, total, err := e.svc.Changes(ctx(), e.hr, ChangeFilter{EmployeeID: &e.alice, Status: ChangeApproved}, 1, 20)
	if err != nil || total != 3 || len(rows) != 3 {
		t.Fatal(total, err)
	}
	if _, _, err := e.svc.Changes(ctx(), e.hr, ChangeFilter{Status: "weird"}, 1, 20); err == nil {
		t.Fatal("status filter")
	}
}

func TestChangeFailures(t *testing.T) {
	e := setup(t)
	e.payroll.err = errBoom
	if _, err := e.svc.CreateChange(ctx(), e.hr, e.change(1000000, "2026-01-01")); !errors.Is(err, errBoom) || len(e.store.changes) != 0 {
		t.Fatal("payroll failure must roll back", err)
	}
	e.payroll.err = nil
	for _, k := range []string{"lock", "changes", "records", "create", "event"} {
		e.store.fail[k] = errBoom
		if _, err := e.svc.CreateChange(ctx(), e.hr, e.change(1000000, "2026-01-01")); !errors.Is(err, errBoom) {
			t.Fatal(k, err)
		}
		delete(e.store.fail, k)
	}
	_, err := e.svc.CreateChange(ctx(), e.hr, e.change(1000000, "2026-01-01"))
	ok(t, err)
	big, err := e.svc.CreateChange(ctx(), e.hr, e.change(2000000, "2026-02-01"))
	ok(t, err)
	for _, k := range []string{"change", "save"} {
		e.store.fail[k] = errBoom
		if _, err := e.svc.DecideChange(ctx(), e.approver, big.ID, ChangeApproved, nil); !errors.Is(err, errBoom) {
			t.Fatal(k, err)
		}
		delete(e.store.fail, k)
	}
	e.store.fail["change"] = errBoom
	if _, err := e.svc.GetChange(ctx(), e.hr, big.ID); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	delete(e.store.fail, "change")
	for _, k := range []string{"records", "changes"} {
		e.store.fail[k] = errBoom
		if _, err := e.svc.Worker(ctx(), e.hr, e.alice); !errors.Is(err, errBoom) {
			t.Fatal(k, err)
		}
		delete(e.store.fail, k)
	}
	e.store.fail["ranges"] = errBoom
	if _, err := e.svc.Worker(ctx(), e.hr, e.alice); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	delete(e.store.fail, "ranges")
	e.emps.err = errBoom
	if _, err := e.svc.Me(ctx(), e.mgrUser); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
}

// --- merit cycles (CO-C20..CO-C26) ---

func (e *env) seedRecords(t *testing.T) {
	t.Helper()
	_, err := e.svc.CreateRange(ctx(), e.hr, e.rangeIn("2025-01-01"))
	ok(t, err)
	for _, emp := range []uuid.UUID{e.alice, e.bob, e.mgr} {
		in := e.change(1000000, "2025-04-01")
		in.EmployeeID = emp.String()
		_, err := e.svc.CreateChange(ctx(), e.hr, in)
		ok(t, err)
	}
}

func TestMeritCycle(t *testing.T) {
	e := setup(t)
	e.seedRecords(t)
	for _, in := range []CycleInput{{}, {Name: str(""), EffectiveDate: str("2026-07-01")}, {Name: str("x"), EffectiveDate: str("bad")},
		{Name: str("x"), EffectiveDate: str("2026-07-01"), PerformanceCycleID: str("bad")},
		{Name: str("x"), EffectiveDate: str("2026-07-01"), Guidelines: &Guidelines{{Rating: "great", Compa: "within"}}},
		{Name: str("x"), EffectiveDate: str("2026-07-01"), Guidelines: &Guidelines{{Rating: "high", Compa: "within", MinPct: 500, MaxPct: 100}}},
		{Name: str("x"), EffectiveDate: str("2026-07-01"), Guidelines: &Guidelines{{Rating: "high", Compa: "within"}, {Rating: "high", Compa: "within"}}}} {
		code(t, mustErr(e.svc.CreateCycle(ctx(), e.hr, in)), "VALIDATION_ERROR")
	}
	perf := uuid.NewString()
	c, err := e.svc.CreateCycle(ctx(), e.hr, CycleInput{Name: str(" FY26 merit "), EffectiveDate: str("2026-07-01"), PerformanceCycleID: &perf,
		Guidelines: &Guidelines{{Rating: "high", Compa: "within", MinPct: 500, MaxPct: 1000}, {Rating: "unrated", Compa: "within", MinPct: 0, MaxPct: 300}}})
	ok(t, err)
	if c.Name != "FY26 merit" || c.Status != CycleDraft {
		t.Fatalf("%+v", c)
	}
	_, err = e.svc.UpdateCycle(ctx(), e.hr, c.ID, CycleInput{PerformanceCycleID: str("")})
	ok(t, err)
	_, err = e.svc.UpdateCycle(ctx(), e.hr, c.ID, CycleInput{PerformanceCycleID: &perf})
	ok(t, err)
	code(t, mustErr(e.svc.UpdateCycle(ctx(), e.hr, uuid.New(), CycleInput{})), "NOT_FOUND")
	code(t, mustErr(e.svc.SetBudgets(ctx(), e.hr, c.ID, []BudgetInput{{DepartmentID: "x"}})), "VALIDATION_ERROR")
	code(t, mustErr(e.svc.SetBudgets(ctx(), e.hr, c.ID, []BudgetInput{{DepartmentID: e.dept.String()}, {DepartmentID: e.dept.String()}})), "VALIDATION_ERROR")
	bs, err := e.svc.SetBudgets(ctx(), e.hr, c.ID, []BudgetInput{{DepartmentID: e.dept.String(), Amount: kernel.Rupees(120000)}})
	if err != nil || len(bs) != 1 {
		t.Fatal(bs, err)
	}
	code(t, mustErr(e.svc.Budgets(ctx(), e.hr, uuid.New())), "NOT_FOUND")
	code(t, mustErr(e.svc.Move(ctx(), e.hr, c.ID, CycleApproved)), "CYCLE_STATE")

	// Open snapshots proposals for every worker with a record and a department.
	e.emps.byID[uuid.New()] = employeeDTO.EmployeeResponse{} // no record: skipped
	opened, err := e.svc.Move(ctx(), e.hr, c.ID, CycleOpen)
	ok(t, err)
	if opened.Status != CycleOpen || len(e.store.proposals) != 3 {
		t.Fatalf("status %s proposals %d", opened.Status, len(e.store.proposals))
	}
	code(t, mustErr(e.svc.UpdateCycle(ctx(), e.hr, c.ID, CycleInput{})), "CYCLE_STATE")
	code(t, mustErr(e.svc.SetBudgets(ctx(), e.hr, c.ID, nil)), "CYCLE_STATE")
	var alice, bob Proposal
	for _, p := range e.store.proposals {
		switch p.EmployeeProfileID {
		case e.alice:
			alice = p
		case e.bob:
			bob = p
		}
	}
	if alice.RatingBand != "high" || bob.RatingBand != "unrated" || alice.CompaBand != "within" || *alice.ManagerEmployeeID != e.mgr {
		t.Fatalf("%+v %+v", alice, bob)
	}

	// Manager views and edits; others cannot.
	mine, err := e.svc.Proposals(ctx(), e.mgrUser, c.ID, true)
	if err != nil || len(mine) != 2 {
		t.Fatal(len(mine), err)
	}
	if all, _ := e.svc.Proposals(ctx(), e.hr, c.ID, false); len(all) != 3 {
		t.Fatal("hr sees all")
	}
	if none, _ := e.svc.Proposals(ctx(), Actor{TenantID: e.tenant, UserID: uuid.New()}, c.ID, false); len(none) != 0 {
		t.Fatal("stranger sees nothing")
	}
	code(t, mustErr(e.svc.Proposals(ctx(), e.hr, uuid.New(), false)), "NOT_FOUND")
	stranger := Actor{TenantID: e.tenant, UserID: e.emps.byID[e.bob].UserID}
	code(t, mustErr(e.svc.UpdateProposal(ctx(), stranger, alice.ID, ProposalInput{IncreasePct: 500})), "NOT_FOUND")
	code(t, mustErr(e.svc.UpdateProposal(ctx(), e.mgrUser, alice.ID, ProposalInput{IncreasePct: 6000})), "VALIDATION_ERROR")
	code(t, mustErr(e.svc.UpdateProposal(ctx(), e.mgrUser, alice.ID, ProposalInput{IncreasePct: 100, Justification: str(strings.Repeat("x", 1001))})), "VALIDATION_ERROR")
	code(t, mustErr(e.svc.UpdateProposal(ctx(), e.mgrUser, alice.ID, ProposalInput{IncreasePct: 1500})), "VALIDATION_ERROR") // outside, no justification
	p, err := e.svc.UpdateProposal(ctx(), e.mgrUser, alice.ID, ProposalInput{IncreasePct: 800})
	ok(t, err)
	if p.Amount != kernel.Rupees(80000) || p.OutsideGuideline {
		t.Fatalf("%+v", p)
	}
	code(t, mustErr(e.svc.UpdateProposal(ctx(), e.mgrUser, bob.ID, ProposalInput{IncreasePct: 500, Justification: str("strong")})), "BUDGET_EXCEEDED")
	code(t, mustErr(e.svc.UpdateProposal(ctx(), e.mgrUser, bob.ID, ProposalInput{IncreasePct: 500, OverrideBudget: true})), "VALIDATION_ERROR")
	p, err = e.svc.UpdateProposal(ctx(), e.mgrUser, bob.ID, ProposalInput{IncreasePct: 500, OverrideBudget: true, Justification: str("retention")})
	if err != nil || !p.OutsideGuideline || !p.OverrideBudget {
		t.Fatal(p, err)
	}
	usage, err := e.svc.Usage(ctx(), e.hr, c.ID)
	if err != nil || len(usage) != 1 || usage[0].Used != kernel.Rupees(130000) || usage[0].Remaining != kernel.Rupees(-10000) || usage[0].Proposals != 3 {
		t.Fatal(usage, err)
	}
	code(t, mustErr(e.svc.Usage(ctx(), e.hr, uuid.New())), "NOT_FOUND")

	// Close, approve (never the opener), apply once.
	_, err = e.svc.Move(ctx(), e.hr, c.ID, CycleInReview)
	ok(t, err)
	code(t, mustErr(e.svc.UpdateProposal(ctx(), e.mgrUser, alice.ID, ProposalInput{IncreasePct: 800})), "CYCLE_STATE")
	code(t, mustErr(e.svc.Move(ctx(), e.hr, c.ID, CycleApproved)), "COMP_SELF_APPROVAL")
	_, err = e.svc.Move(ctx(), e.approver, c.ID, CycleApproved)
	ok(t, err)
	if after, _ := e.svc.Proposals(ctx(), e.mgrUser, c.ID, true); len(after) != 0 {
		t.Fatal("managers see proposals only while open or in review")
	}
	before := len(e.store.records)
	applied, err := e.svc.Move(ctx(), e.hr, c.ID, CycleApplied)
	ok(t, err)
	if applied.Status != CycleApplied || len(e.store.records) != before+2 || e.store.events[len(e.store.events)-1] != "hr.compensation.merit_applied" {
		t.Fatalf("records %d events %v", len(e.store.records), e.store.events)
	}
	v, _ := e.svc.Worker(ctx(), e.hr, e.alice)
	_ = v
	code(t, mustErr(e.svc.Move(ctx(), e.hr, c.ID, CycleApplied)), "CYCLE_STATE")

	got, err := e.svc.GetCycle(ctx(), e.hr, c.ID)
	if err != nil || got.ID != c.ID {
		t.Fatal(err)
	}
	rows, total, err := e.svc.Cycles(ctx(), e.hr, CycleApplied, 1, 20)
	if err != nil || total != 1 || len(rows) != 1 {
		t.Fatal(total, err)
	}
	if _, _, err := e.svc.Cycles(ctx(), e.hr, "weird", 1, 20); err == nil {
		t.Fatal("status filter")
	}
}

func TestMeritFailures(t *testing.T) {
	e := setup(t)
	e.seedRecords(t)
	perf := uuid.NewString()
	c, err := e.svc.CreateCycle(ctx(), e.hr, CycleInput{Name: str("m"), EffectiveDate: str("2026-07-01"), PerformanceCycleID: &perf})
	ok(t, err)
	_, err = e.svc.SetBudgets(ctx(), e.hr, c.ID, []BudgetInput{{DepartmentID: e.dept.String(), Amount: kernel.Rupees(500000)}})
	ok(t, err)
	e.ratings.err = errBoom
	if _, err := e.svc.Move(ctx(), e.hr, c.ID, CycleOpen); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	e.ratings.err = nil
	e.emps.err = errBoom
	if _, err := e.svc.Move(ctx(), e.hr, c.ID, CycleOpen); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	e.emps.err = nil
	for _, k := range []string{"records", "ranges", "create", "cycle", "save"} {
		e.store.fail[k] = errBoom
		if _, err := e.svc.Move(ctx(), e.hr, c.ID, CycleOpen); !errors.Is(err, errBoom) {
			t.Fatal(k, err)
		}
		delete(e.store.fail, k)
	}
	e.core.err = errBoom
	if _, err := e.svc.Move(ctx(), e.hr, c.ID, CycleOpen); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	e.core.err = nil
	if _, err := e.svc.Move(ctx(), e.hr, c.ID, CycleOpen); err != nil {
		t.Fatal(err)
	}
	var pid uuid.UUID
	for id, p := range e.store.proposals {
		if p.EmployeeProfileID == e.alice {
			pid = id
		}
	}
	for _, k := range []string{"proposal", "budgets", "proposals"} {
		e.store.fail[k] = errBoom
		if _, err := e.svc.UpdateProposal(ctx(), e.hr, pid, ProposalInput{IncreasePct: 100}); !errors.Is(err, errBoom) {
			t.Fatal(k, err)
		}
		delete(e.store.fail, k)
	}
	e.emps.err = errBoom
	if _, err := e.svc.UpdateProposal(ctx(), e.mgrUser, pid, ProposalInput{IncreasePct: 100}); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	if _, err := e.svc.Proposals(ctx(), e.mgrUser, c.ID, true); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	e.emps.err = nil
	e.store.fail["budgets"] = errBoom
	if _, err := e.svc.Usage(ctx(), e.hr, c.ID); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	delete(e.store.fail, "budgets")
	_, err = e.svc.UpdateProposal(ctx(), e.hr, pid, ProposalInput{IncreasePct: 100})
	ok(t, err)
	_, _ = e.svc.Move(ctx(), e.hr, c.ID, CycleInReview)
	_, _ = e.svc.Move(ctx(), e.approver, c.ID, CycleApproved)
	e.payroll.err = errBoom
	if _, err := e.svc.Move(ctx(), e.hr, c.ID, CycleApplied); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	e.payroll.err = nil
	for _, k := range []string{"proposals", "event"} {
		e.store.fail[k] = errBoom
		if _, err := e.svc.Move(ctx(), e.hr, c.ID, CycleApplied); !errors.Is(err, errBoom) {
			t.Fatal(k, err)
		}
		delete(e.store.fail, k)
	}
	if got, _ := e.svc.GetCycle(ctx(), e.hr, c.ID); got.Status != CycleApproved {
		t.Fatal("failed apply must roll back", got.Status)
	}
	e.store.fail["cycle"] = errBoom
	if _, err := e.svc.GetCycle(ctx(), e.hr, c.ID); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	e.store.fail["create"] = errBoom
	if _, err := e.svc.CreateCycle(ctx(), e.hr, CycleInput{Name: str("m"), EffectiveDate: str("2026-07-01")}); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
}

func TestModelsAndHelpers(t *testing.T) {
	var g Guidelines
	if v, _ := g.Value(); v != "[]" {
		t.Fatal(v)
	}
	g = Guidelines{{Rating: "high", Compa: "within", MinPct: 100, MaxPct: 200}}
	v, err := g.Value()
	ok(t, err)
	var back Guidelines
	if back.Scan(v) != nil || back.Scan([]byte(v.(string))) != nil || len(back) != 1 || back.Scan(nil) != nil || back != nil || back.Scan(1) == nil {
		t.Fatal("guidelines scan")
	}
	var d Date
	if d.Scan(time.Date(2026, 1, 2, 5, 0, 0, 0, time.UTC)) != nil || d.Scan("2026-02-03T00:00:00Z") != nil || d.Scan(1) == nil {
		t.Fatal("date scan")
	}
	if b, _ := d.MarshalJSON(); string(b) != `"2026-02-03"` {
		t.Fatal(string(b))
	}
	if v, _ := d.Value(); v != "2026-02-03" {
		t.Fatal(v)
	}
	for _, m := range []interface{ TableName() string }{PayRange{}, Record{}, Change{}, Cycle{}, Budget{}, Proposal{}} {
		if !strings.HasPrefix(m.TableName(), "comp_") {
			t.Fatal(m.TableName())
		}
	}
	for _, m := range []interface{ BeforeCreate(*gorm.DB) error }{&PayRange{}, &Record{}, &Change{}, &Cycle{}, &Proposal{}} {
		ok(t, m.BeforeCreate(nil))
	}
	if outside(Guidelines{}, &Proposal{IncreasePct: 4000}) {
		t.Fatal("no cell means no limit")
	}
	if len(Catalogue()) != 3 || len((&Module{}).RegisterModels()) != 6 || NewModule(nil, Ports{}).svc == nil || NewStore(nil).DB() != nil {
		t.Fatal("module")
	}
	if orNotFound(errBoom) != errBoom {
		t.Fatal("orNotFound")
	}
}

// --- HTTP edge ---

type guards struct{ deny, noFlag map[string]bool }

func (g guards) TenantResolver() gin.HandlerFunc { return func(c *gin.Context) { c.Next() } }
func (g guards) Authenticate() gin.HandlerFunc   { return func(c *gin.Context) { c.Next() } }
func (g guards) Require(_, action string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if g.deny[action] {
			c.AbortWithStatusJSON(403, gin.H{"error": gin.H{"code": "FORBIDDEN"}})
			return
		}
		c.Next()
	}
}
func (g guards) RequireScoped(r, a string, _ access.Scope) gin.HandlerFunc { return g.Require(r, a) }
func (g guards) Flag(_, _, key string) gin.HandlerFunc {
	return func(c *gin.Context) { c.Set(key, !g.noFlag[key]); c.Next() }
}
func (g guards) AuditTrail(string, string) gin.HandlerFunc { return func(c *gin.Context) { c.Next() } }

var _ identityport.HTTP = guards{}

func router(e *env, user uuid.UUID) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		if user != uuid.Nil {
			c.Set("tenant_id", e.tenant)
			c.Set("user_id", user)
		}
	})
	(&Module{svc: e.svc}).RegisterRoutes(r.Group(""), guards{deny: map[string]bool{}, noFlag: map[string]bool{}})
	return r
}

func do(r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	return w
}

func TestHTTP(t *testing.T) {
	e := setup(t)
	hr, appr := router(e, e.hr.UserID), router(e, e.approver.UserID)
	const base = "/compensation"
	expect := func(w *httptest.ResponseRecorder, status int, label string) string {
		t.Helper()
		if w.Code != status {
			t.Fatalf("%s: %d %s", label, w.Code, w.Body.String())
		}
		return w.Body.String()
	}
	idOf := func(body string) string {
		i := strings.Index(body, `"id":"`)
		return body[i+6 : i+42]
	}
	expect(do(router(e, uuid.Nil), "GET", base+"/me", ""), 401, "unauthenticated")
	expect(do(hr, "POST", base+"/pay-ranges", "{"), 400, "bad json")
	expect(do(hr, "POST", base+"/pay-ranges", `{"grade_id":"`+e.grade.String()+`","min_amount":800000,"mid_amount":1000000,"max_amount":1200000,"effective_from":"2026-01-01"}`), 201, "range")
	expect(do(hr, "GET", base+"/pay-ranges?grade_id="+e.grade.String(), ""), 200, "ranges")
	expect(do(hr, "GET", base+"/pay-ranges?grade_id=x", ""), 400, "bad grade")

	body := expect(do(hr, "POST", base+"/changes", `{"employee_id":"`+e.alice.String()+`","annual_base":1000000,"effective_from":"2026-01-01","reason":"hire"}`), 201, "first change")
	expect(do(hr, "GET", base+"/changes/"+idOf(body), ""), 200, "get change")
	body = expect(do(hr, "POST", base+"/changes", `{"employee_id":"`+e.alice.String()+`","annual_base":1500000,"effective_from":"2026-03-01","reason":"promotion"}`), 201, "big change")
	big := idOf(body)
	expect(do(hr, "POST", base+"/changes/"+big+"/approve", ""), 403, "self approval")
	expect(do(appr, "POST", base+"/changes/"+big+"/approve", "{"), 400, "bad decide body")
	expect(do(appr, "POST", base+"/changes/"+big+"/approve", `{"comment":"fine"}`), 200, "approve")
	expect(do(appr, "POST", base+"/changes/"+big+"/reject", ""), 409, "reject decided")
	expect(do(appr, "POST", base+"/changes/"+big+"/cancel", ""), 409, "cancel decided")
	expect(do(hr, "GET", base+"/changes?employee_id="+e.alice.String()+"&status=approved", ""), 200, "list changes")
	expect(do(hr, "GET", base+"/changes?employee_id=x", ""), 400, "bad employee filter")
	expect(do(hr, "GET", base+"/changes?status=weird", ""), 400, "bad status filter")
	expect(do(hr, "GET", base+"/changes/x", ""), 400, "bad id")
	expect(do(hr, "GET", base+"/workers/"+e.alice.String(), ""), 200, "worker")
	expect(do(router(e, e.emps.byID[e.mgr].UserID), "GET", base+"/me", ""), 200, "me")

	body = expect(do(hr, "POST", base+"/merit-cycles", `{"name":"FY","effective_date":"2026-07-01","guidelines":[]}`), 201, "cycle")
	cid := idOf(body)
	expect(do(hr, "POST", base+"/merit-cycles", "{"), 400, "bad cycle json")
	expect(do(hr, "PATCH", base+"/merit-cycles/"+cid, `{"name":"FY26"}`), 200, "update cycle")
	expect(do(hr, "PATCH", base+"/merit-cycles/"+cid, "{"), 400, "bad patch")
	expect(do(hr, "PUT", base+"/merit-cycles/"+cid+"/budgets", `[{"department_id":"`+e.dept.String()+`","amount":500000}]`), 200, "budgets")
	expect(do(hr, "PUT", base+"/merit-cycles/"+cid+"/budgets", "{"), 400, "bad budgets")
	expect(do(hr, "GET", base+"/merit-cycles/"+cid+"/budgets", ""), 200, "get budgets")
	expect(do(hr, "GET", base+"/merit-cycles?status=draft", ""), 200, "list cycles")
	expect(do(hr, "GET", base+"/merit-cycles?status=weird", ""), 400, "bad cycle status")
	expect(do(hr, "GET", base+"/merit-cycles/"+cid, ""), 200, "get cycle")
	expect(do(hr, "POST", base+"/merit-cycles/"+cid+"/open", ""), 200, "open")
	body = expect(do(hr, "GET", base+"/merit-cycles/"+cid+"/proposals", ""), 200, "proposals")
	pid := idOf(body)
	expect(do(router(e, e.emps.byID[e.mgr].UserID), "GET", base+"/merit-cycles/"+cid+"/proposals/mine", ""), 200, "mine")
	expect(do(hr, "PATCH", base+"/proposals/"+pid, `{"increase_pct":5}`), 200, "proposal")
	expect(do(hr, "PATCH", base+"/proposals/"+pid, "{"), 400, "bad proposal")
	expect(do(hr, "GET", base+"/merit-cycles/"+cid+"/usage", ""), 200, "usage")
	expect(do(hr, "POST", base+"/merit-cycles/"+cid+"/close", ""), 200, "close")
	expect(do(hr, "POST", base+"/merit-cycles/"+cid+"/approve", ""), 403, "opener approves")
	expect(do(appr, "POST", base+"/merit-cycles/"+cid+"/approve", ""), 200, "approve")
	expect(do(hr, "POST", base+"/merit-cycles/"+cid+"/apply", ""), 200, "apply")
	expect(do(hr, "POST", base+"/merit-cycles/"+cid+"/apply", ""), 409, "apply twice")

	e.store.fail["cycles"] = errBoom
	expect(do(hr, "GET", base+"/merit-cycles", ""), 500, "internal error")
}

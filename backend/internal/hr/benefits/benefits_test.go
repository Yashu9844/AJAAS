package benefits

import (
	"context"
	"errors"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	coreAPI "github.com/jaas/jaas/internal/hr/core/api"
	employeeDTO "github.com/jaas/jaas/internal/hr/employee/dto"
	"github.com/jaas/jaas/internal/hr/identityport"
	"github.com/jaas/jaas/internal/hr/kernel"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"gorm.io/gorm"
)

var errBoom = errors.New("boom")

// --- fake store ---

type fakeStore struct {
	settings  *Settings
	plans     map[uuid.UUID]Plan
	versions  []PlanVersion
	windows   map[uuid.UUID]Window
	deps      map[uuid.UUID]Dependant
	events    []LifeEvent
	elections map[uuid.UUID]Election
	outbox    []string
	fail      map[string]error
}

func newStore() *fakeStore {
	return &fakeStore{plans: map[uuid.UUID]Plan{}, windows: map[uuid.UUID]Window{}, deps: map[uuid.UUID]Dependant{}, elections: map[uuid.UUID]Election{}, fail: map[string]error{}}
}

func cp[K comparable, V any](m map[K]V) map[K]V {
	out := make(map[K]V, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func (f *fakeStore) InTx(_ context.Context, fn func(*gorm.DB) error) error {
	pl, wi, de, el, vs, ob, st := cp(f.plans), cp(f.windows), cp(f.deps), cp(f.elections), append([]PlanVersion(nil), f.versions...), len(f.outbox), f.settings
	if err := fn(nil); err != nil {
		f.plans, f.windows, f.deps, f.elections, f.versions, f.outbox, f.settings = pl, wi, de, el, vs, f.outbox[:ob], st
		return err
	}
	return nil
}
func (f *fakeStore) DB() *gorm.DB                                 { return nil }
func (f *fakeStore) Lock(context.Context, *gorm.DB, string) error { return f.fail["lock"] }
func (f *fakeStore) Create(_ context.Context, _ *gorm.DB, row interface{}) error {
	if err := f.fail["create"]; err != nil {
		return err
	}
	switch v := row.(type) {
	case *Plan:
		f.plans[v.ID] = *v
	case *PlanVersion:
		f.versions = append(f.versions, *v)
	case *Window:
		_ = v.BeforeCreate(nil)
		f.windows[v.ID] = *v
	case *Dependant:
		_ = v.BeforeCreate(nil)
		f.deps[v.ID] = *v
	case *LifeEvent:
		_ = v.BeforeCreate(nil)
		f.events = append(f.events, *v)
	case *Election:
		for _, e := range f.elections {
			if e.EmployeeProfileID == v.EmployeeProfileID && e.PlanID == v.PlanID && e.EffectiveTo == nil && v.EffectiveTo == nil {
				return errors.New("duplicate live election") // the partial unique index
			}
		}
		_ = v.BeforeCreate(nil)
		f.elections[v.ID] = *v
	}
	return nil
}
func (f *fakeStore) Save(_ context.Context, _ *gorm.DB, row interface{}) error {
	if err := f.fail["save"]; err != nil {
		return err
	}
	switch v := row.(type) {
	case *Plan:
		f.plans[v.ID] = *v
	case *PlanVersion:
		for i := range f.versions {
			if f.versions[i].PlanID == v.PlanID && f.versions[i].Version == v.Version {
				f.versions[i] = *v
			}
		}
	case *Window:
		f.windows[v.ID] = *v
	case *Dependant:
		f.deps[v.ID] = *v
	case *Election:
		f.elections[v.ID] = *v
	}
	return nil
}
func (f *fakeStore) Settings(context.Context, *gorm.DB, uuid.UUID) (*Settings, error) {
	return f.settings, f.fail["settings"]
}
func (f *fakeStore) SaveSettings(_ context.Context, _ *gorm.DB, row *Settings) error {
	f.settings = row
	return f.fail["savesettings"]
}
func (f *fakeStore) Plan(_ context.Context, _ *gorm.DB, _, id uuid.UUID, _ bool) (*Plan, error) {
	if p, ok := f.plans[id]; ok {
		return &p, f.fail["plan"]
	}
	return nil, f.fail["plan"]
}
func (f *fakeStore) PlanCodeTaken(_ context.Context, _ *gorm.DB, _ uuid.UUID, code string) (bool, error) {
	for _, p := range f.plans {
		if strings.EqualFold(p.Code, code) {
			return true, f.fail["codetaken"]
		}
	}
	return false, f.fail["codetaken"]
}
func page[T any](rows []T, p, pp int) []T {
	lo := (p - 1) * pp
	if lo >= len(rows) {
		return nil
	}
	return rows[lo:min(lo+pp, len(rows))]
}
func (f *fakeStore) Plans(_ context.Context, _ *gorm.DB, _ uuid.UUID, status string, p, pp int) ([]Plan, int64, error) {
	var out []Plan
	for _, x := range f.plans {
		if status == "" || x.Status == status {
			out = append(out, x)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	return page(out, p, pp), int64(len(out)), f.fail["plans"]
}
func (f *fakeStore) ActivePlans(_ context.Context, _ *gorm.DB, _ uuid.UUID) ([]Plan, error) {
	var out []Plan
	for _, x := range f.plans {
		if x.Status == PlanActive {
			out = append(out, x)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	return out, f.fail["activeplans"]
}
func (f *fakeStore) Versions(_ context.Context, _ *gorm.DB, _ uuid.UUID, ids []uuid.UUID) ([]PlanVersion, error) {
	var out []PlanVersion
	for _, v := range f.versions {
		for _, id := range ids {
			if v.PlanID == id {
				out = append(out, v)
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	return out, f.fail["versions"]
}
func (f *fakeStore) Window(_ context.Context, _ *gorm.DB, _, id uuid.UUID, _ bool) (*Window, error) {
	if w, ok := f.windows[id]; ok {
		return &w, f.fail["window"]
	}
	return nil, f.fail["window"]
}
func (f *fakeStore) Windows(context.Context, *gorm.DB, uuid.UUID) ([]Window, error) {
	var out []Window
	for _, w := range f.windows {
		out = append(out, w)
	}
	return out, f.fail["windows"]
}
func (f *fakeStore) OpenWindowOn(_ context.Context, _ *gorm.DB, _ uuid.UUID, on Date) (bool, error) {
	for _, w := range f.windows {
		if w.Status == WindowActive && !on.Before(w.StartsOn.Time) && !on.After(w.EndsOn.Time) {
			return true, f.fail["openwindow"]
		}
	}
	return false, f.fail["openwindow"]
}
func (f *fakeStore) Dependant(_ context.Context, _ *gorm.DB, _, id uuid.UUID, _ bool) (*Dependant, error) {
	if d, ok := f.deps[id]; ok {
		return &d, f.fail["dependant"]
	}
	return nil, f.fail["dependant"]
}
func (f *fakeStore) Dependants(_ context.Context, _ *gorm.DB, _, e uuid.UUID) ([]Dependant, error) {
	var out []Dependant
	for _, d := range f.deps {
		if d.EmployeeProfileID == e && d.Status == "active" {
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, f.fail["dependants"]
}
func (f *fakeStore) LifeEvents(_ context.Context, _ *gorm.DB, _, e uuid.UUID) ([]LifeEvent, error) {
	var out []LifeEvent
	for _, l := range f.events {
		if l.EmployeeProfileID == e {
			out = append(out, l)
		}
	}
	return out, f.fail["lifeevents"]
}
func (f *fakeStore) LifeEventSince(_ context.Context, _ *gorm.DB, _, e uuid.UUID, since, upTo Date) (*LifeEvent, error) {
	for _, l := range f.events {
		if l.EmployeeProfileID == e && !l.EventDate.Before(since.Time) && !l.EventDate.After(upTo.Time) {
			l := l
			return &l, f.fail["lifesince"]
		}
	}
	return nil, f.fail["lifesince"]
}
func (f *fakeStore) Election(_ context.Context, _ *gorm.DB, _, id uuid.UUID, _ bool) (*Election, error) {
	if e, ok := f.elections[id]; ok {
		return &e, f.fail["election"]
	}
	return nil, f.fail["election"]
}
func (f *fakeStore) LiveElection(_ context.Context, _ *gorm.DB, _, e, p uuid.UUID, _ bool) (*Election, error) {
	for _, x := range f.elections {
		if x.EmployeeProfileID == e && x.PlanID == p && x.EffectiveTo == nil {
			x := x
			return &x, f.fail["live"]
		}
	}
	return nil, f.fail["live"]
}
func (f *fakeStore) sorted() []Election {
	var out []Election
	for _, e := range f.elections {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].EffectiveFrom.Equal(out[j].EffectiveFrom.Time) {
			return out[i].EffectiveFrom.After(out[j].EffectiveFrom.Time)
		}
		return out[i].ID.String() < out[j].ID.String()
	})
	return out
}
func (f *fakeStore) Elections(_ context.Context, _ *gorm.DB, _ uuid.UUID, fl ElectionFilter, p, pp int) ([]Election, int64, error) {
	var out []Election
	for _, e := range f.sorted() {
		if (fl.EmployeeID == nil || e.EmployeeProfileID == *fl.EmployeeID) && (fl.PlanID == nil || e.PlanID == *fl.PlanID) && (fl.Status == "" || e.Status == fl.Status) {
			out = append(out, e)
		}
	}
	return page(out, p, pp), int64(len(out)), f.fail["elections"]
}
func (f *fakeStore) EmployeeElections(_ context.Context, _ *gorm.DB, _, e uuid.UUID) ([]Election, error) {
	var out []Election
	for _, x := range f.sorted() {
		if x.EmployeeProfileID == e {
			out = append(out, x)
		}
	}
	return out, f.fail["empelections"]
}
func (f *fakeStore) ElectionsOn(_ context.Context, _ *gorm.DB, _, e uuid.UUID, on Date) ([]Election, error) {
	var out []Election
	for _, x := range f.sorted() {
		if x.EmployeeProfileID == e && x.CoversDay(on.Time) {
			out = append(out, x)
		}
	}
	return out, f.fail["electionson"]
}
func (f *fakeStore) EndForTermination(_ context.Context, _ *gorm.DB, _, e uuid.UUID, end Date, action uuid.UUID) (int, error) {
	if err := f.fail["endterm"]; err != nil {
		return 0, err
	}
	n := 0
	for id, x := range f.elections {
		if x.EmployeeProfileID == e && x.Status == ElActive && x.EffectiveTo == nil {
			to := end
			if end.Before(x.EffectiveFrom.Time) {
				to = x.EffectiveFrom
			}
			reason := ReasonTermination
			x.Status, x.EffectiveTo, x.EndReason, x.EndActionID = ElEnded, &to, &reason, &action
			f.elections[id] = x
			n++
		}
	}
	return n, nil
}
func (f *fakeStore) Reinstate(_ context.Context, _ *gorm.DB, _, action uuid.UUID) (int, error) {
	if err := f.fail["reinstate"]; err != nil {
		return 0, err
	}
	n := 0
	for id, x := range f.elections {
		if x.EndActionID != nil && *x.EndActionID == action && x.EndReason != nil && *x.EndReason == ReasonTermination {
			x.Status, x.EffectiveTo, x.EndReason, x.EndActionID = ElActive, nil, nil, nil
			f.elections[id] = x
			n++
		}
	}
	return n, nil
}
func (f *fakeStore) Counts(_ context.Context, _ *gorm.DB, _ uuid.UUID, on Date) ([]CountRow, error) {
	type key struct {
		p uuid.UUID
		l string
		s string
	}
	agg := map[key]int{}
	for _, e := range f.elections {
		if !e.EffectiveFrom.After(on.Time) && (e.EffectiveTo == nil || on.Before(e.EffectiveTo.Time)) {
			l := ""
			if e.LevelCode != nil {
				l = *e.LevelCode
			}
			agg[key{e.PlanID, l, e.Status}]++
		}
	}
	var out []CountRow
	for k, n := range agg {
		var lc *string
		if k.l != "" {
			l := k.l
			lc = &l
		}
		out = append(out, CountRow{PlanID: k.p, LevelCode: lc, Status: k.s, N: n})
	}
	return out, f.fail["counts"]
}
func (f *fakeStore) Event(_ context.Context, _ *gorm.DB, _ uuid.UUID, key string, _ map[string]string) error {
	if err := f.fail["event"]; err != nil {
		return err
	}
	f.outbox = append(f.outbox, key)
	return nil
}

// --- fake ports ---

type fakeEmployees struct {
	byID   map[uuid.UUID]employeeDTO.EmployeeResponse
	byUser map[uuid.UUID]uuid.UUID
	err    error
}

func (e *fakeEmployees) GetByID(_ context.Context, _, id uuid.UUID) (*employeeDTO.EmployeeResponse, error) {
	if r, ok := e.byID[id]; ok {
		return &r, e.err
	}
	return nil, sharedErrors.ErrNotFound
}
func (e *fakeEmployees) GetByUserID(_ context.Context, _, u uuid.UUID) (*employeeDTO.EmployeeResponse, error) {
	if id, ok := e.byUser[u]; ok {
		return e.GetByID(context.Background(), uuid.Nil, id)
	}
	return nil, sharedErrors.ErrNotFound
}

type fakeCore struct {
	place map[uuid.UUID]*coreAPI.Placement
	err   error
}

func (c *fakeCore) PlacementOn(_ context.Context, _, emp uuid.UUID, _ time.Time) (*coreAPI.Placement, error) {
	return c.place[emp], c.err
}
func (c *fakeCore) GradeState(context.Context, uuid.UUID, uuid.UUID) (bool, bool, error) {
	return true, true, nil
}

type noAudit struct{}

func (noAudit) Log(context.Context, *gorm.DB, string, string, string, string, string, interface{}, string, string) error {
	return nil
}

var _ identityport.Auditor = noAudit{}

// --- environment ---

type person struct {
	emp, user uuid.UUID
	act       Actor
}

type env struct {
	svc    *Service
	st     *fakeStore
	emps   *fakeEmployees
	core   *fakeCore
	tenant uuid.UUID
	clock  time.Time
	hr     person
	ann    person // joined 20 days ago: inside the new-hire window
	bob    person // joined years ago: no window unless one is opened
}

func setup(t *testing.T) *env {
	t.Helper()
	e := &env{st: newStore(), tenant: uuid.New(), clock: time.Date(2026, 6, 10, 10, 0, 0, 0, time.UTC)}
	e.emps = &fakeEmployees{byID: map[uuid.UUID]employeeDTO.EmployeeResponse{}, byUser: map[uuid.UUID]uuid.UUID{}}
	e.core = &fakeCore{place: map[uuid.UUID]*coreAPI.Placement{}}
	e.svc = NewService(e.st, Ports{Employees: e.emps, Core: e.core, Audit: noAudit{}}, func() time.Time { return e.clock })
	e.hr = e.person(time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC))
	e.hr.act.CanRead, e.hr.act.CanManage, e.hr.act.CanSensitiveRead = true, true, true
	e.ann = e.person(time.Date(2026, 5, 21, 0, 0, 0, 0, time.UTC))
	e.bob = e.person(time.Date(2021, 3, 1, 0, 0, 0, 0, time.UTC))
	return e
}

func (e *env) person(joined time.Time) person {
	p := person{emp: uuid.New(), user: uuid.New()}
	p.act = Actor{TenantID: e.tenant, UserID: p.user}
	e.emps.byID[p.emp] = employeeDTO.EmployeeResponse{ID: p.emp, TenantID: e.tenant, UserID: p.user, Status: "active",
		Employment: &employeeDTO.EmploymentDetail{EmploymentType: "full_time", JoiningDate: joined}}
	e.emps.byUser[p.user] = p.emp
	e.core.place[p.emp] = &coreAPI.Placement{WorkerType: "employee"}
	return p
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

func mustErr[T any](_ T, err error) error             { return err }
func mustErr3[T any](_ []T, _ int64, err error) error { return err }
func str(s string) *string                            { return &s }
func intp(n int) *int                                 { return &n }

func day(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }
func ymd(t time.Time) string                   { return t.Format(DateLayout) }

func rupees(n int64) kernel.Money { return kernel.Rupees(n) }

func levels() []Level {
	return []Level{
		{Code: "SELF", Name: "Employee only", EmployeeCost: rupees(500), EmployerCost: rupees(1500), MaxDependants: 0},
		{Code: "FAMILY", Name: "Family", EmployeeCost: rupees(1200), EmployerCost: rupees(2800), MaxDependants: 2},
	}
}

// plan creates an active health plan in force from 1 January 2026.
func (e *env) plan(t *testing.T, mutate func(*PlanInput)) *Plan {
	t.Helper()
	in := PlanInput{Code: str("health-" + uuid.NewString()[:4]), Name: str("Group health"), Category: str("health"), PlanYear: intp(2026), EffectiveFrom: str("2026-01-01"), Levels: levels()}
	if mutate != nil {
		mutate(&in)
	}
	p, err := e.svc.CreatePlan(ctx(), e.hr.act, in)
	ok(t, err)
	p, err = e.svc.ActivatePlan(ctx(), e.hr.act, p.ID)
	ok(t, err)
	return p
}

func (e *env) elect(t *testing.T, who person, p *Plan, level string, deps ...string) *Election {
	t.Helper()
	el, err := e.svc.Elect(ctx(), who.act, ElectInput{PlanID: p.ID.String(), LevelCode: level, DependantIDs: deps})
	ok(t, err)
	return el
}

// --- BN-C01..BN-C03 plans ---

func TestPlanValidationAndLifecycle(t *testing.T) {
	e := setup(t)
	good := func() PlanInput {
		return PlanInput{Code: str("med-26"), Name: str("Medical"), Category: str("health"), PlanYear: intp(2026), EffectiveFrom: str("2026-01-01"), Levels: levels()}
	}
	bad := map[string]func(*PlanInput){
		"missing":         func(i *PlanInput) { i.Code = nil },
		"short code":      func(i *PlanInput) { i.Code = str("a") },
		"bad code":        func(i *PlanInput) { i.Code = str("a b") },
		"no name":         func(i *PlanInput) { i.Name = str(" ") },
		"category":        func(i *PlanInput) { i.Category = str("pet") },
		"year":            func(i *PlanInput) { i.PlanYear = intp(1999) },
		"provider":        func(i *PlanInput) { i.Provider = str(strings.Repeat("p", 101)) },
		"date":            func(i *PlanInput) { i.EffectiveFrom = str("1/1/2026") },
		"no levels":       func(i *PlanInput) { i.Levels = nil },
		"many levels":     func(i *PlanInput) { i.Levels = make([]Level, 11) },
		"level code":      func(i *PlanInput) { i.Levels = []Level{{Code: "a b", Name: "x"}} },
		"dup level":       func(i *PlanInput) { i.Levels = []Level{{Code: "A", Name: "x"}, {Code: "a", Name: "y"}} },
		"level name":      func(i *PlanInput) { i.Levels = []Level{{Code: "A", Name: " "}} },
		"negative cost":   func(i *PlanInput) { i.Levels = []Level{{Code: "A", Name: "x", EmployeeCost: -1}} },
		"huge cost":       func(i *PlanInput) { i.Levels = []Level{{Code: "A", Name: "x", EmployerCost: rupees(2_000_000)}} },
		"dependants":      func(i *PlanInput) { i.Levels = []Level{{Code: "A", Name: "x", MaxDependants: 11}} },
		"eligibility n":   func(i *PlanInput) { i.Eligibility = &Eligibility{MinTenureDays: -1} },
		"eligibility big": func(i *PlanInput) { i.Eligibility = &Eligibility{MinTenureDays: 4000} },
		"eligibility ws":  func(i *PlanInput) { i.Eligibility = &Eligibility{WorkerTypes: []string{" "}} },
		"eligibility lst": func(i *PlanInput) { i.Eligibility = &Eligibility{EmploymentTypes: make([]string, 11)} },
		"eligibility ids": func(i *PlanInput) { i.Eligibility = &Eligibility{GradeIDs: make([]uuid.UUID, 51)} },
	}
	for name, mutate := range bad {
		in := good()
		mutate(&in)
		if _, err := e.svc.CreatePlan(ctx(), e.hr.act, in); err == nil {
			t.Fatalf("%s must be refused", name)
		}
	}
	p, err := e.svc.CreatePlan(ctx(), e.hr.act, good())
	if err != nil || p.Code != "MED-26" || p.Status != PlanDraft || len(p.Versions) != 1 || p.Versions[0].Version != 1 {
		t.Fatalf("created: %+v %v", p, err)
	}
	dup := good()
	dup.Code = str("med-26")
	code(t, mustErr(e.svc.CreatePlan(ctx(), e.hr.act, dup)), "CONFLICT")
	// Costs and structure cannot be patched; names, provider and eligibility can.
	for _, in := range []PlanInput{{Levels: levels()}, {Code: str("x1")}, {Category: str("life")}, {PlanYear: intp(2027)}, {EffectiveFrom: str("2027-01-01")}} {
		code(t, mustErr(e.svc.UpdatePlan(ctx(), e.hr.act, p.ID, in)), "VALIDATION_ERROR")
	}
	up, err := e.svc.UpdatePlan(ctx(), e.hr.act, p.ID, PlanInput{Name: str("Medical 2026"), Provider: str("Acme Health"), Eligibility: &Eligibility{WorkerTypes: []string{"Employee"}, MinTenureDays: 90}})
	if err != nil || up.Name != "Medical 2026" || *up.Provider != "Acme Health" || up.Eligibility.WorkerTypes[0] != "employee" {
		t.Fatalf("patch: %+v %v", up, err)
	}
	code(t, mustErr(e.svc.UpdatePlan(ctx(), e.hr.act, p.ID, PlanInput{Name: str(" ")})), "VALIDATION_ERROR")
	code(t, mustErr(e.svc.UpdatePlan(ctx(), e.hr.act, p.ID, PlanInput{Provider: str(strings.Repeat("p", 101))})), "VALIDATION_ERROR")
	code(t, mustErr(e.svc.UpdatePlan(ctx(), e.hr.act, p.ID, PlanInput{Eligibility: &Eligibility{MinTenureDays: -5}})), "VALIDATION_ERROR")
	code(t, mustErr(e.svc.UpdatePlan(ctx(), e.hr.act, uuid.New(), PlanInput{})), "NOT_FOUND")

	// BN-C02 versions: strictly later than the current one, never in the past; the previous one ends where the new starts.
	code(t, mustErr(e.svc.AddVersion(ctx(), e.hr.act, p.ID, VersionInput{EffectiveFrom: "2026-01-01", Levels: levels()})), "VALIDATION_ERROR") // not after v1
	code(t, mustErr(e.svc.AddVersion(ctx(), e.hr.act, p.ID, VersionInput{EffectiveFrom: "2026-06-09", Levels: levels()})), "VALIDATION_ERROR") // past
	code(t, mustErr(e.svc.AddVersion(ctx(), e.hr.act, p.ID, VersionInput{EffectiveFrom: "x", Levels: levels()})), "VALIDATION_ERROR")
	code(t, mustErr(e.svc.AddVersion(ctx(), e.hr.act, p.ID, VersionInput{EffectiveFrom: "2026-07-01"})), "VALIDATION_ERROR")
	code(t, mustErr(e.svc.AddVersion(ctx(), e.hr.act, uuid.New(), VersionInput{EffectiveFrom: "2026-07-01", Levels: levels()})), "NOT_FOUND")
	v2, err := e.svc.AddVersion(ctx(), e.hr.act, p.ID, VersionInput{EffectiveFrom: "2026-07-01", Levels: []Level{{Code: "SELF", Name: "Employee only", EmployeeCost: rupees(650), EmployerCost: rupees(1500)}}})
	if err != nil || len(v2.Versions) != 2 || v2.Versions[0].EffectiveTo == nil || ymd(v2.Versions[0].EffectiveTo.Time) != "2026-07-01" || v2.Versions[1].EffectiveTo != nil {
		t.Fatalf("versions: %+v %v", v2, err)
	}
	code(t, mustErr(e.svc.AddVersion(ctx(), e.hr.act, p.ID, VersionInput{EffectiveFrom: "2026-07-01", Levels: levels()})), "VALIDATION_ERROR")

	// State changes.
	act, err := e.svc.ActivatePlan(ctx(), e.hr.act, p.ID)
	if err != nil || act.Status != PlanActive {
		t.Fatal(act, err)
	}
	code(t, mustErr(e.svc.ActivatePlan(ctx(), e.hr.act, p.ID)), "PLAN_STATE")
	ret, err := e.svc.RetirePlan(ctx(), e.hr.act, p.ID)
	if err != nil || ret.Status != PlanRetired {
		t.Fatal(ret, err)
	}
	code(t, mustErr(e.svc.RetirePlan(ctx(), e.hr.act, p.ID)), "PLAN_STATE")
	code(t, mustErr(e.svc.UpdatePlan(ctx(), e.hr.act, p.ID, PlanInput{Name: str("x")})), "PLAN_STATE")
	code(t, mustErr(e.svc.AddVersion(ctx(), e.hr.act, p.ID, VersionInput{EffectiveFrom: "2026-09-01", Levels: levels()})), "PLAN_STATE")
	code(t, mustErr(e.svc.ActivatePlan(ctx(), e.hr.act, uuid.New())), "NOT_FOUND")
	if g, err := e.svc.GetPlan(ctx(), e.hr.act, p.ID); err != nil || len(g.Versions) != 2 {
		t.Fatal(g, err)
	}
	code(t, mustErr(e.svc.GetPlan(ctx(), e.hr.act, uuid.New())), "NOT_FOUND")
	if rows, total, err := e.svc.Plans(ctx(), e.hr.act, PlanRetired, 1, 20); err != nil || total != 1 || len(rows) != 1 {
		t.Fatal(rows, err)
	}
	code(t, mustErr3(e.svc.Plans(ctx(), e.hr.act, "weird", 1, 20)), "VALIDATION_ERROR")

	// Failures roll back.
	for _, k := range []string{"create", "codetaken", "lock"} {
		e.st.fail[k] = errBoom
		in := good()
		in.Code = str("other-" + k)
		if _, err := e.svc.CreatePlan(ctx(), e.hr.act, in); !errors.Is(err, errBoom) {
			t.Fatal(k, err)
		}
		delete(e.st.fail, k)
	}
	d := e.plan(t, nil)
	_ = d
	fresh, _ := e.svc.CreatePlan(ctx(), e.hr.act, PlanInput{Code: str("fresh-1"), Name: str("F"), Category: str("life"), PlanYear: intp(2026), EffectiveFrom: str("2026-01-01"), Levels: levels()})
	for _, k := range []string{"event", "save"} {
		e.st.fail[k] = errBoom
		if _, err := e.svc.ActivatePlan(ctx(), e.hr.act, fresh.ID); !errors.Is(err, errBoom) {
			t.Fatal(k, err)
		}
		delete(e.st.fail, k)
		if e.st.plans[fresh.ID].Status != PlanDraft {
			t.Fatalf("%s: a failed activation changes nothing", k)
		}
	}
}

// --- BN-C04 settings and windows ---

func TestSettingsAndWindows(t *testing.T) {
	e := setup(t)
	if st, err := e.svc.GetSettings(ctx(), e.hr.act); err != nil || st.NewHireDays != 30 || st.LifeEventDays != 30 {
		t.Fatal(st, err)
	}
	for _, in := range []SettingsInput{{NewHireDays: intp(-1)}, {LifeEventDays: intp(366)}} {
		code(t, mustErr(e.svc.PutSettings(ctx(), e.hr.act, in)), "VALIDATION_ERROR")
	}
	st, err := e.svc.PutSettings(ctx(), e.hr.act, SettingsInput{NewHireDays: intp(45)})
	if err != nil || st.NewHireDays != 45 || st.LifeEventDays != 30 {
		t.Fatal(st, err)
	}
	e.st.fail["savesettings"] = errBoom
	if _, err := e.svc.PutSettings(ctx(), e.hr.act, SettingsInput{}); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	delete(e.st.fail, "savesettings")
	e.st.fail["settings"] = errBoom
	if _, err := e.svc.GetSettings(ctx(), e.hr.act); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	delete(e.st.fail, "settings")

	bad := []WindowInput{{Name: " ", StartsOn: "2026-11-01", EndsOn: "2026-11-15"}, {Name: "x", StartsOn: "bad", EndsOn: "2026-11-15"}, {Name: "x", StartsOn: "2026-11-01", EndsOn: "bad"},
		{Name: "x", StartsOn: "2026-11-15", EndsOn: "2026-11-01"}, {Name: "x", StartsOn: "2026-01-01", EndsOn: "2026-12-31"}, {Name: strings.Repeat("n", 101), StartsOn: "2026-11-01", EndsOn: "2026-11-02"}}
	for _, in := range bad {
		if _, err := e.svc.CreateWindow(ctx(), e.hr.act, in); err == nil {
			t.Fatalf("%+v must be refused", in)
		}
	}
	w, err := e.svc.CreateWindow(ctx(), e.hr.act, WindowInput{Name: "Annual", StartsOn: "2026-11-01", EndsOn: "2026-11-15"})
	if err != nil || w.Status != WindowActive {
		t.Fatal(w, err)
	}
	c, err := e.svc.CancelWindow(ctx(), e.hr.act, w.ID)
	if err != nil || c.Status != WindowCancelled {
		t.Fatal(c, err)
	}
	if again, err := e.svc.CancelWindow(ctx(), e.hr.act, w.ID); err != nil || again.Status != WindowCancelled {
		t.Fatal("cancel is idempotent", again, err)
	}
	code(t, mustErr(e.svc.CancelWindow(ctx(), e.hr.act, uuid.New())), "NOT_FOUND")
	if rows, err := e.svc.Windows(ctx(), e.hr.act); err != nil || len(rows) != 1 {
		t.Fatal(rows, err)
	}
	e.st.fail["create"] = errBoom
	if _, err := e.svc.CreateWindow(ctx(), e.hr.act, WindowInput{Name: "x", StartsOn: "2026-11-01", EndsOn: "2026-11-02"}); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
}

// --- BN-C04 enrolment windows (BG1) ---

func TestEnrolmentWindows(t *testing.T) {
	e := setup(t)
	p := e.plan(t, nil)
	// ann joined 21 May: inside the 30-day new-hire window on 10 June (20 days), bob has no window.
	if o, err := e.svc.Options(ctx(), e.ann.act); err != nil || !o.Window.Open || o.Window.Reason != WindowNewHire {
		t.Fatalf("new hire: %+v %v", o, err)
	}
	code(t, mustErr(e.svc.Elect(ctx(), e.bob.act, ElectInput{PlanID: p.ID.String(), LevelCode: "SELF"})), "ENROLMENT_WINDOW_CLOSED") // BG1
	if o, _ := e.svc.Options(ctx(), e.bob.act); o.Window.Open {
		t.Fatal("bob has no window")
	}
	// The new-hire window ends on joining date + days, inclusive.
	e.clock = time.Date(2026, 6, 20, 10, 0, 0, 0, time.UTC) // 30 days after 21 May
	if o, _ := e.svc.Options(ctx(), e.ann.act); !o.Window.Open {
		t.Fatal("the 30th day is still inside")
	}
	e.clock = time.Date(2026, 6, 21, 10, 0, 0, 0, time.UTC)
	if o, _ := e.svc.Options(ctx(), e.ann.act); o.Window.Open {
		t.Fatal("the 31st day is outside")
	}
	e.clock = time.Date(2026, 6, 10, 10, 0, 0, 0, time.UTC)
	// An open-enrolment window covers everyone, until it is cancelled.
	w, err := e.svc.CreateWindow(ctx(), e.hr.act, WindowInput{Name: "Annual", StartsOn: "2026-06-01", EndsOn: "2026-06-10"})
	ok(t, err)
	if o, _ := e.svc.Options(ctx(), e.bob.act); !o.Window.Open || o.Window.Reason != WindowOpenEnrolment {
		t.Fatalf("open enrolment: %+v", o)
	}
	_, err = e.svc.CancelWindow(ctx(), e.hr.act, w.ID)
	ok(t, err)
	if o, _ := e.svc.Options(ctx(), e.bob.act); o.Window.Open {
		t.Fatal("a cancelled window opens nothing")
	}
	// A life event opens a window for 30 days, inclusive.
	_, err = e.svc.RecordLifeEvent(ctx(), e.bob.act, LifeEventInput{Type: "marriage", EventDate: "2026-05-11"}) // 30 days before 10 June
	ok(t, err)
	if o, _ := e.svc.Options(ctx(), e.bob.act); !o.Window.Open || o.Window.Reason != WindowLifeEvent {
		t.Fatalf("life event: %+v", o)
	}
	e.clock = time.Date(2026, 6, 11, 10, 0, 0, 0, time.UTC)
	if o, _ := e.svc.Options(ctx(), e.bob.act); o.Window.Open {
		t.Fatal("31 days after the event the window is shut")
	}
	// Window length is a setting.
	_, err = e.svc.PutSettings(ctx(), e.hr.act, SettingsInput{LifeEventDays: intp(60)})
	ok(t, err)
	if o, _ := e.svc.Options(ctx(), e.bob.act); !o.Window.Open {
		t.Fatal("the longer setting reopens it")
	}
	for _, k := range []string{"openwindow", "settings", "lifesince"} {
		e.st.fail[k] = errBoom
		if _, err := e.svc.Options(ctx(), e.bob.act); !errors.Is(err, errBoom) {
			t.Fatal(k, err)
		}
		delete(e.st.fail, k)
	}
	// A worker without an employment record only qualifies through a window or event.
	ghost := e.person(day(2026, 1, 1))
	r := e.emps.byID[ghost.emp]
	r.Employment = nil
	e.emps.byID[ghost.emp] = r
	if o, _ := e.svc.Options(ctx(), ghost.act); o.Window.Open {
		t.Fatal("no joining date, no new-hire window")
	}
}

// --- BN-C03 eligibility ---

func TestEligibility(t *testing.T) {
	e := setup(t)
	grade, loc := uuid.New(), uuid.New()
	open := e.plan(t, nil)
	tenure := e.plan(t, func(i *PlanInput) { i.Eligibility = &Eligibility{MinTenureDays: 365} })
	graded := e.plan(t, func(i *PlanInput) { i.Eligibility = &Eligibility{GradeIDs: []uuid.UUID{grade}} })
	located := e.plan(t, func(i *PlanInput) { i.Eligibility = &Eligibility{LocationIDs: []uuid.UUID{loc}} })
	contractors := e.plan(t, func(i *PlanInput) { i.Eligibility = &Eligibility{WorkerTypes: []string{"Contractor"}} })
	interns := e.plan(t, func(i *PlanInput) { i.Eligibility = &Eligibility{EmploymentTypes: []string{"intern"}} })
	_, err := e.svc.CreateWindow(ctx(), e.hr.act, WindowInput{Name: "Annual", StartsOn: "2026-06-01", EndsOn: "2026-06-30"})
	ok(t, err)

	offered := func(who person) map[uuid.UUID]bool {
		o, err := e.svc.Options(ctx(), who.act)
		ok(t, err)
		out := map[uuid.UUID]bool{}
		for _, x := range o.Plans {
			out[x.Plan.ID] = true
		}
		return out
	}
	// ann: 20 days of tenure, no grade or location; bob: years of tenure.
	if got := offered(e.ann); !got[open.ID] || got[tenure.ID] || got[graded.ID] || got[located.ID] || got[contractors.ID] || got[interns.ID] {
		t.Fatalf("ann is offered only the open plan: %v", got)
	}
	e.core.place[e.bob.emp] = &coreAPI.Placement{GradeID: &grade, LocationID: &loc, WorkerType: "employee"}
	if got := offered(e.bob); !got[open.ID] || !got[tenure.ID] || !got[graded.ID] || !got[located.ID] || got[contractors.ID] || got[interns.ID] {
		t.Fatalf("bob: %v", got)
	}
	e.core.place[e.bob.emp].WorkerType = "contractor"
	if got := offered(e.bob); !got[contractors.ID] {
		t.Fatal("worker type match is case-insensitive")
	}
	e.core.place[e.bob.emp] = nil // no placement: every placement restriction fails
	if got := offered(e.bob); got[graded.ID] || got[located.ID] || got[contractors.ID] || !got[open.ID] || !got[tenure.ID] {
		t.Fatalf("no placement: %v", got)
	}
	r := e.emps.byID[e.bob.emp]
	r.Employment.EmploymentType = "Intern"
	e.emps.byID[e.bob.emp] = r
	if got := offered(e.bob); !got[interns.ID] {
		t.Fatal("employment type match")
	}
	code(t, mustErr(e.svc.Elect(ctx(), e.ann.act, ElectInput{PlanID: tenure.ID.String(), LevelCode: "SELF"})), "NOT_ELIGIBLE")
	// No version in force today hides a plan.
	future := e.plan(t, func(i *PlanInput) { i.EffectiveFrom = str("2026-12-01") })
	if got := offered(e.ann); got[future.ID] {
		t.Fatal("a plan with no version in force is not offered")
	}
	// Failures.
	for _, k := range []string{"activeplans", "versions", "live"} {
		e.st.fail[k] = errBoom
		if _, err := e.svc.Options(ctx(), e.ann.act); !errors.Is(err, errBoom) {
			t.Fatal(k, err)
		}
		delete(e.st.fail, k)
	}
	e.core.err = errBoom
	if _, err := e.svc.Options(ctx(), e.ann.act); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	e.core.err = nil
	code(t, mustErr(e.svc.Options(ctx(), Actor{TenantID: e.tenant, UserID: uuid.New()})), "EMPLOYEE_NOT_FOUND")
}

// --- BN-C05, BN-C06 elections ---

func (e *env) dependant(t *testing.T, who person, name string) *Dependant {
	t.Helper()
	d, err := e.svc.CreateDependant(ctx(), who.act, DependantInput{Name: str(name), Relationship: str("child"), DateOfBirth: str("2018-04-02")})
	ok(t, err)
	return d
}

func TestElect(t *testing.T) {
	e := setup(t)
	p := e.plan(t, nil)
	draft, _ := e.svc.CreatePlan(ctx(), e.hr.act, PlanInput{Code: str("draft-1"), Name: str("D"), Category: str("life"), PlanYear: intp(2026), EffectiveFrom: str("2026-01-01"), Levels: levels()})
	d1, d2, d3 := e.dependant(t, e.ann, "A"), e.dependant(t, e.ann, "B"), e.dependant(t, e.ann, "C")
	other := e.dependant(t, e.bob, "Z")
	bad := []ElectInput{
		{PlanID: "x", LevelCode: "SELF"},
		{PlanID: p.ID.String(), LevelCode: "GOLD"},
		{PlanID: p.ID.String(), LevelCode: "SELF", DependantIDs: []string{d1.ID.String()}},                                   // level covers none
		{PlanID: p.ID.String(), LevelCode: "FAMILY", DependantIDs: []string{d1.ID.String(), d2.ID.String(), d3.ID.String()}}, // too many
		{PlanID: p.ID.String(), LevelCode: "FAMILY", DependantIDs: []string{other.ID.String()}},                              // someone else's
		{PlanID: p.ID.String(), LevelCode: "FAMILY", DependantIDs: []string{"x"}},                                            // not a uuid
		{PlanID: p.ID.String(), LevelCode: "SELF", EffectiveFrom: str("2026-06-09")},                                         // past
		{PlanID: p.ID.String(), LevelCode: "SELF", EffectiveFrom: str("2026-09-09")},                                         // too far
		{PlanID: p.ID.String(), LevelCode: "SELF", EffectiveFrom: str("bad")},
	}
	for i, in := range bad {
		if _, err := e.svc.Elect(ctx(), e.ann.act, in); err == nil {
			t.Fatalf("case %d must be refused: %+v", i, in)
		}
	}
	code(t, mustErr(e.svc.Elect(ctx(), e.ann.act, ElectInput{PlanID: draft.ID.String(), LevelCode: "SELF"})), "PLAN_STATE")
	code(t, mustErr(e.svc.Elect(ctx(), e.ann.act, ElectInput{PlanID: uuid.NewString(), LevelCode: "SELF"})), "NOT_FOUND")
	code(t, mustErr(e.svc.Elect(ctx(), Actor{TenantID: e.tenant, UserID: uuid.New()}, ElectInput{PlanID: p.ID.String(), LevelCode: "SELF"})), "EMPLOYEE_NOT_FOUND")

	el, err := e.svc.Elect(ctx(), e.ann.act, ElectInput{PlanID: p.ID.String(), LevelCode: "FAMILY", DependantIDs: []string{d1.ID.String(), d1.ID.String(), d2.ID.String()}})
	if err != nil || el.Status != ElActive || ymd(el.EffectiveFrom.Time) != "2026-06-10" || len(el.DependantIDs) != 2 || *el.LevelCode != "FAMILY" {
		t.Fatalf("elected: %+v %v", el, err)
	}
	code(t, mustErr(e.svc.Elect(ctx(), e.ann.act, ElectInput{PlanID: p.ID.String(), LevelCode: "SELF"})), "ELECTION_EXISTS")
	if e.st.outbox[len(e.st.outbox)-1] != "benefits.election.created" {
		t.Fatal(e.st.outbox)
	}
	// A future start within 90 days is allowed; the version must be in force then.
	other2 := e.plan(t, nil)
	fut, err := e.svc.Elect(ctx(), e.ann.act, ElectInput{PlanID: other2.ID.String(), LevelCode: "SELF", EffectiveFrom: str("2026-07-01")})
	if err != nil || ymd(fut.EffectiveFrom.Time) != "2026-07-01" {
		t.Fatal(fut, err)
	}
	// Failures roll back.
	third := e.plan(t, nil)
	for _, k := range []string{"lock", "live", "create", "event", "versions"} {
		e.st.fail[k] = errBoom
		if _, err := e.svc.Elect(ctx(), e.ann.act, ElectInput{PlanID: third.ID.String(), LevelCode: "SELF"}); !errors.Is(err, errBoom) {
			t.Fatal(k, err)
		}
		delete(e.st.fail, k)
	}
	e.st.fail["dependants"] = errBoom
	if _, err := e.svc.Elect(ctx(), e.ann.act, ElectInput{PlanID: third.ID.String(), LevelCode: "FAMILY", DependantIDs: []string{d1.ID.String()}}); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	delete(e.st.fail, "dependants")
	for _, x := range e.st.elections {
		if x.PlanID == third.ID {
			t.Fatal("a failed election leaves nothing behind")
		}
	}
	e.core.err = errBoom
	if _, err := e.svc.Elect(ctx(), e.ann.act, ElectInput{PlanID: third.ID.String(), LevelCode: "SELF"}); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	e.core.err = nil
	// A version that has not started yet cannot be elected for today.
	late := e.plan(t, func(i *PlanInput) { i.EffectiveFrom = str("2026-08-01") })
	code(t, mustErr(e.svc.Elect(ctx(), e.ann.act, ElectInput{PlanID: late.ID.String(), LevelCode: "SELF"})), "VALIDATION_ERROR")
}

func TestChangeWaiveEnd(t *testing.T) {
	e := setup(t)
	p := e.plan(t, nil)
	dep := e.dependant(t, e.ann, "A")
	el := e.elect(t, e.ann, p, "SELF")

	// Change ends the old election on the new one's start and writes the new one.
	code(t, mustErr(e.svc.Change(ctx(), e.ann.act, el.ID, ChangeInput{LevelCode: "GOLD"})), "VALIDATION_ERROR")
	code(t, mustErr(e.svc.Change(ctx(), e.ann.act, el.ID, ChangeInput{LevelCode: "FAMILY", EffectiveFrom: str("bad")})), "VALIDATION_ERROR")
	code(t, mustErr(e.svc.Change(ctx(), e.bob.act, el.ID, ChangeInput{LevelCode: "FAMILY"})), "ENROLMENT_WINDOW_CLOSED")
	code(t, mustErr(e.svc.Change(ctx(), e.ann.act, uuid.New(), ChangeInput{LevelCode: "FAMILY"})), "NOT_FOUND")
	_, err := e.svc.CreateWindow(ctx(), e.hr.act, WindowInput{Name: "Annual", StartsOn: "2026-06-01", EndsOn: "2026-06-30"})
	ok(t, err)
	code(t, mustErr(e.svc.Change(ctx(), e.bob.act, el.ID, ChangeInput{LevelCode: "FAMILY"})), "NOT_FOUND") // someone else's election
	ch, err := e.svc.Change(ctx(), e.ann.act, el.ID, ChangeInput{LevelCode: "FAMILY", DependantIDs: []string{dep.ID.String()}, EffectiveFrom: str("2026-07-01")})
	if err != nil || *ch.LevelCode != "FAMILY" || ymd(ch.EffectiveFrom.Time) != "2026-07-01" || ch.Status != ElActive {
		t.Fatalf("changed: %+v %v", ch, err)
	}
	old := e.st.elections[el.ID]
	if old.Status != ElEnded || ymd(old.EffectiveTo.Time) != "2026-07-01" || *old.EndReason != ReasonChanged {
		t.Fatalf("the old election ends where the new one starts: %+v", old)
	}
	code(t, mustErr(e.svc.Change(ctx(), e.ann.act, el.ID, ChangeInput{LevelCode: "SELF"})), "ELECTION_STATE") // already ended
	code(t, mustErr(e.svc.Change(ctx(), e.ann.act, ch.ID, ChangeInput{LevelCode: "SELF", EffectiveFrom: str("2026-06-20")})), "VALIDATION_ERROR")
	for _, k := range []string{"save", "event", "create"} {
		e.st.fail[k] = errBoom
		if _, err := e.svc.Change(ctx(), e.ann.act, ch.ID, ChangeInput{LevelCode: "SELF", EffectiveFrom: str("2026-07-15")}); !errors.Is(err, errBoom) {
			t.Fatal(k, err)
		}
		delete(e.st.fail, k)
		if e.st.elections[ch.ID].EffectiveTo != nil {
			t.Fatalf("%s: a failed change leaves the election as it was", k)
		}
	}
	// A retired plan cannot be changed.
	rp := e.plan(t, nil)
	rel := e.elect(t, e.ann, rp, "SELF")
	_, _ = e.svc.RetirePlan(ctx(), e.hr.act, rp.ID)
	code(t, mustErr(e.svc.Change(ctx(), e.ann.act, rel.ID, ChangeInput{LevelCode: "FAMILY"})), "PLAN_STATE")

	// Waive: explicit, idempotent, ends any active election, and is replaced by a later election.
	p2 := e.plan(t, nil)
	live := e.elect(t, e.ann, p2, "SELF")
	w, err := e.svc.Waive(ctx(), e.ann.act, p2.ID.String())
	if err != nil || w.Status != ElWaived || w.LevelCode != nil {
		t.Fatalf("waived: %+v %v", w, err)
	}
	if st := e.st.elections[live.ID]; st.Status != ElEnded || *st.EndReason != ReasonWaived {
		t.Fatalf("the active election ended: %+v", st)
	}
	if again, err := e.svc.Waive(ctx(), e.ann.act, p2.ID.String()); err != nil || again.ID != w.ID {
		t.Fatal("waiving twice returns the waiver", again, err)
	}
	code(t, mustErr(e.svc.Waive(ctx(), e.ann.act, "x")), "VALIDATION_ERROR")
	code(t, mustErr(e.svc.Waive(ctx(), e.ann.act, uuid.NewString())), "NOT_FOUND")
	back, err := e.svc.Elect(ctx(), e.ann.act, ElectInput{PlanID: p2.ID.String(), LevelCode: "SELF"})
	if err != nil || e.st.elections[w.ID].Status != ElEnded || *e.st.elections[w.ID].EndReason != ReasonReplaced {
		t.Fatalf("electing replaces the waiver: %+v %v", back, err)
	}
	for _, k := range []string{"live", "create"} {
		p3 := e.plan(t, nil)
		e.st.fail[k] = errBoom
		if _, err := e.svc.Waive(ctx(), e.ann.act, p3.ID.String()); !errors.Is(err, errBoom) {
			t.Fatal(k, err)
		}
		delete(e.st.fail, k)
	}

	// End: the owner inside a window; HR any time; a future election is cancelled; waivers and ended ones are state errors.
	p4 := e.plan(t, nil)
	mine := e.elect(t, e.ann, p4, "SELF")
	code(t, mustErr(e.svc.End(ctx(), e.bob.act, mine.ID)), "NOT_FOUND") // not bob's election
	code(t, mustErr(e.svc.End(ctx(), e.ann.act, uuid.New())), "NOT_FOUND")
	fut, _ := e.svc.Elect(ctx(), e.ann.act, ElectInput{PlanID: e.plan(t, nil).ID.String(), LevelCode: "SELF", EffectiveFrom: str("2026-07-05")})
	cancelled, err := e.svc.End(ctx(), e.ann.act, fut.ID)
	if err != nil || ymd(cancelled.EffectiveTo.Time) != "2026-07-05" || cancelled.Status != ElEnded {
		t.Fatalf("a future election is cancelled (empty range): %+v %v", cancelled, err)
	}
	ended, err := e.svc.End(ctx(), e.ann.act, mine.ID)
	if err != nil || ymd(ended.EffectiveTo.Time) != "2026-06-10" || *ended.EndReason != ReasonEnded {
		t.Fatal(ended, err)
	}
	code(t, mustErr(e.svc.End(ctx(), e.ann.act, mine.ID)), "ELECTION_STATE")
	code(t, mustErr(e.svc.End(ctx(), e.ann.act, w.ID)), "ELECTION_STATE")
	// Close the window: the owner cannot end, HR can.
	for id, win := range e.st.windows {
		win.Status = WindowCancelled
		e.st.windows[id] = win
	}
	e.clock = day(2026, 9, 1)
	p5 := e.plan(t, nil)
	bobs := e.elect2(t, e.bob, p5)
	code(t, mustErr(e.svc.End(ctx(), e.bob.act, bobs.ID)), "ENROLMENT_WINDOW_CLOSED")
	hrEnd, err := e.svc.End(ctx(), e.hr.act, bobs.ID)
	if err != nil || hrEnd.Status != ElEnded {
		t.Fatal("benefits:manage ends an election at any time", hrEnd, err)
	}
	for _, k := range []string{"save", "event"} {
		x := e.elect2(t, e.hr, e.plan(t, nil))
		e.st.fail[k] = errBoom
		if _, err := e.svc.End(ctx(), e.hr.act, x.ID); !errors.Is(err, errBoom) {
			t.Fatal(k, err)
		}
		delete(e.st.fail, k)
	}
	e.emps.err = errBoom
	if _, err := e.svc.End(ctx(), e.ann.act, bobs.ID); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
}

// elect2 writes an active election directly, bypassing windows.
func (e *env) elect2(t *testing.T, who person, p *Plan) Election {
	t.Helper()
	x := Election{ID: uuid.New(), TenantID: e.tenant, EmployeeProfileID: who.emp, PlanID: p.ID, LevelCode: str("SELF"), DependantIDs: IDs{}, Status: ElActive, EffectiveFrom: Date{day(2026, 1, 1)}}
	e.st.elections[x.ID] = x
	return x
}

// --- BN-C07 payroll hand-off (BG5) ---

func TestDeductions(t *testing.T) {
	e := setup(t)
	p := e.plan(t, func(i *PlanInput) { i.Code = str("health-a") })
	_, err := e.svc.AddVersion(ctx(), e.hr.act, p.ID, VersionInput{EffectiveFrom: "2026-07-01", Levels: []Level{
		{Code: "SELF", Name: "Employee only", EmployeeCost: rupees(650), EmployerCost: rupees(1500)}, {Code: "FAMILY", Name: "Family", EmployeeCost: rupees(1400), EmployerCost: rupees(2800), MaxDependants: 2}}})
	ok(t, err)
	life := e.plan(t, func(i *PlanInput) {
		i.Code = str("life-b")
		i.Category = str("life")
		i.Levels = []Level{{Code: "BASIC", Name: "Basic", EmployeeCost: rupees(100)}, {Code: "FREE", Name: "Free"}}
	})
	el := e.elect(t, e.ann, p, "SELF")
	_ = el
	e.elect(t, e.ann, life, "BASIC")
	lines := func(from, to time.Time) []string {
		out, err := e.svc.Deductions(ctx(), e.tenant, e.ann.emp, from, to)
		ok(t, err)
		var s []string
		for _, l := range out {
			s = append(s, l.Code+"="+l.Amount.String())
		}
		return s
	}
	june := lines(day(2026, 6, 1), day(2026, 6, 30))
	if len(june) != 2 || june[0] != "BEN_HEALTH_A=500.00" || june[1] != "BEN_LIFE_B=100.00" {
		t.Fatalf("BG5 June at the version-1 cost: %v", june)
	}
	july := lines(day(2026, 7, 1), day(2026, 7, 31))
	if july[0] != "BEN_HEALTH_A=650.00" {
		t.Fatalf("BG5 a cost change applies only from its new version: %v", july)
	}
	// An election that starts mid-month is charged a whole month when it covers the last day, none before it starts.
	if got := lines(day(2026, 5, 1), day(2026, 5, 31)); len(got) != 0 {
		t.Fatalf("not elected in May: %v", got)
	}
	// Ending before the last day of the period stops the charge.
	_, err = e.svc.CreateWindow(ctx(), e.hr.act, WindowInput{Name: "Annual", StartsOn: "2026-06-01", EndsOn: "2026-06-30"})
	ok(t, err)
	var lifeEl Election
	for _, x := range e.st.elections {
		if x.PlanID == life.ID {
			lifeEl = x
		}
	}
	_, err = e.svc.End(ctx(), e.ann.act, lifeEl.ID)
	ok(t, err)
	if got := lines(day(2026, 6, 1), day(2026, 6, 30)); len(got) != 1 {
		t.Fatalf("ended today: no charge for the month: %v", got)
	}
	// Waived elections, free levels and unknown levels cost nothing; employer cost is never deducted.
	free := e.plan(t, func(i *PlanInput) { i.Code = str("free-c") })
	e.elect(t, e.bob, free, "SELF")
	if _, err := e.svc.Waive(ctx(), e.ann.act, free.ID.String()); err != nil {
		t.Fatal(err)
	}
	for id, x := range e.st.elections {
		if x.PlanID == free.ID && x.EmployeeProfileID == e.bob.emp {
			lv := "GONE"
			x.LevelCode = &lv
			e.st.elections[id] = x
		}
	}
	if out, _ := e.svc.Deductions(ctx(), e.tenant, e.bob.emp, day(2026, 6, 1), day(2026, 6, 30)); len(out) != 0 {
		t.Fatalf("an unknown level is skipped: %v", out)
	}
	// MyDeductions previews a month.
	got, err := e.svc.MyDeductions(ctx(), e.ann.act, "2026-07")
	if err != nil || len(got) != 1 || got[0].Amount != rupees(650) {
		t.Fatal(got, err)
	}
	if cur, err := e.svc.MyDeductions(ctx(), e.ann.act, ""); err != nil || len(cur) != 1 {
		t.Fatal("the default is the current month", cur, err)
	}
	code(t, mustErr(e.svc.MyDeductions(ctx(), e.ann.act, "June")), "VALIDATION_ERROR")
	code(t, mustErr(e.svc.MyDeductions(ctx(), Actor{TenantID: e.tenant, UserID: uuid.New()}, "")), "EMPLOYEE_NOT_FOUND")
	for _, k := range []string{"electionson", "plan", "versions"} {
		e.st.fail[k] = errBoom
		if _, err := e.svc.Deductions(ctx(), e.tenant, e.ann.emp, day(2026, 6, 1), day(2026, 6, 30)); !errors.Is(err, errBoom) {
			t.Fatal(k, err)
		}
		delete(e.st.fail, k)
	}
}

// --- BN-C08 termination (BG3) ---

func TestTerminationHook(t *testing.T) {
	e := setup(t)
	p := e.plan(t, nil)
	el := e.elect(t, e.ann, p, "FAMILY")
	wp := e.plan(t, nil)
	_, err := e.svc.CreateWindow(ctx(), e.hr.act, WindowInput{Name: "Annual", StartsOn: "2026-06-01", EndsOn: "2026-06-30"})
	ok(t, err)
	_, err = e.svc.Waive(ctx(), e.ann.act, wp.ID.String())
	ok(t, err)
	action := uuid.New()
	last := day(2026, 6, 15)
	jun30 := func() int {
		out, err := e.svc.Deductions(ctx(), e.tenant, e.ann.emp, day(2026, 6, 1), day(2026, 6, 30))
		ok(t, err)
		return len(out)
	}
	if jun30() != 1 {
		t.Fatal("deducting before the termination")
	}
	// Other action types do nothing.
	ok(t, e.svc.CoreHook(ctx(), nil, e.tenant, coreAPI.ActionFact{ActionID: uuid.New(), EmployeeID: e.ann.emp, Type: "promote"}))
	if jun30() != 1 {
		t.Fatal("a promotion does not end elections")
	}
	ok(t, e.svc.CoreHook(ctx(), nil, e.tenant, coreAPI.ActionFact{ActionID: action, EmployeeID: e.ann.emp, Type: "terminate", EffectiveDate: day(2026, 6, 14), LastWorkingDay: &last}))
	ended := e.st.elections[el.ID]
	if ended.Status != ElEnded || ymd(ended.EffectiveTo.Time) != "2026-06-16" || *ended.EndReason != ReasonTermination || *ended.EndActionID != action {
		t.Fatalf("BG3 ends the day after the last working day: %+v", ended)
	}
	if jun30() != 0 {
		t.Fatal("BG3 no deduction for the month containing the last working day")
	}
	if e.st.outbox[len(e.st.outbox)-1] != "benefits.election.ended" {
		t.Fatal(e.st.outbox)
	}
	// A waiver stays; ending again changes nothing and writes no event.
	before := len(e.st.outbox)
	ok(t, e.svc.CoreHook(ctx(), nil, e.tenant, coreAPI.ActionFact{ActionID: uuid.New(), EmployeeID: e.ann.emp, Type: "terminate", EffectiveDate: day(2026, 6, 14)}))
	if len(e.st.outbox) != before {
		t.Fatal("nothing left to end, no event")
	}
	// Rescinding the termination reinstates exactly those elections.
	ok(t, e.svc.CoreHook(ctx(), nil, e.tenant, coreAPI.ActionFact{ActionID: action, EmployeeID: e.ann.emp, Type: "terminate", Rescinded: true}))
	back := e.st.elections[el.ID]
	if back.Status != ElActive || back.EffectiveTo != nil || back.EndReason != nil || jun30() != 1 {
		t.Fatalf("BG3 rescinded: %+v", back)
	}
	// Without a last working day the effective date counts.
	ok(t, e.svc.CoreHook(ctx(), nil, e.tenant, coreAPI.ActionFact{ActionID: uuid.New(), EmployeeID: e.ann.emp, Type: "terminate", EffectiveDate: day(2026, 6, 20)}))
	if ymd(e.st.elections[el.ID].EffectiveTo.Time) != "2026-06-21" {
		t.Fatal("effective date is the fallback")
	}
	// A termination dated before the election starts leaves an empty range.
	fut := e.plan(t, nil)
	f, err := e.svc.Elect(ctx(), e.ann.act, ElectInput{PlanID: fut.ID.String(), LevelCode: "SELF", EffectiveFrom: str("2026-07-20")})
	ok(t, err)
	ok(t, e.svc.CoreHook(ctx(), nil, e.tenant, coreAPI.ActionFact{ActionID: uuid.New(), EmployeeID: e.ann.emp, Type: "terminate", EffectiveDate: day(2026, 6, 30)}))
	if got := e.st.elections[f.ID]; got.EffectiveTo == nil || ymd(got.EffectiveTo.Time) != "2026-07-20" {
		t.Fatalf("never takes effect: %+v", got)
	}
	// A failure rolls the action back.
	e.st.fail["endterm"] = errBoom
	if err := e.svc.CoreHook(ctx(), nil, e.tenant, coreAPI.ActionFact{ActionID: uuid.New(), EmployeeID: e.ann.emp, Type: "terminate"}); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	delete(e.st.fail, "endterm")
	e.st.fail["reinstate"] = errBoom
	if err := e.svc.CoreHook(ctx(), nil, e.tenant, coreAPI.ActionFact{ActionID: action, EmployeeID: e.ann.emp, Type: "terminate", Rescinded: true}); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
}

// --- BN-C09 dependants and life events (BG4) ---

func TestDependantsAndLifeEvents(t *testing.T) {
	e := setup(t)
	bad := []DependantInput{{}, {Name: str(" "), Relationship: str("child"), DateOfBirth: str("2018-01-01")}, {Name: str("A"), Relationship: str("pet"), DateOfBirth: str("2018-01-01")},
		{Name: str("A"), Relationship: str("child"), DateOfBirth: str("bad")}, {Name: str("A"), Relationship: str("child"), DateOfBirth: str("2027-01-01")},
		{Name: str("A"), Relationship: str("child"), DateOfBirth: str("1899-01-01")}, {Name: str(strings.Repeat("n", 101)), Relationship: str("child"), DateOfBirth: str("2018-01-01")}}
	for i, in := range bad {
		if _, err := e.svc.CreateDependant(ctx(), e.ann.act, in); err == nil {
			t.Fatalf("case %d must be refused", i)
		}
	}
	code(t, mustErr(e.svc.CreateDependant(ctx(), Actor{TenantID: e.tenant, UserID: uuid.New()}, DependantInput{Name: str("A"), Relationship: str("child"), DateOfBirth: str("2018-01-01")})), "EMPLOYEE_NOT_FOUND")
	d := e.dependant(t, e.ann, "Asha")
	up, err := e.svc.UpdateDependant(ctx(), e.ann.act, d.ID, DependantInput{Name: str("Asha K"), Relationship: str("spouse")})
	if err != nil || up.Name != "Asha K" || up.Relationship != "spouse" {
		t.Fatal(up, err)
	}
	code(t, mustErr(e.svc.UpdateDependant(ctx(), e.bob.act, d.ID, DependantInput{Name: str("x")})), "NOT_FOUND") // BG4 not anyone else's
	code(t, mustErr(e.svc.UpdateDependant(ctx(), e.ann.act, uuid.New(), DependantInput{})), "NOT_FOUND")
	code(t, mustErr(e.svc.UpdateDependant(ctx(), e.ann.act, d.ID, DependantInput{DateOfBirth: str("bad")})), "VALIDATION_ERROR")
	if rows, err := e.svc.MyDependants(ctx(), e.ann.act); err != nil || len(rows) != 1 {
		t.Fatal(rows, err)
	}
	if rows, _ := e.svc.MyDependants(ctx(), e.bob.act); len(rows) != 0 {
		t.Fatal("BG4 each worker sees only their own")
	}
	if rows, err := e.svc.EmployeeDependants(ctx(), e.hr.act, e.ann.emp); err != nil || len(rows) != 1 {
		t.Fatal(rows, err)
	}
	code(t, mustErr(e.svc.EmployeeDependants(ctx(), e.hr.act, uuid.New())), "EMPLOYEE_NOT_FOUND")

	// A covered dependant cannot be removed while the election is live.
	p := e.plan(t, nil)
	el := e.elect(t, e.ann, p, "FAMILY", d.ID.String())
	code(t, mustErr(e.svc.RemoveDependant(ctx(), e.ann.act, d.ID)), "DEPENDANT_IN_USE")
	code(t, mustErr(e.svc.RemoveDependant(ctx(), e.bob.act, d.ID)), "NOT_FOUND")
	_, err = e.svc.CreateWindow(ctx(), e.hr.act, WindowInput{Name: "Annual", StartsOn: "2026-06-01", EndsOn: "2026-06-30"})
	ok(t, err)
	_, err = e.svc.End(ctx(), e.ann.act, el.ID)
	ok(t, err)
	rm, err := e.svc.RemoveDependant(ctx(), e.ann.act, d.ID)
	if err != nil || rm.Status != "removed" {
		t.Fatal(rm, err)
	}
	if again, err := e.svc.RemoveDependant(ctx(), e.ann.act, d.ID); err != nil || again.Status != "removed" {
		t.Fatal("removing twice is fine", again, err)
	}
	code(t, mustErr(e.svc.UpdateDependant(ctx(), e.ann.act, d.ID, DependantInput{Name: str("x")})), "NOT_FOUND") // removed
	for _, k := range []string{"create", "save", "dependant", "electionson"} {
		e.st.fail[k] = errBoom
		var err error
		switch k {
		case "create":
			_, err = e.svc.CreateDependant(ctx(), e.ann.act, DependantInput{Name: str("A"), Relationship: str("child"), DateOfBirth: str("2018-01-01")})
		case "save":
			x := e.dependant2(t, e.ann)
			_, err = e.svc.UpdateDependant(ctx(), e.ann.act, x.ID, DependantInput{Name: str("y")})
		default:
			x := e.dependant2(t, e.ann)
			_, err = e.svc.RemoveDependant(ctx(), e.ann.act, x.ID)
		}
		if !errors.Is(err, errBoom) {
			t.Fatal(k, err)
		}
		delete(e.st.fail, k)
	}

	// Life events.
	badEv := []LifeEventInput{{Type: "party", EventDate: "2026-06-01"}, {Type: "birth", EventDate: "bad"}, {Type: "birth", EventDate: "2026-12-01"}, {Type: "birth", EventDate: "1899-01-01"},
		{Type: "birth", EventDate: "2026-06-01", EvidenceRef: str(strings.Repeat("e", 301))}, {Type: "birth", EventDate: "2026-06-01", EmployeeID: str("x")}}
	for i, in := range badEv {
		if _, err := e.svc.RecordLifeEvent(ctx(), e.hr.act, in); err == nil {
			t.Fatalf("life event case %d must be refused", i)
		}
	}
	code(t, mustErr(e.svc.RecordLifeEvent(ctx(), e.ann.act, LifeEventInput{Type: "birth", EventDate: "2026-06-01", EmployeeID: str(e.bob.emp.String())})), "FORBIDDEN")
	code(t, mustErr(e.svc.RecordLifeEvent(ctx(), e.hr.act, LifeEventInput{Type: "birth", EventDate: "2026-06-01", EmployeeID: str(uuid.NewString())})), "EMPLOYEE_NOT_FOUND")
	code(t, mustErr(e.svc.RecordLifeEvent(ctx(), Actor{TenantID: e.tenant, UserID: uuid.New()}, LifeEventInput{Type: "birth", EventDate: "2026-06-01"})), "EMPLOYEE_NOT_FOUND")
	ev, err := e.svc.RecordLifeEvent(ctx(), e.hr.act, LifeEventInput{Type: "birth", EventDate: "2026-06-01", EmployeeID: str(e.bob.emp.String()), EvidenceRef: str("docs/birth.pdf")})
	if err != nil || ev.EmployeeProfileID != e.bob.emp {
		t.Fatal("HR records for a worker", ev, err)
	}
	if rows, err := e.svc.MyLifeEvents(ctx(), e.bob.act); err != nil || len(rows) != 1 {
		t.Fatal(rows, err)
	}
	if rows, _ := e.svc.MyLifeEvents(ctx(), e.ann.act); len(rows) != 0 {
		t.Fatal("BG4 own events only")
	}
	if rows, err := e.svc.EmployeeLifeEvents(ctx(), e.hr.act, e.bob.emp); err != nil || len(rows) != 1 {
		t.Fatal(rows, err)
	}
	code(t, mustErr(e.svc.EmployeeLifeEvents(ctx(), e.hr.act, uuid.New())), "EMPLOYEE_NOT_FOUND")
	e.st.fail["create"] = errBoom
	if _, err := e.svc.RecordLifeEvent(ctx(), e.ann.act, LifeEventInput{Type: "birth", EventDate: "2026-06-01"}); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
}

func (e *env) dependant2(t *testing.T, who person) *Dependant {
	t.Helper()
	return e.dependant(t, who, "Extra")
}

// --- BN-C10 report and listings ---

func TestReportAndListings(t *testing.T) {
	e := setup(t)
	p := e.plan(t, nil)
	_, err := e.svc.CreateWindow(ctx(), e.hr.act, WindowInput{Name: "Annual", StartsOn: "2026-06-01", EndsOn: "2026-06-30"})
	ok(t, err)
	e.elect(t, e.ann, p, "SELF")
	e.elect(t, e.bob, p, "SELF")
	third := e.person(day(2022, 1, 1))
	e.elect(t, third, p, "FAMILY")
	w := e.person(day(2022, 1, 1))
	_, err = e.svc.Waive(ctx(), w.act, p.ID.String())
	ok(t, err)
	rows, err := e.svc.EnrolmentReport(ctx(), e.hr.act)
	if err != nil || len(rows) != 3 {
		t.Fatalf("report: %+v %v", rows, err)
	}
	var self, fam, waived ReportRow
	for _, r := range rows {
		switch r.LevelCode {
		case "SELF":
			self = r
		case "FAMILY":
			fam = r
		case "":
			waived = r
		}
	}
	if self.Active != 2 || self.EmployeeCost != rupees(1000) || self.EmployerCost != rupees(3000) || fam.Active != 1 || fam.EmployeeCost != rupees(1200) || waived.Waived != 1 || waived.Active != 0 {
		t.Fatalf("counts and costs: %+v", rows)
	}
	if rows, _ := (setup(t)).svc.EnrolmentReport(ctx(), e.hr.act); rows == nil || len(rows) != 0 {
		t.Fatal("an empty report is an empty list")
	}
	for _, k := range []string{"counts", "versions", "plan"} {
		e.st.fail[k] = errBoom
		if _, err := e.svc.EnrolmentReport(ctx(), e.hr.act); !errors.Is(err, errBoom) {
			t.Fatal(k, err)
		}
		delete(e.st.fail, k)
	}
	// Listings.
	mine, err := e.svc.MyElections(ctx(), e.ann.act)
	if err != nil || len(mine) != 1 {
		t.Fatal(mine, err)
	}
	if rows, total, err := e.svc.Elections(ctx(), e.hr.act, ElectionFilter{PlanID: &p.ID, Status: ElActive}, 1, 20); err != nil || total != 3 || len(rows) != 3 {
		t.Fatal(total, err)
	}
	code(t, mustErr3(e.svc.Elections(ctx(), e.hr.act, ElectionFilter{Status: "weird"}, 1, 20)), "VALIDATION_ERROR")
	e.st.fail["empelections"] = errBoom
	if _, err := e.svc.MyElections(ctx(), e.ann.act); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	code(t, mustErr(e.svc.MyElections(ctx(), Actor{TenantID: e.tenant, UserID: uuid.New()})), "EMPLOYEE_NOT_FOUND")
}

func TestModelsAndHelpers(t *testing.T) {
	var d Date
	if d.Scan(time.Date(2026, 6, 10, 15, 0, 0, 0, time.UTC)) != nil || ymd(d.Time) != "2026-06-10" || d.Scan("2026-07-01T00:00:00Z") != nil || d.Scan(5) == nil {
		t.Fatal("date scan")
	}
	if v, _ := d.Value(); v != "2026-07-01" {
		t.Fatal(v)
	}
	if b, _ := d.MarshalJSON(); string(b) != `"2026-07-01"` {
		t.Fatal(string(b))
	}
	if d.Scan("bad") == nil {
		t.Fatal("bad date")
	}
	var el Eligibility
	v, err := Eligibility{MinTenureDays: 30}.Value()
	ok(t, err)
	if el.Scan(v.(string)) != nil || el.MinTenureDays != 30 || el.Scan([]byte(`{}`)) != nil || el.Scan(1) == nil {
		t.Fatal("eligibility")
	}
	var lv Levels
	v, _ = Levels(nil).Value()
	if v != "[]" || lv.Scan(`[{"code":"A","name":"x","employee_cost":"10.00","employer_cost":"5.00","max_dependants":1}]`) != nil || lv[0].EmployeeCost != rupees(10) {
		t.Fatal("levels")
	}
	var ids IDs
	v, _ = IDs(nil).Value()
	if v != "[]" || ids.Scan(`["`+uuid.NewString()+`"]`) != nil || len(ids) != 1 || ids.Scan(1) == nil {
		t.Fatal("ids")
	}
	for _, m := range []interface{ TableName() string }{Plan{}, PlanVersion{}, Window{}, Settings{}, Dependant{}, LifeEvent{}, Election{}} {
		if !strings.HasPrefix(m.TableName(), "ben_") {
			t.Fatal(m.TableName())
		}
	}
	for _, m := range []interface{ BeforeCreate(*gorm.DB) error }{&Plan{}, &Window{}, &Dependant{}, &LifeEvent{}, &Election{}} {
		ok(t, m.BeforeCreate(nil))
	}
	if len(Catalogue()) != 3 || NewModule(nil, Ports{}).Service() == nil || len((&Module{}).RegisterModels()) != 7 || NewStore(nil).DB() != nil || orNotFound(errBoom) != errBoom {
		t.Fatal("module")
	}
	pv := PlanVersion{EffectiveFrom: Date{day(2026, 1, 1)}, EffectiveTo: &Date{day(2026, 7, 1)}}
	if !pv.In(day(2026, 6, 30)) || pv.In(day(2026, 7, 1)) || pv.In(day(2025, 12, 31)) {
		t.Fatal("version range is half-open")
	}
	if (Election{Status: ElWaived, EffectiveFrom: Date{day(2026, 1, 1)}}).CoversDay(day(2026, 6, 1)) {
		t.Fatal("a waiver covers nothing")
	}
	if !hasID([]uuid.UUID{uuid.Nil}, &uuid.Nil) || hasID(nil, nil) {
		t.Fatal("hasID")
	}
}

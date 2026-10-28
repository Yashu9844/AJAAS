package plus

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
	"github.com/jaas/jaas/internal/shared/identityport"
	"github.com/jaas/jaas/internal/hr/leave/calc"
	leaveDTO "github.com/jaas/jaas/internal/hr/leave/dto"
	leaveModels "github.com/jaas/jaas/internal/hr/leave/models"
	"github.com/jaas/jaas/internal/hr/leave/services"
	"github.com/jaas/jaas/internal/identity/access"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"gorm.io/gorm"
)

var errBoom = errors.New("boom")

// --- fakes ---

type fakeStore struct {
	policy      *Policy
	balances    bool
	types       map[uuid.UUID]bool // id â†’ active
	locHolidays map[uuid.UUID]LocationHoliday
	weekOffs    map[uuid.UUID]WeekOff
	tenantHols  []TenantHoliday
	requests    map[uuid.UUID]leaveModels.Request
	compOffs    map[uuid.UUID]CompOff
	rules       map[uuid.UUID]EncashmentRule
	encash      map[uuid.UUID]Encashment
	typeDays    []TypeDays
	events      []string
	fail        map[string]error
}

func newStore() *fakeStore {
	return &fakeStore{types: map[uuid.UUID]bool{}, locHolidays: map[uuid.UUID]LocationHoliday{}, weekOffs: map[uuid.UUID]WeekOff{},
		requests: map[uuid.UUID]leaveModels.Request{}, compOffs: map[uuid.UUID]CompOff{}, rules: map[uuid.UUID]EncashmentRule{},
		encash: map[uuid.UUID]Encashment{}, fail: map[string]error{}}
}

func (f *fakeStore) InTx(_ context.Context, fn func(*gorm.DB) error) error {
	co, en, lh, wo, ev := map[uuid.UUID]CompOff{}, map[uuid.UUID]Encashment{}, map[uuid.UUID]LocationHoliday{}, map[uuid.UUID]WeekOff{}, len(f.events)
	for k, v := range f.compOffs {
		co[k] = v
	}
	for k, v := range f.encash {
		en[k] = v
	}
	for k, v := range f.locHolidays {
		lh[k] = v
	}
	for k, v := range f.weekOffs {
		wo[k] = v
	}
	if err := fn(nil); err != nil {
		f.compOffs, f.encash, f.locHolidays, f.weekOffs, f.events = co, en, lh, wo, f.events[:ev]
		return err
	}
	return nil
}
func (f *fakeStore) DB() *gorm.DB { return nil }
func (f *fakeStore) Create(_ context.Context, _ *gorm.DB, row interface{}) error {
	if err := f.fail["create"]; err != nil {
		return err
	}
	switch v := row.(type) {
	case *LocationHoliday:
		_ = v.BeforeCreate(nil)
		f.locHolidays[v.ID] = *v
	case *CompOff:
		_ = v.BeforeCreate(nil)
		f.compOffs[v.ID] = *v
	case *Encashment:
		_ = v.BeforeCreate(nil)
		f.encash[v.ID] = *v
	}
	return nil
}
func (f *fakeStore) Save(_ context.Context, _ *gorm.DB, row interface{}) error {
	if err := f.fail["save"]; err != nil {
		return err
	}
	switch v := row.(type) {
	case *CompOff:
		f.compOffs[v.ID] = *v
	case *Encashment:
		f.encash[v.ID] = *v
	}
	return nil
}
func (f *fakeStore) Delete(_ context.Context, _ *gorm.DB, row interface{}) error {
	if err := f.fail["delete"]; err != nil {
		return err
	}
	switch v := row.(type) {
	case *LocationHoliday:
		delete(f.locHolidays, v.ID)
	case *WeekOff:
		delete(f.weekOffs, v.LocationID)
	}
	return nil
}
func (f *fakeStore) Policy(context.Context, *gorm.DB, uuid.UUID) (*Policy, error) {
	return f.policy, f.fail["policy"]
}
func (f *fakeStore) SavePolicy(_ context.Context, _ *gorm.DB, p *Policy) error {
	f.policy = p
	return f.fail["savepolicy"]
}
func (f *fakeStore) BalancesExist(context.Context, *gorm.DB, uuid.UUID) (bool, error) {
	return f.balances, f.fail["balances"]
}
func (f *fakeStore) TypeState(_ context.Context, _ *gorm.DB, _, id uuid.UUID) (bool, bool, error) {
	active, ok := f.types[id]
	return ok, active, f.fail["type"]
}
func (f *fakeStore) CompOffTenants(context.Context, *gorm.DB) ([]uuid.UUID, error) {
	if f.policy != nil && f.policy.CompOffLeaveTypeID != nil {
		return []uuid.UUID{f.policy.TenantID}, f.fail["tenants"]
	}
	return nil, f.fail["tenants"]
}
func (f *fakeStore) LocationHoliday(_ context.Context, _ *gorm.DB, _, id uuid.UUID) (*LocationHoliday, error) {
	if h, ok := f.locHolidays[id]; ok {
		return &h, f.fail["lh"]
	}
	return nil, f.fail["lh"]
}
func (f *fakeStore) LocationHolidays(_ context.Context, _ *gorm.DB, _ uuid.UUID, loc *uuid.UUID, from, to time.Time) ([]LocationHoliday, error) {
	var out []LocationHoliday
	for _, h := range f.locHolidays {
		if (loc == nil || h.LocationID == *loc) && !h.HolidayDate.Before(from) && !h.HolidayDate.After(to) {
			out = append(out, h)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].HolidayDate.Before(out[j].HolidayDate.Time) })
	return out, f.fail["lhs"]
}
func (f *fakeStore) HolidayTaken(_ context.Context, _ *gorm.DB, _, loc uuid.UUID, date time.Time) (bool, error) {
	for _, h := range f.locHolidays {
		if h.LocationID == loc && h.HolidayDate.Equal(date) {
			return true, f.fail["taken"]
		}
	}
	return false, f.fail["taken"]
}
func (f *fakeStore) WeekOff(_ context.Context, _ *gorm.DB, _, loc uuid.UUID) (*WeekOff, error) {
	if w, ok := f.weekOffs[loc]; ok {
		return &w, f.fail["weekoff"]
	}
	return nil, f.fail["weekoff"]
}
func (f *fakeStore) WeekOffs(context.Context, *gorm.DB, uuid.UUID) ([]WeekOff, error) {
	var out []WeekOff
	for _, w := range f.weekOffs {
		out = append(out, w)
	}
	return out, f.fail["weekoffs"]
}
func (f *fakeStore) SaveWeekOff(_ context.Context, _ *gorm.DB, w *WeekOff) error {
	if err := f.fail["saveweekoff"]; err != nil {
		return err
	}
	f.weekOffs[w.LocationID] = *w
	return nil
}
func (f *fakeStore) TenantHolidays(_ context.Context, _ *gorm.DB, _ uuid.UUID, from, to time.Time) ([]TenantHoliday, error) {
	var out []TenantHoliday
	for _, h := range f.tenantHols {
		if !h.Date.Before(from) && !h.Date.After(to) {
			out = append(out, h)
		}
	}
	return out, f.fail["tenantholidays"]
}
func (f *fakeStore) PendingRequests(_ context.Context, _ *gorm.DB, _ uuid.UUID, ids []uuid.UUID, all bool, _ int) ([]leaveModels.Request, error) {
	var out []leaveModels.Request
	for _, r := range f.requests {
		match := all
		for _, id := range ids {
			match = match || id == r.EmployeeProfileID
		}
		if match && r.Status == leaveModels.StatusPending {
			out = append(out, r)
		}
	}
	return out, f.fail["pending"]
}
func (f *fakeStore) Request(_ context.Context, _ *gorm.DB, _, id uuid.UUID) (*leaveModels.Request, error) {
	if r, ok := f.requests[id]; ok {
		return &r, f.fail["request"]
	}
	return nil, f.fail["request"]
}
func (f *fakeStore) RequestsOverlapping(_ context.Context, _ *gorm.DB, _ uuid.UUID, ids []uuid.UUID, all bool, from, to time.Time) ([]leaveModels.Request, error) {
	var out []leaveModels.Request
	for _, r := range f.requests {
		match := all
		for _, id := range ids {
			match = match || id == r.EmployeeProfileID
		}
		if match && (r.Status == leaveModels.StatusPending || r.Status == leaveModels.StatusApproved) && !r.StartDate.After(to) && !r.EndDate.Before(from) {
			out = append(out, r)
		}
	}
	return out, f.fail["overlapping"]
}
func (f *fakeStore) CompOff(_ context.Context, _ *gorm.DB, _, id uuid.UUID, _ bool) (*CompOff, error) {
	if c, ok := f.compOffs[id]; ok {
		return &c, f.fail["compoff"]
	}
	return nil, f.fail["compoff"]
}
func (f *fakeStore) CompOffOnDay(_ context.Context, _ *gorm.DB, _, emp uuid.UUID, date time.Time) (*CompOff, error) {
	for _, c := range f.compOffs {
		if c.EmployeeProfileID == emp && c.WorkedDate.Equal(date) && (c.Status == StatusPending || c.Status == StatusApproved) {
			return &c, f.fail["onday"]
		}
	}
	return nil, f.fail["onday"]
}
func (f *fakeStore) CompOffs(_ context.Context, _ *gorm.DB, _ uuid.UUID, fl CompOffFilter, _ int) ([]CompOff, error) {
	var out []CompOff
	for _, c := range f.compOffs {
		if (fl.EmployeeID == nil || c.EmployeeProfileID == *fl.EmployeeID) && (fl.Status == "" || c.Status == fl.Status) {
			out = append(out, c)
		}
	}
	return out, f.fail["compoffs"]
}
func (f *fakeStore) ExpiredCompOffs(_ context.Context, _ *gorm.DB, _ uuid.UUID, today time.Time) ([]CompOff, error) {
	var out []CompOff
	for _, c := range f.compOffs {
		if c.Status == StatusApproved && c.ExpiredAt == nil && c.ExpiresOn != nil && c.ExpiresOn.Before(today) {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ExpiresOn.Before(out[j].ExpiresOn.Time) })
	return out, f.fail["expired"]
}
func (f *fakeStore) EncashmentRule(_ context.Context, _ *gorm.DB, _, typ uuid.UUID) (*EncashmentRule, error) {
	if r, ok := f.rules[typ]; ok {
		return &r, f.fail["rule"]
	}
	return nil, f.fail["rule"]
}
func (f *fakeStore) EncashmentRules(context.Context, *gorm.DB, uuid.UUID) ([]EncashmentRule, error) {
	var out []EncashmentRule
	for _, r := range f.rules {
		out = append(out, r)
	}
	return out, f.fail["rules"]
}
func (f *fakeStore) SaveEncashmentRule(_ context.Context, _ *gorm.DB, r *EncashmentRule) error {
	f.rules[r.LeaveTypeID] = *r
	return f.fail["saverule"]
}
func (f *fakeStore) Encashment(_ context.Context, _ *gorm.DB, _, id uuid.UUID, _ bool) (*Encashment, error) {
	if e, ok := f.encash[id]; ok {
		return &e, f.fail["encashment"]
	}
	return nil, f.fail["encashment"]
}
func (f *fakeStore) PendingEncashment(_ context.Context, _ *gorm.DB, _, emp, typ uuid.UUID) (*Encashment, error) {
	for _, e := range f.encash {
		if e.EmployeeProfileID == emp && e.LeaveTypeID == typ && e.Status == StatusPending {
			return &e, f.fail["pendingenc"]
		}
	}
	return nil, f.fail["pendingenc"]
}
func (f *fakeStore) EncashedDays(_ context.Context, _ *gorm.DB, _, emp, typ uuid.UUID, year int, statuses ...string) (calc.Days, error) {
	var sum calc.Days
	for _, e := range f.encash {
		for _, s := range statuses {
			if e.EmployeeProfileID == emp && e.LeaveTypeID == typ && e.Year == year && e.Status == s {
				sum += e.Days
			}
		}
	}
	return sum, f.fail["encashed"]
}
func (f *fakeStore) Encashments(_ context.Context, _ *gorm.DB, _ uuid.UUID, fl EncashFilter, _ int) ([]Encashment, error) {
	var out []Encashment
	for _, e := range f.encash {
		if (fl.EmployeeID == nil || e.EmployeeProfileID == *fl.EmployeeID) && (fl.LeaveTypeID == nil || e.LeaveTypeID == *fl.LeaveTypeID) && (fl.Status == "" || e.Status == fl.Status) {
			out = append(out, e)
		}
	}
	return out, f.fail["encashments"]
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

type fakeReports struct {
	byManager map[uuid.UUID][]uuid.UUID
	err       error
}

func (r *fakeReports) DirectReportUserIDs(_ context.Context, _, m uuid.UUID) ([]uuid.UUID, error) {
	return r.byManager[m], r.err
}

type fakeLocations struct {
	byEmployee map[uuid.UUID]uuid.UUID
	err        error
}

func (l *fakeLocations) LocationOn(_ context.Context, _, e uuid.UUID, _ time.Time) (*uuid.UUID, error) {
	if id, ok := l.byEmployee[e]; ok {
		return &id, l.err
	}
	return nil, l.err
}

type fakeRequests struct {
	approved, rejected []uuid.UUID
	err                error
}

func (r *fakeRequests) Preview(context.Context, services.Actor, leaveDTO.ApplyLeaveRequest) (*leaveDTO.PreviewResponse, error) {
	return nil, nil
}
func (r *fakeRequests) Apply(context.Context, services.Actor, leaveDTO.ApplyLeaveRequest) (*leaveDTO.LeaveRequestResponse, error) {
	return nil, nil
}
func (r *fakeRequests) ListMine(context.Context, services.Actor, string, leaveDTO.Page) ([]leaveDTO.LeaveRequestResponse, leaveDTO.PageMeta, error) {
	return nil, leaveDTO.PageMeta{}, nil
}
func (r *fakeRequests) List(context.Context, uuid.UUID, services.RequestQuery, leaveDTO.Page) ([]leaveDTO.LeaveRequestResponse, leaveDTO.PageMeta, error) {
	return nil, leaveDTO.PageMeta{}, nil
}
func (r *fakeRequests) Get(context.Context, uuid.UUID, uuid.UUID) (*leaveDTO.LeaveRequestResponse, error) {
	return nil, nil
}
func (r *fakeRequests) Cancel(context.Context, services.Actor, uuid.UUID) (*leaveDTO.LeaveRequestResponse, error) {
	return nil, nil
}
func (r *fakeRequests) Approve(_ context.Context, _ services.Actor, id uuid.UUID, _ leaveDTO.ReviewRequest) (*leaveDTO.LeaveRequestResponse, error) {
	r.approved = append(r.approved, id)
	return &leaveDTO.LeaveRequestResponse{}, r.err
}
func (r *fakeRequests) Reject(_ context.Context, _ services.Actor, id uuid.UUID, _ leaveDTO.ReviewRequest) (*leaveDTO.LeaveRequestResponse, error) {
	r.rejected = append(r.rejected, id)
	return &leaveDTO.LeaveRequestResponse{}, r.err
}

type fakeMover struct {
	avail   map[[2]uuid.UUID]calc.Days
	adjusts []calc.Days
	notes   []string
	err     error
}

func (m *fakeMover) AvailableTx(_ context.Context, _ *gorm.DB, _ services.Actor, emp, typ uuid.UUID) (calc.Days, error) {
	return m.avail[[2]uuid.UUID{emp, typ}], m.err
}
func (m *fakeMover) AdjustTx(_ context.Context, _ *gorm.DB, _ services.Actor, emp, typ uuid.UUID, days calc.Days, note string) (calc.Days, error) {
	if m.err != nil {
		return 0, m.err
	}
	m.avail[[2]uuid.UUID{emp, typ}] += days
	m.adjusts, m.notes = append(m.adjusts, days), append(m.notes, note)
	return m.avail[[2]uuid.UUID{emp, typ}], nil
}

type fakePresence struct {
	minutes map[string]int
	err     error
}

func (p *fakePresence) WorkedMinutes(_ context.Context, _ *gorm.DB, _, e uuid.UUID, d time.Time) (int, error) {
	return p.minutes[e.String()+d.Format(DateLayout)], p.err
}

type noAudit struct{}

func (noAudit) Log(context.Context, *gorm.DB, string, string, string, string, string, interface{}, string, string) error {
	return nil
}

var _ identityport.Auditor = noAudit{}

// --- environment ---

type env struct {
	svc                  *Service
	store                *fakeStore
	emps                 *fakeEmployees
	reports              *fakeReports
	locs                 *fakeLocations
	reqs                 *fakeRequests
	mover                *fakeMover
	presence             *fakePresence
	tenant, loc, other   uuid.UUID
	compType, annualType uuid.UUID
	hr, boss, stranger   Actor
	alice, bob, bossE    uuid.UUID
}

// 2026-06-10 is a Wednesday.
var now = time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC)

func setup(t *testing.T) *env {
	t.Helper()
	e := &env{store: newStore(), tenant: uuid.New(), loc: uuid.New(), other: uuid.New(), compType: uuid.New(), annualType: uuid.New(),
		alice: uuid.New(), bob: uuid.New(), bossE: uuid.New()}
	aliceU, bobU, bossU := uuid.New(), uuid.New(), uuid.New()
	e.emps = &fakeEmployees{byID: map[uuid.UUID]employeeDTO.EmployeeResponse{
		e.alice: {ID: e.alice, UserID: aliceU, DisplayName: "Alice", Status: "active"},
		e.bob:   {ID: e.bob, UserID: bobU, DisplayName: "Bob", Status: "active"},
		e.bossE: {ID: e.bossE, UserID: bossU, DisplayName: "Boss", Status: "active"}}}
	e.reports = &fakeReports{byManager: map[uuid.UUID][]uuid.UUID{bossU: {aliceU}}}
	e.locs = &fakeLocations{byEmployee: map[uuid.UUID]uuid.UUID{e.alice: e.loc}}
	e.reqs = &fakeRequests{}
	e.mover = &fakeMover{avail: map[[2]uuid.UUID]calc.Days{}}
	e.presence = &fakePresence{minutes: map[string]int{}}
	e.store.types[e.compType], e.store.types[e.annualType] = true, true
	cal := NewCalendar(e.store)
	cal.SetLocations(e.locs)
	e.svc = NewService(e.store, Ports{Employees: e.emps, Reports: e.reports, Requests: e.reqs, Mover: e.mover, Presence: e.presence, Audit: noAudit{}}, cal,
		func() time.Time { return now })
	e.hr = Actor{TenantID: e.tenant, UserID: uuid.New(), CanRead: true, CanManage: true, CanApproveAll: true}
	e.boss = Actor{TenantID: e.tenant, UserID: bossU}
	e.stranger = Actor{TenantID: e.tenant, UserID: uuid.New()}
	return e
}

func (e *env) as(emp uuid.UUID) Actor {
	return Actor{TenantID: e.tenant, UserID: e.emps.byID[emp].UserID}
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

func mustErr(_ interface{}, err error) error { return err }

func str(s string) *string { return &s }

func date(s string) time.Time {
	d, _ := time.Parse(DateLayout, s)
	return d
}

// --- policy and calendar (LP-01..LP-05) ---

func TestPolicy(t *testing.T) {
	e := setup(t)
	if p, err := e.svc.GetPolicy(ctx(), e.hr); err != nil || p.YearStartMonth != 1 || p.CompOffExpiryDays != 90 {
		t.Fatalf("defaults: %+v %v", p, err)
	}
	typ := e.compType.String()
	for _, in := range []PolicyInput{{YearStartMonth: 0, CompOffExpiryDays: 90}, {YearStartMonth: 13, CompOffExpiryDays: 90}, {YearStartMonth: 4, CompOffExpiryDays: 0},
		{YearStartMonth: 4, CompOffExpiryDays: 366}, {YearStartMonth: 4, CompOffExpiryDays: 90, CompOffLeaveTypeID: str("x")}} {
		code(t, mustErr(e.svc.SetPolicy(ctx(), e.hr, in)), "VALIDATION_ERROR")
	}
	code(t, mustErr(e.svc.SetPolicy(ctx(), e.hr, PolicyInput{YearStartMonth: 4, CompOffExpiryDays: 90, CompOffLeaveTypeID: str(uuid.NewString())})), "NOT_FOUND")
	inactive := uuid.New()
	e.store.types[inactive] = false
	code(t, mustErr(e.svc.SetPolicy(ctx(), e.hr, PolicyInput{YearStartMonth: 4, CompOffExpiryDays: 90, CompOffLeaveTypeID: str(inactive.String())})), "INACTIVE_REFERENCE")
	p, err := e.svc.SetPolicy(ctx(), e.hr, PolicyInput{YearStartMonth: 4, CompOffExpiryDays: 60, CompOffLeaveTypeID: &typ})
	if err != nil || p.YearStartMonth != 4 || *p.CompOffLeaveTypeID != e.compType {
		t.Fatal(p, err)
	}
	// LP-01: the year cannot move once balances exist, but other fields can.
	e.store.balances = true
	code(t, mustErr(e.svc.SetPolicy(ctx(), e.hr, PolicyInput{YearStartMonth: 1, CompOffExpiryDays: 60})), "YEAR_LOCKED")
	if _, err := e.svc.SetPolicy(ctx(), e.hr, PolicyInput{YearStartMonth: 4, CompOffExpiryDays: 30}); err != nil {
		t.Fatal(err)
	}
	m, err := e.svc.cal.YearStartMonth(ctx(), nil, e.tenant)
	if err != nil || m != 4 {
		t.Fatal(m, err)
	}
	e.store.balances = false
	for _, k := range []string{"policy", "type", "balances", "savepolicy"} {
		e.store.fail[k] = errBoom
		in := PolicyInput{YearStartMonth: 7, CompOffExpiryDays: 30, CompOffLeaveTypeID: &typ}
		if _, err := e.svc.SetPolicy(ctx(), e.hr, in); !errors.Is(err, errBoom) {
			t.Fatal(k, err)
		}
		delete(e.store.fail, k)
	}
	e.store.fail["policy"] = errBoom
	if _, err := e.svc.GetPolicy(ctx(), e.hr); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	if _, err := e.svc.cal.YearStartMonth(ctx(), nil, e.tenant); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
}

func TestLocationCalendar(t *testing.T) {
	e := setup(t)
	for _, in := range []LocationHolidayInput{{LocationID: "x"}, {LocationID: e.loc.String(), Date: "x"}, {LocationID: e.loc.String(), Date: "2026-06-12", Name: " "},
		{LocationID: e.loc.String(), Date: "2026-06-12", Name: strings.Repeat("n", 101)}} {
		code(t, mustErr(e.svc.CreateLocationHoliday(ctx(), e.hr, in)), "VALIDATION_ERROR")
	}
	h, err := e.svc.CreateLocationHoliday(ctx(), e.hr, LocationHolidayInput{LocationID: e.loc.String(), Date: "2026-06-12", Name: " Local day "})
	if err != nil || h.Name != "Local day" {
		t.Fatal(h, err)
	}
	code(t, mustErr(e.svc.CreateLocationHoliday(ctx(), e.hr, LocationHolidayInput{LocationID: e.loc.String(), Date: "2026-06-12", Name: "Again"})), "CONFLICT")
	_, err = e.svc.CreateLocationHoliday(ctx(), e.hr, LocationHolidayInput{LocationID: e.loc.String(), Date: "2026-06-15", Name: "Optional", IsOptional: true})
	ok(t, err)
	e.store.fail["create"] = errors.New("ERROR: insert violates foreign key constraint")
	code(t, mustErr(e.svc.CreateLocationHoliday(ctx(), e.hr, LocationHolidayInput{LocationID: uuid.NewString(), Date: "2026-06-20", Name: "X"})), "LOCATION_NOT_FOUND")
	e.store.fail["create"] = errBoom
	if _, err := e.svc.CreateLocationHoliday(ctx(), e.hr, LocationHolidayInput{LocationID: e.loc.String(), Date: "2026-06-20", Name: "X"}); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	delete(e.store.fail, "create")
	e.store.fail["taken"] = errBoom
	if _, err := e.svc.CreateLocationHoliday(ctx(), e.hr, LocationHolidayInput{LocationID: e.loc.String(), Date: "2026-06-21", Name: "X"}); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	delete(e.store.fail, "taken")
	rows, err := e.svc.LocationHolidays(ctx(), e.hr, e.loc.String(), "2026")
	if err != nil || len(rows) != 2 {
		t.Fatal(rows, err)
	}
	if rows, _ := e.svc.LocationHolidays(ctx(), e.hr, "", ""); len(rows) != 2 {
		t.Fatal("defaults to the current year, all locations")
	}
	code(t, mustErr(e.svc.LocationHolidays(ctx(), e.hr, "x", "")), "VALIDATION_ERROR")
	code(t, mustErr(e.svc.LocationHolidays(ctx(), e.hr, "", "yy")), "VALIDATION_ERROR")

	// LG2 week-off pattern: Friday/Saturday for the location.
	code(t, mustErr(e.svc.SetWeekOff(ctx(), e.hr, "x", []int{5})), "VALIDATION_ERROR")
	code(t, mustErr(e.svc.SetWeekOff(ctx(), e.hr, e.loc.String(), []int{7})), "VALIDATION_ERROR")
	code(t, mustErr(e.svc.SetWeekOff(ctx(), e.hr, e.loc.String(), []int{1, 1})), "VALIDATION_ERROR")
	w, err := e.svc.SetWeekOff(ctx(), e.hr, e.loc.String(), []int{5, 6})
	if err != nil || len(w.Days) != 2 || w.Days[0] != 5 {
		t.Fatal(w, err)
	}
	e.store.fail["saveweekoff"] = errors.New("violates foreign key")
	code(t, mustErr(e.svc.SetWeekOff(ctx(), e.hr, uuid.NewString(), []int{1})), "LOCATION_NOT_FOUND")
	e.store.fail["saveweekoff"] = errBoom
	if _, err := e.svc.SetWeekOff(ctx(), e.hr, e.loc.String(), []int{1}); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	delete(e.store.fail, "saveweekoff")

	holidays, weekOff, err := e.svc.cal.For(ctx(), nil, e.tenant, e.alice, date("2026-06-08"), date("2026-06-20"))
	if err != nil || len(holidays) != 1 || holidays[0] != date("2026-06-12") || weekOff == nil || !weekOff(date("2026-06-12")) || weekOff(date("2026-06-13")) == false && false {
		t.Fatalf("For: %v %v", holidays, err)
	}
	if !weekOff(date("2026-06-13")) || weekOff(date("2026-06-14")) {
		t.Fatal("Saturday is off, Sunday works at this location")
	}
	if h, w, err := e.svc.cal.For(ctx(), nil, e.tenant, e.bob, date("2026-06-08"), date("2026-06-20")); h != nil || w != nil || err != nil {
		t.Fatal("a worker without a location uses the tenant calendar")
	}
	fresh := NewCalendar(e.store)
	if h, w, err := fresh.For(ctx(), nil, e.tenant, e.alice, date("2026-06-08"), date("2026-06-20")); h != nil || w != nil || err != nil {
		t.Fatal("no location source yet")
	}

	// DayKind: tenant holiday, location holiday, week-off verdict.
	e.store.tenantHols = []TenantHoliday{{Date: Date{date("2026-06-11")}, Name: "Tenant day"}, {Date: Date{date("2026-06-17")}, Name: "Optional", IsOptional: true}}
	hol, off, err := e.svc.cal.DayKind(ctx(), nil, e.tenant, e.alice, date("2026-06-11"))
	if err != nil || !hol || off == nil || *off {
		t.Fatal(hol, off, err)
	}
	if hol, off, _ := e.svc.cal.DayKind(ctx(), nil, e.tenant, e.alice, date("2026-06-12")); !hol || off == nil || !*off {
		t.Fatal("location holiday that is also a weekly off")
	}
	if hol, off, _ := e.svc.cal.DayKind(ctx(), nil, e.tenant, e.alice, date("2026-06-17")); hol || off == nil {
		t.Fatal("optional tenant holidays do not block")
	}
	if hol, off, _ := e.svc.cal.DayKind(ctx(), nil, e.tenant, e.bob, date("2026-06-11")); !hol || off != nil {
		t.Fatal("no location pattern means no verdict")
	}
	e.store.fail["tenantholidays"] = errBoom
	if _, _, err := e.svc.cal.DayKind(ctx(), nil, e.tenant, e.alice, date("2026-06-11")); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	delete(e.store.fail, "tenantholidays")
	e.locs.err = errBoom
	if _, _, err := e.svc.cal.DayKind(ctx(), nil, e.tenant, e.alice, date("2026-06-11")); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	e.locs.err = nil
	for _, k := range []string{"lhs", "weekoff"} {
		e.store.fail[k] = errBoom
		if _, _, err := e.svc.cal.For(ctx(), nil, e.tenant, e.alice, date("2026-06-08"), date("2026-06-20")); !errors.Is(err, errBoom) {
			t.Fatal(k, err)
		}
		delete(e.store.fail, k)
	}

	views, err := e.svc.WeekOffs(ctx(), e.hr)
	if err != nil || len(views) != 1 {
		t.Fatal(views, err)
	}
	code(t, e.svc.DeleteWeekOff(ctx(), e.hr, "x"), "VALIDATION_ERROR")
	code(t, e.svc.DeleteWeekOff(ctx(), e.hr, uuid.NewString()), "NOT_FOUND")
	ok(t, e.svc.DeleteWeekOff(ctx(), e.hr, e.loc.String()))
	e.store.fail["weekoffs"] = errBoom
	if _, err := e.svc.WeekOffs(ctx(), e.hr); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	code(t, e.svc.DeleteLocationHoliday(ctx(), e.hr, uuid.New()), "NOT_FOUND")
	ok(t, e.svc.DeleteLocationHoliday(ctx(), e.hr, h.ID))
	e.store.fail["delete"] = errBoom
	if err := e.svc.DeleteLocationHoliday(ctx(), e.hr, h.ID); err == nil {
		t.Fatal("deleted holiday is gone")
	}
}

// --- inbox and team calendar (LP-06, LP-07) ---

func (e *env) pendingLeave(emp uuid.UUID, start, end string) uuid.UUID {
	id := uuid.New()
	e.store.requests[id] = leaveModels.Request{ID: id, TenantID: e.tenant, EmployeeProfileID: emp, LeaveTypeID: e.annualType, StartDate: date(start), EndDate: date(end),
		TotalDays: calc.FromWhole(2), Reason: "private", Status: leaveModels.StatusPending}
	return id
}

func TestInboxAndCalendar(t *testing.T) {
	e := setup(t)
	mine, other := e.pendingLeave(e.alice, "2026-06-15", "2026-06-16"), e.pendingLeave(e.bob, "2026-06-15", "2026-06-16")
	items, err := e.svc.Inbox(ctx(), e.boss, false)
	if err != nil || len(items) != 1 || items[0].ID != mine || items[0].EmployeeName != "Alice" {
		t.Fatal(items, err)
	}
	if items, _ := e.svc.Inbox(ctx(), e.stranger, false); len(items) != 0 {
		t.Fatal("LG3 stranger inbox")
	}
	code(t, mustErr(e.svc.Inbox(ctx(), e.boss, true)), "FORBIDDEN")
	if all, err := e.svc.Inbox(ctx(), e.hr, true); err != nil || len(all) != 2 {
		t.Fatal(len(all), err)
	}
	ownHR := e.hr
	ownHR.UserID = e.emps.byID[e.alice].UserID
	if all, _ := e.svc.Inbox(ctx(), ownHR, true); len(all) != 1 {
		t.Fatal("own request hidden")
	}
	_, err = e.svc.Decide(ctx(), e.boss, mine, true, nil)
	ok(t, err)
	_, err = e.svc.Decide(ctx(), e.hr, other, false, str("no"))
	ok(t, err)
	code(t, mustErr(e.svc.Decide(ctx(), e.boss, other, true, nil)), "NOT_FOUND")
	code(t, mustErr(e.svc.Decide(ctx(), e.boss, uuid.New(), true, nil)), "NOT_FOUND")
	if len(e.reqs.approved) != 1 || len(e.reqs.rejected) != 1 {
		t.Fatal("decisions go through the request service")
	}
	e.reports.err = errBoom
	if _, err := e.svc.Inbox(ctx(), e.boss, false); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	if _, err := e.svc.Decide(ctx(), e.boss, mine, true, nil); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	e.reports.err = nil
	e.store.fail["pending"] = errBoom
	if _, err := e.svc.Inbox(ctx(), e.hr, true); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	delete(e.store.fail, "pending")
	e.emps.err = errBoom
	if _, err := e.svc.Inbox(ctx(), e.hr, true); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	e.emps.err = nil
	e.store.fail["request"] = errBoom
	if _, err := e.svc.Decide(ctx(), e.hr, mine, true, nil); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	delete(e.store.fail, "request")

	// Team calendar: reports only (all for leave:read), no reasons, tenant holidays.
	e.store.tenantHols = []TenantHoliday{{Date: Date{date("2026-06-17")}, Name: "Day"}}
	cal, err := e.svc.Calendar(ctx(), e.boss, "2026-06-14", "2026-06-20")
	if err != nil || len(cal.Leaves) != 1 || cal.Leaves[0].EmployeeName != "Alice" || len(cal.Holidays) != 1 {
		t.Fatalf("%+v %v", cal, err)
	}
	if cal, _ := e.svc.Calendar(ctx(), e.hr, "2026-06-14", "2026-06-20"); len(cal.Leaves) != 2 {
		t.Fatal("hr sees everyone")
	}
	if cal, _ := e.svc.Calendar(ctx(), e.stranger, "2026-06-14", "2026-06-20"); len(cal.Leaves) != 0 {
		t.Fatal("stranger sees nobody")
	}
	e.store.requests[mine] = func() leaveModels.Request {
		r := e.store.requests[mine]
		r.Status = leaveModels.StatusRejected
		return r
	}()
	if cal, _ := e.svc.Calendar(ctx(), e.boss, "2026-06-14", "2026-06-20"); len(cal.Leaves) != 0 {
		t.Fatal("rejected leave is never shown")
	}
	for _, c := range [][2]string{{"x", "2026-06-20"}, {"2026-06-14", "x"}, {"2026-06-20", "2026-06-14"}, {"2026-01-01", "2026-12-31"}} {
		if _, err := e.svc.Calendar(ctx(), e.boss, c[0], c[1]); err == nil {
			t.Fatal("range", c)
		}
	}
	e.store.fail["overlapping"] = errBoom
	if _, err := e.svc.Calendar(ctx(), e.hr, "2026-06-14", "2026-06-20"); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	delete(e.store.fail, "overlapping")
	e.store.fail["tenantholidays"] = errBoom
	if _, err := e.svc.Calendar(ctx(), e.hr, "2026-06-14", "2026-06-20"); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	delete(e.store.fail, "tenantholidays")
	e.reports.err = errBoom
	if _, err := e.svc.Calendar(ctx(), e.boss, "2026-06-14", "2026-06-20"); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
}

// --- comp-off (LP-08..LP-11) ---

func (e *env) configureCompOff(t *testing.T) {
	t.Helper()
	typ := e.compType.String()
	_, err := e.svc.SetPolicy(ctx(), e.hr, PolicyInput{YearStartMonth: 1, CompOffExpiryDays: 30, CompOffLeaveTypeID: &typ})
	ok(t, err)
}

func TestCompOff(t *testing.T) {
	e := setup(t)
	alice := e.as(e.alice)
	// 2026-06-06 is a Saturday: the default weekly off.
	in := CompOffInput{WorkedDate: "2026-06-06", Days: 100, Reason: "release"}
	code(t, mustErr(e.svc.RequestCompOff(ctx(), alice, in)), "COMP_OFF_NOT_CONFIGURED")
	e.configureCompOff(t)
	for _, bad := range []CompOffInput{{WorkedDate: "x", Days: 100, Reason: "r"}, {WorkedDate: "2026-06-06", Days: 75, Reason: "r"}, {WorkedDate: "2026-06-06", Days: 100, Reason: " "},
		{WorkedDate: "2026-06-11", Days: 100, Reason: "future"}, {WorkedDate: "2026-04-01", Days: 100, Reason: "old"}} {
		code(t, mustErr(e.svc.RequestCompOff(ctx(), alice, bad)), "VALIDATION_ERROR")
	}
	code(t, mustErr(e.svc.RequestCompOff(ctx(), e.stranger, in)), "EMPLOYEE_NOT_FOUND")
	inactive := e.emps.byID[e.bob]
	inactive.Status = "terminated"
	e.emps.byID[e.bob] = inactive
	code(t, mustErr(e.svc.RequestCompOff(ctx(), e.as(e.bob), in)), "EMPLOYEE_NOT_ACTIVE")
	code(t, mustErr(e.svc.RequestCompOff(ctx(), alice, CompOffInput{WorkedDate: "2026-06-09", Days: 100, Reason: "tuesday"})), "NOT_A_NON_WORKING_DAY")
	code(t, mustErr(e.svc.RequestCompOff(ctx(), alice, in)), "NO_WORK_RECORDED")
	e.presence.minutes[e.alice.String()+"2026-06-06"] = 300
	c, err := e.svc.RequestCompOff(ctx(), alice, in)
	if err != nil || c.Status != StatusPending {
		t.Fatal(c, err)
	}
	code(t, mustErr(e.svc.RequestCompOff(ctx(), alice, in)), "CONFLICT")

	// Decisions: stranger 404, own approval 403, manager approves and credits once.
	code(t, mustErr(e.svc.DecideCompOff(ctx(), e.stranger, c.ID, StatusApproved, nil)), "NOT_FOUND")
	code(t, mustErr(e.svc.DecideCompOff(ctx(), alice, c.ID, StatusApproved, nil)), "SELF_APPROVAL_FORBIDDEN")
	code(t, mustErr(e.svc.DecideCompOff(ctx(), e.boss, c.ID, StatusRejected, nil)), "VALIDATION_ERROR")
	code(t, mustErr(e.svc.DecideCompOff(ctx(), e.boss, uuid.New(), StatusApproved, nil)), "NOT_FOUND")
	code(t, mustErr(e.svc.DecideCompOff(ctx(), e.boss, c.ID, StatusApproved, str(strings.Repeat("c", 501)))), "VALIDATION_ERROR")
	done, err := e.svc.DecideCompOff(ctx(), e.boss, c.ID, StatusApproved, str("ok"))
	if err != nil || done.Status != StatusApproved || done.ExpiresOn.Format(DateLayout) != "2026-07-06" || len(e.mover.adjusts) != 1 || e.mover.adjusts[0] != 100 || e.mover.notes[0] != "comp-off" {
		t.Fatalf("approve: %+v %v", done, err)
	}
	if e.store.events[len(e.store.events)-1] != "leave.comp_off_credited" {
		t.Fatal("credit event")
	}
	code(t, mustErr(e.svc.DecideCompOff(ctx(), e.boss, c.ID, StatusApproved, nil)), "REQUEST_STATE")

	// Cancel and reject paths.
	e.presence.minutes[e.alice.String()+"2026-06-07"] = 120
	c2, err := e.svc.RequestCompOff(ctx(), alice, CompOffInput{WorkedDate: "2026-06-07", Days: 50, Reason: "half day"})
	ok(t, err)
	code(t, mustErr(e.svc.DecideCompOff(ctx(), e.boss, c2.ID, StatusCancelled, nil)), "NOT_FOUND")
	cancelled, err := e.svc.DecideCompOff(ctx(), alice, c2.ID, StatusCancelled, nil)
	if err != nil || cancelled.Status != StatusCancelled || cancelled.DecidedByUserID != nil {
		t.Fatal(cancelled, err)
	}
	c3, err := e.svc.RequestCompOff(ctx(), alice, CompOffInput{WorkedDate: "2026-06-07", Days: 50, Reason: "again"})
	ok(t, err)
	rej, err := e.svc.DecideCompOff(ctx(), e.hr, c3.ID, StatusRejected, str("no budget"))
	if err != nil || rej.Status != StatusRejected || len(e.mover.adjusts) != 1 {
		t.Fatal(rej, err)
	}

	// Lists.
	mine, err := e.svc.MyCompOffs(ctx(), alice)
	if err != nil || len(mine) != 3 {
		t.Fatal(len(mine), err)
	}
	code(t, mustErr(e.svc.MyCompOffs(ctx(), e.stranger)), "EMPLOYEE_NOT_FOUND")
	if rows, err := e.svc.CompOffs(ctx(), e.hr, &e.alice, StatusApproved); err != nil || len(rows) != 1 {
		t.Fatal(rows, err)
	}
	code(t, mustErr(e.svc.CompOffs(ctx(), e.hr, nil, "weird")), "VALIDATION_ERROR")

	// Failures roll the decision back.
	e.presence.minutes[e.alice.String()+"2026-05-30"] = 60
	c4, err := e.svc.RequestCompOff(ctx(), alice, CompOffInput{WorkedDate: "2026-05-30", Days: 100, Reason: "x"})
	ok(t, err)
	e.mover.err = errBoom
	if _, err := e.svc.DecideCompOff(ctx(), e.boss, c4.ID, StatusApproved, nil); !errors.Is(err, errBoom) || e.store.compOffs[c4.ID].Status != StatusPending {
		t.Fatal("LP-18 rollback", err)
	}
	e.mover.err = nil
	for _, k := range []string{"event", "save", "compoff"} {
		e.store.fail[k] = errBoom
		if _, err := e.svc.DecideCompOff(ctx(), e.boss, c4.ID, StatusApproved, nil); !errors.Is(err, errBoom) {
			t.Fatal(k, err)
		}
		delete(e.store.fail, k)
	}
	e.reports.err = errBoom
	if _, err := e.svc.DecideCompOff(ctx(), e.boss, c4.ID, StatusApproved, nil); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	e.reports.err = nil
	e.presence.minutes[e.alice.String()+"2026-05-31"] = 60
	for _, k := range []string{"policy", "create", "onday", "tenantholidays"} {
		e.store.fail[k] = errBoom
		if _, err := e.svc.RequestCompOff(ctx(), alice, CompOffInput{WorkedDate: "2026-05-31", Days: 100, Reason: "x"}); !errors.Is(err, errBoom) {
			t.Fatal(k, err)
		}
		delete(e.store.fail, k)
	}
	e.presence.err = errBoom
	if _, err := e.svc.RequestCompOff(ctx(), alice, CompOffInput{WorkedDate: "2026-05-31", Days: 100, Reason: "x"}); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	e.presence.err = nil
	e.emps.err = errBoom
	if _, err := e.svc.RequestCompOff(ctx(), alice, CompOffInput{WorkedDate: "2026-05-31", Days: 100, Reason: "x"}); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	e.emps.err = nil
	e.store.fail["compoffs"] = errBoom
	if _, err := e.svc.CompOffs(ctx(), e.hr, nil, ""); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
}

func TestCompOffUsesLocationCalendar(t *testing.T) {
	e := setup(t)
	e.configureCompOff(t)
	alice := e.as(e.alice)
	_, err := e.svc.SetWeekOff(ctx(), e.hr, e.loc.String(), []int{5}) // Friday only at this location
	ok(t, err)
	e.presence.minutes[e.alice.String()+"2026-06-06"] = 200 // Saturday works at this location
	e.presence.minutes[e.alice.String()+"2026-06-05"] = 200 // Friday is off
	code(t, mustErr(e.svc.RequestCompOff(ctx(), alice, CompOffInput{WorkedDate: "2026-06-06", Days: 100, Reason: "r"})), "NOT_A_NON_WORKING_DAY")
	if _, err := e.svc.RequestCompOff(ctx(), alice, CompOffInput{WorkedDate: "2026-06-05", Days: 100, Reason: "r"}); err != nil {
		t.Fatal(err)
	}
	e.store.tenantHols = []TenantHoliday{{Date: Date{date("2026-06-04")}, Name: "Tenant"}}
	e.presence.minutes[e.alice.String()+"2026-06-04"] = 60
	if _, err := e.svc.RequestCompOff(ctx(), alice, CompOffInput{WorkedDate: "2026-06-04", Days: 50, Reason: "r"}); err != nil {
		t.Fatal(err)
	}
	_, err = e.svc.CreateLocationHoliday(ctx(), e.hr, LocationHolidayInput{LocationID: e.loc.String(), Date: "2026-06-03", Name: "Local"})
	ok(t, err)
	e.presence.minutes[e.alice.String()+"2026-06-03"] = 60
	if _, err := e.svc.RequestCompOff(ctx(), alice, CompOffInput{WorkedDate: "2026-06-03", Days: 50, Reason: "r"}); err != nil {
		t.Fatal(err)
	}
}

func TestCompOffExpiry(t *testing.T) {
	e := setup(t)
	e.configureCompOff(t)
	alice := e.as(e.alice)
	mk := func(worked string, days calc.Days) *CompOff {
		e.presence.minutes[e.alice.String()+worked] = 100
		c, err := e.svc.RequestCompOff(ctx(), alice, CompOffInput{WorkedDate: worked, Days: days, Reason: "w"})
		ok(t, err)
		c, err = e.svc.DecideCompOff(ctx(), e.boss, c.ID, StatusApproved, nil)
		ok(t, err)
		return c
	}
	// Policy expiry is 30 days; fix the clocks by editing expires_on directly.
	old, recent := mk("2026-06-06", 100), mk("2026-06-07", 50)
	for id, past := range map[uuid.UUID]string{old.ID: "2026-06-01", recent.ID: "2026-06-05"} {
		c := e.store.compOffs[id]
		c.ExpiresOn = &Date{date(past)}
		e.store.compOffs[id] = c
	}
	key := [2]uuid.UUID{e.alice, e.compType}
	if e.mover.avail[key] != 150 {
		t.Fatal("credited 1.5 days")
	}
	e.mover.avail[key] = 30 // part of the balance was already used
	res, err := e.svc.Expire(ctx(), e.hr)
	if err != nil || res.Expired != 2 || res.Days != 30 || e.mover.avail[key] != 0 {
		t.Fatalf("LG4 expiry never goes negative: %+v %v avail=%v", res, err, e.mover.avail[key])
	}
	if e.store.compOffs[old.ID].ExpiredDays == nil || *e.store.compOffs[old.ID].ExpiredDays != 30 || *e.store.compOffs[recent.ID].ExpiredDays != 0 {
		t.Fatal("debited days recorded per credit")
	}
	if res, _ := e.svc.Expire(ctx(), e.hr); res.Expired != 0 {
		t.Fatal("idempotent")
	}
	// Sweep and Run.
	fresh := mk("2026-05-30", 100)
	c := e.store.compOffs[fresh.ID]
	c.ExpiresOn = &Date{date("2026-06-02")}
	e.store.compOffs[fresh.ID] = c
	if n, err := e.svc.Sweep(ctx()); err != nil || n != 1 {
		t.Fatal(n, err)
	}
	e.store.fail["tenants"] = errBoom
	if _, err := e.svc.Sweep(ctx()); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	cx, cancel := context.WithCancel(ctx())
	var seen error
	done := make(chan struct{})
	go func() { e.svc.Run(cx, 0, func(err error) { seen = err; cancel() }); close(done) }()
	<-done
	if !errors.Is(seen, errBoom) {
		t.Fatal("Run reports sweep errors", seen)
	}
	delete(e.store.fail, "tenants")

	// Failures.
	third := mk("2026-05-31", 100)
	c = e.store.compOffs[third.ID]
	c.ExpiresOn = &Date{date("2026-06-02")}
	e.store.compOffs[third.ID] = c
	e.mover.err = errBoom
	if _, err := e.svc.ExpireCompOffs(ctx(), e.tenant, uuid.Nil); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	e.mover.err = nil
	e.store.fail["expired"] = errBoom
	if _, err := e.svc.ExpireCompOffs(ctx(), e.tenant, uuid.Nil); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	delete(e.store.fail, "expired")
	e.store.fail["policy"] = errBoom
	if _, err := e.svc.ExpireCompOffs(ctx(), e.tenant, uuid.Nil); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	delete(e.store.fail, "policy")
	e.store.policy.CompOffLeaveTypeID = nil
	if res, err := e.svc.ExpireCompOffs(ctx(), e.tenant, uuid.Nil); err != nil || res.Expired != 0 {
		t.Fatal("no comp-off type means nothing to expire")
	}
}

// --- encashment (LP-12..LP-15) ---

func (e *env) rule(t *testing.T, max, min calc.Days) {
	t.Helper()
	_, err := e.svc.SetRule(ctx(), e.hr, e.annualType.String(), RuleInput{MaxDaysPerYear: max, MinBalanceAfter: min})
	ok(t, err)
}

func TestEncashment(t *testing.T) {
	e := setup(t)
	alice := e.as(e.alice)
	key := [2]uuid.UUID{e.alice, e.annualType}
	e.mover.avail[key] = calc.FromWhole(8)

	for _, in := range []struct {
		typ string
		r   RuleInput
	}{{"x", RuleInput{}}, {e.annualType.String(), RuleInput{MaxDaysPerYear: 75}}, {e.annualType.String(), RuleInput{MaxDaysPerYear: -50}},
		{e.annualType.String(), RuleInput{MaxDaysPerYear: 100, MinBalanceAfter: -1}}} {
		code(t, mustErr(e.svc.SetRule(ctx(), e.hr, in.typ, in.r)), "VALIDATION_ERROR")
	}
	code(t, mustErr(e.svc.SetRule(ctx(), e.hr, uuid.NewString(), RuleInput{})), "NOT_FOUND")
	req := EncashInput{LeaveTypeID: e.annualType.String(), Days: calc.FromWhole(3), Reason: str("medical")}
	code(t, mustErr(e.svc.RequestEncashment(ctx(), alice, req)), "NOT_ENCASHABLE")
	e.rule(t, calc.FromWhole(5), calc.FromWhole(4))

	for _, bad := range []EncashInput{{LeaveTypeID: "x", Days: 100}, {LeaveTypeID: e.annualType.String(), Days: 0}, {LeaveTypeID: e.annualType.String(), Days: 75},
		{LeaveTypeID: e.annualType.String(), Days: 100, Reason: str(strings.Repeat("r", 501))}} {
		code(t, mustErr(e.svc.RequestEncashment(ctx(), alice, bad)), "VALIDATION_ERROR")
	}
	code(t, mustErr(e.svc.RequestEncashment(ctx(), alice, EncashInput{LeaveTypeID: uuid.NewString(), Days: 100})), "NOT_FOUND")
	dormant := uuid.New()
	e.store.types[dormant] = false
	code(t, mustErr(e.svc.RequestEncashment(ctx(), alice, EncashInput{LeaveTypeID: dormant.String(), Days: 100})), "INACTIVE_REFERENCE")
	code(t, mustErr(e.svc.RequestEncashment(ctx(), e.stranger, req)), "EMPLOYEE_NOT_FOUND")

	// LG5: cap of 5 days a year and 4 days must remain.
	code(t, mustErr(e.svc.RequestEncashment(ctx(), alice, EncashInput{LeaveTypeID: e.annualType.String(), Days: calc.FromWhole(6)})), "ENCASH_LIMIT")
	code(t, mustErr(e.svc.RequestEncashment(ctx(), alice, EncashInput{LeaveTypeID: e.annualType.String(), Days: calc.FromWhole(5) + 50})), "ENCASH_LIMIT")
	code(t, mustErr(e.svc.RequestEncashment(ctx(), alice, EncashInput{LeaveTypeID: e.annualType.String(), Days: calc.FromWhole(5)})), "INSUFFICIENT_BALANCE")
	enc, err := e.svc.RequestEncashment(ctx(), alice, req)
	if err != nil || enc.Status != StatusPending || enc.Year != 2026 {
		t.Fatal(enc, err)
	}
	code(t, mustErr(e.svc.RequestEncashment(ctx(), alice, EncashInput{LeaveTypeID: e.annualType.String(), Days: 50})), "CONFLICT")

	// Decisions.
	code(t, mustErr(e.svc.DecideEncashment(ctx(), e.stranger, enc.ID, StatusApproved, nil)), "NOT_FOUND")
	code(t, mustErr(e.svc.DecideEncashment(ctx(), alice, enc.ID, StatusApproved, nil)), "SELF_APPROVAL_FORBIDDEN")
	code(t, mustErr(e.svc.DecideEncashment(ctx(), e.boss, enc.ID, StatusRejected, nil)), "VALIDATION_ERROR")
	code(t, mustErr(e.svc.DecideEncashment(ctx(), e.boss, uuid.New(), StatusApproved, nil)), "NOT_FOUND")
	// The balance drops below the minimum before approval: refused, nothing debited.
	e.mover.avail[key] = calc.FromWhole(6)
	code(t, mustErr(e.svc.DecideEncashment(ctx(), e.boss, enc.ID, StatusApproved, nil)), "INSUFFICIENT_BALANCE")
	if len(e.mover.adjusts) != 0 || e.store.encash[enc.ID].Status != StatusPending {
		t.Fatal("refused approval changes nothing")
	}
	e.mover.avail[key] = calc.FromWhole(10)
	done, err := e.svc.DecideEncashment(ctx(), e.boss, enc.ID, StatusApproved, str("ok"))
	if err != nil || done.Status != StatusApproved || len(e.mover.adjusts) != 1 || e.mover.adjusts[0] != -calc.FromWhole(3) || e.mover.notes[0] != "encashment" || e.mover.avail[key] != calc.FromWhole(7) {
		t.Fatalf("LG5 debit once: %+v %v %v", done, err, e.mover.adjusts)
	}
	if e.store.events[len(e.store.events)-1] != "leave.encashment_approved" {
		t.Fatal("event")
	}
	code(t, mustErr(e.svc.DecideEncashment(ctx(), e.boss, enc.ID, StatusApproved, nil)), "REQUEST_STATE")
	// The remaining annual allowance is 2 days: 3 more is refused, 2 is fine.
	code(t, mustErr(e.svc.RequestEncashment(ctx(), alice, EncashInput{LeaveTypeID: e.annualType.String(), Days: calc.FromWhole(3)})), "ENCASH_LIMIT")
	two, err := e.svc.RequestEncashment(ctx(), alice, EncashInput{LeaveTypeID: e.annualType.String(), Days: calc.FromWhole(2)})
	ok(t, err)
	cancelled, err := e.svc.DecideEncashment(ctx(), alice, two.ID, StatusCancelled, nil)
	if err != nil || cancelled.Status != StatusCancelled {
		t.Fatal(cancelled, err)
	}
	code(t, mustErr(e.svc.DecideEncashment(ctx(), alice, two.ID, StatusCancelled, nil)), "REQUEST_STATE")
	three, err := e.svc.RequestEncashment(ctx(), alice, EncashInput{LeaveTypeID: e.annualType.String(), Days: 100})
	ok(t, err)
	code(t, mustErr(e.svc.DecideEncashment(ctx(), e.boss, three.ID, StatusCancelled, nil)), "NOT_FOUND")
	if rej, err := e.svc.DecideEncashment(ctx(), e.hr, three.ID, StatusRejected, str("not now")); err != nil || rej.Status != StatusRejected {
		t.Fatal(rej, err)
	}

	// Lists and rules.
	if rows, err := e.svc.MyEncashments(ctx(), alice); err != nil || len(rows) != 3 {
		t.Fatal(len(rows), err)
	}
	code(t, mustErr(e.svc.MyEncashments(ctx(), e.stranger)), "EMPLOYEE_NOT_FOUND")
	if rows, _ := e.svc.Encashments(ctx(), e.hr, &e.alice, &e.annualType, StatusApproved); len(rows) != 1 {
		t.Fatal("filters")
	}
	code(t, mustErr(e.svc.Encashments(ctx(), e.hr, nil, nil, "weird")), "VALIDATION_ERROR")
	if rules, err := e.svc.Rules(ctx(), e.hr); err != nil || len(rules) != 1 {
		t.Fatal(rules, err)
	}

	// Failures.
	e.store.fail["rule"] = errBoom
	if _, err := e.svc.RequestEncashment(ctx(), alice, EncashInput{LeaveTypeID: e.annualType.String(), Days: 50}); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	delete(e.store.fail, "rule")
	for _, k := range []string{"encashed", "pendingenc", "create", "type"} {
		e.store.fail[k] = errBoom
		if _, err := e.svc.RequestEncashment(ctx(), alice, EncashInput{LeaveTypeID: e.annualType.String(), Days: 50}); !errors.Is(err, errBoom) {
			t.Fatal(k, err)
		}
		delete(e.store.fail, k)
	}
	e.mover.err = errBoom
	if _, err := e.svc.RequestEncashment(ctx(), alice, EncashInput{LeaveTypeID: e.annualType.String(), Days: 50}); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	e.mover.err = nil
	last, err := e.svc.RequestEncashment(ctx(), alice, EncashInput{LeaveTypeID: e.annualType.String(), Days: 50})
	ok(t, err)
	e.store.fail["event"] = errBoom
	if _, err := e.svc.DecideEncashment(ctx(), e.boss, last.ID, StatusApproved, nil); !errors.Is(err, errBoom) || e.store.encash[last.ID].Status != StatusPending {
		t.Fatal("LP-18 rollback", err)
	}
	delete(e.store.fail, "event")
	for _, k := range []string{"saverule", "encashments", "rules"} {
		e.store.fail[k] = errBoom
		var err error
		switch k {
		case "saverule":
			_, err = e.svc.SetRule(ctx(), e.hr, e.annualType.String(), RuleInput{})
		case "encashments":
			_, err = e.svc.Encashments(ctx(), e.hr, nil, nil, "")
		default:
			_, err = e.svc.Rules(ctx(), e.hr)
		}
		if !errors.Is(err, errBoom) {
			t.Fatal(k, err)
		}
		delete(e.store.fail, k)
	}
}

func TestModelsAndHelpers(t *testing.T) {
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
	for _, m := range []interface{ TableName() string }{Policy{}, LocationHoliday{}, WeekOff{}, CompOff{}, EncashmentRule{}, Encashment{}} {
		if !strings.HasPrefix(m.TableName(), "leave_") {
			t.Fatal(m.TableName())
		}
	}
	for _, m := range []interface{ BeforeCreate(*gorm.DB) error }{&LocationHoliday{}, &CompOff{}, &Encashment{}} {
		ok(t, m.BeforeCreate(nil))
	}
	if w := (WeekOff{WeekOffMask: 0b1100001}); len(w.days()) != 3 {
		t.Fatal("mask days")
	}
	m := NewModule(nil, NewCalendarFor(nil), Ports{})
	if len(m.RegisterModels()) != 6 || m.Service() == nil || NewStore(nil).DB() != nil || orNotFound(errBoom) != errBoom {
		t.Fatal("module")
	}
	if a := (Actor{TenantID: uuid.New()}).leaveActor(); a.TenantID == uuid.Nil || a.CorrelationID == uuid.Nil {
		t.Fatal("leave actor")
	}
}

// --- HTTP edge ---

type guards struct{ deny map[string]bool }

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
func (g guards) Flag(_, action, key string) gin.HandlerFunc {
	return func(c *gin.Context) { c.Set(key, !g.deny[action]); c.Next() }
}
func (g guards) AuditTrail(string, string) gin.HandlerFunc { return func(c *gin.Context) { c.Next() } }

var _ identityport.HTTP = guards{}

func router(e *env, user uuid.UUID, deny ...string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		if user != uuid.Nil {
			c.Set("tenant_id", e.tenant)
			c.Set("user_id", user)
		}
	})
	g := guards{deny: map[string]bool{}}
	for _, d := range deny {
		g.deny[d] = true
	}
	(&Module{svc: e.svc, cal: e.svc.cal}).RegisterRoutes(r.Group(""), g)
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
	hr := router(e, e.hr.UserID)
	alice := router(e, e.emps.byID[e.alice].UserID, "read", "manage", "approve")
	boss := router(e, e.boss.UserID, "read", "manage", "approve")
	const B = "/leave"
	expect := func(w *httptest.ResponseRecorder, status int, label string) string {
		t.Helper()
		if w.Code != status {
			t.Fatalf("%s: %d %s", label, w.Code, w.Body.String())
		}
		return w.Body.String()
	}
	idOf := func(body string) string { return body[strings.Index(body, `"id":"`)+6:][:36] }
	expect(do(router(e, uuid.Nil), "GET", B+"/inbox", ""), 401, "unauthenticated")

	expect(do(hr, "PUT", B+"/policy", `{"year_start_month":4,"comp_off_expiry_days":60,"comp_off_leave_type_id":"`+e.compType.String()+`"}`), 200, "policy")
	expect(do(hr, "PUT", B+"/policy", "{"), 400, "bad policy")
	expect(do(hr, "GET", B+"/policy", ""), 200, "get policy")
	expect(do(alice, "PUT", B+"/policy", `{}`), 403, "worker cannot set policy")

	out := expect(do(hr, "POST", B+"/location-holidays", `{"location_id":"`+e.loc.String()+`","date":"2026-06-12","name":"Local"}`), 201, "location holiday")
	hid := idOf(out)
	expect(do(hr, "POST", B+"/location-holidays", "{"), 400, "bad holiday")
	expect(do(hr, "GET", B+"/location-holidays?location_id="+e.loc.String()+"&year=2026", ""), 200, "holidays")
	expect(do(hr, "DELETE", B+"/location-holidays/x", ""), 400, "bad id")
	expect(do(hr, "DELETE", B+"/location-holidays/"+hid, ""), 204, "delete holiday")
	expect(do(hr, "PUT", B+"/week-offs/"+e.loc.String(), `{"week_off_days":[5,6]}`), 200, "week off")
	expect(do(hr, "PUT", B+"/week-offs/"+e.loc.String(), "{"), 400, "bad week off")
	expect(do(hr, "GET", B+"/week-offs", ""), 200, "week offs")
	expect(do(hr, "DELETE", B+"/week-offs/"+e.loc.String(), ""), 204, "delete week off")
	expect(do(hr, "DELETE", B+"/week-offs/"+uuid.NewString(), ""), 404, "unknown week off")

	pending := e.pendingLeave(e.alice, "2026-06-15", "2026-06-16")
	expect(do(boss, "GET", B+"/inbox", ""), 200, "inbox")
	expect(do(boss, "GET", B+"/inbox?scope=all", ""), 403, "scope all")
	expect(do(boss, "POST", B+"/inbox/"+pending.String()+"/approve", `{"comment":"ok"}`), 200, "approve")
	expect(do(boss, "POST", B+"/inbox/"+pending.String()+"/reject", "{"), 400, "bad body")
	expect(do(boss, "POST", B+"/inbox/x/reject", ""), 400, "bad id")
	expect(do(boss, "GET", B+"/team-calendar?from=2026-06-14&to=2026-06-20", ""), 200, "calendar")
	expect(do(boss, "GET", B+"/team-calendar?from=x&to=2026-06-20", ""), 400, "bad calendar")

	e.presence.minutes[e.alice.String()+"2026-06-06"] = 120
	out = expect(do(alice, "POST", B+"/comp-off", `{"worked_date":"2026-06-06","days":1,"reason":"release"}`), 201, "comp-off")
	cid := idOf(out)
	expect(do(alice, "POST", B+"/comp-off", "{"), 400, "bad comp-off")
	expect(do(alice, "GET", B+"/comp-off/me", ""), 200, "my comp-offs")
	expect(do(hr, "GET", B+"/comp-off?employee_id="+e.alice.String()+"&status=pending", ""), 200, "comp-offs")
	expect(do(hr, "GET", B+"/comp-off?employee_id=x", ""), 400, "bad filter")
	expect(do(alice, "GET", B+"/comp-off", ""), 403, "worker cannot list all")
	expect(do(alice, "POST", B+"/comp-off/"+cid+"/approve", ""), 403, "self approval")
	expect(do(boss, "POST", B+"/comp-off/"+cid+"/approve", `{"comment":"ok"}`), 200, "approve comp-off")
	expect(do(boss, "POST", B+"/comp-off/"+cid+"/reject", "{"), 400, "bad body")
	expect(do(hr, "POST", B+"/comp-off/"+cid+"/cancel", ""), 404, "cancel others")
	expect(do(hr, "POST", B+"/comp-off/expire", ""), 200, "expire")
	expect(do(alice, "POST", B+"/comp-off/expire", ""), 403, "worker cannot expire")

	expect(do(hr, "PUT", B+"/encashment-rules/"+e.annualType.String(), `{"max_days_per_year":5,"min_balance_after":0}`), 200, "rule")
	expect(do(hr, "PUT", B+"/encashment-rules/"+e.annualType.String(), "{"), 400, "bad rule")
	expect(do(hr, "GET", B+"/encashment-rules", ""), 200, "rules")
	e.mover.avail[[2]uuid.UUID{e.alice, e.annualType}] = calc.FromWhole(8)
	out = expect(do(alice, "POST", B+"/encashments", `{"leave_type_id":"`+e.annualType.String()+`","days":2,"reason":"cash"}`), 201, "encashment")
	eid := idOf(out)
	expect(do(alice, "POST", B+"/encashments", "{"), 400, "bad encashment")
	expect(do(alice, "GET", B+"/encashments/me", ""), 200, "my encashments")
	expect(do(hr, "GET", B+"/encashments?employee_id="+e.alice.String()+"&leave_type_id="+e.annualType.String()+"&status=pending", ""), 200, "encashments")
	expect(do(hr, "GET", B+"/encashments?leave_type_id=x", ""), 400, "bad filter")
	expect(do(boss, "POST", B+"/encashments/"+eid+"/approve", `{"comment":"ok"}`), 200, "approve encashment")
	expect(do(boss, "POST", B+"/encashments/"+eid+"/cancel", ""), 404, "manager cancels")
	expect(do(boss, "POST", B+"/encashments/"+eid+"/reject", "{"), 400, "bad body")
	e.store.fail["rules"] = errBoom
	expect(do(hr, "GET", B+"/encashment-rules", ""), 500, "internal")
}

type fakeCoreDir struct {
	place *coreAPI.Placement
	err   error
}

func (f fakeCoreDir) PlacementOn(context.Context, uuid.UUID, uuid.UUID, time.Time) (*coreAPI.Placement, error) {
	return f.place, f.err
}
func (f fakeCoreDir) GradeState(context.Context, uuid.UUID, uuid.UUID) (bool, bool, error) {
	return true, true, nil
}

func TestFromCoreAndEdges(t *testing.T) {
	loc := uuid.New()
	l := FromCore(fakeCoreDir{place: &coreAPI.Placement{LocationID: &loc}})
	if got, err := l.LocationOn(ctx(), uuid.New(), uuid.New(), now); err != nil || got == nil || *got != loc {
		t.Fatal(got, err)
	}
	if got, _ := FromCore(fakeCoreDir{}).LocationOn(ctx(), uuid.New(), uuid.New(), now); got != nil {
		t.Fatal("no placement means no location")
	}
	if _, err := FromCore(fakeCoreDir{err: errBoom}).LocationOn(ctx(), uuid.New(), uuid.New(), now); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}

	e := setup(t)
	// A request whose worker profile is gone still lists, with an empty name.
	id := e.pendingLeave(uuid.New(), "2026-06-15", "2026-06-16")
	items, err := e.svc.Inbox(ctx(), e.hr, true)
	if err != nil || len(items) != 1 || items[0].ID != id || items[0].EmployeeName != "" {
		t.Fatal(items, err)
	}
	e.store.fail["policy"] = errBoom
	e.presence.minutes[e.alice.String()+"2026-06-06"] = 10
	if _, err := e.svc.RequestEncashment(ctx(), e.as(e.alice), EncashInput{LeaveTypeID: e.annualType.String(), Days: 50}); !errors.Is(err, errBoom) {
		t.Fatal("year lookup failure surfaces", err)
	}
	delete(e.store.fail, "policy")
	e.store.fail["compoffs"] = errBoom
	if _, err := e.svc.MyCompOffs(ctx(), e.as(e.alice)); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	delete(e.store.fail, "compoffs")
	e.emps.err = errBoom
	for _, f := range []func() error{
		func() error { return mustErr(e.svc.MyCompOffs(ctx(), e.as(e.alice))) },
		func() error { return mustErr(e.svc.MyEncashments(ctx(), e.as(e.alice))) },
		func() error { return mustErr(e.svc.DecideEncashment(ctx(), e.boss, uuid.New(), StatusApproved, nil)) },
	} {
		if err := f(); !errors.Is(err, errBoom) {
			t.Fatal(err)
		}
	}
	e.emps.err = nil
	e.store.policy = nil
	if _, err := e.svc.Expire(ctx(), e.hr); err != nil {
		t.Fatal(err)
	}
	e.store.fail["policy"] = errBoom
	if _, err := e.svc.Expire(ctx(), e.hr); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	m := &Module{svc: e.svc, cal: e.svc.cal}
	c, cancel := context.WithCancel(ctx())
	cancel()
	m.RunCompOffExpiry(c, nil)
}

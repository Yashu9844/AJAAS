package ops

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
	attDTO "github.com/jaas/jaas/internal/hr/attendance/dto"
	attModels "github.com/jaas/jaas/internal/hr/attendance/models"
	attServices "github.com/jaas/jaas/internal/hr/attendance/services"
	employeeDTO "github.com/jaas/jaas/internal/hr/employee/dto"
	"github.com/jaas/jaas/internal/identity/access"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"github.com/jaas/jaas/internal/shared/identityport"
	"gorm.io/gorm"
)

var errBoom = errors.New("boom")

// --- fakes ---

type fakeStore struct {
	policy  *Policy
	rosters map[uuid.UUID]Roster
	regs    map[uuid.UUID]attModels.Regularization
	ot      []OTRow
	imports []ImportLog
	runs    []JobRun
	tenants []uuid.UUID
	fail    map[string]error
}

func newStore() *fakeStore {
	return &fakeStore{rosters: map[uuid.UUID]Roster{}, regs: map[uuid.UUID]attModels.Regularization{}, fail: map[string]error{}}
}

func (f *fakeStore) InTx(_ context.Context, fn func(*gorm.DB) error) error {
	r, im, ru := map[uuid.UUID]Roster{}, len(f.imports), len(f.runs)
	for k, v := range f.rosters {
		r[k] = v
	}
	if err := fn(nil); err != nil {
		f.rosters, f.imports, f.runs = r, f.imports[:im], f.runs[:ru]
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
	case *Roster:
		_ = v.BeforeCreate(nil)
		f.rosters[v.ID] = *v
	case *ImportLog:
		f.imports = append(f.imports, *v)
	case *JobRun:
		f.runs = append(f.runs, *v)
	}
	return nil
}
func (f *fakeStore) Save(_ context.Context, _ *gorm.DB, row interface{}) error {
	if err := f.fail["save"]; err != nil {
		return err
	}
	if r, ok := row.(*Roster); ok {
		f.rosters[r.ID] = *r
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
func (f *fakeStore) AutoAbsentTenants(context.Context, *gorm.DB) ([]uuid.UUID, error) {
	return f.tenants, f.fail["tenants"]
}
func (f *fakeStore) Roster(_ context.Context, _ *gorm.DB, _, id uuid.UUID) (*Roster, error) {
	if r, ok := f.rosters[id]; ok {
		return &r, f.fail["roster"]
	}
	return nil, f.fail["roster"]
}
func (f *fakeStore) Rosters(_ context.Context, _ *gorm.DB, _ uuid.UUID, status string) ([]Roster, error) {
	var out []Roster
	for _, r := range f.rosters {
		if status == "" || r.Status == status {
			out = append(out, r)
		}
	}
	return out, f.fail["rosters"]
}
func (f *fakeStore) RosterNameTaken(_ context.Context, _ *gorm.DB, _ uuid.UUID, name string, except *uuid.UUID) (bool, error) {
	for _, r := range f.rosters {
		if strings.EqualFold(r.Name, name) && (except == nil || r.ID != *except) {
			return true, f.fail["taken"]
		}
	}
	return false, f.fail["taken"]
}
func (f *fakeStore) Regularization(_ context.Context, _ *gorm.DB, _, id uuid.UUID) (*attModels.Regularization, error) {
	if r, ok := f.regs[id]; ok {
		return &r, f.fail["reg"]
	}
	return nil, f.fail["reg"]
}
func (f *fakeStore) PendingRegularizations(_ context.Context, _ *gorm.DB, _ uuid.UUID, ids []uuid.UUID, all bool, _ int) ([]attModels.Regularization, error) {
	var out []Regularization
	_ = out
	var rows []attModels.Regularization
	for _, r := range f.regs {
		match := all
		for _, id := range ids {
			match = match || id == r.EmployeeProfileID
		}
		if match && r.Status == attModels.RegPending {
			rows = append(rows, r)
		}
	}
	return rows, f.fail["pending"]
}

// Regularization alias keeps the helper above readable.
type Regularization = attModels.Regularization

func (f *fakeStore) OvertimeRows(context.Context, *gorm.DB, uuid.UUID, time.Time, time.Time, *uuid.UUID) ([]OTRow, error) {
	return f.ot, f.fail["ot"]
}
func (f *fakeStore) Imports(context.Context, *gorm.DB, uuid.UUID, int) ([]ImportLog, error) {
	return f.imports, f.fail["imports"]
}
func (f *fakeStore) JobRuns(context.Context, *gorm.DB, uuid.UUID, int) ([]JobRun, error) {
	return f.runs, f.fail["runs"]
}
func (f *fakeStore) JobRan(_ context.Context, _ *gorm.DB, _ uuid.UUID, _ string, date time.Time) (bool, error) {
	for _, r := range f.runs {
		if r.RunDate.Equal(date) {
			return true, f.fail["jobran"]
		}
	}
	return false, f.fail["jobran"]
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
		if (f.Status == "" || r.Status == f.Status) && (f.Search == "" || strings.Contains(strings.ToLower(r.EmployeeCode), strings.ToLower(f.Search))) {
			all = append(all, r)
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].ID.String() < all[j].ID.String() })
	lo := (f.Page - 1) * f.PerPage
	if lo > len(all) {
		lo = len(all)
	}
	return all[lo:min(len(all), lo+f.PerPage)], int64(len(all)), e.err
}

type fakeReports struct {
	byManager map[uuid.UUID][]uuid.UUID
	err       error
}

func (r *fakeReports) DirectReportUserIDs(_ context.Context, _, m uuid.UUID) ([]uuid.UUID, error) {
	return r.byManager[m], r.err
}

type dayKey struct {
	emp  uuid.UUID
	date string
}

type fakeWriter struct {
	days    map[dayKey]string // status
	punched map[dayKey]bool
	ranges  map[uuid.UUID][][2]time.Time
	shifts  map[uuid.UUID]bool
	fail    map[string]error
	sources []string
}

func newWriter() *fakeWriter {
	return &fakeWriter{days: map[dayKey]string{}, punched: map[dayKey]bool{}, ranges: map[uuid.UUID][][2]time.Time{}, shifts: map[uuid.UUID]bool{}, fail: map[string]error{}}
}

func (w *fakeWriter) SetWindow(_ context.Context, _ *gorm.DB, _, emp uuid.UUID, date, _, _ time.Time, source string) error {
	if err := w.fail["set"]; err != nil {
		return err
	}
	k := dayKey{emp, date.Format(DateLayout)}
	w.days[k], w.punched[k] = "present", true
	w.sources = append(w.sources, source)
	return nil
}
func (w *fakeWriter) MarkDay(_ context.Context, _ *gorm.DB, _, emp uuid.UUID, date time.Time, status string) (bool, error) {
	if err := w.fail["mark"]; err != nil {
		return false, err
	}
	k := dayKey{emp, date.Format(DateLayout)}
	if _, ok := w.days[k]; ok {
		return false, nil
	}
	w.days[k] = status
	return true, nil
}
func (w *fakeWriter) AssignShift(_ context.Context, _ *gorm.DB, _, emp, _ uuid.UUID, from, to time.Time, _ uuid.UUID) error {
	if err := w.fail["assign"]; err != nil {
		return err
	}
	for _, r := range w.ranges[emp] {
		if !from.After(r[1]) && !to.Before(r[0]) {
			return attServices.ErrAssignmentOverlap
		}
	}
	w.ranges[emp] = append(w.ranges[emp], [2]time.Time{from, to})
	return nil
}
func (w *fakeWriter) ShiftState(_ context.Context, _ *gorm.DB, _, id uuid.UUID) (bool, bool, error) {
	active, ok := w.shifts[id]
	return ok, active, w.fail["shift"]
}
func (w *fakeWriter) HasRecord(_ context.Context, _ *gorm.DB, _, emp uuid.UUID, date time.Time) (bool, bool, error) {
	k := dayKey{emp, date.Format(DateLayout)}
	_, ok := w.days[k]
	return ok, w.punched[k], w.fail["has"]
}

type fakeRegs struct {
	approved, rejected []uuid.UUID
	err                error
}

func (r *fakeRegs) Create(context.Context, attServices.Actor, attDTO.CreateRegularizationRequest) (*attDTO.RegularizationResponse, error) {
	return nil, nil
}
func (r *fakeRegs) ListMine(context.Context, attServices.Actor, string, attDTO.Page) ([]attDTO.RegularizationResponse, attDTO.PageMeta, error) {
	return nil, attDTO.PageMeta{}, nil
}
func (r *fakeRegs) List(context.Context, uuid.UUID, string, string, attDTO.Page) ([]attDTO.RegularizationResponse, attDTO.PageMeta, error) {
	return nil, attDTO.PageMeta{}, nil
}
func (r *fakeRegs) Cancel(context.Context, attServices.Actor, uuid.UUID) (*attDTO.RegularizationResponse, error) {
	return nil, nil
}
func (r *fakeRegs) Approve(_ context.Context, _ attServices.Actor, id uuid.UUID, _ attDTO.ReviewRequest) (*attDTO.RegularizationResponse, error) {
	r.approved = append(r.approved, id)
	return &attDTO.RegularizationResponse{}, r.err
}
func (r *fakeRegs) Reject(_ context.Context, _ attServices.Actor, id uuid.UUID, _ attDTO.ReviewRequest) (*attDTO.RegularizationResponse, error) {
	r.rejected = append(r.rejected, id)
	return &attDTO.RegularizationResponse{}, r.err
}

type fakeHolidays struct {
	dates   map[string]bool
	weekOff *bool
	err     error
}

func (h *fakeHolidays) IsHolidayFor(_ context.Context, _ *gorm.DB, _, _ uuid.UUID, d time.Time) (bool, *bool, error) {
	return h.dates[d.Format(DateLayout)], h.weekOff, h.err
}

func (h *fakeHolidays) IsHoliday(_ context.Context, _ *gorm.DB, _ uuid.UUID, d time.Time) (bool, error) {
	return h.dates[d.Format(DateLayout)], h.err
}

type noAudit struct{}

func (noAudit) Log(context.Context, *gorm.DB, string, string, string, string, string, interface{}, string, string) error {
	return nil
}

var _ identityport.Auditor = noAudit{}

// --- environment ---

type env struct {
	svc                      *Service
	store                    *fakeStore
	emps                     *fakeEmployees
	reports                  *fakeReports
	writer                   *fakeWriter
	regs                     *fakeRegs
	hol                      *fakeHolidays
	tenant, shiftA, shiftB   uuid.UUID
	hr, boss, stranger       Actor
	alice, bob, carol, bossE uuid.UUID
}

// 2026-06-10 is a Wednesday.
var now = time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC)

func setup(t *testing.T) *env {
	t.Helper()
	e := &env{store: newStore(), writer: newWriter(), regs: &fakeRegs{}, hol: &fakeHolidays{dates: map[string]bool{}}, tenant: uuid.New(),
		shiftA: uuid.New(), shiftB: uuid.New(), alice: uuid.New(), bob: uuid.New(), carol: uuid.New(), bossE: uuid.New()}
	bossU, aliceU, bobU, carolU := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	joined := employeeDTO.EmploymentDetail{JoiningDate: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)}
	e.emps = &fakeEmployees{byID: map[uuid.UUID]employeeDTO.EmployeeResponse{
		e.bossE: {ID: e.bossE, UserID: bossU, EmployeeCode: "BOSS", DisplayName: "Boss", Status: "active", Employment: &joined},
		e.alice: {ID: e.alice, UserID: aliceU, EmployeeCode: "A-1", DisplayName: "Alice", Status: "active", Employment: &joined},
		e.bob:   {ID: e.bob, UserID: bobU, EmployeeCode: "B-1", DisplayName: "Bob", Status: "probation", Employment: &joined},
		e.carol: {ID: e.carol, UserID: carolU, EmployeeCode: "C-1", DisplayName: "Carol", Status: "terminated", Employment: &joined}}}
	e.reports = &fakeReports{byManager: map[uuid.UUID][]uuid.UUID{bossU: {aliceU, bobU}}}
	e.writer.shifts[e.shiftA], e.writer.shifts[e.shiftB] = true, true
	e.svc = NewService(e.store, Ports{Employees: e.emps, Reports: e.reports, Writer: e.writer, Regs: e.regs, Holidays: e.hol, Audit: noAudit{}},
		func() time.Time { return now })
	e.hr = Actor{TenantID: e.tenant, UserID: uuid.New(), CanApproveAll: true}
	e.boss = Actor{TenantID: e.tenant, UserID: bossU}
	e.stranger = Actor{TenantID: e.tenant, UserID: uuid.New()}
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

func mustErr(_ interface{}, err error) error { return err }

func str(s string) *string { return &s }

func date(s string) time.Time {
	d, _ := time.Parse(DateLayout, s)
	return d
}

// --- inbox (AP-01, AP-02) ---

func (e *env) pending(emp uuid.UUID) uuid.UUID {
	id := uuid.New()
	e.store.regs[id] = attModels.Regularization{ID: id, TenantID: e.tenant, EmployeeProfileID: emp, AttendanceDate: date("2026-06-09"), Status: attModels.RegPending,
		RequestedPunchIn: now, RequestedPunchOut: now.Add(time.Hour), Reason: "forgot"}
	return id
}

func TestInbox(t *testing.T) {
	e := setup(t)
	mine, theirs, other := e.pending(e.alice), e.pending(e.bob), e.pending(e.carol)
	items, err := e.svc.Inbox(ctx(), e.boss, false)
	if err != nil || len(items) != 2 || items[0].EmployeeName == "" {
		t.Fatal(items, err)
	}
	if items, _ := e.svc.Inbox(ctx(), e.stranger, false); len(items) != 0 {
		t.Fatal("AG1 a stranger has no reports")
	}
	code(t, mustErr(e.svc.Inbox(ctx(), e.boss, true)), "FORBIDDEN")
	all, err := e.svc.Inbox(ctx(), e.hr, true)
	if err != nil || len(all) != 3 {
		t.Fatal(len(all), err)
	}
	// A worker's own request never shows in their inbox.
	hrWorker := e.hr
	hrWorker.UserID = e.emps.byID[e.alice].UserID
	if all, _ := e.svc.Inbox(ctx(), hrWorker, true); len(all) != 2 {
		t.Fatalf("own request hidden: %d", len(all))
	}

	_, err = e.svc.Decide(ctx(), e.boss, mine, true, nil)
	ok(t, err)
	_, err = e.svc.Decide(ctx(), e.boss, theirs, false, str("no"))
	ok(t, err)
	code(t, mustErr(e.svc.Decide(ctx(), e.boss, other, true, nil)), "NOT_FOUND")
	code(t, mustErr(e.svc.Decide(ctx(), e.boss, uuid.New(), true, nil)), "NOT_FOUND")
	_, err = e.svc.Decide(ctx(), e.hr, other, true, nil)
	ok(t, err)
	if len(e.regs.approved) != 2 || len(e.regs.rejected) != 1 {
		t.Fatal("decisions routed through the regularization service")
	}
	e.regs.err = errBoom
	if _, err := e.svc.Decide(ctx(), e.hr, other, true, nil); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}

	for _, k := range []string{"pending"} {
		e.store.fail[k] = errBoom
		if _, err := e.svc.Inbox(ctx(), e.boss, false); !errors.Is(err, errBoom) {
			t.Fatal(k, err)
		}
		delete(e.store.fail, k)
	}
	e.reports.err = errBoom
	if _, err := e.svc.Inbox(ctx(), e.boss, false); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	if _, err := e.svc.Decide(ctx(), e.boss, mine, true, nil); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	e.reports.err, e.emps.err = nil, errBoom
	if _, err := e.svc.Inbox(ctx(), e.hr, true); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	e.store.fail["reg"] = errBoom
	if _, err := e.svc.Decide(ctx(), e.hr, mine, true, nil); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
}

// --- policy and overtime (AP-03, AP-04) ---

func validPolicy() PolicyInput {
	return PolicyInput{WeekOffDays: []int{0, 6}, OvertimeMinDaily: 30, OvertimeMaxDaily: 120, OvertimeMaxMonthly: 300, OvertimeRateWeekday: 150, OvertimeRateWeekOff: 200}
}

func TestPolicy(t *testing.T) {
	e := setup(t)
	v, err := e.svc.GetPolicy(ctx(), e.hr)
	if err != nil || v.OvertimeMaxDaily != 240 || len(v.WeekOffDays) != 2 || v.AutoAbsentEnabled {
		t.Fatalf("defaults: %+v %v", v, err)
	}
	bad := []func(*PolicyInput){
		func(p *PolicyInput) { p.WeekOffDays = []int{7} }, func(p *PolicyInput) { p.WeekOffDays = []int{1, 1} },
		func(p *PolicyInput) { p.OvertimeMinDaily = 200 }, func(p *PolicyInput) { p.OvertimeMaxDaily = 2000 },
		func(p *PolicyInput) { p.OvertimeMaxMonthly = -1 }, func(p *PolicyInput) { p.OvertimeRateWeekday = 99 }, func(p *PolicyInput) { p.OvertimeRateWeekOff = 501 },
	}
	for _, mutate := range bad {
		in := validPolicy()
		mutate(&in)
		code(t, mustErr(e.svc.SetPolicy(ctx(), e.hr, in)), "VALIDATION_ERROR")
	}
	in := validPolicy()
	in.WeekOffDays, in.AutoAbsentEnabled = []int{5, 6}, true
	v, err = e.svc.SetPolicy(ctx(), e.hr, in)
	if err != nil || len(v.WeekOffDays) != 2 || v.WeekOffDays[0] != 5 || !v.AutoAbsentEnabled {
		t.Fatal(v, err)
	}
	if got, _ := e.svc.GetPolicy(ctx(), e.hr); got.OvertimeMaxMonthly != 300 {
		t.Fatal("saved policy is returned")
	}
	e.store.fail["savepolicy"] = errBoom
	if _, err := e.svc.SetPolicy(ctx(), e.hr, in); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	e.store.fail["policy"] = errBoom
	if _, err := e.svc.GetPolicy(ctx(), e.hr); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
}

func TestOvertimeReport(t *testing.T) {
	e := setup(t)
	_, err := e.svc.SetPolicy(ctx(), e.hr, validPolicy())
	ok(t, err)
	// 2026-06-01 Mon … 06-06 Sat, 06-07 Sun.
	e.store.ot = []OTRow{
		{EmployeeID: e.alice, Date: date("2026-06-01"), WorkMinutes: 540, OvertimeMinutes: 20},  // below the minimum
		{EmployeeID: e.alice, Date: date("2026-06-02"), WorkMinutes: 600, OvertimeMinutes: 60},  // 60 @150
		{EmployeeID: e.alice, Date: date("2026-06-03"), WorkMinutes: 720, OvertimeMinutes: 240}, // capped at 120 @150
		{EmployeeID: e.alice, Date: date("2026-06-06"), WorkMinutes: 100, OvertimeMinutes: 0},   // week off: 100 @200
		{EmployeeID: e.alice, Date: date("2026-06-08"), WorkMinutes: 600, OvertimeMinutes: 100}, // only 20 of the monthly cap left
		{EmployeeID: e.bob, Date: date("2026-06-02"), WorkMinutes: 500, OvertimeMinutes: 10},
	}
	rep, err := e.svc.Overtime(ctx(), e.hr, "2026-06", nil)
	if err != nil || len(rep.Lines) != 1 {
		t.Fatalf("AG2 lines: %+v %v", rep, err)
	}
	l := rep.Lines[0]
	if l.EligibleMinutes != 300 || !l.Capped || len(l.Days) != 4 || l.Days[2].WeekOff != true || l.Days[2].Minutes != 100 {
		t.Fatalf("AG2 caps: %+v", l)
	}
	if want := 60*150/100 + 120*150/100 + 100*200/100 + 20*150/100; l.WeightedMinutes != want {
		t.Fatalf("AG2 weighted %d want %d", l.WeightedMinutes, want)
	}
	code(t, mustErr(e.svc.Overtime(ctx(), e.hr, "June", nil)), "VALIDATION_ERROR")
	emp := e.alice
	if _, err := e.svc.Overtime(ctx(), e.hr, "2026-06", &emp); err != nil {
		t.Fatal(err)
	}
	e.store.fail["ot"] = errBoom
	if _, err := e.svc.Overtime(ctx(), e.hr, "2026-06", nil); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	e.store.fail["policy"] = errBoom
	if _, err := e.svc.Overtime(ctx(), e.hr, "2026-06", nil); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
}

// --- rosters (AP-05, AP-06) ---

func TestRosters(t *testing.T) {
	e := setup(t)
	pattern := []Segment{{ShiftID: e.shiftA, Days: 7}, {ShiftID: e.shiftB, Days: 7}}
	for _, in := range []RosterInput{{}, {Name: str("x")}, {Name: str(" "), Pattern: &pattern}, {Name: str("x"), Pattern: &[]Segment{}},
		{Name: str("x"), Pattern: &[]Segment{{ShiftID: e.shiftA, Days: 0}}}, {Name: str("x"), Pattern: &[]Segment{{ShiftID: uuid.New(), Days: 3}}},
		{Name: str("x"), Pattern: &pattern, Status: str("weird")}, {Name: str("x"), Pattern: func() *[]Segment {
			p := make([]Segment, 13)
			for i := range p {
				p[i] = Segment{ShiftID: e.shiftA, Days: 1}
			}
			return &p
		}()}} {
		code(t, mustErr(e.svc.CreateRoster(ctx(), e.hr, in)), "VALIDATION_ERROR")
	}
	e.writer.shifts[e.shiftB] = false
	code(t, mustErr(e.svc.CreateRoster(ctx(), e.hr, RosterInput{Name: str("x"), Pattern: &pattern})), "INACTIVE_REFERENCE")
	e.writer.shifts[e.shiftB] = true
	r, err := e.svc.CreateRoster(ctx(), e.hr, RosterInput{Name: str(" Rotating "), Pattern: &pattern})
	ok(t, err)
	code(t, mustErr(e.svc.CreateRoster(ctx(), e.hr, RosterInput{Name: str("rotating"), Pattern: &pattern})), "CONFLICT")
	other, err := e.svc.CreateRoster(ctx(), e.hr, RosterInput{Name: str("Other"), Pattern: &pattern})
	ok(t, err)
	code(t, mustErr(e.svc.UpdateRoster(ctx(), e.hr, other.ID, RosterInput{Name: str("ROTATING")})), "CONFLICT")
	up, err := e.svc.UpdateRoster(ctx(), e.hr, other.ID, RosterInput{Name: str("Night"), Status: str(RosterArchived)})
	if err != nil || up.Status != RosterArchived {
		t.Fatal(up, err)
	}
	code(t, mustErr(e.svc.UpdateRoster(ctx(), e.hr, uuid.New(), RosterInput{})), "NOT_FOUND")
	if got, err := e.svc.GetRoster(ctx(), e.hr, r.ID); err != nil || got.Name != "Rotating" {
		t.Fatal(got, err)
	}
	code(t, mustErr(e.svc.GetRoster(ctx(), e.hr, uuid.New())), "NOT_FOUND")
	if rows, _ := e.svc.Rosters(ctx(), e.hr, RosterActive); len(rows) != 1 {
		t.Fatal("status filter")
	}
	code(t, mustErr(e.svc.Rosters(ctx(), e.hr, "weird")), "VALIDATION_ERROR")

	// Apply: segments end to end for two workers.
	res, err := e.svc.ApplyRoster(ctx(), e.hr, r.ID, ApplyInput{EmployeeIDs: []string{e.alice.String(), e.bob.String()}, From: "2026-07-01", Cycles: 2})
	if err != nil || res.Workers != 2 || res.Assignments != 8 {
		t.Fatal(res, err)
	}
	rs := e.writer.ranges[e.alice]
	if rs[0][0] != date("2026-07-01") || rs[0][1] != date("2026-07-07") || rs[1][0] != date("2026-07-08") || rs[3][1] != date("2026-07-28") {
		t.Fatalf("AG3 layout: %v", rs)
	}
	// AG3: an overlap names the worker and changes nothing for anyone.
	before := len(e.writer.ranges[e.bob])
	_, err = e.svc.ApplyRoster(ctx(), e.hr, r.ID, ApplyInput{EmployeeIDs: []string{e.carol.String(), e.bob.String()}, From: "2026-07-20", Cycles: 1})
	code(t, err, "ROSTER_OVERLAP")
	if !strings.Contains(err.Error(), e.bob.String()) || len(e.writer.ranges[e.bob]) != before {
		t.Fatal("overlap detail")
	}
	bad := []ApplyInput{{EmployeeIDs: []string{e.alice.String()}, From: "x", Cycles: 1}, {EmployeeIDs: []string{e.alice.String()}, From: "2026-08-01", Cycles: 0},
		{EmployeeIDs: nil, From: "2026-08-01", Cycles: 1}, {EmployeeIDs: []string{"x"}, From: "2026-08-01", Cycles: 1},
		{EmployeeIDs: []string{e.alice.String(), e.alice.String()}, From: "2026-08-01", Cycles: 1}, {EmployeeIDs: []string{e.alice.String()}, From: "2026-08-01", Cycles: 27}}
	for _, in := range bad {
		code(t, mustErr(e.svc.ApplyRoster(ctx(), e.hr, r.ID, in)), "VALIDATION_ERROR")
	}
	code(t, mustErr(e.svc.ApplyRoster(ctx(), e.hr, other.ID, ApplyInput{EmployeeIDs: []string{e.alice.String()}, From: "2026-08-01", Cycles: 1})), "ROSTER_ARCHIVED")
	code(t, mustErr(e.svc.ApplyRoster(ctx(), e.hr, uuid.New(), ApplyInput{EmployeeIDs: []string{e.alice.String()}, From: "2026-08-01", Cycles: 1})), "NOT_FOUND")
	code(t, mustErr(e.svc.ApplyRoster(ctx(), e.hr, r.ID, ApplyInput{EmployeeIDs: []string{uuid.NewString()}, From: "2026-08-01", Cycles: 1})), "EMPLOYEE_NOT_FOUND")
	long, err := e.svc.CreateRoster(ctx(), e.hr, RosterInput{Name: str("Long"), Pattern: &[]Segment{{ShiftID: e.shiftA, Days: 1}, {ShiftID: e.shiftB, Days: 1}, {ShiftID: e.shiftA, Days: 1},
		{ShiftID: e.shiftB, Days: 1}, {ShiftID: e.shiftA, Days: 1}, {ShiftID: e.shiftB, Days: 1}, {ShiftID: e.shiftA, Days: 1}, {ShiftID: e.shiftB, Days: 1}, {ShiftID: e.shiftA, Days: 1}}})
	ok(t, err)
	code(t, mustErr(e.svc.ApplyRoster(ctx(), e.hr, long.ID, ApplyInput{EmployeeIDs: []string{e.alice.String()}, From: "2027-01-01", Cycles: 26})), "VALIDATION_ERROR")
	e.writer.shifts[e.shiftA] = false
	code(t, mustErr(e.svc.ApplyRoster(ctx(), e.hr, r.ID, ApplyInput{EmployeeIDs: []string{e.alice.String()}, From: "2026-12-01", Cycles: 1})), "INACTIVE_REFERENCE")
	e.writer.shifts[e.shiftA] = true
	e.emps.err = errBoom
	if _, err := e.svc.ApplyRoster(ctx(), e.hr, r.ID, ApplyInput{EmployeeIDs: []string{e.alice.String()}, From: "2026-12-01", Cycles: 1}); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	e.emps.err = nil
	e.writer.fail["assign"] = errBoom
	if _, err := e.svc.ApplyRoster(ctx(), e.hr, r.ID, ApplyInput{EmployeeIDs: []string{e.alice.String()}, From: "2026-12-01", Cycles: 1}); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	e.writer.fail["shift"] = errBoom
	if _, err := e.svc.ApplyRoster(ctx(), e.hr, r.ID, ApplyInput{EmployeeIDs: []string{e.alice.String()}, From: "2026-12-01", Cycles: 1}); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	if _, err := e.svc.CreateRoster(ctx(), e.hr, RosterInput{Name: str("Z"), Pattern: &pattern}); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	delete(e.writer.fail, "shift")
	for _, k := range []string{"taken", "create"} {
		e.store.fail[k] = errBoom
		if _, err := e.svc.CreateRoster(ctx(), e.hr, RosterInput{Name: str("Y" + k), Pattern: &pattern}); !errors.Is(err, errBoom) {
			t.Fatal(k, err)
		}
		delete(e.store.fail, k)
	}
	for _, k := range []string{"taken", "save"} {
		e.store.fail[k] = errBoom
		if _, err := e.svc.UpdateRoster(ctx(), e.hr, r.ID, RosterInput{Name: str("Y" + k)}); !errors.Is(err, errBoom) {
			t.Fatal(k, err)
		}
		delete(e.store.fail, k)
	}
	e.store.fail["roster"] = errBoom
	if _, err := e.svc.ApplyRoster(ctx(), e.hr, r.ID, ApplyInput{EmployeeIDs: []string{e.alice.String()}, From: "2026-12-01", Cycles: 1}); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
}

// --- auto-absent (AP-07, AP-08) ---

func TestAutoAbsent(t *testing.T) {
	e := setup(t)
	// 2026-06-09 is a Tuesday.
	e.writer.days[dayKey{e.alice, "2026-06-09"}] = "present"
	res, err := e.svc.AutoAbsent(ctx(), e.hr, "")
	if err != nil || res.Date.Format(DateLayout) != "2026-06-09" || res.Absent != 2 || res.Skipped != 1 {
		t.Fatalf("AG4 default date: %+v %v", res, err)
	}
	if e.writer.days[dayKey{e.bob, "2026-06-09"}] != attModels.StatusAbsent {
		t.Fatal("bob marked absent")
	}
	if _, ok := e.writer.days[dayKey{e.carol, "2026-06-09"}]; ok {
		t.Fatal("terminated workers are not marked")
	}
	// Idempotent.
	res, err = e.svc.AutoAbsent(ctx(), e.hr, "2026-06-09")
	if err != nil || res.Absent != 0 || res.Skipped != 3 {
		t.Fatalf("AG4 idempotent: %+v %v", res, err)
	}
	// Week off (Saturday 06-06) and holiday (Monday 06-08).
	res, _ = e.svc.AutoAbsent(ctx(), e.hr, "2026-06-06")
	if res.WeekOff != 3 || e.writer.days[dayKey{e.alice, "2026-06-06"}] != attModels.StatusWeekOff {
		t.Fatalf("AG4 week off: %+v", res)
	}
	e.hol.dates["2026-06-08"] = true
	res, _ = e.svc.AutoAbsent(ctx(), e.hr, "2026-06-08")
	if res.Holiday != 3 {
		t.Fatalf("AG4 holiday: %+v", res)
	}
	// Joined later / already exited are skipped.
	late := e.emps.byID[e.bob]
	late.Employment = &employeeDTO.EmploymentDetail{JoiningDate: date("2026-06-20")}
	e.emps.byID[e.bob] = late
	exited := e.emps.byID[e.alice]
	exit := date("2026-05-01")
	exited.Employment = &employeeDTO.EmploymentDetail{JoiningDate: date("2025-01-01"), ExitDate: &exit}
	e.emps.byID[e.alice] = exited
	if res, _ = e.svc.AutoAbsent(ctx(), e.hr, "2026-06-05"); res.Absent != 1 || res.Skipped != 2 {
		t.Fatalf("employment window: %+v", res)
	}
	withoutEmployment := e.emps.byID[e.bossE]
	withoutEmployment.Employment = nil
	e.emps.byID[e.bossE] = withoutEmployment
	if !workedOn(withoutEmployment, now) {
		t.Fatal("no employment detail counts as employed")
	}
	for _, d := range []string{"2026-06-10", "2026-06-11", "2026-04-01", "bad"} {
		if _, err := e.svc.AutoAbsent(ctx(), e.hr, d); err == nil {
			t.Fatal("window", d)
		}
	}
	code(t, mustErr(e.svc.AutoAbsent(ctx(), e.hr, "2026-06-10")), "JOB_WINDOW")
	if runs, err := e.svc.Runs(ctx(), e.hr); err != nil || len(runs) < 4 {
		t.Fatal(runs, err)
	}

	e.hol.err = errBoom
	if _, err := e.svc.AutoAbsent(ctx(), e.hr, "2026-06-04"); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	e.hol.err, e.emps.err = nil, errBoom
	if _, err := e.svc.AutoAbsent(ctx(), e.hr, "2026-06-04"); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	e.emps.err = nil
	e.writer.fail["mark"] = errBoom
	if _, err := e.svc.AutoAbsent(ctx(), e.hr, "2026-06-04"); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	delete(e.writer.fail, "mark")
	e.store.fail["policy"] = errBoom
	if _, err := e.svc.AutoAbsent(ctx(), e.hr, "2026-06-04"); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	delete(e.store.fail, "policy")
	e.store.fail["create"] = errBoom
	if _, err := e.svc.AutoAbsent(ctx(), e.hr, "2026-06-04"); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
}

func TestSweep(t *testing.T) {
	e := setup(t)
	e.store.tenants = []uuid.UUID{e.tenant}
	n, err := e.svc.Sweep(ctx())
	if err != nil || n != 1 {
		t.Fatal(n, err)
	}
	if n, _ = e.svc.Sweep(ctx()); n != 0 {
		t.Fatal("a tenant already run for the day is skipped")
	}
	e.store.fail["tenants"] = errBoom
	if _, err := e.svc.Sweep(ctx()); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	delete(e.store.fail, "tenants")
	e.store.runs = nil
	e.store.fail["jobran"] = errBoom
	if _, err := e.svc.Sweep(ctx()); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	delete(e.store.fail, "jobran")
	e.emps.err = errBoom
	if _, err := e.svc.Sweep(ctx()); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	e.emps.err = nil

	// Run ticks once at start and stops with the context.
	c, cancel := context.WithCancel(ctx())
	e.store.runs = nil
	done := make(chan struct{})
	var seen error
	e.store.fail["tenants"] = errBoom
	go func() { e.svc.Run(c, 0, func(err error) { seen = err; cancel() }); close(done) }()
	<-done
	if !errors.Is(seen, errBoom) {
		t.Fatal("Run reports sweep errors", seen)
	}
}

// --- bulk import (AP-10, AP-11) ---

func row(code, date, in, out string) ImportRow {
	return ImportRow{EmployeeCode: code, Date: date, PunchIn: in, PunchOut: out}
}

func TestImport(t *testing.T) {
	e := setup(t)
	good := []ImportRow{row("A-1", "2026-06-08", "2026-06-08T09:00:00Z", "2026-06-08T18:00:00Z"), row("b-1", "2026-06-08", "2026-06-08T03:30:00+05:30", "2026-06-08T12:30:00+05:30")}
	res, err := e.svc.Import(ctx(), e.hr, ImportInput{DryRun: true, Rows: good})
	if err != nil || res.Applied != 0 || len(res.Errors) != 0 || len(e.writer.punched) != 0 {
		t.Fatalf("dry run writes nothing: %+v %v", res, err)
	}
	res, err = e.svc.Import(ctx(), e.hr, ImportInput{Rows: good})
	if err != nil || res.Applied != 2 || len(e.store.imports) != 1 || e.writer.sources[0] != attModels.SourceImport {
		t.Fatalf("applied: %+v %v", res, err)
	}
	// AG5: one bad row means nothing is written.
	mixed := []ImportRow{row("A-1", "2026-06-07", "2026-06-07T09:00:00Z", "2026-06-07T18:00:00Z"), row("NOPE", "2026-06-07", "2026-06-07T09:00:00Z", "2026-06-07T18:00:00Z")}
	n := len(e.writer.punched)
	res, err = e.svc.Import(ctx(), e.hr, ImportInput{Rows: mixed})
	if err != nil || res.Applied != 0 || len(res.Errors) != 1 || res.Errors[0].Row != 2 || res.Errors[0].Code != "EMPLOYEE_NOT_FOUND" || len(e.writer.punched) != n {
		t.Fatalf("AG5 atomic: %+v", res)
	}
	cases := map[string]ImportRow{
		"VALIDATION_ERROR": row("", "2026-06-07", "2026-06-07T09:00:00Z", "2026-06-07T18:00:00Z"),
		"DATE_WINDOW":      row("A-1", "2026-01-01", "2026-01-01T09:00:00Z", "2026-01-01T18:00:00Z"),
		"DATE_MISMATCH":    row("A-1", "2026-06-07", "2026-06-02T09:00:00Z", "2026-06-02T18:00:00Z"),
		"EXISTS":           row("A-1", "2026-06-08", "2026-06-08T09:00:00Z", "2026-06-08T18:00:00Z"),
	}
	for want, r := range cases {
		res, _ := e.svc.Import(ctx(), e.hr, ImportInput{DryRun: true, Rows: []ImportRow{r}})
		if len(res.Errors) != 1 || res.Errors[0].Code != want {
			t.Fatalf("%s: %+v", want, res.Errors)
		}
	}
	for _, r := range []ImportRow{row("A-1", "x", "2026-06-07T09:00:00Z", "2026-06-07T18:00:00Z"), row("A-1", "2026-06-07", "x", "y"),
		row("A-1", "2026-06-07", "2026-06-07T18:00:00Z", "2026-06-07T09:00:00Z"), row("A-1", "2026-06-07", "2026-06-07T01:00:00Z", "2026-06-08T01:00:00Z"),
		row("A-1", "2026-06-20", "2026-06-20T09:00:00Z", "2026-06-20T18:00:00Z")} {
		if res, _ := e.svc.Import(ctx(), e.hr, ImportInput{DryRun: true, Rows: []ImportRow{r}}); len(res.Errors) != 1 {
			t.Fatalf("expected one error for %+v: %+v", r, res.Errors)
		}
	}
	dup := []ImportRow{row("A-1", "2026-06-05", "2026-06-05T09:00:00Z", "2026-06-05T18:00:00Z"), row("A-1", "2026-06-05", "2026-06-05T10:00:00Z", "2026-06-05T18:00:00Z")}
	if res, _ := e.svc.Import(ctx(), e.hr, ImportInput{Rows: dup}); len(res.Errors) != 1 || res.Errors[0].Code != "DUPLICATE_ROW" {
		t.Fatalf("duplicate: %+v", res.Errors)
	}
	// Overwrite replaces existing punches.
	res, err = e.svc.Import(ctx(), e.hr, ImportInput{Overwrite: true, Rows: []ImportRow{cases["EXISTS"]}})
	if err != nil || res.Applied != 1 || !e.store.imports[len(e.store.imports)-1].Overwrite {
		t.Fatalf("overwrite: %+v %v", res, err)
	}
	code(t, mustErr(e.svc.Import(ctx(), e.hr, ImportInput{})), "IMPORT_TOO_LARGE")
	code(t, mustErr(e.svc.Import(ctx(), e.hr, ImportInput{Rows: make([]ImportRow, 501)})), "IMPORT_TOO_LARGE")

	e.writer.fail["has"] = errBoom
	if _, err := e.svc.Import(ctx(), e.hr, ImportInput{Rows: good}); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	delete(e.writer.fail, "has")
	e.emps.err = errBoom
	if _, err := e.svc.Import(ctx(), e.hr, ImportInput{Rows: good}); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	e.emps.err = nil
	fresh := []ImportRow{row("A-1", "2026-06-04", "2026-06-04T09:00:00Z", "2026-06-04T18:00:00Z")}
	for _, k := range []string{"set"} {
		e.writer.fail[k] = errBoom
		if _, err := e.svc.Import(ctx(), e.hr, ImportInput{Rows: fresh}); !errors.Is(err, errBoom) {
			t.Fatal(k, err)
		}
		delete(e.writer.fail, k)
	}
	e.store.fail["create"] = errBoom
	if _, err := e.svc.Import(ctx(), e.hr, ImportInput{Rows: fresh}); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	delete(e.store.fail, "create")
	if list, err := e.svc.Imports(ctx(), e.hr); err != nil || len(list) == 0 {
		t.Fatal(list, err)
	}
	e.store.fail["imports"] = errBoom
	if _, err := e.svc.Imports(ctx(), e.hr); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
}

func TestModelsAndHelpers(t *testing.T) {
	p := Pattern{{ShiftID: uuid.New(), Days: 3}}
	v, err := p.Value()
	ok(t, err)
	var back Pattern
	if back.Scan(v) != nil || back.Scan([]byte(v.(string))) != nil || len(back) != 1 || back.Scan(1) == nil {
		t.Fatal("pattern")
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
	for _, m := range []interface{ TableName() string }{Policy{}, Roster{}, ImportLog{}, JobRun{}} {
		if !strings.HasPrefix(m.TableName(), "attendance_") {
			t.Fatal(m.TableName())
		}
	}
	for _, m := range []interface{ BeforeCreate(*gorm.DB) error }{&Roster{}, &ImportLog{}, &JobRun{}} {
		ok(t, m.BeforeCreate(nil))
	}
	m := NewModule(nil, Ports{})
	if len(m.RegisterModels()) != 4 || m.Service() == nil || NewStore(nil).DB() != nil {
		t.Fatal("module")
	}
	if orNotFound(errBoom) != errBoom {
		t.Fatal("orNotFound")
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
	(&Module{svc: e.svc}).RegisterRoutes(r.Group(""), g)
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
	boss := router(e, e.boss.UserID, "approve", "manage", "read")
	const B = "/attendance"
	expect := func(w *httptest.ResponseRecorder, status int, label string) string {
		t.Helper()
		if w.Code != status {
			t.Fatalf("%s: %d %s", label, w.Code, w.Body.String())
		}
		return w.Body.String()
	}
	expect(do(router(e, uuid.Nil), "GET", B+"/inbox", ""), 401, "unauthenticated")
	id := e.pending(e.alice)
	expect(do(boss, "GET", B+"/inbox", ""), 200, "manager inbox")
	expect(do(boss, "GET", B+"/inbox?scope=all", ""), 403, "scope all needs approve")
	expect(do(hr, "GET", B+"/inbox?scope=all", ""), 200, "hr inbox")
	expect(do(boss, "POST", B+"/inbox/"+id.String()+"/approve", `{"comment":"ok"}`), 200, "approve")
	expect(do(boss, "POST", B+"/inbox/"+id.String()+"/reject", "{"), 400, "bad body")
	expect(do(boss, "POST", B+"/inbox/x/approve", ""), 400, "bad id")
	expect(do(boss, "POST", B+"/inbox/"+uuid.NewString()+"/reject", ""), 404, "unknown")
	expect(do(boss, "PUT", B+"/policy", `{}`), 403, "manager cannot set policy")

	expect(do(hr, "PUT", B+"/policy", `{"week_off_days":[0,6],"overtime_min_daily":30,"overtime_max_daily":120,"overtime_max_monthly":300,"overtime_rate_weekday":150,"overtime_rate_week_off":200,"auto_absent_enabled":true}`), 200, "policy")
	expect(do(hr, "PUT", B+"/policy", "{"), 400, "bad policy")
	expect(do(hr, "GET", B+"/policy", ""), 200, "get policy")
	expect(do(hr, "GET", B+"/overtime/report?month=2026-06&employee_id="+e.alice.String(), ""), 200, "overtime")
	expect(do(hr, "GET", B+"/overtime/report?month=2026-06&employee_id=x", ""), 400, "bad employee")
	expect(do(hr, "GET", B+"/overtime/report", ""), 400, "month required")

	body := `{"name":"Rotation","pattern":[{"shift_id":"` + e.shiftA.String() + `","days":7}]}`
	out := expect(do(hr, "POST", B+"/rosters", body), 201, "roster")
	rid := out[strings.Index(out, `"id":"`)+6:][:36]
	expect(do(hr, "POST", B+"/rosters", "{"), 400, "bad roster")
	expect(do(hr, "PATCH", B+"/rosters/"+rid, `{"name":"Rotation 2"}`), 200, "patch")
	expect(do(hr, "PATCH", B+"/rosters/"+rid, "{"), 400, "bad patch")
	expect(do(hr, "GET", B+"/rosters?status=active", ""), 200, "list")
	expect(do(hr, "GET", B+"/rosters/"+rid, ""), 200, "get")
	expect(do(hr, "POST", B+"/rosters/"+rid+"/apply", `{"employee_ids":["`+e.alice.String()+`"],"from":"2026-07-01","cycles":1}`), 200, "apply")
	expect(do(hr, "POST", B+"/rosters/"+rid+"/apply", `{"employee_ids":["`+e.alice.String()+`"],"from":"2026-07-01","cycles":1}`), 409, "apply overlap")
	expect(do(hr, "POST", B+"/rosters/"+rid+"/apply", "{"), 400, "bad apply")

	expect(do(hr, "POST", B+"/jobs/auto-absent", `{"date":"2026-06-09"}`), 200, "job")
	expect(do(hr, "POST", B+"/jobs/auto-absent", ""), 200, "job default")
	expect(do(hr, "POST", B+"/jobs/auto-absent", "{"), 400, "bad job")
	expect(do(hr, "POST", B+"/jobs/auto-absent", `{"date":"2026-06-10"}`), 400, "window")
	expect(do(hr, "GET", B+"/jobs/runs", ""), 200, "runs")

	expect(do(hr, "POST", B+"/imports", `{"dry_run":true,"rows":[{"employee_code":"A-1","date":"2026-06-08","punch_in":"2026-06-08T09:00:00Z","punch_out":"2026-06-08T18:00:00Z"}]}`), 200, "import")
	expect(do(hr, "POST", B+"/imports", "{"), 400, "bad import")
	expect(do(hr, "POST", B+"/imports", `{"rows":[]}`), 400, "empty import")
	expect(do(hr, "GET", B+"/imports", ""), 200, "imports")
	e.store.fail["rosters"] = errBoom
	expect(do(hr, "GET", B+"/rosters", ""), 500, "internal")
}

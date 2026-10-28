package plus

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	coreAPI "github.com/jaas/jaas/internal/hr/core/api"
	employeeDTO "github.com/jaas/jaas/internal/hr/employee/dto"
	"github.com/jaas/jaas/internal/hr/kernel"
	leaveAPI "github.com/jaas/jaas/internal/hr/leave/api"
	"github.com/jaas/jaas/internal/hr/payroll/calc"
	"github.com/jaas/jaas/internal/hr/payroll/models"
	"github.com/jaas/jaas/internal/hr/payroll/repositories"
	"github.com/jaas/jaas/internal/identity/access"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"github.com/jaas/jaas/internal/shared/identityport"
	"gorm.io/gorm"
)

var errBoom = errors.New("boom")

// --- fakes ---

type fakeStore struct {
	slabs       []models.PTSlab
	settlements map[uuid.UUID]Settlement
	lines       map[uuid.UUID][]SettlementLine
	events      []string
	fail        map[string]error
}

func newStore() *fakeStore {
	return &fakeStore{settlements: map[uuid.UUID]Settlement{}, lines: map[uuid.UUID][]SettlementLine{}, fail: map[string]error{}}
}

func (f *fakeStore) InTx(_ context.Context, fn func(*gorm.DB) error) error {
	snap, lines, ev := map[uuid.UUID]Settlement{}, map[uuid.UUID][]SettlementLine{}, len(f.events)
	for k, v := range f.settlements {
		snap[k] = v
	}
	for k, v := range f.lines {
		lines[k] = v
	}
	slabs := append([]models.PTSlab(nil), f.slabs...)
	if err := fn(nil); err != nil {
		f.settlements, f.lines, f.events, f.slabs = snap, lines, f.events[:ev], slabs
		return err
	}
	return nil
}
func (f *fakeStore) DB() *gorm.DB { return nil }
func (f *fakeStore) Create(_ context.Context, _ *gorm.DB, row interface{}) error {
	if err := f.fail["create"]; err != nil {
		return err
	}
	if s, ok := row.(*Settlement); ok {
		_ = s.BeforeCreate(nil)
		f.settlements[s.ID] = *s
	}
	return nil
}
func (f *fakeStore) Save(_ context.Context, _ *gorm.DB, row interface{}) error {
	if err := f.fail["save"]; err != nil {
		return err
	}
	if s, ok := row.(*Settlement); ok {
		f.settlements[s.ID] = *s
	}
	return nil
}
func (f *fakeStore) PTSlabs(_ context.Context, _ *gorm.DB, t uuid.UUID, state string) ([]models.PTSlab, error) {
	var out []models.PTSlab
	for _, s := range f.slabs {
		if s.TenantID == t && s.StateCode == strings.ToUpper(state) {
			out = append(out, s)
		}
	}
	return out, f.fail["slabs"]
}
func (f *fakeStore) AllPTSlabs(_ context.Context, _ *gorm.DB, t uuid.UUID) ([]models.PTSlab, error) {
	var out []models.PTSlab
	for _, s := range f.slabs {
		if s.TenantID == t {
			out = append(out, s)
		}
	}
	return out, f.fail["allslabs"]
}
func (f *fakeStore) ReplacePTSlabs(_ context.Context, _ *gorm.DB, t uuid.UUID, state string, rows []models.PTSlab) error {
	if err := f.fail["replace"]; err != nil {
		return err
	}
	kept := f.slabs[:0:0]
	for _, s := range f.slabs {
		if s.TenantID != t || s.StateCode != state {
			kept = append(kept, s)
		}
	}
	f.slabs = append(kept, rows...)
	return nil
}
func (f *fakeStore) DeletePTSlabs(_ context.Context, _ *gorm.DB, t uuid.UUID, state string) (int64, error) {
	var n int64
	kept := f.slabs[:0:0]
	for _, s := range f.slabs {
		if s.TenantID == t && s.StateCode == state {
			n++
			continue
		}
		kept = append(kept, s)
	}
	f.slabs = kept
	return n, f.fail["delete"]
}
func (f *fakeStore) Settlement(_ context.Context, _ *gorm.DB, _, id uuid.UUID, _ bool) (*Settlement, error) {
	if s, ok := f.settlements[id]; ok {
		return &s, f.fail["settlement"]
	}
	return nil, f.fail["settlement"]
}
func (f *fakeStore) OpenSettlement(_ context.Context, _ *gorm.DB, _, emp uuid.UUID) (*Settlement, error) {
	for _, s := range f.settlements {
		if s.EmployeeProfileID == emp && (s.Status == StatusDraft || s.Status == StatusApproved) {
			return &s, f.fail["open"]
		}
	}
	return nil, f.fail["open"]
}
func (f *fakeStore) Settlements(_ context.Context, _ *gorm.DB, _ uuid.UUID, emp *uuid.UUID, status string, _ int) ([]Settlement, error) {
	var out []Settlement
	for _, s := range f.settlements {
		if (emp == nil || s.EmployeeProfileID == *emp) && (status == "" || s.Status == status) {
			out = append(out, s)
		}
	}
	return out, f.fail["list"]
}
func (f *fakeStore) ReplaceLines(_ context.Context, _ *gorm.DB, id uuid.UUID, lines []SettlementLine) error {
	f.lines[id] = lines
	return f.fail["replacelines"]
}
func (f *fakeStore) Lines(_ context.Context, _ *gorm.DB, id uuid.UUID) ([]SettlementLine, error) {
	return f.lines[id], f.fail["lines"]
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

type fakeStructures struct {
	repositories.StructureRepository
	err error
}

func (f fakeStructures) FindByID(_ context.Context, _ *gorm.DB, t, id uuid.UUID) (*models.Structure, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &models.Structure{TenantID: t, PFEnabled: true}, nil
}
func (f fakeStructures) Components(context.Context, *gorm.DB, uuid.UUID, uuid.UUID) ([]models.Component, error) {
	return []models.Component{
		{Code: "BASIC", Name: "Basic", Kind: "earning", Calc: "percent_of_ctc", Value: 5000, Taxable: true},
		{Code: "SPECIAL", Name: "Special", Kind: "earning", Calc: "balance", Taxable: true},
	}, f.err
}

type fakeAssignments struct {
	repositories.AssignmentRepository
	byEmployee map[uuid.UUID][]models.Assignment
	err        error
}

func (f fakeAssignments) ForEmployee(_ context.Context, _ *gorm.DB, _, e uuid.UUID, _ repositories.Period) ([]models.Assignment, error) {
	return f.byEmployee[e], f.err
}

type fakeLeave struct {
	balances []leaveAPI.EncashableBalance
	unpaid   []leaveAPI.UnpaidEncashment
	paid     []uuid.UUID
	err      error
	payErr   error
}

func (l *fakeLeave) EncashableBalances(context.Context, *gorm.DB, uuid.UUID, uuid.UUID) ([]leaveAPI.EncashableBalance, error) {
	return l.balances, l.err
}
func (l *fakeLeave) UnpaidEncashments(context.Context, *gorm.DB, uuid.UUID, uuid.UUID) ([]leaveAPI.UnpaidEncashment, error) {
	return l.unpaid, l.err
}
func (l *fakeLeave) MarkEncashmentsPaid(_ context.Context, _ *gorm.DB, _ uuid.UUID, ids []uuid.UUID, _ time.Time) error {
	if l.payErr != nil {
		return l.payErr
	}
	l.paid = append(l.paid, ids...)
	return nil
}

type fakeClearance struct {
	done bool
	err  error
}

func (c *fakeClearance) ClearanceCompleted(context.Context, *gorm.DB, uuid.UUID, uuid.UUID) (bool, error) {
	return c.done, c.err
}

type noAudit struct{}

func (noAudit) Log(context.Context, *gorm.DB, string, string, string, string, string, interface{}, string, string) error {
	return nil
}

var _ identityport.Auditor = noAudit{}

// --- environment ---

type env struct {
	svc            *Service
	store          *fakeStore
	emps           *fakeEmployees
	assigns        fakeAssignments
	structs        fakeStructures
	leave          *fakeLeave
	clearance      *fakeClearance
	tenant         uuid.UUID
	maker, checker Actor
	leaver, active uuid.UUID
}

var now = time.Date(2026, 1, 10, 9, 0, 0, 0, time.UTC)

func setup(t *testing.T) *env {
	t.Helper()
	e := &env{store: newStore(), tenant: uuid.New(), leaver: uuid.New(), active: uuid.New(), leave: &fakeLeave{}, clearance: &fakeClearance{done: true}}
	exit, resigned := time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC), time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC)
	e.emps = &fakeEmployees{byID: map[uuid.UUID]employeeDTO.EmployeeResponse{
		e.leaver: {ID: e.leaver, UserID: uuid.New(), Status: "terminated", Employment: &employeeDTO.EmploymentDetail{
			JoiningDate: time.Date(2019, 1, 1, 0, 0, 0, 0, time.UTC), ExitDate: &exit, ResignationDate: &resigned, NoticePeriodDays: 60}},
		e.active: {ID: e.active, UserID: uuid.New(), Status: "active", Employment: &employeeDTO.EmploymentDetail{JoiningDate: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)}}}}
	ctc := kernel.Rupees(1248000) // 104,000 a month; BASIC 52,000
	e.assigns = fakeAssignments{byEmployee: map[uuid.UUID][]models.Assignment{
		e.leaver: {{StructureID: uuid.New(), AnnualCTC: ctc, EffectiveFrom: time.Date(2019, 1, 1, 0, 0, 0, 0, time.UTC)}},
		e.active: {{StructureID: uuid.New(), AnnualCTC: ctc}}}}
	e.structs = fakeStructures{}
	e.leave.balances = []leaveAPI.EncashableBalance{{LeaveTypeID: uuid.New(), Days: 500}}
	e.leave.unpaid = []leaveAPI.UnpaidEncashment{{ID: uuid.New(), LeaveTypeID: uuid.New(), Days: 100}}
	pt := NewPT(e.store)
	e.svc = NewService(e.store, Ports{Employees: e.emps, Structures: e.structs, Assignments: e.assigns, Leave: e.leave, Clearance: e.clearance, Audit: noAudit{}}, pt,
		func() time.Time { return now })
	e.maker = Actor{TenantID: e.tenant, UserID: uuid.New()}
	e.checker = Actor{TenantID: e.tenant, UserID: uuid.New()}
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

// --- PT slabs (PP-02, PP-09) ---

func rupees(n int64) kernel.Money { return kernel.Rupees(n) }

func TestPTSlabs(t *testing.T) {
	e := setup(t)
	good := []SlabInput{{MinGross: 0, MaxGross: ptrMoney(rupees(9999)), Amount: 0}, {MinGross: rupees(10000), Amount: rupees(150)}, {MinGross: rupees(10000), Amount: rupees(250), Month: ptrInt(2)}}
	code(t, mustErr(e.svc.SetPTSlabs(ctx(), e.maker, "KAR", good)), "VALIDATION_ERROR")
	code(t, mustErr(e.svc.SetPTSlabs(ctx(), e.maker, "KA", nil)), "INVALID_SLABS")
	code(t, mustErr(e.svc.SetPTSlabs(ctx(), e.maker, "KA", []SlabInput{{MinGross: rupees(5), Amount: 1}})), "INVALID_SLABS")
	views, err := e.svc.SetPTSlabs(ctx(), e.maker, "ka", good)
	if err != nil || len(views) != 3 || views[2].Month == nil {
		t.Fatal(views, err)
	}
	all, err := e.svc.PTSlabs(ctx(), e.maker)
	if err != nil || len(all.Tenant["KA"]) != 3 || len(all.Defaults["MH"]) == 0 {
		t.Fatalf("%+v %v", all, err)
	}
	slabs, err := e.svc.pt.Slabs(ctx(), nil, e.tenant, "KA")
	if err != nil || calc.PTFromSlabs(slabs, 2, rupees(20000)) != rupees(250) || calc.PTFromSlabs(slabs, 3, rupees(20000)) != rupees(150) {
		t.Fatalf("tenant slabs: %v %v", slabs, err)
	}
	if def, _ := e.svc.pt.Slabs(ctx(), nil, e.tenant, "MH"); len(def) == 0 {
		t.Fatal("defaults when the tenant has none")
	}
	if none, _ := e.svc.pt.Slabs(ctx(), nil, e.tenant, "ZZ"); none != nil {
		t.Fatal("unknown state")
	}
	ok(t, e.svc.DeletePTSlabs(ctx(), e.maker, "KA"))
	code(t, e.svc.DeletePTSlabs(ctx(), e.maker, "KA"), "NOT_FOUND")
	code(t, e.svc.DeletePTSlabs(ctx(), e.maker, "K"), "VALIDATION_ERROR")
	e.store.fail["replace"] = errBoom
	if _, err := e.svc.SetPTSlabs(ctx(), e.maker, "KA", good); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	delete(e.store.fail, "replace")
	e.store.fail["allslabs"] = errBoom
	if _, err := e.svc.PTSlabs(ctx(), e.maker); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	e.store.fail["slabs"] = errBoom
	if _, err := e.svc.pt.Slabs(ctx(), nil, e.tenant, "KA"); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
}

func ptrMoney(m kernel.Money) *kernel.Money { return &m }
func ptrInt(i int) *int                     { return &i }

type fakeCore struct {
	place *coreAPI.Placement
	err   error
}

func (c fakeCore) PlacementOn(context.Context, uuid.UUID, uuid.UUID, time.Time) (*coreAPI.Placement, error) {
	return c.place, c.err
}
func (c fakeCore) GradeState(context.Context, uuid.UUID, uuid.UUID) (bool, bool, error) {
	return true, true, nil
}

type fakeLocs struct {
	code string
	err  error
}

func (l fakeLocs) LocationStateCode(context.Context, uuid.UUID, uuid.UUID) (string, error) {
	return l.code, l.err
}

func TestStateLookup(t *testing.T) {
	e := setup(t)
	loc := uuid.New()
	pt := NewPT(e.store)
	if s, err := pt.StateOf(ctx(), e.tenant, e.leaver, now); s != "" || err != nil {
		t.Fatal("no source yet")
	}
	pt.SetStates(StatesFromCore(fakeCore{place: &coreAPI.Placement{LocationID: &loc}}, fakeLocs{code: "ka"}))
	if s, err := pt.StateOf(ctx(), e.tenant, e.leaver, now); s != "KA" || err != nil {
		t.Fatal(s, err)
	}
	for _, c := range []struct {
		core fakeCore
		locs fakeLocs
		want string
		err  bool
	}{{fakeCore{}, fakeLocs{code: "KA"}, "", false}, {fakeCore{place: &coreAPI.Placement{}}, fakeLocs{code: "KA"}, "", false},
		{fakeCore{place: &coreAPI.Placement{LocationID: &loc}}, fakeLocs{code: "KARN"}, "", false},
		{fakeCore{err: errBoom}, fakeLocs{}, "", true}, {fakeCore{place: &coreAPI.Placement{LocationID: &loc}}, fakeLocs{err: errBoom}, "", true}} {
		pt.SetStates(StatesFromCore(c.core, c.locs))
		s, err := pt.StateOf(ctx(), e.tenant, e.leaver, now)
		if s != c.want || (err != nil) != c.err {
			t.Fatalf("%+v: %q %v", c, s, err)
		}
	}
}

// --- settlements (PP-06..PP-08) ---

func TestSettlementLifecycle(t *testing.T) {
	e := setup(t)
	code(t, mustErr(e.svc.Calculate(ctx(), e.maker, "x")), "VALIDATION_ERROR")
	code(t, mustErr(e.svc.Calculate(ctx(), e.maker, uuid.NewString())), "EMPLOYEE_NOT_FOUND")
	code(t, mustErr(e.svc.Calculate(ctx(), e.maker, e.active.String())), "NOT_A_LEAVER")

	set, err := e.svc.Calculate(ctx(), e.maker, e.leaver.String())
	ok(t, err)
	// 7 completed years: 52,000 Ã— 15 / 26 Ã— 7 = 210,000; leave 5 days Ã— 2,000; encashment 1 day; notice short by 30 days = 104,000.
	if set.Gratuity != rupees(210000) || set.LeaveEncashment != rupees(10000) || set.Encashments != rupees(2000) || set.NoticeRecovery != rupees(104000) ||
		set.Net != rupees(118000) || set.ServiceYears != 7 || set.Status != StatusDraft || len(set.Lines) != 4 {
		t.Fatalf("PG5 settlement: %+v", set)
	}
	if set.Lines[3].Kind != "deduction" || set.Lines[0].Code != "GRATUITY" || len(set.EncashmentIDs) != 1 {
		t.Fatalf("lines: %+v", set.Lines)
	}

	// A second calculation refreshes the draft instead of creating another.
	e.leave.balances[0].Days = 1000
	again, err := e.svc.Calculate(ctx(), e.maker, e.leaver.String())
	if err != nil || again.ID != set.ID || again.LeaveEncashment != rupees(20000) || len(e.store.settlements) != 1 {
		t.Fatal(again, err)
	}
	re, err := e.svc.Recalculate(ctx(), e.maker, set.ID)
	ok(t, err)
	if re.ID != set.ID {
		t.Fatal("recalculate keeps the id")
	}
	code(t, mustErr(e.svc.Recalculate(ctx(), e.maker, uuid.New())), "NOT_FOUND")

	// Approval: never the calculator, never before clearance.
	code(t, mustErr(e.svc.Approve(ctx(), e.maker, set.ID)), "PAY_SELF_APPROVAL")
	e.clearance.done = false
	code(t, mustErr(e.svc.Approve(ctx(), e.checker, set.ID)), "CLEARANCE_PENDING")
	code(t, mustErr(e.svc.Pay(ctx(), e.checker, set.ID)), "SETTLEMENT_STATE")
	e.clearance.done = true
	e.clearance.err = errBoom
	if _, err := e.svc.Approve(ctx(), e.checker, set.ID); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	e.clearance.err = nil
	approved, err := e.svc.Approve(ctx(), e.checker, set.ID)
	if err != nil || approved.Status != StatusApproved || *approved.ApprovedByUserID != e.checker.UserID {
		t.Fatal(approved, err)
	}
	code(t, mustErr(e.svc.Approve(ctx(), e.checker, set.ID)), "SETTLEMENT_STATE")
	code(t, mustErr(e.svc.Recalculate(ctx(), e.maker, set.ID)), "SETTLEMENT_STATE")
	code(t, mustErr(e.svc.Calculate(ctx(), e.maker, e.leaver.String())), "CONFLICT")

	// Paying marks the encashments paid once.
	e.leave.payErr = errBoom
	if _, err := e.svc.Pay(ctx(), e.checker, set.ID); !errors.Is(err, errBoom) || e.store.settlements[set.ID].Status != StatusApproved {
		t.Fatal("LP failure rolls the payment back", err)
	}
	e.leave.payErr = nil
	paid, err := e.svc.Pay(ctx(), e.checker, set.ID)
	if err != nil || paid.Status != StatusPaid || paid.PaidAt == nil || len(e.leave.paid) != 1 {
		t.Fatalf("PG6 paid: %+v %v", paid, err)
	}
	code(t, mustErr(e.svc.Pay(ctx(), e.checker, set.ID)), "SETTLEMENT_STATE")
	code(t, mustErr(e.svc.Cancel(ctx(), e.checker, set.ID)), "SETTLEMENT_STATE")
	if len(e.leave.paid) != 1 || e.store.events[len(e.store.events)-1] != "payroll.settlement.paid" {
		t.Fatal("encashments are marked once; events carry ids")
	}

	// Reads.
	got, err := e.svc.Get(ctx(), e.maker, set.ID)
	if err != nil || len(got.Lines) != 4 {
		t.Fatal(got, err)
	}
	code(t, mustErr(e.svc.Get(ctx(), e.maker, uuid.New())), "NOT_FOUND")
	if rows, err := e.svc.List(ctx(), e.maker, &e.leaver, StatusPaid); err != nil || len(rows) != 1 {
		t.Fatal(rows, err)
	}
	code(t, mustErr(e.svc.List(ctx(), e.maker, nil, "weird")), "VALIDATION_ERROR")
	mine := Actor{TenantID: e.tenant, UserID: e.emps.byID[e.leaver].UserID}
	if rows, err := e.svc.Mine(ctx(), mine); err != nil || len(rows) != 1 || len(rows[0].Lines) != 4 {
		t.Fatal(rows, err)
	}
	code(t, mustErr(e.svc.Mine(ctx(), Actor{TenantID: e.tenant, UserID: uuid.New()})), "EMPLOYEE_NOT_FOUND")

	// A cancelled settlement frees the worker for a new one.
	e.store.settlements = map[uuid.UUID]Settlement{}
	d, err := e.svc.Calculate(ctx(), e.maker, e.leaver.String())
	ok(t, err)
	c, err := e.svc.Cancel(ctx(), e.maker, d.ID)
	if err != nil || c.Status != StatusCancelled {
		t.Fatal(c, err)
	}
	if _, err := e.svc.Calculate(ctx(), e.maker, e.leaver.String()); err != nil {
		t.Fatal(err)
	}
}

func TestSettlementRules(t *testing.T) {
	e := setup(t)
	// A worker flagged by exit date alone (not terminated yet) is a leaver; a worker with neither is not.
	exit := time.Date(2025, 6, 30, 0, 0, 0, 0, time.UTC)
	l := e.emps.byID[e.leaver]
	l.Status, l.Employment.ExitDate = "active", &exit
	e.emps.byID[e.leaver] = l
	if _, err := e.svc.Calculate(ctx(), e.maker, e.leaver.String()); err != nil {
		t.Fatal(err)
	}
	// Terminated without an exit date settles as of today.
	noExit := e.emps.byID[e.leaver]
	noExit.Status, noExit.Employment.ExitDate, noExit.Employment.ResignationDate = "terminated", nil, nil
	e.emps.byID[e.leaver] = noExit
	e.store.settlements = map[uuid.UUID]Settlement{}
	if set, err := e.svc.Calculate(ctx(), e.maker, e.leaver.String()); err != nil || set.LastWorkingDay.Format(DateLayout) != "2026-01-10" || set.NoticeRecovery != 0 {
		t.Fatal(set, err)
	}
	noJoin := e.emps.byID[e.leaver]
	noJoin.Employment = nil
	e.emps.byID[e.leaver] = noJoin
	code(t, mustErr(e.svc.Calculate(ctx(), e.maker, e.leaver.String())), "NOT_A_LEAVER")

	e = setup(t)
	e.assigns.byEmployee[e.leaver] = nil
	e.svc.p.Assignments = e.assigns
	code(t, mustErr(e.svc.Calculate(ctx(), e.maker, e.leaver.String())), "NO_ASSIGNMENT")
}

func TestSettlementFailures(t *testing.T) {
	e := setup(t)
	for _, k := range []string{"open", "create", "replacelines"} {
		e.store.fail[k] = errBoom
		if _, err := e.svc.Calculate(ctx(), e.maker, e.leaver.String()); !errors.Is(err, errBoom) {
			t.Fatal(k, err)
		}
		delete(e.store.fail, k)
	}
	if len(e.store.settlements) != 0 {
		t.Fatal("failed calculations roll back")
	}
	e.leave.err = errBoom
	if _, err := e.svc.Calculate(ctx(), e.maker, e.leaver.String()); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	e.leave.err = nil
	e.structs.err = errBoom
	e.svc.p.Structures = e.structs
	if _, err := e.svc.Calculate(ctx(), e.maker, e.leaver.String()); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	e.structs.err = nil
	e.svc.p.Structures = e.structs
	e.assigns.err = errBoom
	e.svc.p.Assignments = e.assigns
	if _, err := e.svc.Calculate(ctx(), e.maker, e.leaver.String()); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	e.assigns.err = nil
	e.svc.p.Assignments = e.assigns
	e.emps.err = errBoom
	if _, err := e.svc.Calculate(ctx(), e.maker, e.leaver.String()); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	e.emps.err = nil
	set, err := e.svc.Calculate(ctx(), e.maker, e.leaver.String())
	ok(t, err)
	for _, k := range []string{"settlement", "save"} {
		e.store.fail[k] = errBoom
		if _, err := e.svc.Approve(ctx(), e.checker, set.ID); !errors.Is(err, errBoom) {
			t.Fatal(k, err)
		}
		delete(e.store.fail, k)
	}
	e.store.fail["event"] = errBoom
	if _, err := e.svc.Approve(ctx(), e.checker, set.ID); !errors.Is(err, errBoom) || e.store.settlements[set.ID].Status != StatusDraft {
		t.Fatal("event failure rolls the approval back", err)
	}
	delete(e.store.fail, "event")
	e.store.fail["lines"] = errBoom
	if _, err := e.svc.Get(ctx(), e.maker, set.ID); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	delete(e.store.fail, "lines")
	e.store.fail["list"] = errBoom
	if _, err := e.svc.List(ctx(), e.maker, nil, ""); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	mine := Actor{TenantID: e.tenant, UserID: e.emps.byID[e.leaver].UserID}
	if _, err := e.svc.Mine(ctx(), mine); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	delete(e.store.fail, "list")
	e.emps.err = errBoom
	if _, err := e.svc.Mine(ctx(), mine); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
}

func TestModelsAndHelpers(t *testing.T) {
	ids := UUIDList{uuid.New()}
	v, err := ids.Value()
	ok(t, err)
	var back UUIDList
	if back.Scan(v) != nil || back.Scan([]byte(v.(string))) != nil || len(back) != 1 || back.Scan(nil) != nil || back != nil || back.Scan(1) == nil {
		t.Fatal("ids")
	}
	var empty UUIDList
	if v, _ := empty.Value(); v != "[]" {
		t.Fatal(v)
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
	for _, m := range []interface{ TableName() string }{Settlement{}, SettlementLine{}} {
		if !strings.HasPrefix(m.TableName(), "payroll_settlement") {
			t.Fatal(m.TableName())
		}
	}
	for _, m := range []interface{ BeforeCreate(*gorm.DB) error }{&Settlement{}, &SettlementLine{}} {
		ok(t, m.BeforeCreate(nil))
	}
	m := NewModule(nil, NewPTFor(nil), Ports{})
	m.SetClearance(&fakeClearance{})
	if len(m.RegisterModels()) != 2 || m.Service() == nil || NewStore(nil).DB() != nil || orNotFound(errBoom) != errBoom {
		t.Fatal("module")
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
func (g guards) Flag(_, _, key string) gin.HandlerFunc {
	return func(c *gin.Context) { c.Set(key, true); c.Next() }
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
	(&Module{svc: e.svc, pt: e.svc.pt}).RegisterRoutes(r.Group(""), g)
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
	maker, checker := router(e, e.maker.UserID), router(e, e.checker.UserID)
	worker := router(e, e.emps.byID[e.leaver].UserID, "read", "manage", "approve")
	const B = "/payroll"
	expect := func(w *httptest.ResponseRecorder, status int, label string) string {
		t.Helper()
		if w.Code != status {
			t.Fatalf("%s: %d %s", label, w.Code, w.Body.String())
		}
		return w.Body.String()
	}
	expect(do(router(e, uuid.Nil), "GET", B+"/settlements/me", ""), 401, "unauthenticated")
	expect(do(maker, "PUT", B+"/pt-slabs/KA", `{"slabs":[{"min_gross":0,"amount":100}]}`), 200, "slabs")
	expect(do(maker, "PUT", B+"/pt-slabs/KA", "{"), 400, "bad body")
	expect(do(maker, "PUT", B+"/pt-slabs/KA", `{"slabs":[]}`), 400, "no slabs")
	expect(do(maker, "GET", B+"/pt-slabs", ""), 200, "list slabs")
	expect(do(maker, "DELETE", B+"/pt-slabs/KA", ""), 204, "delete slabs")
	expect(do(maker, "DELETE", B+"/pt-slabs/KA", ""), 404, "delete again")
	expect(do(worker, "PUT", B+"/pt-slabs/KA", `{}`), 403, "worker cannot manage")

	out := expect(do(maker, "POST", B+"/settlements", `{"employee_id":"`+e.leaver.String()+`"}`), 201, "settlement")
	sid := out[strings.Index(out, `"id":"`)+6:][:36]
	expect(do(maker, "POST", B+"/settlements", "{"), 400, "bad settlement")
	expect(do(maker, "POST", B+"/settlements", `{"employee_id":"`+e.active.String()+`"}`), 409, "not a leaver")
	expect(do(maker, "GET", B+"/settlements?employee_id="+e.leaver.String()+"&status=draft", ""), 200, "list")
	expect(do(maker, "GET", B+"/settlements?employee_id=x", ""), 400, "bad filter")
	expect(do(maker, "GET", B+"/settlements/"+sid, ""), 200, "get")
	expect(do(maker, "GET", B+"/settlements/x", ""), 400, "bad id")
	expect(do(maker, "POST", B+"/settlements/"+sid+"/recalculate", ""), 200, "recalculate")
	expect(do(maker, "POST", B+"/settlements/"+sid+"/approve", ""), 403, "self approval")
	expect(do(checker, "POST", B+"/settlements/"+sid+"/approve", ""), 200, "approve")
	expect(do(checker, "POST", B+"/settlements/"+sid+"/pay", ""), 200, "pay")
	expect(do(checker, "POST", B+"/settlements/"+sid+"/pay", ""), 409, "pay twice")
	expect(do(checker, "POST", B+"/settlements/"+sid+"/cancel", ""), 409, "cancel paid")
	expect(do(worker, "GET", B+"/settlements/me", ""), 200, "worker sees the paid settlement")
	expect(do(worker, "POST", B+"/settlements/"+sid+"/approve", ""), 403, "worker cannot approve")
}

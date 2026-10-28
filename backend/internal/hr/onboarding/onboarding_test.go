package onboarding

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
	recruitmentAPI "github.com/jaas/jaas/internal/hr/recruitment/api"
	"github.com/jaas/jaas/internal/identity/access"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"github.com/jaas/jaas/internal/shared/identityport"
	"gorm.io/gorm"
)

var errBoom = errors.New("boom")

// --- fakes ---

type fakeStore struct {
	templates  map[uuid.UUID]Template
	ttasks     map[uuid.UUID][]TemplateTask
	checklists map[uuid.UUID]Checklist
	tasks      map[uuid.UUID]Task
	clearances map[uuid.UUID][]Clearance
	exits      map[uuid.UUID]ExitInterview
	events     []string
	fail       map[string]error
	seq        int
}

func newStore() *fakeStore {
	return &fakeStore{templates: map[uuid.UUID]Template{}, ttasks: map[uuid.UUID][]TemplateTask{}, checklists: map[uuid.UUID]Checklist{},
		tasks: map[uuid.UUID]Task{}, clearances: map[uuid.UUID][]Clearance{}, exits: map[uuid.UUID]ExitInterview{}, fail: map[string]error{}}
}

func clone[K comparable, V any](m map[K]V) map[K]V {
	out := make(map[K]V, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func cloneCl(m map[uuid.UUID][]Clearance) map[uuid.UUID][]Clearance {
	out := map[uuid.UUID][]Clearance{}
	for k, v := range m {
		out[k] = append([]Clearance{}, v...)
	}
	return out
}

func (f *fakeStore) InTx(_ context.Context, fn func(*gorm.DB) error) error {
	t, tt, c, ts, cl, ex, ev := clone(f.templates), clone(f.ttasks), clone(f.checklists), clone(f.tasks), cloneCl(f.clearances), clone(f.exits), len(f.events)
	if err := fn(nil); err != nil {
		f.templates, f.ttasks, f.checklists, f.tasks, f.clearances, f.exits, f.events = t, tt, c, ts, cl, ex, f.events[:ev]
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
	case *Template:
		_ = v.BeforeCreate(nil)
		f.seq++
		if op == "create" {
			v.CreatedAt = v.CreatedAt.Add(time.Duration(f.seq) * time.Second)
		}
		f.templates[v.ID] = *v
	case *Checklist:
		_ = v.BeforeCreate(nil)
		f.checklists[v.ID] = *v
	case *Task:
		_ = v.BeforeCreate(nil)
		f.tasks[v.ID] = *v
	case *Clearance:
		rows := f.clearances[v.ChecklistID]
		for i := range rows {
			if rows[i].Area == v.Area {
				rows[i] = *v
				return nil
			}
		}
		f.clearances[v.ChecklistID] = append(rows, *v)
	case *ExitInterview:
		f.exits[v.ChecklistID] = *v
	}
	return nil
}
func (f *fakeStore) Template(_ context.Context, _ *gorm.DB, _, id uuid.UUID, _ bool) (*Template, error) {
	if t, ok := f.templates[id]; ok {
		return &t, f.fail["template"]
	}
	return nil, f.fail["template"]
}
func (f *fakeStore) Templates(_ context.Context, _ *gorm.DB, _ uuid.UUID, kind, status string) ([]Template, error) {
	var out []Template
	for _, t := range f.templates {
		if (kind == "" || t.Kind == kind) && (status == "" || t.Status == status) {
			out = append(out, t)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, f.fail["templates"]
}
func (f *fakeStore) TemplateTasks(_ context.Context, _ *gorm.DB, _ uuid.UUID, ids []uuid.UUID) ([]TemplateTask, error) {
	var out []TemplateTask
	for _, id := range ids {
		out = append(out, f.ttasks[id]...)
	}
	return out, f.fail["ttasks"]
}
func (f *fakeStore) ReplaceTemplateTasks(_ context.Context, _ *gorm.DB, _, id uuid.UUID, rows []TemplateTask) error {
	f.ttasks[id] = rows
	return f.fail["replace"]
}
func (f *fakeStore) Checklist(_ context.Context, _ *gorm.DB, _, id uuid.UUID, _ bool) (*Checklist, error) {
	if c, ok := f.checklists[id]; ok {
		return &c, f.fail["checklist"]
	}
	return nil, f.fail["checklist"]
}
func (f *fakeStore) find(pred func(Checklist) bool) *Checklist {
	for _, c := range f.checklists {
		if pred(c) {
			c := c
			return &c
		}
	}
	return nil
}
func (f *fakeStore) ChecklistBySource(_ context.Context, _ *gorm.DB, _ uuid.UUID, kind string, src uuid.UUID) (*Checklist, error) {
	return f.find(func(c Checklist) bool { return c.Kind == kind && c.SourceID != nil && *c.SourceID == src }), f.fail["bysource"]
}
func (f *fakeStore) OpenChecklist(_ context.Context, _ *gorm.DB, _, e uuid.UUID, kind string) (*Checklist, error) {
	return f.find(func(c Checklist) bool { return c.EmployeeProfileID == e && c.Kind == kind && c.Status == ChecklistOpen }), f.fail["open"]
}
func (f *fakeStore) LatestOffboarding(_ context.Context, _ *gorm.DB, _, e uuid.UUID) (*Checklist, error) {
	return f.find(func(c Checklist) bool {
		return c.EmployeeProfileID == e && c.Kind == KindOffboarding && c.Status != ChecklistCancelled
	}), f.fail["latest"]
}
func (f *fakeStore) Checklists(_ context.Context, _ *gorm.DB, _ uuid.UUID, fl ChecklistFilter, _, _ int) ([]Checklist, int64, error) {
	var out []Checklist
	for _, c := range f.checklists {
		if (fl.Kind == "" || c.Kind == fl.Kind) && (fl.Status == "" || c.Status == fl.Status) && (fl.EmployeeID == nil || c.EmployeeProfileID == *fl.EmployeeID) &&
			(fl.DepartmentID == nil || (c.DepartmentID != nil && *c.DepartmentID == *fl.DepartmentID)) && (fl.Involving == nil || involves(&c, fl.Involving)) {
			out = append(out, c)
		}
	}
	return out, int64(len(out)), f.fail["checklists"]
}
func (f *fakeStore) Tasks(_ context.Context, _ *gorm.DB, _, c uuid.UUID) ([]Task, error) {
	var out []Task
	for _, t := range f.tasks {
		if t.ChecklistID == c {
			out = append(out, t)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Position < out[j].Position })
	return out, f.fail["tasks"]
}
func (f *fakeStore) Task(_ context.Context, _ *gorm.DB, _, id uuid.UUID, _ bool) (*Task, error) {
	if t, ok := f.tasks[id]; ok {
		return &t, f.fail["task"]
	}
	return nil, f.fail["task"]
}
func (f *fakeStore) AssignedTasks(_ context.Context, _ *gorm.DB, _ uuid.UUID, e *uuid.UUID, pool bool) ([]Task, error) {
	var out []Task
	for _, t := range f.tasks {
		if f.checklists[t.ChecklistID].Status != ChecklistOpen {
			continue
		}
		if (e != nil && t.AssigneeEmployeeID != nil && *t.AssigneeEmployeeID == *e) || (pool && t.AssigneeEmployeeID == nil) {
			out = append(out, t)
		}
	}
	return out, f.fail["assigned"]
}
func (f *fakeStore) Clearances(_ context.Context, _ *gorm.DB, _, c uuid.UUID) ([]Clearance, error) {
	return append([]Clearance{}, f.clearances[c]...), f.fail["clearances"]
}
func (f *fakeStore) ExitInterview(_ context.Context, _ *gorm.DB, _, c uuid.UUID) (*ExitInterview, error) {
	if e, ok := f.exits[c]; ok {
		return &e, f.fail["exit"]
	}
	return nil, f.fail["exit"]
}
func (f *fakeStore) Progress(_ context.Context, _ *gorm.DB, _ uuid.UUID, kind string, today time.Time) ([]ProgressRow, error) {
	rows := map[string]*ProgressRow{}
	for _, c := range f.checklists {
		if c.Kind != kind {
			continue
		}
		key := ""
		if c.DepartmentID != nil {
			key = c.DepartmentID.String()
		}
		r := rows[key]
		if r == nil {
			r = &ProgressRow{DepartmentID: c.DepartmentID}
			rows[key] = r
		}
		switch c.Status {
		case ChecklistOpen:
			r.Open++
			for _, t := range f.tasks {
				if t.ChecklistID == c.ID && t.Status == TaskOpen {
					r.OpenTasks++
					if t.DueDate.Before(today) {
						r.OverdueTasks++
					}
				}
			}
		case ChecklistCompleted:
			r.Completed++
		}
	}
	var out []ProgressRow
	for _, r := range rows {
		out = append(out, *r)
	}
	return out, f.fail["progress"]
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

type fakeCore struct {
	place map[uuid.UUID]*coreAPI.Placement
	err   error
}

func (c *fakeCore) PlacementOn(_ context.Context, _, e uuid.UUID, _ time.Time) (*coreAPI.Placement, error) {
	return c.place[e], c.err
}
func (c *fakeCore) GradeState(context.Context, uuid.UUID, uuid.UUID) (bool, bool, error) {
	return true, true, nil
}

type fakeDocs struct {
	verified map[string]uuid.UUID
	err      error
}

func (d *fakeDocs) VerifiedDocument(_ context.Context, _ *gorm.DB, _, _ uuid.UUID, typ string) (*uuid.UUID, error) {
	if id, ok := d.verified[typ]; ok {
		return &id, d.err
	}
	return nil, d.err
}

type noAudit struct{}

func (noAudit) Log(context.Context, *gorm.DB, string, string, string, string, string, interface{}, string, string) error {
	return nil
}

var _ identityport.Auditor = noAudit{}

// --- environment ---

type env struct {
	svc                   *Service
	store                 *fakeStore
	emps                  *fakeEmployees
	core                  *fakeCore
	docs                  *fakeDocs
	tenant, dept, other   uuid.UUID
	hr, worker, mgr, peer Actor
	alice, boss, bob      uuid.UUID
}

var now = time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)

func setup(t *testing.T) *env {
	t.Helper()
	e := &env{store: newStore(), tenant: uuid.New(), dept: uuid.New(), other: uuid.New(), alice: uuid.New(), boss: uuid.New(), bob: uuid.New()}
	aliceU, bossU, bobU := uuid.New(), uuid.New(), uuid.New()
	e.emps = &fakeEmployees{byID: map[uuid.UUID]employeeDTO.EmployeeResponse{e.alice: {ID: e.alice, UserID: aliceU}, e.boss: {ID: e.boss, UserID: bossU},
		e.bob: {ID: e.bob, UserID: bobU}}}
	d, b := e.dept, e.boss
	e.core = &fakeCore{place: map[uuid.UUID]*coreAPI.Placement{e.alice: {DepartmentID: &d, ManagerEmployeeID: &b, WorkerType: "employee"}}}
	e.docs = &fakeDocs{verified: map[string]uuid.UUID{}}
	e.svc = NewService(e.store, Ports{Employees: e.emps, Core: e.core, Documents: e.docs, Audit: noAudit{}}, func() time.Time { return now })
	e.hr = Actor{TenantID: e.tenant, UserID: uuid.New(), CanRead: true, CanManage: true, CanReadEI: true}
	e.worker = Actor{TenantID: e.tenant, UserID: aliceU}
	e.mgr = Actor{TenantID: e.tenant, UserID: bossU}
	e.peer = Actor{TenantID: e.tenant, UserID: bobU}
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

// template creates an active template with tasks.
func (e *env) template(t *testing.T, kind string, in TemplateInput, tasks ...TaskInput) *Template {
	t.Helper()
	in.Kind = &kind
	if in.Name == nil {
		in.Name = str(kind + " template")
	}
	tpl, err := e.svc.CreateTemplate(ctx(), e.hr, in)
	ok(t, err)
	tpl, err = e.svc.SetTasks(ctx(), e.hr, tpl.ID, tasks)
	ok(t, err)
	return tpl
}

func (e *env) hire(t *testing.T, action uuid.UUID) *Checklist {
	t.Helper()
	ok(t, e.svc.CoreHook(ctx(), nil, e.tenant, coreAPI.ActionFact{ActionID: action, EmployeeID: e.alice, Type: "hire", EffectiveDate: now,
		Placement: e.core.place[e.alice]}))
	return e.store.find(func(c Checklist) bool { return c.SourceID != nil && *c.SourceID == action })
}

func (e *env) tasksOf(c *Checklist) []Task {
	rows, _ := e.store.Tasks(ctx(), nil, e.tenant, c.ID)
	return rows
}

// --- templates (OB-01) ---

func TestTemplates(t *testing.T) {
	e := setup(t)
	for _, in := range []TemplateInput{{}, {Kind: str("onboarding")}, {Kind: str("onboarding"), Name: str(" ")},
		{Kind: str("onboarding"), Name: str("x"), DepartmentID: str("bad")}, {Kind: str("onboarding"), Name: str("x"), WorkerType: str("robot")},
		{Kind: str("onboarding"), Name: str("x"), Status: str("weird")}} {
		code(t, mustErr(e.svc.CreateTemplate(ctx(), e.hr, in)), "VALIDATION_ERROR")
	}
	tpl, err := e.svc.CreateTemplate(ctx(), e.hr, TemplateInput{Kind: str("onboarding"), Name: str(" Engineering "), DepartmentID: str(e.dept.String()),
		WorkerType: str("employee")})
	ok(t, err)
	if tpl.Name != "Engineering" || tpl.specificity() != 2 || tpl.Status != TemplateActive {
		t.Fatalf("%+v", tpl)
	}
	code(t, mustErr(e.svc.UpdateTemplate(ctx(), e.hr, tpl.ID, TemplateInput{Kind: str("offboarding")})), "VALIDATION_ERROR")
	code(t, mustErr(e.svc.UpdateTemplate(ctx(), e.hr, uuid.New(), TemplateInput{})), "NOT_FOUND")
	code(t, mustErr(e.svc.UpdateTemplate(ctx(), e.hr, tpl.ID, TemplateInput{Name: str("")})), "VALIDATION_ERROR")
	up, err := e.svc.UpdateTemplate(ctx(), e.hr, tpl.ID, TemplateInput{WorkerType: str(""), DepartmentID: str(""), Status: str(TemplateArchived)})
	ok(t, err)
	if up.specificity() != 0 || up.Status != TemplateArchived {
		t.Fatalf("%+v", up)
	}
	bad := [][]TaskInput{nil, {{Title: ""}}, {{Title: "x", OwnerRole: "ceo"}}, {{Title: "x", OwnerRole: "hr", DueOffsetDays: 400}},
		{{Title: "x", OwnerRole: "hr", Description: str(strings.Repeat("d", 2001))}}, {{Title: "x", OwnerRole: "hr", DocumentType: str(strings.Repeat("d", 51))}},
		make([]TaskInput, 51)}
	for _, in := range bad {
		code(t, mustErr(e.svc.SetTasks(ctx(), e.hr, tpl.ID, in)), "VALIDATION_ERROR")
	}
	code(t, mustErr(e.svc.SetTasks(ctx(), e.hr, uuid.New(), []TaskInput{{Title: "x", OwnerRole: "hr"}})), "NOT_FOUND")
	got, err := e.svc.SetTasks(ctx(), e.hr, tpl.ID, []TaskInput{{Title: "Laptop", OwnerRole: "it"}, {Title: "ID proof", OwnerRole: "employee", DocumentType: str("id")}})
	if err != nil || len(got.Tasks) != 2 || got.Tasks[1].Position != 2 {
		t.Fatal(got, err)
	}
	code(t, mustErr(e.svc.GetTemplate(ctx(), e.hr, uuid.New())), "NOT_FOUND")
	if rows, err := e.svc.Templates(ctx(), e.hr, KindOnboarding, TemplateArchived); err != nil || len(rows) != 1 {
		t.Fatal(rows, err)
	}
	if rows, _ := e.svc.Templates(ctx(), e.hr, KindOffboarding, ""); len(rows) != 0 {
		t.Fatal("kind filter")
	}
	code(t, mustErr(e.svc.Templates(ctx(), e.hr, "x", "")), "VALIDATION_ERROR")
	code(t, mustErr(e.svc.Templates(ctx(), e.hr, "", "x")), "VALIDATION_ERROR")
	for _, k := range []string{"create"} {
		e.store.fail[k] = errBoom
		if _, err := e.svc.CreateTemplate(ctx(), e.hr, TemplateInput{Kind: str("onboarding"), Name: str("x")}); !errors.Is(err, errBoom) {
			t.Fatal(err)
		}
		delete(e.store.fail, k)
	}
	for _, k := range []string{"save", "template"} {
		e.store.fail[k] = errBoom
		if _, err := e.svc.UpdateTemplate(ctx(), e.hr, tpl.ID, TemplateInput{}); !errors.Is(err, errBoom) {
			t.Fatal(k, err)
		}
		delete(e.store.fail, k)
	}
	e.store.fail["replace"] = errBoom
	if _, err := e.svc.SetTasks(ctx(), e.hr, tpl.ID, []TaskInput{{Title: "x", OwnerRole: "hr"}}); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
}

// --- start (OB-02, OB-03, OB-04) ---

func TestStartFromCoreAndRecruitment(t *testing.T) {
	e := setup(t)
	e.template(t, KindOnboarding, TemplateInput{Name: str("General")}, TaskInput{Title: "General task", OwnerRole: "hr"})
	e.template(t, KindOnboarding, TemplateInput{Name: str("Dept"), DepartmentID: str(e.dept.String())},
		TaskInput{Title: "Sign contract", OwnerRole: "employee", DueOffsetDays: -3, Blocking: true},
		TaskInput{Title: "Welcome", OwnerRole: "manager", DueOffsetDays: 1}, TaskInput{Title: "Laptop", OwnerRole: "it"})
	e.template(t, KindOnboarding, TemplateInput{Name: str("Other dept"), DepartmentID: str(e.other.String())}, TaskInput{Title: "Never", OwnerRole: "hr"})
	e.template(t, KindOnboarding, TemplateInput{Name: str("Archived"), DepartmentID: str(e.dept.String()), Status: str(TemplateArchived)},
		TaskInput{Title: "Archived", OwnerRole: "hr"})

	action := uuid.New()
	c := e.hire(t, action)
	tasks := e.tasksOf(c)
	if c == nil || c.Kind != KindOnboarding || c.Source != SourceCoreAction || *c.ManagerEmployeeID != e.boss || len(tasks) != 3 {
		t.Fatalf("checklist %+v tasks %d", c, len(tasks))
	}
	if *tasks[0].AssigneeEmployeeID != e.alice || *tasks[1].AssigneeEmployeeID != e.boss || tasks[2].AssigneeEmployeeID != nil ||
		tasks[0].DueDate.Format(DateLayout) != "2026-05-29" {
		t.Fatalf("OB-04 assignees/due: %+v", tasks)
	}
	if e.store.events[0] != "hr.onboarding.checklist_started" {
		t.Fatal(e.store.events)
	}
	// OG2: the same action again and a recruitment hire for the same worker are no-ops.
	e.hire(t, action)
	ok(t, e.svc.RecruitmentHook(ctx(), nil, e.tenant, recruitmentAPI.HireFact{CandidateID: uuid.New(), EmployeeID: e.alice, JoiningDate: now}))
	if len(e.store.checklists) != 1 {
		t.Fatalf("OG2 duplicates: %d", len(e.store.checklists))
	}
	// Other action types are ignored; a rescinded hire cancels its checklist (OG8).
	ok(t, e.svc.CoreHook(ctx(), nil, e.tenant, coreAPI.ActionFact{ActionID: uuid.New(), EmployeeID: e.alice, Type: "promote"}))
	ok(t, e.svc.CoreHook(ctx(), nil, e.tenant, coreAPI.ActionFact{ActionID: action, EmployeeID: e.alice, Type: "hire", Rescinded: true}))
	if e.store.checklists[c.ID].Status != ChecklistCancelled {
		t.Fatal("OG8 rescind")
	}
	ok(t, e.svc.CoreHook(ctx(), nil, e.tenant, coreAPI.ActionFact{ActionID: action, EmployeeID: e.alice, Type: "hire", Rescinded: true}))
	ok(t, e.svc.CoreHook(ctx(), nil, e.tenant, coreAPI.ActionFact{ActionID: uuid.New(), EmployeeID: e.alice, Type: "hire", Rescinded: true}))

	// Recruitment hire without placement: only criteria-free templates apply.
	cand := uuid.New()
	ok(t, e.svc.RecruitmentHook(ctx(), nil, e.tenant, recruitmentAPI.HireFact{CandidateID: cand, EmployeeID: e.bob, JoiningDate: now}))
	rc := e.store.find(func(c Checklist) bool { return c.SourceID != nil && *c.SourceID == cand })
	if rc == nil || rc.Source != SourceRecruitment || len(e.tasksOf(rc)) != 1 || e.tasksOf(rc)[0].Title != "General task" {
		t.Fatal("recruitment start")
	}

	// Termination: offboarding anchored on the last working day with four clearances.
	lwd := now.AddDate(0, 0, 14)
	term := uuid.New()
	ok(t, e.svc.CoreHook(ctx(), nil, e.tenant, coreAPI.ActionFact{ActionID: term, EmployeeID: e.alice, Type: "terminate", EffectiveDate: now, LastWorkingDay: &lwd}))
	off := e.store.find(func(c Checklist) bool { return c.SourceID != nil && *c.SourceID == term })
	if off == nil || off.Kind != KindOffboarding || off.AnchorDate.Format(DateLayout) != "2026-06-15" || len(e.store.clearances[off.ID]) != 4 {
		t.Fatalf("offboarding: %+v", off)
	}

	// Failures roll back.
	for _, k := range []string{"lock", "bysource", "open", "templates", "ttasks", "create", "event"} {
		e.store.fail[k] = errBoom
		err := e.svc.CoreHook(ctx(), nil, e.tenant, coreAPI.ActionFact{ActionID: uuid.New(), EmployeeID: e.bob, Type: "terminate", EffectiveDate: now})
		if !errors.Is(err, errBoom) {
			t.Fatal(k, err)
		}
		delete(e.store.fail, k)
	}
	e.store.fail["bysource"] = errBoom
	if err := e.svc.CoreHook(ctx(), nil, e.tenant, coreAPI.ActionFact{ActionID: term, Type: "terminate", Rescinded: true}); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
}

func TestMostSpecificWins(t *testing.T) {
	e := setup(t)
	e.template(t, KindOnboarding, TemplateInput{Name: str("Dept")}, TaskInput{Title: "dept", OwnerRole: "hr"})
	e.template(t, KindOnboarding, TemplateInput{Name: str("Dept+type A"), DepartmentID: str(e.dept.String()), WorkerType: str("employee")},
		TaskInput{Title: "A", OwnerRole: "hr"})
	e.template(t, KindOnboarding, TemplateInput{Name: str("Dept+type B"), DepartmentID: str(e.dept.String()), WorkerType: str("employee")},
		TaskInput{Title: "B", OwnerRole: "hr"})
	e.template(t, KindOnboarding, TemplateInput{Name: str("Intern"), WorkerType: str("intern")}, TaskInput{Title: "intern", OwnerRole: "hr"})
	tasks := e.tasksOf(e.hire(t, uuid.New()))
	if len(tasks) != 2 || tasks[0].Title != "A" || tasks[1].Title != "B" {
		t.Fatalf("OG3: %+v", tasks)
	}
	if matches(&Template{JobProfileID: &e.dept}, nil) {
		t.Fatal("criteria need a placement")
	}
}

func TestManualStart(t *testing.T) {
	e := setup(t)
	for _, in := range []StartInput{{EmployeeID: "x"}, {EmployeeID: e.alice.String(), Kind: "x"}, {EmployeeID: e.alice.String(), Kind: KindOnboarding, AnchorDate: "x"}} {
		code(t, mustErr(e.svc.Start(ctx(), e.hr, in)), "VALIDATION_ERROR")
	}
	code(t, mustErr(e.svc.Start(ctx(), e.hr, StartInput{EmployeeID: uuid.NewString(), Kind: KindOnboarding, AnchorDate: "2026-06-01"})), "EMPLOYEE_NOT_FOUND")
	v, err := e.svc.Start(ctx(), e.hr, StartInput{EmployeeID: e.alice.String(), Kind: KindOnboarding, AnchorDate: "2026-06-01"})
	if err != nil || v.Source != SourceManual || v.Tasks == nil || v.Clearances == nil {
		t.Fatal(v, err)
	}
	code(t, mustErr(e.svc.Start(ctx(), e.hr, StartInput{EmployeeID: e.alice.String(), Kind: KindOnboarding, AnchorDate: "2026-06-01"})), "CONFLICT")
	e.core.err = errBoom
	if _, err := e.svc.Start(ctx(), e.hr, StartInput{EmployeeID: e.bob.String(), Kind: KindOnboarding, AnchorDate: "2026-06-01"}); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	e.core.err, e.emps.err = nil, errBoom
	if _, err := e.svc.Start(ctx(), e.hr, StartInput{EmployeeID: e.bob.String(), Kind: KindOnboarding, AnchorDate: "2026-06-01"}); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	e.emps.err = nil
	e.store.fail["create"] = errBoom
	if _, err := e.svc.Start(ctx(), e.hr, StartInput{EmployeeID: e.bob.String(), Kind: KindOnboarding, AnchorDate: "2026-06-01"}); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
}

// --- tasks and completion (OB-05, OB-06) ---

func TestTasksAndCompletion(t *testing.T) {
	e := setup(t)
	e.template(t, KindOnboarding, TemplateInput{},
		TaskInput{Title: "Upload PAN", OwnerRole: "employee", DocumentType: str("pan"), Blocking: true},
		TaskInput{Title: "Welcome lunch", OwnerRole: "manager", DueOffsetDays: -10},
		TaskInput{Title: "Laptop", OwnerRole: "it"})
	c := e.hire(t, uuid.New())
	tasks := e.tasksOf(c)
	pan, lunch, laptop := tasks[0], tasks[1], tasks[2]

	code(t, mustErr(e.svc.Done(ctx(), e.peer, pan.ID, nil)), "NOT_FOUND")
	code(t, mustErr(e.svc.Done(ctx(), e.worker, laptop.ID, nil)), "NOT_FOUND") // pool task without manage
	code(t, mustErr(e.svc.Done(ctx(), e.worker, uuid.New(), nil)), "NOT_FOUND")
	code(t, mustErr(e.svc.Done(ctx(), e.worker, pan.ID, str(strings.Repeat("n", 1001)))), "VALIDATION_ERROR")
	code(t, mustErr(e.svc.Done(ctx(), e.worker, pan.ID, nil)), "DOCUMENT_REQUIRED")
	docID := uuid.New()
	e.docs.verified["pan"] = docID
	done, err := e.svc.Done(ctx(), e.worker, pan.ID, str(" uploaded "))
	if err != nil || done.Status != TaskDone || *done.EvidenceDocumentID != docID || *done.Note != "uploaded" {
		t.Fatal(done, err)
	}
	code(t, mustErr(e.svc.Done(ctx(), e.worker, pan.ID, nil)), "TASK_STATE")

	// Views while open.
	mine, err := e.svc.MyTasks(ctx(), e.mgr)
	if err != nil || len(mine) != 1 || mine[0].ID != lunch.ID {
		t.Fatal(mine, err)
	}
	if pool, _ := e.svc.MyTasks(ctx(), e.hr); len(pool) != 1 || pool[0].ID != laptop.ID {
		t.Fatal("pool for manage")
	}
	v, err := e.svc.Get(ctx(), e.mgr, c.ID)
	if err != nil || v.Progress.Done != 1 || v.Progress.Open != 2 || v.Progress.Overdue != 1 {
		t.Fatalf("%+v %v", v.Progress, err)
	}
	code(t, mustErr(e.svc.Get(ctx(), e.peer, c.ID)), "NOT_FOUND")
	code(t, mustErr(e.svc.Get(ctx(), e.peer, uuid.New())), "NOT_FOUND")

	// Skip and reopen.
	code(t, mustErr(e.svc.Skip(ctx(), e.hr, lunch.ID, nil)), "VALIDATION_ERROR")
	code(t, mustErr(e.svc.Skip(ctx(), e.hr, lunch.ID, str(" "))), "VALIDATION_ERROR")
	code(t, mustErr(e.svc.Reopen(ctx(), e.hr, lunch.ID)), "TASK_STATE")
	_, err = e.svc.Skip(ctx(), e.hr, lunch.ID, str("remote"))
	ok(t, err)
	code(t, mustErr(e.svc.Skip(ctx(), e.hr, lunch.ID, str("again"))), "TASK_STATE")
	_, err = e.svc.Reopen(ctx(), e.hr, pan.ID)
	ok(t, err)
	code(t, mustErr(e.svc.Skip(ctx(), e.hr, pan.ID, str("no"))), "TASK_BLOCKING")
	code(t, mustErr(e.svc.Complete(ctx(), e.hr, c.ID)), "BLOCKING_OPEN")
	_, err = e.svc.Done(ctx(), e.hr, pan.ID, nil)
	ok(t, err)
	// The last open task completes the checklist automatically.
	_, err = e.svc.Done(ctx(), e.hr, laptop.ID, nil)
	ok(t, err)
	if got := e.store.checklists[c.ID]; got.Status != ChecklistCompleted || got.CompletedAt == nil || e.store.events[len(e.store.events)-1] != "hr.onboarding.checklist_completed" {
		t.Fatalf("auto-complete: %+v", got)
	}
	code(t, mustErr(e.svc.Reopen(ctx(), e.hr, laptop.ID)), "CHECKLIST_STATE")
	code(t, mustErr(e.svc.Complete(ctx(), e.hr, c.ID)), "CHECKLIST_STATE")
	code(t, mustErr(e.svc.Complete(ctx(), e.hr, uuid.New())), "NOT_FOUND")
	code(t, mustErr(e.svc.Cancel(ctx(), e.hr, c.ID, str("x"))), "CHECKLIST_STATE")
	code(t, mustErr(e.svc.Cancel(ctx(), e.hr, c.ID, nil)), "VALIDATION_ERROR")
	code(t, mustErr(e.svc.Cancel(ctx(), e.hr, c.ID, str(""))), "VALIDATION_ERROR")
}

func TestEarlyCompleteAndCancel(t *testing.T) {
	e := setup(t)
	e.template(t, KindOnboarding, TemplateInput{}, TaskInput{Title: "a", OwnerRole: "hr"}, TaskInput{Title: "b", OwnerRole: "hr"})
	c := e.hire(t, uuid.New())
	v, err := e.svc.Complete(ctx(), e.hr, c.ID)
	if err != nil || v.Status != ChecklistCompleted || v.Progress.Skipped != 2 {
		t.Fatal(v, err)
	}
	c2, err := e.svc.Start(ctx(), e.hr, StartInput{EmployeeID: e.alice.String(), Kind: KindOnboarding, AnchorDate: "2026-06-01"})
	ok(t, err)
	for _, k := range []string{"tasks", "save"} {
		e.store.fail[k] = errBoom
		if _, err := e.svc.Complete(ctx(), e.hr, c2.ID); !errors.Is(err, errBoom) {
			t.Fatal(k, err)
		}
		delete(e.store.fail, k)
	}
	v, err = e.svc.Cancel(ctx(), e.hr, c2.ID, str("duplicate"))
	if err != nil || v.Status != ChecklistCancelled || *v.CloseReason != "duplicate" {
		t.Fatal(v, err)
	}
}

// --- clearance and exit interview (OB-07, OB-08) ---

func (e *env) offboard(t *testing.T) *Checklist {
	t.Helper()
	lwd := now.AddDate(0, 0, 7)
	id := uuid.New()
	ok(t, e.svc.CoreHook(ctx(), nil, e.tenant, coreAPI.ActionFact{ActionID: id, EmployeeID: e.alice, Type: "terminate", EffectiveDate: now, LastWorkingDay: &lwd,
		Placement: e.core.place[e.alice]}))
	return e.store.find(func(c Checklist) bool { return c.SourceID != nil && *c.SourceID == id })
}

func TestClearance(t *testing.T) {
	e := setup(t)
	e.template(t, KindOffboarding, TemplateInput{}, TaskInput{Title: "Return laptop", OwnerRole: "it"})
	c := e.offboard(t)
	code(t, mustErr(e.svc.Clear(ctx(), e.hr, c.ID, "legal", nil)), "VALIDATION_ERROR")
	code(t, mustErr(e.svc.Clear(ctx(), e.hr, c.ID, "hr", str(strings.Repeat("n", 1001)))), "VALIDATION_ERROR")
	code(t, mustErr(e.svc.Clear(ctx(), e.worker, c.ID, "hr", nil)), "FORBIDDEN")
	code(t, mustErr(e.svc.Clear(ctx(), e.peer, c.ID, "hr", nil)), "NOT_FOUND")
	code(t, mustErr(e.svc.Clear(ctx(), e.mgr, c.ID, "it", nil)), "FORBIDDEN")
	code(t, mustErr(e.svc.Clear(ctx(), e.hr, uuid.New(), "hr", nil)), "NOT_FOUND")
	_, err := e.svc.Clear(ctx(), e.mgr, c.ID, AreaManager, str("handover done"))
	ok(t, err)
	code(t, mustErr(e.svc.Clear(ctx(), e.hr, c.ID, AreaManager, nil)), "ALREADY_CLEARED")
	code(t, mustErr(e.svc.Complete(ctx(), e.hr, c.ID)), "CLEARANCE_PENDING")
	if done, _ := e.svc.ClearanceCompleted(ctx(), nil, e.tenant, e.alice); done {
		t.Fatal("OG6 clearance not complete yet")
	}
	for _, area := range []string{"hr", "it"} {
		_, err := e.svc.Clear(ctx(), e.hr, c.ID, area, nil)
		ok(t, err)
	}
	v, err := e.svc.Clear(ctx(), e.hr, c.ID, "finance", nil)
	if err != nil || v.ClearanceCompletedAt == nil || v.Status != ChecklistOpen {
		t.Fatal(v, err)
	}
	if done, _ := e.svc.ClearanceCompleted(ctx(), nil, e.tenant, e.alice); !done {
		t.Fatal("OG6 port")
	}
	found := false
	for _, ev := range e.store.events {
		found = found || ev == "hr.onboarding.clearance_completed"
	}
	if !found {
		t.Fatal("clearance event")
	}
	// The last task completes the offboarding checklist now that clearance is done.
	_, err = e.svc.Done(ctx(), e.hr, e.tasksOf(c)[0].ID, nil)
	ok(t, err)
	if e.store.checklists[c.ID].Status != ChecklistCompleted {
		t.Fatal("offboarding auto-complete")
	}

	on := e.hire(t, uuid.New())
	code(t, mustErr(e.svc.Clear(ctx(), e.hr, on.ID, "hr", nil)), "CHECKLIST_STATE")

	// Failures.
	c2 := setup(t)
	c2.template(t, KindOffboarding, TemplateInput{}, TaskInput{Title: "t", OwnerRole: "hr"})
	off := c2.offboard(t)
	for _, k := range []string{"clearances", "save", "checklist"} {
		c2.store.fail[k] = errBoom
		if _, err := c2.svc.Clear(ctx(), c2.hr, off.ID, "hr", nil); !errors.Is(err, errBoom) {
			t.Fatal(k, err)
		}
		delete(c2.store.fail, k)
	}
	for _, a := range []string{"manager", "hr", "it"} {
		_, err := c2.svc.Clear(ctx(), c2.hr, off.ID, a, nil)
		ok(t, err)
	}
	c2.store.fail["event"] = errBoom
	if _, err := c2.svc.Clear(ctx(), c2.hr, off.ID, "finance", nil); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	delete(c2.store.fail, "event")
	c2.emps.err = errBoom
	if _, err := c2.svc.Clear(ctx(), c2.mgr, off.ID, "finance", nil); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	c2.emps.err = nil
	c2.store.fail["latest"] = errBoom
	if _, err := c2.svc.ClearanceCompleted(ctx(), nil, c2.tenant, c2.alice); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
}

func TestExitInterview(t *testing.T) {
	e := setup(t)
	c := e.offboard(t)
	many := Answers{}
	for i := 0; i < 51; i++ {
		many[uuid.NewString()] = "x"
	}
	for _, in := range []Answers{nil, many, {" ": "x"}, {"q": strings.Repeat("a", 2001)}} {
		code(t, mustErr(e.svc.SubmitExit(ctx(), e.worker, c.ID, in)), "VALIDATION_ERROR")
	}
	code(t, mustErr(e.svc.SubmitExit(ctx(), e.mgr, c.ID, Answers{"why": "x"})), "NOT_FOUND")
	code(t, mustErr(e.svc.GetExit(ctx(), e.worker, c.ID)), "NOT_FOUND") // nothing submitted yet
	ei, err := e.svc.SubmitExit(ctx(), e.worker, c.ID, Answers{" why ": " growth "})
	if err != nil || ei.Answers["why"] != "growth" {
		t.Fatal(ei, err)
	}
	code(t, mustErr(e.svc.SubmitExit(ctx(), e.worker, c.ID, Answers{"why": "again"})), "ALREADY_SUBMITTED")
	if got, err := e.svc.GetExit(ctx(), e.worker, c.ID); err != nil || got.Answers["why"] != "growth" {
		t.Fatal(got, err)
	}
	if _, err := e.svc.GetExit(ctx(), e.hr, c.ID); err != nil {
		t.Fatal(err)
	}
	code(t, mustErr(e.svc.GetExit(ctx(), e.mgr, c.ID)), "NOT_FOUND")
	reader := Actor{TenantID: e.tenant, UserID: uuid.New(), CanRead: true}
	code(t, mustErr(e.svc.GetExit(ctx(), reader, c.ID)), "NOT_FOUND")
	code(t, mustErr(e.svc.GetExit(ctx(), e.hr, uuid.New())), "NOT_FOUND")
	on := e.hire(t, uuid.New())
	code(t, mustErr(e.svc.SubmitExit(ctx(), e.worker, on.ID, Answers{"why": "x"})), "CHECKLIST_STATE")
	e.store.fail["exit"] = errBoom
	off2 := setup(t)
	_ = off2
	if _, err := e.svc.GetExit(ctx(), e.hr, c.ID); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	e.emps.err = errBoom
	if _, err := e.svc.GetExit(ctx(), e.worker, c.ID); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	if _, err := e.svc.SubmitExit(ctx(), e.worker, c.ID, Answers{"a": "b"}); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
}

// --- views (OB-09, OB-10) ---

func TestViews(t *testing.T) {
	e := setup(t)
	e.template(t, KindOnboarding, TemplateInput{}, TaskInput{Title: "a", OwnerRole: "employee", DueOffsetDays: -5})
	c := e.hire(t, uuid.New())
	if rows, total, err := e.svc.Checklists(ctx(), e.hr, ChecklistFilter{Kind: KindOnboarding, Status: ChecklistOpen, DepartmentID: &e.dept}, 1, 20); err != nil || total != 1 || len(rows) != 1 {
		t.Fatal(total, err)
	}
	if rows, _, _ := e.svc.Checklists(ctx(), e.mgr, ChecklistFilter{}, 1, 20); len(rows) != 1 {
		t.Fatal("manager sees reports")
	}
	if rows, _, _ := e.svc.Checklists(ctx(), e.peer, ChecklistFilter{}, 1, 20); len(rows) != 0 {
		t.Fatal("peer sees nothing")
	}
	if rows, _, _ := e.svc.Checklists(ctx(), Actor{TenantID: e.tenant, UserID: uuid.New()}, ChecklistFilter{}, 1, 20); len(rows) != 0 {
		t.Fatal("no profile sees nothing")
	}
	code(t, mustErr3(e.svc.Checklists(ctx(), e.hr, ChecklistFilter{Kind: "x"}, 1, 20)), "VALIDATION_ERROR")
	code(t, mustErr3(e.svc.Checklists(ctx(), e.hr, ChecklistFilter{Status: "x"}, 1, 20)), "VALIDATION_ERROR")
	me, err := e.svc.Me(ctx(), e.worker)
	if err != nil || len(me) != 1 || me[0].ID != c.ID || len(me[0].Tasks) != 1 {
		t.Fatal(me, err)
	}
	code(t, mustErr(e.svc.Me(ctx(), Actor{TenantID: e.tenant, UserID: uuid.New()})), "EMPLOYEE_NOT_FOUND")
	rows, err := e.svc.Progress(ctx(), e.hr, "")
	if err != nil || len(rows) != 1 || rows[0].Open != 1 || rows[0].OverdueTasks != 1 {
		t.Fatal(rows, err)
	}
	code(t, mustErr(e.svc.Progress(ctx(), e.hr, "x")), "VALIDATION_ERROR")
	for _, k := range []string{"tasks", "clearances"} {
		e.store.fail[k] = errBoom
		if _, err := e.svc.Me(ctx(), e.worker); !errors.Is(err, errBoom) {
			t.Fatal(k, err)
		}
		delete(e.store.fail, k)
	}
	e.store.fail["checklists"] = errBoom
	if _, err := e.svc.Me(ctx(), e.worker); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	delete(e.store.fail, "checklists")
	e.emps.err = errBoom
	for _, f := range []func() error{
		func() error { return mustErr(e.svc.Me(ctx(), e.worker)) },
		func() error { return mustErr(e.svc.MyTasks(ctx(), e.worker)) },
		func() error { return mustErr(e.svc.Get(ctx(), e.worker, c.ID)) },
		func() error { return mustErr3(e.svc.Checklists(ctx(), e.worker, ChecklistFilter{}, 1, 20)) },
		func() error { return mustErr(e.svc.Done(ctx(), e.worker, uuid.New(), nil)) },
	} {
		if err := f(); !errors.Is(err, errBoom) {
			t.Fatal(err)
		}
	}
}

func mustErr3(_ interface{}, _ int64, err error) error { return err }

func TestTaskFailures(t *testing.T) {
	e := setup(t)
	e.template(t, KindOnboarding, TemplateInput{}, TaskInput{Title: "doc", OwnerRole: "hr", DocumentType: str("pan")}, TaskInput{Title: "b", OwnerRole: "hr"})
	c := e.hire(t, uuid.New())
	task := e.tasksOf(c)[0]
	e.docs.err = errBoom
	if _, err := e.svc.Done(ctx(), e.hr, task.ID, nil); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	e.docs.err = nil
	e.docs.verified["pan"] = uuid.New()
	for _, k := range []string{"task", "checklist", "save", "tasks"} {
		e.store.fail[k] = errBoom
		if _, err := e.svc.Done(ctx(), e.hr, task.ID, nil); !errors.Is(err, errBoom) {
			t.Fatal(k, err)
		}
		delete(e.store.fail, k)
	}
	e.store.fail["checklist"] = errBoom
	if _, err := e.svc.Get(ctx(), e.hr, c.ID); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	if _, err := e.svc.Cancel(ctx(), e.hr, c.ID, str("x")); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
}

func TestModelsAndHelpers(t *testing.T) {
	a := Answers{"q": "a"}
	v, err := a.Value()
	ok(t, err)
	var back Answers
	if back.Scan(v) != nil || back.Scan([]byte(v.(string))) != nil || back["q"] != "a" || back.Scan(1) == nil {
		t.Fatal("answers")
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
	for _, m := range []interface{ TableName() string }{Template{}, TemplateTask{}, Checklist{}, Task{}, Clearance{}, ExitInterview{}} {
		if !strings.HasPrefix(m.TableName(), "onb_") {
			t.Fatal(m.TableName())
		}
	}
	for _, m := range []interface{ BeforeCreate(*gorm.DB) error }{&Template{}, &TemplateTask{}, &Checklist{}, &Task{}} {
		ok(t, m.BeforeCreate(nil))
	}
	m := NewModule(nil, Ports{})
	if len(Catalogue()) != 3 || len(m.RegisterModels()) != 6 || m.Clearance() == nil || m.Service() == nil || NewStore(nil).DB() != nil {
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
	worker := router(e, e.worker.UserID, "read", "manage", "exit_interview")
	const B = "/onboarding"
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
	expect(do(router(e, uuid.Nil), "GET", B+"/me", ""), 401, "unauthenticated")
	body := expect(do(hr, "POST", B+"/templates", `{"kind":"offboarding","name":"Exit"}`), 201, "template")
	tid := idOf(body)
	expect(do(hr, "POST", B+"/templates", "{"), 400, "bad json")
	expect(do(hr, "PATCH", B+"/templates/"+tid, `{"name":"Exit v2"}`), 200, "patch")
	expect(do(hr, "PATCH", B+"/templates/"+tid, "{"), 400, "bad patch")
	expect(do(hr, "PUT", B+"/templates/"+tid+"/tasks", `[{"title":"Laptop","owner_role":"it"}]`), 200, "tasks")
	expect(do(hr, "PUT", B+"/templates/"+tid+"/tasks", "{"), 400, "bad tasks")
	expect(do(hr, "GET", B+"/templates?kind=offboarding", ""), 200, "templates")
	expect(do(hr, "GET", B+"/templates/"+tid, ""), 200, "template get")
	expect(do(hr, "GET", B+"/templates/x", ""), 400, "bad id")
	expect(do(worker, "POST", B+"/templates", `{}`), 403, "worker cannot manage")

	body = expect(do(hr, "POST", B+"/checklists", `{"employee_id":"`+e.alice.String()+`","kind":"offboarding","anchor_date":"2026-06-30"}`), 201, "start")
	cid := idOf(body)
	expect(do(hr, "POST", B+"/checklists", "{"), 400, "bad start")
	expect(do(hr, "GET", B+"/checklists?kind=offboarding&employee_id="+e.alice.String()+"&department_id="+e.dept.String(), ""), 200, "list")
	expect(do(hr, "GET", B+"/checklists?employee_id=x", ""), 400, "bad filter")
	expect(do(hr, "GET", B+"/checklists?kind=x", ""), 400, "bad kind")
	expect(do(worker, "GET", B+"/checklists/"+cid, ""), 200, "worker get")
	body = expect(do(worker, "GET", B+"/me", ""), 200, "me")
	task := e.tasksOf(&Checklist{ID: uuid.MustParse(cid)})[0].ID.String()
	_ = body
	expect(do(hr, "GET", B+"/my-tasks", ""), 200, "my tasks")
	expect(do(hr, "POST", B+"/tasks/"+task+"/skip", `{"reason":"not needed"}`), 200, "skip")
	expect(do(hr, "POST", B+"/tasks/"+task+"/skip", "{"), 400, "bad skip")
	expect(do(hr, "POST", B+"/tasks/"+task+"/reopen", ""), 200, "reopen")
	expect(do(hr, "POST", B+"/tasks/"+task+"/done", `{"note":"returned"}`), 200, "done")
	expect(do(hr, "POST", B+"/checklists/"+cid+"/complete", ""), 409, "clearance pending")
	expect(do(worker, "PUT", B+"/checklists/"+cid+"/exit-interview", `{"answers":{"why":"growth"}}`), 201, "exit interview")
	expect(do(worker, "PUT", B+"/checklists/"+cid+"/exit-interview", "{"), 400, "bad exit")
	expect(do(worker, "GET", B+"/checklists/"+cid+"/exit-interview", ""), 200, "own exit")
	expect(do(hr, "GET", B+"/checklists/"+cid+"/exit-interview", ""), 200, "hr exit")
	for _, area := range []string{"manager", "hr", "it", "finance"} {
		expect(do(hr, "POST", B+"/checklists/"+cid+"/clearances/"+area+"/clear", `{"note":"ok"}`), 200, "clear "+area)
	}
	expect(do(hr, "POST", B+"/checklists/"+cid+"/clearances/hr/clear", "{"), 400, "bad clear")
	expect(do(hr, "GET", B+"/reports/progress?kind=offboarding", ""), 200, "progress")
	expect(do(hr, "POST", B+"/checklists/"+cid+"/cancel", `{"reason":"x"}`), 409, "cancel completed")
	expect(do(hr, "POST", B+"/checklists/"+cid+"/cancel", "{"), 400, "bad cancel")
	expect(do(hr, "POST", B+"/checklists/"+cid+"/complete", ""), 409, "complete twice")
	e.store.fail["checklists"] = errBoom
	expect(do(hr, "GET", B+"/checklists", ""), 500, "internal")
}

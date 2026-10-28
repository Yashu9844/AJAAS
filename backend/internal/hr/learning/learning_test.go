package learning

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
	"github.com/jaas/jaas/internal/shared/identityport"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"gorm.io/gorm"
)

var errBoom = errors.New("boom")

// --- fake store ---

type fakeStore struct {
	settings    *Settings
	courses     map[uuid.UUID]Course
	versions    []CourseVersion
	plans       map[uuid.UUID]Plan
	items       map[uuid.UUID][]PlanItem
	assignments map[uuid.UUID]Assignment
	enrolments  map[uuid.UUID]Enrolment
	completions map[uuid.UUID]Completion
	events      []string
	fail        map[string]error
}

func newStore() *fakeStore {
	return &fakeStore{courses: map[uuid.UUID]Course{}, plans: map[uuid.UUID]Plan{}, items: map[uuid.UUID][]PlanItem{}, assignments: map[uuid.UUID]Assignment{},
		enrolments: map[uuid.UUID]Enrolment{}, completions: map[uuid.UUID]Completion{}, fail: map[string]error{}}
}

func cp[K comparable, V any](m map[K]V) map[K]V {
	out := make(map[K]V, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func (f *fakeStore) snapshot() func() {
	c, p, it, a, e, co, ev, vs, st := cp(f.courses), cp(f.plans), cp(f.items), cp(f.assignments), cp(f.enrolments), cp(f.completions), len(f.events), len(f.versions), f.settings
	return func() {
		f.courses, f.plans, f.items, f.assignments, f.enrolments, f.completions, f.events, f.versions, f.settings = c, p, it, a, e, co, f.events[:ev], f.versions[:vs], st
	}
}

func (f *fakeStore) InTx(_ context.Context, fn func(*gorm.DB) error) error {
	undo := f.snapshot()
	if err := fn(nil); err != nil {
		undo()
		return err
	}
	return nil
}
func (f *fakeStore) Savepoint(_ context.Context, _ *gorm.DB, fn func(*gorm.DB) error) error {
	undo := f.snapshot()
	if err := fn(nil); err != nil {
		undo()
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
	case *Course:
		_ = v.BeforeCreate(nil)
		f.courses[v.ID] = *v
	case *CourseVersion:
		f.versions = append(f.versions, *v)
	case *Plan:
		_ = v.BeforeCreate(nil)
		f.plans[v.ID] = *v
	case *Assignment:
		_ = v.BeforeCreate(nil)
		f.assignments[v.ID] = *v
	case *Completion:
		_ = v.BeforeCreate(nil)
		f.completions[v.ID] = *v
	}
	return nil
}
func (f *fakeStore) Save(_ context.Context, _ *gorm.DB, row interface{}) error {
	if err := f.fail["save"]; err != nil {
		return err
	}
	switch v := row.(type) {
	case *Course:
		f.courses[v.ID] = *v
	case *Plan:
		f.plans[v.ID] = *v
	case *Assignment:
		f.assignments[v.ID] = *v
	case *Enrolment:
		f.enrolments[v.ID] = *v
	case *Completion:
		f.completions[v.ID] = *v
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
func (f *fakeStore) Course(_ context.Context, _ *gorm.DB, _, id uuid.UUID, _ bool) (*Course, error) {
	if c, ok := f.courses[id]; ok {
		return &c, f.fail["course"]
	}
	return nil, f.fail["course"]
}
func page[T any](rows []T, p, pp int) []T {
	lo := (p - 1) * pp
	if lo >= len(rows) {
		return nil
	}
	return rows[lo:min(lo+pp, len(rows))]
}
func (f *fakeStore) Courses(_ context.Context, _ *gorm.DB, _ uuid.UUID, status string, p, pp int) ([]Course, int64, error) {
	var out []Course
	for _, c := range f.courses {
		if status == "" || c.Status == status {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Title < out[j].Title })
	return page(out, p, pp), int64(len(out)), f.fail["courses"]
}
func (f *fakeStore) CoursesByIDs(_ context.Context, _ *gorm.DB, _ uuid.UUID, ids []uuid.UUID) ([]Course, error) {
	var out []Course
	for _, id := range ids {
		if c, ok := f.courses[id]; ok {
			out = append(out, c)
		}
	}
	return out, f.fail["coursesbyids"]
}
func (f *fakeStore) Published(ctx context.Context, db *gorm.DB, t uuid.UUID, p, pp int) ([]Course, int64, error) {
	return f.Courses(ctx, db, t, CoursePublished, p, pp)
}
func (f *fakeStore) Versions(_ context.Context, _ *gorm.DB, _, c uuid.UUID) ([]CourseVersion, error) {
	var out []CourseVersion
	for i := len(f.versions) - 1; i >= 0; i-- {
		if f.versions[i].CourseID == c {
			out = append(out, f.versions[i])
		}
	}
	return out, f.fail["versions"]
}
func (f *fakeStore) Plan(_ context.Context, _ *gorm.DB, _, id uuid.UUID, _ bool) (*Plan, error) {
	if p, ok := f.plans[id]; ok {
		return &p, f.fail["plan"]
	}
	return nil, f.fail["plan"]
}
func (f *fakeStore) Plans(_ context.Context, _ *gorm.DB, _ uuid.UUID, status string, p, pp int) ([]Plan, int64, error) {
	var out []Plan
	for _, x := range f.plans {
		if status == "" || x.Status == status {
			out = append(out, x)
		}
	}
	return page(out, p, pp), int64(len(out)), f.fail["plans"]
}
func (f *fakeStore) PlanNameTaken(_ context.Context, _ *gorm.DB, _ uuid.UUID, name string, except uuid.UUID) (bool, error) {
	for _, p := range f.plans {
		if strings.EqualFold(p.Name, name) && p.ID != except {
			return true, f.fail["nametaken"]
		}
	}
	return false, f.fail["nametaken"]
}
func (f *fakeStore) PlanItems(_ context.Context, _ *gorm.DB, _, p uuid.UUID) ([]PlanItem, error) {
	return append([]PlanItem(nil), f.items[p]...), f.fail["planitems"]
}
func (f *fakeStore) ReplaceItems(_ context.Context, _ *gorm.DB, _, p uuid.UUID, rows []PlanItem) error {
	f.items[p] = rows
	return f.fail["replace"]
}
func (f *fakeStore) Assignment(_ context.Context, _ *gorm.DB, _, id uuid.UUID, _ bool) (*Assignment, error) {
	if a, ok := f.assignments[id]; ok {
		return &a, f.fail["assignment"]
	}
	return nil, f.fail["assignment"]
}
func (f *fakeStore) Assignments(_ context.Context, _ *gorm.DB, _ uuid.UUID, status string, p, pp int) ([]Assignment, int64, error) {
	var out []Assignment
	for _, x := range f.assignments {
		if status == "" || x.Status == status {
			out = append(out, x)
		}
	}
	return page(out, p, pp), int64(len(out)), f.fail["assignments"]
}
func (f *fakeStore) RuleAssignments(_ context.Context, _ *gorm.DB, _ uuid.UUID) ([]Assignment, error) {
	var out []Assignment
	for _, x := range f.assignments {
		if x.Status == AssignActive && x.TargetKind != TargetEmployee {
			out = append(out, x)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID.String() < out[j].ID.String() })
	return out, f.fail["rules"]
}
func (f *fakeStore) DueRecurring(_ context.Context, _ *gorm.DB, _ *uuid.UUID, upTo Date) ([]Assignment, error) {
	var out []Assignment
	for _, x := range f.assignments {
		if x.Status == AssignActive && x.RecurrenceDays != nil && x.NextCycleOn != nil && !x.NextCycleOn.After(upTo.Time) {
			out = append(out, x)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID.String() < out[j].ID.String() })
	return out, f.fail["due"]
}
func (f *fakeStore) InsertEnrolments(_ context.Context, _ *gorm.DB, rows []Enrolment) (int, error) {
	if err := f.fail["insert"]; err != nil {
		return 0, err
	}
	n := 0
	for _, r := range rows {
		dup := false
		for _, e := range f.enrolments {
			dup = dup || (e.EmployeeProfileID == r.EmployeeProfileID && e.DedupeKey == r.DedupeKey)
		}
		if !dup {
			_ = r.BeforeCreate(nil)
			f.enrolments[r.ID] = r
			n++
		}
	}
	return n, nil
}
func (f *fakeStore) Enrolment(_ context.Context, _ *gorm.DB, _, id uuid.UUID, _ bool) (*Enrolment, error) {
	if e, ok := f.enrolments[id]; ok {
		return &e, f.fail["enrolment"]
	}
	return nil, f.fail["enrolment"]
}
func (f *fakeStore) sorted() []Enrolment {
	var out []Enrolment
	for _, e := range f.enrolments {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID.String() < out[j].ID.String() })
	return out
}
func (f *fakeStore) Enrolments(_ context.Context, _ *gorm.DB, _ uuid.UUID, fl EnrolFilter, p, pp int) ([]Enrolment, int64, error) {
	var out []Enrolment
	for _, e := range f.sorted() {
		switch {
		case fl.EmployeeID != nil && e.EmployeeProfileID != *fl.EmployeeID:
		case fl.EmployeeIDs != nil && !contains(fl.EmployeeIDs, e.EmployeeProfileID):
		case fl.CourseID != nil && e.CourseID != *fl.CourseID:
		case fl.AssignmentID != nil && (e.AssignmentID == nil || *e.AssignmentID != *fl.AssignmentID):
		case fl.Status == FilterOverdue && !(e.Live() && e.DueDate != nil && e.DueDate.Before(fl.Today.Time)):
		case fl.Status != "" && fl.Status != FilterOverdue && e.Status != fl.Status:
		default:
			out = append(out, e)
		}
	}
	return page(out, p, pp), int64(len(out)), f.fail["enrolments"]
}
func contains(ids []uuid.UUID, id uuid.UUID) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}
func (f *fakeStore) EnrolmentsByIDs(_ context.Context, _ *gorm.DB, _ uuid.UUID, ids []uuid.UUID) ([]Enrolment, error) {
	var out []Enrolment
	for _, id := range ids {
		if e, ok := f.enrolments[id]; ok {
			out = append(out, e)
		}
	}
	return out, f.fail["byids"]
}
func (f *fakeStore) CourseEnrolments(_ context.Context, _ *gorm.DB, _, emp uuid.UUID, courses []uuid.UUID) ([]Enrolment, error) {
	var out []Enrolment
	for _, e := range f.sorted() {
		if e.EmployeeProfileID == emp && contains(courses, e.CourseID) {
			out = append(out, e)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Cycle > out[j].Cycle })
	return out, f.fail["courseenrolments"]
}
func (f *fakeStore) LiveSelfEnrolment(_ context.Context, _ *gorm.DB, _, emp, course uuid.UUID) (*Enrolment, error) {
	for _, e := range f.sorted() {
		if e.EmployeeProfileID == emp && e.CourseID == course && e.Source == SourceSelf && e.Live() {
			return &e, f.fail["liveself"]
		}
	}
	return nil, f.fail["liveself"]
}
func (f *fakeStore) Compliance(_ context.Context, _ *gorm.DB, _ uuid.UUID, fl ComplianceFilter) ([]ComplianceRow, error) {
	type key struct {
		d *uuid.UUID
		c uuid.UUID
	}
	rows := map[string]*ComplianceRow{}
	for _, e := range f.sorted() {
		if (fl.DepartmentID != nil && (e.DepartmentID == nil || *e.DepartmentID != *fl.DepartmentID)) || (fl.CourseID != nil && e.CourseID != *fl.CourseID) ||
			(fl.Mandatory != nil && e.Mandatory != *fl.Mandatory) {
			continue
		}
		k := e.CourseID.String()
		if e.DepartmentID != nil {
			k += e.DepartmentID.String()
		}
		r := rows[k]
		if r == nil {
			r = &ComplianceRow{DepartmentID: e.DepartmentID, CourseID: e.CourseID}
			rows[k] = r
		}
		r.Total++
		switch e.Status {
		case EnrolNotStarted:
			r.NotStarted++
		case EnrolInProgress:
			r.InProgress++
		case EnrolCompleted:
			r.Completed++
		case EnrolWaived:
			r.Waived++
		}
		if e.Live() && e.DueDate != nil && e.DueDate.Before(fl.Today.Time) {
			r.Overdue++
		}
	}
	var out []ComplianceRow
	for _, r := range rows {
		out = append(out, *r)
	}
	return out, f.fail["compliance"]
}
func (f *fakeStore) Completion(_ context.Context, _ *gorm.DB, _, id uuid.UUID, _ bool) (*Completion, error) {
	if c, ok := f.completions[id]; ok {
		return &c, f.fail["completion"]
	}
	return nil, f.fail["completion"]
}
func (f *fakeStore) liveFor(enrolment uuid.UUID) *Completion {
	for _, c := range f.completions {
		if c.EnrolmentID == enrolment && (c.Status == CompPending || c.Status == CompVerified) {
			c := c
			return &c
		}
	}
	return nil
}
func (f *fakeStore) LiveCompletion(_ context.Context, _ *gorm.DB, _, e uuid.UUID, _ bool) (*Completion, error) {
	return f.liveFor(e), f.fail["live"]
}
func (f *fakeStore) LiveCompletions(_ context.Context, _ *gorm.DB, _ uuid.UUID, ids []uuid.UUID) ([]Completion, error) {
	var out []Completion
	for _, id := range ids {
		if c := f.liveFor(id); c != nil {
			out = append(out, *c)
		}
	}
	return out, f.fail["lives"]
}
func (f *fakeStore) Pending(_ context.Context, _ *gorm.DB, _ uuid.UUID) ([]Completion, error) {
	var out []Completion
	for _, c := range f.completions {
		if c.Status == CompPending {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID.String() < out[j].ID.String() })
	return out, f.fail["pending"]
}
func (f *fakeStore) Event(_ context.Context, _ *gorm.DB, _ uuid.UUID, key string, _ map[string]string) error {
	if err := f.fail["event"]; err != nil {
		return err
	}
	f.events = append(f.events, key)
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
func (e *fakeEmployees) List(_ context.Context, _ uuid.UUID, f employeeDTO.EmployeeFilter) ([]employeeDTO.EmployeeResponse, int64, error) {
	var out []employeeDTO.EmployeeResponse
	for _, r := range e.byID {
		if f.Status == "" || r.Status == f.Status {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID.String() < out[j].ID.String() })
	return page(out, f.Page, f.PerPage), int64(len(out)), e.err
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

type fakeReports struct {
	byManager map[uuid.UUID][]uuid.UUID
	err       error
}

func (r *fakeReports) DirectReportUserIDs(_ context.Context, _, m uuid.UUID) ([]uuid.UUID, error) {
	return r.byManager[m], r.err
}

type noAudit struct{}

func (noAudit) Log(context.Context, *gorm.DB, string, string, string, string, string, interface{}, string, string) error {
	return nil
}

var _ identityport.Auditor = noAudit{}

// --- environment ---

type person struct {
	emp  uuid.UUID
	user uuid.UUID
	act  Actor
}

type env struct {
	svc         *Service
	st          *fakeStore
	emps        *fakeEmployees
	core        *fakeCore
	reports     *fakeReports
	tenant      uuid.UUID
	clock       time.Time
	dept, dept2 uuid.UUID
	profile     uuid.UUID
	hr          person // learning:manage + read
	mgr         person
	ann, bob    person // ann and bob report to mgr; ann in dept, bob in dept2
	outsider    person
}

func setup(t *testing.T) *env {
	t.Helper()
	e := &env{st: newStore(), tenant: uuid.New(), clock: time.Date(2026, 6, 10, 10, 0, 0, 0, time.UTC), dept: uuid.New(), dept2: uuid.New(), profile: uuid.New()}
	e.emps = &fakeEmployees{byID: map[uuid.UUID]employeeDTO.EmployeeResponse{}, byUser: map[uuid.UUID]uuid.UUID{}}
	e.core = &fakeCore{place: map[uuid.UUID]*coreAPI.Placement{}}
	e.reports = &fakeReports{byManager: map[uuid.UUID][]uuid.UUID{}}
	e.svc = NewService(e.st, Ports{Employees: e.emps, Core: e.core, Reports: e.reports, Audit: noAudit{}}, func() time.Time { return e.clock })
	e.hr, e.mgr = e.person("hr", nil), e.person("mgr", &e.dept)
	e.hr.act.CanManage, e.hr.act.CanRead = true, true
	e.ann, e.bob, e.outsider = e.person("ann", &e.dept), e.person("bob", &e.dept2), e.person("out", nil)
	e.reports.byManager[e.mgr.user] = []uuid.UUID{e.ann.user, e.bob.user}
	return e
}

func (e *env) person(name string, dept *uuid.UUID) person {
	p := person{emp: uuid.New(), user: uuid.New()}
	p.act = Actor{TenantID: e.tenant, UserID: p.user}
	e.emps.byID[p.emp] = employeeDTO.EmployeeResponse{ID: p.emp, UserID: p.user, DisplayName: name, Status: "active"}
	e.emps.byUser[p.user] = p.emp
	if dept != nil {
		d := *dept
		pr := e.profile
		e.core.place[p.emp] = &coreAPI.Placement{DepartmentID: &d, JobProfileID: &pr}
	}
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

func str(s string) *string { return &s }
func intp(n int) *int      { return &n }
func boolp(b bool) *bool   { return &b }

// course creates and publishes a link course.
func (e *env) course(t *testing.T, mutate func(*CourseInput)) *Course {
	t.Helper()
	in := CourseInput{Title: str("Fire safety " + uuid.NewString()[:4]), ContentKind: str(KindLink), ContentURL: str("https://learn.example.com/fire")}
	if mutate != nil {
		mutate(&in)
	}
	c, err := e.svc.CreateCourse(ctx(), e.hr.act, in)
	ok(t, err)
	c, err = e.svc.PublishCourse(ctx(), e.hr.act, c.ID)
	ok(t, err)
	return c
}

func (e *env) assign(t *testing.T, in AssignmentInput) *AssignResult {
	t.Helper()
	r, err := e.svc.CreateAssignment(ctx(), e.hr.act, in)
	ok(t, err)
	return r
}

// --- LN-C01 validation (LG4) ---

func TestCourseValidation(t *testing.T) {
	e := setup(t)
	good := func() CourseInput {
		return CourseInput{Title: str("Safety"), ContentKind: str(KindLink), ContentURL: str("https://learn.example.com/x")}
	}
	bad := map[string]func(*CourseInput){
		"http url":         func(i *CourseInput) { i.ContentURL = str("http://learn.example.com/x") },
		"javascript url":   func(i *CourseInput) { i.ContentURL = str("javascript:alert(1)") },
		"data url":         func(i *CourseInput) { i.ContentURL = str("data:text/html,<script>") },
		"user info":        func(i *CourseInput) { i.ContentURL = str("https://user:pw@learn.example.com/x") },
		"no host":          func(i *CourseInput) { i.ContentURL = str("https:///x") },
		"spaces":           func(i *CourseInput) { i.ContentURL = str("https://learn.example.com/a b") },
		"too long":         func(i *CourseInput) { i.ContentURL = str("https://learn.example.com/" + strings.Repeat("a", 500)) },
		"link w/o url":     func(i *CourseInput) { i.ContentURL = nil },
		"no title":         func(i *CourseInput) { i.Title = str("  ") },
		"long title":       func(i *CourseInput) { i.Title = str(strings.Repeat("t", 151)) },
		"long desc":        func(i *CourseInput) { i.Description = str(strings.Repeat("d", 2001)) },
		"long provider":    func(i *CourseInput) { i.Provider = str(strings.Repeat("p", 101)) },
		"unknown kind":     func(i *CourseInput) { i.ContentKind = str("video") },
		"document w/o ref": func(i *CourseInput) { i.ContentKind, i.ContentURL = str(KindDocument), nil },
		"document url ref": func(i *CourseInput) {
			i.ContentKind, i.ContentURL, i.DocumentRef = str(KindDocument), nil, str("https://x.example.com/f.pdf")
		},
		"document bad ref": func(i *CourseInput) { i.ContentKind, i.ContentURL, i.DocumentRef = str(KindDocument), nil, str("a b") },
		"thumb data url":   func(i *CourseInput) { i.ThumbnailRef = str("data:image/png;base64,AAAA") },
		"thumb http":       func(i *CourseInput) { i.ThumbnailRef = str("http://x.example.com/a.png") },
		"thumb bad ref":    func(i *CourseInput) { i.ThumbnailRef = str("a b!") },
		"duration":         func(i *CourseInput) { i.DurationMins = intp(10001) },
		"negative":         func(i *CourseInput) { i.DurationMins = intp(-1) },
		"many tags":        func(i *CourseInput) { i.Tags = &[]string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11"} },
		"empty tag":        func(i *CourseInput) { i.Tags = &[]string{" "} },
		"long tag":         func(i *CourseInput) { i.Tags = &[]string{strings.Repeat("t", 31)} },
		"verification":     func(i *CourseInput) { i.Verification = str("boss") },
	}
	for name, mutate := range bad {
		in := good()
		mutate(&in)
		if _, err := e.svc.CreateCourse(ctx(), e.hr.act, in); err == nil {
			t.Fatalf("LG4 %s must be refused", name)
		}
	}
	if _, err := e.svc.CreateCourse(ctx(), e.hr.act, CourseInput{}); err == nil {
		t.Fatal("title and kind are required")
	}
	// Good ones: a document, a thumbnail URL, an opaque thumbnail, de-duplicated lower-case tags.
	in := good()
	in.Tags, in.ThumbnailRef = &[]string{"Safety", "safety ", "HR"}, str("https://cdn.example.com/t.png")
	c, err := e.svc.CreateCourse(ctx(), e.hr.act, in)
	ok(t, err)
	if len(c.Tags) != 2 || c.Tags[0] != "safety" || c.Version != 1 || c.Status != CourseDraft || c.Verification != VerifySelf {
		t.Fatalf("defaults: %+v", c)
	}
	doc := CourseInput{Title: str("Handbook"), ContentKind: str(KindDocument), DocumentRef: str("docs/handbook-2026.pdf"), ThumbnailRef: str("thumbs/h.png"), Verification: str(VerifyHR)}
	d, err := e.svc.CreateCourse(ctx(), e.hr.act, doc)
	ok(t, err)
	if d.ContentURL != nil || d.DocumentRef == nil {
		t.Fatalf("document: %+v", d)
	}
	e.st.fail["create"] = errBoom
	if _, err := e.svc.CreateCourse(ctx(), e.hr.act, good()); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
}

// --- LN-C02, LN-C03 lifecycle and LG2 ---

func TestCourseLifecycle(t *testing.T) {
	e := setup(t)
	c, err := e.svc.CreateCourse(ctx(), e.hr.act, CourseInput{Title: str("Safety"), ContentKind: str(KindLink), ContentURL: str("https://learn.example.com/x")})
	ok(t, err)
	up, err := e.svc.UpdateCourse(ctx(), e.hr.act, c.ID, CourseInput{Title: str("Safety v2"), DurationMins: intp(30)})
	if err != nil || up.Version != 2 {
		t.Fatalf("content change bumps the version: %+v %v", up, err)
	}
	same, err := e.svc.UpdateCourse(ctx(), e.hr.act, c.ID, CourseInput{SelfEnrol: boolp(true), Verification: str(VerifyManager), ThumbnailRef: str("t/a.png")})
	if err != nil || same.Version != 2 || !same.SelfEnrol {
		t.Fatalf("settings do not bump: %+v %v", same, err)
	}
	if vs, err := e.svc.Versions(ctx(), e.hr.act, c.ID); err != nil || len(vs) != 2 || vs[0].Version != 2 {
		t.Fatalf("versions: %+v %v", vs, err)
	}
	code(t, mustErr(e.svc.Versions(ctx(), e.hr.act, uuid.New())), "NOT_FOUND")
	code(t, mustErr(e.svc.UpdateCourse(ctx(), e.hr.act, uuid.New(), CourseInput{})), "NOT_FOUND")
	if _, err := e.svc.UpdateCourse(ctx(), e.hr.act, c.ID, CourseInput{ContentURL: str("http://bad")}); err == nil {
		t.Fatal("an update is validated too")
	}
	if rows, total, _ := e.svc.Catalog(ctx(), e.ann.act, 1, 20); total != 0 || len(rows) != 0 {
		t.Fatal("a draft is not in the catalogue")
	}
	pub, err := e.svc.PublishCourse(ctx(), e.hr.act, c.ID)
	if err != nil || pub.Status != CoursePublished {
		t.Fatal(pub, err)
	}
	code(t, mustErr(e.svc.PublishCourse(ctx(), e.hr.act, c.ID)), "COURSE_STATE")
	if rows, total, _ := e.svc.Catalog(ctx(), e.ann.act, 1, 20); total != 1 || len(rows) != 1 {
		t.Fatal("published course is listed for learners")
	}
	if rows, total, err := e.svc.Courses(ctx(), e.hr.act, CoursePublished, 1, 20); err != nil || total != 1 || len(rows) != 1 {
		t.Fatal(rows, err)
	}
	code(t, mustErr3(e.svc.Courses(ctx(), e.hr.act, "weird", 1, 20)), "VALIDATION_ERROR")
	ret, err := e.svc.RetireCourse(ctx(), e.hr.act, c.ID)
	if err != nil || ret.Status != CourseRetired {
		t.Fatal(ret, err)
	}
	code(t, mustErr(e.svc.RetireCourse(ctx(), e.hr.act, c.ID)), "COURSE_STATE")
	code(t, mustErr(e.svc.UpdateCourse(ctx(), e.hr.act, c.ID, CourseInput{Title: str("x")})), "COURSE_STATE")
	code(t, mustErr(e.svc.PublishCourse(ctx(), e.hr.act, uuid.New())), "NOT_FOUND")
	if g, err := e.svc.GetCourse(ctx(), e.hr.act, c.ID); err != nil || g.Status != CourseRetired {
		t.Fatal(g, err)
	}
	code(t, mustErr(e.svc.GetCourse(ctx(), e.hr.act, uuid.New())), "NOT_FOUND")
	for _, k := range []string{"event", "save"} {
		d, _ := e.svc.CreateCourse(ctx(), e.hr.act, CourseInput{Title: str("D" + k), ContentKind: str(KindLink), ContentURL: str("https://learn.example.com/d")})
		e.st.fail[k] = errBoom
		if _, err := e.svc.PublishCourse(ctx(), e.hr.act, d.ID); !errors.Is(err, errBoom) {
			t.Fatal(k, err)
		}
		delete(e.st.fail, k)
		if e.st.courses[d.ID].Status != CourseDraft {
			t.Fatalf("%s: a failed publish changes nothing", k)
		}
	}
	e.st.fail["course"] = errBoom
	if _, err := e.svc.GetCourse(ctx(), e.hr.act, c.ID); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
}

func mustErr(_ interface{}, err error) error          { return err }
func mustErr3[T any](_ []T, _ int64, err error) error { return err }

// --- LN-C04 plans ---

func TestPlans(t *testing.T) {
	e := setup(t)
	c1, c2 := e.course(t, nil), e.course(t, nil)
	code(t, mustErr(e.svc.CreatePlan(ctx(), e.hr.act, PlanInput{})), "VALIDATION_ERROR")
	code(t, mustErr(e.svc.CreatePlan(ctx(), e.hr.act, PlanInput{Name: str(" ")})), "VALIDATION_ERROR")
	p, err := e.svc.CreatePlan(ctx(), e.hr.act, PlanInput{Name: str("Onboarding"), Description: str("first week")})
	ok(t, err)
	code(t, mustErr(e.svc.CreatePlan(ctx(), e.hr.act, PlanInput{Name: str("onboarding")})), "CONFLICT")
	code(t, mustErr(e.svc.CreatePlan(ctx(), e.hr.act, PlanInput{Name: str("N"), Description: str(strings.Repeat("d", 1001))})), "VALIDATION_ERROR")
	code(t, mustErr(e.svc.UpdatePlan(ctx(), e.hr.act, p.ID, PlanInput{Status: str("weird")})), "VALIDATION_ERROR")
	code(t, mustErr(e.svc.UpdatePlan(ctx(), e.hr.act, uuid.New(), PlanInput{})), "NOT_FOUND")
	for _, bad := range [][]ItemInput{nil, {{CourseID: "x"}}, {{CourseID: c1.ID.String()}, {CourseID: c1.ID.String()}}, make([]ItemInput, 51)} {
		if _, err := e.svc.SetItems(ctx(), e.hr.act, p.ID, bad); err == nil {
			t.Fatalf("items %v must be refused", bad)
		}
	}
	code(t, mustErr(e.svc.SetItems(ctx(), e.hr.act, p.ID, []ItemInput{{CourseID: uuid.NewString()}})), "NOT_FOUND")
	code(t, mustErr(e.svc.SetItems(ctx(), e.hr.act, uuid.New(), []ItemInput{{CourseID: c1.ID.String()}})), "NOT_FOUND")
	got, err := e.svc.SetItems(ctx(), e.hr.act, p.ID, []ItemInput{{CourseID: c1.ID.String()}, {CourseID: c2.ID.String(), Mandatory: boolp(false)}})
	if err != nil || len(got.Items) != 2 || !got.Items[0].Mandatory || got.Items[1].Mandatory || got.Items[1].Position != 2 {
		t.Fatalf("items: %+v %v", got, err)
	}
	ret, _ := e.svc.CreateCourse(ctx(), e.hr.act, CourseInput{Title: str("Old"), ContentKind: str(KindLink), ContentURL: str("https://learn.example.com/o")})
	_, _ = e.svc.RetireCourse(ctx(), e.hr.act, ret.ID)
	code(t, mustErr(e.svc.SetItems(ctx(), e.hr.act, p.ID, []ItemInput{{CourseID: ret.ID.String()}})), "COURSE_STATE")
	if g, err := e.svc.GetPlan(ctx(), e.hr.act, p.ID); err != nil || len(g.Items) != 2 {
		t.Fatal(g, err)
	}
	code(t, mustErr(e.svc.GetPlan(ctx(), e.hr.act, uuid.New())), "NOT_FOUND")
	arch, err := e.svc.UpdatePlan(ctx(), e.hr.act, p.ID, PlanInput{Name: str("Onboarding 2026"), Status: str(PlanArchived)})
	if err != nil || arch.Status != PlanArchived || arch.Name != "Onboarding 2026" {
		t.Fatal(arch, err)
	}
	code(t, mustErr(e.svc.SetItems(ctx(), e.hr.act, p.ID, []ItemInput{{CourseID: c1.ID.String()}})), "PLAN_STATE")
	if rows, total, err := e.svc.Plans(ctx(), e.hr.act, PlanArchived, 1, 20); err != nil || total != 1 || len(rows) != 1 {
		t.Fatal(rows, err)
	}
	code(t, mustErr3(e.svc.Plans(ctx(), e.hr.act, "weird", 1, 20)), "VALIDATION_ERROR")
	for _, k := range []string{"nametaken", "replace"} {
		e.st.fail[k] = errBoom
		var err error
		if k == "nametaken" {
			_, err = e.svc.CreatePlan(ctx(), e.hr.act, PlanInput{Name: str("Z")})
		} else {
			fresh, _ := e.svc.CreatePlan(ctx(), e.hr.act, PlanInput{Name: str("Fresh")})
			_, err = e.svc.SetItems(ctx(), e.hr.act, fresh.ID, []ItemInput{{CourseID: c1.ID.String()}})
		}
		if !errors.Is(err, errBoom) {
			t.Fatal(k, err)
		}
		delete(e.st.fail, k)
	}
}

// --- LN-C09 settings and time zones (LG3) ---

func TestSettingsAndZones(t *testing.T) {
	e := setup(t)
	if st, err := e.svc.GetSettings(ctx(), e.hr.act); err != nil || st.TimeZone != DefaultZone {
		t.Fatal(st, err)
	}
	for _, bad := range []string{"", "Mars/Base", "Local", strings.Repeat("z", 65)} {
		if _, err := e.svc.PutSettings(ctx(), e.hr.act, SettingsInput{TimeZone: bad}); err == nil {
			t.Fatalf("%q must be refused", bad)
		}
	}
	// 20:00 UTC on 10 June is 01:30 on 11 June in Kolkata and 13:00 on 10 June in Los Angeles.
	e.clock = time.Date(2026, 6, 10, 20, 0, 0, 0, time.UTC)
	if d, _ := e.svc.today(ctx(), e.tenant); d.Format(DateLayout) != "2026-06-11" {
		t.Fatalf("Kolkata today %s", d.Format(DateLayout))
	}
	st, err := e.svc.PutSettings(ctx(), e.hr.act, SettingsInput{TimeZone: "America/Los_Angeles"})
	if err != nil || st.TimeZone != "America/Los_Angeles" {
		t.Fatal(st, err)
	}
	if d, _ := e.svc.today(ctx(), e.tenant); d.Format(DateLayout) != "2026-06-10" {
		t.Fatalf("Los Angeles today %s", d.Format(DateLayout))
	}
	e.st.fail["settings"] = errBoom
	if _, err := e.svc.today(ctx(), e.tenant); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	if _, err := e.svc.GetSettings(ctx(), e.hr.act); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	delete(e.st.fail, "settings")
	e.st.fail["savesettings"] = errBoom
	if _, err := e.svc.PutSettings(ctx(), e.hr.act, SettingsInput{TimeZone: "UTC"}); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	// A stored zone that no longer loads falls back to UTC rather than failing.
	delete(e.st.fail, "savesettings")
	e.st.settings = &Settings{TenantID: e.tenant, TimeZone: "Gone/Zone"}
	if d, err := e.svc.today(ctx(), e.tenant); err != nil || d.Format(DateLayout) != "2026-06-10" {
		t.Fatal(d, err)
	}
}

func TestOverdueFollowsTheTenantZone(t *testing.T) {
	e := setup(t)
	c := e.course(t, nil)
	e.assign(t, AssignmentInput{TargetKind: TargetEmployee, TargetID: str(e.ann.emp.String()), CourseID: str(c.ID.String()), DueDate: str("2026-06-10")})
	e.clock = time.Date(2026, 6, 10, 20, 0, 0, 0, time.UTC) // 11 June in Kolkata, 10 June in Los Angeles
	rows, _, err := e.svc.Mine(ctx(), e.ann.act, FilterOverdue, 1, 20)
	if err != nil || len(rows) != 1 || !rows[0].Overdue {
		t.Fatalf("LG3 overdue in Kolkata: %+v %v", rows, err)
	}
	_, err = e.svc.PutSettings(ctx(), e.hr.act, SettingsInput{TimeZone: "America/Los_Angeles"})
	ok(t, err)
	rows, _, err = e.svc.Mine(ctx(), e.ann.act, "", 1, 20)
	if err != nil || len(rows) != 1 || rows[0].Overdue {
		t.Fatalf("LG3 not yet overdue in Los Angeles: %+v %v", rows, err)
	}
	if over, _, _ := e.svc.Mine(ctx(), e.ann.act, FilterOverdue, 1, 20); len(over) != 0 {
		t.Fatal("the overdue filter agrees")
	}
}

// --- LN-C05, LN-C06 assignments ---

func TestAssignments(t *testing.T) {
	e := setup(t)
	c := e.course(t, nil)
	draft, _ := e.svc.CreateCourse(ctx(), e.hr.act, CourseInput{Title: str("Draft"), ContentKind: str(KindLink), ContentURL: str("https://learn.example.com/d")})
	cid := str(c.ID.String())
	bad := []AssignmentInput{
		{TargetKind: "team", CourseID: cid},
		{TargetKind: TargetAll, TargetID: str(uuid.NewString()), CourseID: cid},
		{TargetKind: TargetDepartment, CourseID: cid},
		{TargetKind: TargetDepartment, TargetID: str("x"), CourseID: cid},
		{TargetKind: TargetAll},
		{TargetKind: TargetAll, CourseID: cid, PlanID: str(uuid.NewString())},
		{TargetKind: TargetAll, CourseID: str("x")},
		{TargetKind: TargetAll, PlanID: str("x")},
		{TargetKind: TargetAll, CourseID: cid, DueDate: str("2026-07-01"), DueInDays: intp(5)},
		{TargetKind: TargetAll, CourseID: cid, DueDate: str("07/01/2026")},
		{TargetKind: TargetAll, CourseID: cid, DueDate: str("2026-01-01")},
		{TargetKind: TargetAll, CourseID: cid, DueInDays: intp(0)},
		{TargetKind: TargetAll, CourseID: cid, DueInDays: intp(1001)},
		{TargetKind: TargetAll, CourseID: cid, RecurrenceDays: intp(29), DueInDays: intp(5)},
		{TargetKind: TargetAll, CourseID: cid, RecurrenceDays: intp(365), DueDate: str("2026-12-01")},
		{TargetKind: TargetAll, CourseID: str(draft.ID.String())},
	}
	for i, in := range bad {
		if _, err := e.svc.CreateAssignment(ctx(), e.hr.act, in); err == nil {
			t.Fatalf("case %d must be refused: %+v", i, in)
		}
	}
	code(t, mustErr(e.svc.CreateAssignment(ctx(), e.hr.act, AssignmentInput{TargetKind: TargetAll, CourseID: str(uuid.NewString())})), "NOT_FOUND")
	code(t, mustErr(e.svc.CreateAssignment(ctx(), e.hr.act, AssignmentInput{TargetKind: TargetAll, PlanID: str(uuid.NewString())})), "NOT_FOUND")
	code(t, mustErr(e.svc.CreateAssignment(ctx(), e.hr.act, AssignmentInput{TargetKind: TargetEmployee, TargetID: str(uuid.NewString()), CourseID: cid})), "EMPLOYEE_NOT_FOUND")

	// A department rule: ann (dept) and mgr (dept) match; bob (dept2), hr and outsider do not. A leaver never matches.
	leaver := e.person("gone", &e.dept)
	r := e.emps.byID[leaver.emp]
	r.Status = "inactive"
	e.emps.byID[leaver.emp] = r
	res := e.assign(t, AssignmentInput{TargetKind: TargetDepartment, TargetID: str(e.dept.String()), CourseID: cid, DueInDays: intp(14)})
	if res.Matched != 2 || res.Created != 2 || res.Assignment.Status != AssignActive {
		t.Fatalf("department rule: %+v", res)
	}
	rows, _, _ := e.svc.Mine(ctx(), e.ann.act, "", 1, 20)
	if len(rows) != 1 || rows[0].DueDate.Format(DateLayout) != "2026-06-24" || !rows[0].Mandatory || *rows[0].DepartmentID != e.dept || rows[0].Source != SourceAssigned {
		t.Fatalf("enrolment: %+v", rows)
	}
	// All, location and job profile rules.
	if res := e.assign(t, AssignmentInput{TargetKind: TargetAll, CourseID: cid, Mandatory: boolp(false)}); res.Matched != 5 || res.Created != 5 {
		t.Fatalf("a second assignment has its own enrolments (the key includes the assignment), got %+v", res)
	}
	loc := uuid.New()
	e.core.place[e.bob.emp].LocationID = &loc
	if res := e.assign(t, AssignmentInput{TargetKind: TargetLocation, TargetID: str(loc.String()), CourseID: cid}); res.Matched != 1 {
		t.Fatalf("location rule: %+v", res)
	}
	if res := e.assign(t, AssignmentInput{TargetKind: TargetProfile, TargetID: str(e.profile.String()), CourseID: cid}); res.Matched != 3 {
		t.Fatalf("job profile rule: %+v", res)
	}
	// An employee target with a due date; an inactive employee matches nobody.
	if res := e.assign(t, AssignmentInput{TargetKind: TargetEmployee, TargetID: str(e.outsider.emp.String()), CourseID: cid, DueDate: str("2026-12-31")}); res.Matched != 1 || res.Created != 1 {
		t.Fatalf("employee: %+v", res)
	}
	if res := e.assign(t, AssignmentInput{TargetKind: TargetEmployee, TargetID: str(leaver.emp.String()), CourseID: cid}); res.Matched != 0 || res.Created != 0 {
		t.Fatalf("inactive: %+v", res)
	}

	// Listing, reading, cancelling (idempotent).
	if rows, total, err := e.svc.Assignments(ctx(), e.hr.act, AssignActive, 1, 50); err != nil || total != 6 || len(rows) != 6 {
		t.Fatal(total, err)
	}
	code(t, mustErr3(e.svc.Assignments(ctx(), e.hr.act, "weird", 1, 20)), "VALIDATION_ERROR")
	if g, err := e.svc.GetAssignment(ctx(), e.hr.act, res.Assignment.ID); err != nil || g.ID != res.Assignment.ID {
		t.Fatal(g, err)
	}
	code(t, mustErr(e.svc.GetAssignment(ctx(), e.hr.act, uuid.New())), "NOT_FOUND")
	cancelled, err := e.svc.CancelAssignment(ctx(), e.hr.act, res.Assignment.ID)
	if err != nil || cancelled.Status != AssignCancelled {
		t.Fatal(cancelled, err)
	}
	if again, err := e.svc.CancelAssignment(ctx(), e.hr.act, res.Assignment.ID); err != nil || again.Status != AssignCancelled {
		t.Fatal("cancel is idempotent", again, err)
	}
	if mine, _, _ := e.svc.Mine(ctx(), e.ann.act, "", 1, 20); len(mine) != 3 {
		t.Fatalf("cancelling keeps existing enrolments: %d", len(mine))
	}
	code(t, mustErr(e.svc.CancelAssignment(ctx(), e.hr.act, uuid.New())), "NOT_FOUND")

	// Failures.
	for _, k := range []string{"create", "insert", "event"} {
		e.st.fail[k] = errBoom
		before := len(e.st.assignments)
		if _, err := e.svc.CreateAssignment(ctx(), e.hr.act, AssignmentInput{TargetKind: TargetAll, CourseID: cid}); !errors.Is(err, errBoom) {
			t.Fatal(k, err)
		}
		delete(e.st.fail, k)
		if len(e.st.assignments) != before {
			t.Fatalf("%s: a failed creation leaves nothing", k)
		}
	}
	e.emps.err = errBoom
	if _, err := e.svc.CreateAssignment(ctx(), e.hr.act, AssignmentInput{TargetKind: TargetAll, CourseID: cid}); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	e.emps.err = nil
	e.core.err = errBoom
	if _, err := e.svc.CreateAssignment(ctx(), e.hr.act, AssignmentInput{TargetKind: TargetAll, CourseID: cid}); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	if _, err := e.svc.CreateAssignment(ctx(), e.hr.act, AssignmentInput{TargetKind: TargetEmployee, TargetID: str(e.ann.emp.String()), CourseID: cid}); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	e.core.err = nil
}

func TestPlanAssignment(t *testing.T) {
	e := setup(t)
	c1, c2 := e.course(t, nil), e.course(t, nil)
	p, _ := e.svc.CreatePlan(ctx(), e.hr.act, PlanInput{Name: str("Path")})
	empty := e.assignErr(AssignmentInput{TargetKind: TargetAll, PlanID: str(p.ID.String())})
	code(t, empty, "VALIDATION_ERROR")
	_, err := e.svc.SetItems(ctx(), e.hr.act, p.ID, []ItemInput{{CourseID: c1.ID.String()}, {CourseID: c2.ID.String(), Mandatory: boolp(false)}})
	ok(t, err)
	res := e.assign(t, AssignmentInput{TargetKind: TargetEmployee, TargetID: str(e.ann.emp.String()), PlanID: str(p.ID.String())})
	if res.Created != 2 {
		t.Fatalf("a plan enrols in each course: %+v", res)
	}
	rows, _, _ := e.svc.Mine(ctx(), e.ann.act, "", 1, 20)
	mand := 0
	for _, r := range rows {
		if r.PlanID == nil || *r.PlanID != p.ID {
			t.Fatal("plan id kept")
		}
		if r.Mandatory {
			mand++
		}
	}
	if mand != 1 {
		t.Fatalf("an optional item stays optional: %d", mand)
	}
	arch := PlanArchived
	_, _ = e.svc.UpdatePlan(ctx(), e.hr.act, p.ID, PlanInput{Status: &arch})
	code(t, e.assignErr(AssignmentInput{TargetKind: TargetAll, PlanID: str(p.ID.String())}), "PLAN_STATE")
}

func (e *env) assignErr(in AssignmentInput) error {
	_, err := e.svc.CreateAssignment(ctx(), e.hr.act, in)
	return err
}

// --- LN-C07 joiners and movers (LG1) ---

func TestCoreHook(t *testing.T) {
	e := setup(t)
	c := e.course(t, nil)
	e.assign(t, AssignmentInput{TargetKind: TargetDepartment, TargetID: str(e.dept.String()), CourseID: str(c.ID.String()), DueInDays: intp(30)})
	e.assign(t, AssignmentInput{TargetKind: TargetEmployee, TargetID: str(e.outsider.emp.String()), CourseID: str(c.ID.String())})

	joiner := e.person("joiner", &e.dept)
	fact := coreAPI.ActionFact{EmployeeID: joiner.emp, Type: "hire", Placement: e.core.place[joiner.emp]}
	for i := 0; i < 3; i++ { // LG1 exactly once however often the hook runs
		ok(t, e.svc.CoreHook(ctx(), nil, e.tenant, fact))
	}
	if rows, _, _ := e.svc.Mine(ctx(), joiner.act, "", 1, 20); len(rows) != 1 || rows[0].DueDate.Format(DateLayout) != "2026-07-10" {
		t.Fatalf("LG1 joiner enrolled once: %+v", rows)
	}

	// A mover into the department is enrolled too; one who stays elsewhere, or a leaver, is not.
	mover := e.person("mover", &e.dept2)
	ok(t, e.svc.CoreHook(ctx(), nil, e.tenant, coreAPI.ActionFact{EmployeeID: mover.emp, Type: "transfer", Placement: e.core.place[mover.emp]}))
	if rows, _, _ := e.svc.Mine(ctx(), mover.act, "", 1, 20); len(rows) != 0 {
		t.Fatal("the wrong department is not enrolled")
	}
	e.core.place[mover.emp].DepartmentID = &e.dept
	ok(t, e.svc.CoreHook(ctx(), nil, e.tenant, coreAPI.ActionFact{EmployeeID: mover.emp, Type: "transfer", Placement: e.core.place[mover.emp]}))
	if rows, _, _ := e.svc.Mine(ctx(), mover.act, "", 1, 20); len(rows) != 1 {
		t.Fatal("the mover is enrolled")
	}
	other := e.person("other", &e.dept)
	for _, f := range []coreAPI.ActionFact{
		{EmployeeID: other.emp, Type: "terminate", Placement: e.core.place[other.emp]},
		{EmployeeID: other.emp, Type: "hire", Rescinded: true, Placement: e.core.place[other.emp]},
		{EmployeeID: other.emp, Type: "hire"},
	} {
		ok(t, e.svc.CoreHook(ctx(), nil, e.tenant, f))
	}
	if rows, _, _ := e.svc.Mine(ctx(), other.act, "", 1, 20); len(rows) != 0 {
		t.Fatal("termination, rescission and a missing placement enrol nobody")
	}

	// Cancelled assignments and retired courses stop enrolling; employee assignments are not rules.
	for _, a := range e.st.assignments {
		if a.TargetKind == TargetDepartment {
			_, _ = e.svc.CancelAssignment(ctx(), e.hr.act, a.ID)
		}
	}
	late := e.person("late", &e.dept)
	ok(t, e.svc.CoreHook(ctx(), nil, e.tenant, coreAPI.ActionFact{EmployeeID: late.emp, Type: "hire", Placement: e.core.place[late.emp]}))
	if rows, _, _ := e.svc.Mine(ctx(), late.act, "", 1, 20); len(rows) != 0 {
		t.Fatal("a cancelled rule enrols nobody")
	}

	// A learning failure never reaches the HR action.
	e.assign(t, AssignmentInput{TargetKind: TargetAll, CourseID: str(c.ID.String())})
	for _, k := range []string{"rules", "settings", "insert", "coursesbyids"} {
		e.st.fail[k] = errBoom
		ok(t, e.svc.CoreHook(ctx(), nil, e.tenant, coreAPI.ActionFact{EmployeeID: late.emp, Type: "hire", Placement: e.core.place[late.emp]}))
		delete(e.st.fail, k)
	}
}

// --- LN-C14 recurrence ---

func TestRecurrence(t *testing.T) {
	e := setup(t)
	c := e.course(t, nil)
	res := e.assign(t, AssignmentInput{TargetKind: TargetEmployee, TargetID: str(e.ann.emp.String()), CourseID: str(c.ID.String()), DueInDays: intp(10), RecurrenceDays: intp(30)})
	if res.Assignment.NextCycleOn.Format(DateLayout) != "2026-07-10" {
		t.Fatalf("next cycle %v", res.Assignment.NextCycleOn)
	}
	run := func() *RunResult {
		r, err := e.svc.RunRecurrence(ctx(), &e.tenant)
		ok(t, err)
		return r
	}
	if r := run(); r.Assignments != 0 {
		t.Fatalf("not due yet: %+v", r)
	}
	e.clock = e.clock.AddDate(0, 0, 31) // 11 July
	if r := run(); r.Assignments != 1 || r.Created != 1 {
		t.Fatalf("cycle 1: %+v", r)
	}
	if r := run(); r.Assignments != 0 {
		t.Fatalf("one cycle per run, nothing else is due: %+v", r)
	}
	a := e.st.assignments[res.Assignment.ID]
	if a.Cycle != 1 || a.NextCycleOn.Format(DateLayout) != "2026-08-09" {
		t.Fatalf("assignment: %+v", a)
	}
	mine, _, _ := e.svc.Mine(ctx(), e.ann.act, "", 1, 20)
	if len(mine) != 2 {
		t.Fatalf("a second enrolment for the new cycle: %d", len(mine))
	}
	// Two missed cycles are caught up one run at a time, never skipped.
	e.clock = e.clock.AddDate(0, 0, 65)
	if r := run(); r.Assignments != 1 {
		t.Fatalf("catch-up 1: %+v", r)
	}
	if r := run(); r.Assignments != 1 {
		t.Fatalf("catch-up 2: %+v", r)
	}
	if r := run(); r.Assignments != 0 {
		t.Fatalf("caught up: %+v", r)
	}
	if got := e.st.assignments[res.Assignment.ID].Cycle; got != 3 {
		t.Fatalf("cycle %d", got)
	}
	// A stale copy (another run got there first, or it was cancelled) does nothing; the all-tenants run works too.
	stale := e.st.assignments[res.Assignment.ID]
	stale.NextCycleOn = &Date{e.clock.AddDate(0, 0, -1)}
	if n, ran, err := e.svc.advance(ctx(), &stale); err != nil || ran || n != 0 {
		t.Fatalf("stale: %d %v %v", n, ran, err)
	}
	_, _ = e.svc.CancelAssignment(ctx(), e.hr.act, res.Assignment.ID)
	e.clock = e.clock.AddDate(0, 0, 100)
	if r, err := e.svc.RunRecurrence(ctx(), nil); err != nil || r.Assignments != 0 {
		t.Fatal("cancelled assignments stop recurring", r, err)
	}
	// Failures.
	e.st.fail["due"] = errBoom
	if _, err := e.svc.RunRecurrence(ctx(), nil); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	delete(e.st.fail, "due")
	live := e.assign(t, AssignmentInput{TargetKind: TargetEmployee, TargetID: str(e.ann.emp.String()), CourseID: str(c.ID.String()), DueInDays: intp(5), RecurrenceDays: intp(30)})
	e.clock = e.clock.AddDate(0, 0, 31)
	for _, k := range []string{"insert", "coursesbyids", "settings"} {
		e.st.fail[k] = errBoom
		if _, err := e.svc.RunRecurrence(ctx(), nil); !errors.Is(err, errBoom) {
			t.Fatal(k, err)
		}
		delete(e.st.fail, k)
	}
	if e.st.assignments[live.Assignment.ID].Cycle != 0 {
		t.Fatal("a failed run advances nothing")
	}
}

// --- LN-C08 self-enrolment ---

func TestSelfEnrol(t *testing.T) {
	e := setup(t)
	open := e.course(t, func(i *CourseInput) { i.SelfEnrol = boolp(true) })
	closed := e.course(t, nil)
	draft, _ := e.svc.CreateCourse(ctx(), e.hr.act, CourseInput{Title: str("D"), ContentKind: str(KindLink), ContentURL: str("https://learn.example.com/d"), SelfEnrol: boolp(true)})
	for _, in := range []SelfEnrolInput{{}, {CourseID: str("x")}, {PlanID: str("x")}, {CourseID: str(open.ID.String()), PlanID: str(uuid.NewString())}} {
		if _, err := e.svc.EnrolSelf(ctx(), e.ann.act, in); err == nil {
			t.Fatalf("%+v must be refused", in)
		}
	}
	code(t, mustErr(e.svc.EnrolSelf(ctx(), Actor{TenantID: e.tenant, UserID: uuid.New()}, SelfEnrolInput{CourseID: str(open.ID.String())})), "EMPLOYEE_NOT_FOUND")
	code(t, mustErr(e.svc.EnrolSelf(ctx(), e.ann.act, SelfEnrolInput{CourseID: str(closed.ID.String())})), "SELF_ENROL_OFF")
	code(t, mustErr(e.svc.EnrolSelf(ctx(), e.ann.act, SelfEnrolInput{CourseID: str(draft.ID.String())})), "COURSE_STATE")
	code(t, mustErr(e.svc.EnrolSelf(ctx(), e.ann.act, SelfEnrolInput{CourseID: str(uuid.NewString())})), "NOT_FOUND")
	got, err := e.svc.EnrolSelf(ctx(), e.ann.act, SelfEnrolInput{CourseID: str(open.ID.String())})
	if err != nil || len(got) != 1 || got[0].Source != SourceSelf || *got[0].DepartmentID != e.dept || got[0].Mandatory {
		t.Fatalf("%+v %v", got, err)
	}
	code(t, mustErr(e.svc.EnrolSelf(ctx(), e.ann.act, SelfEnrolInput{CourseID: str(open.ID.String())})), "ALREADY_ENROLLED")
	// A plan: every course must allow it; already-enrolled ones are skipped; none new ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â€šÂ¬Ã‚Â ÃƒÂ¢Ã¢â€šÂ¬Ã¢â€žÂ¢ ALREADY_ENROLLED.
	open2 := e.course(t, func(i *CourseInput) { i.SelfEnrol = boolp(true) })
	p, _ := e.svc.CreatePlan(ctx(), e.hr.act, PlanInput{Name: str("Self path")})
	_, _ = e.svc.SetItems(ctx(), e.hr.act, p.ID, []ItemInput{{CourseID: open.ID.String()}, {CourseID: open2.ID.String()}})
	plan, err := e.svc.EnrolSelf(ctx(), e.ann.act, SelfEnrolInput{PlanID: str(p.ID.String())})
	if err != nil || len(plan) != 1 || plan[0].CourseID != open2.ID {
		t.Fatalf("only the missing course: %+v %v", plan, err)
	}
	code(t, mustErr(e.svc.EnrolSelf(ctx(), e.ann.act, SelfEnrolInput{PlanID: str(p.ID.String())})), "ALREADY_ENROLLED")
	_, _ = e.svc.SetItems(ctx(), e.hr.act, p.ID, []ItemInput{{CourseID: closed.ID.String()}})
	code(t, mustErr(e.svc.EnrolSelf(ctx(), e.bob.act, SelfEnrolInput{PlanID: str(p.ID.String())})), "SELF_ENROL_OFF")
	// After completing, the worker may enrol again.
	done, err := e.svc.Complete(ctx(), e.ann.act, got[0].ID, CompleteInput{})
	ok(t, err)
	if done.Status != EnrolCompleted {
		t.Fatal(done.Status)
	}
	if again, err := e.svc.EnrolSelf(ctx(), e.ann.act, SelfEnrolInput{CourseID: str(open.ID.String())}); err != nil || len(again) != 1 {
		t.Fatal("a completed course can be taken again", again, err)
	}
	for _, k := range []string{"lock", "liveself", "insert", "coursesbyids"} {
		e.st.fail[k] = errBoom
		if _, err := e.svc.EnrolSelf(ctx(), e.bob.act, SelfEnrolInput{CourseID: str(open2.ID.String())}); !errors.Is(err, errBoom) {
			t.Fatal(k, err)
		}
		delete(e.st.fail, k)
	}
	e.core.err = errBoom
	if _, err := e.svc.EnrolSelf(ctx(), e.bob.act, SelfEnrolInput{CourseID: str(open2.ID.String())}); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
}

// --- LN-C10..LN-C13 completion, verification, correction, waiver (LG2, LG5) ---

func (e *env) enrol(t *testing.T, who person, c *Course) *Enrolment {
	t.Helper()
	e.assign(t, AssignmentInput{TargetKind: TargetEmployee, TargetID: str(who.emp.String()), CourseID: str(c.ID.String()), DueInDays: intp(20)})
	for _, en := range e.st.sorted() {
		if en.EmployeeProfileID == who.emp && en.CourseID == c.ID && en.Live() {
			return &en
		}
	}
	t.Fatal("no enrolment")
	return nil
}

func TestSelfAttestedCompletion(t *testing.T) {
	e := setup(t)
	c := e.course(t, nil)
	en := e.enrol(t, e.ann, c)
	code(t, mustErr(e.svc.Start(ctx(), e.bob.act, en.ID)), "NOT_FOUND") // not bob's
	code(t, mustErr(e.svc.Start(ctx(), e.ann.act, uuid.New())), "NOT_FOUND")
	started, err := e.svc.Start(ctx(), e.ann.act, en.ID)
	if err != nil || started.Status != EnrolInProgress || started.StartedAt == nil {
		t.Fatal(started, err)
	}
	code(t, mustErr(e.svc.Start(ctx(), e.ann.act, en.ID)), "ENROLMENT_STATE")
	for _, in := range []CompleteInput{{Score: intp(101)}, {Score: intp(-1)}, {EvidenceRef: str(strings.Repeat("e", 301))}, {CertificateRef: str(strings.Repeat("c", 301))}} {
		if _, err := e.svc.Complete(ctx(), e.ann.act, en.ID, in); err == nil {
			t.Fatalf("%+v must be refused", in)
		}
	}
	code(t, mustErr(e.svc.Complete(ctx(), e.bob.act, en.ID, CompleteInput{})), "NOT_FOUND")
	view, err := e.svc.Complete(ctx(), e.ann.act, en.ID, CompleteInput{Score: intp(92), EvidenceRef: str("certs/a.pdf")})
	if err != nil || view.Status != EnrolCompleted || view.Completion.Status != CompVerified || *view.Completion.Score != 92 || view.CompletedAt == nil {
		t.Fatalf("self-attested completion is verified at once: %+v %v", view, err)
	}
	code(t, mustErr(e.svc.Complete(ctx(), e.ann.act, en.ID, CompleteInput{})), "ENROLMENT_STATE")
	if e.st.events[len(e.st.events)-1] != "learning.enrolment.completed" {
		t.Fatal(e.st.events)
	}
	// The transcript lists it with its completion.
	tr, total, err := e.svc.Transcript(ctx(), e.ann.act, 1, 20)
	if err != nil || total != 1 || tr[0].Completion == nil || tr[0].Completion.Status != CompVerified {
		t.Fatal(tr, err)
	}
	// Completing straight from not_started works and records the start.
	c2 := e.course(t, nil)
	en2 := e.enrol(t, e.ann, c2)
	if v, err := e.svc.Complete(ctx(), e.ann.act, en2.ID, CompleteInput{}); err != nil || v.StartedAt == nil {
		t.Fatal(v, err)
	}
	for _, k := range []string{"event", "create", "live", "course"} {
		c3 := e.course(t, nil)
		en3 := e.enrol(t, e.ann, c3)
		e.st.fail[k] = errBoom
		if _, err := e.svc.Complete(ctx(), e.ann.act, en3.ID, CompleteInput{}); !errors.Is(err, errBoom) {
			t.Fatal(k, err)
		}
		delete(e.st.fail, k)
		if e.st.enrolments[en3.ID].Status != EnrolNotStarted {
			t.Fatalf("%s: a failed completion changes nothing", k)
		}
	}
	e.emps.err = errBoom
	if _, err := e.svc.Complete(ctx(), e.ann.act, en.ID, CompleteInput{}); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
}

func TestVerification(t *testing.T) {
	e := setup(t)
	byMgr := e.course(t, func(i *CourseInput) { i.Verification = str(VerifyManager) })
	byHR := e.course(t, func(i *CourseInput) { i.Verification = str(VerifyHR) })
	en := e.enrol(t, e.ann, byMgr)
	view, err := e.svc.Complete(ctx(), e.ann.act, en.ID, CompleteInput{Score: intp(80)})
	if err != nil || view.Status != EnrolInProgress || view.Completion.Status != CompPending {
		t.Fatalf("LG5 stays in progress until verified: %+v %v", view, err)
	}
	code(t, mustErr(e.svc.Complete(ctx(), e.ann.act, en.ID, CompleteInput{})), "COMPLETION_PENDING")
	cid := view.Completion.ID
	code(t, mustErr(e.svc.Verify(ctx(), e.ann.act, cid)), "LEARNING_SELF_VERIFY")
	code(t, mustErr(e.svc.Verify(ctx(), e.outsider.act, cid)), "NOT_FOUND")
	code(t, mustErr(e.svc.Verify(ctx(), e.bob.act, cid)), "NOT_FOUND")
	code(t, mustErr(e.svc.Verify(ctx(), e.mgr.act, uuid.New())), "NOT_FOUND")
	// The manager sees it in their pending list; a stranger sees nothing; HR sees all.
	if p, err := e.svc.Pending(ctx(), e.mgr.act); err != nil || len(p) != 1 || p[0].EmployeeID != e.ann.emp || p[0].CourseID != byMgr.ID {
		t.Fatal(p, err)
	}
	if p, _ := e.svc.Pending(ctx(), e.outsider.act); len(p) != 0 {
		t.Fatal("outsider sees none")
	}
	if p, _ := e.svc.Pending(ctx(), e.hr.act); len(p) != 1 {
		t.Fatal("HR sees all")
	}
	// Reject needs a comment, sends it back, and the learner can resubmit.
	code(t, mustErr(e.svc.Reject(ctx(), e.mgr.act, cid, " ")), "VALIDATION_ERROR")
	rej, err := e.svc.Reject(ctx(), e.mgr.act, cid, "evidence missing")
	if err != nil || rej.Completion.Status != CompRejected || rej.Status != EnrolInProgress {
		t.Fatal(rej, err)
	}
	code(t, mustErr(e.svc.Verify(ctx(), e.mgr.act, cid)), "COMPLETION_STATE")
	again, err := e.svc.Complete(ctx(), e.ann.act, en.ID, CompleteInput{EvidenceRef: str("certs/b.pdf")})
	ok(t, err)
	done, err := e.svc.Verify(ctx(), e.mgr.act, again.Completion.ID)
	if err != nil || done.Status != EnrolCompleted || done.Completion.Status != CompVerified || done.Completion.DecidedAt == nil {
		t.Fatalf("manager verifies: %+v %v", done, err)
	}
	// An hr-verified course: the manager cannot, HR can.
	en2 := e.enrol(t, e.ann, byHR)
	v2, _ := e.svc.Complete(ctx(), e.ann.act, en2.ID, CompleteInput{})
	code(t, mustErr(e.svc.Verify(ctx(), e.mgr.act, v2.Completion.ID)), "NOT_FOUND")
	if p, _ := e.svc.Pending(ctx(), e.mgr.act); len(p) != 0 {
		t.Fatal("a manager sees only manager-verified completions")
	}
	if v, err := e.svc.Verify(ctx(), e.hr.act, v2.Completion.ID); err != nil || v.Status != EnrolCompleted {
		t.Fatal(v, err)
	}
	// HR may be the learner: their own completion still goes to someone else.
	hrCourse := e.course(t, func(i *CourseInput) { i.Verification = str(VerifyHR) })
	en3 := e.enrol(t, e.hr, hrCourse)
	v3, _ := e.svc.Complete(ctx(), e.hr.act, en3.ID, CompleteInput{})
	code(t, mustErr(e.svc.Verify(ctx(), e.hr.act, v3.Completion.ID)), "LEARNING_SELF_VERIFY")
	// Failures roll back.
	en4 := e.enrol(t, e.ann, byMgr)
	v4, _ := e.svc.Complete(ctx(), e.ann.act, en4.ID, CompleteInput{})
	for _, k := range []string{"completion", "enrolment", "save", "event"} {
		e.st.fail[k] = errBoom
		if _, err := e.svc.Verify(ctx(), e.mgr.act, v4.Completion.ID); !errors.Is(err, errBoom) {
			t.Fatal(k, err)
		}
		delete(e.st.fail, k)
	}
	e.reports.err = errBoom
	if _, err := e.svc.Verify(ctx(), e.mgr.act, v4.Completion.ID); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	if _, err := e.svc.Pending(ctx(), e.mgr.act); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	e.reports.err = nil
	for _, k := range []string{"pending", "byids"} {
		e.st.fail[k] = errBoom
		if _, err := e.svc.Pending(ctx(), e.hr.act); !errors.Is(err, errBoom) {
			t.Fatal(k, err)
		}
		delete(e.st.fail, k)
	}
}

func TestCorrectionAndWaiver(t *testing.T) {
	e := setup(t)
	c := e.course(t, nil)
	en := e.enrol(t, e.ann, c)
	code(t, mustErr(e.svc.Correct(ctx(), e.hr.act, en.ID, CorrectInput{})), "VALIDATION_ERROR")
	code(t, mustErr(e.svc.Correct(ctx(), e.hr.act, en.ID, CorrectInput{Reason: "x", Score: intp(200)})), "VALIDATION_ERROR")
	code(t, mustErr(e.svc.Correct(ctx(), e.hr.act, uuid.New(), CorrectInput{Reason: "x"})), "NOT_FOUND")
	// HR completes on the worker's behalf when nothing was submitted.
	first, err := e.svc.Correct(ctx(), e.hr.act, en.ID, CorrectInput{Reason: "attended in person", Score: intp(70)})
	if err != nil || first.Status != EnrolCompleted || first.Completion.Attestation != VerifyHR || first.Completion.SupersedesID != nil {
		t.Fatal(first, err)
	}
	// A correction supersedes the live completion; the old row is kept (LN-C12).
	second, err := e.svc.Correct(ctx(), e.hr.act, en.ID, CorrectInput{Reason: "score typo", Score: intp(75)})
	if err != nil || second.Completion.SupersedesID == nil || *second.Completion.SupersedesID != first.Completion.ID {
		t.Fatal(second, err)
	}
	if e.st.completions[first.Completion.ID].Status != CompSuperseded || len(e.st.completions) != 2 {
		t.Fatal("nothing is deleted or edited, only superseded")
	}
	if live := e.st.liveFor(en.ID); live == nil || live.ID != second.Completion.ID {
		t.Fatal("one live completion")
	}
	// Waiver rules.
	w := e.enrol(t, e.bob, c)
	code(t, mustErr(e.svc.Waive(ctx(), e.hr.act, w.ID, " ")), "VALIDATION_ERROR")
	code(t, mustErr(e.svc.Waive(ctx(), e.hr.act, uuid.New(), "x")), "NOT_FOUND")
	waived, err := e.svc.Waive(ctx(), e.hr.act, w.ID, "on long leave")
	if err != nil || waived.Status != EnrolWaived || *waived.WaiveReason != "on long leave" {
		t.Fatal(waived, err)
	}
	code(t, mustErr(e.svc.Waive(ctx(), e.hr.act, w.ID, "again")), "ENROLMENT_STATE")
	code(t, mustErr(e.svc.Waive(ctx(), e.hr.act, en.ID, "x")), "ENROLMENT_STATE")
	code(t, mustErr(e.svc.Correct(ctx(), e.hr.act, w.ID, CorrectInput{Reason: "x"})), "ENROLMENT_STATE")
	code(t, mustErr(e.svc.Complete(ctx(), e.bob.act, w.ID, CompleteInput{})), "ENROLMENT_STATE")
	// A waiver cannot hide a pending verification.
	mc := e.course(t, func(i *CourseInput) { i.Verification = str(VerifyManager) })
	pe := e.enrol(t, e.ann, mc)
	_, _ = e.svc.Complete(ctx(), e.ann.act, pe.ID, CompleteInput{})
	code(t, mustErr(e.svc.Waive(ctx(), e.hr.act, pe.ID, "x")), "COMPLETION_PENDING")
	for _, k := range []string{"live", "event", "save"} {
		fresh := e.enrol(t, e.outsider, e.course(t, nil))
		e.st.fail[k] = errBoom
		if _, err := e.svc.Waive(ctx(), e.hr.act, fresh.ID, "x"); !errors.Is(err, errBoom) {
			t.Fatal(k, err)
		}
		if _, err := e.svc.Correct(ctx(), e.hr.act, fresh.ID, CorrectInput{Reason: "x"}); !errors.Is(err, errBoom) {
			t.Fatal(k, err)
		}
		delete(e.st.fail, k)
	}
}

func TestRetiredCourseKeepsCompletions(t *testing.T) {
	e := setup(t)
	c := e.course(t, func(i *CourseInput) { i.SelfEnrol = boolp(true) })
	done := e.enrol(t, e.ann, c)
	_, err := e.svc.Complete(ctx(), e.ann.act, done.ID, CompleteInput{Score: intp(88)})
	ok(t, err)
	open := e.enrol(t, e.bob, c)
	_, err = e.svc.RetireCourse(ctx(), e.hr.act, c.ID)
	ok(t, err)
	tr, _, err := e.svc.Transcript(ctx(), e.ann.act, 1, 20)
	if err != nil || len(tr) != 1 || tr[0].Completion == nil || *tr[0].Completion.Score != 88 {
		t.Fatalf("LG2 the completion survives retirement: %+v %v", tr, err)
	}
	if v, err := e.svc.Complete(ctx(), e.bob.act, open.ID, CompleteInput{}); err != nil || v.Status != EnrolCompleted {
		t.Fatal("an open enrolment can still be completed", v, err)
	}
	code(t, mustErr(e.svc.EnrolSelf(ctx(), e.outsider.act, SelfEnrolInput{CourseID: str(c.ID.String())})), "COURSE_STATE")
	code(t, e.assignErr(AssignmentInput{TargetKind: TargetAll, CourseID: str(c.ID.String())}), "COURSE_STATE")
}

// --- LN-C15, LN-C16 views and reports ---

func TestViews(t *testing.T) {
	e := setup(t)
	c1, c2 := e.course(t, nil), e.course(t, nil)
	a1 := e.enrol(t, e.ann, c1)
	b1 := e.enrol(t, e.bob, c1)
	_ = e.enrol(t, e.ann, c2)
	_, _ = e.svc.Complete(ctx(), e.ann.act, a1.ID, CompleteInput{})
	_, _ = e.svc.Waive(ctx(), e.hr.act, b1.ID, "n/a")

	if rows, total, err := e.svc.Mine(ctx(), e.ann.act, "", 1, 20); err != nil || total != 2 || len(rows) != 2 {
		t.Fatal(total, err)
	}
	if rows, _, _ := e.svc.Mine(ctx(), e.ann.act, EnrolCompleted, 1, 20); len(rows) != 1 {
		t.Fatal("status filter")
	}
	code(t, mustErr3(e.svc.Mine(ctx(), e.ann.act, "weird", 1, 20)), "VALIDATION_ERROR")
	code(t, mustErr3(e.svc.Mine(ctx(), Actor{TenantID: e.tenant, UserID: uuid.New()}, "", 1, 20)), "EMPLOYEE_NOT_FOUND")
	if rows, total, err := e.svc.Team(ctx(), e.mgr.act, "", 1, 20); err != nil || total != 3 || len(rows) != 3 {
		t.Fatalf("a manager sees both reports' enrolments: %d %v", total, err)
	}
	if rows, _, _ := e.svc.Team(ctx(), e.outsider.act, "", 1, 20); len(rows) != 0 {
		t.Fatal("a non-manager sees no team")
	}
	f := EnrolFilter{CourseID: &c1.ID}
	if rows, _, err := e.svc.Enrolments(ctx(), e.hr.act, f, 1, 20); err != nil || len(rows) != 2 {
		t.Fatal(rows, err)
	}
	f = EnrolFilter{EmployeeID: &e.ann.emp, AssignmentID: e.st.enrolments[a1.ID].AssignmentID}
	if rows, _, _ := e.svc.Enrolments(ctx(), e.hr.act, f, 1, 20); len(rows) != 1 {
		t.Fatal("assignment filter")
	}
	// GetEnrolment: the learner, their manager, a reader; nobody else.
	if v, err := e.svc.GetEnrolment(ctx(), e.ann.act, a1.ID); err != nil || v.Completion == nil {
		t.Fatal(v, err)
	}
	if _, err := e.svc.GetEnrolment(ctx(), e.mgr.act, a1.ID); err != nil {
		t.Fatal("manager", err)
	}
	if _, err := e.svc.GetEnrolment(ctx(), e.hr.act, a1.ID); err != nil {
		t.Fatal("reader", err)
	}
	code(t, mustErr(e.svc.GetEnrolment(ctx(), e.outsider.act, a1.ID)), "NOT_FOUND")
	code(t, mustErr(e.svc.GetEnrolment(ctx(), e.bob.act, a1.ID)), "NOT_FOUND")
	code(t, mustErr(e.svc.GetEnrolment(ctx(), e.hr.act, uuid.New())), "NOT_FOUND")
	e.reports.err = errBoom
	if _, err := e.svc.GetEnrolment(ctx(), e.outsider.act, a1.ID); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	e.reports.err = nil

	// Compliance counts per department and course (LN-C16), no personal data.
	rows, err := e.svc.Compliance(ctx(), e.hr.act, ComplianceFilter{CourseID: &c1.ID})
	if err != nil || len(rows) != 2 {
		t.Fatal(rows, err)
	}
	total := ComplianceRow{}
	for _, r := range rows {
		total.Total += r.Total
		total.Completed += r.Completed
		total.Waived += r.Waived
	}
	if total.Total != 2 || total.Completed != 1 || total.Waived != 1 {
		t.Fatalf("%+v", total)
	}
	mand := true
	if rows, _ := e.svc.Compliance(ctx(), e.hr.act, ComplianceFilter{DepartmentID: &e.dept, Mandatory: &mand}); len(rows) != 2 {
		t.Fatalf("department + mandatory: %+v", rows)
	}
	if rows, err := e.svc.Compliance(ctx(), e.hr.act, ComplianceFilter{CourseID: &c1.ID, DepartmentID: &e.profile}); err != nil || rows == nil || len(rows) != 0 {
		t.Fatal("empty report is an empty list", rows, err)
	}
	e.st.fail["compliance"] = errBoom
	if _, err := e.svc.Compliance(ctx(), e.hr.act, ComplianceFilter{}); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	delete(e.st.fail, "compliance")
	e.st.fail["lives"] = errBoom
	if _, _, err := e.svc.Transcript(ctx(), e.ann.act, 1, 20); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
}

func TestPlanProgress(t *testing.T) {
	e := setup(t)
	c1, c2 := e.course(t, nil), e.course(t, nil)
	p, _ := e.svc.CreatePlan(ctx(), e.hr.act, PlanInput{Name: str("Path")})
	_, err := e.svc.SetItems(ctx(), e.hr.act, p.ID, []ItemInput{{CourseID: c1.ID.String()}, {CourseID: c2.ID.String()}})
	ok(t, err)
	pr, err := e.svc.Progress(ctx(), e.ann.act, p.ID, nil)
	if err != nil || pr.Complete || pr.Items[0].Status != "not_enrolled" {
		t.Fatal(pr, err)
	}
	e.assign(t, AssignmentInput{TargetKind: TargetEmployee, TargetID: str(e.ann.emp.String()), PlanID: str(p.ID.String()), DueDate: str("2026-06-11")})
	e.clock = e.clock.AddDate(0, 0, 2)
	pr, _ = e.svc.Progress(ctx(), e.ann.act, p.ID, nil)
	if pr.Complete || pr.Items[0].Status != EnrolNotStarted || !pr.Items[0].Overdue {
		t.Fatalf("overdue is reported: %+v", pr)
	}
	for _, en := range e.st.sorted() {
		if en.CourseID == c1.ID {
			_, err = e.svc.Complete(ctx(), e.ann.act, en.ID, CompleteInput{})
			ok(t, err)
		} else {
			_, err = e.svc.Waive(ctx(), e.hr.act, en.ID, "excused")
			ok(t, err)
		}
	}
	if pr, _ := e.svc.Progress(ctx(), e.ann.act, p.ID, nil); !pr.Complete {
		t.Fatalf("completed and waived mandatory items complete the plan: %+v", pr)
	}
	if pr, err := e.svc.Progress(ctx(), e.hr.act, p.ID, &e.bob.emp); err != nil || pr.Complete {
		t.Fatal("a reader looks at another worker", pr, err)
	}
	code(t, mustErr(e.svc.Progress(ctx(), e.ann.act, p.ID, &e.bob.emp)), "FORBIDDEN")
	code(t, mustErr(e.svc.Progress(ctx(), e.ann.act, uuid.New(), nil)), "NOT_FOUND")
	code(t, mustErr(e.svc.Progress(ctx(), Actor{TenantID: e.tenant, UserID: uuid.New()}, p.ID, nil)), "EMPLOYEE_NOT_FOUND")
	for _, k := range []string{"planitems", "courseenrolments", "settings"} {
		e.st.fail[k] = errBoom
		if _, err := e.svc.Progress(ctx(), e.ann.act, p.ID, nil); !errors.Is(err, errBoom) {
			t.Fatal(k, err)
		}
		delete(e.st.fail, k)
	}
}

func TestModelsAndHelpers(t *testing.T) {
	var d Date
	if d.Scan(time.Date(2026, 6, 10, 15, 0, 0, 0, time.UTC)) != nil || d.Format(DateLayout) != "2026-06-10" || d.Scan("2026-07-01T00:00:00Z") != nil || d.Scan(5) == nil {
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
	var tags Tags
	v, err := Tags(nil).Value()
	ok(t, err)
	if v != "[]" || tags.Scan("[\"a\"]") != nil || tags.Scan([]byte("[\"b\"]")) != nil || len(tags) != 1 || tags.Scan(1) == nil {
		t.Fatal("tags")
	}
	for _, m := range []interface{ TableName() string }{Settings{}, Course{}, CourseVersion{}, Plan{}, PlanItem{}, Assignment{}, Enrolment{}, Completion{}} {
		if !strings.HasPrefix(m.TableName(), "lrn_") {
			t.Fatal(m.TableName())
		}
	}
	for _, m := range []interface{ BeforeCreate(*gorm.DB) error }{&Course{}, &Plan{}, &Assignment{}, &Enrolment{}, &Completion{}} {
		ok(t, m.BeforeCreate(nil))
	}
	if len(Catalogue()) != 2 || NewModule(nil, Ports{}).Service() == nil || len((&Module{}).RegisterModels()) != 8 || NewStore(nil).DB() != nil || orNotFound(errBoom) != errBoom {
		t.Fatal("module")
	}
	if itoa(7) != "7" {
		t.Fatal("itoa")
	}
}

// A worker on probation, in notice or on leave is still workforce; a leaver is not.
func TestWorkforceStatuses(t *testing.T) {
	e := setup(t)
	c := e.course(t, nil)
	for status, want := range map[string]bool{"active": true, "probation": true, "notice": true, "on_leave": true, "terminated": false, "inactive": false, "resigned": false} {
		p := e.person(status, &e.dept)
		r := e.emps.byID[p.emp]
		r.Status = status
		e.emps.byID[p.emp] = r
		got := e.assign(t, AssignmentInput{TargetKind: TargetEmployee, TargetID: str(p.emp.String()), CourseID: str(c.ID.String())})
		if (got.Matched == 1) != want {
			t.Fatalf("%s: matched %d, want %v", status, got.Matched, want)
		}
	}
	res := e.assign(t, AssignmentInput{TargetKind: TargetDepartment, TargetID: str(e.dept.String()), CourseID: str(c.ID.String())})
	// ann and mgr (active), plus the active, probation, notice and on_leave workers above.
	if res.Matched != 6 {
		t.Fatalf("rule matches the whole workforce in the department: %d", res.Matched)
	}
}

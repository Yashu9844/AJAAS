package records

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/identity/access"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"github.com/jaas/jaas/internal/shared/identityport"
	"gorm.io/gorm"
)

var errBoom = errors.New("boom")

// fakeStore is an in-memory Store; fail injects an error per method name.
type fakeStore struct {
	workers  map[uuid.UUID]*Worker
	contacts map[uuid.UUID]EmergencyContact
	edus     map[uuid.UUID]Education
	priors   map[uuid.UUID]PriorEmployment
	reqs     map[uuid.UUID]ChangeRequest
	applied  []string
	events   []string
	fail     map[string]error
	tick     time.Duration
}

func newFake() *fakeStore {
	return &fakeStore{workers: map[uuid.UUID]*Worker{}, contacts: map[uuid.UUID]EmergencyContact{}, edus: map[uuid.UUID]Education{},
		priors: map[uuid.UUID]PriorEmployment{}, reqs: map[uuid.UUID]ChangeRequest{}, fail: map[string]error{}}
}

func clone[K comparable, V any](m map[K]V) map[K]V {
	out := make(map[K]V, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func (f *fakeStore) InTx(_ context.Context, fn func(*gorm.DB) error) error {
	c, e, p, r := clone(f.contacts), clone(f.edus), clone(f.priors), clone(f.reqs)
	if err := fn(nil); err != nil {
		f.contacts, f.edus, f.priors, f.reqs = c, e, p, r
		return err
	}
	return nil
}
func (f *fakeStore) DB() *gorm.DB { return nil }
func (f *fakeStore) WorkerByUser(_ context.Context, _, u uuid.UUID) (*Worker, error) {
	for _, w := range f.workers {
		if w.UserID == u {
			return w, f.fail["worker"]
		}
	}
	return nil, f.fail["worker"]
}
func (f *fakeStore) Worker(_ context.Context, _, id uuid.UUID) (*Worker, error) {
	return f.workers[id], f.fail["worker"]
}

func (f *fakeStore) stamp() time.Time {
	f.tick += time.Second
	return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).Add(f.tick)
}

func (f *fakeStore) Contacts(_ context.Context, _ *gorm.DB, _, e uuid.UUID) ([]EmergencyContact, error) {
	var out []EmergencyContact
	for _, c := range f.contacts {
		if c.EmployeeProfileID == e {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].IsPrimary && !out[j].IsPrimary || out[i].IsPrimary == out[j].IsPrimary && out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, f.fail["contacts"]
}
func (f *fakeStore) Educations(_ context.Context, _ *gorm.DB, _, e uuid.UUID) ([]Education, error) {
	var out []Education
	for _, r := range f.edus {
		if r.EmployeeProfileID == e {
			out = append(out, r)
		}
	}
	return out, f.fail["educations"]
}
func (f *fakeStore) Priors(_ context.Context, _ *gorm.DB, _, e uuid.UUID) ([]PriorEmployment, error) {
	var out []PriorEmployment
	for _, r := range f.priors {
		if r.EmployeeProfileID == e {
			out = append(out, r)
		}
	}
	return out, f.fail["priors"]
}
func (f *fakeStore) put(op string, row interface{}) error {
	if err := f.fail[op]; err != nil {
		return err
	}
	switch v := row.(type) {
	case *EmergencyContact:
		_ = v.BeforeCreate(nil)
		if v.CreatedAt.IsZero() || op == "create" {
			v.CreatedAt = f.stamp()
		}
		f.contacts[v.ID] = *v
	case *Education:
		_ = v.BeforeCreate(nil)
		f.edus[v.ID] = *v
	case *PriorEmployment:
		_ = v.BeforeCreate(nil)
		f.priors[v.ID] = *v
	case *ChangeRequest:
		_ = v.BeforeCreate(nil)
		f.reqs[v.ID] = *v
	}
	return nil
}
func (f *fakeStore) Create(_ context.Context, _ *gorm.DB, row interface{}) error {
	return f.put("create", row)
}
func (f *fakeStore) Save(_ context.Context, _ *gorm.DB, row interface{}) error {
	return f.put("save", row)
}
func (f *fakeStore) Delete(_ context.Context, _ *gorm.DB, row interface{}) error {
	if err := f.fail["delete"]; err != nil {
		return err
	}
	switch v := row.(type) {
	case *EmergencyContact:
		delete(f.contacts, v.ID)
	case *Education:
		delete(f.edus, v.ID)
	case *PriorEmployment:
		delete(f.priors, v.ID)
	}
	return nil
}
func (f *fakeStore) ClearPrimary(_ context.Context, _ *gorm.DB, _, e uuid.UUID) error {
	for id, c := range f.contacts {
		if c.EmployeeProfileID == e && c.IsPrimary {
			c.IsPrimary = false
			f.contacts[id] = c
		}
	}
	return f.fail["clear"]
}
func (f *fakeStore) Request(_ context.Context, _ *gorm.DB, _, id uuid.UUID) (*ChangeRequest, error) {
	if r, ok := f.reqs[id]; ok {
		return &r, f.fail["request"]
	}
	return nil, f.fail["request"]
}
func (f *fakeStore) LockRequest(ctx context.Context, db *gorm.DB, t, id uuid.UUID) (*ChangeRequest, error) {
	if err := f.fail["lock"]; err != nil {
		return nil, err
	}
	return f.Request(ctx, db, t, id)
}
func (f *fakeStore) OpenRequest(_ context.Context, _ *gorm.DB, _, e uuid.UUID, typ string) (bool, error) {
	for _, r := range f.reqs {
		if r.EmployeeProfileID == e && r.Type == typ && r.Status == StatusSubmitted {
			return true, nil
		}
	}
	return false, f.fail["open"]
}
func (f *fakeStore) Requests(_ context.Context, _ *gorm.DB, _ uuid.UUID, flt RequestFilter, _, _ int) ([]ChangeRequest, int64, error) {
	var out []ChangeRequest
	for _, r := range f.reqs {
		if (flt.EmployeeID == nil || r.EmployeeProfileID == *flt.EmployeeID) && (flt.Status == "" || r.Status == flt.Status) && (flt.Type == "" || r.Type == flt.Type) {
			out = append(out, r)
		}
	}
	return out, int64(len(out)), f.fail["requests"]
}
func (f *fakeStore) Apply(_ context.Context, _ *gorm.DB, _, _ uuid.UUID, typ string, _ Payload) error {
	if err := f.fail["apply"]; err != nil {
		return err
	}
	f.applied = append(f.applied, typ)
	return nil
}
func (f *fakeStore) Event(_ context.Context, _ *gorm.DB, _ uuid.UUID, key string, _ map[string]string) error {
	if err := f.fail["event"]; err != nil {
		return err
	}
	f.events = append(f.events, key)
	return nil
}

type audit struct{ n int }

func (a *audit) Log(context.Context, *gorm.DB, string, string, string, string, string, interface{}, string, string) error {
	a.n++
	return nil
}

type env struct {
	st             *fakeStore
	svc            *Service
	tenant         uuid.UUID
	me, hr, other  Actor
	mine, otherEmp *Worker
}

func newEnv() *env {
	e := &env{st: newFake(), tenant: uuid.New()}
	e.svc = NewService(e.st, &audit{}, func() time.Time { return time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC) })
	e.mine = &Worker{ID: uuid.New(), UserID: uuid.New(), Status: "active"}
	e.otherEmp = &Worker{ID: uuid.New(), UserID: uuid.New(), Status: "active"}
	e.st.workers[e.mine.ID], e.st.workers[e.otherEmp.ID] = e.mine, e.otherEmp
	e.me = Actor{TenantID: e.tenant, UserID: e.mine.UserID}
	e.other = Actor{TenantID: e.tenant, UserID: e.otherEmp.UserID}
	e.hr = Actor{TenantID: e.tenant, UserID: uuid.New(), Sensitive: true}
	return e
}

func code(err error) string {
	var app *sharedErrors.AppError
	if errors.As(err, &app) {
		return app.Code
	}
	if err == nil {
		return ""
	}
	return "?"
}

func want(t *testing.T, err error, c string) {
	t.Helper()
	if code(err) != c {
		t.Fatalf("want %s, got %v", c, err)
	}
}

func sp(s string) *string { return &s }
func ip(n int) *int       { return &n }

func TestContacts(t *testing.T) {
	e := newEnv()
	c1, err := e.svc.SaveContact(context.Background(), e.me, nil, nil, ContactInput{Name: "Asha", Relationship: "Mother", Phone: "+91980001", Email: sp("a@b.c")})
	if err != nil || !c1.IsPrimary {
		t.Fatalf("first is primary: %+v %v", c1, err)
	}
	c2, err := e.svc.SaveContact(context.Background(), e.me, nil, nil, ContactInput{Name: "Ravi", Relationship: "Brother", Phone: "+91980002", IsPrimary: true})
	if err != nil || !c2.IsPrimary || e.st.contacts[c1.ID].IsPrimary {
		t.Fatalf("new primary replaces: %v", err)
	}
	upd, err := e.svc.SaveContact(context.Background(), e.me, nil, &c2.ID, ContactInput{Name: "Ravi K", Relationship: "Brother", Phone: "+91980002"})
	if err != nil || !upd.IsPrimary || upd.Name != "Ravi K" {
		t.Fatalf("update keeps primary: %+v %v", upd, err)
	}
	for _, bad := range []ContactInput{{Name: "", Relationship: "R", Phone: "12345"}, {Name: "N", Relationship: "", Phone: "12345"},
		{Name: "N", Relationship: "R", Phone: "12"}, {Name: "N", Relationship: "R", Phone: "12345", Email: sp("nope")},
		{Name: "N", Relationship: "R", Phone: "12345", Email: sp(strings.Repeat("e", 256))}} {
		_, err = e.svc.SaveContact(context.Background(), e.me, nil, nil, bad)
		want(t, err, "VALIDATION_ERROR")
	}
	_, err = e.svc.SaveContact(context.Background(), e.me, nil, idp(uuid.New()), ContactInput{Name: "N", Relationship: "R", Phone: "12345"})
	want(t, err, "NOT_FOUND")
	for i := 0; i < 3; i++ {
		_, err = e.svc.SaveContact(context.Background(), e.me, nil, nil, ContactInput{Name: "N", Relationship: "R", Phone: "12345"})
		if err != nil {
			t.Fatal(err)
		}
	}
	_, err = e.svc.SaveContact(context.Background(), e.me, nil, nil, ContactInput{Name: "N", Relationship: "R", Phone: "12345"})
	want(t, err, "CONFLICT")
	if err := e.svc.DeleteContact(context.Background(), e.me, nil, c2.ID); err != nil {
		t.Fatal(err)
	}
	list, _ := e.svc.Contacts(context.Background(), e.me, nil)
	if len(list) != 4 || !list[0].IsPrimary {
		t.Fatalf("primary promoted after delete: %+v", list)
	}
	if err := e.svc.DeleteContact(context.Background(), e.me, nil, list[3].ID); err != nil {
		t.Fatal(err)
	}
	want(t, e.svc.DeleteContact(context.Background(), e.me, nil, uuid.New()), "NOT_FOUND")
	// HR on behalf of another worker; unknown worker; caller without profile.
	if _, err := e.svc.Contacts(context.Background(), e.hr, &e.otherEmp.ID); err != nil {
		t.Fatal(err)
	}
	_, err = e.svc.Contacts(context.Background(), e.hr, idp(uuid.New()))
	want(t, err, "EMPLOYEE_NOT_FOUND")
	_, err = e.svc.Contacts(context.Background(), e.hr, nil)
	want(t, err, "EMPLOYEE_NOT_FOUND")
}

func idp(id uuid.UUID) *uuid.UUID { return &id }

func TestEducationAndPrior(t *testing.T) {
	e := newEnv()
	ctx := context.Background()
	ed, err := e.svc.SaveEducation(ctx, e.me, nil, nil, EducationInput{Institution: "IIT", Qualification: "B.Tech", FieldOfStudy: sp("CS"),
		StartYear: ip(2012), EndYear: ip(2016), Grade: sp("8.9")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.svc.SaveEducation(ctx, e.me, nil, &ed.ID, EducationInput{Institution: "IIT M", Qualification: "B.Tech"}); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []EducationInput{{Qualification: "Q"}, {Institution: "I"}, {Institution: "I", Qualification: "Q", StartYear: ip(1800)},
		{Institution: "I", Qualification: "Q", EndYear: ip(2040)}, {Institution: "I", Qualification: "Q", StartYear: ip(2016), EndYear: ip(2012)},
		{Institution: "I", Qualification: "Q", FieldOfStudy: sp(strings.Repeat("f", 151))}, {Institution: "I", Qualification: "Q", Grade: sp(strings.Repeat("g", 51))}} {
		_, err = e.svc.SaveEducation(ctx, e.me, nil, nil, bad)
		want(t, err, "VALIDATION_ERROR")
	}
	_, err = e.svc.SaveEducation(ctx, e.me, nil, idp(uuid.New()), EducationInput{Institution: "I", Qualification: "Q"})
	want(t, err, "NOT_FOUND")
	if l, _ := e.svc.Educations(ctx, e.me, nil); len(l) != 1 {
		t.Fatalf("educations: %d", len(l))
	}
	if err := e.svc.DeleteEducation(ctx, e.me, nil, ed.ID); err != nil {
		t.Fatal(err)
	}
	want(t, e.svc.DeleteEducation(ctx, e.me, nil, ed.ID), "NOT_FOUND")

	p, err := e.svc.SavePrior(ctx, e.me, nil, nil, PriorInput{Employer: "Acme", Title: "SE", StartDate: "2016-07-01", EndDate: "2020-06-30", ReasonForLeaving: sp("growth")})
	if err != nil {
		t.Fatal(err)
	}
	_, err = e.svc.SavePrior(ctx, e.me, nil, nil, PriorInput{Employer: "B", Title: "T", StartDate: "2020-06-30", EndDate: "2021-01-01"})
	want(t, err, "OVERLAP")
	if _, err = e.svc.SavePrior(ctx, e.me, nil, &p.ID, PriorInput{Employer: "Acme", Title: "Senior SE", StartDate: "2016-07-01", EndDate: "2020-06-30"}); err != nil {
		t.Fatal("editing itself is no overlap:", err)
	}
	for _, bad := range []PriorInput{{Title: "T", StartDate: "2010-01-01", EndDate: "2011-01-01"}, {Employer: "E", StartDate: "2010-01-01", EndDate: "2011-01-01"},
		{Employer: "E", Title: "T", StartDate: "x", EndDate: "2011-01-01"}, {Employer: "E", Title: "T", StartDate: "2010-01-01", EndDate: "x"},
		{Employer: "E", Title: "T", StartDate: "2011-01-01", EndDate: "2010-01-01"}, {Employer: "E", Title: "T", StartDate: "2026-01-01", EndDate: "2027-01-01"},
		{Employer: "E", Title: "T", StartDate: "2010-01-01", EndDate: "2011-01-01", ReasonForLeaving: sp(strings.Repeat("r", 501))}} {
		_, err = e.svc.SavePrior(ctx, e.me, nil, nil, bad)
		want(t, err, "VALIDATION_ERROR")
	}
	_, err = e.svc.SavePrior(ctx, e.me, nil, idp(uuid.New()), PriorInput{Employer: "E", Title: "T", StartDate: "2010-01-01", EndDate: "2011-01-01"})
	want(t, err, "NOT_FOUND")
	if l, _ := e.svc.Priors(ctx, e.me, nil); len(l) != 1 {
		t.Fatalf("priors: %d", len(l))
	}
	if err := e.svc.DeletePrior(ctx, e.me, nil, p.ID); err != nil {
		t.Fatal(err)
	}
}

func TestChangeRequests(t *testing.T) {
	e := newEnv()
	ctx := context.Background()
	bank := ChangeInput{Type: TypeBank, Payload: Payload{BankName: sp("HDFC"), BankAccountNumber: sp("50100123456789"), BankRoutingSwift: sp("HDFC0001")}, Reason: sp("moved")}
	r, err := e.svc.Submit(ctx, e.me, bank)
	if err != nil || *r.Payload.BankAccountNumber != "****6789" || r.Status != StatusSubmitted {
		t.Fatalf("submit masked: %+v %v", r, err)
	}
	_, err = e.svc.Submit(ctx, e.me, bank)
	want(t, err, "CONFLICT")
	for _, bad := range []ChangeInput{
		{Type: "ssn"},
		{Type: TypeName, Payload: Payload{LastName: sp("X")}},
		{Type: TypeName, Payload: Payload{FirstName: sp("A")}},
		{Type: TypeName, Payload: Payload{FirstName: sp("A"), LastName: sp("B"), DisplayName: sp(" ")}},
		{Type: TypeAddress},
		{Type: TypeAddress, Payload: Payload{CurrentAddress: sp(" ")}},
		{Type: TypeAddress, Payload: Payload{PermanentAddress: sp(strings.Repeat("a", 1001))}},
		{Type: TypeBank, Payload: Payload{BankAccountNumber: sp("123456"), BankRoutingSwift: sp("ABCD")}},
		{Type: TypeBank, Payload: Payload{BankName: sp("B"), BankAccountNumber: sp("12-345"), BankRoutingSwift: sp("ABCD")}},
		{Type: TypeBank, Payload: Payload{BankName: sp("B"), BankAccountNumber: sp("123456"), BankRoutingSwift: sp("A")}},
		{Type: TypeName, Payload: Payload{FirstName: sp("A"), LastName: sp("B")}, Reason: sp(strings.Repeat("r", 501))},
	} {
		_, err = e.svc.Submit(ctx, e.me, bad)
		want(t, err, "VALIDATION_ERROR")
	}
	n, err := e.svc.Submit(ctx, e.me, ChangeInput{Type: TypeName, Payload: Payload{FirstName: sp("Priya"), LastName: sp("Iyer"), DisplayName: sp("Priya I")}})
	if err != nil {
		t.Fatal(err)
	}
	addr, err := e.svc.Submit(ctx, e.me, ChangeInput{Type: TypeAddress, Payload: Payload{CurrentAddress: sp("1 Road"), PermanentAddress: sp("2 Road")}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = e.svc.Cancel(ctx, e.other, n.ID)
	want(t, err, "NOT_FOUND")
	if c, err := e.svc.Cancel(ctx, e.me, n.ID); err != nil || c.Status != StatusCancelled {
		t.Fatalf("cancel: %+v %v", c, err)
	}
	_, err = e.svc.Approve(ctx, e.me, r.ID, nil)
	want(t, err, "EMPLOYEE_SELF_APPROVAL")
	// An approver whose user owns the worker but who did not file the request is still refused.
	e.st.reqs[addr.ID] = func() ChangeRequest { x := e.st.reqs[addr.ID]; x.RequestedByUserID = uuid.New(); return x }()
	_, err = e.svc.Approve(ctx, e.me, addr.ID, nil)
	want(t, err, "EMPLOYEE_SELF_APPROVAL")
	ok, err := e.svc.Approve(ctx, e.hr, r.ID, sp("verified"))
	if err != nil || ok.Status != StatusApproved || *ok.Payload.BankAccountNumber != "50100123456789" || len(e.st.applied) != 1 {
		t.Fatalf("approve applies, sensitive caller sees the number: %+v %v", ok, err)
	}
	_, err = e.svc.Reject(ctx, e.hr, r.ID, nil)
	want(t, err, "REQUEST_STATE")
	if rj, err := e.svc.Reject(ctx, e.hr, addr.ID, sp("proof needed")); err != nil || rj.Status != StatusRejected {
		t.Fatalf("reject: %+v %v", rj, err)
	}
	_, err = e.svc.Approve(ctx, e.hr, uuid.New(), nil)
	want(t, err, "NOT_FOUND")
	_, err = e.svc.Approve(ctx, e.hr, r.ID, sp(strings.Repeat("c", 1001)))
	want(t, err, "VALIDATION_ERROR")
	mine, total, err := e.svc.Mine(ctx, e.me, "", 1, 20)
	if err != nil || total != 3 || len(mine) != 3 {
		t.Fatalf("mine: %d %v", total, err)
	}
	all, _, err := e.svc.List(ctx, e.hr, RequestFilter{Status: StatusApproved, Type: TypeBank, EmployeeID: &e.mine.ID}, 1, 20)
	if err != nil || len(all) != 1 {
		t.Fatalf("queue filter: %d %v", len(all), err)
	}
	for _, f := range []RequestFilter{{Status: "open"}, {Type: "ssn"}} {
		_, _, err = e.svc.List(ctx, e.hr, f, 1, 20)
		want(t, err, "VALIDATION_ERROR")
	}
	if g, err := e.svc.Get(ctx, e.me, r.ID); err != nil || *g.Payload.BankAccountNumber != "****6789" {
		t.Fatalf("get masked: %+v %v", g, err)
	}
	_, err = e.svc.Get(ctx, e.hr, uuid.New())
	want(t, err, "NOT_FOUND")
	_, _, err = e.svc.Mine(ctx, e.hr, "", 1, 20)
	want(t, err, "EMPLOYEE_NOT_FOUND")
	_, err = e.svc.Cancel(ctx, e.hr, r.ID)
	want(t, err, "EMPLOYEE_NOT_FOUND")
	_, err = e.svc.Submit(ctx, e.hr, ChangeInput{Type: TypeName, Payload: Payload{FirstName: sp("A"), LastName: sp("B")}})
	want(t, err, "EMPLOYEE_NOT_FOUND")
	if len(e.st.events) != 5 {
		t.Fatalf("events: %v", e.st.events)
	}
}

func TestFailures(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		op  string
		run func(e *env) error
	}{
		{"worker", func(e *env) error { _, err := e.svc.Contacts(ctx, e.me, nil); return err }},
		{"contacts", func(e *env) error {
			_, err := e.svc.SaveContact(ctx, e.me, nil, nil, ContactInput{Name: "N", Relationship: "R", Phone: "12345"})
			return err
		}},
		{"create", func(e *env) error {
			_, err := e.svc.SaveContact(ctx, e.me, nil, nil, ContactInput{Name: "N", Relationship: "R", Phone: "12345"})
			return err
		}},
		{"clear", func(e *env) error {
			_, err := e.svc.SaveContact(ctx, e.me, nil, nil, ContactInput{Name: "N", Relationship: "R", Phone: "12345"})
			return err
		}},
		{"contacts", func(e *env) error { return e.svc.DeleteContact(ctx, e.me, nil, uuid.New()) }},
		{"educations", func(e *env) error {
			_, err := e.svc.SaveEducation(ctx, e.me, nil, nil, EducationInput{Institution: "I", Qualification: "Q"})
			return err
		}},
		{"educations", func(e *env) error { return e.svc.DeleteEducation(ctx, e.me, nil, uuid.New()) }},
		{"priors", func(e *env) error {
			_, err := e.svc.SavePrior(ctx, e.me, nil, nil, PriorInput{Employer: "E", Title: "T", StartDate: "2010-01-01", EndDate: "2011-01-01"})
			return err
		}},
		{"priors", func(e *env) error { return e.svc.DeletePrior(ctx, e.me, nil, uuid.New()) }},
		{"open", func(e *env) error {
			_, err := e.svc.Submit(ctx, e.me, ChangeInput{Type: TypeName, Payload: Payload{FirstName: sp("A"), LastName: sp("B")}})
			return err
		}},
		{"event", func(e *env) error {
			_, err := e.svc.Submit(ctx, e.me, ChangeInput{Type: TypeName, Payload: Payload{FirstName: sp("A"), LastName: sp("B")}})
			return err
		}},
		{"requests", func(e *env) error { _, _, err := e.svc.List(ctx, e.hr, RequestFilter{}, 1, 20); return err }},
		{"request", func(e *env) error { _, err := e.svc.Get(ctx, e.hr, uuid.New()); return err }},
		{"lock", func(e *env) error { _, err := e.svc.Approve(ctx, e.hr, uuid.New(), nil); return err }},
	} {
		t.Run(tc.op, func(t *testing.T) {
			e := newEnv()
			e.st.fail[tc.op] = errBoom
			if err := tc.run(e); !errors.Is(err, errBoom) {
				t.Fatalf("want boom, got %v", err)
			}
		})
	}
	for _, op := range []string{"apply", "save", "event", "worker"} {
		t.Run("approve/"+op, func(t *testing.T) {
			e := newEnv()
			r, err := e.svc.Submit(ctx, e.me, ChangeInput{Type: TypeAddress, Payload: Payload{CurrentAddress: sp("1 Road")}})
			if err != nil {
				t.Fatal(err)
			}
			e.st.fail[op] = errBoom
			if _, err := e.svc.Approve(ctx, e.hr, r.ID, nil); !errors.Is(err, errBoom) {
				t.Fatalf("want boom, got %v", err)
			}
			if e.st.reqs[r.ID].Status != StatusSubmitted {
				t.Fatal("rolled back")
			}
		})
	}
	e := newEnv()
	c, _ := e.svc.SaveContact(ctx, e.me, nil, nil, ContactInput{Name: "N", Relationship: "R", Phone: "12345"})
	_, _ = e.svc.SaveContact(ctx, e.me, nil, nil, ContactInput{Name: "M", Relationship: "R", Phone: "12345"})
	for _, op := range []string{"delete", "save"} {
		e.st.fail = map[string]error{op: errBoom}
		if err := e.svc.DeleteContact(ctx, e.me, nil, c.ID); !errors.Is(err, errBoom) {
			t.Fatalf("%s: %v", op, err)
		}
	}
}

func TestModelsCodec(t *testing.T) {
	var p Payload
	if err := p.Scan(`{"first_name":"A"}`); err != nil || *p.FirstName != "A" {
		t.Fatal("scan string")
	}
	if err := p.Scan([]byte(`{}`)); err != nil || p.Scan(nil) != nil || p.Scan(1) == nil {
		t.Fatal("scan variants")
	}
	if v, err := (Payload{LastName: sp("B")}).Value(); err != nil || !strings.Contains(v.(string), "last_name") {
		t.Fatal("value")
	}
	var d Date
	if err := d.UnmarshalJSON([]byte(`"2020-01-02"`)); err != nil || d.Year() != 2020 {
		t.Fatal("date json")
	}
	if d.UnmarshalJSON([]byte(`"x"`)) == nil || d.UnmarshalJSON([]byte(`1`)) == nil {
		t.Fatal("date json errors")
	}
	if b, _ := d.MarshalJSON(); string(b) != `"2020-01-02"` {
		t.Fatal("marshal")
	}
	if v, _ := d.Value(); v != "2020-01-02" {
		t.Fatal("value")
	}
	if d.Scan(time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)) != nil || d.Scan("2022-02-02T00:00:00Z") != nil || d.Year() != 2022 || d.Scan(1) == nil {
		t.Fatal("date scan")
	}
	for _, m := range []interface{ TableName() string }{EmergencyContact{}, Education{}, PriorEmployment{}, ChangeRequest{}} {
		if !strings.HasPrefix(m.TableName(), "employee_") {
			t.Fatal(m.TableName())
		}
	}
	r := &ChangeRequest{}
	_ = r.BeforeCreate(nil)
	if r.ID == uuid.Nil {
		t.Fatal("id")
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
	NewHandler(e.svc).Register(r.Group("/employees/me"), r.Group("/employees/:id"), r.Group("/employee-change-requests"), g)
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
	e := newEnv()
	me := router(e, e.mine.UserID)
	hr := router(e, uuid.New())
	check := func(r *gin.Engine, method, path, body string, status int) *httptest.ResponseRecorder {
		t.Helper()
		w := do(r, method, path, body)
		if w.Code != status {
			t.Fatalf("%s %s → %d %s", method, path, w.Code, w.Body.String())
		}
		return w
	}
	c := check(me, "POST", "/employees/me/emergency-contacts", `{"name":"A","relationship":"M","phone":"12345"}`, 201)
	id := c.Body.String()[strings.Index(c.Body.String(), `"id":"`)+6:][:36]
	check(me, "GET", "/employees/me/emergency-contacts", "", 200)
	check(me, "PUT", "/employees/me/emergency-contacts/"+id, `{"name":"B","relationship":"M","phone":"12345"}`, 200)
	check(me, "PUT", "/employees/me/emergency-contacts/"+id, `{`, 400)
	check(me, "DELETE", "/employees/me/emergency-contacts/"+id, "", 204)
	check(me, "DELETE", "/employees/me/emergency-contacts/"+id, "", 404)
	check(me, "DELETE", "/employees/me/emergency-contacts/bad", "", 400)
	check(me, "POST", "/employees/me/education", `{"institution":"I","qualification":"Q"}`, 201)
	check(me, "POST", "/employees/me/prior-employment", `{"employer":"E","title":"T","start_date":"2010-01-01","end_date":"2011-01-01"}`, 201)
	check(hr, "GET", "/employees/"+e.mine.ID.String()+"/education", "", 200)
	check(hr, "GET", "/employees/bad/education", "", 400)
	check(router(e, uuid.New(), "update"), "POST", "/employees/"+e.mine.ID.String()+"/education", `{"institution":"I","qualification":"Q"}`, 403)
	check(router(e, uuid.New(), "read"), "GET", "/employees/"+e.mine.ID.String()+"/education", "", 403)
	r := check(me, "POST", "/employees/me/change-requests", `{"type":"name","payload":{"first_name":"A","last_name":"B"}}`, 201)
	rid := r.Body.String()[strings.Index(r.Body.String(), `"id":"`)+6:][:36]
	check(me, "POST", "/employees/me/change-requests", `{`, 400)
	check(me, "GET", "/employees/me/change-requests?status=submitted", "", 200)
	check(me, "POST", "/employee-change-requests/"+rid+"/approve", "", 403)
	check(hr, "GET", "/employee-change-requests?employee_id="+e.mine.ID.String(), "", 200)
	check(hr, "GET", "/employee-change-requests?employee_id=bad", "", 400)
	check(hr, "GET", "/employee-change-requests?status=bad", "", 400)
	check(hr, "GET", "/employee-change-requests/"+rid, "", 200)
	check(hr, "POST", "/employee-change-requests/"+rid+"/approve", `{`, 400)
	check(hr, "POST", "/employee-change-requests/"+rid+"/reject", `{"comment":"no"}`, 200)
	check(hr, "POST", "/employee-change-requests/"+rid+"/approve", "", 409)
	r2 := check(me, "POST", "/employees/me/change-requests", `{"type":"name","payload":{"first_name":"A","last_name":"B"}}`, 201)
	rid2 := r2.Body.String()[strings.Index(r2.Body.String(), `"id":"`)+6:][:36]
	check(me, "POST", "/employees/me/change-requests/"+rid2+"/cancel", "", 200)
	r3 := check(me, "POST", "/employees/me/change-requests", `{"type":"name","payload":{"first_name":"A","last_name":"B"}}`, 201)
	check(hr, "POST", "/employee-change-requests/"+r3.Body.String()[strings.Index(r3.Body.String(), `"id":"`)+6:][:36]+"/approve", `{"comment":"ok"}`, 200)
	anon := router(e, uuid.Nil)
	check(anon, "GET", "/employees/me/education", "", 401)
	e.st.fail["requests"] = errBoom
	if w := do(hr, "GET", "/employee-change-requests", ""); w.Code != 500 || strings.Contains(w.Body.String(), "boom") {
		t.Fatalf("opaque 500: %s", w.Body.String())
	}
	_ = http.StatusOK
}

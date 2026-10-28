package project

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/testdb"
	"github.com/jaas/jaas/internal/shared/identityport"
	"gorm.io/gorm"
)

type fakeUsers struct{ active map[uuid.UUID]bool }

func (f fakeUsers) GetByID(_ context.Context, _ *gorm.DB, tenantID, id uuid.UUID) (*identityport.User, error) {
	if !f.active[id] {
		return nil, nil
	}
	return &identityport.User{ID: id, TenantID: tenantID, Status: "active"}, nil
}
func (fakeUsers) DeactivateUser(context.Context, *gorm.DB, uuid.UUID, uuid.UUID, uuid.UUID) error {
	return nil
}
func (fakeUsers) ActivateUser(context.Context, *gorm.DB, uuid.UUID, uuid.UUID) error { return nil }
func (fakeUsers) InviteUser(context.Context, *gorm.DB, uuid.UUID, identityport.Invite, uuid.UUID) (*identityport.User, error) {
	return nil, nil
}

type fakeAudit struct{ actions []string }

func (f *fakeAudit) Log(_ context.Context, _ *gorm.DB, _, _ string, action, _, _ string, _ interface{}, _, _ string) error {
	f.actions = append(f.actions, action)
	return nil
}

// fakePeople is an in-memory ports.People; members maps a department, team or manager id to its users.
type fakePeople struct {
	members map[uuid.UUID][]uuid.UUID
	err     error
}

func (f *fakePeople) DepartmentMembers(_ context.Context, _, id uuid.UUID) ([]uuid.UUID, error) {
	return f.members[id], f.err
}
func (f *fakePeople) TeamMembers(_ context.Context, _, id uuid.UUID) ([]uuid.UUID, error) {
	return f.members[id], f.err
}
func (f *fakePeople) DirectReports(_ context.Context, _, id uuid.UUID) ([]uuid.UUID, error) {
	return f.members[id], f.err
}

type env struct {
	people *fakePeople
	t      *testing.T
	db     *gorm.DB
	svc    *Service
	audit  *fakeAudit
	users  fakeUsers
	tenant uuid.UUID
}

func newEnv(t *testing.T) *env {
	t.Helper()
	db := testdb.Open(t)
	e := &env{t: t, db: db, audit: &fakeAudit{}, users: fakeUsers{active: map[uuid.UUID]bool{}}, tenant: uuid.New(),
		people: &fakePeople{members: map[uuid.UUID][]uuid.UUID{}}}
	e.svc = NewService(db, Ports{Users: e.users, Audit: e.audit, People: e.people})
	return e
}

// user registers an active login and returns an actor for it; admin sets the tenant-level flags.
func (e *env) user(flags ...string) api.Actor {
	id := uuid.New()
	e.users.active[id] = true
	a := api.Actor{TenantID: e.tenant, UserID: id}
	for _, f := range flags {
		switch f {
		case "create":
			a.CreateProject = true
		case "readall":
			a.ReadAll = true
		case "admin":
			a.ManageAll, a.CreateProject, a.ManageRoles = true, true, true
		case "roles":
			a.ManageRoles = true
		}
	}
	return a
}

func (e *env) project(owner api.Actor, key string, opts ...func(*CreateInput)) *Project {
	e.t.Helper()
	in := CreateInput{Key: key, Name: "Project " + key}
	for _, o := range opts {
		o(&in)
	}
	p, err := e.svc.Create(context.Background(), owner, in)
	if err != nil {
		e.t.Fatalf("create %s: %v", key, err)
	}
	return p
}

func (e *env) count(sql string, args ...any) int64 {
	var n int64
	e.db.Raw(sql, args...).Scan(&n)
	return n
}

func wantCode(t *testing.T, err error, code string) {
	t.Helper()
	type coded interface{ Error() string }
	if err == nil {
		t.Fatalf("want error %s, got nil", code)
	}
	if !contains(err.Error(), code) {
		t.Fatalf("want error %s, got %v", code, err)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

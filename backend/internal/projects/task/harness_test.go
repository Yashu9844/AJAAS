package task

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
	"github.com/jaas/jaas/internal/projects/project"
	"github.com/jaas/jaas/internal/projects/testdb"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"github.com/jaas/jaas/internal/shared/identityport"
	"gorm.io/gorm"
)

var bg = context.Background()

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

type fakeAudit struct {
	mu      sync.Mutex
	actions []string
	metas   []string
}

func (f *fakeAudit) Log(_ context.Context, _ *gorm.DB, _, _ string, action, _, _ string, meta interface{}, _, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.actions = append(f.actions, action)
	f.metas = append(f.metas, fmt.Sprint(meta))
	return nil
}

func (f *fakeAudit) has(action string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, a := range f.actions {
		if a == action {
			return true
		}
	}
	return false
}

type env struct {
	t      *testing.T
	db     *gorm.DB
	proj   *project.Service
	svc    *Service
	audit  *fakeAudit
	users  fakeUsers
	tenant uuid.UUID
}

func newEnv(t *testing.T) *env {
	t.Helper()
	db := testdb.Open(t)
	e := &env{t: t, db: db, audit: &fakeAudit{}, users: fakeUsers{active: map[uuid.UUID]bool{}}, tenant: uuid.New()}
	e.proj = project.NewService(db, project.Ports{Users: e.users, Audit: e.audit})
	e.svc = NewService(db, e.proj, Ports{Audit: e.audit})
	return e
}

// user registers an active login; flags: create, readall, admin.
func (e *env) user(flags ...string) api.Actor {
	id := uuid.New()
	e.users.active[id] = true
	a := api.Actor{TenantID: e.tenant, UserID: id, CreateProject: true}
	for _, f := range flags {
		switch f {
		case "readall":
			a.ReadAll = true
		case "admin":
			a.ManageAll, a.ManageRoles = true, true
		}
	}
	return a
}

func (e *env) project(owner api.Actor, key string) *project.Project {
	e.t.Helper()
	p, err := e.proj.Create(bg, owner, project.CreateInput{Key: key, Name: "Project " + key})
	if err != nil {
		e.t.Fatalf("create project %s: %v", key, err)
	}
	return p
}

// member adds a user to the project with a built-in role and returns their actor.
func (e *env) member(owner api.Actor, p *project.Project, role string) api.Actor {
	e.t.Helper()
	a := e.user()
	a.CreateProject = false
	if _, err := e.proj.AddMember(bg, owner, p.ID, project.MemberInput{UserID: a.UserID, Role: role}); err != nil {
		e.t.Fatalf("add member: %v", err)
	}
	return a
}

func (e *env) col(p *project.Project, key string) uuid.UUID {
	var ids []uuid.UUID
	if err := e.db.Raw(`SELECT id FROM board_columns WHERE project_id = ? AND key = ?`, p.ID, key).Scan(&ids).Error; err != nil || len(ids) == 0 {
		e.t.Fatalf("column %s: %v", key, err)
	}
	return ids[0]
}

// mk creates a task with a title in the default column unless in.ColumnID is set.
func (e *env) mk(a api.Actor, p *project.Project, title string, mod ...func(*CreateInput)) *View {
	e.t.Helper()
	in := CreateInput{Title: title}
	for _, m := range mod {
		m(&in)
	}
	v, err := e.svc.Create(bg, a, p.ID, in)
	if err != nil {
		e.t.Fatalf("create task %q: %v", title, err)
	}
	return v
}

func (e *env) count(sql string, args ...any) int64 {
	var n int64
	e.db.Raw(sql, args...).Scan(&n)
	return n
}

// code returns the stable error code of err ("" for nil, "VERSION_CONFLICT" for stale versions).
func code(err error) string {
	if err == nil {
		return ""
	}
	if _, ok := kernel.AsVersionConflict(err); ok {
		return "VERSION_CONFLICT"
	}
	var app *sharedErrors.AppError
	if errors.As(err, &app) {
		return app.Code
	}
	return "INTERNAL:" + err.Error()
}

func wantCode(t *testing.T, err error, want string) {
	t.Helper()
	if got := code(err); got != want {
		t.Fatalf("want error %s, got %q (%v)", want, got, err)
	}
}

func str(s string) *string { return &s }
func num(n int) *int       { return &n }
func boolp(b bool) *bool   { return &b }

// outbox returns the payloads of the events with the routing key written so far.
func (e *env) outbox(key string) []string {
	var rows []string
	e.db.Raw(`SELECT payload::text FROM projects_events_outbox WHERE routing_key = ? ORDER BY created_at, id`, key).Scan(&rows)
	return rows
}

func (e *env) activityTypes(taskID uuid.UUID) []string {
	var rows []string
	e.db.Raw(`SELECT type FROM activity WHERE task_id = ? ORDER BY seq`, taskID).Scan(&rows)
	return rows
}

func has(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func (e *env) label(a api.Actor, p *project.Project, name string) *project.Label {
	e.t.Helper()
	l, err := e.proj.CreateLabel(bg, a, &p.ID, project.LabelInput{Name: name})
	if err != nil {
		e.t.Fatalf("label: %v", err)
	}
	return l
}

func (e *env) milestone(a api.Actor, p *project.Project, name string) *project.Milestone {
	e.t.Helper()
	m, err := e.proj.CreateMilestone(bg, a, p.ID, project.MilestoneInput{Name: &name})
	if err != nil {
		e.t.Fatalf("milestone: %v", err)
	}
	return m
}

// addColumn adds a board column with a category and optional WIP limit.
func (e *env) addColumn(a api.Actor, p *project.Project, name, category string, wip *int, hard bool) uuid.UUID {
	e.t.Helper()
	c, err := e.proj.CreateColumn(bg, a, p.ID, project.ColumnInput{Name: &name, Category: &category, WipLimit: wip, WipHard: &hard})
	if err != nil {
		e.t.Fatalf("column: %v", err)
	}
	return c.ID
}

func projectIn(key, visibility string) project.CreateInput {
	return project.CreateInput{Key: key, Name: "Project " + key, Visibility: visibility}
}

func memberIn(user uuid.UUID, role string) project.MemberInput {
	return project.MemberInput{UserID: user, Role: role}
}

func roleIn(name string, perms ...string) project.RoleInput {
	return project.RoleInput{Name: name, Permissions: perms}
}

func customMemberIn(user, role uuid.UUID) project.MemberInput {
	return project.MemberInput{UserID: user, CustomRoleID: &role}
}

var versionConflictForTest = kernel.VersionConflict{Current: 7}

type errPlain struct{}

func (errPlain) Error() string { return "boom: secret detail" }

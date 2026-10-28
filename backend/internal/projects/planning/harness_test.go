package planning

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
	"github.com/jaas/jaas/internal/projects/project"
	"github.com/jaas/jaas/internal/projects/testdb"
	"github.com/jaas/jaas/internal/shared/identityport"
	"gorm.io/gorm"
)

var bg = context.Background()

type fakeUsers struct {
	mu     *sync.Mutex
	active map[uuid.UUID]bool
}

func (f fakeUsers) GetByID(_ context.Context, _ *gorm.DB, tenantID, id uuid.UUID) (*identityport.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
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
}

func (f *fakeAudit) Log(_ context.Context, _ *gorm.DB, _, _ string, action, _, _ string, _ interface{}, _, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.actions = append(f.actions, action)
	return nil
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
	e := &env{t: t, db: db, audit: &fakeAudit{}, users: fakeUsers{mu: &sync.Mutex{}, active: map[uuid.UUID]bool{}}, tenant: uuid.New()}
	e.proj = project.NewService(db, project.Ports{Users: e.users, Audit: e.audit})
	e.svc = NewService(db, e.proj, Ports{Users: e.users, Audit: e.audit})
	return e
}

// user registers an active login; flags: create, readall, admin.
func (e *env) user(flags ...string) api.Actor {
	id := uuid.New()
	e.users.mu.Lock()
	e.users.active[id] = true
	e.users.mu.Unlock()
	a := api.Actor{TenantID: e.tenant, UserID: id}
	for _, f := range flags {
		switch f {
		case "create":
			a.CreateProject = true
		case "readall":
			a.ReadAll = true
		case "admin":
			a.ManageAll, a.CreateProject, a.ManageRoles = true, true, true
		}
	}
	return a
}

func (e *env) project(owner api.Actor, key, mode string) *project.Project {
	e.t.Helper()
	p, err := e.proj.Create(bg, owner, project.CreateInput{Key: key, Name: "Project " + key, Mode: mode})
	if err != nil {
		e.t.Fatalf("create %s: %v", key, err)
	}
	return p
}

func (e *env) scrum(owner api.Actor, key string) *project.Project {
	return e.project(owner, key, "scrum")
}

func (e *env) join(owner api.Actor, p *project.Project, u api.Actor, role string) {
	e.t.Helper()
	if _, err := e.proj.AddMember(bg, owner, p.ID, project.MemberInput{UserID: u.UserID, Role: role}); err != nil {
		e.t.Fatalf("add member: %v", err)
	}
}

// col returns the first column of the category.
func (e *env) col(p *project.Project, category string) uuid.UUID {
	var id uuid.UUID
	_ = e.db.Raw(`SELECT id FROM board_columns WHERE project_id = ? AND category = ? ORDER BY position LIMIT 1`, p.ID, category).Row().Scan(&id)
	if id == uuid.Nil {
		e.t.Fatalf("no %s column", category)
	}
	return id
}

// canceledCol adds a canceled column through the project context.
func (e *env) canceledCol(owner api.Actor, p *project.Project) uuid.UUID {
	name, cat := "Dropped", "canceled"
	c, err := e.proj.CreateColumn(bg, owner, p.ID, project.ColumnInput{Name: &name, Category: &cat})
	if err != nil {
		e.t.Fatal(err)
	}
	return c.ID
}

// tk describes a task row inserted directly (the task context owns creation in production).
type tk struct {
	title    string
	desc     string
	col      uuid.UUID
	sprint   *uuid.UUID
	due      *time.Time
	start    string
	est      *int
	assignee *uuid.UUID
	archived bool
	deleted  bool
}

func (e *env) task(p *project.Project, t tk) uuid.UUID {
	e.t.Helper()
	if t.title == "" {
		t.title = "Task"
	}
	var n int
	e.db.Raw(`UPDATE projects SET task_counter = task_counter + 1 WHERE id = ? RETURNING task_counter`, p.ID).Scan(&n)
	var start any
	if t.start != "" {
		start = t.start
	}
	var archived, deleted any
	if t.archived {
		archived = time.Now().UTC()
	}
	if t.deleted {
		deleted = time.Now().UTC()
	}
	id := uuid.New()
	err := e.db.Exec(`INSERT INTO tasks (id, tenant_id, project_id, number, title, description, column_id, reporter_user_id, rank, sprint_id,
		due_at, start_on, estimate_minutes, archived_at, deleted_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, p.TenantID, p.ID, n, t.title, t.desc, t.col, p.OwnerUserID, fmt.Sprintf("r%08d", n), t.sprint, t.due, start, t.est, archived, deleted).Error
	if err != nil {
		e.t.Fatalf("insert task: %v", err)
	}
	if t.assignee != nil {
		e.assign(p, id, *t.assignee)
	}
	return id
}

func (e *env) assign(p *project.Project, task, user uuid.UUID) {
	e.t.Helper()
	if err := e.db.Exec(`INSERT INTO task_assignees (task_id, user_id, tenant_id, assigned_by) VALUES (?, ?, ?, ?)`, task, user, p.TenantID, p.OwnerUserID).Error; err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) sprint(owner api.Actor, p *project.Project, name, start, end string) *Sprint {
	e.t.Helper()
	in := SprintInput{Name: &name}
	if start != "" {
		in.StartOn = date(start)
	}
	if end != "" {
		in.EndOn = date(end)
	}
	sp, err := e.svc.CreateSprint(bg, owner, p.ID, in)
	if err != nil {
		e.t.Fatalf("create sprint: %v", err)
	}
	return sp
}

func (e *env) count(sql string, args ...any) int64 {
	var n int64
	e.db.Raw(sql, args...).Scan(&n)
	return n
}

func (e *env) sprintOf(task uuid.UUID) *uuid.UUID {
	var s *uuid.UUID
	if err := e.db.Raw(`SELECT sprint_id FROM tasks WHERE id = ?`, task).Row().Scan(&s); err != nil {
		e.t.Fatal(err)
	}
	return s
}

func wantCode(t *testing.T, err error, code string) {
	t.Helper()
	if err == nil {
		t.Fatalf("want error %s, got nil", code)
	}
	if !strings.Contains(err.Error(), code) {
		t.Fatalf("want error %s, got %v", code, err)
	}
}

func ptr[T any](v T) *T { return &v }

func at(s string) *time.Time {
	t, _ := time.Parse(time.RFC3339, s)
	return &t
}

func date(s string) *kernel.Date {
	d, _ := kernel.ParseDate(s)
	return &d
}

package recurrence

import (
	"context"
	"errors"
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

// spawned is one copy the fake task actor created.
type spawned struct {
	id    uuid.UUID
	agent string
	user  uuid.UUID
	dates Dates
}

// fakeTasks reads templates and creates copies on the real tables.
type fakeTasks struct {
	db      *gorm.DB
	mu      sync.Mutex
	copies  []spawned
	failAll error
	partial error
}

func (f *fakeTasks) Template(ctx context.Context, a api.Actor, id uuid.UUID) (Template, error) {
	var rows []struct {
		ID, ProjectID string
		StartOn       *kernel.Date
		DueAt         *time.Time
		CreatedAt     time.Time
	}
	err := f.db.WithContext(ctx).Raw(`SELECT id::text AS id, project_id::text AS project_id, start_on, due_at, created_at FROM tasks
		WHERE id = ? AND tenant_id = ? AND deleted_at IS NULL`, id, a.TenantID).Scan(&rows).Error
	if err != nil {
		return Template{}, err
	}
	if len(rows) == 0 {
		return Template{}, kernel.NotFound("task")
	}
	r := rows[0]
	return Template{ID: uuid.MustParse(r.ID), ProjectID: uuid.MustParse(r.ProjectID), StartOn: r.StartOn, DueAt: r.DueAt, CreatedAt: r.CreatedAt}, nil
}

func (f *fakeTasks) Spawn(ctx context.Context, a api.Actor, templateID uuid.UUID, d Dates) (uuid.UUID, error) {
	if f.failAll != nil {
		return uuid.Nil, f.failAll
	}
	var proj string
	if err := f.db.Raw(`SELECT project_id::text FROM tasks WHERE id = ?`, templateID).Scan(&proj).Error; err != nil {
		return uuid.Nil, err
	}
	id := uuid.New()
	err := f.db.Exec(`INSERT INTO tasks (id, tenant_id, project_id, number, title, column_id, reporter_user_id, rank, start_on, due_at)
		SELECT ?, tenant_id, project_id, (SELECT coalesce(max(number), 0) + 1 FROM tasks WHERE project_id = ?), title, column_id, ?, 'W', ?, ? FROM tasks WHERE id = ?`,
		id, proj, a.UserID, d.StartOn, d.DueAt, templateID).Error
	if err != nil {
		return uuid.Nil, err
	}
	f.mu.Lock()
	f.copies = append(f.copies, spawned{id, kernel.AgentFrom(ctx), a.UserID, d})
	f.mu.Unlock()
	return id, f.partial
}

func (f *fakeTasks) n() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.copies)
}

type env struct {
	t      *testing.T
	db     *gorm.DB
	proj   *project.Service
	tasks  *fakeTasks
	svc    *Service
	audit  *fakeAudit
	users  fakeUsers
	tenant uuid.UUID
	clock  time.Time
	n      int
}

func newEnv(t *testing.T) *env {
	t.Helper()
	db := testdb.Open(t)
	e := &env{t: t, db: db, audit: &fakeAudit{}, users: fakeUsers{active: map[uuid.UUID]bool{}}, tenant: uuid.New(),
		clock: time.Date(2026, 3, 10, 9, 30, 0, 0, time.UTC)}
	e.proj = project.NewService(db, project.Ports{Users: e.users, Audit: e.audit})
	e.tasks = &fakeTasks{db: db}
	e.svc = NewService(db, e.proj, e.tasks, Ports{Audit: e.audit})
	e.svc.now = func() time.Time { return e.clock }
	return e
}

func (e *env) user() api.Actor {
	id := uuid.New()
	e.users.active[id] = true
	return api.Actor{TenantID: e.tenant, UserID: id, CreateProject: true}
}

type fixture struct {
	p                    *project.Project
	owner, member, viewr api.Actor
	tpl                  uuid.UUID
}

func (e *env) fixture(key string) fixture {
	e.t.Helper()
	owner := e.user()
	p, err := e.proj.Create(bg, owner, project.CreateInput{Key: key, Name: "P " + key})
	if err != nil {
		e.t.Fatal(err)
	}
	f := fixture{p: p, owner: owner, member: e.user(), viewr: e.user()}
	for a, role := range map[*api.Actor]string{&f.member: api.RoleMember, &f.viewr: api.RoleViewer} {
		if _, err := e.proj.AddMember(bg, owner, p.ID, project.MemberInput{UserID: a.UserID, Role: role}); err != nil {
			e.t.Fatal(err)
		}
	}
	f.tpl = e.task(p, owner.UserID)
	return f
}

func (e *env) task(p *project.Project, reporter uuid.UUID) uuid.UUID {
	e.t.Helper()
	e.n++
	id := uuid.New()
	err := e.db.Exec(`INSERT INTO tasks (id, tenant_id, project_id, number, title, column_id, reporter_user_id, rank)
		VALUES (?, ?, ?, ?, 'Weekly report', (SELECT id FROM board_columns WHERE project_id = ? AND is_default), ?, 'V')`,
		id, e.tenant, p.ID, e.n, p.ID, reporter).Error
	if err != nil {
		e.t.Fatal(err)
	}
	return id
}

func (e *env) create(f fixture, in Input) *Recurrence {
	e.t.Helper()
	in.TemplateTaskID = f.tpl
	r, err := e.svc.Create(bg, f.owner, f.p.ID, in)
	if err != nil {
		e.t.Fatalf("create recurrence: %v", err)
	}
	return r
}

// due makes the recurrence due at the given time.
func (e *env) due(r *Recurrence, at time.Time) {
	e.t.Helper()
	if err := e.db.Exec(`UPDATE task_recurrences SET next_run_at = ? WHERE id = ?`, at, r.ID).Error; err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) reload(r *Recurrence) Recurrence {
	var got Recurrence
	if err := e.db.Take(&got, "id = ?", r.ID).Error; err != nil {
		e.t.Fatal(err)
	}
	return got
}

func (e *env) tick() int {
	e.t.Helper()
	n, err := e.svc.Tick(bg)
	if err != nil {
		e.t.Fatalf("tick: %v", err)
	}
	return n
}

func wantCode(t *testing.T, err error, code string) {
	t.Helper()
	if err == nil {
		t.Fatalf("want error %s, got nil", code)
	}
	if got := err.Error(); !errors.Is(err, err) || len(got) < len(code) || !contains(got, code) {
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

func ptr[T any](v T) *T { return &v }

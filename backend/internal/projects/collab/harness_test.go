package collab

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
	"github.com/jaas/jaas/internal/projects/project"
	"github.com/jaas/jaas/internal/projects/testdb"
	"github.com/jaas/jaas/internal/shared/identityport"
	"github.com/jaas/jaas/internal/shared/storage"
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

// fakeAudit records every audit call with its metadata serialised, so tests can prove it holds ids only (PG12).
type fakeAudit struct {
	mu      sync.Mutex
	actions []string
	meta    []string
}

func (f *fakeAudit) Log(_ context.Context, _ *gorm.DB, _, _ string, action, _, _ string, meta interface{}, _, _ string) error {
	raw, _ := json.Marshal(meta)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.actions = append(f.actions, action)
	f.meta = append(f.meta, string(raw))
	return nil
}

// testResolver is the test-side api.TaskResolver: it reads the tasks table by SQL and builds the Access with the real
// project Authorizer, as the task context's resolver will.
type testResolver struct{ auth api.Authorizer }

func (r testResolver) Task(ctx context.Context, db *gorm.DB, a api.Actor, id uuid.UUID) (api.TaskRef, api.Access, error) {
	var ref api.TaskRef
	row := db.WithContext(ctx).Raw(`SELECT id, project_id, number, title, column_id, version FROM tasks
		WHERE id = ? AND tenant_id = ? AND deleted_at IS NULL`, id, a.TenantID).Row()
	if err := row.Scan(&ref.ID, &ref.ProjectID, &ref.Number, &ref.Title, &ref.ColumnID, &ref.Version); err != nil {
		return ref, api.Access{}, kernel.NotFound("task")
	}
	acc, err := r.auth.Access(ctx, db, a, ref.ProjectID)
	return ref, acc, err
}

type env struct {
	t      *testing.T
	db     *gorm.DB
	proj   *project.Service
	svc    *Service
	mem    *storage.Memory
	audit  *fakeAudit
	users  fakeUsers
	tenant uuid.UUID
}

func newEnv(t *testing.T) *env {
	t.Helper()
	db := testdb.Open(t)
	e := &env{t: t, db: db, mem: storage.NewMemory(), audit: &fakeAudit{}, users: fakeUsers{active: map[uuid.UUID]bool{}}, tenant: uuid.New()}
	e.proj = project.NewService(db, project.Ports{Users: e.users, Audit: &fakeAudit{}})
	e.svc = NewService(db, Ports{Authorizer: e.proj, Tasks: testResolver{auth: e.proj}, Storage: e.mem, Users: e.users, Audit: e.audit})
	return e
}

// actor registers an active login; flags: create, readall, admin.
func (e *env) actor(flags ...string) api.Actor {
	id := uuid.New()
	e.users.active[id] = true
	a := api.Actor{TenantID: e.tenant, UserID: id, CreateProject: true}
	for _, f := range flags {
		switch f {
		case "readall":
			a.ReadAll = true
		case "admin":
			a.ManageAll, a.ReadAll = true, true
		}
	}
	return a
}

// project creates a project owned by owner.
func (e *env) project(owner api.Actor, key string, visibility string) *project.Project {
	e.t.Helper()
	p, err := e.proj.Create(bg, owner, project.CreateInput{Key: key, Name: "Project " + key, Visibility: visibility})
	if err != nil {
		e.t.Fatalf("create project %s: %v", key, err)
	}
	return p
}

// join adds a member with a built-in role.
func (e *env) join(owner api.Actor, p *project.Project, who api.Actor, role string) {
	e.t.Helper()
	if _, err := e.proj.AddMember(bg, owner, p.ID, project.MemberInput{UserID: who.UserID, Role: role}); err != nil {
		e.t.Fatalf("add member: %v", err)
	}
}

// task inserts a minimal task row (the task context is not part of this package).
func (e *env) task(p *project.Project, estimate *int) uuid.UUID {
	e.t.Helper()
	var col uuid.UUID
	must(e.t, e.db.Raw(`SELECT id FROM board_columns WHERE project_id = ? AND is_default`, p.ID).Row().Scan(&col))
	var n int
	e.db.Raw(`SELECT COALESCE(MAX(number), 0) + 1 FROM tasks WHERE project_id = ?`, p.ID).Scan(&n)
	id := uuid.New()
	err := e.db.Exec(`INSERT INTO tasks (id, tenant_id, project_id, number, title, column_id, reporter_user_id, rank, estimate_minutes)
		VALUES (?, ?, ?, ?, 'T', ?, ?, ?, ?)`, id, p.TenantID, p.ID, n, col, uuid.New(), kernel.RankBetween("", ""), estimate).Error
	if err != nil {
		e.t.Fatal(err)
	}
	return id
}

func (e *env) count(sql string, args ...any) int64 {
	var n int64
	e.db.Raw(sql, args...).Scan(&n)
	return n
}

// events returns the outbox payloads of one event type.
func (e *env) events(key string) []map[string]any {
	var raws []string
	e.db.Raw(`SELECT payload::text FROM projects_events_outbox WHERE routing_key = ? ORDER BY created_at`, key).Scan(&raws)
	out := make([]map[string]any, 0, len(raws))
	for _, r := range raws {
		var env struct{ Payload map[string]any }
		_ = json.Unmarshal([]byte(r), &env)
		out = append(out, env.Payload)
	}
	return out
}

// noLeak fails when any outbox payload, activity row or audit record contains one of the secrets (PG12).
func (e *env) noLeak(secrets ...string) {
	e.t.Helper()
	var blobs []string
	e.db.Raw(`SELECT payload::text FROM projects_events_outbox`).Scan(&blobs)
	var acts []string
	e.db.Raw(`SELECT data::text FROM activity`).Scan(&acts)
	blobs = append(append(blobs, acts...), e.audit.meta...)
	for _, b := range blobs {
		for _, s := range secrets {
			if strings.Contains(b, s) {
				e.t.Fatalf("%q leaked into %s", s, b)
			}
		}
	}
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

func wantStatus(t *testing.T, err error, status string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), "status: "+status) {
		t.Fatalf("want status %s, got %v", status, err)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

var errBoom = errors.New("boom")

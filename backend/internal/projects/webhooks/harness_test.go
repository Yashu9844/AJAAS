package webhooks

import (
	"context"
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

type auditEntry struct{ action, userID, meta string }

type fakeAudit struct {
	mu      sync.Mutex
	entries []auditEntry
}

func (f *fakeAudit) Log(_ context.Context, _ *gorm.DB, _, userID string, action, _, _ string, meta interface{}, _, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.entries = append(f.entries, auditEntry{action: action, userID: userID, meta: toJSON(meta)})
	return nil
}

func (f *fakeAudit) has(action string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, e := range f.entries {
		if e.action == action {
			return true
		}
	}
	return false
}

// fakeDoer records outgoing requests and answers from a queue (then with def).
type fakeDoer struct {
	mu   sync.Mutex
	reqs []Request
	next []Result
	def  Result
}

func (f *fakeDoer) Do(_ context.Context, r Request) Result {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reqs = append(f.reqs, r)
	if len(f.next) > 0 {
		res := f.next[0]
		f.next = f.next[1:]
		return res
	}
	return f.def
}

func (f *fakeDoer) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.reqs)
}

func (f *fakeDoer) last() Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.reqs[len(f.reqs)-1]
}

// testResolver is the test-side api.TaskResolver (the task context provides the real one).
type testResolver struct{ auth api.Authorizer }

func (r testResolver) Task(ctx context.Context, db *gorm.DB, a api.Actor, id uuid.UUID) (api.TaskRef, api.Access, error) {
	var rows []struct {
		ProjectID string
		Number    int
	}
	err := db.WithContext(ctx).Raw(`SELECT project_id::text AS project_id, number FROM tasks WHERE id = ? AND tenant_id = ? AND deleted_at IS NULL`, id, a.TenantID).Scan(&rows).Error
	if err != nil {
		return api.TaskRef{}, api.Access{}, err
	}
	if len(rows) != 1 {
		return api.TaskRef{}, api.Access{}, kernel.NotFound("task")
	}
	ref := api.TaskRef{ID: id, ProjectID: uuid.MustParse(rows[0].ProjectID), Number: rows[0].Number}
	acc, err := r.auth.Access(ctx, db, a, ref.ProjectID)
	return ref, acc, err
}

type env struct {
	t      *testing.T
	db     *gorm.DB
	proj   *project.Service
	svc    *Service
	audit  *fakeAudit
	doer   *fakeDoer
	users  fakeUsers
	tenant uuid.UUID
	clock  time.Time
}

func newEnv(t *testing.T) *env {
	t.Helper()
	db := testdb.Open(t)
	e := &env{t: t, db: db, audit: &fakeAudit{}, doer: &fakeDoer{def: Result{Status: 200}}, users: fakeUsers{active: map[uuid.UUID]bool{}}, tenant: uuid.New()}
	e.proj = project.NewService(db, project.Ports{Users: e.users, Audit: e.audit})
	svc, err := NewService(db, e.proj, testResolver{e.proj}, Ports{Audit: e.audit, SecretsKey: testKey()})
	if err != nil {
		t.Fatal(err)
	}
	svc.SetSender(e.doer)
	svc.jitter = func() float64 { return 0.5 } // factor 1.0
	e.svc, e.clock = svc, time.Now().UTC()
	svc.now = func() time.Time { return e.clock }
	return e
}

func (e *env) user(admin bool) api.Actor {
	id := uuid.New()
	e.users.active[id] = true
	return api.Actor{TenantID: e.tenant, UserID: id, CreateProject: true, ManageAll: admin}
}

func (e *env) project(owner api.Actor, key string) *project.Project {
	e.t.Helper()
	p, err := e.proj.Create(bg, owner, project.CreateInput{Key: key, Name: "P " + key})
	if err != nil {
		e.t.Fatal(err)
	}
	return p
}

// task inserts task number n into a project.
func (e *env) task(owner api.Actor, p *project.Project, n int) uuid.UUID {
	e.t.Helper()
	var col string
	if err := e.db.Raw(`SELECT id::text FROM board_columns WHERE project_id = ? AND is_default`, p.ID).Scan(&col).Error; err != nil {
		e.t.Fatal(err)
	}
	id := uuid.New()
	if err := e.db.Exec(`INSERT INTO tasks (id, tenant_id, project_id, number, title, column_id, reporter_user_id, rank)
		VALUES (?, ?, ?, ?, 'Secret title', ?, ?, ?)`, id, e.tenant, p.ID, n, uuid.MustParse(col), owner.UserID, "V"+strings.Repeat("a", n)).Error; err != nil {
		e.t.Fatal(err)
	}
	return id
}

// emit writes an outbox event the way every M2 context does.
func (e *env) emit(key string, payload map[string]any) {
	e.t.Helper()
	if err := e.db.Transaction(func(tx *gorm.DB) error { return kernel.Emit(bg, tx, e.tenant, key, payload) }); err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) count(sql string, args ...any) int64 {
	var n int64
	e.db.Raw(sql, args...).Scan(&n)
	return n
}

func (e *env) webhook(owner api.Actor, p *project.Project, in WebhookInput) *WebhookWithSecret {
	e.t.Helper()
	if in.Name == "" {
		in.Name = "hook-" + uuid.NewString()[:6]
	}
	if in.URL == "" {
		in.URL = "https://hooks.example.com/in"
	}
	w, err := e.svc.CreateWebhook(bg, owner, p.ID, in)
	if err != nil {
		e.t.Fatal(err)
	}
	return w
}

func wantCode(t *testing.T, err error, code string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), code) {
		t.Fatalf("want error %s, got %v", code, err)
	}
}

package fields

import (
	"context"
	"testing"

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

// testResolver is the test-side api.TaskResolver (the task context provides the real one).
type testResolver struct{ auth api.Authorizer }

func (r testResolver) Task(ctx context.Context, db *gorm.DB, a api.Actor, id uuid.UUID) (api.TaskRef, api.Access, error) {
	var rows []struct {
		ID, ProjectID, ColumnID string
		Number, Version         int
		Title                   string
	}
	err := db.WithContext(ctx).Raw(`SELECT id::text AS id, project_id::text AS project_id, column_id::text AS column_id, number, title, version
		FROM tasks WHERE id = ? AND tenant_id = ? AND deleted_at IS NULL`, id, a.TenantID).Scan(&rows).Error
	if err != nil {
		return api.TaskRef{}, api.Access{}, err
	}
	if len(rows) != 1 {
		return api.TaskRef{}, api.Access{}, kernel.NotFound("task")
	}
	ref := api.TaskRef{ID: uuid.MustParse(rows[0].ID), ProjectID: uuid.MustParse(rows[0].ProjectID), ColumnID: uuid.MustParse(rows[0].ColumnID),
		Number: rows[0].Number, Title: rows[0].Title, Version: rows[0].Version}
	acc, err := r.auth.Access(ctx, db, a, ref.ProjectID)
	return ref, acc, err
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
	e.svc = NewService(db, e.proj, testResolver{e.proj}, Ports{Audit: e.audit})
	return e
}

func (e *env) user(admin bool) api.Actor {
	id := uuid.New()
	e.users.active[id] = true
	return api.Actor{TenantID: e.tenant, UserID: id, CreateProject: true, ManageAll: admin}
}

// task creates a project (owner a) with one task and returns both.
func (e *env) task(owner api.Actor, key string) (*project.Project, uuid.UUID) {
	e.t.Helper()
	p, err := e.proj.Create(bg, owner, project.CreateInput{Key: key, Name: "P " + key})
	if err != nil {
		e.t.Fatal(err)
	}
	var colText string
	if err := e.db.Raw(`SELECT id::text FROM board_columns WHERE project_id = ? AND is_default`, p.ID).Scan(&colText).Error; err != nil {
		e.t.Fatal(err)
	}
	col := uuid.MustParse(colText)
	id := uuid.New()
	if err := e.db.Exec(`INSERT INTO tasks (id, tenant_id, project_id, number, title, column_id, reporter_user_id, rank)
		VALUES (?, ?, ?, 1, 'T', ?, ?, 'V')`, id, e.tenant, p.ID, col, owner.UserID).Error; err != nil {
		e.t.Fatal(err)
	}
	return p, id
}

func (e *env) count(sql string, args ...any) int64 {
	var n int64
	e.db.Raw(sql, args...).Scan(&n)
	return n
}

func wantCode(t *testing.T, err error, code string) {
	t.Helper()
	if err == nil {
		t.Fatalf("want error %s, got nil", code)
	}
	if got := err.Error(); len(got) < len(code) || !containsStr(got, code) {
		t.Fatalf("want error %s, got %v", code, err)
	}
}

func containsStr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

package automation

import (
	"context"
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

// call is one action the fake task actor performed.
type call struct {
	kind  string
	agent string
	user  uuid.UUID
	task  uuid.UUID
}

// fakeTasks is a minimal task actor on the real tables: it applies the change and writes the same outbox event the task
// context would, carrying the agent label of the context, so the engine can be tested without the task context.
type fakeTasks struct {
	db    *gorm.DB
	mu    sync.Mutex
	calls []call
	fail  map[string]error
}

func (f *fakeTasks) log(ctx context.Context, kind string, a api.Actor, task uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, call{kind, kernel.AgentFrom(ctx), a.UserID, task})
	return f.fail[kind]
}

func (f *fakeTasks) count(kind string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if c.kind == kind {
			n++
		}
	}
	return n
}

func (f *fakeTasks) State(ctx context.Context, a api.Actor, id uuid.UUID) (TaskState, error) {
	var rows []struct {
		ProjectID, ColumnID string
		Category, Priority  string
	}
	err := f.db.WithContext(ctx).Raw(`SELECT t.project_id::text AS project_id, t.column_id::text AS column_id, c.category, t.priority FROM tasks t JOIN board_columns c ON c.id = t.column_id
		WHERE t.id = ? AND t.tenant_id = ? AND t.deleted_at IS NULL`, id, a.TenantID).Scan(&rows).Error
	if err != nil {
		return TaskState{}, err
	}
	if len(rows) == 0 {
		return TaskState{}, kernel.NotFound("task")
	}
	st := TaskState{ID: id, ProjectID: uuid.MustParse(rows[0].ProjectID), ColumnID: uuid.MustParse(rows[0].ColumnID), ColumnCategory: rows[0].Category, Priority: rows[0].Priority}
	if st.LabelIDs, err = f.ids(`SELECT label_id::text FROM task_labels WHERE task_id = ?`, id); err != nil {
		return st, err
	}
	st.AssigneeIDs, err = f.ids(`SELECT user_id::text FROM task_assignees WHERE task_id = ? AND active`, id)
	return st, err
}

func (f *fakeTasks) ids(sql string, args ...any) ([]uuid.UUID, error) {
	var txt []string
	err := f.db.Raw(sql, args...).Scan(&txt).Error
	out := []uuid.UUID{}
	for _, v := range txt {
		out = append(out, uuid.MustParse(v))
	}
	return out, err
}

// change runs one write and its event in a transaction.
func (f *fakeTasks) change(ctx context.Context, a api.Actor, id uuid.UUID, kind string, sql string, args []any, payload map[string]any) error {
	if err := f.log(ctx, kind, a, id); err != nil {
		return err
	}
	return f.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var txt string
		if err := tx.Raw(`SELECT project_id::text FROM tasks WHERE id = ?`, id).Scan(&txt).Error; err != nil {
			return err
		}
		pid := uuid.MustParse(txt)
		if sql != "" {
			if err := tx.Exec(sql, args...).Error; err != nil {
				return err
			}
		}
		payload["task_id"], payload["project_id"] = id, pid
		return kernel.Emit(ctx, tx, a.TenantID, eventFor(kind), payload)
	})
}

func eventFor(kind string) string {
	switch kind {
	case ActionMove:
		return "task.moved"
	case ActionAssign:
		return "task.assigned"
	case ActionComment:
		return "task.commented"
	case ActionDue:
		return "task.due_changed"
	}
	return "task.updated"
}

func (f *fakeTasks) Move(ctx context.Context, a api.Actor, id, col uuid.UUID) error {
	from := uuid.Nil
	ids, err := f.ids(`SELECT column_id::text FROM tasks WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if len(ids) == 1 {
		from = ids[0]
	}
	return f.change(ctx, a, id, ActionMove, `UPDATE tasks SET column_id = ? WHERE id = ?`, []any{col, id},
		map[string]any{"from_column_id": from, "to_column_id": col})
}

func (f *fakeTasks) SetPriority(ctx context.Context, a api.Actor, id uuid.UUID, p string) error {
	return f.change(ctx, a, id, ActionPriority, `UPDATE tasks SET priority = ? WHERE id = ?`, []any{p, id}, map[string]any{})
}

func (f *fakeTasks) AddLabel(ctx context.Context, a api.Actor, id, label uuid.UUID) error {
	return f.change(ctx, a, id, ActionLabel, `INSERT INTO task_labels (task_id, label_id, tenant_id) VALUES (?, ?, ?) ON CONFLICT DO NOTHING`,
		[]any{id, label, a.TenantID}, map[string]any{})
}

func (f *fakeTasks) Assign(ctx context.Context, a api.Actor, id, user uuid.UUID) error {
	return f.change(ctx, a, id, ActionAssign, `INSERT INTO task_assignees (task_id, user_id, tenant_id, assigned_by) VALUES (?, ?, ?, ?) ON CONFLICT DO NOTHING`,
		[]any{id, user, a.TenantID, a.UserID}, map[string]any{"user_ids": []uuid.UUID{user}})
}

func (f *fakeTasks) SetDue(ctx context.Context, a api.Actor, id uuid.UUID, due time.Time) error {
	return f.change(ctx, a, id, ActionDue, `UPDATE tasks SET due_at = ? WHERE id = ?`, []any{due, id}, map[string]any{})
}

func (f *fakeTasks) Comment(ctx context.Context, a api.Actor, id uuid.UUID, _ string) error {
	return f.change(ctx, a, id, ActionComment, "", nil, map[string]any{})
}

type env struct {
	t      *testing.T
	db     *gorm.DB
	proj   *project.Service
	tasks  *fakeTasks
	svc    *Service
	eng    *Engine
	audit  *fakeAudit
	users  fakeUsers
	tenant uuid.UUID
	n      int
}

func newEnv(t *testing.T) *env {
	t.Helper()
	db := testdb.Open(t)
	e := &env{t: t, db: db, audit: &fakeAudit{}, users: fakeUsers{active: map[uuid.UUID]bool{}}, tenant: uuid.New()}
	e.proj = project.NewService(db, project.Ports{Users: e.users, Audit: e.audit})
	e.tasks = &fakeTasks{db: db, fail: map[string]error{}}
	e.svc = NewService(db, e.proj, e.tasks, Ports{Audit: e.audit})
	e.eng = NewEngine(e.svc)
	e.eng.settle = 0
	return e
}

func (e *env) user(admin bool) api.Actor {
	id := uuid.New()
	e.users.active[id] = true
	return api.Actor{TenantID: e.tenant, UserID: id, CreateProject: true, ManageAll: admin}
}

// fixture is a project with an owner, a member and a viewer.
type fixture struct {
	p                    *project.Project
	owner, member, viewr api.Actor
}

func (e *env) fixture(key string) fixture {
	e.t.Helper()
	owner := e.user(false)
	p, err := e.proj.Create(bg, owner, project.CreateInput{Key: key, Name: "P " + key})
	if err != nil {
		e.t.Fatal(err)
	}
	f := fixture{p: p, owner: owner, member: e.user(false), viewr: e.user(false)}
	for a, role := range map[*api.Actor]string{&f.member: api.RoleMember, &f.viewr: api.RoleViewer} {
		if _, err := e.proj.AddMember(bg, owner, p.ID, project.MemberInput{UserID: a.UserID, Role: role}); err != nil {
			e.t.Fatal(err)
		}
	}
	return f
}

func (e *env) col(p *project.Project, key string) uuid.UUID {
	var ids []uuid.UUID
	if err := e.db.Raw(`SELECT id FROM board_columns WHERE project_id = ? AND key = ?`, p.ID, key).Scan(&ids).Error; err != nil || len(ids) == 0 {
		e.t.Fatalf("column %s: %v", key, err)
	}
	return ids[0]
}

// task inserts a task in the named column.
func (e *env) task(p *project.Project, colKey, priority string, reporter uuid.UUID) uuid.UUID {
	e.t.Helper()
	e.n++
	id := uuid.New()
	err := e.db.Exec(`INSERT INTO tasks (id, tenant_id, project_id, number, title, column_id, priority, reporter_user_id, rank)
		VALUES (?, ?, ?, ?, 'T', ?, ?, ?, 'V')`, id, e.tenant, p.ID, e.n, e.col(p, colKey), priority, reporter).Error
	if err != nil {
		e.t.Fatal(err)
	}
	return id
}

// emit writes a task event as a person would (no agent label).
func (e *env) emit(key string, taskID, projectID uuid.UUID, extra map[string]any) {
	e.t.Helper()
	payload := map[string]any{"task_id": taskID, "project_id": projectID}
	for k, v := range extra {
		payload[k] = v
	}
	if err := e.db.Transaction(func(tx *gorm.DB) error { return kernel.Emit(bg, tx, e.tenant, key, payload) }); err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) tick() int {
	e.t.Helper()
	n, err := e.eng.Tick(bg)
	if err != nil {
		e.t.Fatalf("tick: %v", err)
	}
	return n
}

func (e *env) rule(owner api.Actor, projectID uuid.UUID, in RuleInput) *Rule {
	e.t.Helper()
	r, err := e.svc.CreateRule(bg, owner, projectID, in)
	if err != nil {
		e.t.Fatalf("create rule: %v", err)
	}
	return r
}

func (e *env) count(sql string, args ...any) int64 {
	var n int64
	e.db.Raw(sql, args...).Scan(&n)
	return n
}

func (e *env) runs(ruleID uuid.UUID, status string) int64 {
	return e.count(`SELECT count(*) FROM workflow_rule_runs WHERE rule_id = ? AND status = ?`, ruleID, status)
}

func wantCode(t *testing.T, err error, code string) {
	t.Helper()
	if err == nil {
		t.Fatalf("want error %s, got nil", code)
	}
	if got := err.Error(); len(got) < len(code) || !contains(got, code) {
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

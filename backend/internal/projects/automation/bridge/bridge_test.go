package bridge

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/automation"
	"github.com/jaas/jaas/internal/projects/collab"
	"github.com/jaas/jaas/internal/projects/project"
	"github.com/jaas/jaas/internal/projects/recurrence"
	"github.com/jaas/jaas/internal/projects/task"
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

type fakeAudit struct{}

func (fakeAudit) Log(context.Context, *gorm.DB, string, string, string, string, string, interface{}, string, string) error {
	return nil
}

// world is the real project, task and collaboration services wired to the automation suite.
type world struct {
	t      *testing.T
	db     *gorm.DB
	proj   *project.Service
	task   *task.Service
	suite  *Suite
	tenant uuid.UUID
	owner  api.Actor
	p      *project.Project
}

func newWorld(t *testing.T) *world {
	t.Helper()
	db := testdb.Open(t)
	w := &world{t: t, db: db, tenant: uuid.New()}
	users := fakeUsers{active: map[uuid.UUID]bool{}}
	w.proj = project.NewService(db, project.Ports{Users: users, Audit: fakeAudit{}})
	w.task = task.NewService(db, w.proj, task.Ports{Audit: fakeAudit{}})
	cl := collab.NewService(db, collab.Ports{Authorizer: w.proj, Tasks: w.task, Users: users, Audit: fakeAudit{}})
	w.task.SetHooks(cl.TaskHooks())
	w.suite = NewSuite(Deps{DB: db, Authz: w.proj, Tasks: w.task, Collab: cl, Audit: fakeAudit{}})
	w.suite.Engine.SetSettle(0)
	id := uuid.New()
	users.active[id] = true
	w.owner = api.Actor{TenantID: w.tenant, UserID: id, CreateProject: true}
	p, err := w.proj.Create(bg, w.owner, project.CreateInput{Key: "BR", Name: "Bridge"})
	if err != nil {
		t.Fatal(err)
	}
	w.p = p
	return w
}

func (w *world) col(key string) uuid.UUID {
	var txt string
	if err := w.db.Raw(`SELECT id::text FROM board_columns WHERE project_id = ? AND key = ?`, w.p.ID, key).Scan(&txt).Error; err != nil || txt == "" {
		w.t.Fatalf("column %s: %v", key, err)
	}
	return uuid.MustParse(txt)
}

func (w *world) count(sql string, args ...any) int64 {
	var n int64
	w.db.Raw(sql, args...).Scan(&n)
	return n
}

func TestRulesActThroughTheRealTaskService(t *testing.T) {
	w := newWorld(t)
	done, todo := w.col("done"), w.col("todo")
	label, err := w.proj.CreateLabel(bg, w.owner, &w.p.ID, project.LabelInput{Name: "auto"})
	if err != nil {
		t.Fatal(err)
	}
	rule, err := w.suite.Rules.CreateRule(bg, w.owner, w.p.ID, automation.RuleInput{Name: "Urgent to done", Trigger: "task.moved",
		Conditions: automation.Conditions{{Field: automation.FieldPriority, Value: "urgent"}},
		Actions: automation.Actions{{Type: automation.ActionMove, ColumnID: &done}, {Type: automation.ActionLabel, LabelID: &label.ID},
			{Type: automation.ActionAssign, UserID: &w.owner.UserID}, {Type: automation.ActionComment, Text: "Closed by automation"},
			{Type: automation.ActionDue, Days: ptr(0)}}})
	if err != nil {
		t.Fatal(err)
	}
	v, err := w.task.Create(bg, w.owner, w.p.ID, task.CreateInput{Title: "Ship it", Priority: "urgent"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = w.task.Move(bg, w.owner, v.ID, task.MoveInput{ColumnID: &todo}); err != nil { // a person moves the task
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if _, err = w.suite.Engine.Tick(bg); err != nil {
			t.Fatal(err)
		}
	}
	got, err := w.task.Get(bg, w.owner, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ColumnCategory != "done" || got.CompletedAt == nil || len(got.LabelIDs) != 1 || len(got.AssigneeIDs) != 1 || got.DueAt == nil {
		t.Fatalf("the rule acted through the real use cases: %+v", got)
	}
	agent := "automation." + rule.ID.String()[:8]
	if n := w.count(`SELECT count(*) FROM activity WHERE task_id = ? AND source = 'ai' AND data->>'agent' = ?`, v.ID, agent); n < 4 {
		t.Fatalf("activity carries the automation label, got %d entries", n)
	}
	if n := w.count(`SELECT count(*) FROM comments WHERE task_id = ? AND body = 'Closed by automation'`, v.ID); n != 1 {
		t.Fatalf("comment authored once: %d", n)
	}
	if n := w.count(`SELECT count(*) FROM projects_events_outbox WHERE routing_key = 'task.moved' AND payload->>'agent' = ?`, agent); n != 1 {
		t.Fatalf("the rule's move event carries the agent label: %d", n)
	}
	if n := w.count(`SELECT count(*) FROM workflow_rule_runs WHERE rule_id = ? AND status = 'ok'`, rule.ID); n != 1 {
		t.Fatalf("one run, no cascade: %d", n)
	}
}

func TestRuleHonoursWipLimitsLikeAPerson(t *testing.T) {
	w := newWorld(t)
	prog, todo := w.col("in-progress"), w.col("todo")
	hard, limit := true, 1
	if _, err := w.proj.UpdateColumn(bg, w.owner, w.p.ID, prog, project.ColumnInput{WipLimit: &limit, WipHard: &hard}); err != nil {
		t.Fatal(err)
	}
	if _, err := w.task.Create(bg, w.owner, w.p.ID, task.CreateInput{Title: "Occupant", ColumnID: &prog}); err != nil {
		t.Fatal(err)
	}
	rule, err := w.suite.Rules.CreateRule(bg, w.owner, w.p.ID, automation.RuleInput{Name: "Start", Trigger: "task.created",
		Actions: automation.Actions{{Type: automation.ActionMove, ColumnID: &prog}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = w.task.Create(bg, w.owner, w.p.ID, task.CreateInput{Title: "Waiting", ColumnID: &todo}); err != nil {
		t.Fatal(err)
	}
	if _, err = w.suite.Engine.Tick(bg); err != nil {
		t.Fatal(err)
	}
	var code string
	w.db.Raw(`SELECT coalesce(error_code, '') FROM workflow_rule_runs WHERE rule_id = ? AND status = 'failed' AND error_code <> 'INTERRUPTED'`, rule.ID).Scan(&code)
	if code != "WIP_LIMIT" {
		t.Fatalf("the hard WIP limit stops the rule like it stops a person, got %q", code)
	}
}

func TestRecurrenceSpawnsThroughTheRealTaskService(t *testing.T) {
	w := newWorld(t)
	label, _ := w.proj.CreateLabel(bg, w.owner, &w.p.ID, project.LabelInput{Name: "weekly"})
	backlog := w.col("backlog")
	due := time.Now().UTC().AddDate(0, 0, 1)
	tpl, err := w.task.Create(bg, w.owner, w.p.ID, task.CreateInput{Title: "Weekly report", ColumnID: &backlog, DueAt: &due,
		AssigneeIDs: []uuid.UUID{w.owner.UserID}, LabelIDs: []uuid.UUID{label.ID}})
	if err != nil {
		t.Fatal(err)
	}
	rec, err := w.suite.Recurrence.Create(bg, w.owner, w.p.ID, recurrence.Input{TemplateTaskID: tpl.ID, Frequency: recurrence.Weekly})
	if err != nil {
		t.Fatal(err)
	}
	id, err := w.suite.Recurrence.RunNow(bg, w.owner, w.p.ID, rec.ID)
	if err != nil || id == uuid.Nil {
		t.Fatalf("run now: %v", err)
	}
	cp, err := w.task.Get(bg, w.owner, id)
	if err != nil {
		t.Fatal(err)
	}
	if cp.Title != "Weekly report" || strings.Contains(cp.Title, "(copy)") || len(cp.LabelIDs) != 1 || len(cp.AssigneeIDs) != 1 || cp.DueAt == nil || cp.Number == tpl.Number {
		t.Fatalf("copy: %+v", cp)
	}
	if n := w.count(`SELECT count(*) FROM activity WHERE task_id = ? AND data->>'agent' = ?`, id, "recurrence."+rec.ID.String()[:8]); n == 0 {
		t.Fatal("the copy is labelled with the recurrence agent")
	}
	// the scheduler path creates one more copy and moves the schedule
	w.db.Exec(`UPDATE task_recurrences SET next_run_at = now() - interval '1 minute' WHERE id = ?`, rec.ID)
	if n, err := w.suite.Recurrence.Tick(bg); err != nil || n != 1 {
		t.Fatalf("tick: %d %v", n, err)
	}
	if n := w.count(`SELECT count(*) FROM tasks WHERE project_id = ?`, w.p.ID); n != 3 {
		t.Fatalf("template plus two copies, got %d", n)
	}
}

func TestSuiteRegistersRoutes(t *testing.T) {
	w := newWorld(t)
	ctx, cancel := context.WithCancel(bg)
	w.suite.Run(ctx, func(string, error) {})
	cancel()
	w.suite.SetPublicLimiter(func(*gin.Context) {})
	r := gin.New()
	w.suite.Register(r.Group("/api/v1"))
	w.suite.RegisterPublic(r.Group("/api/v1"))
	if n := len(r.Routes()); n != 15 {
		t.Fatalf("14 authenticated routes and 1 public route, got %d", n)
	}
}

func ptr[T any](v T) *T { return &v }

func TestBridgeEntryPointsAndErrors(t *testing.T) {
	w := newWorld(t)
	b := New(w.task, nil)
	v, err := w.task.Create(bg, w.owner, w.p.ID, task.CreateInput{Title: "Plain"})
	if err != nil {
		t.Fatal(err)
	}
	if err = b.SetPriority(bg, w.owner, v.ID, "high"); err != nil {
		t.Fatal(err)
	}
	st, err := b.State(bg, w.owner, v.ID)
	if err != nil || st.Priority != "high" {
		t.Fatalf("state: %v %+v", err, st)
	}
	missing := uuid.New()
	if _, err = b.State(bg, w.owner, missing); err == nil {
		t.Fatal("unknown task")
	}
	if err = b.SetPriority(bg, w.owner, missing, "low"); err == nil {
		t.Fatal("unknown task")
	}
	if err = b.SetPriority(bg, w.owner, v.ID, "bogus"); err == nil {
		t.Fatal("invalid priority is refused by the task use case")
	}
	if _, err = b.Template(bg, w.owner, missing); err == nil {
		t.Fatal("unknown template")
	}
	if _, err = b.Spawn(bg, w.owner, missing, recurrence.Dates{}); err == nil {
		t.Fatal("unknown template")
	}
	viewer := api.Actor{TenantID: w.tenant, UserID: uuid.New()}
	if _, err = b.Spawn(bg, viewer, v.ID, recurrence.Dates{}); err == nil {
		t.Fatal("a stranger cannot copy")
	}
}

package task

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/kernel"
)

func TestCreateAndGet(t *testing.T) {
	e := newEnv(t)
	owner := e.user()
	p := e.project(owner, "ALPHA")
	v := e.mk(owner, p, "First task")
	if v.Number != 1 || v.Key != "ALPHA-1" || v.Version != 1 || v.ColumnCategory != "todo" || v.Priority != "none" {
		t.Fatalf("created: %+v", v)
	}
	v2 := e.mk(owner, p, "Second")
	if v2.Number != 2 || v2.Rank <= v.Rank {
		t.Fatalf("second: %+v vs %+v", v2, v)
	}
	got, err := e.svc.Get(bg, owner, v.ID)
	if err != nil || got.Title != "First task" || got.ReporterUserID != owner.UserID {
		t.Fatalf("get: %v %v", got, err)
	}
	if _, err := e.svc.Get(bg, owner, uuid.New()); code(err) != "NOT_FOUND" {
		t.Fatalf("unknown id: %v", err)
	}
	ref, acc, err := e.svc.Task(bg, nil, owner, v.ID)
	if err != nil || ref.Number != 1 || ref.ProjectID != p.ID || !acc.Can("task.update") {
		t.Fatalf("resolver: %+v %v", ref, err)
	}
	if _, _, err := e.svc.Task(bg, e.db, e.user(), v.ID); code(err) != "NOT_FOUND" {
		t.Fatalf("resolver hides foreign tasks: %v", err)
	}
}

func TestCreateValidation(t *testing.T) {
	e := newEnv(t)
	owner := e.user()
	p := e.project(owner, "VAL")
	past := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	start, _ := kernel.ParseDate("2030-05-10")
	neg := -1
	cases := map[string]CreateInput{
		"empty title":       {Title: "   "},
		"long title":        {Title: strings.Repeat("x", 301)},
		"long description":  {Title: "t", Description: strings.Repeat("d", 50001)},
		"bad priority":      {Title: "t", Priority: "huge"},
		"negative est":      {Title: "t", EstimateMinutes: &neg},
		"due before start":  {Title: "t", StartOn: &start, DueAt: &past},
		"unknown column":    {Title: "t", ColumnID: ptrID(uuid.New())},
		"unknown milestone": {Title: "t", MilestoneID: ptrID(uuid.New())},
		"unknown sprint":    {Title: "t", SprintID: ptrID(uuid.New())},
		"unknown label":     {Title: "t", LabelIDs: []uuid.UUID{uuid.New()}},
		"nil label id":      {Title: "t", LabelIDs: []uuid.UUID{uuid.Nil}},
		"stranger assignee": {Title: "t", AssigneeIDs: []uuid.UUID{uuid.New()}},
		"unknown parent":    {Title: "t", ParentTaskID: ptrID(uuid.New())},
	}
	for name, in := range cases {
		if _, err := e.svc.Create(bg, owner, p.ID, in); code(err) != "VALIDATION_ERROR" {
			t.Errorf("%s: want VALIDATION_ERROR, got %v", name, err)
		}
	}
	long := make([]uuid.UUID, 21)
	for i := range long {
		long[i] = uuid.New()
	}
	if _, err := e.svc.Create(bg, owner, p.ID, CreateInput{Title: "t", LabelIDs: long}); code(err) != "VALIDATION_ERROR" {
		t.Errorf("too many labels: %v", err)
	}
	if e.count(`SELECT task_counter FROM projects WHERE id = ?`, p.ID) != 0 {
		t.Fatal("failed creates must not consume ticket numbers")
	}
}

func ptrID(id uuid.UUID) *uuid.UUID { return &id }

func TestCreatePermissionsAndTenancy(t *testing.T) {
	e := newEnv(t)
	owner := e.user()
	p := e.project(owner, "PRM")
	viewer, member := e.member(owner, p, "viewer"), e.member(owner, p, "member")
	if _, err := e.svc.Create(bg, viewer, p.ID, CreateInput{Title: "x"}); code(err) != "FORBIDDEN" {
		t.Fatalf("viewer: %v", err)
	}
	if _, err := e.svc.Create(bg, e.user(), p.ID, CreateInput{Title: "x"}); code(err) != "NOT_FOUND" {
		t.Fatalf("stranger gets 404: %v", err)
	}
	foreign := e.user()
	foreign.TenantID = uuid.New()
	if _, err := e.svc.Create(bg, foreign, p.ID, CreateInput{Title: "x"}); code(err) != "NOT_FOUND" {
		t.Fatalf("foreign tenant gets 404: %v", err)
	}
	v := e.mk(member, p, "ok")
	if _, err := e.svc.Get(bg, foreign, v.ID); code(err) != "NOT_FOUND" {
		t.Fatalf("foreign get: %v", err)
	}
	if _, err := e.proj.Archive(bg, owner, p.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Create(bg, member, p.ID, CreateInput{Title: "late"}); code(err) != "PROJECT_ARCHIVED" {
		t.Fatalf("archived project: %v", err)
	}
	if _, err := e.svc.Get(bg, member, v.ID); err != nil {
		t.Fatalf("archived projects stay readable: %v", err)
	}
}

func TestCreateWithRelations(t *testing.T) {
	e := newEnv(t)
	owner := e.user()
	p, q := e.project(owner, "REL"), e.project(owner, "OTH")
	member := e.member(owner, p, "member")
	label, foreignLabel, ms := e.label(owner, p, "bug"), e.label(owner, q, "other"), e.milestone(owner, p, "M1")
	done := e.col(p, "done")
	v := e.mk(owner, p, "full", func(in *CreateInput) {
		in.Priority, in.Description = "high", "details"
		in.AssigneeIDs = []uuid.UUID{member.UserID, member.UserID}
		in.LabelIDs = []uuid.UUID{label.ID}
		in.MilestoneID, in.ColumnID = &ms.ID, &done
	})
	if len(v.AssigneeIDs) != 1 || len(v.LabelIDs) != 1 || v.CompletedAt == nil || v.MilestoneID == nil || v.ColumnCategory != "done" {
		t.Fatalf("view: %+v", v)
	}
	if _, err := e.svc.Create(bg, owner, p.ID, CreateInput{Title: "x", LabelIDs: []uuid.UUID{foreignLabel.ID}}); code(err) != "VALIDATION_ERROR" {
		t.Fatalf("label of another project: %v", err)
	}
	child := e.mk(owner, p, "child", func(in *CreateInput) { in.ParentTaskID = &v.ID })
	if child.Depth != 1 || child.ParentTaskID == nil {
		t.Fatalf("child: %+v", child)
	}
	if !has(e.activityTypes(v.ID), "created") || !has(e.activityTypes(v.ID), "assigned") {
		t.Fatalf("activity: %v", e.activityTypes(v.ID))
	}
}

func TestSubtaskDepthLimit(t *testing.T) {
	e := newEnv(t)
	owner := e.user()
	p := e.project(owner, "DEP")
	cur := e.mk(owner, p, "L0")
	for i := 1; i <= 3; i++ {
		parent := cur.ID
		cur = e.mk(owner, p, "L", func(in *CreateInput) { in.ParentTaskID = &parent })
		if cur.Depth != i {
			t.Fatalf("depth %d != %d", cur.Depth, i)
		}
	}
	if _, err := e.svc.Create(bg, owner, p.ID, CreateInput{Title: "L4", ParentTaskID: &cur.ID}); code(err) != "SUBTASK_DEPTH" {
		t.Fatalf("depth 4 must be refused: %v", err)
	}
	other := e.project(owner, "DE2")
	if _, err := e.svc.Create(bg, owner, other.ID, CreateInput{Title: "x", ParentTaskID: &cur.ID}); code(err) != "VALIDATION_ERROR" {
		t.Fatalf("parent in another project: %v", err)
	}
}

// TestTicketNumbersGapless is PG5: concurrent creates get unique, gapless numbers.
func TestTicketNumbersGapless(t *testing.T) {
	e := newEnv(t)
	owner := e.user()
	p := e.project(owner, "NUM")
	const n = 40
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := e.svc.Create(bg, owner, p.ID, CreateInput{Title: "c"})
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if got := e.count(`SELECT count(DISTINCT number) FROM tasks WHERE project_id = ?`, p.ID); got != n {
		t.Fatalf("distinct numbers = %d", got)
	}
	if got := e.count(`SELECT max(number) - min(number) + 1 FROM tasks WHERE project_id = ?`, p.ID); got != n {
		t.Fatalf("numbers are not gapless: span %d", got)
	}
	if got := e.count(`SELECT task_counter FROM projects WHERE id = ?`, p.ID); got != n {
		t.Fatalf("counter = %d", got)
	}
}

// TestEventsAndAuditCarryIDsOnly is PG12: no title, description or name reaches the outbox, the audit or the activity.
func TestEventsAndAuditCarryIDsOnly(t *testing.T) {
	e := newEnv(t)
	owner := e.user()
	p := e.project(owner, "PII")
	v := e.mk(owner, p, "SECRET-TITLE", func(in *CreateInput) { in.Description = "SECRET-DESC" })
	if _, err := e.svc.Update(bg, owner, v.ID, UpdateInput{Version: num(1), Title: str("SECRET-NEW"), Description: str("SECRET-DESC2")}); err != nil {
		t.Fatal(err)
	}
	if n := e.count(`SELECT count(*) FROM projects_events_outbox WHERE payload::text LIKE '%SECRET%'`); n != 0 {
		t.Fatalf("%d events contain free text", n)
	}
	if n := e.count(`SELECT count(*) FROM activity WHERE data::text LIKE '%SECRET%'`); n != 0 {
		t.Fatalf("%d activity entries contain free text", n)
	}
	for _, m := range e.audit.metas {
		if strings.Contains(m, "SECRET") {
			t.Fatalf("audit meta leaks text: %s", m)
		}
	}
	if !e.audit.has("task.created") || !e.audit.has("task.updated") {
		t.Fatalf("audit actions: %v", e.audit.actions)
	}
	created := e.outbox("task.created")
	if len(created) != 1 || !strings.Contains(created[0], v.ID.String()) || !strings.Contains(created[0], `"version": 1`) {
		t.Fatalf("created event: %v", created)
	}
}

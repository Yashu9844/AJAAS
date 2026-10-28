package planning

import (
	"testing"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/kernel"
)

// SP-01: create, read, edit and delete with every 403/404/409 path.
func TestSprintCrudAndAccess(t *testing.T) {
	e := newEnv(t)
	owner, member, viewer, stranger := e.user("create"), e.user(), e.user(), e.user()
	p := e.scrum(owner, "SPR")
	e.join(owner, p, member, "member")
	e.join(owner, p, viewer, "viewer")

	for name, in := range map[string]SprintInput{
		"no name": {}, "blank name": {Name: ptr("  ")}, "long goal": {Name: ptr("S"), Goal: ptr(string(make([]byte, 1001)))},
		"dates": {Name: ptr("S"), StartOn: date("2026-02-02"), EndOn: date("2026-02-01")},
	} {
		if _, err := e.svc.CreateSprint(bg, owner, p.ID, in); err == nil {
			t.Errorf("%s: want validation error", name)
		}
	}
	_, err := e.svc.CreateSprint(bg, member, p.ID, SprintInput{Name: ptr("M")})
	wantCode(t, err, "FORBIDDEN")
	_, err = e.svc.CreateSprint(bg, stranger, p.ID, SprintInput{Name: ptr("M")})
	wantCode(t, err, "NOT_FOUND")

	sp := e.sprint(owner, p, "Sprint 1", "2026-03-02", "2026-03-13")
	if sp.Status != SprintPlanned || sp.Version != 1 || sp.ProjectID != p.ID {
		t.Fatalf("sprint: %+v", sp)
	}
	_, err = e.svc.CreateSprint(bg, owner, p.ID, SprintInput{Name: ptr("sprint 1")})
	wantCode(t, err, "SPRINT_NAME_TAKEN")

	got, err := e.svc.GetSprint(bg, viewer, sp.ID)
	if err != nil || got.ID != sp.ID {
		t.Fatalf("viewer reads: %v", err)
	}
	_, err = e.svc.GetSprint(bg, stranger, sp.ID)
	wantCode(t, err, "NOT_FOUND")
	_, err = e.svc.GetSprint(bg, owner, uuid.New())
	wantCode(t, err, "NOT_FOUND")

	_, err = e.svc.UpdateSprint(bg, member, sp.ID, SprintInput{Goal: ptr("x")})
	wantCode(t, err, "FORBIDDEN")
	up, err := e.svc.UpdateSprint(bg, owner, sp.ID, SprintInput{Goal: ptr("Ship it"), Version: ptr(1)})
	if err != nil || up.Goal != "Ship it" || up.Version != 2 {
		t.Fatalf("update: %+v %v", up, err)
	}
	_, err = e.svc.UpdateSprint(bg, owner, sp.ID, SprintInput{Goal: ptr("late"), Version: ptr(1)})
	if v, ok := kernel.AsVersionConflict(err); !ok || v.Current != 2 {
		t.Fatalf("want version conflict at 2, got %v", err)
	}
	_, err = e.svc.UpdateSprint(bg, owner, sp.ID, SprintInput{EndOn: date("2026-01-01")})
	if err == nil {
		t.Fatal("end before start must fail")
	}
	e.sprint(owner, p, "Sprint 2", "", "")
	_, err = e.svc.UpdateSprint(bg, owner, sp.ID, SprintInput{Name: ptr("SPRINT 2")})
	wantCode(t, err, "SPRINT_NAME_TAKEN")
}

func TestSprintListPagingAndFilters(t *testing.T) {
	e := newEnv(t)
	owner, stranger := e.user("create"), e.user()
	p := e.scrum(owner, "LST")
	for _, n := range []string{"A", "B", "C"} {
		e.sprint(owner, p, n, "", "")
	}
	task := e.task(p, tk{col: e.col(p, "todo")})
	rows, next, err := e.svc.ListSprints(bg, owner, p.ID, SprintFilter{Limit: 2})
	if err != nil || len(rows) != 2 || next == "" {
		t.Fatalf("first page: %d %q %v", len(rows), next, err)
	}
	rest, next2, err := e.svc.ListSprints(bg, owner, p.ID, SprintFilter{Limit: 2, Cursor: next})
	if err != nil || len(rest) != 1 || next2 != "" || rest[0].ID == rows[0].ID || rest[0].ID == rows[1].ID {
		t.Fatalf("second page: %d %q %v", len(rest), next2, err)
	}
	e.db.Exec(`UPDATE tasks SET sprint_id = ? WHERE id = ?`, rows[0].ID, task)
	again, _, _ := e.svc.ListSprints(bg, owner, p.ID, SprintFilter{Limit: 5, Status: SprintPlanned})
	counted := int64(0)
	for _, r := range again {
		counted += int64(r.TaskCount)
	}
	if len(again) != 3 || counted != 1 {
		t.Fatalf("filter/counts: %d %d", len(again), counted)
	}
	none, _, _ := e.svc.ListSprints(bg, owner, p.ID, SprintFilter{Limit: 5, Status: SprintActive})
	if len(none) != 0 {
		t.Fatal("no active sprints expected")
	}
	if _, _, err = e.svc.ListSprints(bg, owner, p.ID, SprintFilter{Limit: 5, Status: "weird"}); err == nil {
		t.Fatal("bad status must fail")
	}
	if _, _, err = e.svc.ListSprints(bg, owner, p.ID, SprintFilter{Limit: 5, Cursor: "###"}); err == nil {
		t.Fatal("bad cursor must fail")
	}
	_, _, err = e.svc.ListSprints(bg, stranger, p.ID, SprintFilter{Limit: 5})
	wantCode(t, err, "NOT_FOUND")
}

// SP-01: sprints exist in scrum mode only.
func TestKanbanProjectsHaveNoSprints(t *testing.T) {
	e := newEnv(t)
	owner := e.user("create")
	p := e.project(owner, "KAN", "kanban")
	_, err := e.svc.CreateSprint(bg, owner, p.ID, SprintInput{Name: ptr("S")})
	wantCode(t, err, "PROJECT_MODE")
	_, _, err = e.svc.ListSprints(bg, owner, p.ID, SprintFilter{Limit: 5})
	wantCode(t, err, "PROJECT_MODE")
	_, err = e.svc.PlanMove(bg, owner, p.ID, PlanInput{TaskIDs: []uuid.UUID{uuid.New()}})
	wantCode(t, err, "PROJECT_MODE")
	// A sprint left over from a project switched to kanban is unreachable too.
	sc := e.scrum(owner, "OLD")
	sp := e.sprint(owner, sc, "Old", "2026-03-02", "2026-03-13").ID
	e.db.Exec(`UPDATE projects SET mode = 'kanban' WHERE id = ?`, sc.ID)
	for _, fn := range []func() error{
		func() error { _, err := e.svc.GetSprint(bg, owner, sp); return err },
		func() error { _, err := e.svc.StartSprint(bg, owner, sp); return err },
		func() error { _, err := e.svc.SprintReport(bg, owner, sp); return err },
	} {
		wantCode(t, fn(), "PROJECT_MODE")
	}
}

func TestDeleteSprintReleasesTasks(t *testing.T) {
	e := newEnv(t)
	owner, member := e.user("create"), e.user()
	p := e.scrum(owner, "DEL")
	e.join(owner, p, member, "member")
	sp := e.sprint(owner, p, "Gone", "2026-03-02", "2026-03-13")
	a := e.task(p, tk{col: e.col(p, "todo"), sprint: &sp.ID})
	b := e.task(p, tk{col: e.col(p, "todo"), sprint: &sp.ID})
	_, err := e.svc.DeleteSprint(bg, member, sp.ID)
	wantCode(t, err, "FORBIDDEN")
	n, err := e.svc.DeleteSprint(bg, owner, sp.ID)
	if err != nil || n != 2 {
		t.Fatalf("delete: %d %v", n, err)
	}
	if e.sprintOf(a) != nil || e.sprintOf(b) != nil {
		t.Fatal("tasks must return to the backlog")
	}
	if e.count(`SELECT version FROM tasks WHERE id = ?`, a) != 2 {
		t.Fatal("released tasks bump their version")
	}
	_, err = e.svc.GetSprint(bg, owner, sp.ID)
	wantCode(t, err, "NOT_FOUND")
	_, err = e.svc.DeleteSprint(bg, owner, sp.ID)
	wantCode(t, err, "NOT_FOUND")

	active := e.sprint(owner, p, "Live", "2026-03-02", "2026-03-13")
	if _, err = e.svc.StartSprint(bg, owner, active.ID); err != nil {
		t.Fatal(err)
	}
	_, err = e.svc.DeleteSprint(bg, owner, active.ID)
	wantCode(t, err, "SPRINT_STATE")
}

// PR-03: an archived project is frozen for planning writes but readable.
func TestArchivedProjectFreezesPlanning(t *testing.T) {
	e := newEnv(t)
	owner := e.user("create")
	p := e.scrum(owner, "ARC")
	sp := e.sprint(owner, p, "S", "2026-03-02", "2026-03-13")
	if _, err := e.proj.Archive(bg, owner, p.ID); err != nil {
		t.Fatal(err)
	}
	_, err := e.svc.CreateSprint(bg, owner, p.ID, SprintInput{Name: ptr("N")})
	wantCode(t, err, "PROJECT_ARCHIVED")
	_, err = e.svc.StartSprint(bg, owner, sp.ID)
	wantCode(t, err, "PROJECT_ARCHIVED")
	if _, err = e.svc.GetSprint(bg, owner, sp.ID); err != nil {
		t.Fatalf("archived projects stay readable: %v", err)
	}
}

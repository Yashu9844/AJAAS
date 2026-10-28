package planning

import (
	"sync"
	"testing"

	"github.com/google/uuid"
)

func planStatuses(rs []PlanResult) map[uuid.UUID]string {
	out := map[uuid.UUID]string{}
	for _, r := range rs {
		out[r.TaskID] = r.Status
	}
	return out
}

// SP-03: a planning move changes tasks.sprint_id, version and updated_at and nothing else, with a result per task.
func TestPlanMoveWritesOnlySprintColumn(t *testing.T) {
	e := newEnv(t)
	owner := e.user("create")
	p := e.scrum(owner, "PLN")
	sp := e.sprint(owner, p, "S1", "", "")
	todo := e.col(p, "todo")
	a := e.task(p, tk{title: "Alpha", col: todo, est: ptr(30)})
	b := e.task(p, tk{title: "Beta", col: todo})
	type snap struct{ Title, Rank, Priority string }
	var before, after snap
	e.db.Raw(`SELECT title, rank, priority FROM tasks WHERE id = ?`, a).Scan(&before)

	res, err := e.svc.PlanMove(bg, owner, p.ID, PlanInput{TaskIDs: []uuid.UUID{a, b}, SprintID: &sp.ID, Versions: map[string]int{a.String(): 1}})
	if err != nil {
		t.Fatal(err)
	}
	if st := planStatuses(res); st[a] != PlanMoved || st[b] != PlanMoved || res[0].Version != 2 {
		t.Fatalf("results: %+v", res)
	}
	e.db.Raw(`SELECT title, rank, priority FROM tasks WHERE id = ?`, a).Scan(&after)
	if before != after || e.count(`SELECT estimate_minutes FROM tasks WHERE id = ?`, a) != 30 {
		t.Fatalf("only sprint_id may change: %+v -> %+v", before, after)
	}
	if got := e.sprintOf(a); got == nil || *got != sp.ID {
		t.Fatal("task not in the sprint")
	}
	if e.count(`SELECT version FROM tasks WHERE id = ?`, a) != 2 {
		t.Fatal("version must be bumped")
	}
	if e.count(`SELECT count(*) FROM activity WHERE project_id = ? AND type = 'sprint_changed' AND task_id IS NOT NULL`, p.ID) != 2 ||
		e.count(`SELECT count(*) FROM projects_events_outbox WHERE routing_key = 'task.updated'`) != 2 {
		t.Fatal("one activity entry and one event per moved task")
	}

	again, _ := e.svc.PlanMove(bg, owner, p.ID, PlanInput{TaskIDs: []uuid.UUID{a}, SprintID: &sp.ID})
	if again[0].Status != PlanUnchanged || e.count(`SELECT version FROM tasks WHERE id = ?`, a) != 2 {
		t.Fatalf("a repeated move is a no-op: %+v", again)
	}
	back, err := e.svc.PlanMove(bg, owner, p.ID, PlanInput{TaskIDs: []uuid.UUID{a, b}})
	if err != nil || planStatuses(back)[a] != PlanMoved || e.sprintOf(a) != nil || e.sprintOf(b) != nil {
		t.Fatalf("back to the backlog: %+v %v", back, err)
	}
}

func TestPlanMovePerTaskOutcomes(t *testing.T) {
	e := newEnv(t)
	owner := e.user("create")
	p := e.scrum(owner, "OUT")
	other := e.scrum(owner, "OUT2")
	sp := e.sprint(owner, p, "S1", "", "")
	todo := e.col(p, "todo")
	fresh, stale := e.task(p, tk{col: todo}), e.task(p, tk{col: todo})
	archived := e.task(p, tk{col: todo, archived: true})
	deleted := e.task(p, tk{col: todo, deleted: true})
	foreign := e.task(other, tk{col: e.col(other, "todo")})
	e.db.Exec(`UPDATE tasks SET version = 5 WHERE id = ?`, stale)

	res, err := e.svc.PlanMove(bg, owner, p.ID, PlanInput{
		TaskIDs:  []uuid.UUID{fresh, stale, archived, deleted, foreign, uuid.New()},
		SprintID: &sp.ID, Versions: map[string]int{stale.String(): 4}})
	if err != nil {
		t.Fatal(err)
	}
	st := planStatuses(res)
	if st[fresh] != PlanMoved || st[stale] != PlanConflict || st[archived] != PlanArchived || st[deleted] != PlanNotFound || st[foreign] != PlanNotFound {
		t.Fatalf("outcomes: %+v", res)
	}
	if res[1].Version != 5 {
		t.Fatalf("a conflict reports the current version, got %d", res[1].Version)
	}
	if e.sprintOf(stale) != nil || e.sprintOf(foreign) != nil {
		t.Fatal("conflicting and foreign tasks must not move")
	}
}

// PG2 flavour: two planners with the same version, exactly one wins and nothing is lost.
func TestPlanMoveConcurrentSameVersion(t *testing.T) {
	e := newEnv(t)
	owner := e.user("create")
	p := e.scrum(owner, "CON")
	s1, s2 := e.sprint(owner, p, "S1", "", ""), e.sprint(owner, p, "S2", "", "")
	task := e.task(p, tk{col: e.col(p, "todo")})
	var wg sync.WaitGroup
	results := make([][]PlanResult, 2)
	for i, target := range []uuid.UUID{s1.ID, s2.ID} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i], _ = e.svc.PlanMove(bg, owner, p.ID, PlanInput{TaskIDs: []uuid.UUID{task}, SprintID: &target, Versions: map[string]int{task.String(): 1}})
		}()
	}
	wg.Wait()
	moved, conflicts := 0, 0
	for _, r := range results {
		switch r[0].Status {
		case PlanMoved:
			moved++
		case PlanConflict:
			conflicts++
		}
	}
	if moved != 1 || conflicts != 1 || e.count(`SELECT version FROM tasks WHERE id = ?`, task) != 2 {
		t.Fatalf("moved=%d conflicts=%d", moved, conflicts)
	}
}

func TestPlanMoveRefusals(t *testing.T) {
	e := newEnv(t)
	owner, viewer, stranger := e.user("create"), e.user(), e.user()
	p := e.scrum(owner, "REF")
	other := e.scrum(owner, "REF2")
	e.join(owner, p, viewer, "viewer")
	sp := e.sprint(owner, p, "S1", "2026-03-02", "2026-03-13")
	foreign := e.sprint(owner, other, "S1", "", "")
	task := e.task(p, tk{col: e.col(p, "todo")})
	ids := []uuid.UUID{task}

	_, err := e.svc.PlanMove(bg, viewer, p.ID, PlanInput{TaskIDs: ids, SprintID: &sp.ID})
	wantCode(t, err, "FORBIDDEN")
	_, err = e.svc.PlanMove(bg, stranger, p.ID, PlanInput{TaskIDs: ids, SprintID: &sp.ID})
	wantCode(t, err, "NOT_FOUND")
	_, err = e.svc.PlanMove(bg, owner, p.ID, PlanInput{TaskIDs: ids, SprintID: &foreign.ID})
	wantCode(t, err, "NOT_FOUND")
	missing := uuid.New()
	_, err = e.svc.PlanMove(bg, owner, p.ID, PlanInput{TaskIDs: ids, SprintID: &missing})
	wantCode(t, err, "NOT_FOUND")
	many := make([]uuid.UUID, 101)
	for i := range many {
		many[i] = uuid.New()
	}
	for _, bad := range [][]uuid.UUID{nil, many, {task, task}} {
		_, err = e.svc.PlanMove(bg, owner, p.ID, PlanInput{TaskIDs: bad})
		wantCode(t, err, "VALIDATION_ERROR")
	}
	if _, err = e.svc.StartSprint(bg, owner, sp.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = e.svc.PlanMove(bg, owner, p.ID, PlanInput{TaskIDs: ids, SprintID: &sp.ID}); err != nil {
		t.Fatalf("scope may be added to an active sprint: %v", err)
	}
	if _, err = e.svc.CompleteSprint(bg, owner, sp.ID, CompleteInput{MoveUnfinishedTo: "backlog"}); err != nil {
		t.Fatal(err)
	}
	_, err = e.svc.PlanMove(bg, owner, p.ID, PlanInput{TaskIDs: ids, SprintID: &sp.ID})
	wantCode(t, err, "SPRINT_STATE")
}

// SP-03: the backlog depends on the project mode.
func TestBacklogByMode(t *testing.T) {
	e := newEnv(t)
	owner, stranger := e.user("create"), e.user()
	scrum := e.scrum(owner, "BKS")
	sp := e.sprint(owner, scrum, "S", "", "")
	todo, done := e.col(scrum, "todo"), e.col(scrum, "done")
	var open []uuid.UUID
	for i := 0; i < 3; i++ {
		open = append(open, e.task(scrum, tk{col: todo}))
	}
	e.task(scrum, tk{col: todo, sprint: &sp.ID})
	e.task(scrum, tk{col: done})
	e.task(scrum, tk{col: todo, archived: true})

	var got []uuid.UUID
	cursor := ""
	for page := 0; page < 5; page++ {
		rows, next, err := e.svc.Backlog(bg, owner, scrum.ID, cursor, 2)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range rows {
			got = append(got, r.ID)
		}
		if cursor = next; next == "" {
			break
		}
	}
	if len(got) != 3 || got[0] != open[0] || got[2] != open[2] {
		t.Fatalf("scrum backlog = open tasks without a sprint in rank order, got %v", got)
	}

	kanban := e.project(owner, "BKK", "kanban")
	inBacklog := e.task(kanban, tk{col: e.col(kanban, "backlog")})
	e.task(kanban, tk{col: e.col(kanban, "todo")})
	rows, _, err := e.svc.Backlog(bg, owner, kanban.ID, "", 10)
	if err != nil || len(rows) != 1 || rows[0].ID != inBacklog || rows[0].Ticket != "BKK-1" {
		t.Fatalf("kanban backlog: %+v %v", rows, err)
	}
	_, _, err = e.svc.Backlog(bg, stranger, scrum.ID, "", 10)
	wantCode(t, err, "NOT_FOUND")
	if _, _, err = e.svc.Backlog(bg, owner, scrum.ID, "!!", 10); err == nil {
		t.Fatal("bad cursor must fail")
	}
}

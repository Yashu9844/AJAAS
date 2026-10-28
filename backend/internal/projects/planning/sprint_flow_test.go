package planning

import (
	"encoding/json"
	"sync"
	"testing"

	"github.com/google/uuid"
)

// SP-01/SP-02: planned -> active -> completed, with every refused transition.
func TestSprintStateMachine(t *testing.T) {
	e := newEnv(t)
	owner, member, stranger := e.user("create"), e.user(), e.user()
	p := e.scrum(owner, "SM")
	e.join(owner, p, member, "member")
	nodates := e.sprint(owner, p, "No dates", "", "")
	s1 := e.sprint(owner, p, "One", "2026-03-02", "2026-03-13")
	s2 := e.sprint(owner, p, "Two", "2026-03-16", "2026-03-27")

	_, err := e.svc.StartSprint(bg, owner, nodates.ID)
	wantCode(t, err, "SPRINT_DATES")
	_, err = e.svc.StartSprint(bg, member, s1.ID)
	wantCode(t, err, "FORBIDDEN")
	_, err = e.svc.StartSprint(bg, stranger, s1.ID)
	wantCode(t, err, "NOT_FOUND")
	_, err = e.svc.CompleteSprint(bg, owner, s1.ID, CompleteInput{MoveUnfinishedTo: "backlog"})
	wantCode(t, err, "SPRINT_STATE")

	started, err := e.svc.StartSprint(bg, owner, s1.ID)
	if err != nil || started.Status != SprintActive || started.StartedAt == nil || started.Version != 2 {
		t.Fatalf("start: %+v %v", started, err)
	}
	_, err = e.svc.StartSprint(bg, owner, s1.ID)
	wantCode(t, err, "SPRINT_STATE")
	_, err = e.svc.StartSprint(bg, owner, s2.ID)
	wantCode(t, err, "SPRINT_ACTIVE")

	_, err = e.svc.CompleteSprint(bg, member, s1.ID, CompleteInput{MoveUnfinishedTo: "backlog"})
	wantCode(t, err, "FORBIDDEN")
	for _, bad := range []string{"", "nonsense"} {
		_, err = e.svc.CompleteSprint(bg, owner, s1.ID, CompleteInput{MoveUnfinishedTo: bad})
		wantCode(t, err, "VALIDATION_ERROR")
	}
	other := e.scrum(owner, "SM2")
	foreign := e.sprint(owner, other, "Elsewhere", "", "")
	for _, target := range []string{s1.ID.String(), foreign.ID.String(), uuid.NewString()} {
		_, err = e.svc.CompleteSprint(bg, owner, s1.ID, CompleteInput{MoveUnfinishedTo: target})
		wantCode(t, err, "SPRINT_TARGET")
	}
	done, err := e.svc.CompleteSprint(bg, owner, s1.ID, CompleteInput{MoveUnfinishedTo: s2.ID.String()})
	if err != nil || done.Status != SprintCompleted || done.CompletedAt == nil || done.Report == nil {
		t.Fatalf("complete: %+v %v", done, err)
	}
	_, err = e.svc.CompleteSprint(bg, owner, s1.ID, CompleteInput{MoveUnfinishedTo: "backlog"})
	wantCode(t, err, "SPRINT_STATE")
	_, err = e.svc.UpdateSprint(bg, owner, s1.ID, SprintInput{Goal: ptr("edit")})
	wantCode(t, err, "SPRINT_STATE")
	// A new sprint can start once the previous one is done; a completed sprint is no target.
	_, err = e.svc.CompleteSprint(bg, owner, s2.ID, CompleteInput{MoveUnfinishedTo: "backlog"})
	wantCode(t, err, "SPRINT_STATE")
	if _, err = e.svc.StartSprint(bg, owner, s2.ID); err != nil {
		t.Fatalf("start after completion: %v", err)
	}
	_, err = e.svc.CompleteSprint(bg, owner, s2.ID, CompleteInput{MoveUnfinishedTo: s1.ID.String()})
	wantCode(t, err, "SPRINT_TARGET")
	if e.count(`SELECT count(*) FROM sprints WHERE project_id = ? AND status = 'active'`, p.ID) != 1 {
		t.Fatal("exactly one active sprint")
	}
}

// PG6: concurrent starts leave exactly one active sprint; the losers get 409 SPRINT_ACTIVE.
func TestPG6OneActiveSprintUnderRace(t *testing.T) {
	e := newEnv(t)
	owner := e.user("create")
	p := e.scrum(owner, "RACE")
	const n = 8
	ids := make([]uuid.UUID, n)
	for i := range ids {
		ids[i] = e.sprint(owner, p, "Race "+string(rune('A'+i)), "2026-03-02", "2026-03-13").ID
	}
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = e.svc.StartSprint(bg, owner, ids[i])
		}()
	}
	wg.Wait()
	wins := 0
	for _, err := range errs {
		switch {
		case err == nil:
			wins++
		default:
			wantCode(t, err, "SPRINT_ACTIVE")
		}
	}
	if wins != 1 || e.count(`SELECT count(*) FROM sprints WHERE project_id = ? AND status = 'active'`, p.ID) != 1 {
		t.Fatalf("want exactly one active sprint, wins=%d", wins)
	}
	if e.count(`SELECT count(*) FROM projects_events_outbox WHERE routing_key = 'sprint.started'`) != 1 {
		t.Fatal("only the winner emits sprint.started")
	}
}

// SP-02: completion moves only unfinished tasks and stores the commitment report.
func TestCompleteSprintMovesUnfinishedAndReports(t *testing.T) {
	e := newEnv(t)
	owner := e.user("create")
	p := e.scrum(owner, "RPT")
	dropped := e.canceledCol(owner, p)
	s1 := e.sprint(owner, p, "Now", "2026-03-02", "2026-03-13")
	s2 := e.sprint(owner, p, "Next", "2026-03-16", "2026-03-27")
	in := func(col uuid.UUID, est int) uuid.UUID {
		return e.task(p, tk{col: col, sprint: &s1.ID, est: ptr(est)})
	}
	doneA, doneB := in(e.col(p, "done"), 60), in(e.col(p, "done"), 30)
	todo, doing, cancelled := in(e.col(p, "todo"), 20), e.task(p, tk{col: e.col(p, "in_progress"), sprint: &s1.ID}), in(dropped, 5)
	if _, err := e.svc.StartSprint(bg, owner, s1.ID); err != nil {
		t.Fatal(err)
	}
	live, err := e.svc.SprintReport(bg, owner, s1.ID)
	if err != nil || !live.Live || live.Report.CommittedCount != 5 {
		t.Fatalf("live report: %+v %v", live, err)
	}
	done, err := e.svc.CompleteSprint(bg, owner, s1.ID, CompleteInput{MoveUnfinishedTo: s2.ID.String()})
	if err != nil {
		t.Fatal(err)
	}
	want := Report{CommittedCount: 5, CompletedCount: 2, CommittedEstimateMinutes: 115, CompletedEstimateMinutes: 90,
		MovedUnfinishedCount: 2, MovedTo: s2.ID.String()}
	if *done.Report != want {
		t.Fatalf("report %+v, want %+v", *done.Report, want)
	}
	for _, id := range []uuid.UUID{todo, doing} {
		if got := e.sprintOf(id); got == nil || *got != s2.ID {
			t.Fatalf("unfinished task %s must move to the next sprint", id)
		}
	}
	for _, id := range []uuid.UUID{doneA, doneB, cancelled} {
		if got := e.sprintOf(id); got == nil || *got != s1.ID {
			t.Fatalf("finished task %s stays in the completed sprint", id)
		}
	}
	stored, err := e.svc.SprintReport(bg, owner, s1.ID)
	if err != nil || stored.Live || stored.Report != want {
		t.Fatalf("stored report: %+v %v", stored, err)
	}

	// Completing into the backlog clears the sprint of unfinished tasks.
	if _, err = e.svc.StartSprint(bg, owner, s2.ID); err != nil {
		t.Fatal(err)
	}
	back, err := e.svc.CompleteSprint(bg, owner, s2.ID, CompleteInput{MoveUnfinishedTo: "backlog"})
	if err != nil || back.Report.MovedUnfinishedCount != 2 || back.Report.CommittedCount != 2 {
		t.Fatalf("backlog completion: %+v %v", back.Report, err)
	}
	if e.sprintOf(todo) != nil || e.sprintOf(doing) != nil {
		t.Fatal("unfinished tasks must return to the backlog")
	}
}

// PG12: events and audit carry ids, versions and counts only; never names, goals or titles.
func TestPG12PlanningEventsHoldIdsOnly(t *testing.T) {
	e := newEnv(t)
	owner := e.user("create")
	p := e.scrum(owner, "PG12")
	sp := e.sprint(owner, p, "Secret sprint name", "2026-03-02", "2026-03-13")
	if _, err := e.svc.UpdateSprint(bg, owner, sp.ID, SprintInput{Goal: ptr("Secret goal")}); err != nil {
		t.Fatal(err)
	}
	task := e.task(p, tk{title: "Secret title", col: e.col(p, "todo")})
	if _, err := e.svc.PlanMove(bg, owner, p.ID, PlanInput{TaskIDs: []uuid.UUID{task}, SprintID: &sp.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.StartSprint(bg, owner, sp.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.CompleteSprint(bg, owner, sp.ID, CompleteInput{MoveUnfinishedTo: "backlog"}); err != nil {
		t.Fatal(err)
	}
	var payloads []string
	e.db.Raw(`SELECT payload::text FROM projects_events_outbox WHERE routing_key IN ('sprint.started', 'sprint.completed', 'task.updated')`).Scan(&payloads)
	if len(payloads) != 3 {
		t.Fatalf("want 3 events, got %d", len(payloads))
	}
	allowed := map[string]bool{"project_id": true, "sprint_id": true, "task_id": true, "version": true}
	for _, raw := range payloads {
		var env struct {
			Payload map[string]any `json:"payload"`
		}
		if err := json.Unmarshal([]byte(raw), &env); err != nil {
			t.Fatal(err)
		}
		for k, v := range env.Payload {
			_, isNum := v.(float64)
			if !allowed[k] || (!isNum && !isUUIDOrNil(v)) {
				t.Errorf("event payload field %s=%v is not an id or version", k, v)
			}
		}
	}
	var data []string
	e.db.Raw(`SELECT data::text FROM activity WHERE project_id = ?`, p.ID).Scan(&data)
	for _, d := range data {
		if containsAny(d, "Secret") {
			t.Errorf("activity holds free text: %s", d)
		}
	}
	for _, a := range e.audit.actions {
		if containsAny(a, "Secret") {
			t.Errorf("audit action holds free text: %s", a)
		}
	}
}

func isUUIDOrNil(v any) bool {
	if v == nil {
		return true
	}
	s, ok := v.(string)
	if !ok {
		return false
	}
	_, err := uuid.Parse(s)
	return err == nil
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
	}
	return false
}

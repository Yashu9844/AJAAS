package task

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/kernel"
)

func TestUpdateFieldsAndVersions(t *testing.T) {
	e := newEnv(t)
	owner := e.user()
	p := e.project(owner, "UPD")
	member, viewer := e.member(owner, p, "member"), e.member(owner, p, "viewer")
	ms := e.milestone(owner, p, "Beta")
	v := e.mk(member, p, "Old")
	due := time.Now().Add(48 * time.Hour).UTC().Truncate(time.Second)
	start, _ := kernel.ParseDate("2020-01-01")
	got, err := e.svc.Update(bg, member, v.ID, UpdateInput{Version: num(1), Title: str("  New  "), Description: str("d"), Priority: str("urgent"),
		DueAt: Opt[time.Time]{Set: true, Val: &due}, StartOn: Opt[kernel.Date]{Set: true, Val: &start},
		EstimateMinutes: Opt[int]{Set: true, Val: num(90)}, MilestoneID: Opt[uuid.UUID]{Set: true, Val: &ms.ID}})
	if err != nil || got.Title != "New" || got.Version != 2 || got.Priority != "urgent" || got.DueAt == nil || *got.EstimateMinutes != 90 || got.MilestoneID == nil {
		t.Fatalf("update: %+v %v", got, err)
	}
	_, err = e.svc.Update(bg, member, v.ID, UpdateInput{Version: num(1), Title: str("Stale")})
	vc, ok := kernel.AsVersionConflict(err)
	if !ok || vc.Current != 2 {
		t.Fatalf("stale version: %v", err)
	}
	if _, err = e.svc.Update(bg, member, v.ID, UpdateInput{Title: str("x")}); code(err) != "VALIDATION_ERROR" {
		t.Fatalf("version is required: %v", err)
	}
	cleared, err := e.svc.Update(bg, member, v.ID, UpdateInput{Version: num(2), DueAt: Opt[time.Time]{Set: true}, EstimateMinutes: Opt[int]{Set: true},
		MilestoneID: Opt[uuid.UUID]{Set: true}, StartOn: Opt[kernel.Date]{Set: true}})
	if err != nil || cleared.DueAt != nil || cleared.EstimateMinutes != nil || cleared.MilestoneID != nil || cleared.StartOn != nil || cleared.Version != 3 {
		t.Fatalf("null clears: %+v %v", cleared, err)
	}
	same, err := e.svc.Update(bg, member, v.ID, UpdateInput{Version: num(3), Title: str("New"), Priority: str("urgent")})
	if err != nil || same.Version != 3 {
		t.Fatalf("an update that changes nothing keeps the version: %+v %v", same, err)
	}
	if _, err = e.svc.Update(bg, viewer, v.ID, UpdateInput{Version: num(3), Title: str("x")}); code(err) != "FORBIDDEN" {
		t.Fatalf("viewer: %v", err)
	}
	if _, err = e.svc.Update(bg, e.user(), v.ID, UpdateInput{Version: num(3), Title: str("x")}); code(err) != "NOT_FOUND" {
		t.Fatalf("stranger: %v", err)
	}
	if !e.audit.has("task.due_changed") || len(e.outbox("task.due_changed")) != 2 {
		t.Fatalf("due_changed events: %d", len(e.outbox("task.due_changed")))
	}
	var data string
	e.db.Raw(`SELECT data::text FROM activity WHERE task_id = ? AND type = 'updated' ORDER BY seq LIMIT 1`, v.ID).Scan(&data)
	if !strings.Contains(data, `"priority"`) || strings.Contains(data, "New") {
		t.Fatalf("activity keeps old/new for enums and names for text: %s", data)
	}
}

func TestUpdateValidation(t *testing.T) {
	e := newEnv(t)
	owner := e.user()
	p := e.project(owner, "UVA")
	v := e.mk(owner, p, "t")
	past := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	start, _ := kernel.ParseDate("2030-01-01")
	cases := map[string]UpdateInput{
		"empty title":   {Title: str(" ")},
		"long desc":     {Description: str(strings.Repeat("x", 50001))},
		"bad priority":  {Priority: str("max")},
		"negative est":  {EstimateMinutes: Opt[int]{Set: true, Val: num(-5)}},
		"due < start":   {StartOn: Opt[kernel.Date]{Set: true, Val: &start}, DueAt: Opt[time.Time]{Set: true, Val: &past}},
		"bad milestone": {MilestoneID: Opt[uuid.UUID]{Set: true, Val: ptrID(uuid.New())}},
		"bad sprint":    {SprintID: Opt[uuid.UUID]{Set: true, Val: ptrID(uuid.New())}},
		"bad parent":    {ParentTaskID: Opt[uuid.UUID]{Set: true, Val: ptrID(uuid.New())}},
		"self parent":   {ParentTaskID: Opt[uuid.UUID]{Set: true, Val: &v.ID}},
	}
	for name, in := range cases {
		in.Version = num(1)
		if _, err := e.svc.Update(bg, owner, v.ID, in); code(err) != "VALIDATION_ERROR" {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestUpdateJSONNullSemantics(t *testing.T) {
	var in UpdateInput
	if err := json.Unmarshal([]byte(`{"version":2,"due_at":null,"milestone_id":"`+uuid.NewString()+`","estimate_minutes":5}`), &in); err != nil {
		t.Fatal(err)
	}
	if !in.DueAt.Set || in.DueAt.Val != nil || !in.MilestoneID.Set || in.MilestoneID.Val == nil || in.StartOn.Set || *in.EstimateMinutes.Val != 5 {
		t.Fatalf("decoded: %+v", in)
	}
	if err := json.Unmarshal([]byte(`{"estimate_minutes":"x"}`), &in); err == nil {
		t.Fatal("a wrong type must fail")
	}
	if b, _ := json.Marshal(Opt[int]{Val: num(3)}); string(b) != "3" {
		t.Fatalf("marshal: %s", b)
	}
}

func TestReparent(t *testing.T) {
	e := newEnv(t)
	owner := e.user()
	p := e.project(owner, "REP")
	parent, other, child := e.mk(owner, p, "P"), e.mk(owner, p, "O"), e.mk(owner, p, "C")
	got, err := e.svc.Update(bg, owner, child.ID, UpdateInput{Version: num(1), ParentTaskID: Opt[uuid.UUID]{Set: true, Val: &parent.ID}})
	if err != nil || got.Depth != 1 || got.SubtaskTotal != 0 {
		t.Fatalf("attach: %+v %v", got, err)
	}
	if _, err = e.svc.Update(bg, owner, parent.ID, UpdateInput{Version: num(1), ParentTaskID: Opt[uuid.UUID]{Set: true, Val: &other.ID}}); code(err) != "TASK_HAS_SUBTASKS" {
		t.Fatalf("a task with subtasks cannot be re-parented: %v", err)
	}
	got, err = e.svc.Update(bg, owner, child.ID, UpdateInput{Version: num(2), ParentTaskID: Opt[uuid.UUID]{Set: true}})
	if err != nil || got.Depth != 0 || got.ParentTaskID != nil {
		t.Fatalf("detach: %+v %v", got, err)
	}
}

// TestConcurrentUpdates is PG2: of N writers holding the same version exactly one wins and nothing is lost.
func TestConcurrentUpdates(t *testing.T) {
	e := newEnv(t)
	owner := e.user()
	p := e.project(owner, "CON")
	v := e.mk(owner, p, "race")
	const n = 12
	var wg sync.WaitGroup
	results := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			prio := []string{"low", "medium", "high", "urgent"}[i%4]
			_, err := e.svc.Update(bg, owner, v.ID, UpdateInput{Version: num(1), Priority: &prio, Description: str(strings.Repeat("d", i+1))})
			results <- err
		}(i)
	}
	wg.Wait()
	close(results)
	wins, conflicts := 0, 0
	for err := range results {
		switch code(err) {
		case "":
			wins++
		case "VERSION_CONFLICT":
			conflicts++
		default:
			t.Fatalf("unexpected: %v", err)
		}
	}
	if wins != 1 || conflicts != n-1 {
		t.Fatalf("wins=%d conflicts=%d", wins, conflicts)
	}
	if got, _ := e.svc.Get(bg, owner, v.ID); got.Version != 2 {
		t.Fatalf("version %d", got.Version)
	}
	if e.count(`SELECT count(*) FROM activity WHERE task_id = ? AND type = 'updated'`, v.ID) != 1 {
		t.Fatal("one winner, one activity entry")
	}
}

// TestSequentialUpdatesAllLand: writers that re-read the version each time never lose an update.
func TestSequentialUpdatesAllLand(t *testing.T) {
	e := newEnv(t)
	owner := e.user()
	p := e.project(owner, "SEQ")
	v := e.mk(owner, p, "seq")
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for {
				cur, err := e.svc.Get(bg, owner, v.ID)
				if err != nil {
					t.Error(err)
					return
				}
				_, err = e.svc.Update(bg, owner, v.ID, UpdateInput{Version: &cur.Version, EstimateMinutes: Opt[int]{Set: true, Val: num(i + 1)}})
				if _, retry := kernel.AsVersionConflict(err); !retry {
					if err != nil {
						t.Error(err)
					}
					return
				}
			}
		}(i)
	}
	wg.Wait()
	if got, _ := e.svc.Get(bg, owner, v.ID); got.Version < 2 || got.Version > 9 {
		t.Fatalf("version %d", got.Version)
	}
}

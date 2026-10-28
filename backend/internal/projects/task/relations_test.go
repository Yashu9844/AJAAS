package task

import (
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/project"
)

func TestRelationsBasics(t *testing.T) {
	e := newEnv(t)
	owner := e.user()
	p := e.project(owner, "REL")
	viewer := e.member(owner, p, "viewer")
	a, b := e.mk(owner, p, "a"), e.mk(owner, p, "b")
	rel, err := e.svc.AddRelation(bg, owner, a.ID, RelationInput{TargetTaskID: b.ID, Type: "blocks"})
	if err != nil || rel.SourceTaskID != a.ID || rel.TargetTaskID != b.ID {
		t.Fatalf("blocks: %+v %v", rel, err)
	}
	if _, err = e.svc.AddRelation(bg, owner, a.ID, RelationInput{TargetTaskID: b.ID, Type: "blocks"}); code(err) != "RELATION_EXISTS" {
		t.Fatalf("duplicate: %v", err)
	}
	if _, err = e.svc.AddRelation(bg, owner, b.ID, RelationInput{TargetTaskID: a.ID, Type: "blocks"}); code(err) != "DEPENDENCY_CYCLE" {
		t.Fatalf("two-node cycle: %v", err)
	}
	if _, err = e.svc.AddRelation(bg, owner, a.ID, RelationInput{TargetTaskID: a.ID, Type: "blocks"}); code(err) != "VALIDATION_ERROR" {
		t.Fatalf("self: %v", err)
	}
	if _, err = e.svc.AddRelation(bg, owner, a.ID, RelationInput{TargetTaskID: b.ID, Type: "nope"}); code(err) != "VALIDATION_ERROR" {
		t.Fatalf("type: %v", err)
	}
	if _, err = e.svc.AddRelation(bg, owner, a.ID, RelationInput{TargetTaskID: uuid.New(), Type: "related"}); code(err) != "NOT_FOUND" {
		t.Fatalf("unknown target: %v", err)
	}
	if _, err = e.svc.AddRelation(bg, viewer, a.ID, RelationInput{TargetTaskID: b.ID, Type: "related"}); code(err) != "FORBIDDEN" {
		t.Fatalf("viewer: %v", err)
	}
	r1, _ := e.svc.AddRelation(bg, owner, a.ID, RelationInput{TargetTaskID: b.ID, Type: "related"})
	if _, err = e.svc.AddRelation(bg, owner, b.ID, RelationInput{TargetTaskID: a.ID, Type: "related"}); code(err) != "RELATION_EXISTS" {
		t.Fatalf("related is symmetric: %v", err)
	}
	if _, err = e.svc.AddRelation(bg, owner, b.ID, RelationInput{TargetTaskID: a.ID, Type: "duplicates"}); err != nil {
		t.Fatalf("duplicates: %v", err)
	}
	list, err := e.svc.Relations(bg, owner, b.ID)
	if err != nil || len(list) != 3 {
		t.Fatalf("list: %+v %v", list, err)
	}
	if list[0].Direction != "incoming" || list[0].OtherKey != "REL-1" || list[0].OtherTitle != "a" {
		t.Fatalf("view: %+v", list[0])
	}
	if got, _ := e.svc.Get(bg, owner, b.ID); !got.Blocked {
		t.Fatal("b is blocked by a")
	}
	if err = e.svc.RemoveRelation(bg, viewer, b.ID, r1.ID); code(err) != "FORBIDDEN" {
		t.Fatalf("viewer remove: %v", err)
	}
	if err = e.svc.RemoveRelation(bg, owner, a.ID, uuid.New()); code(err) != "NOT_FOUND" {
		t.Fatalf("unknown relation: %v", err)
	}
	if err = e.svc.RemoveRelation(bg, owner, b.ID, rel.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := e.svc.Get(bg, owner, b.ID); got.Blocked {
		t.Fatal("unblocked after removal")
	}
	if len(e.outbox("task.relation.added")) != 3 || len(e.outbox("task.relation.removed")) != 1 {
		t.Fatal("relation events")
	}
	if !has(e.activityTypes(b.ID), "relation_added") || !has(e.activityTypes(a.ID), "relation_removed") {
		t.Fatalf("both tasks record the relation: %v", e.activityTypes(b.ID))
	}
}

// TestDependencyCycles is PG3: longer cycles are refused, also across projects.
func TestDependencyCycles(t *testing.T) {
	e := newEnv(t)
	owner := e.user()
	p, q := e.project(owner, "CYA"), e.project(owner, "CYB")
	ts := []*View{e.mk(owner, p, "0"), e.mk(owner, q, "1"), e.mk(owner, p, "2"), e.mk(owner, q, "3")}
	for i := 0; i < 3; i++ {
		if _, err := e.svc.AddRelation(bg, owner, ts[i].ID, RelationInput{TargetTaskID: ts[i+1].ID, Type: "blocks"}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := e.svc.AddRelation(bg, owner, ts[3].ID, RelationInput{TargetTaskID: ts[0].ID, Type: "blocks"}); code(err) != "DEPENDENCY_CYCLE" {
		t.Fatalf("four-node cross-project cycle: %v", err)
	}
	if _, err := e.svc.AddRelation(bg, owner, ts[3].ID, RelationInput{TargetTaskID: ts[0].ID, Type: "related"}); err != nil {
		t.Fatalf("related edges are not dependencies: %v", err)
	}
}

// TestConcurrentCycleInserts is PG3: closing the cycle from every side at once leaves an acyclic graph.
func TestConcurrentCycleInserts(t *testing.T) {
	e := newEnv(t)
	owner := e.user()
	p := e.project(owner, "CCY")
	const n = 6
	ts := make([]*View, n)
	for i := range ts {
		ts[i] = e.mk(owner, p, "t")
	}
	for i := 0; i < n-1; i++ { // chain 0 -> 1 -> ... -> n-1 is added concurrently with the closing edges
		_, _ = e.svc.AddRelation(bg, owner, ts[i].ID, RelationInput{TargetTaskID: ts[i+1].ID, Type: "blocks"})
	}
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			if i == j {
				continue
			}
			wg.Add(1)
			go func(i, j int) {
				defer wg.Done()
				_, err := e.svc.AddRelation(bg, owner, ts[i].ID, RelationInput{TargetTaskID: ts[j].ID, Type: "blocks"})
				if c := code(err); c != "" && c != "DEPENDENCY_CYCLE" && c != "RELATION_EXISTS" {
					t.Errorf("unexpected: %v", err)
				}
			}(i, j)
		}
	}
	wg.Wait()
	if hasCycle(e, ts) {
		t.Fatal("the dependency graph contains a cycle")
	}
}

func hasCycle(e *env, ts []*View) bool {
	var edges []struct{ S, T uuid.UUID }
	e.db.Raw(`SELECT source_task_id AS s, target_task_id AS t FROM task_relations WHERE type = 'blocks'`).Scan(&edges)
	adj := map[uuid.UUID][]uuid.UUID{}
	for _, ed := range edges {
		adj[ed.S] = append(adj[ed.S], ed.T)
	}
	state := map[uuid.UUID]int{}
	var visit func(uuid.UUID) bool
	visit = func(n uuid.UUID) bool {
		state[n] = 1
		for _, m := range adj[n] {
			if state[m] == 1 || (state[m] == 0 && visit(m)) {
				return true
			}
		}
		state[n] = 2
		return false
	}
	for _, v := range ts {
		if state[v.ID] == 0 && visit(v.ID) {
			return true
		}
	}
	return false
}

func TestRelationsAcrossProjectsRespectReadAccess(t *testing.T) {
	e := newEnv(t)
	owner, other := e.user(), e.user()
	p, q := e.project(owner, "RAA"), e.project(other, "RAB")
	a, b := e.mk(owner, p, "mine"), e.mk(other, q, "theirs")
	if _, err := e.svc.AddRelation(bg, owner, a.ID, RelationInput{TargetTaskID: b.ID, Type: "related"}); code(err) != "NOT_FOUND" {
		t.Fatalf("a task of an unreadable project is 404: %v", err)
	}
	if _, err := e.proj.AddMember(bg, other, q.ID, project.MemberInput{UserID: owner.UserID, Role: "viewer"}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.AddRelation(bg, owner, a.ID, RelationInput{TargetTaskID: b.ID, Type: "related"}); err != nil {
		t.Fatalf("readable now: %v", err)
	}
	if rows, _ := e.svc.Relations(bg, owner, a.ID); len(rows) != 1 || rows[0].OtherKey != "RAB-1" {
		t.Fatalf("visible relation: %+v", rows)
	}
	if err := e.proj.RemoveMember(bg, other, q.ID, owner.UserID); err != nil {
		t.Fatal(err)
	}
	if rows, _ := e.svc.Relations(bg, owner, a.ID); len(rows) != 0 {
		t.Fatalf("hidden once the other project is unreadable: %+v", rows)
	}
}

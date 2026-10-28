package task

import (
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/kernel"
	"gorm.io/gorm"
)

func TestMoveColumnCompletion(t *testing.T) {
	e := newEnv(t)
	owner := e.user()
	p := e.project(owner, "MOV")
	member := e.member(owner, p, "member")
	done, prog, canceled := e.col(p, "done"), e.col(p, "in-progress"), e.addColumn(owner, p, "Dropped", "canceled", nil, false)
	v := e.mk(member, p, "t")
	got, err := e.svc.Move(bg, member, v.ID, MoveInput{ColumnID: &done, Version: num(1)})
	if err != nil || got.CompletedAt == nil || got.ColumnCategory != "done" || got.Version != 2 {
		t.Fatalf("to done: %+v %v", got, err)
	}
	if len(e.outbox("task.completed")) != 1 || len(e.outbox("task.moved")) != 1 {
		t.Fatalf("events: completed=%d moved=%d", len(e.outbox("task.completed")), len(e.outbox("task.moved")))
	}
	got, err = e.svc.Move(bg, member, v.ID, MoveInput{ColumnID: &prog})
	if err != nil || got.CompletedAt != nil || len(e.outbox("task.reopened")) != 1 {
		t.Fatalf("reopen: %+v %v", got, err)
	}
	got, err = e.svc.Move(bg, member, v.ID, MoveInput{ColumnID: &canceled})
	if err != nil || got.CompletedAt != nil || len(e.outbox("task.completed")) != 1 {
		t.Fatalf("canceled is closed but not done: %+v %v", got, err)
	}
	if _, err = e.svc.Move(bg, member, v.ID, MoveInput{ColumnID: &prog, Version: num(1)}); code(err) != "VERSION_CONFLICT" {
		t.Fatalf("stale: %v", err)
	}
	other := e.project(owner, "MO2")
	foreign := e.col(other, "done")
	if _, err = e.svc.Move(bg, member, v.ID, MoveInput{ColumnID: &foreign}); code(err) != "VALIDATION_ERROR" {
		t.Fatalf("column of another project: %v", err)
	}
	viewer := e.member(owner, p, "viewer")
	if _, err = e.svc.Move(bg, viewer, v.ID, MoveInput{ColumnID: &done}); code(err) != "FORBIDDEN" {
		t.Fatalf("viewer: %v", err)
	}
	if _, err = e.svc.Archive(bg, member, v.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = e.svc.Move(bg, member, v.ID, MoveInput{ColumnID: &done}); code(err) != "TASK_ARCHIVED" {
		t.Fatalf("archived task: %v", err)
	}
}

func TestWipLimits(t *testing.T) {
	e := newEnv(t)
	owner := e.user()
	p := e.project(owner, "WIP")
	soft, hard := e.addColumn(owner, p, "Soft", "in_progress", num(1), false), e.addColumn(owner, p, "Hard", "in_progress", num(1), true)
	a, b, c := e.mk(owner, p, "a"), e.mk(owner, p, "b"), e.mk(owner, p, "c")
	if _, err := e.svc.Move(bg, owner, a.ID, MoveInput{ColumnID: &soft}); err != nil {
		t.Fatal(err)
	}
	got, err := e.svc.Move(bg, owner, b.ID, MoveInput{ColumnID: &soft})
	if err != nil || len(got.Warnings) != 1 || got.Warnings[0] != "WIP_LIMIT_EXCEEDED" {
		t.Fatalf("soft limit warns: %+v %v", got, err)
	}
	if _, err = e.svc.Move(bg, owner, a.ID, MoveInput{ColumnID: &hard}); err != nil {
		t.Fatal(err)
	}
	if _, err = e.svc.Move(bg, owner, c.ID, MoveInput{ColumnID: &hard}); code(err) != "WIP_LIMIT" {
		t.Fatalf("hard limit refuses: %v", err)
	}
	if _, err = e.svc.Create(bg, owner, p.ID, CreateInput{Title: "d", ColumnID: &hard}); code(err) != "WIP_LIMIT" {
		t.Fatalf("create into a full hard column: %v", err)
	}
	if e.count(`SELECT task_counter FROM projects WHERE id = ?`, p.ID) != 3 {
		t.Fatal("a refused create must not consume a number")
	}
	w, err := e.svc.Create(bg, owner, p.ID, CreateInput{Title: "e", ColumnID: &soft})
	if err != nil || len(w.Warnings) != 1 {
		t.Fatalf("create into a full soft column warns: %+v %v", w, err)
	}
}

func TestConcurrentHardWip(t *testing.T) {
	e := newEnv(t)
	owner := e.user()
	p := e.project(owner, "WPC")
	hard := e.addColumn(owner, p, "Hard", "in_progress", num(3), true)
	const n = 9
	ids := make([]uuid.UUID, n)
	for i := range ids {
		ids[i] = e.mk(owner, p, "t").ID
	}
	var wg sync.WaitGroup
	res := make(chan error, n)
	for _, id := range ids {
		wg.Add(1)
		go func(id uuid.UUID) {
			defer wg.Done()
			_, err := e.svc.Move(bg, owner, id, MoveInput{ColumnID: &hard})
			res <- err
		}(id)
	}
	wg.Wait()
	close(res)
	ok := 0
	for err := range res {
		if err == nil {
			ok++
		} else if code(err) != "WIP_LIMIT" {
			t.Fatal(err)
		}
	}
	if ok != 3 || e.count(`SELECT count(*) FROM tasks WHERE column_id = ?`, hard) != 3 {
		t.Fatalf("exactly the limit may enter: ok=%d", ok)
	}
}

func TestDoneRulesBlockersAndSubtasks(t *testing.T) {
	e := newEnv(t)
	owner := e.user()
	p := e.project(owner, "DNR")
	member := e.member(owner, p, "member")
	done := e.col(p, "done")
	blocker, blocked := e.mk(owner, p, "blocker"), e.mk(owner, p, "blocked")
	if _, err := e.svc.AddRelation(bg, owner, blocker.ID, RelationInput{TargetTaskID: blocked.ID, Type: "blocks"}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Move(bg, owner, blocked.ID, MoveInput{ColumnID: &done}); code(err) != "TASK_BLOCKED" {
		t.Fatalf("blocked: %v", err)
	}
	if _, err := e.svc.Move(bg, member, blocked.ID, MoveInput{ColumnID: &done, Force: true}); code(err) != "FORBIDDEN" {
		t.Fatalf("a member cannot force: %v", err)
	}
	if v, err := e.svc.Move(bg, owner, blocked.ID, MoveInput{ColumnID: &done, Force: true}); err != nil || v.CompletedAt == nil {
		t.Fatalf("a manager can force: %v", err)
	}
	parent := e.mk(owner, p, "parent")
	child := e.mk(owner, p, "child", func(in *CreateInput) { in.ParentTaskID = &parent.ID })
	if _, err := e.svc.Move(bg, owner, parent.ID, MoveInput{ColumnID: &done}); code(err) != "OPEN_SUBTASKS" {
		t.Fatalf("open subtasks: %v", err)
	}
	if _, err := e.svc.Move(bg, owner, child.ID, MoveInput{ColumnID: &done}); err != nil {
		t.Fatal(err)
	}
	v, err := e.svc.Move(bg, owner, parent.ID, MoveInput{ColumnID: &done})
	if err != nil || v.SubtaskTotal != 1 || v.SubtaskDone != 1 || v.Progress != 100 {
		t.Fatalf("parent done with a rolled-up progress: %+v %v", v, err)
	}
	if _, err := e.svc.Move(bg, owner, blocker.ID, MoveInput{ColumnID: &done}); err != nil {
		t.Fatalf("a task without blockers moves freely: %v", err)
	}
}

func (e *env) order(col uuid.UUID) []uuid.UUID {
	var ids []uuid.UUID
	e.db.Raw(`SELECT id FROM tasks WHERE column_id = ? AND deleted_at IS NULL ORDER BY rank, id`, col).Scan(&ids)
	return ids
}

func TestReorder(t *testing.T) {
	e := newEnv(t)
	owner := e.user()
	p := e.project(owner, "ORD")
	todo := e.col(p, "todo")
	a, b, c, d := e.mk(owner, p, "a"), e.mk(owner, p, "b"), e.mk(owner, p, "c"), e.mk(owner, p, "d")
	if got := e.order(todo); got[0] != a.ID || got[3] != d.ID {
		t.Fatalf("append order: %v", got)
	}
	v, err := e.svc.Reorder(bg, owner, d.ID, MoveInput{BeforeID: &a.ID})
	if err != nil || v.Version != 1 {
		t.Fatalf("a rank move keeps the version: %+v %v", v, err)
	}
	if got := e.order(todo); got[0] != d.ID || got[1] != a.ID {
		t.Fatalf("before: %v", got)
	}
	if _, err = e.svc.Reorder(bg, owner, d.ID, MoveInput{AfterID: &c.ID}); err != nil {
		t.Fatal(err)
	}
	if got := e.order(todo); got[0] != a.ID || got[1] != b.ID || got[2] != c.ID || got[3] != d.ID {
		t.Fatalf("after: %v", got)
	}
	if _, err = e.svc.Move(bg, owner, a.ID, MoveInput{}); err != nil {
		t.Fatal(err)
	}
	if got := e.order(todo); got[3] != a.ID {
		t.Fatalf("a move without position goes last: %v", got)
	}
	bad := []MoveInput{{}, {BeforeID: &a.ID, AfterID: &b.ID}, {BeforeID: &d.ID}, {AfterID: &d.ID}, {BeforeID: ptrID(uuid.New())}}
	for i, in := range bad {
		if _, err = e.svc.Reorder(bg, owner, d.ID, in); code(err) != "VALIDATION_ERROR" {
			t.Errorf("case %d: %v", i, err)
		}
	}
	prog := e.col(p, "in-progress")
	x := e.mk(owner, p, "x", func(in *CreateInput) { in.ColumnID = &prog })
	if _, err = e.svc.Reorder(bg, owner, d.ID, MoveInput{BeforeID: &x.ID}); code(err) != "VALIDATION_ERROR" {
		t.Fatalf("a neighbour of another column: %v", err)
	}
	v, err = e.svc.Move(bg, owner, d.ID, MoveInput{ColumnID: &prog, AfterID: &x.ID})
	if err != nil || v.ColumnID != prog || e.order(prog)[1] != d.ID {
		t.Fatalf("move between columns at a position: %+v %v", v, err)
	}
}

func TestRebalanceOnEqualAndLongRanks(t *testing.T) {
	e := newEnv(t)
	owner := e.user()
	p := e.project(owner, "REB")
	todo := e.col(p, "todo")
	ids := make([]uuid.UUID, 5)
	for i := range ids {
		ids[i] = e.mk(owner, p, "t").ID
	}
	e.db.Exec(`UPDATE tasks SET rank = 'V' WHERE project_id = ?`, p.ID)
	e.db.Exec(`UPDATE tasks SET rank = 'VVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVV1' WHERE id = ?`, ids[4])
	first := e.order(todo)[0]
	if _, err := e.svc.Reorder(bg, owner, ids[4], MoveInput{AfterID: &first}); err != nil {
		t.Fatalf("equal ranks trigger a rebalance, not an error: %v", err)
	}
	got := e.order(todo)
	if got[0] != first || got[1] != ids[4] {
		t.Fatalf("order after rebalance: %v", got)
	}
	var ranks []string
	e.db.Raw(`SELECT rank FROM tasks WHERE project_id = ? ORDER BY rank`, p.ID).Scan(&ranks)
	for i := 1; i < len(ranks); i++ {
		if ranks[i] == ranks[i-1] || len(ranks[i]) > maxRankLen || !kernel.ValidRank(ranks[i]) {
			t.Fatalf("ranks after rebalance: %v", ranks)
		}
	}
}

func TestSpreadRanks(t *testing.T) {
	for _, n := range []int{0, 1, 2, 61, 62, 63, 500, 5000} {
		r := spreadRanks(n)
		if len(r) != n {
			t.Fatalf("n=%d: %d ranks", n, len(r))
		}
		for i := range r {
			if !kernel.ValidRank(r[i]) || len(r[i]) > 6 || (i > 0 && r[i] <= r[i-1]) {
				t.Fatalf("n=%d rank %d = %q", n, i, r[i])
			}
		}
	}
}

func TestRebalanceBound(t *testing.T) {
	e := newEnv(t)
	owner := e.user()
	p := e.project(owner, "BND")
	todo := e.col(p, "todo")
	e.mk(owner, p, "seed")
	if err := e.db.Exec(`INSERT INTO tasks (id, tenant_id, project_id, number, title, column_id, reporter_user_id, rank)
		SELECT gen_random_uuid(), ?, ?, 1000 + g, 't', ?, ?, 'V' FROM generate_series(1, ?) g`, e.tenant, p.ID, todo, owner.UserID,
		rebalanceLimit+1).Error; err != nil {
		t.Fatal(err)
	}
	err := e.db.Transaction(func(tx *gorm.DB) error { return rebalance(tx, e.tenant, todo) })
	wantCode(t, err, "COLUMN_TOO_LARGE")
}

func TestConcurrentMovesLoseNothing(t *testing.T) {
	e := newEnv(t)
	owner := e.user()
	p := e.project(owner, "CMV")
	prog := e.col(p, "in-progress")
	const n = 16
	ids := make([]uuid.UUID, n)
	for i := range ids {
		ids[i] = e.mk(owner, p, "t").ID
	}
	var wg sync.WaitGroup
	for _, id := range ids {
		wg.Add(1)
		go func(id uuid.UUID) {
			defer wg.Done()
			if _, err := e.svc.Move(bg, owner, id, MoveInput{ColumnID: &prog}); err != nil {
				t.Error(err)
			}
		}(id)
	}
	wg.Wait()
	if got := e.order(prog); len(got) != n {
		t.Fatalf("lost tasks: %d of %d", len(got), n)
	}
	if e.count(`SELECT count(DISTINCT rank) FROM tasks WHERE column_id = ?`, prog) != n {
		t.Fatal("concurrent moves must allocate distinct ranks")
	}
}

package planning

import (
	"testing"

	"github.com/google/uuid"
)

func (e *env) boardColumn(b *Board, id uuid.UUID) BoardColumn {
	e.t.Helper()
	for _, c := range b.Columns {
		if c.ID == id {
			return c
		}
	}
	e.t.Fatal("column missing from the board")
	return BoardColumn{}
}

// VW-01: counts per column and a stable, cursor-paged slice of tasks per column.
func TestBoardPagesEachColumn(t *testing.T) {
	e := newEnv(t)
	owner, viewer := e.user("create"), e.user()
	p := e.scrum(owner, "BRD")
	e.join(owner, p, viewer, "viewer")
	todo, doing := e.col(p, "todo"), e.col(p, "in_progress")
	var want []uuid.UUID
	for i := 0; i < 7; i++ {
		want = append(want, e.task(p, tk{col: todo}))
	}
	e.task(p, tk{col: doing})
	e.task(p, tk{col: todo, archived: true})
	e.task(p, tk{col: todo, deleted: true})

	b, err := e.svc.Board(bg, viewer, p.ID, BoardParams{PerColumn: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Columns) != 4 || b.Key != "BRD" || b.Mode != "scrum" || b.PerColumn != 3 {
		t.Fatalf("board: %+v", b)
	}
	col := e.boardColumn(b, todo)
	if col.Count != 7 || len(col.Tasks) != 3 || col.NextCursor == nil || e.boardColumn(b, doing).Count != 1 {
		t.Fatalf("counts/paging: %+v", col)
	}
	if empty := e.boardColumn(b, e.col(p, "done")); empty.Count != 0 || empty.Tasks == nil || empty.NextCursor != nil {
		t.Fatalf("empty column: %+v", empty)
	}

	got := append([]uuid.UUID{}, ids(col.Tasks)...)
	cursor := *col.NextCursor
	for cursor != "" {
		next, err := e.svc.Board(bg, viewer, p.ID, BoardParams{PerColumn: 3, Cursors: map[uuid.UUID]string{todo: cursor}})
		if err != nil {
			t.Fatal(err)
		}
		c := e.boardColumn(next, todo)
		got = append(got, ids(c.Tasks)...)
		cursor = ""
		if c.NextCursor != nil {
			cursor = *c.NextCursor
		}
	}
	if len(got) != len(want) {
		t.Fatalf("paged %d tasks, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order is rank order, position %d", i)
		}
	}
}

// A task inserted while a client is paging must not duplicate or skip the rows after the cursor.
func TestBoardPagingIsStableUnderInserts(t *testing.T) {
	e := newEnv(t)
	owner := e.user("create")
	p := e.scrum(owner, "STB")
	todo := e.col(p, "todo")
	for i := 0; i < 6; i++ {
		e.task(p, tk{col: todo})
	}
	first, _ := e.svc.Board(bg, owner, p.ID, BoardParams{PerColumn: 2})
	c1 := e.boardColumn(first, todo)
	e.db.Exec(`INSERT INTO tasks (id, tenant_id, project_id, number, title, column_id, reporter_user_id, rank)
		VALUES (?, ?, ?, 99, 'early', ?, ?, 'a0')`, uuid.New(), p.TenantID, p.ID, todo, owner.UserID)
	second, _ := e.svc.Board(bg, owner, p.ID, BoardParams{PerColumn: 2, Cursors: map[uuid.UUID]string{todo: *c1.NextCursor}})
	c2 := e.boardColumn(second, todo)
	seen := map[uuid.UUID]bool{}
	for _, c := range append(ids(c1.Tasks), ids(c2.Tasks)...) {
		if seen[c] {
			t.Fatal("a row was returned twice")
		}
		seen[c] = true
	}
	if len(c2.Tasks) != 2 || c2.Count != 7 {
		t.Fatalf("second page: %+v", c2)
	}
}

func TestBoardSprintScopeAndErrors(t *testing.T) {
	e := newEnv(t)
	owner, stranger := e.user("create"), e.user()
	p := e.scrum(owner, "SCP")
	sp := e.sprint(owner, p, "S", "2026-03-02", "2026-03-13")
	todo := e.col(p, "todo")
	in := e.task(p, tk{col: todo, sprint: &sp.ID})
	e.task(p, tk{col: todo})
	if _, err := e.svc.StartSprint(bg, owner, sp.ID); err != nil {
		t.Fatal(err)
	}
	for scope, want := range map[string]int{"": 2, "none": 1, "active": 1, sp.ID.String(): 1} {
		b, err := e.svc.Board(bg, owner, p.ID, BoardParams{Sprint: scope})
		if err != nil {
			t.Fatalf("%q: %v", scope, err)
		}
		if got := e.boardColumn(b, todo).Count; got != want {
			t.Errorf("sprint %q: count %d, want %d", scope, got, want)
		}
	}
	b, _ := e.svc.Board(bg, owner, p.ID, BoardParams{Sprint: "active"})
	if c := e.boardColumn(b, todo); len(c.Tasks) != 1 || c.Tasks[0].ID != in || c.Tasks[0].SprintID == nil {
		t.Fatalf("active sprint card: %+v", c.Tasks)
	}
	_, err := e.svc.Board(bg, owner, p.ID, BoardParams{Sprint: "sideways"})
	wantCode(t, err, "VALIDATION_ERROR")
	_, err = e.svc.Board(bg, owner, p.ID, BoardParams{Sprint: uuid.NewString()})
	wantCode(t, err, "NOT_FOUND")
	_, err = e.svc.Board(bg, stranger, p.ID, BoardParams{})
	wantCode(t, err, "NOT_FOUND")
	_, err = e.svc.Board(bg, owner, p.ID, BoardParams{Cursors: map[uuid.UUID]string{todo: "%%%"}})
	wantCode(t, err, "VALIDATION_ERROR")
	if ClampPerColumn(0) != 25 || ClampPerColumn(500) != 100 || ClampPerColumn(40) != 40 {
		t.Fatal("per_column clamp")
	}
}

// Cards carry assignees and labels without a query per task.
func TestBoardCardsCarryAssigneesAndLabels(t *testing.T) {
	e := newEnv(t)
	owner, dev := e.user("create"), e.user()
	p := e.scrum(owner, "CRD")
	e.join(owner, p, dev, "member")
	task := e.task(p, tk{title: "Card", col: e.col(p, "todo"), assignee: &dev.UserID})
	var label uuid.UUID
	e.db.Raw(`INSERT INTO labels (id, tenant_id, project_id, name) VALUES (gen_random_uuid(), ?, ?, 'bug') RETURNING id`, p.TenantID, p.ID).Row().Scan(&label)
	e.db.Exec(`INSERT INTO task_labels (task_id, label_id, tenant_id) VALUES (?, ?, ?)`, task, label, p.TenantID)
	b, _ := e.svc.Board(bg, owner, p.ID, BoardParams{})
	card := e.boardColumn(b, e.col(p, "todo")).Tasks[0]
	if len(card.AssigneeIDs) != 1 || card.AssigneeIDs[0] != dev.UserID.String() || len(card.LabelIDs) != 1 || card.Ticket != "CRD-1" || card.Category != "todo" {
		t.Fatalf("card: %+v", card)
	}
}

func ids(cards []Card) []uuid.UUID {
	out := make([]uuid.UUID, len(cards))
	for i, c := range cards {
		out[i] = c.ID
	}
	return out
}

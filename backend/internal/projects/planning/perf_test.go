package planning

import (
	"os"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
)

const perfBudget = 500 * time.Millisecond

func perfOnly(t *testing.T) {
	t.Helper()
	if os.Getenv("JAAS_PERF") != "1" || testing.Short() {
		t.Skip("set JAAS_PERF=1 to run the 50 000-task performance test (PG7)")
	}
}

// p95 runs fn n times and returns the 95th percentile latency.
func p95(t *testing.T, n int, fn func(i int)) time.Duration {
	t.Helper()
	d := make([]time.Duration, n)
	for i := range d {
		start := time.Now()
		fn(i)
		d[i] = time.Since(start)
	}
	sort.Slice(d, func(a, b int) bool { return d[a] < d[b] })
	return d[(n*95)/100]
}

func (e *env) seedBigProject(owner api.Actor) (uuid.UUID, uuid.UUID) {
	e.t.Helper()
	p := e.scrum(owner, "BIGP")
	cols := []string{"todo", "in_progress", "backlog", "done"}
	weights := []int{30000, 10000, 5000, 5000}
	start := 1
	for i, c := range cols {
		err := e.db.Exec(`INSERT INTO tasks (id, tenant_id, project_id, number, title, description, column_id, reporter_user_id, rank, due_at, priority)
			SELECT gen_random_uuid(), ?, ?, i, 'Task ' || i || CASE WHEN i % 100 = 0 THEN ' needle' ELSE '' END, 'seeded task body ' || i,
			?, ?, 'r' || lpad(i::text, 8, '0'), CASE WHEN i % 3 = 0 THEN now() + (i % 30) * interval '1 day' END, 'medium'
			FROM generate_series(?::int, ?::int) i`, p.TenantID, p.ID, e.col(p, c), owner.UserID, start, start+weights[i]-1).Error
		if err != nil {
			e.t.Fatal(err)
		}
		start += weights[i]
	}
	e.db.Exec(`UPDATE projects SET task_counter = 50000 WHERE id = ?`, p.ID)
	e.db.Exec(`INSERT INTO task_assignees (task_id, user_id, tenant_id, assigned_by) SELECT id, ?, tenant_id, ? FROM tasks WHERE project_id = ? AND number % 50 = 0`,
		owner.UserID, owner.UserID, p.ID)
	e.db.Exec(`ANALYZE`)
	return p.ID, e.col(p, "todo")
}

// PG7: a 50 000-task project pages within the list budget with a stable keyset order.
func TestPG7LargeBoardPaginationWithinBudget(t *testing.T) {
	perfOnly(t)
	e := newEnv(t)
	owner := e.user("create")
	seedStart := time.Now()
	pid, todo := e.seedBigProject(owner)
	t.Logf("seeded 50000 tasks in %v", time.Since(seedStart).Round(time.Millisecond))
	if e.count(`SELECT count(*) FROM tasks WHERE project_id = ?`, pid) != 50000 {
		t.Fatal("seed size")
	}

	// Stable order: walk the whole todo column (30 000 tasks) and require strictly increasing (rank, id), no gaps, no repeats.
	cursor, seen, last := "", 0, ""
	var walk []time.Duration
	for {
		start := time.Now()
		b, err := e.svc.Board(bg, owner, pid, BoardParams{PerColumn: 100, Cursors: map[uuid.UUID]string{todo: cursor}})
		walk = append(walk, time.Since(start))
		if err != nil {
			t.Fatal(err)
		}
		col := e.boardColumn(b, todo)
		for _, c := range col.Tasks {
			if c.Rank <= last {
				t.Fatalf("order broke at %s after %s", c.Rank, last)
			}
			last = c.Rank
			seen++
		}
		if col.NextCursor == nil {
			break
		}
		cursor = *col.NextCursor
	}
	if seen != 30000 {
		t.Fatalf("walked %d tasks, want 30000", seen)
	}
	sort.Slice(walk, func(a, b int) bool { return walk[a] < walk[b] })
	walkP95 := walk[len(walk)*95/100]
	t.Logf("board walk: %d pages, p95 %v, max %v", len(walk), walkP95, walk[len(walk)-1])
	if walkP95 > perfBudget {
		t.Errorf("board page p95 %v over budget %v", walkP95, perfBudget)
	}

	mid := kernelCursor(t, e, owner, pid, todo)
	checks := map[string]func(i int){
		"board first page (50/col)": func(int) { _, err := e.svc.Board(bg, owner, pid, BoardParams{PerColumn: 50}); noErr(t, err) },
		"board deep page": func(int) {
			_, err := e.svc.Board(bg, owner, pid, BoardParams{PerColumn: 100, Cursors: map[uuid.UUID]string{todo: mid}})
			noErr(t, err)
		},
		"backlog page":  func(int) { _, _, err := e.svc.Backlog(bg, owner, pid, "", 100); noErr(t, err) },
		"search needle": func(int) { _, err := e.svc.Search(bg, owner, SearchParams{Query: "needle", Limit: 50}); noErr(t, err) },
		"search ticket": func(int) {
			_, err := e.svc.Search(bg, owner, SearchParams{Query: "BIGP-12345", Limit: 50})
			noErr(t, err)
		},
		"gantt page":     func(int) { _, err := e.svc.Gantt(bg, owner, pid, GanttParams{Limit: 500}); noErr(t, err) },
		"my-work":        func(int) { _, err := e.svc.MyWork(bg, owner, WorkParams{PerGroup: 50}); noErr(t, err) },
		"my-work counts": func(int) { _, err := e.svc.MyWorkCounts(bg, owner); noErr(t, err) },
	}
	for name, fn := range checks {
		got := p95(t, 30, fn)
		t.Logf("%-28s p95 %v", name, got.Round(time.Millisecond))
		if got > perfBudget {
			t.Errorf("%s: p95 %v over budget %v", name, got, perfBudget)
		}
	}
}

// kernelCursor returns a cursor in the middle of the todo column.
func kernelCursor(t *testing.T, e *env, owner api.Actor, pid, todo uuid.UUID) string {
	t.Helper()
	var c rankCursor
	row := e.db.Raw(`SELECT rank, id::text FROM tasks WHERE project_id = ? AND column_id = ? ORDER BY rank OFFSET 15000 LIMIT 1`, pid, todo).Row()
	if err := row.Scan(&c.Rank, &c.ID); err != nil {
		t.Fatal(err)
	}
	return kernel.EncodeCursor(c)
}

func noErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

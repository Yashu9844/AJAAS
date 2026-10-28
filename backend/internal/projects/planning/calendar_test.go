package planning

import "testing"

func TestParseWindow(t *testing.T) {
	if w, err := ParseWindow("2026-03-01", "2026-05-01", 62); err != nil || w.From.String() != "2026-03-01" {
		t.Fatalf("62 days is the maximum: %v", err)
	}
	for name, c := range map[string][3]string{
		"63 days": {"2026-03-01", "2026-05-02", ""}, "reversed": {"2026-03-02", "2026-03-01", ""},
		"bad from": {"yesterday", "2026-03-01", ""}, "bad to": {"2026-03-01", "", ""}, "missing": {"", "", ""},
	} {
		if _, err := ParseWindow(c[0], c[1], 62); err == nil {
			t.Errorf("%s: want a validation error", name)
		}
	}
	w, _ := ParseWindow("2026-03-01", "2026-03-01", 62)
	if from, to := w.bounds(); to.Sub(from).Hours() != 24 {
		t.Fatal("a one-day window spans 24h")
	}
}

// VW-03: tasks by due_at or start_on for a window.
func TestProjectCalendarWindow(t *testing.T) {
	e := newEnv(t)
	owner, stranger := e.user("create"), e.user()
	p := e.scrum(owner, "CAL")
	todo := e.col(p, "todo")
	dueIn := e.task(p, tk{title: "due in", col: todo, due: at("2026-03-10T09:00:00Z")})
	lastDay := e.task(p, tk{title: "last day", col: todo, due: at("2026-03-31T23:30:00Z")})
	starts := e.task(p, tk{title: "starts in", col: todo, start: "2026-03-05"})
	e.task(p, tk{title: "before", col: todo, due: at("2026-02-28T23:59:00Z")})
	e.task(p, tk{title: "after", col: todo, due: at("2026-04-01T00:00:00Z")})
	e.task(p, tk{title: "archived", col: todo, due: at("2026-03-10T09:00:00Z"), archived: true})
	e.task(p, tk{title: "undated", col: todo})

	w, _ := ParseWindow("2026-03-01", "2026-03-31", 62)
	v, err := e.svc.ProjectCalendar(bg, owner, p.ID, w)
	if err != nil {
		t.Fatal(err)
	}
	got := ids(v.Tasks)
	if len(got) != 3 || got[0] != starts || got[1] != dueIn || got[2] != lastDay || v.Truncated || v.From != "2026-03-01" {
		t.Fatalf("calendar: %+v", v)
	}
	_, err = e.svc.ProjectCalendar(bg, stranger, p.ID, w)
	wantCode(t, err, "NOT_FOUND")
}

func TestCalendarTruncatesAt500(t *testing.T) {
	e := newEnv(t)
	owner := e.user("create")
	p := e.scrum(owner, "BIG")
	e.db.Exec(`INSERT INTO tasks (id, tenant_id, project_id, number, title, column_id, reporter_user_id, rank, due_at)
		SELECT gen_random_uuid(), ?, ?, i, 'T', ?, ?, 'r' || lpad(i::text, 8, '0'), '2026-03-10T09:00:00Z'::timestamptz
		FROM generate_series(1, 501) i`, p.TenantID, p.ID, e.col(p, "todo"), owner.UserID)
	w, _ := ParseWindow("2026-03-01", "2026-03-31", 62)
	v, err := e.svc.ProjectCalendar(bg, owner, p.ID, w)
	if err != nil || len(v.Tasks) != 500 || !v.Truncated {
		t.Fatalf("want 500 and truncated, got %d %v %v", len(v.Tasks), v.Truncated, err)
	}
}

// VW-03: my calendar lists only my assigned tasks, only in projects I can read.
func TestMyCalendar(t *testing.T) {
	e := newEnv(t)
	owner, me := e.user("create"), e.user()
	p, hidden := e.scrum(owner, "MYC"), e.scrum(owner, "HID")
	e.join(owner, p, me, "member")
	due := at("2026-03-10T09:00:00Z")
	mine := e.task(p, tk{col: e.col(p, "todo"), due: due, assignee: &me.UserID})
	e.task(p, tk{col: e.col(p, "todo"), due: due})
	e.task(hidden, tk{col: e.col(hidden, "todo"), due: due, assignee: &me.UserID})
	w, _ := ParseWindow("2026-03-01", "2026-03-31", 62)
	v, err := e.svc.MyCalendar(bg, me, w)
	if err != nil || len(v.Tasks) != 1 || v.Tasks[0].ID != mine {
		t.Fatalf("my calendar: %+v %v", v, err)
	}
	empty, err := e.svc.MyCalendar(bg, e.user(), w)
	if err != nil || len(empty.Tasks) != 0 || empty.Tasks == nil {
		t.Fatalf("a user with no projects gets an empty list: %+v %v", empty, err)
	}
}

// VW-04: dated tasks, milestones and blocks relations, paged.
func TestGantt(t *testing.T) {
	e := newEnv(t)
	owner, stranger := e.user("create"), e.user()
	p := e.scrum(owner, "GNT")
	todo := e.col(p, "todo")
	a := e.task(p, tk{col: todo, start: "2026-03-02", due: at("2026-03-06T10:00:00Z")})
	b := e.task(p, tk{col: todo, due: at("2026-03-09T10:00:00Z")})
	c := e.task(p, tk{col: todo, start: "2026-03-20"})
	e.task(p, tk{col: todo})
	e.task(p, tk{col: todo, start: "2026-06-01"})
	e.db.Exec(`INSERT INTO milestones (id, tenant_id, project_id, name, due_on) VALUES (gen_random_uuid(), ?, ?, 'Beta', '2026-03-15'),
		(gen_random_uuid(), ?, ?, 'Later', '2026-09-01')`, p.TenantID, p.ID, p.TenantID, p.ID)
	e.db.Exec(`INSERT INTO task_relations (id, tenant_id, source_task_id, target_task_id, type, created_by) VALUES (gen_random_uuid(), ?, ?, ?, 'blocks', ?)`,
		p.TenantID, a, b, owner.UserID)

	w, _ := ParseWindow("2026-03-01", "2026-03-31", maxGanttWindow)
	v, err := e.svc.Gantt(bg, owner, p.ID, GanttParams{Window: &w, Limit: 2})
	if err != nil || len(v.Tasks) != 2 || v.NextCursor == nil || v.Tasks[0].ID != a || v.Tasks[1].ID != b {
		t.Fatalf("first page: %+v %v", v, err)
	}
	if len(v.Milestones) != 1 || v.Milestones[0].Name != "Beta" || v.Milestones[0].State != "open" {
		t.Fatalf("milestones in the window: %+v", v.Milestones)
	}
	if len(v.Dependencies) != 1 || v.Dependencies[0].SourceTaskID != a || v.Tasks[1].DependencyCount != 1 || v.Tasks[0].DependencyCount != 0 {
		t.Fatalf("dependencies: %+v", v.Dependencies)
	}
	rest, err := e.svc.Gantt(bg, owner, p.ID, GanttParams{Window: &w, Limit: 2, Cursor: *v.NextCursor})
	if err != nil || len(rest.Tasks) != 1 || rest.Tasks[0].ID != c || rest.NextCursor != nil {
		t.Fatalf("second page: %+v %v", rest, err)
	}
	all, err := e.svc.Gantt(bg, owner, p.ID, GanttParams{Limit: 50})
	if err != nil || len(all.Tasks) != 4 || len(all.Milestones) != 2 {
		t.Fatalf("no window: %d tasks %d milestones %v", len(all.Tasks), len(all.Milestones), err)
	}
	_, err = e.svc.Gantt(bg, stranger, p.ID, GanttParams{Limit: 5})
	wantCode(t, err, "NOT_FOUND")
	_, err = e.svc.Gantt(bg, owner, p.ID, GanttParams{Limit: 5, Cursor: "@@"})
	wantCode(t, err, "VALIDATION_ERROR")
	if ClampGanttLimit("") != 200 || ClampGanttLimit("9999") != 500 || ClampGanttLimit("7") != 7 {
		t.Fatal("gantt limit clamp")
	}
}

// A blocks edge never reveals a task of a project the caller cannot read.
func TestGanttDependenciesHideUnreadableProjects(t *testing.T) {
	e := newEnv(t)
	owner, other := e.user("create"), e.user("create")
	mine, secret := e.scrum(owner, "VIS"), e.scrum(other, "SEC")
	e.join(other, secret, owner, "viewer")
	e.db.Exec(`DELETE FROM project_members WHERE project_id = ? AND user_id = ?`, secret.ID, owner.UserID)
	a := e.task(mine, tk{col: e.col(mine, "todo"), start: "2026-03-02"})
	hidden := e.task(secret, tk{col: e.col(secret, "todo"), start: "2026-03-02"})
	e.db.Exec(`INSERT INTO task_relations (id, tenant_id, source_task_id, target_task_id, type, created_by) VALUES (gen_random_uuid(), ?, ?, ?, 'blocks', ?)`,
		mine.TenantID, hidden, a, owner.UserID)
	v, err := e.svc.Gantt(bg, owner, mine.ID, GanttParams{Limit: 10})
	if err != nil || len(v.Dependencies) != 0 || v.Tasks[0].DependencyCount != 0 {
		t.Fatalf("an unreadable blocker must stay hidden: %+v %v", v.Dependencies, err)
	}
}

package planning

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
)

// A Wednesday, so "this week" runs Thursday..Sunday.
func fixedNow() time.Time { return time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC) }

func TestDayBounds(t *testing.T) {
	for day, wantNext := range map[string]string{"2026-10-26": "2026-11-02", "2026-10-28": "2026-11-02", "2026-11-01": "2026-11-02", "2026-11-02": "2026-11-09"} {
		d, _ := time.Parse("2006-01-02", day)
		if got := boundsAt(d.Add(5 * time.Hour)).nextWeek.Format("2006-01-02"); got != wantNext {
			t.Errorf("%s: next week starts %s, want %s", day, got, wantNext)
		}
	}
}

func (e *env) workSetup() (api.Actor, api.Actor, map[string]uuid.UUID) {
	owner, me := e.user("create"), e.user()
	p := e.scrum(owner, "WRK")
	e.join(owner, p, me, "member")
	todo := e.col(p, "todo")
	mk := func(due string) uuid.UUID {
		var d *time.Time
		if due != "" {
			d = at(due)
		}
		return e.task(p, tk{col: todo, due: d, assignee: &me.UserID})
	}
	t := map[string]uuid.UUID{
		"overdue": mk("2026-10-27T10:00:00Z"), "today": mk("2026-10-28T08:00:00Z"), "today2": mk("2026-10-28T23:00:00Z"),
		"week": mk("2026-10-30T10:00:00Z"), "later": mk("2026-11-02T00:00:00Z"), "nodate": mk(""),
	}
	t["done"] = e.task(p, tk{col: e.col(p, "done"), due: at("2026-10-27T10:00:00Z"), assignee: &me.UserID})
	t["others"] = e.task(p, tk{col: todo, due: at("2026-10-27T10:00:00Z"), assignee: &owner.UserID})
	t["archived"] = e.task(p, tk{col: todo, due: at("2026-10-27T10:00:00Z"), assignee: &me.UserID, archived: true})
	gone := e.scrum(owner, "GONE")
	e.join(owner, gone, me, "member")
	t["gone"] = e.task(gone, tk{col: e.col(gone, "todo"), due: at("2026-10-27T10:00:00Z"), assignee: &me.UserID})
	e.db.Exec(`DELETE FROM project_members WHERE project_id = ? AND user_id = ?`, gone.ID, me.UserID) // no longer readable
	frozen := e.scrum(owner, "FRZ")
	e.join(owner, frozen, me, "member")
	t["frozen"] = e.task(frozen, tk{col: e.col(frozen, "todo"), assignee: &me.UserID})
	e.db.Exec(`UPDATE projects SET status = 'archived' WHERE id = ?`, frozen.ID)
	return owner, me, t
}

// VW-05: grouped by due date, open tasks only, readable active projects only.
func TestMyWorkGroupsAndCounts(t *testing.T) {
	e := newEnv(t)
	e.svc.now = fixedNow
	_, me, tasks := e.workSetup()
	v, err := e.svc.MyWork(bg, me, WorkParams{})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]uuid.UUID{GroupOverdue: {tasks["overdue"]}, GroupToday: {tasks["today"], tasks["today2"]}, GroupThisWeek: {tasks["week"]},
		GroupLater: {tasks["later"]}, GroupNoDate: {tasks["nodate"]}}
	for g, wantIDs := range want {
		got := ids(v.Groups[g])
		if len(got) != len(wantIDs) {
			t.Fatalf("group %s: got %d tasks, want %d", g, len(got), len(wantIDs))
		}
		for i := range wantIDs {
			if got[i] != wantIDs[i] {
				t.Errorf("group %s position %d", g, i)
			}
		}
	}
	if c := v.Counts; c.Overdue != 1 || c.Today != 2 || c.ThisWeek != 1 || c.Later != 1 || c.NoDate != 1 || c.Total != 6 {
		t.Fatalf("counts: %+v", c)
	}
	badge, err := e.svc.MyWorkCounts(bg, me)
	if err != nil || badge.Total != 6 || badge.Today != 2 {
		t.Fatalf("badge counts: %+v %v", badge, err)
	}
	none, err := e.svc.MyWorkCounts(bg, e.user())
	if err != nil || none.Total != 0 {
		t.Fatalf("no projects: %+v %v", none, err)
	}
	empty, _ := e.svc.MyWork(bg, e.user(), WorkParams{})
	if len(empty.Groups) != 7 || empty.Groups[GroupToday] == nil {
		t.Fatalf("empty groups must still be present: %+v", empty.Groups)
	}
}

func TestMyWorkPagesOneGroup(t *testing.T) {
	e := newEnv(t)
	e.svc.now = fixedNow
	_, me, tasks := e.workSetup()
	one, err := e.svc.MyWork(bg, me, WorkParams{Group: GroupToday, PerGroup: 1})
	if err != nil || len(one.Groups[GroupToday]) != 1 || len(one.Groups[GroupLater]) != 0 || one.NextCursor == nil || one.Groups[GroupToday][0].ID != tasks["today"] {
		t.Fatalf("first page: %+v %v", one, err)
	}
	two, err := e.svc.MyWork(bg, me, WorkParams{Group: GroupToday, PerGroup: 1, Cursor: *one.NextCursor})
	if err != nil || two.Groups[GroupToday][0].ID != tasks["today2"] || two.NextCursor != nil {
		t.Fatalf("second page: %+v %v", two, err)
	}
	nodate, err := e.svc.MyWork(bg, me, WorkParams{Group: GroupNoDate})
	if err != nil || len(nodate.Groups[GroupNoDate]) != 1 || nodate.NextCursor != nil {
		t.Fatalf("no_date group: %+v %v", nodate, err)
	}
	_, err = e.svc.MyWork(bg, me, WorkParams{Group: "someday"})
	wantCode(t, err, "VALIDATION_ERROR")
	_, err = e.svc.MyWork(bg, me, WorkParams{Group: GroupToday, Cursor: "%"})
	wantCode(t, err, "VALIDATION_ERROR")
	if ClampWorkLimit(0) != 20 || ClampWorkLimit(500) != 100 {
		t.Fatal("per_group clamp")
	}
}

// VW-05: watching and mentions come from the collaboration tables and respect readability.
func TestMyWorkWatchingAndMentions(t *testing.T) {
	e := newEnv(t)
	e.svc.now = fixedNow
	_, me, tasks := e.workSetup()
	watch := func(task uuid.UUID) {
		e.db.Exec(`INSERT INTO task_watchers (task_id, user_id, tenant_id) VALUES (?, ?, ?)`, task, me.UserID, me.TenantID)
	}
	watch(tasks["others"])
	watch(tasks["today"])
	watch(tasks["done"]) // finished work is not listed
	watch(tasks["gone"]) // unreadable project
	e.db.Exec(`INSERT INTO mentions (tenant_id, source_kind, source_id, task_id, user_id) VALUES (?, 'task', ?, ?, ?)`,
		me.TenantID, tasks["later"], tasks["later"], me.UserID)
	v, err := e.svc.MyWork(bg, me, WorkParams{})
	if err != nil || v.Counts.Watching != 2 || v.Counts.Mentions != 1 || len(v.Groups[GroupWatching]) != 2 || len(v.Groups[GroupMentions]) != 1 {
		t.Fatalf("watching/mentions: %+v %v", v.Counts, err)
	}
	page, err := e.svc.MyWork(bg, me, WorkParams{Group: GroupWatching, PerGroup: 1})
	if err != nil || len(page.Groups[GroupWatching]) != 1 || page.NextCursor == nil {
		t.Fatalf("watching page: %+v %v", page, err)
	}
	next, err := e.svc.MyWork(bg, me, WorkParams{Group: GroupWatching, PerGroup: 1, Cursor: *page.NextCursor})
	if err != nil || len(next.Groups[GroupWatching]) != 1 || next.Groups[GroupWatching][0].ID == page.Groups[GroupWatching][0].ID {
		t.Fatalf("watching second page: %+v %v", next, err)
	}
	badge, _ := e.svc.MyWorkCounts(bg, me)
	if badge.Watching != 2 || badge.Mentions != 1 {
		t.Fatalf("badge: %+v", badge)
	}
	_, err = e.svc.MyWork(bg, me, WorkParams{Group: GroupMentions, Cursor: "%"})
	wantCode(t, err, "VALIDATION_ERROR")
}

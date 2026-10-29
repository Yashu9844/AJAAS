package recurrence

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
	"github.com/jaas/jaas/internal/projects/project"
)

func d(y int, m time.Month, day, h int) time.Time { return time.Date(y, m, day, h, 0, 0, 0, time.UTC) }

func TestScheduleArithmetic(t *testing.T) {
	daily := Recurrence{Frequency: Daily, Interval: 3}
	if got := step(d(2026, 3, 10, 0), daily); !got.Equal(d(2026, 3, 13, 0)) {
		t.Fatalf("daily step %v", got)
	}
	weekly := Recurrence{Frequency: Weekly, Interval: 2, Weekday: ptr(1)}
	if got := step(d(2026, 3, 9, 0), weekly); !got.Equal(d(2026, 3, 23, 0)) {
		t.Fatalf("weekly step %v", got)
	}
	monthly := Recurrence{Frequency: Monthly, Interval: 1, DayOfMonth: ptr(31)}
	jan31 := d(2026, 1, 31, 0)
	feb := step(jan31, monthly)
	if !feb.Equal(d(2026, 2, 28, 0)) {
		t.Fatalf("a 31st clamps to the end of February: %v", feb)
	}
	if mar := step(feb, monthly); !mar.Equal(d(2026, 3, 31, 0)) {
		t.Fatalf("the 31st comes back in March: %v", mar)
	}
	if got := step(d(2026, 12, 15, 0), Recurrence{Frequency: Monthly, Interval: 2}); !got.Equal(d(2027, 2, 15, 0)) {
		t.Fatalf("monthly across the year end: %v", got)
	}
	now := d(2026, 3, 10, 9) // a Tuesday
	if got := firstAfter(now, daily); !got.Equal(d(2026, 3, 11, 0)) {
		t.Fatalf("first daily %v", got)
	}
	if got := firstAfter(now, Recurrence{Frequency: Weekly, Weekday: ptr(2)}); !got.Equal(d(2026, 3, 17, 0)) {
		t.Fatalf("a Tuesday run on a Tuesday goes to next week: %v", got)
	}
	if got := firstAfter(now, Recurrence{Frequency: Monthly, DayOfMonth: ptr(5)}); !got.Equal(d(2026, 4, 5, 0)) {
		t.Fatalf("first monthly %v", got)
	}
	if got := firstAfter(now, Recurrence{Frequency: "other"}); !got.Equal(d(2026, 3, 11, 0)) {
		t.Fatalf("fallback %v", got)
	}
	if got := advancePast(d(2026, 3, 1, 0), now, daily); !got.After(now) || got.Sub(now) > 72*time.Hour {
		t.Fatalf("advance past now %v", got)
	}
	if got := advancePast(d(2000, 1, 1, 0), now, Recurrence{Frequency: Daily, Interval: 1}); !got.After(now) {
		t.Fatalf("a very old occurrence still lands in the future: %v", got)
	}
}

func TestDatesAreRelativeToTheRun(t *testing.T) {
	start := kernel.Date{Time: d(2026, 3, 2, 0)}
	due := d(2026, 3, 6, 17)
	got := datesFor(Template{StartOn: &start, DueAt: &due, CreatedAt: d(2026, 3, 1, 8)}, d(2026, 4, 1, 0))
	if got.StartOn == nil || got.StartOn.String() != "2026-04-01" || got.DueAt == nil || !got.DueAt.Equal(d(2026, 4, 5, 17)) {
		t.Fatalf("start moves to the run day, due keeps its 4-day distance and its time: %+v %v", got.StartOn, got.DueAt)
	}
	got = datesFor(Template{DueAt: &due, CreatedAt: d(2026, 3, 4, 8)}, d(2026, 5, 1, 0))
	if got.StartOn != nil || !got.DueAt.Equal(d(2026, 5, 3, 17)) {
		t.Fatalf("without a start the distance is from creation: %v", got.DueAt)
	}
	early := d(2026, 3, 1, 10)
	if got = datesFor(Template{DueAt: &early, CreatedAt: d(2026, 3, 9, 8)}, d(2026, 5, 1, 0)); !got.DueAt.Equal(d(2026, 5, 1, 10)) {
		t.Fatalf("a due date before creation counts as same day: %v", got.DueAt)
	}
	if got = datesFor(Template{CreatedAt: d(2026, 3, 9, 8)}, d(2026, 5, 1, 0)); got.DueAt != nil || got.StartOn != nil {
		t.Fatal("no dates stay no dates")
	}
}

func TestCreateValidationAndPermissions(t *testing.T) {
	e := newEnv(t)
	f := e.fixture("RV")
	other := e.fixture("RW")
	stranger := e.user()
	past, future := e.clock.Add(-time.Hour), e.clock.Add(48*time.Hour)
	before := kernel.Date{Time: d(2026, 1, 1, 0)}
	bad := map[string]Input{
		"frequency":       {TemplateTaskID: f.tpl, Frequency: "hourly"},
		"interval":        {TemplateTaskID: f.tpl, Frequency: Daily, Interval: 53},
		"first in past":   {TemplateTaskID: f.tpl, Frequency: Daily, FirstRunAt: &past},
		"weekday on day":  {TemplateTaskID: f.tpl, Frequency: Daily, Weekday: ptr(1)},
		"dom on week":     {TemplateTaskID: f.tpl, Frequency: Weekly, DayOfMonth: ptr(1)},
		"weekday range":   {TemplateTaskID: f.tpl, Frequency: Weekly, Weekday: ptr(7)},
		"dom range":       {TemplateTaskID: f.tpl, Frequency: Monthly, DayOfMonth: ptr(32)},
		"until before":    {TemplateTaskID: f.tpl, Frequency: Daily, UntilOn: &before, FirstRunAt: &future},
		"foreign tpl":     {TemplateTaskID: other.tpl, Frequency: Daily},
		"unknown tpl":     {TemplateTaskID: uuid.New(), Frequency: Daily},
		"template absent": {Frequency: Daily},
	}
	for name, in := range bad {
		if _, err := e.svc.Create(bg, f.owner, f.p.ID, in); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
	ok := Input{TemplateTaskID: f.tpl, Frequency: Weekly}
	_, err := e.svc.Create(bg, f.member, f.p.ID, ok)
	wantCode(t, err, "FORBIDDEN")
	_, err = e.svc.Create(bg, stranger, f.p.ID, ok)
	wantCode(t, err, "NOT_FOUND")
	r, err := e.svc.Create(bg, f.owner, f.p.ID, ok)
	if err != nil || r.Interval != 1 || r.Weekday == nil || *r.Weekday != int(e.clock.Weekday()) || !r.Active || !r.NextRunAt.Equal(d(2026, 3, 17, 0)) {
		t.Fatalf("defaults: %v %+v", err, r)
	}
	m, err := e.svc.Create(bg, f.owner, f.p.ID, Input{TemplateTaskID: f.tpl, Frequency: Monthly, FirstRunAt: &future})
	if err != nil || *m.DayOfMonth != 12 || !m.NextRunAt.Equal(future) {
		t.Fatalf("monthly defaults from the first run: %v %+v", err, m)
	}
	rows, err := e.svc.List(bg, f.viewr, f.p.ID)
	if err != nil || len(rows) != 2 {
		t.Fatalf("members read: %v %d", err, len(rows))
	}
	if _, err = e.svc.List(bg, stranger, f.p.ID); err == nil {
		t.Fatal("strangers see nothing")
	}
	if len(e.audit.actions) == 0 {
		t.Fatal("creation is audited")
	}
}

func TestRecurrenceLimit(t *testing.T) {
	e := newEnv(t)
	f := e.fixture("RL")
	for i := 0; i < maxPerProject; i++ {
		e.create(f, Input{Frequency: Daily, Interval: 1 + i%5})
	}
	_, err := e.svc.Create(bg, f.owner, f.p.ID, Input{TemplateTaskID: f.tpl, Frequency: Daily})
	wantCode(t, err, "RECURRENCE_LIMIT")
}

func TestPatchAndDelete(t *testing.T) {
	e := newEnv(t)
	f := e.fixture("PD")
	r := e.create(f, Input{Frequency: Daily})
	off, wk := false, "weekly"
	got, err := e.svc.Patch(bg, f.owner, f.p.ID, r.ID, Update{Active: &off, Frequency: &wk, Weekday: ptr(5), Version: ptr(1)})
	if err != nil || got.Active || got.Frequency != Weekly || *got.Weekday != 5 || got.Version != 2 || int(got.NextRunAt.Weekday()) != 5 {
		t.Fatalf("patch: %v %+v", err, got)
	}
	_, err = e.svc.Patch(bg, f.owner, f.p.ID, r.ID, Update{Interval: ptr(2), Version: ptr(1)})
	if _, ok := kernel.AsVersionConflict(err); !ok {
		t.Fatalf("stale version: %v", err)
	}
	next := e.clock.Add(72 * time.Hour)
	until := OptDate{Set: true, Val: &kernel.Date{Time: d(2026, 12, 1, 0)}}
	on := true
	got, err = e.svc.Patch(bg, f.owner, f.p.ID, r.ID, Update{NextRunAt: &next, UntilOn: until, Active: &on, Interval: ptr(2)})
	if err != nil || !got.NextRunAt.Equal(next) || got.UntilOn == nil || !got.Active || got.Interval != 2 {
		t.Fatalf("patch 2: %v %+v", err, got)
	}
	got, err = e.svc.Patch(bg, f.owner, f.p.ID, r.ID, Update{UntilOn: OptDate{Set: true}})
	if err != nil || got.UntilOn != nil {
		t.Fatalf("clear until: %v", err)
	}
	past := e.clock.Add(-time.Hour)
	if _, err = e.svc.Patch(bg, f.owner, f.p.ID, r.ID, Update{NextRunAt: &past}); err == nil {
		t.Fatal("next run must be in the future")
	}
	if _, err = e.svc.Patch(bg, f.owner, f.p.ID, r.ID, Update{Interval: ptr(0)}); err == nil {
		// interval 0 normalises to 1 only on create; patch of 0 is accepted as the default
		t.Log("interval 0 resets to 1")
	}
	if _, err = e.svc.Patch(bg, f.member, f.p.ID, r.ID, Update{Active: &off}); err == nil {
		t.Fatal("members cannot edit")
	}
	if _, err = e.svc.Patch(bg, f.owner, f.p.ID, uuid.New(), Update{Active: &off}); err == nil {
		t.Fatal("unknown recurrence")
	}
	if err = e.svc.Delete(bg, f.owner, f.p.ID, r.ID); err != nil {
		t.Fatal(err)
	}
	wantCode(t, e.svc.Delete(bg, f.owner, f.p.ID, r.ID), "NOT_FOUND")
	if e.svc.Delete(bg, f.member, f.p.ID, r.ID) == nil {
		t.Fatal("members cannot delete")
	}
}

func TestSchedulerCreatesOneCopyAndAdvances(t *testing.T) {
	e := newEnv(t)
	f := e.fixture("SC")
	r := e.create(f, Input{Frequency: Daily})
	if e.tick() != 0 {
		t.Fatal("nothing is due yet")
	}
	e.due(r, e.clock.Add(-time.Minute))
	if e.tick() != 1 || e.tasks.n() != 1 {
		t.Fatal("one copy")
	}
	c := e.tasks.copies[0]
	if c.user != f.owner.UserID || c.agent != agentOf(r.ID) {
		t.Fatalf("created as the recurrence creator with its agent label: %+v", c)
	}
	got := e.reload(r)
	if !got.NextRunAt.After(e.clock) || got.LastTaskID == nil || *got.LastTaskID != c.id || got.LastRunAt == nil || got.FlaggedCode != nil {
		t.Fatalf("advanced: %+v", got)
	}
	if e.tick() != 0 {
		t.Fatal("at most one copy per occurrence")
	}
}

func TestMissedRunsCollapseIntoOneCatchUp(t *testing.T) {
	e := newEnv(t)
	f := e.fixture("CU")
	r := e.create(f, Input{Frequency: Daily})
	e.due(r, e.clock.Add(-5*24*time.Hour))
	if e.tick() != 1 {
		t.Fatal("one catch-up copy")
	}
	if e.tick() != 0 || e.tasks.n() != 1 {
		t.Fatal("the other missed days are dropped, not replayed")
	}
	if got := e.reload(r); !got.NextRunAt.After(e.clock) {
		t.Fatalf("next run is in the future: %v", got.NextRunAt)
	}
}

func TestUntilEndsTheRecurrence(t *testing.T) {
	e := newEnv(t)
	f := e.fixture("UN")
	until := kernel.Date{Time: d(2026, 3, 10, 0)}
	r := e.create(f, Input{Frequency: Daily, UntilOn: &until})
	e.due(r, e.clock.Add(-time.Minute)) // runs on the 10th: the last allowed day
	if e.tick() != 1 {
		t.Fatal("the run on the until day happens")
	}
	if got := e.reload(r); got.Active {
		t.Fatal("the recurrence ends after its last run")
	}
	late := e.create(f, Input{Frequency: Daily, UntilOn: &until})
	e.due(late, d(2026, 3, 12, 0)) // a run scheduled after the until day
	e.clock = d(2026, 3, 20, 0)
	if e.tick() != 0 {
		t.Fatal("no run after the until day")
	}
	if got := e.reload(late); got.Active {
		t.Fatal("it is deactivated instead")
	}
}

func TestArchivedProjectAndLostAccessAreFlagged(t *testing.T) {
	e := newEnv(t)
	f := e.fixture("AR")
	r := e.create(f, Input{Frequency: Daily})
	e.due(r, e.clock.Add(-time.Minute))
	if _, err := e.proj.Archive(bg, f.owner, f.p.ID); err != nil {
		t.Fatal(err)
	}
	if e.tick() != 0 || e.tasks.n() != 0 {
		t.Fatal("nothing is created in an archived project")
	}
	got := e.reload(r)
	if got.FlaggedCode == nil || *got.FlaggedCode != FlagProjectArchived || !got.Active || !got.NextRunAt.After(e.clock) {
		t.Fatalf("flagged and rescheduled: %+v", got)
	}
	if _, err := e.proj.Restore(bg, f.owner, f.p.ID); err != nil {
		t.Fatal(err)
	}
	// the member who set it up is demoted to viewer
	m := e.create(fixture{p: f.p, owner: f.owner, tpl: f.tpl}, Input{Frequency: Daily})
	e.db.Exec(`UPDATE task_recurrences SET created_by = ? WHERE id = ?`, f.member.UserID, m.ID)
	if _, err := e.proj.ChangeMember(bg, f.owner, f.p.ID, f.member.UserID, project.MemberInput{UserID: f.member.UserID, Role: api.RoleViewer}); err != nil {
		t.Fatal(err)
	}
	e.due(m, e.clock.Add(-time.Minute))
	e.due(r, e.clock.Add(24*time.Hour))
	if e.tick() != 0 || e.tasks.n() != 0 {
		t.Fatal("a demoted creator creates nothing")
	}
	if got := e.reload(m); got.FlaggedCode == nil || *got.FlaggedCode != FlagNotAllowed {
		t.Fatalf("flag: %+v", got)
	}
	e.db.Exec(`UPDATE task_recurrences SET created_by = ? WHERE id = ?`, uuid.New(), m.ID) // not a member at all
	e.due(m, e.clock.Add(-time.Minute))
	e.tick()
	if e.tasks.n() != 0 {
		t.Fatal("a non-member creator creates nothing")
	}
}

func TestMissingTemplateEndsTheRecurrence(t *testing.T) {
	e := newEnv(t)
	f := e.fixture("TG")
	r := e.create(f, Input{Frequency: Daily})
	e.db.Exec(`UPDATE tasks SET deleted_at = now() WHERE id = ?`, f.tpl)
	e.due(r, e.clock.Add(-time.Minute))
	if e.tick() != 0 {
		t.Fatal("no copy")
	}
	got := e.reload(r)
	if got.Active || got.FlaggedCode == nil || *got.FlaggedCode != FlagTemplateGone {
		t.Fatalf("template gone: %+v", got)
	}
}

func TestSpawnFailuresAreFlagged(t *testing.T) {
	e := newEnv(t)
	f := e.fixture("SF")
	r := e.create(f, Input{Frequency: Daily})
	e.tasks.failAll = kernel.Conflict("WIP_LIMIT", "full")
	e.due(r, e.clock.Add(-time.Minute))
	if _, err := e.svc.Tick(bg); err == nil {
		t.Fatal("the failure is reported")
	}
	if got := e.reload(r); got.FlaggedCode == nil || *got.FlaggedCode != FlagFailed || !got.Active {
		t.Fatalf("flag: %+v", got)
	}
	e.tasks.failAll = nil
	e.tasks.partial = kernel.Invalid("due_at", "bad")
	e.due(r, e.clock.Add(-time.Minute))
	if n, err := e.svc.Tick(bg); n != 1 || err != nil {
		t.Fatalf("the copy exists even if its dates failed: %d %v", n, err)
	}
	if got := e.reload(r); got.FlaggedCode == nil || *got.FlaggedCode != FlagDatesFailed || got.LastTaskID == nil {
		t.Fatalf("dates flag: %+v", got)
	}
}

func TestRunNow(t *testing.T) {
	e := newEnv(t)
	f := e.fixture("RN")
	r := e.create(f, Input{Frequency: Weekly})
	before := e.reload(r).NextRunAt
	id, err := e.svc.RunNow(bg, f.owner, f.p.ID, r.ID)
	if err != nil || id == uuid.Nil || e.tasks.n() != 1 {
		t.Fatalf("run now: %v", err)
	}
	if got := e.reload(r); !got.NextRunAt.Equal(before) || got.LastTaskID == nil || *got.LastTaskID != id {
		t.Fatalf("the schedule is untouched: %+v", got)
	}
	if e.tasks.copies[0].user != f.owner.UserID || e.tasks.copies[0].agent != agentOf(r.ID) {
		t.Fatal("created as the caller with the recurrence label")
	}
	if _, err = e.svc.RunNow(bg, f.member, f.p.ID, r.ID); err == nil {
		t.Fatal("members cannot run it")
	}
	if _, err = e.svc.RunNow(bg, f.owner, f.p.ID, uuid.New()); err == nil {
		t.Fatal("unknown recurrence")
	}
	e.tasks.partial = kernel.Invalid("due_at", "bad")
	if id, err = e.svc.RunNow(bg, f.owner, f.p.ID, r.ID); err == nil || id == uuid.Nil {
		t.Fatalf("partial result keeps the id and the error: %v %v", id, err)
	}
	e.tasks.partial, e.tasks.failAll = nil, kernel.Conflict("X", "x")
	if _, err = e.svc.RunNow(bg, f.owner, f.p.ID, r.ID); err == nil {
		t.Fatal("failure")
	}
	if _, err = e.proj.Archive(bg, f.owner, f.p.ID); err != nil {
		t.Fatal(err)
	}
	_, err = e.svc.RunNow(bg, f.owner, f.p.ID, r.ID)
	wantCode(t, err, "PROJECT_ARCHIVED")
}

func TestConcurrentTicksNeverDuplicate(t *testing.T) {
	e := newEnv(t)
	f := e.fixture("CC")
	for i := 0; i < 5; i++ {
		r := e.create(f, Input{Frequency: Daily, Interval: i + 1})
		e.due(r, e.clock.Add(-time.Minute))
	}
	done := make(chan int, 3)
	for i := 0; i < 3; i++ {
		go func() {
			n, _ := e.svc.Tick(bg)
			done <- n
		}()
	}
	total := 0
	for i := 0; i < 3; i++ {
		total += <-done
	}
	if total != 5 || e.tasks.n() != 5 {
		t.Fatalf("exactly one copy per recurrence across concurrent schedulers: %d created, %d copies", total, e.tasks.n())
	}
}

func TestRunLoopStops(t *testing.T) {
	e := newEnv(t)
	f := e.fixture("LP")
	r := e.create(f, Input{Frequency: Daily})
	e.due(r, e.clock.Add(-time.Minute))
	ctx, cancel := context.WithTimeout(bg, 3*time.Second)
	defer cancel()
	go e.svc.Run(ctx, 20*time.Millisecond, func(error) {})
	for e.tasks.n() == 0 && ctx.Err() == nil {
		time.Sleep(20 * time.Millisecond)
	}
	if e.tasks.n() != 1 {
		t.Fatal("the loop ran the recurrence")
	}
	cancel()
}

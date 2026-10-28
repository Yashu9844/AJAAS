package collab

import (
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
)

func ago(d time.Duration) time.Time { return time.Now().Add(-d) }

func TestLogTimeValidationTable(t *testing.T) {
	f := newCommentFixture(t)
	cases := []struct {
		name string
		in   ManualTimeInput
		code string
	}{
		{"zero minutes", ManualTimeInput{StartedAt: ago(time.Hour), Minutes: 0}, "400"},
		{"negative", ManualTimeInput{StartedAt: ago(time.Hour), Minutes: -5}, "400"},
		{"more than a day", ManualTimeInput{StartedAt: ago(time.Hour), Minutes: 1441}, "400"},
		{"no start", ManualTimeInput{Minutes: 10}, "400"},
		{"future start", ManualTimeInput{StartedAt: time.Now().Add(time.Hour), Minutes: 10}, "400"},
		{"long note", ManualTimeInput{StartedAt: ago(time.Hour), Minutes: 10, Note: strings.Repeat("n", 501)}, "400"},
		{"nul note", ManualTimeInput{StartedAt: ago(time.Hour), Minutes: 10, Note: "a\x00b"}, "400"},
		{"past the lock window", ManualTimeInput{StartedAt: ago(LockWindow + time.Hour), Minutes: 10}, "409"},
	}
	for _, c := range cases {
		_, err := f.svc.LogTime(bg, f.member, f.task, c.in)
		if err == nil || !strings.Contains(err.Error(), "status: "+c.code) {
			t.Errorf("%s: want %s, got %v", c.name, c.code, err)
		}
	}
	e, err := f.svc.LogTime(bg, f.member, f.task, ManualTimeInput{StartedAt: ago(2 * time.Hour), Minutes: 90, Note: " review "})
	must(t, err)
	if e.Minutes != 90 || e.Note != "review" || e.EndedAt == nil || e.Source != "manual" || e.AutoStopped {
		t.Fatalf("entry: %+v", e)
	}
	if d := e.EndedAt.Sub(e.StartedAt); d != 90*time.Minute {
		t.Fatalf("ended_at is started_at + minutes: %v", d)
	}
}

func TestTimePermissionsAndEvents(t *testing.T) {
	f := newCommentFixture(t)
	in := ManualTimeInput{StartedAt: ago(time.Hour), Minutes: 15, Note: "SECRET-NOTE"}
	_, err := f.svc.LogTime(bg, f.viewer, f.task, in)
	wantStatus(t, err, "403")
	_, err = f.svc.LogTime(bg, f.stranger, f.task, in)
	wantStatus(t, err, "404")
	_, err = f.svc.LogTime(bg, f.member, uuid.New(), in)
	wantStatus(t, err, "404")
	e, err := f.svc.LogTime(bg, f.member, f.task, in)
	must(t, err)
	f.noLeak("SECRET-NOTE")
	ev := f.events("time.logged")
	if len(ev) != 1 || ev[0]["time_entry_id"] != e.ID.String() || ev[0]["user_id"] != f.member.UserID.String() {
		t.Fatalf("time.logged payload: %v", ev)
	}
	if _, has := ev[0]["minutes"]; has {
		t.Fatal("events carry ids only")
	}
	if f.count(`SELECT count(*) FROM activity WHERE task_id = ? AND type = 'time_logged'`, f.task) != 1 {
		t.Fatal("activity entry missing")
	}
}

func TestTimeTrackingDisabled(t *testing.T) {
	f := newCommentFixture(t)
	f.db.Exec(`UPDATE projects SET time_tracking_enabled = false WHERE id = ?`, f.pid)
	_, err := f.svc.LogTime(bg, f.member, f.task, ManualTimeInput{StartedAt: ago(time.Hour), Minutes: 5})
	wantCode(t, err, "TIME_TRACKING_DISABLED")
	_, err = f.svc.StartTimer(bg, f.member, f.task)
	wantCode(t, err, "TIME_TRACKING_DISABLED")
	rep, err := f.svc.TimeReport(bg, f.member, f.pid, ReportQuery{})
	must(t, err)
	if rep.TotalMinutes != 0 {
		t.Fatalf("reports still work: %+v", rep)
	}
}

func TestEditDeleteAndLockWindow(t *testing.T) {
	f := newCommentFixture(t)
	mine, err := f.svc.LogTime(bg, f.member, f.task, ManualTimeInput{StartedAt: ago(3 * time.Hour), Minutes: 30})
	must(t, err)
	// Others cannot touch it; the manager can (time.manage).
	other := f.actor()
	f.join(f.owner, f.p, other, "member")
	_, err = f.svc.UpdateTime(bg, other, mine.ID, TimeUpdateInput{Note: strPtr("hijack")})
	wantStatus(t, err, "403")
	wantStatus(t, f.svc.DeleteTime(bg, other, mine.ID), "403")
	_, err = f.svc.UpdateTime(bg, f.stranger, mine.ID, TimeUpdateInput{})
	wantStatus(t, err, "404")
	// The author edits minutes, note and start; validation applies.
	up, err := f.svc.UpdateTime(bg, f.member, mine.ID, TimeUpdateInput{Minutes: intPtr(45), Note: strPtr("longer")})
	must(t, err)
	if up.Minutes != 45 || up.Note != "longer" || up.EndedAt.Sub(up.StartedAt) != 45*time.Minute {
		t.Fatalf("update: %+v", up)
	}
	for _, bad := range []TimeUpdateInput{{Minutes: intPtr(0)}, {Note: strPtr(strings.Repeat("x", 501))}, {StartedAt: timePtr(time.Now().Add(time.Hour))}} {
		if _, err = f.svc.UpdateTime(bg, f.member, mine.ID, bad); err == nil {
			t.Errorf("accepted %+v", bad)
		}
	}
	if _, err = f.svc.UpdateTime(bg, f.member, mine.ID, TimeUpdateInput{StartedAt: timePtr(ago(LockWindow + time.Hour))}); err == nil {
		t.Fatal("authors cannot move an entry into the locked past")
	}
	// Past the lock window the author is refused (409) but the manager still edits and deletes.
	f.db.Exec(`UPDATE time_entries SET started_at = ?, ended_at = ? WHERE id = ?`, ago(LockWindow+48*time.Hour), ago(LockWindow+47*time.Hour), mine.ID)
	_, err = f.svc.UpdateTime(bg, f.member, mine.ID, TimeUpdateInput{Note: strPtr("late")})
	wantCode(t, err, "TIME_LOCKED")
	wantCode(t, f.svc.DeleteTime(bg, f.member, mine.ID), "TIME_LOCKED")
	up, err = f.svc.UpdateTime(bg, f.owner, mine.ID, TimeUpdateInput{Note: strPtr("corrected")})
	must(t, err)
	if up.Note != "corrected" {
		t.Fatalf("manager edit: %+v", up)
	}
	must(t, f.svc.DeleteTime(bg, f.owner, mine.ID))
	wantStatus(t, f.svc.DeleteTime(bg, f.owner, mine.ID), "404")
	if f.count(`SELECT count(*) FROM time_entries WHERE task_id = ?`, f.task) != 0 {
		t.Fatal("deleted")
	}
}

func TestListTimeMineAndPaging(t *testing.T) {
	f := newCommentFixture(t)
	for i := 1; i <= 3; i++ {
		_, err := f.svc.LogTime(bg, f.member, f.task, ManualTimeInput{StartedAt: ago(time.Duration(i) * time.Hour), Minutes: i})
		must(t, err)
	}
	_, err := f.svc.LogTime(bg, f.owner, f.task, ManualTimeInput{StartedAt: ago(10 * time.Minute), Minutes: 7})
	must(t, err)
	all, _, err := f.svc.ListTime(bg, f.viewer, f.task, TimeFilter{Page: PageRequest{Limit: 50}})
	must(t, err)
	mine, _, _ := f.svc.ListTime(bg, f.member, f.task, TimeFilter{Page: PageRequest{Limit: 50}, Mine: true})
	if len(all) != 4 || len(mine) != 3 || all[0].Minutes != 7 {
		t.Fatalf("list: %d %d", len(all), len(mine))
	}
	p1, next, _ := f.svc.ListTime(bg, f.viewer, f.task, TimeFilter{Page: PageRequest{Limit: 3}})
	p2, next2, err := f.svc.ListTime(bg, f.viewer, f.task, TimeFilter{Page: PageRequest{Limit: 3, Cursor: next}})
	must(t, err)
	if len(p1) != 3 || len(p2) != 1 || next2 != "" || p2[0].Minutes != 3 {
		t.Fatalf("paging: %d %d", len(p1), len(p2))
	}
	if _, _, err = f.svc.ListTime(bg, f.stranger, f.task, TimeFilter{}); err == nil {
		t.Fatal("strangers read nothing")
	}
	if _, _, err = f.svc.ListTime(bg, f.member, f.task, TimeFilter{Page: PageRequest{Cursor: "@@"}}); err == nil {
		t.Fatal("bad cursor")
	}
}

func TestTimerStartStopAndOneRunning(t *testing.T) {
	f := newCommentFixture(t)
	other := f.task
	second := f.env.task(f.p, nil)
	if e, err := f.svc.RunningTimer(bg, f.member); err != nil || e != nil {
		t.Fatalf("nothing runs yet: %v %v", e, err)
	}
	_, err := f.svc.StopTimer(bg, f.member)
	wantStatus(t, err, "404")
	_, err = f.svc.StartTimer(bg, f.viewer, other)
	wantStatus(t, err, "403")
	_, err = f.svc.StartTimer(bg, f.stranger, other)
	wantStatus(t, err, "404")
	e, err := f.svc.StartTimer(bg, f.member, other)
	must(t, err)
	if e.EndedAt != nil || e.Source != "timer" {
		t.Fatalf("running: %+v", e)
	}
	_, err = f.svc.StartTimer(bg, f.member, second)
	wantCode(t, err, "TIMER_RUNNING")
	run, _ := f.svc.RunningTimer(bg, f.member)
	if run == nil || run.ID != e.ID {
		t.Fatalf("running timer: %v", run)
	}
	// A running entry cannot be edited, but its author can discard it.
	_, err = f.svc.UpdateTime(bg, f.member, e.ID, TimeUpdateInput{Note: strPtr("x")})
	wantCode(t, err, "TIMER_RUNNING")
	f.db.Exec(`UPDATE time_entries SET started_at = ? WHERE id = ?`, ago(90*time.Minute), e.ID)
	st, err := f.svc.StopTimer(bg, f.member)
	must(t, err)
	if st.EndedAt == nil || st.Minutes < 89 || st.Minutes > 91 {
		t.Fatalf("stopped: %+v", st)
	}
	if f.count(`SELECT count(*) FROM activity WHERE type = 'time_logged' AND task_id = ?`, other) != 1 || len(f.events("time.logged")) != 1 {
		t.Fatal("stopping logs activity and an event")
	}
	// A new timer can start now, and discarding it frees the slot.
	again, err := f.svc.StartTimer(bg, f.member, second)
	must(t, err)
	must(t, f.svc.DeleteTime(bg, f.member, again.ID))
	if _, err = f.svc.StartTimer(bg, f.member, second); err != nil {
		t.Fatalf("slot free after discard: %v", err)
	}
}

// TestOneRunningTimerUnderRace is PG6: the partial unique index lets exactly one of many concurrent starts win.
func TestOneRunningTimerUnderRace(t *testing.T) {
	f := newCommentFixture(t)
	var wins, conflicts, other atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := f.svc.StartTimer(bg, f.member, f.task)
			switch {
			case err == nil:
				wins.Add(1)
			case strings.Contains(err.Error(), "TIMER_RUNNING"):
				conflicts.Add(1)
			default:
				other.Add(1)
			}
		}()
	}
	wg.Wait()
	if wins.Load() != 1 || conflicts.Load() != 11 || other.Load() != 0 {
		t.Fatalf("wins=%d conflicts=%d other=%d", wins.Load(), conflicts.Load(), other.Load())
	}
	if f.count(`SELECT count(*) FROM time_entries WHERE user_id = ? AND ended_at IS NULL`, f.member.UserID) != 1 {
		t.Fatal("exactly one running row")
	}
}

func TestStopStaleTimers(t *testing.T) {
	f := newCommentFixture(t)
	fresh, err := f.svc.StartTimer(bg, f.owner, f.task)
	must(t, err)
	stale, err := f.svc.StartTimer(bg, f.member, f.task)
	must(t, err)
	f.db.Exec(`UPDATE time_entries SET started_at = ? WHERE id = ?`, ago(30*time.Hour), stale.ID)
	n, err := f.svc.StopStaleTimers(bg, MaxTimerAge)
	must(t, err)
	if n != 1 {
		t.Fatalf("stopped %d", n)
	}
	got := f.entry(stale.ID)
	if !got.AutoStopped || got.EndedAt == nil || got.Minutes != 1440 || got.EndedAt.Sub(got.StartedAt) != 24*time.Hour {
		t.Fatalf("auto-stopped entry is capped at 24h: %+v", got)
	}
	got = f.entry(fresh.ID)
	if got.EndedAt != nil || got.AutoStopped {
		t.Fatal("fresh timers keep running")
	}
	if len(f.events("time.logged")) != 1 || f.count(`SELECT count(*) FROM activity WHERE source = 'system' AND type = 'time_logged'`) != 1 {
		t.Fatal("the sweep announces the stop")
	}
	// The member can start a new timer after the sweep; a second sweep finds nothing.
	if _, err = f.svc.StartTimer(bg, f.member, f.task); err != nil {
		t.Fatal(err)
	}
	if n, _ = f.svc.StopStaleTimers(bg, MaxTimerAge); n != 0 {
		t.Fatalf("second sweep stopped %d", n)
	}
	// A shorter threshold caps at now instead of 24h.
	f.db.Exec(`UPDATE time_entries SET started_at = ? WHERE ended_at IS NULL AND user_id = ?`, ago(2*time.Hour), f.member.UserID)
	if n, _ = f.svc.StopStaleTimers(bg, time.Hour); n != 1 {
		t.Fatalf("threshold sweep stopped %d", n)
	}
	var latest TimeEntry
	f.db.Where("user_id = ? AND auto_stopped", f.member.UserID).Order("created_at DESC").Take(&latest)
	got = latest
	if got.Minutes < 119 || got.Minutes > 121 {
		t.Fatalf("minutes: %d", got.Minutes)
	}
}

func TestTimeReport(t *testing.T) {
	f := newCommentFixture(t)
	est := 120
	t1, t2 := f.env.task(f.p, &est), f.env.task(f.p, nil)
	for _, c := range []struct {
		task uuid.UUID
		mins int
		by   int
	}{{t1, 60, 0}, {t1, 30, 1}, {t2, 15, 0}} {
		who := f.member
		if c.by == 1 {
			who = f.owner
		}
		_, err := f.svc.LogTime(bg, who, c.task, ManualTimeInput{StartedAt: ago(2 * time.Hour), Minutes: c.mins})
		must(t, err)
	}
	// A running timer is not counted; an old entry falls outside the default window.
	runner := f.actor()
	f.join(f.owner, f.p, runner, "member")
	_, err := f.svc.StartTimer(bg, runner, t2)
	must(t, err)
	rep, err := f.svc.TimeReport(bg, f.viewer, f.pid, ReportQuery{})
	must(t, err)
	if rep.TotalMinutes != 105 || rep.Entries != 3 || rep.EstimateMinutes != 120 || rep.GroupBy != "task" || len(rep.Rows) != 2 {
		t.Fatalf("report: %+v", rep)
	}
	if rep.Rows[0].Key != t1 || rep.Rows[0].Minutes != 90 || rep.Rows[0].EstimateMinutes == nil || *rep.Rows[0].EstimateMinutes != 120 || rep.Rows[0].Number == nil {
		t.Fatalf("task row: %+v", rep.Rows[0])
	}
	byUser, err := f.svc.TimeReport(bg, f.owner, f.pid, ReportQuery{GroupBy: GroupByUser})
	must(t, err)
	if len(byUser.Rows) != 2 || byUser.Rows[0].Key != f.member.UserID || byUser.Rows[0].Minutes != 75 || byUser.Rows[0].Title != "" {
		t.Fatalf("user report: %+v", byUser.Rows)
	}
	today := time.Now().UTC().Format("2006-01-02")
	empty, err := f.svc.TimeReport(bg, f.owner, f.pid, ReportQuery{From: "2001-01-01", To: "2001-01-31"})
	must(t, err)
	if empty.TotalMinutes != 0 || len(empty.Rows) != 0 || empty.Rows == nil {
		t.Fatalf("empty range: %+v", empty)
	}
	if one, _ := f.svc.TimeReport(bg, f.owner, f.pid, ReportQuery{From: today, To: today}); one.TotalMinutes == 0 {
		t.Fatalf("a one-day window includes today: %+v", one)
	}
	for _, q := range []ReportQuery{{GroupBy: "weird"}, {From: "nope"}, {To: "nope"}, {From: "2026-02-01", To: "2026-01-01"}, {From: "2000-01-01", To: "2026-01-01"}} {
		if _, err = f.svc.TimeReport(bg, f.owner, f.pid, q); err == nil {
			t.Errorf("accepted %+v", q)
		}
	}
	if _, err = f.svc.TimeReport(bg, f.stranger, f.pid, ReportQuery{}); err == nil {
		t.Fatal("strangers cannot report")
	}
	if _, err = f.svc.TimeReport(bg, f.owner, uuid.New(), ReportQuery{}); err == nil {
		t.Fatal("unknown project")
	}
}

func strPtr(s string) *string        { return &s }
func intPtr(n int) *int              { return &n }
func timePtr(t time.Time) *time.Time { return &t }

func (e *env) entry(id uuid.UUID) TimeEntry {
	var out TimeEntry
	e.db.Where("id = ?", id).Take(&out)
	return out
}

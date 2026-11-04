package integration

import (
	"strings"
	"testing"
	"time"
)

// startedMeeting creates a single meeting that started five minutes ago, with the note-taker and the attendee.
func (c *meetingCast) startedMeeting(t *testing.T, extra map[string]any) (seriesID, occurrenceID string) {
	t.Helper()
	start := time.Now().UTC().Add(-5 * time.Minute)
	b := map[string]any{"title": "Ops sync", "start": start.Format("2006-01-02T15:04"), "tz": "UTC", "duration_min": 60,
		"participants": []map[string]any{{"user_id": c.noteTakerID, "role": "meeting_note_taker"}, {"user_id": c.attendeeID}}}
	for k, v := range extra {
		b[k] = v
	}
	r := c.as(t, c.organizer, "POST", "/meetings", body(b))
	expect(t, r, 201, "started meeting")
	return r.str("data.id"), r.str("data.next_occurrence.id")
}

// TestMeetingsJ4RunTheMeeting covers J4: attendance, notes per section with versions and conflicts, decisions.
func TestMeetingsJ4RunTheMeeting(t *testing.T) {
	c := newMeetingCast(t)
	_, occ := c.startedMeeting(t, nil)
	if s := c.as(t, c.noteTaker, "GET", "/occurrences/"+occ); s.str("data.status") != "in_progress" {
		t.Fatalf("the clock drives the status: %s", s.Raw)
	}
	att := c.as(t, c.noteTaker, "PUT", "/occurrences/"+occ+"/attendance", body(map[string]any{"entries": []map[string]any{
		{"user_id": c.attendeeID, "state": "excused"}, {"user_id": c.organizerID, "state": "present"}, {"user_id": c.noteTakerID, "state": "late"}}}))
	expect(t, att, 200, "mark attendance")
	if !att.has("data", "state", "excused") || len(att.list("data")) != 3 {
		t.Fatalf("attendance: %s", att.Raw)
	}
	expectErr(t, c.as(t, c.noteTaker, "PUT", "/occurrences/"+occ+"/attendance", body(map[string]any{"entries": []map[string]any{{"user_id": c.eveID, "state": "present"}}})),
		400, "VALIDATION_ERROR", "only participants")
	expectErr(t, c.as(t, c.attendee, "PUT", "/occurrences/"+occ+"/attendance", body(map[string]any{"entries": []map[string]any{{"user_id": c.attendeeID, "state": "present"}}})),
		403, "FORBIDDEN", "attendee cannot mark")
	item := c.as(t, c.organizer, "POST", "/occurrences/"+occ+"/agenda-items", body(map[string]any{"title": "Rota", "kind": "decision"}))
	expect(t, item, 201, "agenda item")
	itemID := item.str("data.id")
	v1 := c.as(t, c.noteTaker, "PUT", "/occurrences/"+occ+"/notes/general", body(map[string]any{"body": "Opened at 9", "version": 0}))
	expect(t, v1, 200, "first save")
	if v1.get("data.version") != float64(1) {
		t.Fatalf("version 1: %s", v1.Raw)
	}
	stale := c.as(t, c.organizer, "PUT", "/occurrences/"+occ+"/notes/general", body(map[string]any{"body": "Other text", "version": 0}))
	expectErr(t, stale, 409, "VERSION_CONFLICT", "stale base version")
	if stale.str("error.current_body") != "Opened at 9" || stale.get("error.current_version") != float64(1) {
		t.Fatalf("the conflict carries the current text: %s", stale.Raw)
	}
	// A save in another section never conflicts with the general section.
	expect(t, c.as(t, c.organizer, "PUT", "/occurrences/"+occ+"/notes/"+itemID, body(map[string]any{"body": "3 shifts", "version": 0})), 200, "other section")
	expect(t, c.as(t, c.noteTaker, "PUT", "/occurrences/"+occ+"/notes/general", header("If-Match", "1"), body(map[string]any{"body": "Opened at 9, all present"})), 200, "v2")
	vers := c.as(t, c.noteTaker, "GET", "/occurrences/"+occ+"/notes/general/versions")
	if len(vers.list("data")) != 2 {
		t.Fatalf("history: %s", vers.Raw)
	}
	notes := c.as(t, c.noteTaker, "GET", "/occurrences/"+occ+"/notes")
	if l := notes.list("data"); len(l) != 2 || l[0].(map[string]any)["section"] != "general" || l[1].(map[string]any)["title"] != "Rota" {
		t.Fatalf("sections in order: %s", notes.Raw)
	}
	expectErr(t, c.as(t, c.attendee, "GET", "/occurrences/"+occ+"/notes"), 403, "FORBIDDEN", "attendees read published minutes only")
	expectErr(t, c.as(t, c.attendee, "PUT", "/occurrences/"+occ+"/notes/general", body(map[string]any{"body": "x", "version": 2})), 403, "FORBIDDEN", "attendee cannot write")
	dec := c.as(t, c.noteTaker, "POST", "/occurrences/"+occ+"/decisions", body(map[string]any{"text": "Move to 3-shift rota from July", "agenda_item_id": itemID,
		"decided_by": c.organizerID}))
	expect(t, dec, 201, "decision")
	if l := c.as(t, c.attendee, "GET", "/occurrences/"+occ+"/decisions"); !l.has("data", "status", "approved") {
		t.Fatalf("attendees read decisions: %s", l.Raw)
	}
	if err := testDB.Exec("UPDATE meeting_note_versions SET body_md = 'x' WHERE occurrence_id = ?", occ).Error; err == nil {
		t.Fatal("note versions are append-only")
	}
}

// TestMeetingsJ5ActionItemsAreM2Tasks covers J5: tasks created as the caller in M2, linking, restriction and the refusals.
func TestMeetingsJ5ActionItemsAreM2Tasks(t *testing.T) {
	c := newMeetingCast(t)
	pm, _, _ := c.tc.limitedUser(t, [2]string{"projects", "create"})
	proj := c.as(t, pm, "POST", "/projects", body(map[string]any{"key": "OPS", "name": "Ops"}))
	expect(t, proj, 201, "project")
	pid := proj.str("data.id")
	expect(t, c.as(t, pm, "POST", "/projects/"+pid+"/members", body(map[string]any{"user_id": c.noteTakerID, "role": "member"})), 201, "note-taker joins project")
	series, occ := c.startedMeeting(t, map[string]any{"default_project_id": pid})
	due := time.Now().UTC().AddDate(0, 0, 30).Format(time.RFC3339)
	a := c.as(t, c.noteTaker, "POST", "/occurrences/"+occ+"/action-items", header("Idempotency-Key", "ai-"+uniq()),
		body(map[string]any{"title": "Draft new rota", "assignee_ids": []string{c.noteTakerID}, "due_at": due}))
	expect(t, a, 201, "action item creates an M2 task")
	taskID := a.str("data.task_id")
	if !strings.HasPrefix(a.str("data.task.key"), "OPS-") || a.str("data.task.project_id") != pid {
		t.Fatalf("task in the default project: %s", a.Raw)
	}
	if task := c.as(t, c.noteTaker, "GET", "/tasks/"+taskID); task.str("data.title") != "Draft new rota" {
		t.Fatalf("the task lives in M2: %s", task.Raw)
	}
	expectErr(t, c.as(t, c.noteTaker, "POST", "/occurrences/"+occ+"/action-items", body(map[string]any{"task_id": taskID})), 409, "ALREADY_LINKED", "duplicate link")
	hire := c.as(t, pm, "POST", "/projects/"+pid+"/tasks", body(map[string]any{"title": "Hire 2 operators"}))
	expect(t, hire, 201, "existing task")
	expect(t, c.as(t, c.noteTaker, "POST", "/occurrences/"+occ+"/action-items", body(map[string]any{"task_id": hire.str("data.id")})), 201, "link existing task")
	// The attendee is not a project member: the meeting shows the actions as restricted, without task data.
	l := c.as(t, c.attendee, "GET", "/occurrences/"+occ+"/action-items")
	expect(t, l, 200, "attendee lists actions")
	if items := l.list("data"); len(items) != 2 || items[0].(map[string]any)["restricted"] != true || items[0].(map[string]any)["task"] != nil {
		t.Fatalf("restricted for a non-member: %s", l.Raw)
	}
	if n := c.as(t, c.noteTaker, "GET", "/occurrences/"+occ+"/action-items"); n.has("data", "restricted", "true") || len(n.list("data")) != 2 {
		t.Fatalf("live for the member: %s", n.Raw)
	}
	expectErr(t, c.as(t, c.attendee, "POST", "/occurrences/"+occ+"/action-items", body(map[string]any{"title": "x"})), 403, "FORBIDDEN", "attendee cannot create")
	// The organizer is not a project member: M2 refuses the task, so nothing is linked.
	expectErr(t, c.as(t, c.organizer, "POST", "/occurrences/"+occ+"/action-items", body(map[string]any{"title": "Mine"})), 404, "NOT_FOUND", "M2 decides")
	_, bare := c.startedMeeting(t, nil)
	expectErr(t, c.as(t, c.noteTaker, "POST", "/occurrences/"+bare+"/action-items", body(map[string]any{"title": "No project"})), 422, "NO_TASK_PROJECT", "no project")
	expectErr(t, c.as(t, c.noteTaker, "POST", "/occurrences/"+bare+"/action-items", body(map[string]any{"task_id": "00000000-0000-0000-0000-000000000009"})),
		404, "NOT_FOUND", "unreadable task")
	first := l.list("data")[0].(map[string]any)["id"].(string)
	expect(t, c.as(t, c.organizer, "DELETE", "/action-items/"+first), 204, "unlink")
	if task := c.as(t, c.noteTaker, "GET", "/tasks/"+taskID); task.Status != 200 {
		t.Fatalf("unlinking keeps the M2 task: %s", task.Raw)
	}
	_ = series
}

// TestMeetingsJ6PublishAndCorrectMinutes covers J6: publish freezes, corrections need a reason, versions stay.
func TestMeetingsJ6PublishAndCorrectMinutes(t *testing.T) {
	c := newMeetingCast(t)
	_, occ := c.startedMeeting(t, nil)
	expectErr(t, c.as(t, c.noteTaker, "POST", "/occurrences/"+occ+"/minutes/publish"), 422, "NOTES_EMPTY", "nothing to publish")
	expectErr(t, c.as(t, c.attendee, "GET", "/occurrences/"+occ+"/minutes"), 404, "NOT_FOUND", "no minutes yet")
	expect(t, c.as(t, c.noteTaker, "PUT", "/occurrences/"+occ+"/notes/general", body(map[string]any{"body": "We agreed on the rota.", "version": 0})), 200, "notes")
	expectErr(t, c.as(t, c.attendee, "POST", "/occurrences/"+occ+"/minutes/publish"), 403, "FORBIDDEN", "attendee cannot publish")
	p := c.as(t, c.noteTaker, "POST", "/occurrences/"+occ+"/minutes/publish")
	expect(t, p, 201, "publish")
	if p.get("data.version") != float64(1) || len(p.str("data.sha256")) != 64 {
		t.Fatalf("published v1: %s", p.Raw)
	}
	m := c.as(t, c.attendee, "GET", "/occurrences/"+occ+"/minutes")
	if !strings.Contains(m.str("data.body"), "We agreed on the rota.") || !strings.Contains(m.str("data.body"), "# Ops sync") {
		t.Fatalf("attendees read the published minutes: %s", m.Raw)
	}
	if s := c.as(t, c.attendee, "GET", "/occurrences/"+occ); s.str("data.status") != "minutes_published" {
		t.Fatalf("status: %s", s.Raw)
	}
	expectErr(t, c.as(t, c.noteTaker, "POST", "/occurrences/"+occ+"/minutes/publish"), 409, "MINUTES_PUBLISHED", "publish once")
	expectErr(t, c.as(t, c.noteTaker, "POST", "/occurrences/"+occ+"/minutes/corrections", body(map[string]any{})), 400, "VALIDATION_ERROR", "reason required")
	expect(t, c.as(t, c.noteTaker, "PUT", "/occurrences/"+occ+"/notes/general", body(map[string]any{"body": "We agreed on the new rota.", "version": 1})), 200, "fix typo")
	v2 := c.as(t, c.noteTaker, "POST", "/occurrences/"+occ+"/minutes/corrections", body(map[string]any{"reason": "typo"}))
	expect(t, v2, 201, "correction")
	vs := c.as(t, c.attendee, "GET", "/occurrences/"+occ+"/minutes/versions")
	if l := vs.list("data"); len(l) != 2 || l[0].(map[string]any)["reason"] != "typo" {
		t.Fatalf("both versions stay: %s", vs.Raw)
	}
	if v1 := c.as(t, c.attendee, "GET", "/occurrences/"+occ+"/minutes/versions/1"); strings.Contains(v1.str("data.body"), "new rota") {
		t.Fatalf("version 1 is frozen: %s", v1.Raw)
	}
	if err := testDB.Exec("UPDATE meeting_minutes SET body_md = 'x' WHERE occurrence_id = ?", occ).Error; err == nil {
		t.Fatal("published minutes are append-only")
	}
	expectErr(t, c.as(t, c.organizer, "PATCH", "/occurrences/"+occ, header("If-Match", "2"), body(map[string]any{"title": "x"})), 409, "OCCURRENCE_HAS_MINUTES", "no reschedule after publish")
}

// TestMeetingsJ7PrivateTalkingPoints covers J7: private points reach their author only, through every route and role.
func TestMeetingsJ7PrivateTalkingPoints(t *testing.T) {
	c := newMeetingCast(t)
	auditor, _, _ := c.tc.limitedUser(t, [2]string{"meetings", "read_all"}, [2]string{"meetings", "read_confidential"})
	berlin, _ := time.LoadLocation("Europe/Berlin")
	r := c.as(t, c.organizer, "POST", "/meetings", body(map[string]any{"title": "1:1 Mark / Arun", "start": nextWeekday(berlin, time.Monday).Format("2006-01-02") + "T10:00",
		"tz": "Europe/Berlin", "duration_min": 30, "rrule": "FREQ=WEEKLY", "participants": []map[string]any{{"user_id": c.attendeeID}}}))
	expect(t, r, 201, "1:1")
	occ := r.str("data.next_occurrence.id")
	mine := c.as(t, c.organizer, "POST", "/occurrences/"+occ+"/private-points", body(map[string]any{"body": "Ask about workload"}))
	expect(t, mine, 201, "manager point")
	expect(t, c.as(t, c.attendee, "POST", "/occurrences/"+occ+"/private-points", body(map[string]any{"body": "Ask for training budget"})), 201, "report point")
	if l := c.as(t, c.attendee, "GET", "/occurrences/"+occ+"/private-points"); len(l.list("data")) != 1 || strings.Contains(l.Raw, "workload") {
		t.Fatalf("each sees only their own: %s", l.Raw)
	}
	pointID := mine.str("data.id")
	for name, tok := range map[string]string{"report": c.attendee, "admin": c.tc.token, "auditor": auditor} {
		expectErr(t, c.as(t, tok, "PATCH", "/private-points/"+pointID, body(map[string]any{"body": "x"})), 404, "NOT_FOUND", name+" edits")
		expectErr(t, c.as(t, tok, "DELETE", "/private-points/"+pointID), 404, "NOT_FOUND", name+" deletes")
		if l := c.as(t, tok, "GET", "/occurrences/"+occ+"/private-points"); strings.Contains(l.Raw, "workload") {
			t.Fatalf("%s must never read the manager's point: %s", name, l.Raw)
		}
	}
	expectErr(t, c.as(t, auditor, "GET", "/occurrences/"+occ+"/private-points"), 404, "NOT_FOUND", "auditor is not a participant")
	promoted := c.as(t, c.organizer, "POST", "/private-points/"+pointID+"/promote")
	expect(t, promoted, 201, "promote to agenda")
	if promoted.str("data.title") != "Ask about workload" {
		t.Fatalf("promoted: %s", promoted.Raw)
	}
	if l := c.as(t, c.organizer, "GET", "/occurrences/"+occ+"/private-points"); len(l.list("data")) != 0 {
		t.Fatalf("the private copy is removed: %s", l.Raw)
	}
	if a := c.as(t, c.attendee, "GET", "/occurrences/"+occ+"/agenda-items"); !a.has("data", "title", "Ask about workload") {
		t.Fatalf("now shared: %s", a.Raw)
	}
}

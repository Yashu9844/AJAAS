package integration

// M3 Meetings user journeys through the production router (docs/modules/m3-meetings/research/user-journeys.md).

import (
	"fmt"
	"testing"
	"time"
)

// meetingCast is the set of users the journeys need, each with only the tenant permissions their persona holds.
type meetingCast struct {
	tc                                   *tenantCtx
	organizer, noteTaker, attendee, eve  string // tokens
	organizerID, noteTakerID, attendeeID string
	eveID                                string
}

func newMeetingCast(t *testing.T) *meetingCast {
	t.Helper()
	tc := newTenant(t)
	c := &meetingCast{tc: tc}
	c.organizer, _, c.organizerID = tc.limitedUser(t, [2]string{"meetings", "create"})
	c.noteTaker, _, c.noteTakerID = tc.limitedUser(t, [2]string{"meetings", "create"})
	c.attendee, _, c.attendeeID = tc.limitedUser(t, [2]string{"meetings", "create"})
	c.eve, _, c.eveID = tc.limitedUser(t, [2]string{"meetings", "create"})
	return c
}

func (c *meetingCast) as(t *testing.T, tok, method, path string, opts ...opt) resp {
	t.Helper()
	return c.tc.asTok(t, tok, method, path, opts...)
}

// nextWeekday returns the local date of the next given weekday at least 2 days ahead, in loc.
func nextWeekday(loc *time.Location, wd time.Weekday) time.Time {
	d := time.Now().In(loc).AddDate(0, 0, 2)
	for d.Weekday() != wd {
		d = d.AddDate(0, 0, 1)
	}
	return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, loc)
}

// weekly creates the J1 series: weekly on Tuesday 09:00 Europe/Berlin, open-ended, with the note-taker and attendee.
func (c *meetingCast) weekly(t *testing.T, extra map[string]any) resp {
	t.Helper()
	berlin, _ := time.LoadLocation("Europe/Berlin")
	start := nextWeekday(berlin, time.Tuesday)
	b := map[string]any{"title": "Weekly ops review", "description": "Ops **weekly**", "start": start.Format("2006-01-02") + "T09:00",
		"tz": "Europe/Berlin", "duration_min": 45, "rrule": "FREQ=WEEKLY;BYDAY=TU", "visibility": "internal",
		"location": "Room B2", "video_url": "https://meet.example.com/ops",
		"participants": []map[string]any{{"user_id": c.noteTakerID, "role": "meeting_note_taker"}, {"user_id": c.attendeeID, "kind": "optional"}}}
	for k, v := range extra {
		b[k] = v
	}
	return c.as(t, c.organizer, "POST", "/meetings", body(b))
}

func countRows(t *testing.T, q string, args ...any) int {
	t.Helper()
	var n int
	if err := testDB.Raw(q, args...).Scan(&n).Error; err != nil {
		t.Fatal(err)
	}
	return n
}

// TestMeetingsJ1CreateRecurringSeries covers J1: create, materialise 12 months, M0 roles, idempotency and the refusals.
func TestMeetingsJ1CreateRecurringSeries(t *testing.T) {
	c := newMeetingCast(t)
	key := "m-" + uniq()
	first := c.as(t, c.organizer, "POST", "/meetings", header("Idempotency-Key", key), body(map[string]any{"title": "Once", "start": "2030-01-08T09:00",
		"tz": "Europe/Berlin", "duration_min": 30}))
	expect(t, first, 201, "single meeting")
	again := c.as(t, c.organizer, "POST", "/meetings", header("Idempotency-Key", key), body(map[string]any{"title": "Once", "start": "2030-01-08T09:00",
		"tz": "Europe/Berlin", "duration_min": 30}))
	if again.str("data.id") != first.str("data.id") {
		t.Fatalf("a retried create replays the same series: %s vs %s", again.Raw, first.Raw)
	}
	r := c.weekly(t, nil)
	expect(t, r, 201, "weekly series")
	id := r.str("data.id")
	if r.str("data.template") != "meeting_organizer" || r.str("data.level") != "full" || r.str("data.rrule") != "FREQ=WEEKLY;BYDAY=TU" {
		t.Fatalf("organizer view: %s", r.Raw)
	}
	n := countRows(t, "SELECT count(*) FROM meeting_occurrences WHERE series_id = ?", id)
	if n < 52 || n > 54 {
		t.Fatalf("an open-ended weekly series fills a 12-month window: %d", n)
	}
	// DST: every occurrence starts at 09:00 Berlin time, so the UTC hour is 7 in summer and 8 in winter.
	if hours := countRows(t, `SELECT count(DISTINCT extract(hour FROM start_at AT TIME ZONE 'Europe/Berlin')) FROM meeting_occurrences WHERE series_id = ?`, id); hours != 1 {
		t.Fatalf("wall clock must stay 09:00 across DST: %d distinct local hours", hours)
	}
	if utc := countRows(t, `SELECT count(DISTINCT extract(hour FROM start_at AT TIME ZONE 'UTC')) FROM meeting_occurrences WHERE series_id = ?`, id); utc != 2 {
		t.Fatalf("UTC hours must change with DST: %d", utc)
	}
	parts := c.as(t, c.organizer, "GET", "/meetings/"+id+"/participants")
	expect(t, parts, 200, "participants")
	if len(parts.list("data")) != 3 || !parts.has("data", "template", "meeting_note_taker") || !parts.has("data", "template", "meeting_attendee") {
		t.Fatalf("roles come from M0: %s", parts.Raw)
	}
	bad := map[string]map[string]any{
		"rrule": {"rrule": "FREQ=HOURLY"}, "count": {"rrule": "FREQ=WEEKLY;COUNT=501"}, "setpos": {"rrule": "FREQ=MONTHLY;BYSETPOS=1"},
	}
	for name, b := range bad {
		expectErr(t, c.weekly(t, b), 422, "INVALID_RRULE", "refused rule "+name)
	}
	expectErr(t, c.weekly(t, map[string]any{"video_url": "http://meet.example.com"}), 400, "VALIDATION_ERROR", "https only")
	expectErr(t, c.weekly(t, map[string]any{"tz": "Mars/Base"}), 400, "VALIDATION_ERROR", "unknown zone")
	expectErr(t, c.weekly(t, map[string]any{"duration_min": 0}), 400, "VALIDATION_ERROR", "duration")
	noCreate, _, _ := c.tc.limitedUser(t)
	expectErr(t, c.as(t, noCreate, "POST", "/meetings", body(map[string]any{"title": "x", "start": "2030-01-01T09:00", "tz": "UTC", "duration_min": 30})),
		403, "FORBIDDEN", "meetings:create required")
	gone, _ := c.tc.newUser(t, "gone")
	expect(t, c.tc.do(t, "POST", "/users/"+gone+"/deactivate"), 200, "deactivate")
	expectErr(t, c.weekly(t, map[string]any{"participants": []map[string]any{{"user_id": gone}}}), 422, "PARTICIPANT_INACTIVE", "inactive invitee")
}

// TestMeetingsJ2ExceptionsSplitAndCancel covers J2: one occurrence, this-and-following, cancel; data of others untouched.
func TestMeetingsJ2ExceptionsSplitAndCancel(t *testing.T) {
	c := newMeetingCast(t)
	r := c.weekly(t, nil)
	expect(t, r, 201, "series")
	id := r.str("data.id")
	occ := c.as(t, c.organizer, "GET", "/meetings/"+id+"/occurrences", query("from", time.Now().UTC().Format(time.RFC3339),
		"to", time.Now().UTC().AddDate(0, 0, 60).Format(time.RFC3339)))
	expect(t, occ, 200, "occurrences")
	list := occ.list("data")
	if len(list) < 7 {
		t.Fatalf("60 days of a weekly series: %s", occ.Raw)
	}
	o1, o2, o4 := list[0].(map[string]any), list[1].(map[string]any), list[3].(map[string]any)
	o1ID, o2ID := o1["id"].(string), o2["id"].(string)
	start := o1["start_at"].(string)
	t1, _ := time.Parse(time.RFC3339, start)
	moved := t1.Add(24 * time.Hour).UTC().Format(time.RFC3339)
	expectErr(t, c.as(t, c.organizer, "PATCH", "/occurrences/"+o1ID, body(map[string]any{"start_at": moved})), 428, "VERSION_REQUIRED", "version needed")
	m := c.as(t, c.organizer, "PATCH", "/occurrences/"+o1ID, header("If-Match", "1"), body(map[string]any{"start_at": moved, "reason": "holiday"}))
	expect(t, m, 200, "move one occurrence")
	if m.get("data.is_exception") != true || m.str("data.start_at") == start {
		t.Fatalf("exception: %s", m.Raw)
	}
	expectErr(t, c.as(t, c.organizer, "PATCH", "/occurrences/"+o1ID, header("If-Match", "1"), body(map[string]any{"title": "x"})), 409, "VERSION_CONFLICT", "stale")
	if got := c.as(t, c.organizer, "GET", "/occurrences/"+o2ID); got.str("data.start_at") != o2["start_at"] || got.get("data.is_exception") != false {
		t.Fatalf("moving occurrence 1 leaves occurrence 2 alone: %s", got.Raw)
	}
	expectErr(t, c.as(t, c.noteTaker, "PATCH", "/occurrences/"+o2ID, header("If-Match", "1"), body(map[string]any{"title": "x"})), 403, "FORBIDDEN", "note-taker cannot reschedule")
	cancel := c.as(t, c.organizer, "POST", "/occurrences/"+o2ID+"/cancel", body(map[string]any{"reason": "offsite", "version": 1}))
	expect(t, cancel, 200, "cancel one")
	if cancel.str("data.status") != "cancelled" {
		t.Fatalf("cancelled: %s", cancel.Raw)
	}
	// Agenda prepared on occurrence 4 must survive the split (it moves to the new series).
	o4ID := o4["id"].(string)
	expect(t, c.as(t, c.attendee, "POST", "/occurrences/"+o4ID+"/agenda-items", body(map[string]any{"title": "Shift rota"})), 201, "agenda before split")
	ver := c.as(t, c.organizer, "GET", "/meetings/"+id).get("data.version")
	berlin, _ := time.LoadLocation("Europe/Berlin")
	t3, _ := time.Parse(time.RFC3339, list[2].(map[string]any)["start_at"].(string))
	split := c.as(t, c.organizer, "POST", "/meetings/"+id+"/split", body(map[string]any{"occurrence_id": list[2].(map[string]any)["id"], "version": ver,
		"changes": map[string]any{"start": t3.In(berlin).Format("2006-01-02") + "T10:00", "title": "Ops review (10:00)"}}))
	expect(t, split, 201, "split this and following")
	newID := split.str("data.id")
	if split.str("data.title") != "Ops review (10:00)" || newID == id {
		t.Fatalf("new series: %s", split.Raw)
	}
	oldLeft := countRows(t, "SELECT count(*) FROM meeting_occurrences WHERE series_id = ?", id)
	if oldLeft != 2 {
		t.Fatalf("the old series keeps only the occurrences before the split: %d", oldLeft)
	}
	if n := countRows(t, "SELECT count(*) FROM meeting_agenda_items a JOIN meeting_occurrences o ON o.id = a.occurrence_id WHERE o.series_id = ?", newID); n != 1 {
		t.Fatalf("prepared agenda moved with its occurrence: %d", n)
	}
	if p := c.as(t, c.attendee, "GET", "/meetings/"+newID+"/participants"); len(p.list("data")) != 3 {
		t.Fatalf("participants and roles are copied: %s", p.Raw)
	}
	all := c.as(t, c.organizer, "POST", "/meetings/"+newID+"/cancel", header("If-Match", fmt.Sprint(split.get("data.version"))), body(map[string]any{"reason": "ended"}))
	expect(t, all, 200, "cancel series")
	if all.str("data.status") != "cancelled" || all.get("data.next_occurrence") != nil {
		t.Fatalf("cancelled series: %s", all.Raw)
	}
	expectErr(t, c.as(t, c.attendee, "POST", "/occurrences/"+o4ID+"/agenda-items", body(map[string]any{"title": "late"})), 409, "MEETING_CANCELLED", "no writes on a cancelled series")
}

// TestMeetingsJ3AndJ10RespondPrepareAndNoLeak covers J3 (RSVP, agenda topics) and J10 (404 without an existence oracle).
func TestMeetingsJ3AndJ10RespondPrepareAndNoLeak(t *testing.T) {
	c := newMeetingCast(t)
	r := c.weekly(t, map[string]any{"visibility": "private"})
	expect(t, r, 201, "private series")
	id := r.str("data.id")
	occID := r.str("data.next_occurrence.id")
	expect(t, c.as(t, c.attendee, "PUT", "/meetings/"+id+"/response", body(map[string]any{"rsvp": "accepted"})), 204, "accept series")
	expect(t, c.as(t, c.attendee, "PUT", "/occurrences/"+occID+"/response", body(map[string]any{"rsvp": "declined", "comment": "on leave"})), 204, "decline one")
	d := c.as(t, c.organizer, "GET", "/occurrences/"+occID)
	expect(t, d, 200, "occurrence detail")
	if !d.has("data.responses", "rsvp", "declined") || !d.has("data.responses", "comment", "on leave") {
		t.Fatalf("the organizer sees the answer: %s", d.Raw)
	}
	mine := c.as(t, c.attendee, "GET", "/me/meetings", query("from", time.Now().UTC().Format(time.RFC3339), "to", time.Now().UTC().AddDate(0, 0, 20).Format(time.RFC3339)))
	expect(t, mine, 200, "my meetings")
	if !mine.has("data", "my_rsvp", "declined") || !mine.has("data", "my_rsvp", "accepted") {
		t.Fatalf("my week shows the series answer and the override: %s", mine.Raw)
	}
	expectErr(t, c.as(t, c.attendee, "PUT", "/meetings/"+id+"/response", body(map[string]any{"rsvp": "maybe"})), 400, "VALIDATION_ERROR", "bad rsvp")
	item := c.as(t, c.attendee, "POST", "/occurrences/"+occID+"/agenda-items", body(map[string]any{"title": "Shift rota", "timebox_min": 10}))
	expect(t, item, 201, "attendee proposes a topic")
	expectErr(t, c.as(t, c.attendee, "POST", "/agenda-items/"+item.str("data.id")+"/move", body(map[string]any{})), 403, "FORBIDDEN", "attendee cannot reorder")
	// J10: the uninvited user learns nothing about a private meeting.
	for _, p := range []string{"/meetings/" + id, "/meetings/" + id + "/participants", "/occurrences/" + occID, "/occurrences/" + occID + "/agenda-items",
		"/occurrences/" + occID + "/minutes", "/occurrences/" + occID + "/ics", "/meetings/" + id + "/occurrences"} {
		expectErr(t, c.as(t, c.eve, "GET", p), 404, "NOT_FOUND", "eve "+p)
	}
	expectErr(t, c.as(t, c.eve, "PUT", "/meetings/"+id+"/response", body(map[string]any{"rsvp": "accepted"})), 404, "NOT_FOUND", "eve answers")
	expectErr(t, c.as(t, c.eve, "GET", "/meetings/00000000-0000-0000-0000-000000000001"), 404, "NOT_FOUND", "unknown id answers the same")
	other := newTenant(t)
	expectErr(t, call(t, "GET", "/meetings/"+id, host(other.slug), token(other.token)), 404, "NOT_FOUND", "foreign tenant admin")
	window := query("from", time.Now().UTC().Format(time.RFC3339), "to", time.Now().UTC().AddDate(0, 0, 20).Format(time.RFC3339))
	if l := c.as(t, c.eve, "GET", "/meetings", window); len(l.list("data")) != 0 {
		t.Fatalf("eve sees no private meeting in lists: %s", l.Raw)
	}
	// An internal meeting shows Eve title and time only; a busy_only one a busy block.
	in := c.weekly(t, map[string]any{"title": "Town hall", "visibility": "internal"})
	busy := c.weekly(t, map[string]any{"title": "Secret board", "visibility": "busy_only"})
	ev := c.as(t, c.eve, "GET", "/meetings/"+in.str("data.id"))
	if ev.str("data.title") != "Town hall" || ev.str("data.level") != "internal" || ev.str("data.description") != "" || ev.str("data.video_url") != "" {
		t.Fatalf("internal level: %s", ev.Raw)
	}
	expectErr(t, c.as(t, c.eve, "GET", "/meetings/"+in.str("data.id")+"/participants"), 404, "NOT_FOUND", "internal level has no content")
	if b := c.as(t, c.eve, "GET", "/meetings/"+busy.str("data.id")); b.str("data.title") != "Busy" || b.str("data.location") != "" {
		t.Fatalf("busy level: %s", b.Raw)
	}
	l := c.as(t, c.eve, "GET", "/meetings", window)
	if !l.has("data", "title", "Busy") || !l.has("data", "title", "Town hall") || l.has("data", "title", "Secret board") {
		t.Fatalf("eve's list: %s", l.Raw)
	}
}

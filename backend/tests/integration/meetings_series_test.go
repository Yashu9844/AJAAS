package integration

import (
	"fmt"
	"testing"
	"time"
)

// TestMeetingsSeriesEditConsentStatusDelete covers editing all occurrences (MR-04), consent (DMT-13), the manual status
// (MR-05) and deletion (RT-1).
func TestMeetingsSeriesEditConsentStatusDelete(t *testing.T) {
	c := newMeetingCast(t)
	r := c.weekly(t, map[string]any{"recording_allowed": true})
	id := r.str("data.id")
	occ := r.str("data.next_occurrence.id")
	expect(t, c.as(t, c.attendee, "POST", "/occurrences/"+occ+"/agenda-items", body(map[string]any{"title": "Prepared"})), 201, "data on the next occurrence")
	before := countRows(t, "SELECT count(*) FROM meeting_occurrences WHERE series_id = ?", id)
	t1 := c.as(t, c.organizer, "PATCH", "/meetings/"+id, header("If-Match", "1"), body(map[string]any{"title": "Ops review", "location": "Room C"}))
	expect(t, t1, 200, "edit without schedule change")
	if t1.str("data.title") != "Ops review" || t1.get("data.version") != float64(2) || t1.get("data.sequence") != float64(1) {
		t.Fatalf("edited: %s", t1.Raw)
	}
	if n := countRows(t, "SELECT count(*) FROM meeting_occurrences WHERE series_id = ?", id); n != before {
		t.Fatalf("a non-schedule edit keeps occurrences: %d vs %d", n, before)
	}
	expectErr(t, c.as(t, c.organizer, "PATCH", "/meetings/"+id, header("If-Match", "1"), body(map[string]any{"title": "x"})), 409, "VERSION_CONFLICT", "stale")
	expectErr(t, c.as(t, c.attendee, "PATCH", "/meetings/"+id, header("If-Match", "2"), body(map[string]any{"title": "x"})), 403, "FORBIDDEN", "attendee cannot edit")
	t2 := c.as(t, c.organizer, "PATCH", "/meetings/"+id, header("If-Match", "2"), body(map[string]any{"rrule": "FREQ=WEEKLY;BYDAY=TU;COUNT=4", "duration_min": 60}))
	expect(t, t2, 200, "schedule change")
	if n := countRows(t, "SELECT count(*) FROM meeting_occurrences WHERE series_id = ? AND NOT orphaned", id); n != 4 {
		t.Fatalf("regenerated to 4 occurrences: %d", n)
	}
	if n := countRows(t, "SELECT count(*) FROM meeting_occurrences WHERE id = ? AND end_at - start_at = interval '60 minutes'", occ); n != 1 {
		t.Fatal("the occurrence with data stays and takes the new duration")
	}
	if n := countRows(t, "SELECT count(*) FROM meeting_series WHERE id = ? AND window_until IS NULL", id); n != 1 {
		t.Fatal("a bounded series is complete: no rolling window")
	}
	// Consent: recording needs every active participant.
	expect(t, c.as(t, c.attendee, "PUT", "/meetings/"+id+"/consent", body(map[string]any{"state": "granted"})), 200, "consent")
	cs := c.as(t, c.organizer, "GET", "/meetings/"+id+"/consent")
	if cs.get("data.may_record") != false || cs.get("data.recording_allowed") != true || !cs.has("data.participants", "state", "pending") {
		t.Fatalf("pending participants block recording: %s", cs.Raw)
	}
	for _, tok := range []string{c.organizer, c.noteTaker} {
		expect(t, c.as(t, tok, "PUT", "/meetings/"+id+"/consent", body(map[string]any{"state": "granted"})), 200, "grant")
	}
	if cs = c.as(t, c.organizer, "GET", "/meetings/"+id+"/consent"); cs.get("data.may_record") != true {
		t.Fatalf("all granted: %s", cs.Raw)
	}
	expect(t, c.as(t, c.attendee, "PUT", "/meetings/"+id+"/consent", body(map[string]any{"state": "declined"})), 200, "decline")
	if cs = c.as(t, c.organizer, "GET", "/meetings/"+id+"/consent"); cs.get("data.may_record") != false {
		t.Fatalf("one decline stops recording: %s", cs.Raw)
	}
	expectErr(t, c.as(t, c.eve, "PUT", "/meetings/"+id+"/consent", body(map[string]any{"state": "granted"})), 404, "NOT_FOUND", "outsider")
	// Manual status: the note-taker starts the meeting early, then hands it back to the clock.
	ov := c.as(t, c.noteTaker, "GET", "/occurrences/"+occ).get("data.version")
	st := c.as(t, c.noteTaker, "POST", "/occurrences/"+occ+"/status", body(map[string]any{"status": "in_progress", "version": ov}))
	expect(t, st, 200, "start early")
	if st.str("data.status") != "in_progress" {
		t.Fatalf("manual status: %s", st.Raw)
	}
	back := c.as(t, c.noteTaker, "POST", "/occurrences/"+occ+"/status", header("If-Match", fmt.Sprint(st.get("data.version"))), body(map[string]any{"status": "scheduled"}))
	if back.str("data.status") != "scheduled" {
		t.Fatalf("back to the clock: %s", back.Raw)
	}
	expectErr(t, c.as(t, c.attendee, "POST", "/occurrences/"+occ+"/status", body(map[string]any{"status": "ended", "version": 1})), 403, "FORBIDDEN", "attendee")
	ver := fmt.Sprint(c.as(t, c.organizer, "GET", "/meetings/"+id).get("data.version"))
	expectErr(t, c.as(t, c.noteTaker, "DELETE", "/meetings/"+id, header("If-Match", ver)), 403, "FORBIDDEN", "note-taker cannot delete")
	expect(t, c.as(t, c.organizer, "DELETE", "/meetings/"+id, header("If-Match", ver)), 204, "delete")
	expectErr(t, c.as(t, c.organizer, "GET", "/meetings/"+id), 404, "NOT_FOUND", "deleted")
	window := query("from", time.Now().UTC().Format(time.RFC3339), "to", time.Now().UTC().AddDate(0, 0, 40).Format(time.RFC3339))
	if l := c.as(t, c.organizer, "GET", "/me/meetings", window); l.has("data", "series_id", id) {
		t.Fatalf("deleted series leave the lists: %s", l.Raw)
	}
}

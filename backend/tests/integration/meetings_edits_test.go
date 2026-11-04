package integration

import (
	"strings"
	"testing"
	"time"
)

// TestMeetingsAgendaAndDecisionEdits covers MR-07 (agenda ownership, ordering, ended and published occurrences) and
// decision edits with versions.
func TestMeetingsAgendaAndDecisionEdits(t *testing.T) {
	c := newMeetingCast(t)
	_, occ := c.startedMeeting(t, nil)
	a1 := c.as(t, c.organizer, "POST", "/occurrences/"+occ+"/agenda-items", body(map[string]any{"title": "First"}))
	a2 := c.as(t, c.attendee, "POST", "/occurrences/"+occ+"/agenda-items", body(map[string]any{"title": "Second", "body": "details"}))
	a3 := c.as(t, c.organizer, "POST", "/occurrences/"+occ+"/agenda-items", body(map[string]any{"title": "Third"}))
	for _, r := range []resp{a1, a2, a3} {
		expect(t, r, 201, "agenda item")
	}
	id1, id2, id3 := a1.str("data.id"), a2.str("data.id"), a3.str("data.id")
	expectErr(t, c.as(t, c.attendee, "POST", "/occurrences/"+occ+"/agenda-items", body(map[string]any{"title": "x", "kind": "party"})), 400, "VALIDATION_ERROR", "kind")
	expectErr(t, c.as(t, c.attendee, "POST", "/occurrences/"+occ+"/agenda-items", body(map[string]any{"title": "x", "timebox_min": 0})), 400, "VALIDATION_ERROR", "timebox")
	expectErr(t, c.as(t, c.attendee, "POST", "/occurrences/"+occ+"/agenda-items", body(map[string]any{})), 400, "VALIDATION_ERROR", "title required")
	own := c.as(t, c.attendee, "PATCH", "/agenda-items/"+id2, header("If-Match", "1"), body(map[string]any{"title": "Second (edited)", "state": "discussed", "owner_id": c.attendeeID}))
	expect(t, own, 200, "attendee edits own item")
	expectErr(t, c.as(t, c.attendee, "PATCH", "/agenda-items/"+id1, header("If-Match", "1"), body(map[string]any{"title": "mine now"})), 403, "FORBIDDEN", "not their item")
	expectErr(t, c.as(t, c.organizer, "PATCH", "/agenda-items/"+id2, header("If-Match", "1"), body(map[string]any{"title": "x"})), 409, "VERSION_CONFLICT", "stale")
	expect(t, c.as(t, c.organizer, "POST", "/agenda-items/"+id3+"/move", body(map[string]any{"before_id": id1})), 200, "third to the top")
	expect(t, c.as(t, c.organizer, "POST", "/agenda-items/"+id2+"/move", body(map[string]any{"after_id": id3})), 200, "second after third")
	order := c.as(t, c.attendee, "GET", "/occurrences/"+occ+"/agenda-items").list("data")
	got := []string{}
	for _, it := range order {
		got = append(got, it.(map[string]any)["title"].(string))
	}
	if strings.Join(got, ",") != "Third,Second (edited),First" {
		t.Fatalf("order after moves: %v", got)
	}
	expectErr(t, c.as(t, c.organizer, "POST", "/agenda-items/"+id2+"/move", body(map[string]any{"after_id": id2})), 400, "VALIDATION_ERROR", "self neighbour")
	expectErr(t, c.as(t, c.organizer, "POST", "/agenda-items/"+id2+"/move", body(map[string]any{"after_id": id1, "before_id": id3})), 400, "VALIDATION_ERROR", "inverted")
	expect(t, c.as(t, c.attendee, "DELETE", "/agenda-items/"+id2), 204, "attendee removes own item")
	expectErr(t, c.as(t, c.attendee, "DELETE", "/agenda-items/"+id1), 403, "FORBIDDEN", "not their item")
	// Notes per agenda section, unknown sections.
	expectErr(t, c.as(t, c.noteTaker, "PUT", "/occurrences/"+occ+"/notes/nope", body(map[string]any{"body": "x", "version": 0})), 400, "VALIDATION_ERROR", "section")
	expectErr(t, c.as(t, c.noteTaker, "PUT", "/occurrences/"+occ+"/notes/"+id2, body(map[string]any{"body": "x", "version": 0})), 404, "NOT_FOUND", "removed item")
	expect(t, c.as(t, c.noteTaker, "PUT", "/occurrences/"+occ+"/notes/"+id1, body(map[string]any{"body": "first notes", "version": 0})), 200, "section notes")
	if v := c.as(t, c.noteTaker, "GET", "/occurrences/"+occ+"/notes/"+id1+"/versions"); len(v.list("data")) != 1 {
		t.Fatalf("section versions: %s", v.Raw)
	}
	// Decisions.
	dec := c.as(t, c.noteTaker, "POST", "/occurrences/"+occ+"/decisions", body(map[string]any{"text": "Proposed rota", "status": "proposed"}))
	expect(t, dec, 201, "decision")
	did := dec.str("data.id")
	expectErr(t, c.as(t, c.noteTaker, "POST", "/occurrences/"+occ+"/decisions", body(map[string]any{"text": "x", "status": "maybe"})), 400, "VALIDATION_ERROR", "status")
	expectErr(t, c.as(t, c.noteTaker, "POST", "/occurrences/"+occ+"/decisions", body(map[string]any{"text": "x", "agenda_item_id": id2})), 400, "VALIDATION_ERROR", "agenda item of this occurrence")
	expectErr(t, c.as(t, c.noteTaker, "POST", "/occurrences/"+occ+"/decisions", body(map[string]any{})), 400, "VALIDATION_ERROR", "text required")
	up := c.as(t, c.noteTaker, "PATCH", "/decisions/"+did, header("If-Match", "1"), body(map[string]any{"status": "superseded", "text": "Proposed rota (old)"}))
	expect(t, up, 200, "supersede")
	expectErr(t, c.as(t, c.noteTaker, "PATCH", "/decisions/"+did, header("If-Match", "1"), body(map[string]any{"status": "approved"})), 409, "VERSION_CONFLICT", "stale decision")
	expectErr(t, c.as(t, c.attendee, "PATCH", "/decisions/"+did, header("If-Match", "2"), body(map[string]any{"status": "approved"})), 403, "FORBIDDEN", "attendee")
	expectErr(t, c.as(t, c.eve, "PATCH", "/decisions/"+did, header("If-Match", "2"), body(map[string]any{"status": "approved"})), 404, "NOT_FOUND", "outsider")
	expect(t, c.as(t, c.noteTaker, "PUT", "/occurrences/"+occ+"/notes/general", body(map[string]any{"body": "done", "version": 0})), 200, "notes")
	pub := c.as(t, c.noteTaker, "POST", "/occurrences/"+occ+"/minutes/publish")
	if !strings.Contains(pub.str("data.body"), "## First") || strings.Contains(pub.str("data.body"), "Proposed rota (old)") {
		t.Fatalf("minutes list sections and only current decisions: %s", pub.Raw)
	}
	expectErr(t, c.as(t, c.organizer, "POST", "/occurrences/"+occ+"/agenda-items", body(map[string]any{"title": "late"})), 409, "OCCURRENCE_HAS_MINUTES", "published")
	expectErr(t, c.as(t, c.organizer, "DELETE", "/agenda-items/"+id1), 409, "OCCURRENCE_HAS_MINUTES", "published item")
}

// TestMeetingsGuardsAndRefusals covers the remaining refusals: organizer grants, split points, occurrence edits,
// cancelled occurrences, attendance timing and private-point validation.
func TestMeetingsGuardsAndRefusals(t *testing.T) {
	c := newMeetingCast(t)
	read, manage := c.tc.permID(t, "meeting", "read"), c.tc.permID(t, "meeting", "manage_participants")
	host := c.tc.do(t, "POST", "/roles", body(map[string]any{"name": "Host " + uniq(), "applies_to": "meeting_series", "permission_ids": []string{read, manage}}))
	expect(t, host, 201, "host role")
	r := c.weekly(t, nil)
	id := r.str("data.id")
	expect(t, c.as(t, c.organizer, "PATCH", "/meetings/"+id+"/participants/"+c.attendeeID, body(map[string]any{"role_id": host.str("data.id")})), 200, "make host")
	expectErr(t, c.as(t, c.attendee, "POST", "/meetings/"+id+"/participants", body(map[string]any{"user_id": c.eveID, "role": "meeting_organizer"})), 403, "FORBIDDEN", "host cannot grant organizer")
	expect(t, c.as(t, c.attendee, "POST", "/meetings/"+id+"/participants", body(map[string]any{"user_id": c.eveID})), 201, "host invites an attendee")
	expectErr(t, c.as(t, c.organizer, "PATCH", "/meetings/"+id+"/participants/00000000-0000-0000-0000-000000000002", body(map[string]any{"role": "meeting_viewer"})), 404, "NOT_FOUND", "not a participant")
	expectErr(t, c.as(t, c.organizer, "PATCH", "/meetings/"+id+"/participants/"+c.eveID, body(map[string]any{"kind": "vip"})), 400, "VALIDATION_ERROR", "kind")
	expectErr(t, c.as(t, c.organizer, "PATCH", "/meetings/"+id+"/participants/"+c.eveID, body(map[string]any{"role": "boss"})), 400, "VALIDATION_ERROR", "role key")
	occs := c.as(t, c.organizer, "GET", "/meetings/"+id+"/occurrences").list("data")
	first := occs[0].(map[string]any)
	expectErr(t, c.as(t, c.organizer, "POST", "/meetings/"+id+"/split", body(map[string]any{"occurrence_id": first["id"], "version": 1})), 409, "SPLIT_AT_FIRST", "split at first")
	expectErr(t, c.as(t, c.organizer, "POST", "/meetings/"+id+"/split", body(map[string]any{"occurrence_id": first["id"], "version": 9})), 409, "VERSION_CONFLICT", "split stale")
	single := c.as(t, c.organizer, "POST", "/meetings", body(map[string]any{"title": "One", "start": "2031-05-05T10:00", "tz": "UTC", "duration_min": 30}))
	expectErr(t, c.as(t, c.organizer, "POST", "/meetings/"+single.str("data.id")+"/split", body(map[string]any{"occurrence_id": single.str("data.next_occurrence.id"), "version": 1})),
		409, "NOT_RECURRING", "single meeting")
	oid := first["id"].(string)
	st, _ := time.Parse(time.RFC3339, first["start_at"].(string))
	expectErr(t, c.as(t, c.organizer, "PATCH", "/occurrences/"+oid, header("If-Match", "1"), body(map[string]any{"start_at": st.Format(time.RFC3339), "end_at": st.Add(-time.Hour).Format(time.RFC3339)})),
		400, "VALIDATION_ERROR", "end before start")
	expectErr(t, c.as(t, c.organizer, "PATCH", "/occurrences/"+oid, header("If-Match", "1"), body(map[string]any{"title": ""})), 400, "VALIDATION_ERROR", "empty title")
	loc := c.as(t, c.organizer, "PATCH", "/occurrences/"+oid, header("If-Match", "1"), body(map[string]any{"location": "Room 9", "title": "Special"}))
	if loc.str("data.location") != "Room 9" || loc.str("data.title") != "Special" {
		t.Fatalf("overrides: %s", loc.Raw)
	}
	expect(t, c.as(t, c.organizer, "POST", "/occurrences/"+oid+"/cancel", header("If-Match", "2"), body(map[string]any{})), 200, "cancel")
	expectErr(t, c.as(t, c.organizer, "POST", "/occurrences/"+oid+"/cancel", header("If-Match", "3"), body(map[string]any{})), 409, "OCCURRENCE_CANCELLED", "twice")
	expectErr(t, c.as(t, c.noteTaker, "PUT", "/occurrences/"+oid+"/response", body(map[string]any{"rsvp": "accepted"})), 409, "OCCURRENCE_CANCELLED", "answer a cancelled occurrence")
	expectErr(t, c.as(t, c.noteTaker, "PUT", "/occurrences/"+oid+"/notes/general", body(map[string]any{"body": "x", "version": 0})), 409, "OCCURRENCE_CANCELLED", "notes on cancelled")
	next := occs[1].(map[string]any)["id"].(string)
	expectErr(t, c.as(t, c.noteTaker, "PUT", "/occurrences/"+next+"/attendance", body(map[string]any{"entries": []map[string]any{{"user_id": c.noteTakerID, "state": "present"}}})),
		409, "OCCURRENCE_NOT_STARTED", "attendance before start")
	expectErr(t, c.as(t, c.noteTaker, "PUT", "/occurrences/"+next+"/attendance", body(map[string]any{"entries": []map[string]any{}})), 400, "VALIDATION_ERROR", "no entries")
	p := c.as(t, c.noteTaker, "POST", "/occurrences/"+next+"/private-points", body(map[string]any{"body": "remember"}))
	expect(t, p, 201, "point")
	expectErr(t, c.as(t, c.noteTaker, "POST", "/occurrences/"+next+"/private-points", body(map[string]any{"body": " "})), 400, "VALIDATION_ERROR", "empty point")
	expectErr(t, c.as(t, c.noteTaker, "PATCH", "/private-points/"+p.str("data.id"), body(map[string]any{"body": ""})), 400, "VALIDATION_ERROR", "empty edit")
	if e := c.as(t, c.noteTaker, "PATCH", "/private-points/"+p.str("data.id"), body(map[string]any{"body": "remember the budget"})); e.str("data.body") != "remember the budget" {
		t.Fatalf("edit point: %s", e.Raw)
	}
	expect(t, c.as(t, c.noteTaker, "DELETE", "/private-points/"+p.str("data.id")), 204, "delete point")
	ver := c.as(t, c.organizer, "GET", "/meetings/"+id).get("data.version")
	expectErr(t, c.as(t, c.organizer, "POST", "/meetings/"+id+"/cancel", body(map[string]any{"version": ver.(float64) + 1})), 409, "VERSION_CONFLICT", "stale cancel")
	expectErr(t, c.as(t, c.attendee, "POST", "/meetings/"+id+"/cancel", body(map[string]any{"version": ver})), 403, "FORBIDDEN", "host cannot cancel")
	expectErr(t, c.as(t, c.organizer, "PATCH", "/meetings/"+id, header("If-Match", "1"), body(map[string]any{"rrule": "FREQ=MINUTELY"})), 422, "INVALID_RRULE", "edit to a bad rule")
	expectErr(t, c.as(t, c.organizer, "GET", "/meetings/"+id+"/occurrences", query("from", "yesterday")), 400, "VALIDATION_ERROR", "bad window")
	expectErr(t, c.as(t, c.organizer, "GET", "/meetings/not-a-uuid"), 400, "VALIDATION_ERROR", "bad id")
	expectErr(t, c.as(t, c.organizer, "POST", "/meetings", rawBody("{")), 400, "VALIDATION_ERROR", "malformed JSON")
}

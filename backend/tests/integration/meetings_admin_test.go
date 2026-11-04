package integration

import (
	"strings"
	"testing"
	"time"
)

// TestMeetingsJ8CalendarFeed covers J8: personal feed by token, confidential meetings as busy, rotation, .ics download.
func TestMeetingsJ8CalendarFeed(t *testing.T) {
	c := newMeetingCast(t)
	expect(t, c.weekly(t, nil), 201, "weekly")
	expect(t, c.weekly(t, map[string]any{"title": "Board pay review", "confidential": true}), 201, "confidential")
	tok := c.as(t, c.attendee, "POST", "/meeting-feed-tokens", body(map[string]any{"label": "Outlook"}))
	expect(t, tok, 201, "token")
	raw := tok.str("data.token")
	if !strings.HasPrefix(tok.str("data.feed_path"), "/api/v1/calendar/meetings.ics?token=") || len(raw) < 40 {
		t.Fatalf("token shown once: %s", tok.Raw)
	}
	if l := c.as(t, c.attendee, "GET", "/meeting-feed-tokens"); strings.Contains(l.Raw, raw) {
		t.Fatalf("the secret is never listed: %s", l.Raw)
	}
	feed := call(t, "GET", "/calendar/meetings.ics", query("token", raw))
	if feed.Status != 200 || !strings.HasPrefix(feed.Raw, "BEGIN:VCALENDAR\r\n") || !strings.Contains(feed.Raw, "SUMMARY:Weekly ops review") {
		t.Fatalf("feed: %d %.300s", feed.Status, feed.Raw)
	}
	if strings.Contains(feed.Raw, "Board pay review") || !strings.Contains(feed.Raw, "SUMMARY:Busy") {
		t.Fatal("a confidential title never leaves the system")
	}
	if n := strings.Count(feed.Raw, "BEGIN:VEVENT"); n < 100 {
		t.Fatalf("a year of two weekly series: %d events", n)
	}
	expectErr(t, call(t, "GET", "/calendar/meetings.ics", query("token", "nope")), 404, "NOT_FOUND", "bad token")
	expectErr(t, c.as(t, c.eve, "DELETE", "/meeting-feed-tokens/"+tok.str("data.id")), 404, "NOT_FOUND", "someone else's token")
	expect(t, c.as(t, c.attendee, "DELETE", "/meeting-feed-tokens/"+tok.str("data.id")), 204, "revoke")
	expectErr(t, call(t, "GET", "/calendar/meetings.ics", query("token", raw)), 404, "NOT_FOUND", "revoked at once")
	r := c.weekly(t, map[string]any{"title": "Retro"})
	one := c.as(t, c.attendee, "GET", "/occurrences/"+r.str("data.next_occurrence.id")+"/ics")
	if one.Status != 200 || strings.Count(one.Raw, "BEGIN:VEVENT") != 1 || !strings.Contains(one.Raw, "SUMMARY:Retro") {
		t.Fatalf("one occurrence: %d %s", one.Status, one.Raw)
	}
	if s := c.as(t, c.eve, "GET", "/meetings/"+r.str("data.id")+"/ics"); s.Status != 200 || strings.Contains(s.Raw, "Retro") {
		t.Fatalf("an internal meeting downloads as busy blocks for non-participants: %d", s.Status)
	}
}

// TestMeetingsJ9AdminRolesAndAuditor covers J9: meeting roles in the M0 role editor, organizer guards, auditor scope.
func TestMeetingsJ9AdminRolesAndAuditor(t *testing.T) {
	c := newMeetingCast(t)
	read, propose := c.tc.permID(t, "meeting", "read"), c.tc.permID(t, "meeting", "propose_agenda")
	expectErr(t, c.tc.do(t, "POST", "/roles", body(map[string]any{"name": "Guest", "applies_to": "meeting_series", "permission_ids": []string{propose}})),
		400, "VALIDATION_ERROR", "a meeting role must include meeting:read")
	expectErr(t, c.tc.do(t, "POST", "/roles", body(map[string]any{"name": "Mixed", "applies_to": "meeting_series",
		"permission_ids": []string{read, c.tc.permID(t, "project", "read")}})), 400, "VALIDATION_ERROR", "no project permission in a meeting role")
	guest := c.tc.do(t, "POST", "/roles", body(map[string]any{"name": "Guest attendee", "applies_to": "meeting_series", "permission_ids": []string{read}}))
	expect(t, guest, 201, "custom meeting role")
	r := c.weekly(t, map[string]any{"visibility": "private"})
	id, occ := r.str("data.id"), r.str("data.next_occurrence.id")
	roles := c.as(t, c.organizer, "GET", "/meetings/"+id+"/roles")
	if len(roles.list("data")) != 5 || !roles.has("data", "name", "Guest attendee") {
		t.Fatalf("assignable roles come from M0: %s", roles.Raw)
	}
	expect(t, c.as(t, c.organizer, "POST", "/meetings/"+id+"/participants", body(map[string]any{"user_id": c.eveID, "role_id": guest.str("data.id")})), 201, "custom role")
	expectErr(t, c.as(t, c.eve, "POST", "/occurrences/"+occ+"/agenda-items", body(map[string]any{"title": "x"})), 403, "FORBIDDEN", "guest cannot propose")
	expectErr(t, c.tc.do(t, "DELETE", "/roles/"+guest.str("data.id")), 409, "", "a role in use cannot be deleted")
	expectErr(t, c.as(t, c.noteTaker, "POST", "/meetings/"+id+"/participants", body(map[string]any{"user_id": c.eveID})), 403, "FORBIDDEN", "note-taker cannot invite")
	expectErr(t, c.as(t, c.organizer, "DELETE", "/meetings/"+id+"/participants/"+c.organizerID), 409, "LAST_ORGANIZER", "last organizer stays")
	expect(t, c.as(t, c.organizer, "PATCH", "/meetings/"+id+"/participants/"+c.attendeeID, body(map[string]any{"role": "meeting_organizer"})), 200, "second organizer")
	expect(t, c.as(t, c.organizer, "DELETE", "/meetings/"+id+"/participants/"+c.organizerID), 204, "now the first may leave")
	expectErr(t, c.as(t, c.organizer, "GET", "/meetings/"+id), 404, "NOT_FOUND", "a former participant loses access")
	// Auditor: private meeting readable with published minutes and decisions, never drafts; every read audited.
	expect(t, c.as(t, c.noteTaker, "PUT", "/occurrences/"+occ+"/notes/general", body(map[string]any{"body": "draft only", "version": 0})), 200, "draft")
	auditor, _, auditorID := c.tc.limitedUser(t, [2]string{"meetings", "read_all"})
	a := c.as(t, auditor, "GET", "/meetings/"+id)
	if a.Status != 200 || a.str("data.level") != "audit" || a.get("data.participant") != false {
		t.Fatalf("auditor level: %s", a.Raw)
	}
	expect(t, c.as(t, auditor, "GET", "/meetings/"+id+"/participants"), 200, "auditor reads participants")
	expectErr(t, c.as(t, auditor, "GET", "/occurrences/"+occ+"/notes"), 403, "FORBIDDEN", "auditor never reads drafts")
	expectErr(t, c.as(t, auditor, "PUT", "/occurrences/"+occ+"/notes/general", body(map[string]any{"body": "x", "version": 1})), 403, "FORBIDDEN", "auditor is read-only")
	if n := countRows(t, "SELECT count(*) FROM audit_logs WHERE user_id = ? AND action = 'meeting.read_privileged'", auditorID); n < 1 {
		t.Fatalf("privileged reads are audited: %d", n)
	}
	conf := c.weekly(t, map[string]any{"title": "Layoffs", "visibility": "private", "confidential": true})
	expectErr(t, c.as(t, auditor, "GET", "/meetings/"+conf.str("data.id")), 404, "NOT_FOUND", "confidential needs a second permission")
	senior, _, _ := c.tc.limitedUser(t, [2]string{"meetings", "read_all"}, [2]string{"meetings", "read_confidential"})
	expect(t, c.as(t, senior, "GET", "/meetings/"+conf.str("data.id")), 200, "read_confidential")
}

// TestMeetingsJ11Deactivation covers J11: a departed user leaves every meeting, history stays, the admin sees orphans.
func TestMeetingsJ11Deactivation(t *testing.T) {
	c := newMeetingCast(t)
	admin, _, _ := c.tc.limitedUser(t, [2]string{"meetings", "manage_all"})
	_, occ := c.startedMeeting(t, nil)
	expect(t, c.as(t, c.noteTaker, "PUT", "/occurrences/"+occ+"/attendance", body(map[string]any{"entries": []map[string]any{{"user_id": c.attendeeID, "state": "present"}}})), 200, "attendance")
	tok := c.as(t, c.attendee, "POST", "/meeting-feed-tokens", body(map[string]any{}))
	expect(t, c.tc.do(t, "POST", "/users/"+c.attendeeID+"/deactivate"), 200, "deactivate attendee")
	expect(t, c.tc.do(t, "POST", "/users/"+c.organizerID+"/deactivate"), 200, "deactivate organizer")
	if n := countRows(t, "SELECT count(*) FROM resource_role_assignments WHERE user_id = ? AND resource_type = 'meeting_series'", c.attendeeID); n != 0 {
		t.Fatalf("M0 roles revoked: %d", n)
	}
	att := c.as(t, c.noteTaker, "GET", "/occurrences/"+occ+"/attendance")
	if !att.has("data", "state", "present") {
		t.Fatalf("attendance history stays: %s", att.Raw)
	}
	d := c.as(t, c.noteTaker, "GET", "/occurrences/"+occ)
	if strings.Contains(d.Raw, c.attendeeID) {
		t.Fatalf("an inactive user is no longer listed as invited: %s", d.Raw)
	}
	expectErr(t, call(t, "GET", "/calendar/meetings.ics", query("token", tok.str("data.token"))), 404, "NOT_FOUND", "feed token revoked")
	orphans := c.as(t, admin, "GET", "/meetings/needs-organizer")
	if !orphans.has("data", "title", "Ops sync") {
		t.Fatalf("the admin sees series without organizer: %s", orphans.Raw)
	}
	expectErr(t, c.as(t, c.noteTaker, "GET", "/meetings/needs-organizer"), 403, "FORBIDDEN", "manage_all only")
}

// TestMeetingsJ12SearchAndEvents covers J12 (search by title and participant) and the event contract (ids only).
func TestMeetingsJ12SearchAndEvents(t *testing.T) {
	c := newMeetingCast(t)
	r := c.weekly(t, map[string]any{"title": "Quarterly planning", "visibility": "private"})
	id := r.str("data.id")
	hits := c.as(t, c.attendee, "GET", "/meetings/search", query("q", "quarterly"))
	if !hits.has("data", "id", id) || hits.get("data.0.next_start") == nil {
		t.Fatalf("participant finds it: %s", hits.Raw)
	}
	if e := c.as(t, c.eve, "GET", "/meetings/search", query("q", "quarterly")); len(e.list("data")) != 0 {
		t.Fatalf("eve finds nothing: %s", e.Raw)
	}
	if p := c.as(t, c.organizer, "GET", "/meetings/search", query("participant_id", c.noteTakerID)); !p.has("data", "id", id) {
		t.Fatalf("by participant: %s", p.Raw)
	}
	expectErr(t, c.as(t, c.organizer, "GET", "/meetings/search"), 400, "VALIDATION_ERROR", "a criterion is required")
	expectErr(t, c.as(t, c.organizer, "GET", "/meetings", query("from", time.Now().UTC().Format(time.RFC3339), "to", time.Now().UTC().AddDate(0, 3, 0).Format(time.RFC3339))),
		400, "VALIDATION_ERROR", "window at most 62 days")
	var payloads []string
	if err := testDB.Raw(`SELECT payload::text FROM meetings_events_outbox WHERE payload->'payload'->>'series_id' = ?`, id).Scan(&payloads).Error; err != nil {
		t.Fatal(err)
	}
	if len(payloads) == 0 {
		t.Fatal("meeting.scheduled is written to the outbox")
	}
	for _, p := range payloads {
		if strings.Contains(p, "Quarterly") {
			t.Fatalf("events carry ids only, never titles: %s", p)
		}
	}
}

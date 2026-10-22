package integration

import (
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestLeaveParity — LG1..LG5: non-calendar leave year, per-location calendars, manager inbox, comp-off and encashment.
func TestLeaveParity(t *testing.T) {
	cc := newCoreTenant(t)
	const base = "/hr/leave"
	now := time.Now().UTC().Truncate(24 * time.Hour)

	// A manager (login only) and a location.
	bossUser, bossEmail := cc.newUser(t, "boss")
	bossEmp := cc.do(t, "POST", "/hr/employees", body(empBody(bossUser, "BOSS-"+strings.ToUpper(uniq()), nil)))
	expect(t, bossEmp, 201, "boss profile")
	bossTok := login(t, cc.slug, bossEmail, password).str("data.access_token")
	loc := cc.do(t, "POST", "/hr/locations", body(map[string]any{"code": "lp" + uniq(), "name": "Leave site " + uniq(), "city": "Pune", "state_code": "mh"}))
	expect(t, loc, 201, "location")
	locID := loc.str("data.id")

	// LG1 leave year starting in April; it locks once balances exist.
	expect(t, cc.do(t, "PUT", base+"/policy", body(map[string]any{"year_start_month": 4, "comp_off_expiry_days": 90})), 200, "policy")
	el := cc.leaveType(t, "EL", 12)
	co := cc.leaveType(t, "CO", 0)
	cc.complete(t, "hire", cc.memberEmp, cc.day(-200), map[string]any{"position_id": cc.p1, "manager_employee_id": bossEmp.str("data.id"),
		"worker_type": "employee", "employment_type": "full_time", "location_id": locID})
	b := cc.balance(t, cc.memberTok, el)
	wantYear := now.Year()
	if now.Month() < time.April {
		wantYear--
	}
	if b["accrued"] != 12.0 {
		t.Fatalf("LG1 accrual: %v", b)
	}
	if y := sqlStr(t, "SELECT year::text FROM leave_balances WHERE employee_profile_id = ? AND leave_type_id = ?", cc.memberEmp, el); y != strconv.Itoa(wantYear) {
		t.Fatalf("LG1 leave year label: %s want %d", y, wantYear)
	}
	expectErr(t, cc.do(t, "PUT", base+"/policy", body(map[string]any{"year_start_month": 1, "comp_off_expiry_days": 90})), 409, "YEAR_LOCKED", "LG1 year locked")
	expectErr(t, cc.do(t, "PUT", base+"/policy", body(map[string]any{"year_start_month": 13, "comp_off_expiry_days": 90})), 400, "VALIDATION_ERROR", "month range")

	// LG2 location calendar: Sunday-only weekly off and a local holiday change pricing for that location only.
	sat, mon := monday(0).AddDate(0, 0, 5), monday(0)
	preview := func(tok string, from, to time.Time) resp {
		return cc.as(t, tok, "POST", base+"/requests/preview", body(map[string]string{"leave_type_id": el, "start_date": ymd(from), "end_date": ymd(to), "reason": "x"}))
	}
	expectErr(t, preview(cc.memberTok, sat, sat), 400, "NO_WORKING_DAYS", "LG2 Saturday is off by default")
	expect(t, cc.do(t, "PUT", base+"/week-offs/"+locID, body(map[string]any{"week_off_days": []int{0}})), 200, "week off")
	if p := preview(cc.memberTok, sat, sat); num(p, "data.total_days") != 1 {
		t.Fatalf("LG2 Saturday works at the location: %s", p.Raw)
	}
	expectErr(t, preview(cc.token, sat, sat), 400, "NO_WORKING_DAYS", "LG2 other locations keep Saturday off")
	hol := cc.do(t, "POST", base+"/location-holidays", body(map[string]any{"location_id": locID, "date": ymd(mon), "name": "Local festival"}))
	expect(t, hol, 201, "location holiday")
	expectErr(t, cc.do(t, "POST", base+"/location-holidays", body(map[string]any{"location_id": locID, "date": ymd(mon), "name": "Again"})), 409, "CONFLICT", "duplicate")
	expectErr(t, cc.do(t, "POST", base+"/location-holidays", body(map[string]any{"location_id": "00000000-0000-4000-8000-000000000000", "date": ymd(mon), "name": "X"})), 404, "LOCATION_NOT_FOUND", "unknown location")
	if p := preview(cc.memberTok, mon, mon.AddDate(0, 0, 1)); num(p, "data.total_days") != 1 {
		t.Fatalf("LG2 holiday removes a day at the location: %s", p.Raw)
	}
	if p := preview(cc.token, mon, mon.AddDate(0, 0, 1)); num(p, "data.total_days") != 2 {
		t.Fatalf("LG2 other locations pay both days: %s", p.Raw)
	}
	if l := cc.do(t, "GET", base+"/location-holidays", query("location_id", locID, "year", strconv.Itoa(mon.Year()))); len(l.list("data")) != 1 {
		t.Fatalf("holiday list: %s", l.Raw)
	}
	if w := cc.do(t, "GET", base+"/week-offs"); len(w.list("data")) != 1 {
		t.Fatalf("week-off list: %s", w.Raw)
	}

	// LG3 manager inbox: only the boss (and tenant approvers) decide the member's request.
	req := cc.apply(t, cc.memberTok, el, monday(1), monday(1).AddDate(0, 0, 1))
	expect(t, req, 201, "member applies")
	rid := req.str("data.id")
	if in := cc.as(t, bossTok, "GET", base+"/inbox"); len(in.list("data")) != 1 || in.str("data.0.id") != rid {
		t.Fatalf("LG3 boss inbox: %s", in.Raw)
	}
	_, peerEmail := cc.newUser(t, "peer")
	peerTok := login(t, cc.slug, peerEmail, password).str("data.access_token")
	if in := cc.as(t, peerTok, "GET", base+"/inbox"); len(in.list("data")) != 0 {
		t.Fatalf("LG3 stranger inbox: %s", in.Raw)
	}
	expectErr(t, cc.as(t, peerTok, "POST", base+"/inbox/"+rid+"/approve"), 404, "NOT_FOUND", "LG3 stranger decides")
	expectErr(t, cc.as(t, bossTok, "GET", base+"/inbox", query("scope", "all")), 403, "FORBIDDEN", "scope=all")
	cal := cc.as(t, bossTok, "GET", base+"/team-calendar", query("from", ymd(monday(1)), "to", ymd(monday(1).AddDate(0, 0, 6))))
	if expect(t, cal, 200, "team calendar"); len(cal.list("data.leaves")) != 1 || strings.Contains(cal.Raw, "family wedding") {
		t.Fatalf("LG3 calendar must list the leave without its reason: %s", cal.Raw)
	}
	if c := cc.as(t, peerTok, "GET", base+"/team-calendar", query("from", ymd(monday(1)), "to", ymd(monday(1).AddDate(0, 0, 6)))); len(c.list("data.leaves")) != 0 {
		t.Fatalf("LG3 stranger calendar: %s", c.Raw)
	}
	expect(t, cc.as(t, bossTok, "POST", base+"/inbox/"+rid+"/approve", body(map[string]string{"comment": "enjoy"})), 200, "boss approves")
	if s := sqlStr(t, "SELECT status FROM leave_requests WHERE id = ?", rid); s != "approved" {
		t.Fatalf("LG3 status: %s", s)
	}

	// LG4 comp-off: real work on a weekly-off Sunday, credited once, expired without going negative.
	sun := now.AddDate(0, 0, -3)
	for sun.Weekday() != time.Sunday {
		sun = sun.AddDate(0, 0, -1)
	}
	if now.Sub(sun) > 28*24*time.Hour {
		t.Fatal("test date out of range")
	}
	expect(t, cc.do(t, "PUT", base+"/policy", body(map[string]any{"year_start_month": 4, "comp_off_leave_type_id": co, "comp_off_expiry_days": 1})), 200, "comp-off policy")
	member := cc.as
	cReq := map[string]any{"worked_date": ymd(sun), "days": 1, "reason": "release support"}
	expectErr(t, member(t, cc.memberTok, "POST", base+"/comp-off", body(map[string]any{"worked_date": ymd(sun.AddDate(0, 0, 3)), "days": 1, "reason": "x"})), 409, "NOT_A_NON_WORKING_DAY", "LG4 weekday")
	expectErr(t, member(t, cc.memberTok, "POST", base+"/comp-off", body(cReq)), 409, "NO_WORK_RECORDED", "LG4 no attendance")
	row := map[string]string{"employee_code": cc.do(t, "GET", "/hr/employees/"+cc.memberEmp).str("data.employee_code"), "date": ymd(sun),
		"punch_in": ymd(sun) + "T09:00:00Z", "punch_out": ymd(sun) + "T15:00:00Z"}
	if imp := cc.do(t, "POST", "/hr/attendance/imports", body(map[string]any{"rows": []map[string]string{row}})); imp.get("data.applied") != 1.0 {
		t.Fatalf("import: %s", imp.Raw)
	}
	cr := member(t, cc.memberTok, "POST", base+"/comp-off", body(cReq))
	expect(t, cr, 201, "comp-off request")
	cid := cr.str("data.id")
	expectErr(t, member(t, cc.memberTok, "POST", base+"/comp-off", body(cReq)), 409, "CONFLICT", "LG4 duplicate")
	expectErr(t, member(t, cc.memberTok, "POST", base+"/comp-off/"+cid+"/approve"), 403, "SELF_APPROVAL_FORBIDDEN", "LG4 own approval")
	expectErr(t, cc.as(t, peerTok, "POST", base+"/comp-off/"+cid+"/approve"), 404, "NOT_FOUND", "LG4 stranger")
	expect(t, cc.as(t, bossTok, "POST", base+"/comp-off/"+cid+"/approve", body(map[string]string{"comment": "thanks"})), 200, "boss approves comp-off")
	if b := cc.balance(t, cc.memberTok, co); b["available"] != 1.0 {
		t.Fatalf("LG4 credit: %v", b)
	}
	expectErr(t, cc.as(t, bossTok, "POST", base+"/comp-off/"+cid+"/approve"), 409, "REQUEST_STATE", "credited once")
	if ex := cc.do(t, "POST", base+"/comp-off/expire"); ex.get("data.expired") != 1.0 || ex.get("data.days_debited") != 1.0 {
		t.Fatalf("LG4 expiry: %s", ex.Raw)
	}
	if b := cc.balance(t, cc.memberTok, co); b["available"] != 0.0 {
		t.Fatalf("LG4 expired credit leaves nothing: %v", b)
	}
	if ex := cc.do(t, "POST", base+"/comp-off/expire"); ex.get("data.expired") != 0.0 {
		t.Fatalf("LG4 idempotent: %s", ex.Raw)
	}
	if l := cc.as(t, cc.memberTok, "GET", base+"/comp-off/me"); len(l.list("data")) != 1 {
		t.Fatalf("my comp-offs: %s", l.Raw)
	}

	// LG5 encashment: 3 days a year, 2 days must stay.
	expect(t, cc.do(t, "PUT", base+"/encashment-rules/"+el, body(map[string]any{"max_days_per_year": 3, "min_balance_after": 2})), 200, "rule")
	expectErr(t, member(t, cc.memberTok, "POST", base+"/encashments", body(map[string]any{"leave_type_id": co, "days": 1})), 409, "NOT_ENCASHABLE", "no rule")
	expectErr(t, member(t, cc.memberTok, "POST", base+"/encashments", body(map[string]any{"leave_type_id": el, "days": 4})), 409, "ENCASH_LIMIT", "over the yearly cap")
	en := member(t, cc.memberTok, "POST", base+"/encashments", body(map[string]any{"leave_type_id": el, "days": 2, "reason": "cash needed"}))
	expect(t, en, 201, "encashment")
	eid := en.str("data.id")
	expectErr(t, member(t, cc.memberTok, "POST", base+"/encashments/"+eid+"/approve"), 403, "SELF_APPROVAL_FORBIDDEN", "own approval")
	before := cc.balance(t, cc.memberTok, el)["available"].(float64)
	expect(t, cc.as(t, bossTok, "POST", base+"/encashments/"+eid+"/approve", body(map[string]string{"comment": "ok"})), 200, "boss approves")
	if after := cc.balance(t, cc.memberTok, el)["available"].(float64); after != before-2 {
		t.Fatalf("LG5 debit: %v → %v", before, after)
	}
	expectErr(t, cc.as(t, bossTok, "POST", base+"/encashments/"+eid+"/approve"), 409, "REQUEST_STATE", "debited once")
	expectErr(t, member(t, cc.memberTok, "POST", base+"/encashments", body(map[string]any{"leave_type_id": el, "days": 2})), 409, "ENCASH_LIMIT", "2 + 2 > 3")
	if l := cc.do(t, "GET", base+"/encashments", query("status", "approved")); len(l.list("data")) != 1 {
		t.Fatalf("encashment list: %s", l.Raw)
	}
	if n := sqlStr(t, "SELECT count(*)::text FROM leave_events_outbox WHERE tenant_id = ? AND event_type IN ('leave.comp_off_credited', 'leave.encashment_approved') AND (payload::text LIKE '%release support%' OR payload::text LIKE '%cash needed%')", cc.id); n != "0" {
		t.Fatalf("LP-17 events carry no reasons: %s", n)
	}
	if n := sqlStr(t, "SELECT count(*)::text FROM leave_events_outbox WHERE tenant_id = ? AND event_type IN ('leave.comp_off_credited', 'leave.encashment_approved')", cc.id); n != "2" {
		t.Fatalf("events: %s", n)
	}

	// RBAC and isolation.
	for _, p := range [][2]string{{"PUT", base + "/policy"}, {"POST", base + "/location-holidays"}, {"PUT", base + "/week-offs/" + locID}, {"POST", base + "/comp-off/expire"}, {"PUT", base + "/encashment-rules/" + el}} {
		expectErr(t, cc.as(t, cc.memberTok, p[0], p[1], body(map[string]any{})), 403, "FORBIDDEN", "member "+p[0]+" "+p[1])
	}
	expectErr(t, cc.as(t, cc.memberTok, "GET", base+"/comp-off"), 403, "FORBIDDEN", "member lists all")
	other := newTenant(t)
	expectErr(t, other.do(t, "POST", base+"/encashments/"+eid+"/approve"), 404, "NOT_FOUND", "foreign encashment")
	expect(t, cc.do(t, "DELETE", base+"/location-holidays/"+hol.str("data.id")), 204, "delete holiday")
	expect(t, cc.do(t, "DELETE", base+"/week-offs/"+locID), 204, "delete week off")
}

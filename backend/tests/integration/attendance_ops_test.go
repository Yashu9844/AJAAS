package integration

import (
	"testing"
	"time"
)

// TestAttendanceParity — AG1..AG5: manager inbox, policy and overtime, rosters, auto-absent and bulk import.
func TestAttendanceParity(t *testing.T) {
	tc := newTimeTenant(t)
	const base = "/hr/attendance"
	now := time.Now().UTC()
	yesterday := now.AddDate(0, 0, -1)
	y := ymd(yesterday)
	weekend := func(d time.Time) bool { return d.Weekday() == time.Saturday || d.Weekday() == time.Sunday }

	// A manager (login only, no HR permission) with the member as primary report.
	bossUser, bossEmail := tc.newUser(t, "boss")
	bossTok := login(t, tc.slug, bossEmail, password).str("data.access_token")
	_, peerEmail := tc.newUser(t, "peer")
	peerTok := login(t, tc.slug, peerEmail, password).str("data.access_token")
	dept := tc.mkDept(t, "Parity")
	expect(t, tc.do(t, "POST", "/hr/mappings", body(map[string]any{"user_id": tc.memberUser, "department_id": dept, "manager_user_id": bossUser, "is_primary": true})), 201, "mapping")

	// AG1 inbox: only the manager's reports; a stranger cannot decide.
	req := map[string]string{"attendance_date": y, "requested_punch_in": y + "T09:00:00Z", "requested_punch_out": y + "T18:00:00Z", "reason": "forgot"}
	reg := tc.as(t, tc.memberTok, "POST", base+"/regularizations", body(req))
	expect(t, reg, 201, "member request")
	rid := reg.str("data.id")
	if in := tc.as(t, bossTok, "GET", base+"/inbox"); len(in.list("data")) != 1 || in.str("data.0.employee_id") != tc.memberEmp {
		t.Fatalf("AG1 manager inbox: %s", in.Raw)
	}
	if in := tc.as(t, peerTok, "GET", base+"/inbox"); len(in.list("data")) != 0 {
		t.Fatalf("AG1 stranger inbox: %s", in.Raw)
	}
	expectErr(t, tc.as(t, bossTok, "GET", base+"/inbox", query("scope", "all")), 403, "FORBIDDEN", "scope=all needs approve")
	if in := tc.do(t, "GET", base+"/inbox", query("scope", "all")); len(in.list("data")) != 1 {
		t.Fatalf("AG1 hr inbox: %s", in.Raw)
	}
	expectErr(t, tc.as(t, peerTok, "POST", base+"/inbox/"+rid+"/approve"), 404, "NOT_FOUND", "AG1 stranger decides")
	expect(t, tc.as(t, bossTok, "POST", base+"/inbox/"+rid+"/approve", body(map[string]string{"comment": "ok"})), 200, "manager approves")
	expectErr(t, tc.as(t, bossTok, "POST", base+"/inbox/"+rid+"/reject", body(map[string]string{"comment": "late"})), 409, "REGULARIZATION_NOT_PENDING", "decided twice")

	// AG2 policy and overtime: 540 worked minutes yesterday against the 480 default.
	pol := map[string]any{"week_off_days": []int{0, 6}, "overtime_min_daily": 30, "overtime_max_daily": 240, "overtime_max_monthly": 3600,
		"overtime_rate_weekday": 150, "overtime_rate_week_off": 200, "auto_absent_enabled": true}
	expect(t, tc.do(t, "PUT", base+"/policy", body(pol)), 200, "policy")
	expectErr(t, tc.do(t, "PUT", base+"/policy", body(map[string]any{"week_off_days": []int{9}})), 400, "VALIDATION_ERROR", "bad policy")
	if p := tc.do(t, "GET", base+"/policy"); p.get("data.overtime_rate_week_off") != 200.0 || len(p.list("data.week_off_days")) != 2 {
		t.Fatalf("policy: %s", p.Raw)
	}
	wantMin, wantRate := 60, 150
	if weekend(yesterday) {
		wantMin, wantRate = 240, 200
	}
	ot := tc.do(t, "GET", base+"/overtime/report", query("month", yesterday.Format("2006-01"), "employee_id", tc.memberEmp))
	if expect(t, ot, 200, "overtime"); len(ot.list("data.lines")) != 1 || ot.get("data.lines.0.eligible_minutes") != float64(wantMin) ||
		ot.get("data.lines.0.weighted_minutes") != float64(wantMin*wantRate/100) {
		t.Fatalf("AG2 overtime: %s", ot.Raw)
	}

	// AG3 rosters: segments laid end to end; an overlap changes nothing.
	mkShift := func(name, start, end string) string {
		s := tc.do(t, "POST", "/hr/shifts", body(map[string]any{"name": name + " " + uniq(), "start_time": start, "end_time": end, "grace_period_mins": 0, "break_duration_mins": 0}))
		expect(t, s, 201, "shift")
		return s.str("data.id")
	}
	early, late := mkShift("Early", "06:00", "14:00"), mkShift("Late", "14:00", "22:00")
	ro := tc.do(t, "POST", base+"/rosters", body(map[string]any{"name": "Rotation " + uniq(), "pattern": []map[string]any{{"shift_id": early, "days": 7}, {"shift_id": late, "days": 7}}}))
	expect(t, ro, 201, "roster")
	roID := ro.str("data.id")
	from := ymd(now.AddDate(0, 0, 60))
	ap := tc.do(t, "POST", base+"/rosters/"+roID+"/apply", body(map[string]any{"employee_ids": []string{tc.memberEmp}, "from": from, "cycles": 2}))
	if expect(t, ap, 200, "apply"); ap.get("data.assignments") != 4.0 {
		t.Fatalf("AG3 apply: %s", ap.Raw)
	}
	if n := sqlStr(t, "SELECT count(*)::text FROM shift_assignments WHERE employee_profile_id = ?", tc.memberEmp); n != "4" {
		t.Fatalf("AG3 assignments: %s", n)
	}
	expectErr(t, tc.do(t, "POST", base+"/rosters/"+roID+"/apply", body(map[string]any{"employee_ids": []string{tc.adminEmp, tc.memberEmp}, "from": from, "cycles": 1})), 409, "ROSTER_OVERLAP", "AG3 overlap")
	if n := sqlStr(t, "SELECT count(*)::text FROM shift_assignments WHERE employee_profile_id = ?", tc.adminEmp); n != "0" {
		t.Fatalf("AG3 nothing for the others: %s", n)
	}
	expect(t, tc.do(t, "PATCH", base+"/rosters/"+roID, body(map[string]any{"status": "archived"})), 200, "archive")
	expectErr(t, tc.do(t, "POST", base+"/rosters/"+roID+"/apply", body(map[string]any{"employee_ids": []string{tc.adminEmp}, "from": from, "cycles": 1})), 409, "ROSTER_ARCHIVED", "archived")
	expectErr(t, tc.do(t, "POST", base+"/rosters", body(map[string]any{"name": ro.str("data.name"), "pattern": []map[string]any{{"shift_id": early, "days": 1}}})), 409, "CONFLICT", "duplicate name")

	// AG4 auto-absent: the regularized member is untouched, the admin gets a system record.
	job := tc.do(t, "POST", base+"/jobs/auto-absent", body(map[string]string{"date": y}))
	if expect(t, job, 200, "auto-absent"); job.get("data.skipped") == nil {
		t.Fatalf("AG4 job: %s", job.Raw)
	}
	wantStatus := "absent"
	if weekend(yesterday) {
		wantStatus = "week_off"
	}
	if s := sqlStr(t, "SELECT status||'/'||source FROM attendance_records WHERE employee_profile_id = ? AND attendance_date = ?", tc.adminEmp, y); s != wantStatus+"/system" {
		t.Fatalf("AG4 admin record: %s", s)
	}
	if s := sqlStr(t, "SELECT status||'/'||source FROM attendance_records WHERE employee_profile_id = ? AND attendance_date = ?", tc.memberEmp, y); s != "present/regularization" {
		t.Fatalf("AG4 member untouched: %s", s)
	}
	again := tc.do(t, "POST", base+"/jobs/auto-absent", body(map[string]string{"date": y}))
	if again.get("data.absent") != 0.0 || again.get("data.week_off") != 0.0 {
		t.Fatalf("AG4 idempotent: %s", again.Raw)
	}
	hday := now.AddDate(0, 0, -2)
	expect(t, tc.do(t, "POST", "/hr/leave/holidays", body(map[string]string{"date": ymd(hday), "name": "Founders day"})), 201, "holiday")
	hj := tc.do(t, "POST", base+"/jobs/auto-absent", body(map[string]string{"date": ymd(hday)}))
	if expect(t, hj, 200, "holiday job"); hj.get("data.holiday").(float64) < 1 {
		t.Fatalf("AG4 holiday: %s", hj.Raw)
	}
	expectErr(t, tc.do(t, "POST", base+"/jobs/auto-absent", body(map[string]string{"date": ymd(now)})), 400, "JOB_WINDOW", "today")
	if runs := tc.do(t, "GET", base+"/jobs/runs"); len(runs.list("data")) != 3 {
		t.Fatalf("job runs: %s", runs.Raw)
	}

	// AG5 bulk import: atomic, dry run, overwrite.
	code := tc.do(t, "GET", "/hr/employees/"+tc.memberEmp).str("data.employee_code")
	d5 := now.AddDate(0, 0, -5)
	good := map[string]string{"employee_code": code, "date": ymd(d5), "punch_in": ymd(d5) + "T09:00:00Z", "punch_out": ymd(d5) + "T18:00:00Z"}
	badRow := map[string]string{"employee_code": "NOPE-" + uniq(), "date": ymd(d5), "punch_in": ymd(d5) + "T09:00:00Z", "punch_out": ymd(d5) + "T18:00:00Z"}
	mixed := tc.do(t, "POST", base+"/imports", body(map[string]any{"rows": []map[string]string{good, badRow}}))
	if expect(t, mixed, 200, "mixed import"); mixed.get("data.applied") != 0.0 || len(mixed.list("data.errors")) != 1 || mixed.get("data.errors.0.row") != 2.0 {
		t.Fatalf("AG5 atomic: %s", mixed.Raw)
	}
	if n := sqlStr(t, "SELECT count(*)::text FROM attendance_records WHERE employee_profile_id = ? AND attendance_date = ?", tc.memberEmp, ymd(d5)); n != "0" {
		t.Fatalf("AG5 nothing written: %s", n)
	}
	if dry := tc.do(t, "POST", base+"/imports", body(map[string]any{"dry_run": true, "rows": []map[string]string{good}})); dry.get("data.applied") != 0.0 || len(dry.list("data.errors")) != 0 {
		t.Fatalf("AG5 dry run: %s", dry.Raw)
	}
	done := tc.do(t, "POST", base+"/imports", body(map[string]any{"rows": []map[string]string{good}}))
	if expect(t, done, 200, "import"); done.get("data.applied") != 1.0 {
		t.Fatalf("AG5 applied: %s", done.Raw)
	}
	if s := sqlStr(t, "SELECT source||'/'||total_work_minutes FROM attendance_records WHERE employee_profile_id = ? AND attendance_date = ?", tc.memberEmp, ymd(d5)); s != "import/540" {
		t.Fatalf("AG5 record: %s", s)
	}
	exists := tc.do(t, "POST", base+"/imports", body(map[string]any{"rows": []map[string]string{good}}))
	if exists.get("data.errors.0.code") != "EXISTS" {
		t.Fatalf("AG5 exists: %s", exists.Raw)
	}
	expect(t, tc.do(t, "POST", base+"/imports", body(map[string]any{"overwrite": true, "rows": []map[string]string{good}})), 200, "overwrite")
	if l := tc.do(t, "GET", base+"/imports"); len(l.list("data")) != 2 {
		t.Fatalf("import log: %s", l.Raw)
	}
	if n := sqlStr(t, "SELECT count(*)::text FROM audit_logs WHERE tenant_id = ? AND action LIKE 'attendance.import%' AND metadata::text LIKE '%T09:00%'", tc.id); n != "0" {
		t.Fatalf("AP-12 audit leaks punch times: %s", n)
	}

	// RBAC and isolation.
	for _, p := range [][2]string{{"PUT", base + "/policy"}, {"POST", base + "/imports"}, {"POST", base + "/rosters"}, {"POST", base + "/jobs/auto-absent"}} {
		expectErr(t, tc.as(t, tc.memberTok, p[0], p[1], body(map[string]any{})), 403, "FORBIDDEN", "member "+p[0]+" "+p[1])
	}
	expectErr(t, tc.as(t, tc.memberTok, "GET", base+"/overtime/report", query("month", "2026-06")), 403, "FORBIDDEN", "member overtime")
	other := newTenant(t)
	expectErr(t, other.do(t, "GET", base+"/rosters/"+roID), 404, "NOT_FOUND", "foreign roster")
}

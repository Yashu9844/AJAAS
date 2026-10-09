package integration

import (
	"strings"
	"sync"
	"testing"
	"time"
)

// TestAttendancePunchFlow — G2 alternation + AT-004 precedence (D3-13), G11 audit, G12 outbox privacy, reads.
func TestAttendancePunchFlow(t *testing.T) {
	tc := newTimeTenant(t)
	s := tc.do(t, "POST", "/shifts", body(map[string]any{"name": "Day " + uniq(), "start_time": "00:00", "end_time": "23:59",
		"grace_period_mins": 0, "break_duration_mins": 0}))
	expect(t, s, 201, "create shift")
	shiftID := s.str("data.id")
	expect(t, tc.do(t, "POST", "/shifts/"+shiftID+"/assignments", body(map[string]string{
		"employee_id": tc.adminEmp, "effective_from": ymd(time.Now().UTC().AddDate(0, 0, -30))})), 201, "assign shift")

	in := tc.do(t, "POST", "/attendance/punch", body(map[string]any{"type": "in", "latitude": 12.9716, "longitude": 77.5946, "device_id": "e2e-kiosk"}))
	expect(t, in, 201, "punch in")
	recordID := in.str("data.record.id")
	if in.str("data.record.shift_id") != shiftID || in.str("data.record.status") != "present" || strings.Contains(in.Raw, "ip_address") {
		t.Fatalf("record after IN: %s", in.Raw)
	}
	expectErr(t, tc.do(t, "POST", "/attendance/punch", body(map[string]string{"type": "in"})), 409, "DUPLICATE_PUNCH", "AT-004 wins inside 60 s")
	backdate(t, tc.adminEmp)
	expectErr(t, tc.do(t, "POST", "/attendance/punch", body(map[string]string{"type": "in"})), 409, "ALREADY_PUNCHED_IN", "G2 second IN")
	if today := tc.do(t, "GET", "/attendance/me/today"); today.get("data.open_session") != true {
		t.Fatalf("open session expected: %s", today.Raw)
	}
	expect(t, tc.do(t, "POST", "/attendance/punch", body(map[string]string{"type": "out"})), 201, "punch out")
	backdate(t, tc.adminEmp)
	expectErr(t, tc.do(t, "POST", "/attendance/punch", body(map[string]string{"type": "out"})), 409, "NOT_PUNCHED_IN", "G2 OUT without IN")

	if d := tc.do(t, "GET", "/attendance/records/"+recordID); len(d.list("data.punches")) != 2 {
		t.Fatalf("detail: %s", d.Raw)
	}
	if l := tc.do(t, "GET", "/attendance/records", query("employee_id", tc.adminEmp)); len(l.list("data")) != 1 {
		t.Fatalf("records list: %s", l.Raw)
	}
	if sum := tc.do(t, "GET", "/attendance/summary"); num(sum, "data.total_employees") < 2 {
		t.Fatalf("summary: %s", sum.Raw)
	}
	if n := sqlStr(t, "SELECT count(*) FROM audit_logs WHERE tenant_id = ? AND action IN ('attendance.punch_in','attendance.punch_out','shift.created','shift.assigned')", tc.id); n != "4" {
		t.Fatalf("G11 audit rows = %s", n)
	}
	if n := sqlStr(t, "SELECT count(*) FROM audit_logs WHERE tenant_id = ? AND action LIKE 'attendance.%' AND (metadata::text LIKE '%12.97%' OR metadata::text LIKE '%kiosk%')", tc.id); n != "0" {
		t.Fatalf("G11 audit leaks location/device: %s", n)
	}
	if n := sqlStr(t, "SELECT count(*) FROM attendance_events_outbox WHERE tenant_id = ? AND routing_key LIKE 'attendance.punch.%'", tc.id); n != "2" {
		t.Fatalf("G12 outbox rows = %s", n)
	}
	if n := sqlStr(t, "SELECT count(*) FROM attendance_events_outbox WHERE tenant_id = ? AND (payload::text LIKE '%latitude%' OR payload::text LIKE '%kiosk%' OR payload::text LIKE '%ip_address%')", tc.id); n != "0" {
		t.Fatalf("G12 payload leaks: %s", n)
	}
}

// backdate shifts an employee's punches 2 minutes back so AT-004 (60 s debounce) does not mask AT-003.
func backdate(t *testing.T, employeeID string) {
	t.Helper()
	if err := testDB.Exec("UPDATE attendance_punches SET punch_time = punch_time - interval '2 minutes' WHERE employee_profile_id = ?", employeeID).Error; err != nil {
		t.Fatal(err)
	}
}

// TestAttendanceRegularization — G6 lifecycle, G7 self-approval, G8 one pending, AT-018 cancel.
func TestAttendanceRegularization(t *testing.T) {
	tc := newTimeTenant(t)
	y := ymd(time.Now().UTC().AddDate(0, 0, -1))
	req := map[string]string{"attendance_date": y, "requested_punch_in": y + "T09:00:00Z", "requested_punch_out": y + "T18:00:00Z", "reason": "forgot"}
	m := tc.as(t, tc.memberTok, "POST", "/attendance/regularizations", body(req))
	expect(t, m, 201, "member request")
	expectErr(t, tc.as(t, tc.memberTok, "POST", "/attendance/regularizations", body(req)), 409, "REGULARIZATION_PENDING", "G8")
	own := tc.do(t, "POST", "/attendance/regularizations", body(req))
	expect(t, own, 201, "admin own request")
	expectErr(t, tc.do(t, "POST", "/attendance/regularizations/"+own.str("data.id")+"/approve"), 403, "SELF_APPROVAL_FORBIDDEN", "G7")
	expect(t, tc.do(t, "POST", "/attendance/regularizations/"+own.str("data.id")+"/cancel"), 200, "AT-018 cancel own")
	expect(t, tc.do(t, "POST", "/attendance/regularizations/"+m.str("data.id")+"/approve", body(map[string]string{"comment": "ok"})), 200, "G6 approve")
	rec := tc.do(t, "GET", "/attendance/records", query("employee_id", tc.memberEmp, "from", y, "to", y))
	if rec.get("data.0.is_regularized") != true || num(rec, "data.0.total_work_minutes") != 540 {
		t.Fatalf("G6 record: %s", rec.Raw)
	}
	expectErr(t, tc.do(t, "POST", "/attendance/regularizations/"+m.str("data.id")+"/reject", body(map[string]string{"comment": "x"})), 409, "REGULARIZATION_NOT_PENDING", "re-review")
}

// TestAttendanceRBAC — G9 with exact grants: attendance:read can read but not manage; plain member only self.
func TestAttendanceRBAC(t *testing.T) {
	tc := newTimeTenant(t)
	for _, p := range [][2]string{{"GET", "/attendance/records"}, {"GET", "/attendance/summary"}, {"GET", "/shifts"}, {"POST", "/shifts"}} {
		expectErr(t, tc.as(t, tc.memberTok, p[0], p[1], body(map[string]any{})), 403, "FORBIDDEN", "member "+p[0]+" "+p[1])
	}
	expect(t, tc.as(t, tc.memberTok, "POST", "/attendance/punch", body(map[string]string{"type": "in"})), 201, "member self punch")
	expect(t, tc.as(t, tc.memberTok, "GET", "/attendance/me"), 200, "member self records")
	reader, _, _ := tc.limitedUser(t, [2]string{"attendance", "read"})
	expect(t, tc.as(t, reader, "GET", "/attendance/records"), 200, "attendance:read lists")
	expectErr(t, tc.as(t, reader, "POST", "/shifts", body(map[string]string{"name": "X", "start_time": "09:00", "end_time": "18:00"})), 403, "FORBIDDEN", "read cannot manage")
	expectErr(t, call(t, "GET", "/attendance/me", host(tc.slug)), 401, "", "no token")
}

// TestAttendanceIsolation — G1: foreign ids 404, foreign JWT 403.
func TestAttendanceIsolation(t *testing.T) {
	a, b := newTimeTenant(t), newTimeTenant(t)
	s := a.do(t, "POST", "/shifts", body(map[string]string{"name": "A " + uniq(), "start_time": "09:00", "end_time": "18:00"}))
	p := a.do(t, "POST", "/attendance/punch", body(map[string]string{"type": "in"}))
	expect(t, p, 201, "A punch")
	for _, path := range []string{"/shifts/" + s.str("data.id"), "/attendance/records/" + p.str("data.record.id")} {
		expectErr(t, b.do(t, "GET", path), 404, "NOT_FOUND", "G1 B reads "+path)
	}
	expectErr(t, call(t, "GET", "/attendance/records", host(b.slug), token(a.token)), 403, "", "G1 A token on B host")
}

// TestAttendanceConcurrentPunches — AS-T6: 10 simultaneous INs produce exactly one session; client time ignored.
func TestAttendanceConcurrentPunches(t *testing.T) {
	tc := newTimeTenant(t)
	codes := make([]int, 10)
	var wg sync.WaitGroup
	for i := range codes {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			codes[i] = tc.as(t, tc.memberTok, "POST", "/attendance/punch", body(map[string]string{"type": "in", "punch_time": "2020-01-01T09:00:00Z"})).Status
		}(i)
	}
	wg.Wait()
	created := 0
	for _, c := range codes {
		if c == 201 {
			created++
		}
	}
	if created != 1 || sqlStr(t, "SELECT count(*) FROM attendance_punches WHERE employee_profile_id = ?", tc.memberEmp) != "1" {
		t.Fatalf("AS-T6: want exactly one punch, got codes %v", codes)
	}
	if y := sqlStr(t, "SELECT extract(year FROM punch_time)::int FROM attendance_punches WHERE employee_profile_id = ?", tc.memberEmp); y != time.Now().UTC().Format("2006") {
		t.Fatalf("AS-T1: server time only, got year %s", y)
	}
}

// TestAttendancePagination — G13: hostile paging is clamped, never a 500.
func TestAttendancePagination(t *testing.T) {
	tc := newTimeTenant(t)
	for _, q := range [][]string{{"per_page", "0"}, {"page", "-1", "per_page", "abc"}, {"per_page", "100000"}} {
		for _, path := range []string{"/attendance/records", "/shifts", "/attendance/regularizations"} {
			r := tc.do(t, "GET", path, query(q...))
			if expect(t, r, 200, "G13 "+path); num(r, "meta.per_page") < 1 || num(r, "meta.per_page") > 100 {
				t.Fatalf("G13 %s %v: %s", path, q, r.Raw)
			}
		}
	}
}

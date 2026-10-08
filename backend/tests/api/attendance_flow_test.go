//go:build integration

package api

import (
	"net/http"
	"strconv"
	"strings"
	"testing"
)

// TestAttendanceLive_PunchFlow — live G2 (alternation + debounce), G11 audit rows, G12 outbox rows, records/summary reads.
func TestAttendanceLive_PunchFlow(t *testing.T) {
	a := newAttTenant(t)

	status, env := a.do(t, a.adminTok, http.MethodPost, "/api/v1/shifts", map[string]interface{}{
		"name": "Day " + randomSuffix(), "start_time": "00:00", "end_time": "23:59", "grace_period_mins": 0, "break_duration_mins": 0,
	})
	requireStatus(t, "create shift", status, 201, env)
	shiftID := strField(t, env.Data, "id")
	status, env = a.do(t, a.adminTok, http.MethodPost, "/api/v1/shifts/"+shiftID+"/assignments", map[string]string{
		"employee_id": a.adminEmp, "effective_from": day(-30),
	})
	requireStatus(t, "assign shift", status, 201, env)

	punch := map[string]interface{}{"type": "in", "latitude": 12.9716, "longitude": 77.5946, "device_id": "e2e-kiosk"}
	status, env = a.do(t, a.adminTok, http.MethodPost, "/api/v1/attendance/punch", punch)
	requireStatus(t, "punch in", status, 201, env)
	record := objField(t, env.Data)["record"].(map[string]interface{})
	recordID := record["id"].(string)
	if record["shift_id"] != shiftID || record["status"] != "present" {
		t.Fatalf("record after IN: %+v", record)
	}
	if strings.Contains(string(env.Data), "ip_address") {
		t.Fatalf("IP must never be returned: %s", env.Data)
	}

	// D3-13: AT-004 debounce is evaluated before AT-003 alternation.
	status, env = a.do(t, a.adminTok, http.MethodPost, "/api/v1/attendance/punch", map[string]string{"type": "in"})
	requireCode(t, "AT-004 debounce wins inside 60s", status, 409, env, "DUPLICATE_PUNCH")
	backdatePunches(t, a.adminEmp)
	status, env = a.do(t, a.adminTok, http.MethodPost, "/api/v1/attendance/punch", map[string]string{"type": "in"})
	requireCode(t, "G2 second IN", status, 409, env, "ALREADY_PUNCHED_IN")

	status, env = a.do(t, a.adminTok, http.MethodGet, "/api/v1/attendance/me/today", nil)
	requireStatus(t, "today", status, 200, env)
	if objField(t, env.Data)["open_session"] != true {
		t.Fatalf("expected open session: %s", env.Data)
	}

	status, env = a.do(t, a.adminTok, http.MethodPost, "/api/v1/attendance/punch", map[string]string{"type": "out"})
	requireStatus(t, "punch out", status, 201, env)
	backdatePunches(t, a.adminEmp)
	status, env = a.do(t, a.adminTok, http.MethodPost, "/api/v1/attendance/punch", map[string]string{"type": "out"})
	requireCode(t, "G2 OUT without IN", status, 409, env, "NOT_PUNCHED_IN")

	status, env = a.do(t, a.adminTok, http.MethodGet, "/api/v1/attendance/records/"+recordID, nil)
	requireStatus(t, "record detail", status, 200, env)
	if punches := objField(t, env.Data)["punches"].([]interface{}); len(punches) != 2 {
		t.Fatalf("expected 2 punches, got %d", len(punches))
	}
	status, env = a.do(t, a.adminTok, http.MethodGet, "/api/v1/attendance/records?employee_id="+a.adminEmp, nil)
	requireStatus(t, "records list", status, 200, env)
	if rows := listField(t, env.Data); len(rows) != 1 || rows[0]["first_punch_in"] == nil || rows[0]["last_punch_out"] == nil {
		t.Fatalf("records: %s", env.Data)
	}
	status, env = a.do(t, a.adminTok, http.MethodGet, "/api/v1/attendance/summary", nil)
	requireStatus(t, "summary", status, 200, env)
	if total := objField(t, env.Data)["total_employees"].(float64); total < 2 {
		t.Fatalf("summary total_employees = %v (want ≥2 working employees)", total)
	}

	// G11: audit rows (no coordinates/IP in metadata).
	audits := psql(t, "SELECT count(*) FROM audit_logs WHERE tenant_id = '"+a.tenantID+"' AND action IN ('attendance.punch_in','attendance.punch_out','shift.created','shift.assigned')")
	if n, _ := strconv.Atoi(audits); n != 4 {
		t.Fatalf("G11: expected 4 attendance audit rows, got %s", audits)
	}
	if leaks := psql(t, "SELECT count(*) FROM audit_logs WHERE tenant_id = '"+a.tenantID+"' AND action LIKE 'attendance.%' AND (metadata::text LIKE '%12.97%' OR metadata::text LIKE '%kiosk%')"); leaks != "0" {
		t.Fatalf("G11: audit metadata leaks location/device (%s rows)", leaks)
	}
	// G12: outbox rows written with the punches; payloads carry no location/device/IP.
	outbox := psql(t, "SELECT count(*) FROM attendance_events_outbox WHERE tenant_id = '"+a.tenantID+"' AND routing_key LIKE 'attendance.punch.%'")
	if outbox != "2" {
		t.Fatalf("G12: expected 2 punch outbox rows, got %s", outbox)
	}
	if leaks := psql(t, "SELECT count(*) FROM attendance_events_outbox WHERE tenant_id = '"+a.tenantID+"' AND (payload::text LIKE '%latitude%' OR payload::text LIKE '%kiosk%' OR payload::text LIKE '%ip_address%')"); leaks != "0" {
		t.Fatalf("G12: event payload leaks private data (%s rows)", leaks)
	}
	// Location + IP persisted for audit on the punch row only.
	if got := psql(t, "SELECT count(*) FROM attendance_punches WHERE attendance_record_id = '"+recordID+"' AND ip_address IS NOT NULL"); got != "2" {
		t.Fatalf("punch rows must keep IP for audit, got %s", got)
	}
}

// TestAttendanceLive_G13_Pagination — hostile paging never errors.
func TestAttendanceLive_G13_Pagination(t *testing.T) {
	a := newAttTenant(t)
	for _, q := range []string{"?per_page=0", "?page=-1&per_page=abc", "?per_page=100000"} {
		status, env := a.do(t, a.adminTok, http.MethodGet, "/api/v1/attendance/records"+q, nil)
		requireStatus(t, "G13 "+q, status, 200, env)
		meta := objField(t, env.Meta)
		if pp := meta["per_page"].(float64); pp < 1 || pp > 100 {
			t.Fatalf("G13 %s: per_page %v", q, pp)
		}
	}
}

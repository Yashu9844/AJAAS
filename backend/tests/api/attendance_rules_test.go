//go:build integration

package api

import (
	"net/http"
	"testing"
)

// TestAttendanceLive_Regularization — live G6 (lifecycle), G7 (self-approval), G8 (one pending), AT-018 cancel.
func TestAttendanceLive_Regularization(t *testing.T) {
	a := newAttTenant(t)
	y := day(-1)
	body := map[string]string{"attendance_date": y, "requested_punch_in": at(y, "09:00"), "requested_punch_out": at(y, "18:00"), "reason": "forgot to punch"}

	status, env := a.do(t, a.memberTok, http.MethodPost, "/api/v1/attendance/regularizations", body)
	requireStatus(t, "member request", status, 201, env)
	memberReq := strField(t, env.Data, "id")
	status, env = a.do(t, a.memberTok, http.MethodPost, "/api/v1/attendance/regularizations", body)
	requireCode(t, "G8 duplicate pending", status, 409, env, "REGULARIZATION_PENDING")

	// G7: approver (tenant_admin) cannot approve their own request.
	status, env = a.do(t, a.adminTok, http.MethodPost, "/api/v1/attendance/regularizations", body)
	requireStatus(t, "admin own request", status, 201, env)
	adminReq := strField(t, env.Data, "id")
	status, env = a.do(t, a.adminTok, http.MethodPost, "/api/v1/attendance/regularizations/"+adminReq+"/approve", nil)
	requireCode(t, "G7 self approval", status, 403, env, "SELF_APPROVAL_FORBIDDEN")
	if st := psql(t, "SELECT status FROM attendance_regularizations WHERE id = '"+adminReq+"'"); st != "pending" {
		t.Fatalf("G7: request must stay pending, got %s", st)
	}
	status, env = a.do(t, a.adminTok, http.MethodPost, "/api/v1/attendance/regularizations/"+adminReq+"/cancel", nil)
	requireStatus(t, "AT-018 cancel own", status, 200, env)

	// G6: admin approves member's request → record rebuilt from requested window.
	status, env = a.do(t, a.adminTok, http.MethodPost, "/api/v1/attendance/regularizations/"+memberReq+"/approve", map[string]string{"comment": "verified"})
	requireStatus(t, "G6 approve", status, 200, env)
	status, env = a.do(t, a.adminTok, http.MethodGet, "/api/v1/attendance/records?employee_id="+a.memberEmp+"&from="+y+"&to="+y, nil)
	requireStatus(t, "G6 record", status, 200, env)
	rows := listField(t, env.Data)
	if len(rows) != 1 || rows[0]["is_regularized"] != true || rows[0]["total_work_minutes"].(float64) != 540 || rows[0]["status"] != "present" {
		t.Fatalf("G6 record: %s", env.Data)
	}
	if n := psql(t, "SELECT count(*) FROM attendance_punches WHERE attendance_record_id = '"+rows[0]["id"].(string)+"' AND source = 'regularization' AND NOT is_superseded"); n != "2" {
		t.Fatalf("G6: expected 2 regularization punches, got %s", n)
	}
	if n := psql(t, "SELECT count(*) FROM attendance_events_outbox WHERE tenant_id = '"+a.tenantID+"' AND routing_key = 'attendance.regularization.approved'"); n != "1" {
		t.Fatalf("G6: approved event missing (%s)", n)
	}
	status, env = a.do(t, a.adminTok, http.MethodPost, "/api/v1/attendance/regularizations/"+memberReq+"/reject", map[string]string{"comment": "late"})
	requireCode(t, "re-review", status, 409, env, "REGULARIZATION_NOT_PENDING")

	status, env = a.do(t, a.memberTok, http.MethodGet, "/api/v1/attendance/regularizations/me?status=approved", nil)
	requireStatus(t, "member list mine", status, 200, env)
	if rows := listField(t, env.Data); len(rows) != 1 {
		t.Fatalf("member should see 1 approved request, got %d", len(rows))
	}
}

// TestAttendanceLive_G9_RBAC — member lacks attendance:* permissions but can use self endpoints.
func TestAttendanceLive_G9_RBAC(t *testing.T) {
	a := newAttTenant(t)
	for _, p := range []struct{ m, path string }{
		{http.MethodGet, "/api/v1/attendance/records"},
		{http.MethodGet, "/api/v1/attendance/summary"},
		{http.MethodGet, "/api/v1/attendance/regularizations"},
		{http.MethodGet, "/api/v1/shifts"},
		{http.MethodPost, "/api/v1/shifts"},
		{http.MethodGet, "/api/v1/shift-assignments?employee_id=" + a.memberEmp},
	} {
		status, env := a.do(t, a.memberTok, p.m, p.path, map[string]string{})
		requireStatus(t, "G9 member "+p.m+" "+p.path, status, 403, env)
	}
	status, env := a.do(t, a.memberTok, http.MethodPost, "/api/v1/attendance/punch", map[string]string{"type": "in"})
	requireStatus(t, "G9 member self punch", status, 201, env)
	status, env = a.do(t, a.memberTok, http.MethodGet, "/api/v1/attendance/me", nil)
	requireStatus(t, "G9 member self records", status, 200, env)
	if rows := listField(t, env.Data); len(rows) != 1 {
		t.Fatalf("member sees only own records: %s", env.Data)
	}
	status, env = a.do(t, "", http.MethodGet, "/api/v1/attendance/me", nil)
	requireStatus(t, "no token", status, 401, env)
}

// TestAttendanceLive_G1_Isolation — cross-tenant ids are 404; foreign JWT on a subdomain is 403.
func TestAttendanceLive_G1_Isolation(t *testing.T) {
	a, b := newAttTenant(t), newAttTenant(t)
	status, env := a.do(t, a.adminTok, http.MethodPost, "/api/v1/shifts", map[string]string{"name": "A " + randomSuffix(), "start_time": "09:00", "end_time": "18:00"})
	requireStatus(t, "A shift", status, 201, env)
	shiftA := strField(t, env.Data, "id")
	status, env = a.do(t, a.adminTok, http.MethodPost, "/api/v1/attendance/punch", map[string]string{"type": "in"})
	requireStatus(t, "A punch", status, 201, env)
	recordA := objField(t, env.Data)["record"].(map[string]interface{})["id"].(string)
	y := day(-1)
	status, env = a.do(t, a.memberTok, http.MethodPost, "/api/v1/attendance/regularizations", map[string]string{
		"attendance_date": y, "requested_punch_in": at(y, "09:00"), "requested_punch_out": at(y, "17:00"), "reason": "x"})
	requireStatus(t, "A regularization", status, 201, env)
	regA := strField(t, env.Data, "id")

	for _, p := range []struct{ m, path string }{
		{http.MethodGet, "/api/v1/attendance/records/" + recordA},
		{http.MethodGet, "/api/v1/shifts/" + shiftA},
		{http.MethodPatch, "/api/v1/shifts/" + shiftA},
		{http.MethodPost, "/api/v1/shifts/" + shiftA + "/assignments"},
		{http.MethodPost, "/api/v1/attendance/regularizations/" + regA + "/approve"},
	} {
		var body interface{} = map[string]string{}
		if p.path == "/api/v1/shifts/"+shiftA+"/assignments" {
			body = map[string]string{"employee_id": b.adminEmp, "effective_from": day(0)}
		}
		status, env := b.do(t, b.adminTok, p.m, p.path, body)
		requireStatus(t, "G1 tenant B "+p.m+" "+p.path, status, 404, env)
	}
	status, env = doReq(t, http.MethodGet, "/api/v1/attendance/records", b.host, a.adminTok, nil)
	requireStatus(t, "G1 A token on B subdomain", status, 403, env)
	status, env = b.do(t, b.adminTok, http.MethodGet, "/api/v1/attendance/records?employee_id="+a.adminEmp, nil)
	requireStatus(t, "G1 B lists A employee", status, 200, env)
	if rows := listField(t, env.Data); len(rows) != 0 {
		t.Fatalf("G1: tenant B must not see tenant A rows: %s", env.Data)
	}
}

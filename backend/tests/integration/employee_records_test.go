package integration

import (
	"strings"
	"testing"
	"time"
)

// TestEmployeeRecordsAndChangeRequests — FR-ER001..FR-ER005 and FR-CR001..FR-CR005 end to end.
func TestEmployeeRecordsAndChangeRequests(t *testing.T) {
	tc := newTimeTenant(t)
	me := func(method, path string, b ...map[string]any) resp {
		if len(b) > 0 {
			return tc.as(t, tc.memberTok, method, "/hr/employees/me"+path, body(b[0]))
		}
		return tc.as(t, tc.memberTok, method, "/hr/employees/me"+path)
	}

	// FR-ER001 emergency contacts: first is primary, a new primary replaces it, deleting the primary promotes another.
	c1 := me("POST", "/emergency-contacts", map[string]any{"name": "Asha", "relationship": "Mother", "phone": "+919800000001"})
	if expect(t, c1, 201, "contact 1"); c1.get("data.is_primary") != true {
		t.Fatalf("first contact is primary: %s", c1.Raw)
	}
	c2 := me("POST", "/emergency-contacts", map[string]any{"name": "Ravi", "relationship": "Brother", "phone": "+919800000002", "is_primary": true})
	expect(t, c2, 201, "contact 2")
	if l := me("GET", "/emergency-contacts"); l.str("data.0.id") != c2.str("data.id") || l.get("data.1.is_primary") != false {
		t.Fatalf("single primary, listed first: %s", l.Raw)
	}
	expectErr(t, me("POST", "/emergency-contacts", map[string]any{"name": "X", "relationship": "Y", "phone": "12"}), 400, "VALIDATION_ERROR", "short phone")
	if r := me("DELETE", "/emergency-contacts/"+c2.str("data.id")); r.Status != 204 {
		t.Fatalf("delete: %d", r.Status)
	}
	if l := me("GET", "/emergency-contacts"); l.get("data.0.is_primary") != true || len(l.list("data")) != 1 {
		t.Fatalf("primary promoted: %s", l.Raw)
	}

	// FR-ER002/003 education and prior employment.
	ed := me("POST", "/education", map[string]any{"institution": "IIT Madras", "qualification": "B.Tech", "start_year": 2012, "end_year": 2016})
	expect(t, ed, 201, "education")
	expect(t, me("PUT", "/education/"+ed.str("data.id"), map[string]any{"institution": "IIT Madras", "qualification": "B.Tech", "field_of_study": "CS",
		"start_year": 2012, "end_year": 2016}), 200, "update education")
	expectErr(t, me("POST", "/education", map[string]any{"institution": "X", "qualification": "Y", "start_year": 2016, "end_year": 2012}), 400, "VALIDATION_ERROR", "years")
	expect(t, me("POST", "/prior-employment", map[string]any{"employer": "Acme", "title": "Engineer", "start_date": "2016-07-01", "end_date": "2020-06-30"}), 201, "prior 1")
	expectErr(t, me("POST", "/prior-employment", map[string]any{"employer": "Beta", "title": "SE", "start_date": "2020-01-01", "end_date": "2021-01-01"}),
		409, "OVERLAP", "FR-ER003 overlap")
	tomorrow := time.Now().UTC().AddDate(0, 0, 2).Format("2006-01-02")
	expectErr(t, me("POST", "/prior-employment", map[string]any{"employer": "Beta", "title": "SE", "start_date": "2021-01-01", "end_date": tomorrow}),
		400, "VALIDATION_ERROR", "future end")

	// FR-ER004 HR view and writes; a colleague without permission sees nothing.
	if l := tc.do(t, "GET", "/hr/employees/"+tc.memberEmp+"/education"); len(l.list("data")) != 1 {
		t.Fatalf("HR reads education: %s", l.Raw)
	}
	expect(t, tc.do(t, "POST", "/hr/employees/"+tc.memberEmp+"/prior-employment", body(map[string]any{"employer": "Gamma", "title": "Lead",
		"start_date": "2021-01-01", "end_date": "2022-12-31"})), 201, "HR adds prior employment")
	other := newTenant(t)
	expectErr(t, other.do(t, "GET", "/hr/employees/"+tc.memberEmp+"/education"), 404, "EMPLOYEE_NOT_FOUND", "foreign employee")
	expectErr(t, other.do(t, "PUT", "/hr/employees/"+tc.memberEmp+"/education/"+ed.str("data.id"), body(map[string]any{})), 404, "", "foreign record")

	// FR-CR change requests.
	bank := me("POST", "/change-requests", map[string]any{"type": "bank", "payload": map[string]any{"bank_name": "HDFC",
		"bank_account_number": "50100123456789", "bank_routing_swift": "HDFC0001234"}, "reason": "salary account moved"})
	expect(t, bank, 201, "bank change")
	if acct := bank.str("data.payload.bank_account_number"); acct != "****6789" {
		t.Fatalf("FR-CR005 masked: %s", acct)
	}
	expectErr(t, me("POST", "/change-requests", map[string]any{"type": "bank", "payload": map[string]any{"bank_name": "SBI",
		"bank_account_number": "12345678", "bank_routing_swift": "SBIN0000001"}}), 409, "CONFLICT", "FR-CR002 one open per type")
	expectErr(t, me("POST", "/change-requests", map[string]any{"type": "bank", "payload": map[string]any{"bank_name": "SBI",
		"bank_account_number": "12-34", "bank_routing_swift": "SBIN"}}), 400, "VALIDATION_ERROR", "account format")
	name := me("POST", "/change-requests", map[string]any{"type": "name", "payload": map[string]any{"first_name": "Priya", "last_name": "Iyer"}})
	expect(t, name, 201, "name change")
	expectErr(t, me("GET", "/../../employee-change-requests"), 404, "", "members cannot reach the queue by path tricks")
	expectErr(t, tc.as(t, tc.memberTok, "GET", "/hr/employee-change-requests"), 403, "FORBIDDEN", "member cannot list the queue")
	expect(t, me("POST", "/change-requests/"+name.str("data.id")+"/cancel"), 200, "cancel own request")

	approver, _, _ := tc.limitedUser(t, [2]string{"employee", "approve_changes"})
	q := tc.as(t, approver, "GET", "/hr/employee-change-requests?status=submitted")
	if len(q.list("data")) != 1 {
		t.Fatalf("queue: %s", q.Raw)
	}
	ok := tc.as(t, approver, "POST", "/hr/employee-change-requests/"+bank.str("data.id")+"/approve", body(map[string]any{"comment": "verified with cheque"}))
	if expect(t, ok, 200, "approve bank"); ok.str("data.status") != "approved" {
		t.Fatalf("approved: %s", ok.Raw)
	}
	if acct := sqlStr(t, "SELECT bank_account_number FROM employee_statutory WHERE employee_profile_id = ?", tc.memberEmp); acct != "50100123456789" {
		t.Fatalf("FR-CR004 applied: %s", acct)
	}
	if n := sqlStr(t, "SELECT count(*) FROM employee_timelines WHERE employee_profile_id = ? AND event_type = 'personal_data_changed:bank'", tc.memberEmp); n != "1" {
		t.Fatalf("timeline: %s", n)
	}
	expectErr(t, tc.as(t, approver, "POST", "/hr/employee-change-requests/"+bank.str("data.id")+"/reject"), 409, "REQUEST_STATE", "decided twice")

	// The tenant admin has an employee profile and may approve others but never their own request.
	adminTok := tc.token
	own := tc.do(t, "POST", "/hr/employees/me/change-requests", body(map[string]any{"type": "address", "payload": map[string]any{"current_address": "12 MG Road"}}))
	expect(t, own, 201, "admin's own request")
	expectErr(t, call(t, "POST", "/hr/employee-change-requests/"+own.str("data.id")+"/approve", host(tc.slug), token(adminTok)), 403, "EMPLOYEE_SELF_APPROVAL", "self-approval")
	expect(t, tc.as(t, approver, "POST", "/hr/employee-change-requests/"+own.str("data.id")+"/reject", body(map[string]any{"comment": "use proof"})), 200, "reject")

	// FR-ER005/FR-CR005 privacy.
	for _, q := range []string{
		"SELECT count(*) FROM audit_logs WHERE tenant_id = ? AND (metadata::text LIKE '%50100123456789%' OR metadata::text LIKE '%Priya%' OR metadata::text LIKE '%MG Road%')",
		"SELECT count(*) FROM employee_events_outbox WHERE tenant_id = ? AND (payload::text LIKE '%50100123456789%' OR payload::text LIKE '%Priya%')",
	} {
		if n := sqlStr(t, q, tc.id); n != "0" {
			t.Fatalf("leak: %s rows for %s", n, q)
		}
	}
	if !strings.Contains(sqlStr(t, "SELECT string_agg(routing_key, ',') FROM employee_events_outbox WHERE tenant_id = ?", tc.id), "employee.change_request.approved") {
		t.Fatal("approved event")
	}
}

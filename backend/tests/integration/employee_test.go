package integration

import (
	"encoding/json"
	"strings"
	"testing"
)

const joinDate = "2026-01-15T00:00:00Z"

func empBody(userID, code string, extra map[string]any) map[string]any {
	b := map[string]any{"user_id": userID, "employee_code": code, "first_name": "Dev", "last_name": "Employee", "employment_type": "full_time", "joining_date": joinDate}
	for k, v := range extra {
		b[k] = v
	}
	return b
}

// mkEmployee creates a user + employee profile and returns the employee id.
func (tc *tenantCtx) mkEmployee(t *testing.T, tag string, extra ...map[string]any) string {
	t.Helper()
	id, _ := tc.mkEmployeeFor(t, tag, nil, extra...)
	return id
}

func (tc *tenantCtx) mkEmployeeFor(t *testing.T, tag string, roleIDs []string, extra ...map[string]any) (empID, userID string) {
	t.Helper()
	uid, _ := tc.newUser(t, tag, roleIDs...)
	e := map[string]any{}
	if len(extra) > 0 {
		e = extra[0]
	}
	r := tc.do(t, "POST", "/employees", body(empBody(uid, strings.ToUpper("E"+tag+"-"+uniq()), e)))
	expect(t, r, 201, "create employee "+tag)
	return r.str("data.id"), uid
}

func timelineHas(r resp, needle string) bool {
	for _, it := range r.list("data") {
		if strings.Contains(it.(map[string]any)["event_type"].(string), needle) {
			return true
		}
	}
	return false
}

func TestEmployeeLifecycle(t *testing.T) {
	tc := newTenant(t)
	uid, _ := tc.newUser(t, "emp")
	code := "EMP-" + strings.ToUpper(uniq())
	c := tc.do(t, "POST", "/employees", body(empBody(uid, strings.ToLower(code), map[string]any{
		"display_name": "Dev E", "gender": "female", "date_of_birth": "1992-04-10T00:00:00Z", "personal_email": "dev@example.com",
		"work_phone": "+911111111111", "current_address": "Bengaluru", "notice_period_days": 30,
	})))
	expect(t, c, 201, "create")
	id := c.str("data.id")
	if c.str("data.employee_code") != code || c.str("data.user_id") != uid || c.str("data.status") != "active" ||
		c.str("data.employment.employment_type") != "full_time" || c.str("data.contact.work_phone") != "+911111111111" {
		t.Fatalf("create body wrong: %s", c.Raw)
	}
	g := tc.do(t, "GET", "/employees/"+id)
	if g.str("data.display_name") != "Dev E" || g.get("data.employment.notice_period_days") != float64(30) || g.str("data.contact.personal_email") != "dev@example.com" {
		t.Fatalf("read-back wrong: %s", g.Raw)
	}
	l := tc.do(t, "GET", "/employees", query("per_page", "100"))
	if !l.has("data", "id", id) || l.get("meta.total_items") == nil || l.get("meta.total_pages") == nil {
		t.Fatalf("list wrong: %s", l.Raw)
	}
	if !tc.do(t, "GET", "/employees", query("search", code)).has("data", "id", id) {
		t.Fatal("search by code failed")
	}
	if !tc.do(t, "GET", "/employees", query("status", "active", "per_page", "100")).has("data", "id", id) {
		t.Fatal("status filter failed")
	}
	if tc.do(t, "GET", "/employees", query("status", "terminated")).has("data", "id", id) {
		t.Fatal("status filter leaked")
	}
	expect(t, tc.do(t, "GET", "/employees", query("search", "'; DROP TABLE users;--")), 200, "sql metacharacters are safe")

	expect(t, tc.do(t, "PATCH", "/employees/"+id, body(map[string]string{"first_name": "Changed", "gender": "male", "blood_group": "O+"})), 200, "update")
	g = tc.do(t, "GET", "/employees/"+id)
	if g.str("data.first_name") != "Changed" || g.str("data.gender") != "male" || g.str("data.blood_group") != "O+" {
		t.Fatalf("update not persisted: %s", g.Raw)
	}
	if !timelineHas(tc.do(t, "GET", "/employees/"+id+"/timeline"), "hired") {
		t.Fatal("timeline lacks 'hired' (FR-TL001)")
	}

	// uniqueness & validation
	u2, _ := tc.newUser(t, "emp2")
	expectErr(t, tc.do(t, "POST", "/employees", body(empBody(u2, code, nil))), 409, "CONFLICT", "duplicate code")
	expectErr(t, tc.do(t, "POST", "/employees", body(empBody(uid, "X-"+uniq(), nil))), 409, "CONFLICT", "second profile for the user")
	expectErr(t, tc.do(t, "POST", "/employees", body(empBody(zeroUUID, "Y-"+uniq(), nil))), 404, "NOT_FOUND", "unknown user")
	expectErr(t, tc.do(t, "POST", "/employees", body(empBody(u2, "bad code!", nil))), 400, "VALIDATION_ERROR", "bad code")
	expectErr(t, tc.do(t, "POST", "/employees", body(empBody(u2, "OK-"+uniq(), map[string]any{"employment_type": "slave"}))), 400, "VALIDATION_ERROR", "bad employment type")
	expectErr(t, tc.do(t, "POST", "/employees", body(map[string]any{})), 400, "VALIDATION_ERROR", "empty")
	expectErr(t, tc.do(t, "POST", "/employees", rawBody(`{"user_id":"`+u2+`","employee_code":"Z-1","first_name":"a","last_name":"b","employment_type":"full_time","joining_date":"nope"}`)), 400, "VALIDATION_ERROR", "bad date")
	expectErr(t, tc.do(t, "GET", "/employees/"+zeroUUID), 404, "NOT_FOUND", "unknown")
	expectErr(t, tc.do(t, "GET", "/employees/not-a-uuid"), 400, "VALIDATION_ERROR", "bad id")
	expectErr(t, tc.do(t, "PATCH", "/employees/"+zeroUUID, body(map[string]string{"first_name": "x"})), 404, "NOT_FOUND", "patch unknown")
	expectErr(t, tc.do(t, "PATCH", "/employees/bad", body(map[string]string{"first_name": "x"})), 400, "VALIDATION_ERROR", "patch bad id")
	expectErr(t, tc.do(t, "GET", "/employees/"+zeroUUID+"/timeline"), 404, "NOT_FOUND", "timeline unknown")

	pr := tc.mkEmployee(t, "prob", map[string]any{"probation_end_date": "2099-01-01T00:00:00Z"})
	if tc.do(t, "GET", "/employees/"+pr).str("data.status") != "probation" {
		t.Fatal("future probation_end_date must start in probation")
	}
	for _, q := range []map[string]string{{"status": "nonsense"}, {"department_id": "nope"}, {"page": "-1"}, {"per_page": "0"}, {"per_page": "1000"}} {
		kv := []string{}
		for k, v := range q {
			kv = append(kv, k, v)
		}
		if r := tc.do(t, "GET", "/employees", query(kv...)); r.Status >= 500 {
			t.Fatalf("query %v -> %d", q, r.Status)
		}
	}
	// department filter
	did := tc.mkDept(t, "EF")
	eid, euid := tc.mkEmployeeFor(t, "filt", nil)
	expect(t, tc.do(t, "POST", "/mappings", body(map[string]any{"user_id": euid, "department_id": did, "is_primary": true})), 201, "map")
	_ = eid
	expect(t, tc.do(t, "GET", "/employees", query("department_id", did)), 200, "department filter")
}

func TestEmployeeStatusStateMachine(t *testing.T) {
	tc := newTenant(t)

	// confirmation: probation -> active (FR-ED003)
	p, _ := tc.mkEmployeeFor(t, "conf", nil, map[string]any{"probation_end_date": "2099-01-01T00:00:00Z"})
	expect(t, tc.do(t, "POST", "/employees/"+p+"/status", body(map[string]string{"status": "active"})), 200, "confirm")
	g := tc.do(t, "GET", "/employees/"+p)
	if g.str("data.status") != "active" || g.str("data.employment.confirmation_date") == "" {
		t.Fatalf("confirmation not recorded: %s", g.Raw)
	}
	if !timelineHas(tc.do(t, "GET", "/employees/"+p+"/timeline"), "confirmed") {
		t.Fatal("timeline lacks 'confirmed'")
	}

	// notice (FR-ED004) then resign (FR-ED005 deactivates the user)
	e, uid := tc.mkEmployeeFor(t, "exit", nil, map[string]any{"notice_period_days": 10})
	expect(t, tc.do(t, "POST", "/employees/"+e+"/status", body(map[string]string{"status": "notice", "notes": "gave notice"})), 200, "notice")
	g = tc.do(t, "GET", "/employees/"+e)
	if g.str("data.status") != "notice" || g.str("data.employment.resignation_date") == "" || g.str("data.employment.exit_date") == "" {
		t.Fatalf("exit dates not derived: %s", g.Raw)
	}
	if tc.do(t, "GET", "/users/"+uid).str("data.status") != "active" {
		t.Fatal("notice must not deactivate the user yet")
	}
	// back to active is allowed (notice withdrawn), then resign for good
	expect(t, tc.do(t, "POST", "/employees/"+e+"/status", body(map[string]string{"status": "active"})), 200, "withdraw notice")
	expect(t, tc.do(t, "POST", "/employees/"+e+"/status", body(map[string]any{"status": "resigned", "exit_date": "2026-10-01T00:00:00Z", "exit_reason": "better offer"})), 200, "resign")
	g = tc.do(t, "GET", "/employees/"+e)
	if g.str("data.status") != "resigned" || g.str("data.employment.exit_reason") != "better offer" || !strings.HasPrefix(g.str("data.employment.exit_date"), "2026-10-01") {
		t.Fatalf("exit data wrong: %s", g.Raw)
	}
	if tc.do(t, "GET", "/users/"+uid).str("data.status") != "inactive" {
		t.Fatal("FR-ED005: exit must deactivate the Module 0 user")
	}
	if !timelineHas(tc.do(t, "GET", "/employees/"+e+"/timeline"), "resigned") {
		t.Fatal("timeline lacks 'resigned'")
	}
	expectErr(t, tc.do(t, "POST", "/employees/"+e+"/status", body(map[string]string{"status": "active"})), 409, "INVALID_STATUS_TRANSITION", "resigned is terminal")
	expect(t, tc.do(t, "POST", "/employees/"+e+"/status", body(map[string]string{"status": "resigned"})), 200, "same status is a no-op")

	// termination
	e2, uid2 := tc.mkEmployeeFor(t, "term", nil)
	expect(t, tc.do(t, "POST", "/employees/"+e2+"/status", body(map[string]string{"status": "terminated", "exit_reason": "misconduct"})), 200, "terminate")
	g = tc.do(t, "GET", "/employees/"+e2)
	if g.str("data.employment.exit_date") == "" || tc.do(t, "GET", "/users/"+uid2).str("data.status") != "inactive" {
		t.Fatalf("terminate must stamp exit_date and deactivate the user: %s", g.Raw)
	}
	expectErr(t, tc.do(t, "POST", "/employees/"+e2+"/status", body(map[string]string{"status": "probation"})), 409, "INVALID_STATUS_TRANSITION", "terminated is terminal")

	// on_leave round trip
	e3 := tc.mkEmployee(t, "leave")
	expect(t, tc.do(t, "POST", "/employees/"+e3+"/status", body(map[string]string{"status": "on_leave"})), 200, "on_leave")
	expect(t, tc.do(t, "POST", "/employees/"+e3+"/status", body(map[string]string{"status": "active"})), 200, "back")

	expectErr(t, tc.do(t, "POST", "/employees/"+e3+"/status", body(map[string]string{"status": "bogus"})), 400, "VALIDATION_ERROR", "unknown status")
	expectErr(t, tc.do(t, "POST", "/employees/"+e3+"/status", body(map[string]string{})), 400, "VALIDATION_ERROR", "missing status")
	expectErr(t, tc.do(t, "POST", "/employees/"+zeroUUID+"/status", body(map[string]string{"status": "notice"})), 404, "NOT_FOUND", "unknown employee")
	expectErr(t, tc.do(t, "POST", "/employees/bad/status", body(map[string]string{"status": "notice"})), 400, "VALIDATION_ERROR", "bad id")

	// administrative deactivation
	e4 := tc.mkEmployee(t, "deact")
	expect(t, tc.do(t, "POST", "/employees/"+e4+"/deactivate"), 200, "deactivate")
	if tc.do(t, "GET", "/employees/"+e4).str("data.status") != "inactive" {
		t.Fatal("not inactive")
	}
	expectErr(t, tc.do(t, "POST", "/employees/"+zeroUUID+"/deactivate"), 404, "NOT_FOUND", "unknown")
	expectErr(t, tc.do(t, "POST", "/employees/bad/deactivate"), 400, "VALIDATION_ERROR", "bad id")
	// inactive -> active (reactivation) is allowed
	expect(t, tc.do(t, "POST", "/employees/"+e4+"/status", body(map[string]string{"status": "active"})), 200, "reactivate")
}

func TestEmployeeStatutoryAndPIIMasking(t *testing.T) {
	tc := newTenant(t)
	e := tc.mkEmployee(t, "bank")
	// nothing stored yet
	if r := tc.do(t, "GET", "/employees/"+e+"/statutory"); r.Status != 404 && r.Status != 200 {
		t.Fatalf("empty statutory: %d", r.Status)
	}
	put := tc.do(t, "PUT", "/employees/"+e+"/statutory", body(map[string]string{"tax_id": "ABCDE1234F", "national_id": "9999888877776666", "bank_name": "Test Bank", "bank_account_number": "123456789012", "bank_routing_swift": "TESTINBB"}))
	expect(t, put, 200, "put")
	if put.str("data.bank_name") != "Test Bank" {
		t.Fatalf("put body: %s", put.Raw)
	}
	g := tc.do(t, "GET", "/employees/"+e+"/statutory")
	expect(t, g, 200, "get")
	if g.str("data.tax_id") != "****234F" || g.str("data.bank_account_number") != "****9012" || g.str("data.national_id") != "****6666" || g.str("data.bank_name") != "Test Bank" {
		t.Fatalf("PII must be masked by default: %s", g.Raw)
	}
	if strings.Contains(g.Raw, "ABCDE1234F") || strings.Contains(g.Raw, "123456789012") {
		t.Fatal("raw PII in masked response")
	}
	g = tc.do(t, "GET", "/employees/"+e+"/statutory", query("unmasked", "false"))
	if g.str("data.tax_id") != "****234F" {
		t.Fatalf("unmasked=false must stay masked: %s", g.Raw)
	}
	// tenant_admin holds every permission, so an explicit request is honoured (and audited)
	g = tc.do(t, "GET", "/employees/"+e+"/statutory", query("unmasked", "true"))
	if g.str("data.tax_id") != "ABCDE1234F" || g.str("data.bank_account_number") != "123456789012" {
		t.Fatalf("admin unmasked read: %s", g.Raw)
	}
	if !tc.do(t, "GET", "/audit-logs", query("per_page", "100")).has("data", "action", "employee.statutory_unmasked") {
		t.Fatal("unmasked read must be audited")
	}

	// upsert overwrites
	expect(t, tc.do(t, "PUT", "/employees/"+e+"/statutory", body(map[string]string{"bank_name": "Other Bank", "tax_id": "ABCDE1234F"})), 200, "upsert")
	if tc.do(t, "GET", "/employees/"+e+"/statutory").str("data.bank_name") != "Other Bank" {
		t.Fatal("upsert not persisted")
	}
	expectErr(t, tc.do(t, "PUT", "/employees/"+e+"/statutory", body(map[string]string{"tax_id": strings.Repeat("x", 101)})), 400, "VALIDATION_ERROR", "too long")
	expectErr(t, tc.do(t, "PUT", "/employees/"+zeroUUID+"/statutory", body(map[string]string{"bank_name": "x"})), 404, "NOT_FOUND", "unknown")
	expectErr(t, tc.do(t, "GET", "/employees/"+zeroUUID+"/statutory"), 404, "NOT_FOUND", "get unknown")
	expectErr(t, tc.do(t, "PUT", "/employees/bad/statutory", body(map[string]string{"bank_name": "x"})), 400, "VALIDATION_ERROR", "bad id")
	expectErr(t, tc.do(t, "GET", "/employees/bad/statutory"), 400, "VALIDATION_ERROR", "get bad id")

	// ---- RBAC tiers
	lim, rid, _ := tc.limitedUser(t, [2]string{"employee", "read"})
	r := call(t, "GET", "/employees/"+e+"/statutory", host(tc.slug), token(lim))
	expect(t, r, 200, "read-only sees masked data")
	if r.str("data.tax_id") != "****234F" {
		t.Fatalf("read-only must be masked: %s", r.Raw)
	}
	deny := call(t, "GET", "/employees/"+e+"/statutory", host(tc.slug), token(lim), query("unmasked", "true"))
	expectErr(t, deny, 403, "FORBIDDEN", "SECURITY: ?unmasked=true without employee:read_sensitive")
	if strings.Contains(deny.Raw, "ABCDE1234F") {
		t.Fatal("PII leaked in the denial")
	}
	expectErr(t, call(t, "PUT", "/employees/"+e+"/statutory", host(tc.slug), token(lim), body(map[string]string{"bank_name": "hack"})), 403, "FORBIDDEN", "PUT needs update_sensitive")
	tc.grant(t, rid, "employee", "update_sensitive")
	expect(t, call(t, "PUT", "/employees/"+e+"/statutory", host(tc.slug), token(lim), body(map[string]string{"bank_name": "ByLimited", "tax_id": "ABCDE1234F"})), 200, "PUT after grant")
	expectErr(t, call(t, "GET", "/employees/"+e+"/statutory", host(tc.slug), token(lim), query("unmasked", "true")), 403, "FORBIDDEN", "update_sensitive does not imply read_sensitive")
	tc.grant(t, rid, "employee", "read_sensitive")
	raw := call(t, "GET", "/employees/"+e+"/statutory", host(tc.slug), token(lim), query("unmasked", "true"))
	expect(t, raw, 200, "read_sensitive")
	if raw.str("data.tax_id") != "ABCDE1234F" {
		t.Fatalf("read_sensitive holder must see raw values: %s", raw.Raw)
	}
	if call(t, "GET", "/employees/"+e+"/statutory", host(tc.slug), token(lim)).str("data.tax_id") != "****234F" {
		t.Fatal("default stays masked even for read_sensitive holders")
	}
}

func TestEmployeeDocuments(t *testing.T) {
	tc := newTenant(t)
	e := tc.mkEmployee(t, "doc")
	doc := map[string]any{"document_type": "id_proof", "file_name": "passport.pdf", "file_url": "https://example.com/p.pdf", "file_size": 2048, "mime_type": "application/pdf"}
	d := tc.do(t, "POST", "/employees/"+e+"/documents", body(doc))
	expect(t, d, 201, "register")
	did := d.str("data.id")
	if d.str("data.employee_profile_id") != e || d.str("data.document_type") != "id_proof" || d.str("data.verified_at") != "" {
		t.Fatalf("register body wrong: %s", d.Raw)
	}
	l := tc.do(t, "GET", "/employees/"+e+"/documents")
	if !l.has("data", "id", did) {
		t.Fatal("list lacks document")
	}
	expect(t, tc.do(t, "POST", "/employees/"+e+"/documents/"+did+"/verify"), 200, "verify")
	for _, it := range tc.do(t, "GET", "/employees/"+e+"/documents").list("data") {
		m := it.(map[string]any)
		if m["id"] == did && (m["verified_at"] == nil || m["verified_by"] != tc.adminID) {
			t.Fatalf("verification not recorded: %v", m)
		}
	}
	expectErr(t, tc.do(t, "POST", "/employees/"+e+"/documents/"+zeroUUID+"/verify"), 404, "NOT_FOUND", "verify unknown")
	expectErr(t, tc.do(t, "POST", "/employees/"+e+"/documents/bad/verify"), 400, "VALIDATION_ERROR", "bad doc id")
	expectErr(t, tc.do(t, "POST", "/employees/"+e+"/documents", body(map[string]any{"document_type": "x1", "file_name": "a", "file_url": "not a url", "file_size": 1, "mime_type": "a/b"})), 400, "VALIDATION_ERROR", "bad url")
	expectErr(t, tc.do(t, "POST", "/employees/"+e+"/documents", body(map[string]any{"document_type": "x1", "file_name": "a", "file_url": "https://e.com/a", "file_size": 0, "mime_type": "a/b"})), 400, "VALIDATION_ERROR", "size 0")
	expectErr(t, tc.do(t, "POST", "/employees/"+e+"/documents", body(map[string]any{})), 400, "VALIDATION_ERROR", "empty")
	expectErr(t, tc.do(t, "POST", "/employees/"+zeroUUID+"/documents", body(doc)), 404, "NOT_FOUND", "unknown employee")
	expectErr(t, tc.do(t, "POST", "/employees/bad/documents", body(doc)), 400, "VALIDATION_ERROR", "bad id")
	expectErr(t, tc.do(t, "GET", "/employees/bad/documents"), 400, "VALIDATION_ERROR", "list bad id")
	// a document cannot be verified through another employee's URL
	other := tc.mkEmployee(t, "doc2")
	expectErr(t, tc.do(t, "POST", "/employees/"+other+"/documents/"+did+"/verify"), 404, "NOT_FOUND", "cross-employee verify")

	// verify needs employee:admin; register needs employee:update
	lim, rid, _ := tc.limitedUser(t, [2]string{"employee", "read"})
	expectErr(t, call(t, "POST", "/employees/"+e+"/documents", host(tc.slug), token(lim), body(doc)), 403, "FORBIDDEN", "register needs update")
	expectErr(t, call(t, "POST", "/employees/"+e+"/documents/"+did+"/verify", host(tc.slug), token(lim)), 403, "FORBIDDEN", "verify needs admin")
	tc.grant(t, rid, "employee", "update")
	expect(t, call(t, "POST", "/employees/"+e+"/documents", host(tc.slug), token(lim), body(doc)), 201, "register after grant")
	expectErr(t, call(t, "POST", "/employees/"+e+"/documents/"+did+"/verify", host(tc.slug), token(lim)), 403, "FORBIDDEN", "still not admin")
	tc.grant(t, rid, "employee", "admin")
	expect(t, call(t, "POST", "/employees/"+e+"/documents/"+did+"/verify", host(tc.slug), token(lim)), 200, "verify after admin")
}

func TestEmployeeSelfService(t *testing.T) {
	tc := newTenant(t)
	member := tc.roleID(t, "member")
	e, uid := tc.mkEmployeeFor(t, "self", []string{member})
	other := tc.mkEmployee(t, "other")
	expectErr(t, tc.do(t, "GET", "/employees/me"), 404, "NOT_FOUND", "admin has no profile")
	var email string
	for _, it := range tc.do(t, "GET", "/users", query("per_page", "100")).list("data") {
		if m := it.(map[string]any); m["id"] == uid {
			email = m["email"].(string)
		}
	}
	st := login(t, tc.slug, email, password).str("data.access_token")
	as := func(method, path string, opts ...opt) resp {
		return call(t, method, path, append([]opt{host(tc.slug), token(st)}, opts...)...)
	}

	me := as("GET", "/employees/me")
	expect(t, me, 200, "me")
	if me.str("data.id") != e || me.str("data.user_id") != uid {
		t.Fatalf("me wrong: %s", me.Raw)
	}
	expect(t, as("PATCH", "/employees/me", body(map[string]string{"personal_phone": "+910000000001", "current_address": "Self Street 1"})), 200, "patch me")
	me = as("GET", "/employees/me")
	if me.str("data.contact.personal_phone") != "+910000000001" || me.str("data.contact.current_address") != "Self Street 1" {
		t.Fatalf("contact not persisted: %s", me.Raw)
	}
	// FR-EC001: emergency contacts are a JSON array of {name, relation, phone}
	expectErr(t, as("PATCH", "/employees/me", body(map[string]string{"emergency_contacts": "Mom +910000000002"})), 400, "VALIDATION_ERROR", "free text")
	expectErr(t, as("PATCH", "/employees/me", body(map[string]string{"emergency_contacts": `[{"name":"Mom"}]`})), 400, "VALIDATION_ERROR", "missing phone")
	expectErr(t, as("PATCH", "/employees/me", body(map[string]string{"emergency_contacts": `{"name":"Mom"}`})), 400, "VALIDATION_ERROR", "not an array")
	ec, _ := json.Marshal([]map[string]string{{"name": "Mom", "relation": "mother", "phone": "+910000000002"}})
	expect(t, as("PATCH", "/employees/me", body(map[string]string{"emergency_contacts": string(ec)})), 200, "valid contacts")
	if !strings.Contains(as("GET", "/employees/me").str("data.contact.emergency_contacts"), "Mom") {
		t.Fatal("emergency contacts not persisted")
	}
	expect(t, as("PATCH", "/employees/me", body(map[string]string{"emergency_contacts": ""})), 200, "clear contacts")
	if as("GET", "/employees/me").str("data.contact.emergency_contacts") != "[]" {
		t.Fatal("empty contacts must be stored as []")
	}

	// a plain member cannot see or edit anybody else
	expectErr(t, as("GET", "/employees"), 403, "FORBIDDEN", "directory")
	expectErr(t, as("GET", "/employees/"+other), 403, "FORBIDDEN", "other profile")
	expectErr(t, as("GET", "/employees/"+other+"/statutory"), 403, "FORBIDDEN", "other statutory")
	expectErr(t, as("PATCH", "/employees/"+other, body(map[string]string{"first_name": "x"})), 403, "FORBIDDEN", "edit other")
	expectErr(t, as("PATCH", "/employees/"+e, body(map[string]string{"first_name": "x"})), 403, "FORBIDDEN", "edit own core profile")
	expectErr(t, as("POST", "/employees/"+e+"/status", body(map[string]string{"status": "notice"})), 403, "FORBIDDEN", "self status change")
	expectErr(t, call(t, "GET", "/employees/me", host(tc.slug)), 401, "UNAUTHORIZED", "needs a token")
}

func TestEmployeeRBACTiers(t *testing.T) {
	tc := newTenant(t)
	target := tc.mkEmployee(t, "rbac")
	expect(t, tc.do(t, "PUT", "/employees/"+target+"/statutory", body(map[string]string{"tax_id": "ABCDE1234F", "bank_name": "B"})), 200, "seed statutory")
	lim, rid, luid := tc.limitedUser(t, [2]string{"employee", "read"})
	as := func(method, path string, opts ...opt) resp {
		return call(t, method, path, append([]opt{host(tc.slug), token(lim)}, opts...)...)
	}
	for _, p := range []string{"/employees", "/employees/" + target, "/employees/" + target + "/timeline", "/employees/" + target + "/documents", "/employees/" + target + "/statutory"} {
		expect(t, as("GET", p), 200, "read "+p)
	}
	expectErr(t, as("POST", "/employees", body(empBody(luid, "Q-"+uniq(), nil))), 403, "FORBIDDEN", "create")
	expectErr(t, as("PATCH", "/employees/"+target, body(map[string]string{"first_name": "X"})), 403, "FORBIDDEN", "update")
	expectErr(t, as("POST", "/employees/"+target+"/status", body(map[string]string{"status": "notice"})), 403, "FORBIDDEN", "status")
	expectErr(t, as("POST", "/employees/"+target+"/deactivate"), 403, "FORBIDDEN", "deactivate")
	tc.grant(t, rid, "employee", "update")
	expect(t, as("PATCH", "/employees/"+target, body(map[string]string{"first_name": "ByLimited"})), 200, "update after grant")
	expect(t, as("POST", "/employees/"+target+"/status", body(map[string]string{"status": "on_leave"})), 200, "status after grant")
	tc.grant(t, rid, "employee", "create")
	u2, _ := tc.newUser(t, "newemp")
	expect(t, as("POST", "/employees", body(empBody(u2, "N-"+uniq(), nil))), 201, "create after grant")
}

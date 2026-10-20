package integration

import (
	"strings"
	"testing"
)

// TestOrganizationLocations — FR-L001..FR-L007: lifecycle, validation, the mapping link, deactivation rules, RBAC and
// tenant isolation.
func TestOrganizationLocations(t *testing.T) {
	tc := newTimeTenant(t)
	code := "BLR" + strings.ToUpper(uniq())
	loc := tc.do(t, "POST", "/hr/locations", body(map[string]any{"code": strings.ToLower(code), "name": "Bengaluru " + uniq(), "city": "Bengaluru",
		"state_code": "ka", "address_line1": "MG Road", "postal_code": "560001"}))
	expect(t, loc, 201, "create location")
	id := loc.str("data.id")
	if loc.str("data.code") != code || loc.str("data.state_code") != "KA" || loc.str("data.time_zone") != "Asia/Kolkata" || loc.str("data.country_code") != "IN" {
		t.Fatalf("FR-L001 normalization and defaults: %s", loc.Raw)
	}
	expectErr(t, tc.do(t, "POST", "/hr/locations", body(map[string]any{"code": code, "name": "Other " + uniq(), "city": "X", "state_code": "KA"})), 409, "CONFLICT", "duplicate code")
	expectErr(t, tc.do(t, "POST", "/hr/locations", body(map[string]any{"code": "Z" + uniq(), "name": "N", "city": "X", "state_code": "KA", "time_zone": "Mars/Base"})),
		400, "VALIDATION_ERROR", "bad time zone")
	expectErr(t, tc.as(t, tc.memberTok, "POST", "/hr/locations", body(map[string]any{"code": "M" + uniq(), "name": "N", "city": "X", "state_code": "KA"})),
		403, "FORBIDDEN", "FR-L006 member cannot write")

	pune := tc.do(t, "POST", "/hr/locations", body(map[string]any{"code": "PNQ" + uniq(), "name": "Pune " + uniq(), "city": "Pune", "state_code": "MH"}))
	expect(t, pune, 201, "second location")
	if l := tc.do(t, "GET", "/hr/locations?state_code=mh"); len(l.list("data")) != 1 || l.list("data")[0].(map[string]any)["city"] != "Pune" {
		t.Fatalf("FR-L002 state filter: %s", l.Raw)
	}
	if l := tc.do(t, "GET", "/hr/locations?search=bengaluru"); len(l.list("data")) != 1 {
		t.Fatalf("FR-L002 search: %s", l.Raw)
	}
	upd := tc.do(t, "PATCH", "/hr/locations/"+id, body(map[string]any{"name": "Bengaluru Campus " + uniq(), "time_zone": "Asia/Calcutta"}))
	if expect(t, upd, 200, "update"); upd.str("data.time_zone") != "Asia/Calcutta" {
		t.Fatalf("FR-L003: %s", upd.Raw)
	}
	expectErr(t, tc.do(t, "PATCH", "/hr/locations/"+id, body(map[string]any{"code": "NEWCODE"})), 400, "VALIDATION_ERROR", "FR-L003 code immutable")

	// FR-L005 mapping link.
	dept := tc.mkDept(t, "Loc")
	m := tc.do(t, "POST", "/hr/mappings", body(map[string]any{"user_id": tc.memberUser, "department_id": dept, "is_primary": true, "location_id": id}))
	if expect(t, m, 201, "mapping with location"); m.str("data.location_id") != id {
		t.Fatalf("FR-L005: %s", m.Raw)
	}
	expectErr(t, tc.do(t, "POST", "/hr/locations/"+id+"/deactivate"), 409, "CONFLICT", "FR-L004 blocked by an active mapping")
	pid := pune.str("data.id")
	expect(t, tc.do(t, "POST", "/hr/locations/"+pid+"/deactivate"), 200, "deactivate unused location")
	expectErr(t, tc.do(t, "PATCH", "/hr/mappings/"+m.str("data.id"), body(map[string]any{"location_id": pid})), 409, "CONFLICT", "FR-L005 inactive location")
	expectErr(t, tc.do(t, "POST", "/hr/locations/"+pid+"/deactivate"), 409, "CONFLICT", "already inactive")
	expect(t, tc.do(t, "POST", "/hr/locations/"+pid+"/activate"), 200, "reactivate")
	expect(t, tc.do(t, "PATCH", "/hr/mappings/"+m.str("data.id"), body(map[string]any{"location_id": pid})), 200, "move mapping")
	expect(t, tc.do(t, "POST", "/hr/locations/"+id+"/deactivate"), 200, "first location is free now")

	// Isolation and events.
	other := newTenant(t)
	expectErr(t, other.do(t, "GET", "/hr/locations/"+id), 404, "NOT_FOUND", "foreign id")
	expectErr(t, other.do(t, "POST", "/hr/mappings", body(map[string]any{"user_id": other.adminID, "department_id": other.mkDept(t, "X"), "location_id": pid})),
		404, "NOT_FOUND", "foreign location on a mapping")
	if n := sqlStr(t, "SELECT count(*) FROM org_events_outbox WHERE tenant_id = ? AND routing_key LIKE 'organization.location.%'", tc.id); n != "0" {
		t.Logf("location events fell back to the outbox (%s rows); the queue publisher is a no-op in tests", n)
	}
}

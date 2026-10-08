//go:build integration

package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

// attTenant is one isolated tenant with an admin (tenant_admin) and a plain member, both with employee profiles.
type attTenant struct {
	slug, host            string
	adminTok, memberTok   string
	adminUser, adminEmp   string
	memberUser, memberEmp string
	tenantID              string
}

func newAttTenant(t *testing.T) *attTenant {
	t.Helper()
	slug := "att-" + randomSuffix()
	bootstrapTenant(t, slug)
	seedE2EAdmin(t, slug)
	a := &attTenant{slug: slug, host: slug + ".localhost", tenantID: tenantIDBySlug(t, slug)}
	a.adminTok, a.adminUser = loginUser(t, a.host, "admin@"+slug+".com", slug)

	email := memberEmail(slug)
	status, env := doReq(t, http.MethodPost, "/api/v1/users", a.host, a.adminTok, map[string]string{
		"email": email, "password": "Secret123!", "first_name": "Mia", "last_name": "Member",
	})
	requireStatus(t, "create member user", status, 201, env)
	a.memberTok, a.memberUser = loginUser(t, a.host, email, slug)
	a.adminEmp = createEmployee(t, a, a.adminUser, "ADM")
	a.memberEmp = createEmployee(t, a, a.memberUser, "MEM")
	return a
}

func loginUser(t *testing.T, host, email, slug string) (token, userID string) {
	t.Helper()
	status, env := doReq(t, http.MethodPost, "/api/v1/auth/login", host, "", map[string]string{
		"email": email, "password": "Secret123!", "tenant_slug": slug,
	})
	requireStatus(t, "login "+email, status, 200, env)
	return strField(t, env.Data, "access_token"), nestedField(t, env.Data, "user", "id")
}

func createEmployee(t *testing.T, a *attTenant, userID, prefix string) string {
	t.Helper()
	status, env := doReq(t, http.MethodPost, "/api/v1/employees", a.host, a.adminTok, map[string]interface{}{
		"user_id": userID, "employee_code": prefix + "-" + strings.ToUpper(randomSuffix()), "first_name": "E2E", "last_name": prefix,
		"employment_type": "full_time", "joining_date": "2026-01-01T00:00:00Z",
	})
	requireStatus(t, "create employee "+prefix, status, 201, env)
	return strField(t, env.Data, "id")
}

func (a *attTenant) do(t *testing.T, tok, method, path string, body interface{}) (int, envelope) {
	t.Helper()
	return doReq(t, method, path, a.host, tok, body)
}

func day(offset int) string { return time.Now().UTC().AddDate(0, 0, offset).Format("2006-01-02") }

func at(date string, hhmm string) string { return date + "T" + hhmm + ":00Z" }

// listField decodes a list payload into generic maps.
func listField(t *testing.T, raw json.RawMessage) []map[string]interface{} {
	t.Helper()
	var out []map[string]interface{}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode list: %v (%s)", err, raw)
	}
	return out
}

func objField(t *testing.T, raw json.RawMessage) map[string]interface{} {
	t.Helper()
	var out map[string]interface{}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode object: %v (%s)", err, raw)
	}
	return out
}

func requireCode(t *testing.T, what string, got, want int, env envelope, code string) {
	t.Helper()
	requireStatus(t, what, got, want, env)
	if code != "" && (env.Error == nil || env.Error.Code != code) {
		t.Fatalf("%s: want error code %s, got %+v", what, code, env.Error)
	}
}

// backdatePunches shifts an employee's punches 2 minutes into the past so AT-004 (60 s debounce) does not mask AT-003.
func backdatePunches(t *testing.T, employeeID string) {
	t.Helper()
	psql(t, "UPDATE attendance_punches SET punch_time = punch_time - interval '2 minutes' WHERE employee_profile_id = '"+employeeID+"'")
}

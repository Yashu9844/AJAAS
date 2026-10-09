package integration

import (
	"strings"
	"testing"
	"time"
)

// timeCtx is a tenant for Modules 3/4: the tenant_admin and a plain member, both with employee profiles.
type timeCtx struct {
	*tenantCtx
	adminEmp              string
	memberUser, memberEmp string
	memberTok             string
}

func newTimeTenant(t *testing.T) *timeCtx {
	t.Helper()
	tc := &timeCtx{tenantCtx: newTenant(t)}
	r := tc.do(t, "POST", "/employees", body(empBody(tc.adminID, "ADM-"+strings.ToUpper(uniq()), nil)))
	expect(t, r, 201, "admin employee profile")
	tc.adminEmp = r.str("data.id")
	uid, email := tc.newUser(t, "mem")
	r = tc.do(t, "POST", "/employees", body(empBody(uid, "MEM-"+strings.ToUpper(uniq()), nil)))
	expect(t, r, 201, "member employee profile")
	tc.memberUser, tc.memberEmp = uid, r.str("data.id")
	tc.memberTok = login(t, tc.slug, email, password).str("data.access_token")
	return tc
}

// as calls the API as the member.
func (tc *timeCtx) as(t *testing.T, tok, method, path string, opts ...opt) resp {
	t.Helper()
	return call(t, method, path, append([]opt{host(tc.slug), token(tok)}, opts...)...)
}

// sqlStr runs a scalar query against the test database and returns it as text.
func sqlStr(t *testing.T, q string, args ...any) string {
	t.Helper()
	var out string
	if err := testDB.Raw("SELECT ("+q+")::text", args...).Scan(&out).Error; err != nil {
		t.Fatalf("sql %q: %v", q, err)
	}
	return out
}

// monday returns the Monday k weeks after the next Monday (UTC).
func monday(k int) time.Time {
	d := time.Now().UTC().Truncate(24*time.Hour).AddDate(0, 0, 1)
	for d.Weekday() != time.Monday {
		d = d.AddDate(0, 0, 1)
	}
	return d.AddDate(0, 0, 7*k)
}

func ymd(t time.Time) string { return t.Format("2006-01-02") }

func num(r resp, path string) float64 {
	f, _ := r.get(path).(float64)
	return f
}

package integration

import (
	"context"
	"strings"
	"testing"

	"github.com/jaas/jaas/internal/shared/database"
	"github.com/jaas/jaas/migrations"
)

func TestMigrationsAreIdempotentAndComplete(t *testing.T) {
	applied, err := database.RunMigrations(context.Background(), testDB, migrations.FS)
	if err != nil {
		t.Fatal(err)
	}
	if len(applied) != 0 {
		t.Fatalf("second run must apply nothing, applied %v", applied)
	}
	var n int64
	testDB.Raw("SELECT count(*) FROM schema_migrations").Scan(&n)
	files, _ := migrations.FS.ReadDir(".")
	ups := 0
	for _, f := range files {
		if strings.HasSuffix(f.Name(), ".up.sql") {
			ups++
		}
	}
	if int(n) != ups || ups < 25 {
		t.Fatalf("schema_migrations has %d rows for %d up files", n, ups)
	}
	var perms int64
	testDB.Raw("SELECT count(*) FROM permissions").Scan(&perms)
	if perms < 18 {
		t.Fatalf("permission catalogue not seeded: %d", perms)
	}
}

// Uniqueness of business codes is per tenant: two tenants may both own department "ENG", employee "EMP-001", ...
func TestBusinessCodesAreTenantScoped(t *testing.T) {
	a, b := newTenant(t), newTenant(t)
	for _, tc := range []*tenantCtx{a, b} {
		expect(t, tc.do(t, "POST", "/departments", body(map[string]string{"name": "Engineering", "code": "ENG"})), 201, "department in "+tc.slug)
		expect(t, tc.do(t, "POST", "/designations", body(map[string]string{"title": "Engineer", "code": "ENGR"})), 201, "designation in "+tc.slug)
		uid, _ := tc.newUser(t, "codes")
		expect(t, tc.do(t, "POST", "/employees", body(empBody(uid, "EMP-001", nil))), 201, "employee in "+tc.slug)
		did := tc.mkDept(t, "Teams")
		expect(t, tc.do(t, "POST", "/teams", body(map[string]string{"name": "Core", "code": "CORE", "department_id": did})), 201, "team in "+tc.slug)
	}
	// ...but never twice inside the same tenant
	expectErr(t, a.do(t, "POST", "/departments", body(map[string]string{"name": "Other", "code": "ENG"})), 409, "CONFLICT", "same-tenant duplicate")
	expectErr(t, a.do(t, "POST", "/departments", body(map[string]string{"name": "engineering"})), 409, "CONFLICT", "case-insensitive duplicate name")
}

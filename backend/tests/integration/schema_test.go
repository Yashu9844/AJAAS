package integration

import (
	"context"
	"io/fs"
	"testing"

	"github.com/jaas/jaas/internal/shared/database"
	"github.com/jaas/jaas/migrations"
)

func TestMigrationsAreIdempotentAndComplete(t *testing.T) {
	applied, err := database.RunMigrationSets(context.Background(), testDB, migrations.Sets())
	if err != nil {
		t.Fatal(err)
	}
	if len(applied) != 0 {
		t.Fatalf("second run must apply nothing, applied %v", applied)
	}
	for _, set := range migrations.Sets() {
		var n int64
		testDB.Raw("SELECT count(*) FROM " + set.Schema + ".schema_migrations").Scan(&n)
		ups, _ := fs.Glob(set.FS, "*.up.sql")
		if int(n) != len(ups) || len(ups) < 15 {
			t.Fatalf("%s.schema_migrations has %d rows for %d up files", set.Schema, n, len(ups))
		}
	}
	var perms int64
	testDB.Raw("SELECT count(*) FROM permissions").Scan(&perms)
	if perms < 18 {
		t.Fatalf("permission catalogue not seeded: %d", perms)
	}
}

// Each service owns one schema (DHR-09): identity tables in identity, HR tables in hr, nothing in public, and no
// foreign key crosses a schema boundary.
func TestSchemasAreOwnedAndIsolated(t *testing.T) {
	for table, schema := range map[string]string{"tenants": "identity", "users": "identity", "roles": "identity",
		"identity_events_outbox": "identity", "departments": "hr", "employee_profiles": "hr", "leave_ledger": "hr",
		"payroll_runs": "hr", "recruitment_jobs": "hr", "performance_cycles": "hr"} {
		if got := sqlStr(t, "SELECT table_schema FROM information_schema.tables WHERE table_name = ?", table); got != schema {
			t.Errorf("%s lives in %q, want %q", table, got, schema)
		}
	}
	if n := sqlStr(t, "SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public' AND table_type = 'BASE TABLE'"); n != "0" {
		t.Fatalf("no tables in public: %s", n)
	}
	cross := sqlStr(t, `SELECT count(*) FROM information_schema.referential_constraints rc
		JOIN information_schema.table_constraints fk ON fk.constraint_name = rc.constraint_name AND fk.constraint_schema = rc.constraint_schema
		JOIN information_schema.table_constraints pk ON pk.constraint_name = rc.unique_constraint_name AND pk.constraint_schema = rc.unique_constraint_schema
		WHERE fk.table_schema <> pk.table_schema`)
	if cross != "0" {
		t.Fatalf("foreign keys cross schema boundaries: %s", cross)
	}
}

// Uniqueness of business codes is per tenant: two tenants may both own department "ENG", employee "EMP-001", ...
func TestBusinessCodesAreTenantScoped(t *testing.T) {
	a, b := newTenant(t), newTenant(t)
	for _, tc := range []*tenantCtx{a, b} {
		expect(t, tc.do(t, "POST", "/hr/departments", body(map[string]string{"name": "Engineering", "code": "ENG"})), 201, "department in "+tc.slug)
		expect(t, tc.do(t, "POST", "/hr/designations", body(map[string]string{"title": "Engineer", "code": "ENGR"})), 201, "designation in "+tc.slug)
		uid, _ := tc.newUser(t, "codes")
		expect(t, tc.do(t, "POST", "/hr/employees", body(empBody(uid, "EMP-001", nil))), 201, "employee in "+tc.slug)
		did := tc.mkDept(t, "Teams")
		expect(t, tc.do(t, "POST", "/hr/teams", body(map[string]string{"name": "Core", "code": "CORE", "department_id": did})), 201, "team in "+tc.slug)
	}
	// ...but never twice inside the same tenant
	expectErr(t, a.do(t, "POST", "/hr/departments", body(map[string]string{"name": "Other", "code": "ENG"})), 409, "CONFLICT", "same-tenant duplicate")
	expectErr(t, a.do(t, "POST", "/hr/departments", body(map[string]string{"name": "engineering"})), 409, "CONFLICT", "case-insensitive duplicate name")
}

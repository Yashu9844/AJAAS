package access

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestScopeOrderAndValidity(t *testing.T) {
	order := []Scope{ScopeOwn, ScopeDirectReports, ScopeAllReports, ScopeDepartment, ScopeTenant}
	for i, s := range order {
		if !s.Valid() {
			t.Fatalf("%s should be valid", s)
		}
		for j, o := range order {
			if Covers(s, o) != (i >= j) {
				t.Errorf("Covers(%s, %s) = %v", s, o, Covers(s, o))
			}
		}
	}
	if Scope("galaxy").Valid() || ScopeNone.Valid() {
		t.Fatal("unknown and empty scopes are invalid")
	}
	if Covers(ScopeNone, ScopeOwn) || Covers(Scope("x"), ScopeOwn) {
		t.Fatal("none / unknown covers nothing")
	}
	if Wider(ScopeOwn, ScopeTenant) != ScopeTenant || Wider(ScopeDepartment, ScopeAllReports) != ScopeDepartment || Wider(ScopeNone, ScopeOwn) != ScopeOwn {
		t.Fatal("Wider picks the higher rank")
	}
	if p, err := ParseScope(""); err != nil || p != ScopeTenant {
		t.Fatal("empty scope defaults to tenant (legacy permission_ids)")
	}
	if _, err := ParseScope("galaxy"); !errors.Is(err, ErrInvalidScope) {
		t.Fatal("invalid scope rejected")
	}
}

// roles: employee <- manager <- hr_manager; side role recruiter (parent employee).
func fixture() (Input, map[string]uuid.UUID) {
	ids := map[string]uuid.UUID{"employee": uuid.New(), "manager": uuid.New(), "hr_manager": uuid.New(), "recruiter": uuid.New()}
	p := func(k string) *uuid.UUID { id := ids[k]; return &id }
	in := Input{
		Now: time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC),
		Roles: []RoleNode{
			{ID: ids["employee"], Name: "Employee", Automatic: true, Grants: []Grant{{"organization:read", ScopeTenant}}},
			{ID: ids["manager"], Name: "Manager", Automatic: true, ParentID: p("employee"), Grants: []Grant{{"leave:approve", ScopeDirectReports}, {"employee:read", ScopeAllReports}}},
			{ID: ids["hr_manager"], Name: "HR Manager", ParentID: p("manager"), Grants: []Grant{{"leave:approve", ScopeTenant}}},
			{ID: ids["recruiter"], Name: "Recruiter", ParentID: p("employee"), Grants: []Grant{{"recruitment:manage", ScopeTenant}}},
		},
	}
	return in, ids
}

func TestResolveInheritanceAndWidestScope(t *testing.T) {
	in, ids := fixture()
	in.Assigned = []Assignment{{RoleID: ids["hr_manager"], Source: SourceManual}, {RoleID: ids["manager"], Source: SourceAutomatic}}
	eff, err := Resolve(in)
	if err != nil {
		t.Fatal(err)
	}
	if eff.ScopeFor("leave:approve") != ScopeTenant {
		t.Fatalf("widest scope wins: %s", eff.ScopeFor("leave:approve"))
	}
	if eff.ScopeFor("employee:read") != ScopeAllReports || eff.ScopeFor("organization:read") != ScopeTenant {
		t.Fatal("inherited grants from manager and employee")
	}
	if eff.ScopeFor("recruitment:manage") != ScopeNone {
		t.Fatal("unassigned side role grants nothing")
	}
	e := eff["leave:approve"]
	kinds := map[SourceKind]bool{}
	for _, s := range e.Sources {
		kinds[s.Kind] = true
	}
	if !kinds[SourceRole] || !kinds[SourceInherited] && !kinds[SourceAutomaticRole] {
		t.Fatalf("sources explain where access comes from: %+v", e.Sources)
	}
}

func TestResolveUserGrantsExpiryAndBlocks(t *testing.T) {
	in, ids := fixture()
	in.Assigned = []Assignment{{RoleID: ids["manager"], Source: SourceAutomatic}}
	past, future := in.Now.Add(-time.Hour), in.Now.Add(time.Hour)
	in.UserGrants = []UserGrant{
		{Grant: Grant{"payroll:read", ScopeTenant}, ExpiresAt: &future},
		{Grant: Grant{"payroll:manage", ScopeTenant}, ExpiresAt: &past},
		{Grant: Grant{"leave:approve", ScopeDepartment}},
	}
	in.Blocks = []string{"employee:read", "payroll:read"}
	eff, err := Resolve(in)
	if err != nil {
		t.Fatal(err)
	}
	if eff.ScopeFor("payroll:manage") != ScopeNone {
		t.Fatal("expired grant ignored")
	}
	if eff.ScopeFor("leave:approve") != ScopeDepartment {
		t.Fatal("user grant widens the role scope")
	}
	if eff.ScopeFor("employee:read") != ScopeNone || eff.ScopeFor("payroll:read") != ScopeNone {
		t.Fatal("blocks always win")
	}
	if !eff["employee:read"].Blocked || eff["employee:read"].Scope != ScopeAllReports {
		t.Fatal("blocked entries keep the scope they would have had, for explanation")
	}
	if eff.Permissions()[0] == "employee:read" {
		t.Fatal("blocked permissions are not listed as held")
	}
}

func TestResolveRejectsCyclesAndDeepChains(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	in := Input{Roles: []RoleNode{{ID: a, ParentID: &b}, {ID: b, ParentID: &a}}, Assigned: []Assignment{{RoleID: a}}}
	if _, err := Resolve(in); !errors.Is(err, ErrRoleCycle) {
		t.Fatalf("cycle: %v", err)
	}
	chain := make([]RoleNode, MaxRoleDepth+2)
	for i := range chain {
		chain[i].ID = uuid.New()
		if i > 0 {
			parent := chain[i-1].ID
			chain[i].ParentID = &parent
		}
	}
	in = Input{Roles: chain, Assigned: []Assignment{{RoleID: chain[len(chain)-1].ID}}}
	if _, err := Resolve(in); !errors.Is(err, ErrRoleTooDeep) {
		t.Fatalf("depth: %v", err)
	}
	missing := Input{Assigned: []Assignment{{RoleID: uuid.New()}}}
	if eff, err := Resolve(missing); err != nil || len(eff) != 0 {
		t.Fatal("assignments to unknown roles are ignored")
	}
}

func TestCheckParent(t *testing.T) {
	in, ids := fixture()
	if err := CheckParent(in.Roles, ids["employee"], ids["hr_manager"]); !errors.Is(err, ErrRoleCycle) {
		t.Fatalf("employee under hr_manager makes a cycle: %v", err)
	}
	if err := CheckParent(in.Roles, ids["recruiter"], ids["hr_manager"]); err != nil {
		t.Fatalf("valid re-parent: %v", err)
	}
	if err := CheckParent(in.Roles, ids["recruiter"], ids["recruiter"]); !errors.Is(err, ErrRoleCycle) {
		t.Fatal("self parent")
	}
}

func TestCoversAll(t *testing.T) {
	in, ids := fixture()
	in.Assigned = []Assignment{{RoleID: ids["manager"]}}
	eff, _ := Resolve(in)
	if missing := eff.Missing([]Grant{{"leave:approve", ScopeOwn}, {"employee:read", ScopeAllReports}}); len(missing) != 0 {
		t.Fatalf("held: %v", missing)
	}
	missing := eff.Missing([]Grant{{"leave:approve", ScopeTenant}, {"payroll:read", ScopeOwn}})
	if len(missing) != 2 {
		t.Fatalf("not held: %v", missing)
	}
	if Key("leave", "approve") != "leave:approve" {
		t.Fatal("key format")
	}
	if eff.Permissions()[0] > eff.Permissions()[len(eff.Permissions())-1] {
		t.Fatal("permissions sorted")
	}
}

func TestParseScopeAndCheckParentEdges(t *testing.T) {
	if s, err := ParseScope("department"); err != nil || s != ScopeDepartment {
		t.Fatal("explicit scope parses")
	}
	in, ids := fixture()
	if err := CheckParent(in.Roles, uuid.New(), ids["employee"]); err != nil {
		t.Fatalf("a new role may take any existing parent: %v", err)
	}
	in.Assigned = []Assignment{{RoleID: ids["employee"], Source: SourceManual}}
	in.Blocks = []string{"organization:read", "never:granted"}
	eff, _ := Resolve(in)
	if eff.ScopeFor("organization:read") != ScopeNone || eff["organization:read"].Sources[0].Kind != SourceAutomaticRole {
		t.Fatal("automatic role kind and block on an existing entry")
	}
}

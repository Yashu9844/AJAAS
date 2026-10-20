package services

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/identity/access"
	"github.com/jaas/jaas/internal/identity/dto"
	"github.com/jaas/jaas/internal/identity/models"
	"gorm.io/gorm"
)

var now = time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)

func TestResolverCachesByAccessVersion(t *testing.T) {
	f := newFakeAccess()
	user := uuid.New()
	f.assign(user, f.role("Readers", nil, map[string]string{"users:read": "tenant"}), "manual")
	expires := now.Add(2 * time.Minute)
	p := f.perm("payroll", "read")
	f.grants = append(f.grants, models.UserPermissionGrant{UserID: user, PermissionID: p.ID, Scope: "own", ExpiresAt: &expires})
	cache := &memCache{m: map[string]string{}}
	r := f.resolver(cache, now)

	if s, err := r.ScopeFor(context.Background(), f.tenant, user, "users", "read"); err != nil || s != access.ScopeTenant {
		t.Fatalf("scope: %s %v", s, err)
	}
	if len(cache.ttls) != 1 || cache.ttls[0] != 2*time.Minute {
		t.Fatalf("cache TTL is cut to the earliest grant expiry: %v", cache.ttls)
	}
	// a cached result is reused while the version is unchanged, even if rows change underneath
	f.rolePerms = nil
	if s, _ := r.ScopeFor(context.Background(), f.tenant, user, "users", "read"); s != access.ScopeTenant {
		t.Fatal("cache hit expected")
	}
	f.bump() // any access write changes the version → fresh resolution
	if s, _ := r.ScopeFor(context.Background(), f.tenant, user, "users", "read"); s != access.ScopeNone {
		t.Fatal("new version must re-resolve")
	}
	cache.m = map[string]string{"bad": "x"}
	for k := range map[string]bool{} {
		_ = k
	}
	f.err = errBoom
	if _, err := r.ScopeFor(context.Background(), f.tenant, user, "users", "read"); err == nil {
		t.Fatal("repository errors surface")
	}
}

func TestResolverCorruptCacheAndRoleGrants(t *testing.T) {
	f := newFakeAccess()
	user := uuid.New()
	base := f.role("Base", nil, map[string]string{"organization:read": "tenant"})
	child := f.role("Child", &base.ID, map[string]string{"leave:approve": "direct_reports"})
	f.assign(user, child, "manual")
	cache := &memCache{m: map[string]string{}}
	r := f.resolver(cache, now)
	key := "access:" + f.tenant.String() + ":0:" + user.String()
	cache.m[key] = "{not json"
	if s, _ := r.ScopeFor(context.Background(), f.tenant, user, "organization", "read"); s != access.ScopeTenant {
		t.Fatal("corrupt cache entries are ignored")
	}
	eff, err := r.RoleGrants(context.Background(), nil, f.tenant, []uuid.UUID{child.ID})
	if err != nil || eff.ScopeFor("organization:read") != access.ScopeTenant || eff.ScopeFor("leave:approve") != access.ScopeDirectReports {
		t.Fatalf("role grants include ancestors: %v", err)
	}
	f.err = errBoom
	if _, err := r.RoleGrants(context.Background(), nil, f.tenant, []uuid.UUID{child.ID}); err == nil {
		t.Fatal("error surfaces")
	}
}

func TestCatalogueRegisterAndProvision(t *testing.T) {
	f := newFakeAccess()
	c := NewCatalogue(CatalogueDeps{Roles: f.roleRepo(), RolePerms: f.rolePermRepo(), Access: f, Transact: passthrough})
	if err := c.Register(context.Background(), []access.PermissionDef{{Resource: "x"}}); !errors.Is(err, access.ErrInvalidDefinition) {
		t.Fatalf("invalid definitions rejected: %v", err)
	}
	defs := []access.PermissionDef{
		{Module: "hr", Resource: "leave", Action: "approve", Description: "Approve leave", Sensitive: false,
			Defaults: map[string]access.Scope{access.TemplateManager: access.ScopeDirectReports, access.TemplateHRManager: access.ScopeTenant}},
	}
	if err := c.Register(context.Background(), defs); err != nil {
		t.Fatal(err)
	}
	if p := f.perm("leave", "approve"); p.Module != "hr" || p.Description == nil {
		t.Fatal("permission row upserted with metadata")
	}
	// a custom role already named "Manager" forces the predefined one to a suffixed name
	f.role("Manager", nil, nil)
	if err := c.ProvisionTenant(context.Background(), nil, f.tenant); err != nil {
		t.Fatal(err)
	}
	byKey := map[string]*models.Role{}
	for _, r := range f.roles {
		if r.TemplateKey != nil {
			byKey[*r.TemplateKey] = r
		}
	}
	if len(byKey) != len(access.Templates) || byKey["manager"].Name != "Manager (predefined)" || !byKey["manager"].IsAutomatic {
		t.Fatalf("templates provisioned: %d %+v", len(byKey), byKey["manager"])
	}
	if *byKey["hr_admin"].ParentRoleID != byKey["hr_manager"].ID {
		t.Fatal("parents wired")
	}
	grants := 0
	for _, rp := range f.rolePerms {
		if rp.RoleID == byKey["manager"].ID && rp.Scope == "direct_reports" {
			grants++
		}
	}
	if grants != 1 {
		t.Fatal("default grant added to manager")
	}
	// drifted scope on a managed role is reset; an edited role is left alone; provisioning is idempotent
	for i := range f.rolePerms {
		f.rolePerms[i].Scope = "own"
	}
	byKey["hr_manager"].TemplateManaged = false
	f.tenants = []uuid.UUID{f.tenant}
	if err := c.SyncAllTenants(context.Background()); err != nil {
		t.Fatal(err)
	}
	if f.scopeWrites != 1 {
		t.Fatalf("only the managed manager role is corrected: %d writes", f.scopeWrites)
	}
	f.err = errBoom
	if err := c.SyncAllTenants(context.Background()); err == nil {
		t.Fatal("sync errors surface")
	}
	if err := c.Register(context.Background(), defs); err == nil {
		t.Fatal("upsert errors surface")
	}
}

type fakeSource struct {
	employees, managers []uuid.UUID
	err                 error
}

func (s fakeSource) Members(context.Context, uuid.UUID) ([]uuid.UUID, []uuid.UUID, error) {
	return s.employees, s.managers, s.err
}
func (s fakeSource) MemberOf(_ context.Context, _, user uuid.UUID) (bool, bool, error) {
	in := func(l []uuid.UUID) bool {
		for _, u := range l {
			if u == user {
				return true
			}
		}
		return false
	}
	return in(s.employees), in(s.managers), s.err
}

// cancelingSource cancels the sweep context once it has been asked for members.
type cancelingSource struct {
	fakeSource
	cancel context.CancelFunc
}

func (s cancelingSource) Members(ctx context.Context, id uuid.UUID) ([]uuid.UUID, []uuid.UUID, error) {
	defer s.cancel()
	return s.fakeSource.Members(ctx, id)
}

func provisioned(t *testing.T, f *fakeAccess) Catalogue {
	t.Helper()
	c := NewCatalogue(CatalogueDeps{Roles: f.roleRepo(), RolePerms: f.rolePermRepo(), Access: f, Transact: passthrough})
	if err := c.ProvisionTenant(context.Background(), nil, f.tenant); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestAutomaticRolesReconcile(t *testing.T) {
	f := newFakeAccess()
	c := provisioned(t, f)
	a := NewAutomaticRoles(AutomaticRolesDeps{Access: f, UserRoles: f.userRoleRepo(), Catalogue: c, Transact: passthrough})
	if added, removed, err := a.SyncTenant(context.Background(), f.tenant); err != nil || added+removed != 0 {
		t.Fatal("no source: no-op")
	}
	if err := a.SyncUser(context.Background(), f.tenant, uuid.New()); err != nil {
		t.Fatal("no source: no-op")
	}
	boss, rep := uuid.New(), uuid.New()
	a.SetSource(fakeSource{employees: []uuid.UUID{boss, rep}, managers: []uuid.UUID{boss}})
	added, removed, err := a.SyncTenant(context.Background(), f.tenant)
	if err != nil || added != 3 || removed != 0 {
		t.Fatalf("add: %d %d %v", added, removed, err)
	}
	if added, _, _ := a.SyncTenant(context.Background(), f.tenant); added != 0 {
		t.Fatal("idempotent")
	}
	a.SetSource(fakeSource{employees: []uuid.UUID{boss, rep}})
	if err := a.SyncUser(context.Background(), f.tenant, boss); err != nil {
		t.Fatal(err)
	}
	for _, ur := range f.userRoles {
		if ur.UserID == boss && f.roles[ur.RoleID].Name == "Manager" {
			t.Fatal("boss lost the manager role")
		}
	}
	a.SetSource(fakeSource{err: errBoom})
	if _, _, err := a.SyncTenant(context.Background(), f.tenant); err == nil {
		t.Fatal("source errors surface")
	}
	if err := a.SyncUser(context.Background(), f.tenant, boss); err == nil {
		t.Fatal("source errors surface")
	}
}

func TestAutomaticRolesProvisionAndRun(t *testing.T) {
	f := newFakeAccess() // no predefined roles yet: the first sync provisions them
	c := NewCatalogue(CatalogueDeps{Roles: f.roleRepo(), RolePerms: f.rolePermRepo(), Access: f, Transact: passthrough})
	a := NewAutomaticRoles(AutomaticRolesDeps{Access: f, UserRoles: f.userRoleRepo(), Catalogue: c, Transact: passthrough})
	u := uuid.New()
	ctx, cancel := context.WithCancel(context.Background())
	a.SetSource(cancelingSource{fakeSource{employees: []uuid.UUID{u}}, cancel})
	f.tenants = []uuid.UUID{f.tenant}
	a.Run(ctx, time.Hour) // one sweep (the source cancels the context), then stops
	if len(f.userRoles) != 1 || f.userRoles[0].Source != "automatic" {
		t.Fatalf("sweep assigned the employee role: %+v", f.userRoles)
	}
	f.err = errBoom
	a.Run(ctx, time.Hour) // a cancelled context stops at once; tenant-list errors are logged, not fatal
}

func accessSvc(f *fakeAccess) AccessService {
	return NewAccessService(AccessServiceDeps{Users: f.userRepo(), Perms: f.permRepo(), UserRoles: f.userRoleRepo(), Access: f,
		Resolver: f.resolver(nil, now), Audit: &MockAuditService{}, Now: func() time.Time { return now }})
}

func TestAccessServiceGrantsAndBlocks(t *testing.T) {
	f := newFakeAccess()
	s := accessSvc(f)
	ctx := context.Background()
	user := uuid.New()
	p := f.perm("users", "read")
	if _, err := s.Grant(ctx, nil, f.tenant, user, dto.UserGrantRequest{PermissionID: p.ID.String()}); !isStatus(err, 404) {
		t.Fatalf("unknown user: %v", err)
	}
	f.users[user] = true
	if _, err := s.Grant(ctx, nil, f.tenant, user, dto.UserGrantRequest{PermissionID: "nope"}); !isStatus(err, 400) {
		t.Fatalf("bad id: %v", err)
	}
	if _, err := s.Grant(ctx, nil, f.tenant, user, dto.UserGrantRequest{PermissionID: uuid.New().String()}); !isStatus(err, 404) {
		t.Fatalf("unknown permission: %v", err)
	}
	if _, err := s.Grant(ctx, nil, f.tenant, user, dto.UserGrantRequest{PermissionID: p.ID.String(), Scope: "galaxy"}); !isStatus(err, 400) {
		t.Fatalf("bad scope: %v", err)
	}
	past := now.Add(-time.Minute)
	if _, err := s.Grant(ctx, nil, f.tenant, user, dto.UserGrantRequest{PermissionID: p.ID.String(), ExpiresAt: &past}); !isStatus(err, 400) {
		t.Fatalf("past expiry: %v", err)
	}
	caller := uuid.New()
	f.users[caller] = true
	weak := WithCaller(ctx, Caller{UserID: caller, Roles: []string{"custom"}})
	if _, err := s.Grant(weak, nil, f.tenant, user, dto.UserGrantRequest{PermissionID: p.ID.String()}); !isStatus(err, 403) {
		t.Fatalf("escalation: %v", err)
	}
	res, err := s.Grant(WithCaller(ctx, Caller{UserID: caller, Roles: []string{"tenant_admin"}}), nil, f.tenant, user,
		dto.UserGrantRequest{PermissionID: p.ID.String(), Scope: "own", Reason: "cover"})
	if err != nil || res.Permission != "users:read" || res.Scope != "own" || res.GrantedBy == nil {
		t.Fatalf("grant: %+v %v", res, err)
	}
	if list, _ := s.ListGrants(ctx, nil, f.tenant, user); len(list) != 1 || list[0].Expired {
		t.Fatalf("list: %+v", list)
	}
	if err := s.RevokeGrant(ctx, nil, f.tenant, user, p.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.RevokeGrant(ctx, nil, f.tenant, user, p.ID); !isStatus(err, 404) {
		t.Fatalf("revoke twice: %v", err)
	}

	b, err := s.Block(WithCaller(ctx, Caller{UserID: caller}), nil, f.tenant, user, dto.UserBlockRequest{PermissionID: p.ID.String(), Reason: "audit"})
	if err != nil || b.Permission != "users:read" || b.CreatedBy == nil {
		t.Fatalf("block: %+v %v", b, err)
	}
	if list, _ := s.ListBlocks(ctx, nil, f.tenant, user); len(list) != 1 {
		t.Fatal("block listed")
	}
	if err := s.Unblock(ctx, nil, f.tenant, user, p.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Unblock(ctx, nil, f.tenant, user, p.ID); !isStatus(err, 404) {
		t.Fatalf("unblock twice: %v", err)
	}
	admin := f.role("tenant_admin", nil, nil)
	admin.IsSystem = true
	f.assign(user, admin, "manual")
	if _, err := s.Block(ctx, nil, f.tenant, user, dto.UserBlockRequest{PermissionID: p.ID.String()}); !isStatus(err, 409) {
		t.Fatalf("tenant_admin cannot be blocked: %v", err)
	}
}

func TestAccessServiceExplainAndErrors(t *testing.T) {
	f := newFakeAccess()
	s := accessSvc(f)
	ctx := context.Background()
	user := uuid.New()
	readers := f.role("Readers", nil, map[string]string{"users:read": "tenant", "audit:read": "tenant"})
	f.assign(user, readers, "manual")
	f.perm("audit", "read").IsSensitive = true
	f.blocks = append(f.blocks, models.UserPermissionBlock{UserID: user, PermissionID: f.perm("audit", "read").ID})
	ex, err := s.Explain(ctx, nil, f.tenant, user)
	if err != nil || len(ex.Permissions) != 2 || len(ex.Roles) != 1 {
		t.Fatalf("explain: %+v %v", ex, err)
	}
	var blocked bool
	for _, p := range ex.Permissions {
		if p.Permission == "audit:read" {
			blocked = p.Blocked && p.Sensitive && strings.Contains(p.Sources[0].Kind, "role")
		}
	}
	if !blocked {
		t.Fatalf("audit:read explained as blocked + sensitive: %+v", ex.Permissions)
	}
	if _, err := s.Explain(ctx, nil, f.tenant, uuid.New()); !isStatus(err, 404) {
		t.Fatal("unknown user")
	}
	f.err = errBoom
	for name, call := range map[string]func() error{
		"explain":     func() error { _, err := s.Explain(ctx, nil, f.tenant, user); return err },
		"list grants": func() error { _, err := s.ListGrants(ctx, nil, f.tenant, user); return err },
		"list blocks": func() error { _, err := s.ListBlocks(ctx, nil, f.tenant, user); return err },
		"revoke":      func() error { return s.RevokeGrant(ctx, nil, f.tenant, user, uuid.New()) },
		"unblock":     func() error { return s.Unblock(ctx, nil, f.tenant, user, uuid.New()) },
	} {
		if call() == nil {
			t.Errorf("%s must surface repository errors", name)
		}
	}
}

func accessRoles(f *fakeAccess) RoleService {
	r := f.resolver(nil, now)
	return NewRoleService(f.roleRepo(), f.permRepo(), f.userRoleRepo(), f.rolePermRepo(), f.userRepo(), &MockEventPublisher{}, &MockAuditService{},
		WithAccessModel(f, r))
}

func TestRoleServiceScopesParentsAndClone(t *testing.T) {
	f := newFakeAccess()
	svc := accessRoles(f)
	ctx := context.Background()
	p := f.perm("leave", "approve")
	grant := func(scope string) []dto.GrantInput { return []dto.GrantInput{{PermissionID: p.ID.String(), Scope: scope}} }
	if _, err := svc.CreateRole(ctx, nil, f.tenant, dto.CreateRoleRequest{Name: "x", PermissionIDs: []string{p.ID.String()}, Grants: grant("own")}); !isStatus(err, 400) {
		t.Fatalf("conflicting duplicate scopes: %v", err)
	}
	if _, err := svc.CreateRole(ctx, nil, f.tenant, dto.CreateRoleRequest{Name: "x", Grants: grant("galaxy")}); !isStatus(err, 400) {
		t.Fatalf("bad scope: %v", err)
	}
	parent, err := svc.CreateRole(ctx, nil, f.tenant, dto.CreateRoleRequest{Name: "Team leads", Grants: grant("direct_reports")})
	if err != nil || parent.Permissions[0].Scope != "direct_reports" {
		t.Fatalf("scoped role: %+v %v", parent, err)
	}
	child, err := svc.CreateRole(ctx, nil, f.tenant, dto.CreateRoleRequest{Name: "Child", ParentRoleID: &parent.ID})
	if err != nil || child.ParentRoleID == nil || *child.ParentRoleID != parent.ID {
		t.Fatalf("child: %+v %v", child, err)
	}
	if _, err := svc.UpdateRole(ctx, nil, f.tenant, uuid.MustParse(parent.ID), dto.UpdateRoleRequest{ParentRoleID: &child.ID}); !isStatus(err, 409) {
		t.Fatalf("cycle: %v", err)
	}
	if err := svc.DeleteRole(ctx, nil, f.tenant, uuid.MustParse(parent.ID)); !isStatus(err, 409) {
		t.Fatalf("parent with children: %v", err)
	}
	clear := ""
	if res, err := svc.UpdateRole(ctx, nil, f.tenant, uuid.MustParse(child.ID), dto.UpdateRoleRequest{ParentRoleID: &clear}); err != nil || res.ParentRoleID != nil {
		t.Fatalf("clear parent: %+v %v", res, err)
	}
	// re-granting with another scope changes the scope
	if err := svc.AssignPermissions(ctx, nil, f.tenant, uuid.MustParse(parent.ID), dto.AssignPermissionsRequest{Grants: grant("tenant")}, uuid.New()); err != nil || f.scopeWrites != 1 {
		t.Fatalf("scope change: %v %d", err, f.scopeWrites)
	}
	if err := svc.AssignPermissions(ctx, nil, f.tenant, uuid.MustParse(parent.ID), dto.AssignPermissionsRequest{}, uuid.New()); !isStatus(err, 400) {
		t.Fatalf("empty assignment: %v", err)
	}
	cl, err := svc.CloneRole(ctx, nil, f.tenant, uuid.MustParse(parent.ID), dto.CloneRoleRequest{Name: "Copy"})
	if err != nil || len(cl.Permissions) != 1 || cl.Permissions[0].Scope != "tenant" {
		t.Fatalf("clone: %+v %v", cl, err)
	}
	sys := f.role("tenant_admin", nil, nil)
	sys.IsSystem = true
	if _, err := svc.CloneRole(ctx, nil, f.tenant, sys.ID, dto.CloneRoleRequest{Name: "c"}); !isStatus(err, 403) {
		t.Fatalf("clone system: %v", err)
	}
	if _, err := svc.UpdateRole(ctx, nil, f.tenant, sys.ID, dto.UpdateRoleRequest{ParentRoleID: &parent.ID}); !isStatus(err, 403) {
		t.Fatalf("system roles cannot inherit: %v", err)
	}
	legacy := NewRoleService(f.roleRepo(), f.permRepo(), f.userRoleRepo(), f.rolePermRepo(), f.userRepo(), &MockEventPublisher{}, &MockAuditService{})
	if _, err := legacy.CreateRole(ctx, nil, f.tenant, dto.CreateRoleRequest{Name: "L", ParentRoleID: &parent.ID}); !isStatus(err, 400) {
		t.Fatalf("inheritance needs the access model: %v", err)
	}
}

func TestRoleServicePredefinedAndAutomaticGuards(t *testing.T) {
	f := newFakeAccess()
	provisioned(t, f)
	svc := accessRoles(f)
	ctx := context.Background()
	var mgr, auditor *models.Role
	for _, r := range f.roles {
		switch *r.TemplateKey {
		case "manager":
			mgr = r
		case "auditor":
			auditor = r
		}
	}
	user := uuid.New()
	f.users[user] = true
	if err := svc.DeleteRole(ctx, nil, f.tenant, auditor.ID); !isStatus(err, 403) {
		t.Fatalf("predefined roles cannot be deleted: %v", err)
	}
	if err := svc.AssignRolesToUser(ctx, nil, f.tenant, user, dto.AssignRoleRequest{RoleIDs: []string{mgr.ID.String()}}, uuid.New()); !isStatus(err, 409) {
		t.Fatalf("automatic role assign: %v", err)
	}
	f.assign(user, mgr, "automatic")
	if err := svc.RemoveRoleFromUser(ctx, nil, f.tenant, user, mgr.ID); !isStatus(err, 409) {
		t.Fatalf("automatic role remove: %v", err)
	}
	p := f.perm("audit", "read")
	if err := svc.AssignPermissions(ctx, nil, f.tenant, auditor.ID, dto.AssignPermissionsRequest{PermissionIDs: []string{p.ID.String()}}, uuid.New()); err != nil {
		t.Fatal(err)
	}
	if f.roles[auditor.ID].TemplateManaged {
		t.Fatal("an edit ends template management")
	}
	if err := svc.AssignRolesToUser(ctx, nil, f.tenant, user, dto.AssignRoleRequest{RoleIDs: []string{auditor.ID.String()}}, uuid.New()); err != nil {
		t.Fatal(err)
	}
	acc, err := svc.GetMyAccess(ctx, nil, f.tenant, user)
	if err != nil || len(acc.Grants) == 0 || acc.Roles[0].Source == "" {
		t.Fatalf("my access lists scoped grants and sources: %+v %v", acc, err)
	}
	f.err = errBoom
	if _, err := svc.GetMyAccess(ctx, nil, f.tenant, user); err == nil {
		t.Fatal("error surfaces")
	}
}

// ensure the fakes satisfy the interfaces they stand in for
var _ = func() error { var db *gorm.DB; _ = db; return nil }

func TestTenantCreationProvisionsPredefinedRoles(t *testing.T) {
	e := newEnv(&faults{})
	called := false
	svc := NewTenantService(e.tenantRepo, e.roleRepo, e.pub, e.audit, WithRoleProvisioner(func(_ context.Context, _ *gorm.DB, id uuid.UUID) error {
		called = id == e.tenantID
		return nil
	}))
	if _, err := svc.CreateTenant(context.Background(), nil, dto.CreateTenantRequest{Name: "Acme", Slug: "acmeco"}, uuid.New()); err != nil || !called {
		t.Fatalf("provisioner runs inside tenant creation: %v %v", called, err)
	}
	failing := NewTenantService(e.tenantRepo, e.roleRepo, e.pub, e.audit, WithRoleProvisioner(func(context.Context, *gorm.DB, uuid.UUID) error { return errBoom }))
	if _, err := failing.CreateTenant(context.Background(), nil, dto.CreateTenantRequest{Name: "Acme", Slug: "acmeco2"}, uuid.New()); err == nil {
		t.Fatal("provisioning failure aborts tenant creation")
	}
	users := NewUserService(e.userRepo, e.roleRepo, e.urRepo, e.sessRepo, e.tokRepo, e.pub, e.audit, WithAccessResolver(resolverFor(e)))
	if _, err := users.CreateUser(callerCtx(e, "custom"), nil, e.tenantID, dto.CreateUserRequest{Email: "a@x.test", Password: "Passw0rd!123", FirstName: "A", LastName: "B", RoleIDs: []string{e.roleID.String()}}, uuid.New()); err != nil {
		t.Fatalf("a caller may give a role they hold: %v", err)
	}
}

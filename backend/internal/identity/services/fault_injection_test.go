package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/identity/dto"
	"github.com/jaas/jaas/internal/identity/models"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// These tests drive every service operation through its happy path once, count the dependency calls it makes, then
// re-run it failing each dependency call in turn. A failing dependency must never panic and (for the calls whose error
// is not deliberately best-effort, like audit/publish) must surface as an error — this covers every error branch.

var errBoom = errors.New("boom")

type faults struct{ calls, failAt int }

func (f *faults) hit() error {
	f.calls++
	if f.failAt > 0 && f.calls == f.failAt {
		return errBoom
	}
	return nil
}

// sweep runs op fault-free, then once per dependency call with that call failing.
func sweep(t *testing.T, name string, op func(f *faults) error) {
	t.Helper()
	happy := &faults{}
	if err := op(happy); err != nil {
		t.Fatalf("%s: happy path failed: %v", name, err)
	}
	surfaced := 0
	for i := 1; i <= happy.calls; i++ {
		f := &faults{failAt: i}
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("%s: panic when dependency call #%d fails: %v", name, i, r)
				}
			}()
			if err := op(f); errors.Is(err, errBoom) {
				surfaced++
			}
		}()
	}
	if happy.calls > 0 && surfaced == 0 {
		t.Errorf("%s: no injected failure surfaced as an error", name)
	}
}

type env struct {
	f *faults

	tenantStatus string
	userStatus   string
	slugExists   bool
	emailExists  bool
	roleSystem   bool
	roleName     string
	holders      int64

	tenantRepo *MockTenantRepository
	userRepo   *MockUserRepository
	roleRepo   *MockRoleRepository
	permRepo   *MockPermissionRepository
	urRepo     *MockUserRoleRepository
	rpRepo     *MockRolePermissionRepository
	sessRepo   *MockSessionRepository
	tokRepo    *MockRefreshTokenRepository
	resetRepo  *MockPasswordResetTokenRepository
	tokenSvc   *MockTokenService
	sessSvc    *MockSessionService
	pub        *MockEventPublisher
	audit      *MockAuditService

	tenantID, userID, roleID, permID uuid.UUID
	pwHash                           string
}

func newEnv(f *faults) *env {
	e := &env{
		f: f, tenantStatus: "active", userStatus: "active", roleName: "custom",
		tenantRepo: &MockTenantRepository{}, userRepo: &MockUserRepository{}, roleRepo: &MockRoleRepository{},
		permRepo: &MockPermissionRepository{}, urRepo: &MockUserRoleRepository{}, rpRepo: &MockRolePermissionRepository{},
		sessRepo: &MockSessionRepository{}, tokRepo: &MockRefreshTokenRepository{}, resetRepo: &MockPasswordResetTokenRepository{},
		tokenSvc: &MockTokenService{}, sessSvc: &MockSessionService{}, pub: &MockEventPublisher{}, audit: &MockAuditService{},
		tenantID: uuid.New(), userID: uuid.New(), roleID: uuid.New(), permID: uuid.New(),
	}
	h, _ := bcrypt.GenerateFromPassword([]byte("Passw0rd!123"), bcrypt.MinCost)
	e.pwHash = string(h)

	tenant := func() *models.Tenant {
		t := &models.Tenant{Slug: "acme", Name: "Acme", Status: e.tenantStatus, Plan: "free"}
		t.ID = e.tenantID
		return t
	}
	user := func() *models.User {
		u := &models.User{Email: "u@x.test", PasswordHash: e.pwHash, FirstName: "F", LastName: "L", Status: e.userStatus}
		u.ID, u.TenantID = e.userID, e.tenantID
		return u
	}
	role := func() *models.Role {
		r := &models.Role{TenantID: e.tenantID, Name: e.roleName, IsSystem: e.roleSystem}
		r.ID = e.roleID
		return r
	}

	e.tenantRepo.CreateFunc = func(_ context.Context, _ *gorm.DB, t *models.Tenant) error { t.ID = e.tenantID; return f.hit() }
	e.tenantRepo.FindBySlugFunc = func(_ context.Context, _ *gorm.DB, _ string) (*models.Tenant, error) {
		if err := f.hit(); err != nil {
			return nil, err
		}
		if e.slugExists {
			return tenant(), nil
		}
		return nil, nil
	}
	e.tenantRepo.FindByIDFunc = func(_ context.Context, _ *gorm.DB, _ uuid.UUID) (*models.Tenant, error) {
		return tenant(), f.hit()
	}
	e.tenantRepo.UpdateFunc = func(_ context.Context, _ *gorm.DB, _ *models.Tenant) error { return f.hit() }
	e.tenantRepo.FindAllFunc = func(_ context.Context, _ *gorm.DB, _, _ int) ([]models.Tenant, int64, error) {
		return []models.Tenant{*tenant()}, 1, f.hit()
	}

	e.userRepo.CreateFunc = func(_ context.Context, _ *gorm.DB, u *models.User) error { u.ID = e.userID; return f.hit() }
	e.userRepo.FindByIDFunc = func(_ context.Context, _ *gorm.DB, _, _ uuid.UUID) (*models.User, error) {
		return user(), f.hit()
	}
	e.userRepo.FindByEmailFunc = func(_ context.Context, _ *gorm.DB, _ uuid.UUID, _ string) (*models.User, error) {
		if err := f.hit(); err != nil {
			return nil, err
		}
		if e.emailExists {
			return user(), nil
		}
		return nil, nil
	}
	e.userRepo.UpdateFunc = func(_ context.Context, _ *gorm.DB, _ *models.User) error { return f.hit() }
	e.userRepo.FindAllFunc = func(_ context.Context, _ *gorm.DB, _ uuid.UUID, _, _ int) ([]models.User, int64, error) {
		return []models.User{*user()}, 1, f.hit()
	}

	e.roleRepo.CreateFunc = func(_ context.Context, _ *gorm.DB, r *models.Role) error { r.ID = e.roleID; return f.hit() }
	e.roleRepo.FindByIDFunc = func(_ context.Context, _ *gorm.DB, _, _ uuid.UUID) (*models.Role, error) {
		return role(), f.hit()
	}
	e.roleRepo.FindByNameFunc = func(_ context.Context, _ *gorm.DB, _ uuid.UUID, _ string) (*models.Role, error) {
		return nil, f.hit()
	}
	e.roleRepo.FindAllFunc = func(_ context.Context, _ *gorm.DB, _ uuid.UUID, _, _ int) ([]models.Role, int64, error) {
		return []models.Role{*role()}, 1, f.hit()
	}
	e.roleRepo.UpdateFunc = func(_ context.Context, _ *gorm.DB, _ *models.Role) error { return f.hit() }
	e.roleRepo.DeleteFunc = func(_ context.Context, _ *gorm.DB, _, _ uuid.UUID) error { return f.hit() }

	e.permRepo.FindByIDsFunc = func(_ context.Context, _ *gorm.DB, ids []uuid.UUID) ([]models.Permission, error) {
		out := make([]models.Permission, len(ids))
		for i, id := range ids {
			out[i] = models.Permission{ID: id, Resource: "users", Action: "read"}
		}
		return out, f.hit()
	}

	e.urRepo.CreateFunc = func(_ context.Context, _ *gorm.DB, _ *models.UserRole) error { return f.hit() }
	e.urRepo.FindByUserIDFunc = func(_ context.Context, _ *gorm.DB, _, _ uuid.UUID) ([]models.UserRole, error) {
		r := role()
		return []models.UserRole{{UserID: e.userID, RoleID: e.roleID, Role: r}}, f.hit()
	}
	e.urRepo.CountByRoleIDFunc = func(_ context.Context, _ *gorm.DB, _, _ uuid.UUID) (int64, error) { return e.holders, f.hit() }
	e.urRepo.DeleteFunc = func(_ context.Context, _ *gorm.DB, _, _, _ uuid.UUID) error { return f.hit() }

	e.rpRepo.CreateFunc = func(_ context.Context, _ *gorm.DB, _ *models.RolePermission) error { return f.hit() }
	e.rpRepo.FindByRoleIDFunc = func(_ context.Context, _ *gorm.DB, _, _ uuid.UUID) ([]models.RolePermission, error) {
		return []models.RolePermission{{RoleID: e.roleID, PermissionID: e.permID, Permission: &models.Permission{ID: e.permID, Resource: "users", Action: "read"}}}, f.hit()
	}
	e.rpRepo.DeleteFunc = func(_ context.Context, _ *gorm.DB, _, _, _ uuid.UUID) error { return f.hit() }
	e.rpRepo.DeleteByRoleIDFunc = func(_ context.Context, _ *gorm.DB, _, _ uuid.UUID) error { return f.hit() }

	e.sessRepo.RevokeAllByUserIDFunc = func(_ context.Context, _ *gorm.DB, _, _ uuid.UUID) error { return f.hit() }
	e.tokRepo.RevokeAllByUserIDFunc = func(_ context.Context, _ *gorm.DB, _, _ uuid.UUID) error { return f.hit() }
	e.tokRepo.RevokeBySessionIDFunc = func(_ context.Context, _ *gorm.DB, _ uuid.UUID) error { return f.hit() }
	e.tokRepo.RevokeByIDFunc = func(_ context.Context, _ *gorm.DB, _ uuid.UUID) error { return f.hit() }
	e.tokRepo.CreateFunc = func(_ context.Context, _ *gorm.DB, _ *models.RefreshToken) error { return f.hit() }
	e.tokRepo.FindByTokenHashFunc = func(_ context.Context, _ *gorm.DB, _ string) (*models.RefreshToken, error) {
		rt := &models.RefreshToken{UserID: e.userID, TenantID: e.tenantID, ExpiresAt: time.Now().Add(time.Hour)}
		rt.ID = uuid.New()
		return rt, f.hit()
	}
	e.resetRepo.CreateFunc = func(_ context.Context, _ *gorm.DB, _ *models.PasswordResetToken) error { return f.hit() }
	e.resetRepo.MarkAsUsedFunc = func(_ context.Context, _ *gorm.DB, _ uuid.UUID) error { return f.hit() }
	e.resetRepo.FindByTokenHashFunc = func(_ context.Context, _ *gorm.DB, _ string) (*models.PasswordResetToken, error) {
		p := &models.PasswordResetToken{UserID: e.userID, TenantID: e.tenantID, ExpiresAt: time.Now().Add(time.Hour)}
		p.ID = uuid.New()
		return p, f.hit()
	}

	e.tokenSvc.HashOpaqueTokenFunc = func(s string) string { return "h" + s }
	e.tokenSvc.GenerateOpaqueTokenFunc = func() (string, string, error) { return "raw", "hash", f.hit() }
	e.tokenSvc.GenerateAccessTokenFunc = func(_, _, _ string, _ []string, _ string) (string, time.Time, error) {
		return "jwt", time.Now().Add(time.Minute), f.hit()
	}
	e.sessSvc.CreateSessionFunc = func(_ context.Context, _ *gorm.DB, _, _ uuid.UUID, _, _ string) (*models.Session, error) {
		s := &models.Session{}
		s.ID = uuid.New()
		return s, f.hit()
	}
	e.sessSvc.RevokeSessionFunc = func(_ context.Context, _ *gorm.DB, _ uuid.UUID) error { return f.hit() }
	e.sessSvc.RevokeAllForUserFunc = func(_ context.Context, _ *gorm.DB, _, _ uuid.UUID) error { return f.hit() }
	return e
}

func (e *env) roles() RoleService {
	return NewRoleService(e.roleRepo, e.permRepo, e.urRepo, e.rpRepo, e.userRepo, e.pub, e.audit)
}
func (e *env) users() UserService {
	return NewUserService(e.userRepo, e.roleRepo, e.urRepo, e.sessRepo, e.tokRepo, e.pub, e.audit)
}
func (e *env) tenants() TenantService {
	return NewTenantService(e.tenantRepo, e.roleRepo, e.pub, e.audit, WithAdminProvisioning(e.userRepo, e.urRepo))
}
func (e *env) auth() AuthService {
	return NewAuthService(e.tenantRepo, e.userRepo, e.urRepo, e.sessRepo, e.tokRepo, e.resetRepo, e.tokenSvc, e.sessSvc, e.pub, e.audit)
}

var ctx = context.Background()

func TestFaults_RoleService(t *testing.T) {
	pid := uuid.New().String()
	sweep(t, "CreateRole", func(f *faults) error {
		_, err := newEnv(f).roles().CreateRole(ctx, nil, uuid.New(), dto.CreateRoleRequest{Name: "n", Description: "d", PermissionIDs: []string{pid}})
		return err
	})
	sweep(t, "GetRoleByID", func(f *faults) error {
		_, err := newEnv(f).roles().GetRoleByID(ctx, nil, uuid.New(), uuid.New())
		return err
	})
	sweep(t, "ListRoles", func(f *faults) error { _, err := newEnv(f).roles().ListRoles(ctx, nil, uuid.New(), 1, 20); return err })
	sweep(t, "UpdateRole", func(f *faults) error {
		n := "renamed"
		_, err := newEnv(f).roles().UpdateRole(ctx, nil, uuid.New(), uuid.New(), dto.UpdateRoleRequest{Name: &n})
		return err
	})
	sweep(t, "DeleteRole", func(f *faults) error { return newEnv(f).roles().DeleteRole(ctx, nil, uuid.New(), uuid.New()) })
	sweep(t, "AssignPermissions", func(f *faults) error {
		e := newEnv(f)
		return e.roles().AssignPermissions(ctx, nil, e.tenantID, e.roleID, dto.AssignPermissionsRequest{PermissionIDs: []string{pid}}, uuid.New())
	})
	sweep(t, "AssignRolesToUser", func(f *faults) error {
		e := newEnv(f)
		other := uuid.New().String()
		return e.roles().AssignRolesToUser(ctx, nil, e.tenantID, e.userID, dto.AssignRoleRequest{RoleIDs: []string{other}}, uuid.New())
	})
	sweep(t, "RemovePermission", func(f *faults) error {
		e := newEnv(f)
		return e.roles().RemovePermission(ctx, nil, e.tenantID, e.roleID, e.permID)
	})
	sweep(t, "RemoveRoleFromUser", func(f *faults) error {
		e := newEnv(f)
		return e.roles().RemoveRoleFromUser(ctx, nil, e.tenantID, e.userID, e.roleID)
	})
	sweep(t, "RemoveRoleFromUser(last tenant_admin guard path)", func(f *faults) error {
		e := newEnv(f)
		e.roleSystem, e.roleName, e.holders = true, "tenant_admin", 2
		return e.roles().RemoveRoleFromUser(ctx, nil, e.tenantID, e.userID, e.roleID)
	})
	sweep(t, "GetMyAccess", func(f *faults) error {
		e := newEnv(f)
		_, err := e.roles().GetMyAccess(ctx, nil, e.tenantID, e.userID)
		return err
	})
}

func TestRoleService_BusinessRules(t *testing.T) {
	e := newEnv(&faults{})
	svc := e.roles()

	// delete blocked while assigned
	e.holders = 3
	if err := svc.DeleteRole(ctx, nil, e.tenantID, e.roleID); err == nil || !isStatus(err, 409) {
		t.Fatalf("assigned role must not be deletable: %v", err)
	}
	e.holders = 0
	e.roleSystem = true
	if err := svc.DeleteRole(ctx, nil, e.tenantID, e.roleID); !isStatus(err, 403) {
		t.Fatalf("system role must not be deletable: %v", err)
	}
	if err := svc.RemovePermission(ctx, nil, e.tenantID, e.roleID, e.permID); err != nil {
		t.Fatalf("non-admin system role permissions may change: %v", err) // roleName is "custom"
	}
	// tenant_admin permissions are fixed
	e.roleName = "tenant_admin"
	if err := svc.RemovePermission(ctx, nil, e.tenantID, e.roleID, e.permID); !isStatus(err, 403) {
		t.Fatalf("tenant_admin permissions are fixed: %v", err)
	}
	// last tenant_admin guard
	e.holders = 1
	if err := svc.RemoveRoleFromUser(ctx, nil, e.tenantID, e.userID, e.roleID); !isStatus(err, 409) {
		t.Fatalf("last tenant_admin must be protected: %v", err)
	}
	// permission not assigned / role not assigned
	e.rpRepo.FindByRoleIDFunc = func(context.Context, *gorm.DB, uuid.UUID, uuid.UUID) ([]models.RolePermission, error) {
		return nil, nil
	}
	e.roleName, e.roleSystem = "custom", false
	if err := svc.RemovePermission(ctx, nil, e.tenantID, e.roleID, uuid.New()); !isStatus(err, 404) {
		t.Fatalf("unassigned permission: %v", err)
	}
	e.urRepo.FindByUserIDFunc = func(context.Context, *gorm.DB, uuid.UUID, uuid.UUID) ([]models.UserRole, error) { return nil, nil }
	if err := svc.RemoveRoleFromUser(ctx, nil, e.tenantID, e.userID, e.roleID); !isStatus(err, 404) {
		t.Fatalf("unassigned role: %v", err)
	}
	// unknown permission => 404 before anything is written
	e.permRepo.FindByIDsFunc = func(context.Context, *gorm.DB, []uuid.UUID) ([]models.Permission, error) { return nil, nil }
	created := false
	e.roleRepo.CreateFunc = func(context.Context, *gorm.DB, *models.Role) error { created = true; return nil }
	_, err := svc.CreateRole(ctx, nil, e.tenantID, dto.CreateRoleRequest{Name: "x", PermissionIDs: []string{uuid.New().String()}})
	if !isStatus(err, 404) || created {
		t.Fatalf("unknown permission must fail before writing (created=%v err=%v)", created, err)
	}
	// invalid uuid
	if _, err := svc.CreateRole(ctx, nil, e.tenantID, dto.CreateRoleRequest{Name: "x", PermissionIDs: []string{"nope"}}); !isStatus(err, 400) {
		t.Fatalf("invalid uuid: %v", err)
	}
	// GetMyAccess aggregates + flags admin
	e2 := newEnv(&faults{})
	e2.roleName = "tenant_admin"
	acc, err := e2.roles().GetMyAccess(ctx, nil, e2.tenantID, e2.userID)
	if err != nil || !acc.IsTenantAdmin || len(acc.Permissions) != 1 || acc.Permissions[0] != "users:read" {
		t.Fatalf("access: %+v %v", acc, err)
	}
}

func TestFaults_UserService(t *testing.T) {
	rid := uuid.New().String()
	sweep(t, "CreateUser", func(f *faults) error {
		e := newEnv(f)
		_, err := e.users().CreateUser(ctx, nil, e.tenantID, dto.CreateUserRequest{Email: "n@x.test", Password: "Passw0rd!123", FirstName: "A", LastName: "B", Phone: "1", RoleIDs: []string{rid, rid}}, uuid.New())
		return err
	})
	sweep(t, "InviteUser", func(f *faults) error {
		e := newEnv(f)
		_, err := e.users().InviteUser(ctx, nil, e.tenantID, dto.InviteUserRequest{Email: "n@x.test", FirstName: "A", LastName: "B", RoleIDs: []string{rid}}, uuid.New())
		return err
	})
	sweep(t, "GetByID", func(f *faults) error {
		e := newEnv(f)
		_, err := e.users().GetByID(ctx, nil, e.tenantID, e.userID)
		return err
	})
	sweep(t, "ListUsers", func(f *faults) error {
		e := newEnv(f)
		_, err := e.users().ListUsers(ctx, nil, e.tenantID, 1, 20)
		return err
	})
	sweep(t, "UpdateUser", func(f *faults) error {
		e := newEnv(f)
		fn, ph, av := "a", "b", "https://x.test/a.png"
		_, err := e.users().UpdateUser(ctx, nil, e.tenantID, e.userID, dto.UpdateUserRequest{FirstName: &fn, LastName: &fn, Phone: &ph, AvatarURL: &av})
		return err
	})
	sweep(t, "ActivateUser", func(f *faults) error {
		e := newEnv(f)
		e.userStatus = "inactive"
		_, err := e.users().ActivateUser(ctx, nil, e.tenantID, e.userID)
		return err
	})
	sweep(t, "DeactivateUser", func(f *faults) error {
		e := newEnv(f)
		return e.users().DeactivateUser(ctx, nil, e.tenantID, e.userID, uuid.New())
	})
}

func TestUserService_BusinessRules(t *testing.T) {
	e := newEnv(&faults{})
	svc := e.users()
	if _, err := svc.ActivateUser(ctx, nil, e.tenantID, e.userID); !isStatus(err, 409) {
		t.Fatalf("activating an active user: %v", err)
	}
	e.userStatus = "inactive"
	if err := svc.DeactivateUser(ctx, nil, e.tenantID, e.userID, uuid.New()); !isStatus(err, 409) {
		t.Fatalf("deactivating twice: %v", err)
	}
	u, err := svc.ActivateUser(ctx, nil, e.tenantID, e.userID)
	if err != nil || u.Status != "active" || u.Roles == nil {
		t.Fatalf("activate: %+v %v", u, err)
	}
	e.emailExists = true
	if _, err := svc.CreateUser(ctx, nil, e.tenantID, dto.CreateUserRequest{Email: "u@x.test", Password: "Passw0rd!123", FirstName: "A", LastName: "B"}, uuid.New()); !isStatus(err, 409) {
		t.Fatalf("duplicate email: %v", err)
	}
	if _, err := svc.InviteUser(ctx, nil, e.tenantID, dto.InviteUserRequest{Email: "u@x.test", FirstName: "A", LastName: "B"}, uuid.New()); !isStatus(err, 409) {
		t.Fatalf("duplicate invite: %v", err)
	}
	e.emailExists = false
	if _, err := svc.CreateUser(ctx, nil, e.tenantID, dto.CreateUserRequest{Email: "n@x.test", Password: "Passw0rd!123", FirstName: "A", LastName: "B", RoleIDs: []string{"nope"}}, uuid.New()); !isStatus(err, 400) {
		t.Fatalf("invalid role uuid: %v", err)
	}
	e.roleRepo.FindByIDFunc = func(context.Context, *gorm.DB, uuid.UUID, uuid.UUID) (*models.Role, error) { return nil, nil }
	if _, err := svc.CreateUser(ctx, nil, e.tenantID, dto.CreateUserRequest{Email: "n@x.test", Password: "Passw0rd!123", FirstName: "A", LastName: "B", RoleIDs: []string{uuid.New().String()}}, uuid.New()); !isStatus(err, 404) {
		t.Fatalf("unknown role: %v", err)
	}
	if _, err := svc.InviteUser(ctx, nil, e.tenantID, dto.InviteUserRequest{Email: "n@x.test", FirstName: "A", LastName: "B", RoleIDs: []string{"nope"}}, uuid.New()); !isStatus(err, 400) {
		t.Fatalf("invite invalid role uuid: %v", err)
	}
	if _, err := svc.InviteUser(ctx, nil, e.tenantID, dto.InviteUserRequest{Email: "n@x.test", FirstName: "A", LastName: "B", RoleIDs: []string{uuid.New().String()}}, uuid.New()); !isStatus(err, 404) {
		t.Fatalf("invite unknown role: %v", err)
	}
}

func TestFaults_TenantService(t *testing.T) {
	admin := &dto.BootstrapAdminRequest{Email: "a@x.test", Password: "Passw0rd!123", FirstName: "A", LastName: "B"}
	sweep(t, "CreateTenant(with admin)", func(f *faults) error {
		e := newEnv(f)
		_, err := e.tenants().CreateTenant(ctx, nil, dto.CreateTenantRequest{Name: "N", Slug: "newslug", Domain: "n.example.com", Plan: "pro", Admin: admin}, uuid.New())
		return err
	})
	sweep(t, "CreateTenant(no admin)", func(f *faults) error {
		e := newEnv(f)
		_, err := e.tenants().CreateTenant(ctx, nil, dto.CreateTenantRequest{Name: "N", Slug: "newslug"}, uuid.New())
		return err
	})
	sweep(t, "GetTenantByID", func(f *faults) error {
		e := newEnv(f)
		_, err := e.tenants().GetTenantByID(ctx, nil, e.tenantID)
		return err
	})
	sweep(t, "GetTenantBySlug", func(f *faults) error {
		e := newEnv(f)
		e.slugExists = true
		_, err := e.tenants().GetTenantBySlug(ctx, nil, "acme")
		return err
	})
	sweep(t, "ListTenants", func(f *faults) error { e := newEnv(f); _, err := e.tenants().ListTenants(ctx, nil, 1, 20); return err })
	sweep(t, "UpdateTenant", func(f *faults) error {
		e := newEnv(f)
		n, d, p := "n", "d.example.com", "pro"
		_, err := e.tenants().UpdateTenant(ctx, nil, e.tenantID, dto.UpdateTenantRequest{Name: &n, Domain: &d, Plan: &p})
		return err
	})
	sweep(t, "SuspendTenant", func(f *faults) error {
		e := newEnv(f)
		_, err := e.tenants().SuspendTenant(ctx, nil, e.tenantID, uuid.New())
		return err
	})
	sweep(t, "ActivateTenant", func(f *faults) error {
		e := newEnv(f)
		e.tenantStatus = "suspended"
		_, err := e.tenants().ActivateTenant(ctx, nil, e.tenantID, uuid.New())
		return err
	})

	// business rules
	e := newEnv(&faults{})
	if _, err := e.tenants().ActivateTenant(ctx, nil, e.tenantID, uuid.New()); !isStatus(err, 409) {
		t.Fatalf("activate an active tenant: %v", err)
	}
	e.tenantStatus = "suspended"
	if _, err := e.tenants().SuspendTenant(ctx, nil, e.tenantID, uuid.New()); !isStatus(err, 409) {
		t.Fatalf("suspend a suspended tenant: %v", err)
	}
	plain := NewTenantService(e.tenantRepo, e.roleRepo, e.pub, e.audit) // admin provisioning not configured
	if _, err := plain.CreateTenant(ctx, nil, dto.CreateTenantRequest{Name: "N", Slug: "newslug", Admin: admin}, uuid.New()); !isStatus(err, 400) {
		t.Fatalf("admin without provisioning support: %v", err)
	}
}

func TestFaults_AuthService(t *testing.T) {
	sweep(t, "Login", func(f *faults) error {
		e := newEnv(f)
		e.slugExists, e.emailExists = true, true
		_, err := e.auth().Login(ctx, nil, dto.LoginRequest{TenantSlug: "acme", Email: "u@x.test", Password: "Passw0rd!123"}, "1.2.3.4", "ua", uuid.New())
		return err
	})
	sweep(t, "RefreshToken", func(f *faults) error {
		e := newEnv(f)
		_, err := e.auth().RefreshToken(ctx, nil, dto.RefreshTokenRequest{RefreshToken: "rt"}, uuid.New())
		return err
	})
	sweep(t, "Logout", func(f *faults) error {
		e := newEnv(f)
		return e.auth().Logout(ctx, nil, e.tenantID, e.userID, uuid.New(), uuid.New())
	})
	sweep(t, "ForgotPassword", func(f *faults) error {
		e := newEnv(f)
		e.slugExists, e.emailExists = true, true
		return e.auth().ForgotPassword(ctx, nil, dto.ForgotPasswordRequest{TenantSlug: "acme", Email: "u@x.test"}, uuid.New())
	})
	sweep(t, "ResetPassword", func(f *faults) error {
		e := newEnv(f)
		return e.auth().ResetPassword(ctx, nil, dto.ResetPasswordRequest{Token: "t", NewPassword: "NewPassw0rd!1", ConfirmPassword: "NewPassw0rd!1"}, uuid.New())
	})
}

func TestAuthService_RulesAndTimingPath(t *testing.T) {
	e := newEnv(&faults{})
	svc := e.auth()
	req := dto.LoginRequest{TenantSlug: "acme", Email: "u@x.test", Password: "Passw0rd!123"}

	// unknown tenant / unknown user are indistinguishable
	if _, err := svc.Login(ctx, nil, req, "", "", uuid.New()); !isStatus(err, 401) {
		t.Fatalf("unknown tenant: %v", err)
	}
	e.slugExists = true
	start := time.Now()
	if _, err := svc.Login(ctx, nil, req, "", "", uuid.New()); !isStatus(err, 401) {
		t.Fatalf("unknown user: %v", err)
	}
	if time.Since(start) < 20*time.Millisecond { // a real bcrypt compare (cost 12) must have happened
		t.Fatalf("unknown-user login returned in %v: timing leaks account existence", time.Since(start))
	}
	e.tenantStatus = "suspended"
	if _, err := svc.Login(ctx, nil, req, "", "", uuid.New()); !isStatus(err, 403) {
		t.Fatalf("suspended tenant: %v", err)
	}
	e.tenantStatus, e.emailExists = "active", true
	e.userStatus = "inactive"
	if _, err := svc.Login(ctx, nil, req, "", "", uuid.New()); !isStatus(err, 403) {
		t.Fatalf("inactive user: %v", err)
	}
	e.userStatus = "active"
	res, err := svc.Login(ctx, nil, req, "", "", uuid.New())
	if err != nil || res.AccessToken != "jwt" || res.RefreshToken != "raw" || len(res.User.Roles) != 1 {
		t.Fatalf("login: %+v %v", res, err)
	}

	// refresh: expired token, inactive user, suspended tenant
	expired := true
	e.tokRepo.FindByTokenHashFunc = func(context.Context, *gorm.DB, string) (*models.RefreshToken, error) {
		rt := &models.RefreshToken{UserID: e.userID, TenantID: e.tenantID, ExpiresAt: time.Now().Add(time.Hour)}
		if expired {
			rt.ExpiresAt = time.Now().Add(-time.Hour)
		}
		rt.ID = uuid.New()
		return rt, nil
	}
	if _, err := svc.RefreshToken(ctx, nil, dto.RefreshTokenRequest{RefreshToken: "x"}, uuid.New()); !isStatus(err, 401) {
		t.Fatalf("expired refresh token: %v", err)
	}
	expired = false
	e.userStatus = "inactive"
	if _, err := svc.RefreshToken(ctx, nil, dto.RefreshTokenRequest{RefreshToken: "x"}, uuid.New()); !isStatus(err, 401) {
		t.Fatalf("inactive user refresh: %v", err)
	}
	e.userStatus, e.tenantStatus = "active", "suspended"
	if _, err := svc.RefreshToken(ctx, nil, dto.RefreshTokenRequest{RefreshToken: "x"}, uuid.New()); !isStatus(err, 401) {
		t.Fatalf("suspended tenant refresh: %v", err)
	}

	// forgot: silent for unknown tenant/user
	e.slugExists, e.emailExists = false, false
	if err := svc.ForgotPassword(ctx, nil, dto.ForgotPasswordRequest{TenantSlug: "x", Email: "a@b.co"}, uuid.New()); err != nil {
		t.Fatalf("unknown tenant must be silent: %v", err)
	}
	e.slugExists = true
	if err := svc.ForgotPassword(ctx, nil, dto.ForgotPasswordRequest{TenantSlug: "x", Email: "a@b.co"}, uuid.New()); err != nil {
		t.Fatalf("unknown user must be silent: %v", err)
	}

	// reset: unknown / used / expired token
	e.resetRepo.FindByTokenHashFunc = func(context.Context, *gorm.DB, string) (*models.PasswordResetToken, error) { return nil, nil }
	rr := dto.ResetPasswordRequest{Token: "t", NewPassword: "NewPassw0rd!1", ConfirmPassword: "NewPassw0rd!1"}
	if err := svc.ResetPassword(ctx, nil, rr, uuid.New()); !isStatus(err, 400) {
		t.Fatalf("unknown reset token: %v", err)
	}
	used := time.Now()
	e.resetRepo.FindByTokenHashFunc = func(context.Context, *gorm.DB, string) (*models.PasswordResetToken, error) {
		return &models.PasswordResetToken{ExpiresAt: time.Now().Add(time.Hour), UsedAt: &used}, nil
	}
	if err := svc.ResetPassword(ctx, nil, rr, uuid.New()); !isStatus(err, 400) {
		t.Fatalf("used reset token: %v", err)
	}
	e.resetRepo.FindByTokenHashFunc = func(context.Context, *gorm.DB, string) (*models.PasswordResetToken, error) {
		return &models.PasswordResetToken{ExpiresAt: time.Now().Add(-time.Hour)}, nil
	}
	if err := svc.ResetPassword(ctx, nil, rr, uuid.New()); !isStatus(err, 400) {
		t.Fatalf("expired reset token: %v", err)
	}
	e.resetRepo.FindByTokenHashFunc = func(context.Context, *gorm.DB, string) (*models.PasswordResetToken, error) {
		return &models.PasswordResetToken{ExpiresAt: time.Now().Add(time.Hour), UserID: e.userID, TenantID: e.tenantID}, nil
	}
	e.userRepo.FindByIDFunc = func(context.Context, *gorm.DB, uuid.UUID, uuid.UUID) (*models.User, error) { return nil, nil }
	if err := svc.ResetPassword(ctx, nil, rr, uuid.New()); !isStatus(err, 404) {
		t.Fatalf("reset for vanished user: %v", err)
	}
}

func isStatus(err error, status int) bool {
	var ae *sharedErrors.AppError
	return errors.As(err, &ae) && ae.StatusCode == status
}

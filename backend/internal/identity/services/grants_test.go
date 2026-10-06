package services

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/identity/dto"
	"github.com/jaas/jaas/internal/identity/models"
	"gorm.io/gorm"
)

// holder wires an env where the caller holds exactly the permission e.permID (via role e.roleID).
func callerCtx(e *env, roles ...string) context.Context {
	return WithCaller(context.Background(), Caller{UserID: e.userID, Roles: roles})
}

func TestCallerContext(t *testing.T) {
	if _, ok := CallerFrom(context.Background()); ok {
		t.Fatal("no caller expected on a bare context")
	}
	c, ok := CallerFrom(WithCaller(context.Background(), Caller{UserID: uuid.New(), Roles: []string{"member", "tenant_admin"}}))
	if !ok || !c.IsTenantAdmin() {
		t.Fatalf("caller lost: %+v", c)
	}
	if (Caller{Roles: []string{"member"}}).IsTenantAdmin() {
		t.Fatal("member is not an admin")
	}
}

func TestEnsureCanGrantPermissions(t *testing.T) {
	e := newEnv(&faults{})
	held, other := e.permID, uuid.New()

	// system context and tenant_admin are unrestricted
	if err := ensureCanGrantPermissions(context.Background(), nil, e.tenantID, []uuid.UUID{other}, e.urRepo, e.rpRepo); err != nil {
		t.Fatalf("system context: %v", err)
	}
	if err := ensureCanGrantPermissions(callerCtx(e, "tenant_admin"), nil, e.tenantID, []uuid.UUID{other}, e.urRepo, e.rpRepo); err != nil {
		t.Fatalf("tenant_admin: %v", err)
	}
	// nothing to grant
	if err := ensureCanGrantPermissions(callerCtx(e, "custom"), nil, e.tenantID, nil, e.urRepo, e.rpRepo); err != nil {
		t.Fatalf("empty grant: %v", err)
	}
	// subset of own permissions is fine, anything else is refused
	if err := ensureCanGrantPermissions(callerCtx(e, "custom"), nil, e.tenantID, []uuid.UUID{held}, e.urRepo, e.rpRepo); err != nil {
		t.Fatalf("own permission: %v", err)
	}
	if err := ensureCanGrantPermissions(callerCtx(e, "custom"), nil, e.tenantID, []uuid.UUID{held, other}, e.urRepo, e.rpRepo); !isStatus(err, 403) {
		t.Fatalf("foreign permission must be refused: %v", err)
	}
	// lookup failures surface (fail closed)
	e.urRepo.FindByUserIDFunc = func(context.Context, *gorm.DB, uuid.UUID, uuid.UUID) ([]models.UserRole, error) { return nil, errBoom }
	if err := ensureCanGrantPermissions(callerCtx(e, "custom"), nil, e.tenantID, []uuid.UUID{held}, e.urRepo, e.rpRepo); err == nil {
		t.Fatal("repo failure must not allow the grant")
	}
}

func TestEnsureCanAssignRoles(t *testing.T) {
	e := newEnv(&faults{})
	plain := &models.Role{Name: "plain"}
	plain.ID = uuid.New()
	admin := &models.Role{Name: "tenant_admin", IsSystem: true}
	admin.ID = uuid.New()
	strong := &models.Role{Name: "strong"}
	strong.ID = uuid.New()
	// the caller's own role (e.roleID) carries permission e.permID; "strong" carries a different one
	e.rpRepo.FindByRoleIDFunc = func(_ context.Context, _ *gorm.DB, _, roleID uuid.UUID) ([]models.RolePermission, error) {
		switch roleID {
		case strong.ID:
			return []models.RolePermission{{RoleID: roleID, PermissionID: uuid.New()}}, nil
		case plain.ID:
			return nil, nil
		}
		return []models.RolePermission{{RoleID: roleID, PermissionID: e.permID}}, nil
	}

	member := callerCtx(e, "custom")
	if err := ensureCanAssignRoles(member, nil, e.tenantID, []*models.Role{plain}, e.urRepo, e.rpRepo); err != nil {
		t.Fatalf("permission-less role: %v", err)
	}
	if err := ensureCanAssignRoles(member, nil, e.tenantID, []*models.Role{admin}, e.urRepo, e.rpRepo); !isStatus(err, 403) {
		t.Fatalf("tenant_admin must be refused: %v", err)
	}
	if err := ensureCanAssignRoles(member, nil, e.tenantID, []*models.Role{strong}, e.urRepo, e.rpRepo); !isStatus(err, 403) {
		t.Fatalf("role with foreign permission must be refused: %v", err)
	}
	if err := ensureCanAssignRoles(callerCtx(e, "tenant_admin"), nil, e.tenantID, []*models.Role{admin, strong}, e.urRepo, e.rpRepo); err != nil {
		t.Fatalf("tenant_admin may assign anything: %v", err)
	}
	if err := ensureCanAssignRoles(context.Background(), nil, e.tenantID, []*models.Role{admin}, e.urRepo, e.rpRepo); err != nil {
		t.Fatalf("system context: %v", err)
	}
	if err := ensureCanAssignRoles(member, nil, e.tenantID, nil, e.urRepo, e.rpRepo); err != nil {
		t.Fatalf("no roles: %v", err)
	}
}

func TestServicesEnforceGrantRules(t *testing.T) {
	e := newEnv(&faults{})
	// caller holds role "custom" with permission e.permID. e.roleRepo returns that same role for any id.
	ctx := callerCtx(e, "custom")
	other := uuid.New().String()

	// AssignPermissions: foreign permission refused (parsePermissionIDs finds it, grant check refuses)
	if err := e.roles().AssignPermissions(ctx, nil, e.tenantID, e.roleID, dto.AssignPermissionsRequest{PermissionIDs: []string{other}}, uuid.New()); !isStatus(err, 403) {
		t.Fatalf("assign foreign permission: %v", err)
	}
	if err := e.roles().AssignPermissions(ctx, nil, e.tenantID, e.roleID, dto.AssignPermissionsRequest{PermissionIDs: []string{e.permID.String()}}, uuid.New()); err != nil {
		t.Fatalf("assign own permission: %v", err)
	}
	// CreateRole
	if _, err := e.roles().CreateRole(ctx, nil, e.tenantID, dto.CreateRoleRequest{Name: "n", PermissionIDs: []string{other}}); !isStatus(err, 403) {
		t.Fatalf("create role with foreign permission: %v", err)
	}
	// CreateUser: the stored role in the fixture is non-system "custom" holding e.permID => allowed for its holder
	svc := NewUserService(e.userRepo, e.roleRepo, e.urRepo, e.sessRepo, e.tokRepo, e.pub, e.audit, WithRolePermissions(e.rpRepo))
	if _, err := svc.CreateUser(ctx, nil, e.tenantID, dto.CreateUserRequest{Email: "n@x.test", Password: "Passw0rd!123", FirstName: "A", LastName: "B", RoleIDs: []string{uuid.New().String()}}, uuid.New()); err != nil {
		t.Fatalf("create user with a role the caller holds: %v", err)
	}
	// without the permission repo wired a non-admin caller fails closed
	bare := NewUserService(e.userRepo, e.roleRepo, e.urRepo, e.sessRepo, e.tokRepo, e.pub, e.audit)
	if _, err := bare.CreateUser(ctx, nil, e.tenantID, dto.CreateUserRequest{Email: "n@x.test", Password: "Passw0rd!123", FirstName: "A", LastName: "B", RoleIDs: []string{uuid.New().String()}}, uuid.New()); !isStatus(err, 403) {
		t.Fatalf("must fail closed: %v", err)
	}
	// admins and system calls are unaffected
	if _, err := bare.CreateUser(callerCtx(e, "tenant_admin"), nil, e.tenantID, dto.CreateUserRequest{Email: "n@x.test", Password: "Passw0rd!123", FirstName: "A", LastName: "B", RoleIDs: []string{uuid.New().String()}}, uuid.New()); err != nil {
		t.Fatalf("admin: %v", err)
	}
}

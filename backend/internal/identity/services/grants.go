package services

import (
	"context"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/identity/models"
	"github.com/jaas/jaas/internal/identity/repositories"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"gorm.io/gorm"
)

// Caller identifies the authenticated principal performing a request. Controllers attach it to the request context so
// services can enforce "no privilege escalation": nobody may hand out access they do not hold themselves.
type Caller struct {
	UserID uuid.UUID
	Roles  []string // role names from the verified JWT
}

// IsTenantAdmin reports whether the caller holds the all-powerful tenant_admin system role.
func (c Caller) IsTenantAdmin() bool {
	for _, r := range c.Roles {
		if r == "tenant_admin" {
			return true
		}
	}
	return false
}

type callerKey struct{}

// WithCaller returns a context carrying the authenticated caller.
func WithCaller(ctx context.Context, c Caller) context.Context {
	return context.WithValue(ctx, callerKey{}, c)
}

// CallerFrom extracts the caller; ok is false for system-initiated (internal) operations, which are not restricted.
func CallerFrom(ctx context.Context) (Caller, bool) {
	c, ok := ctx.Value(callerKey{}).(Caller)
	return c, ok
}

func forbiddenGrant(msg string) error {
	return &sharedErrors.AppError{Code: "FORBIDDEN", Message: msg, StatusCode: 403}
}

// callerPermissionIDs resolves the permission ids the caller currently holds through their roles.
func callerPermissionIDs(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, caller Caller, urRepo repositories.UserRoleRepository, rpRepo repositories.RolePermissionRepository) (map[uuid.UUID]bool, error) {
	urs, err := urRepo.FindByUserID(ctx, db, tenantID, caller.UserID)
	if err != nil {
		return nil, err
	}
	roleIDs := make([]uuid.UUID, 0, len(urs))
	for _, ur := range urs {
		roleIDs = append(roleIDs, ur.RoleID)
	}
	rps, err := rpRepo.FindByRoleIDs(ctx, db, tenantID, roleIDs)
	if err != nil {
		return nil, err
	}
	held := make(map[uuid.UUID]bool, len(rps))
	for _, rp := range rps {
		held[rp.PermissionID] = true
	}
	return held, nil
}

// ensureCanGrantPermissions refuses to let a non-admin caller attach permissions they do not hold themselves.
func ensureCanGrantPermissions(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, permIDs []uuid.UUID, urRepo repositories.UserRoleRepository, rpRepo repositories.RolePermissionRepository) error {
	caller, ok := CallerFrom(ctx)
	if !ok || caller.IsTenantAdmin() || len(permIDs) == 0 {
		return nil
	}
	held, err := callerPermissionIDs(ctx, db, tenantID, caller, urRepo, rpRepo)
	if err != nil {
		return err
	}
	for _, id := range permIDs {
		if !held[id] {
			return forbiddenGrant("You cannot grant permissions you do not hold yourself")
		}
	}
	return nil
}

// ensureCanAssignRoles refuses to let a non-admin caller assign tenant_admin, or any role carrying permissions the
// caller does not hold (otherwise users:update would be a path to full privilege).
func ensureCanAssignRoles(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, roles []*models.Role, urRepo repositories.UserRoleRepository, rpRepo repositories.RolePermissionRepository) error {
	caller, ok := CallerFrom(ctx)
	if !ok || caller.IsTenantAdmin() || len(roles) == 0 {
		return nil
	}
	roleIDs := make([]uuid.UUID, 0, len(roles))
	for _, r := range roles {
		if r.IsSystem && r.Name == "tenant_admin" {
			return forbiddenGrant("Only a tenant_admin can assign the tenant_admin role")
		}
		roleIDs = append(roleIDs, r.ID)
	}
	rps, err := rpRepo.FindByRoleIDs(ctx, db, tenantID, roleIDs)
	if err != nil {
		return err
	}
	if len(rps) == 0 {
		return nil
	}
	held, err := callerPermissionIDs(ctx, db, tenantID, caller, urRepo, rpRepo)
	if err != nil {
		return err
	}
	for _, rp := range rps {
		if !held[rp.PermissionID] {
			return forbiddenGrant("You cannot assign a role that carries permissions you do not hold yourself")
		}
	}
	return nil
}

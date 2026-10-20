package services

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/identity/access"
	"github.com/jaas/jaas/internal/identity/models"
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

// restricted reports whether the escalation rules apply: a real caller who is not tenant_admin.
func restricted(ctx context.Context) (Caller, bool) {
	caller, ok := CallerFrom(ctx)
	return caller, ok && !caller.IsTenantAdmin()
}

// ensureCanGrant refuses to let a non-admin caller hand out any grant wider than one they hold themselves
// (docs/ARCHITECTURE.md §17.2).
func ensureCanGrant(ctx context.Context, db *gorm.DB, r AccessResolver, tenantID uuid.UUID, want []access.Grant) error {
	caller, ok := restricted(ctx)
	if !ok || len(want) == 0 {
		return nil
	}
	held, err := r.Effective(ctx, db, tenantID, caller.UserID)
	if err != nil {
		return err
	}
	if missing := held.Missing(want); len(missing) > 0 {
		names := make([]string, len(missing))
		for i, g := range missing {
			names[i] = fmt.Sprintf("%s@%s", g.Permission, g.Scope)
		}
		return forbiddenGrant("You cannot grant access you do not hold yourself: " + strings.Join(names, ", "))
	}
	return nil
}

// ensureCanAssignRoles refuses to let a non-admin caller assign tenant_admin, or any role (with its inherited grants)
// reaching further than the caller's own access.
func ensureCanAssignRoles(ctx context.Context, db *gorm.DB, r AccessResolver, tenantID uuid.UUID, roles []*models.Role) error {
	if _, ok := restricted(ctx); !ok || len(roles) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, 0, len(roles))
	for _, role := range roles {
		if role.IsSystem && role.Name == "tenant_admin" {
			return forbiddenGrant("Only a tenant_admin can assign the tenant_admin role")
		}
		ids = append(ids, role.ID)
	}
	return ensureCanGrantRoles(ctx, db, r, tenantID, ids)
}

// ensureCanGrantRoles checks the effective grants of roleIDs (e.g. a new parent) against the caller.
func ensureCanGrantRoles(ctx context.Context, db *gorm.DB, r AccessResolver, tenantID uuid.UUID, roleIDs []uuid.UUID) error {
	if _, ok := restricted(ctx); !ok || len(roleIDs) == 0 {
		return nil
	}
	eff, err := r.RoleGrants(ctx, db, tenantID, roleIDs)
	if err != nil {
		return err
	}
	want := make([]access.Grant, 0, len(eff))
	for _, e := range eff {
		want = append(want, access.Grant{Permission: e.Permission, Scope: e.Scope})
	}
	return ensureCanGrant(ctx, db, r, tenantID, want)
}

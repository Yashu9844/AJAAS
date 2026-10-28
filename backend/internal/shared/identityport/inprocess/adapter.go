// Package inprocess adapts the in-process M0 identity module to identityport.Port. It is the only HR package that
// imports internal/identity/... packages.
package inprocess

import (
	"context"
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/shared/identityport"
	"github.com/jaas/jaas/internal/identity"
	"github.com/jaas/jaas/internal/identity/access"
	identityDTO "github.com/jaas/jaas/internal/identity/dto"
	identityMiddleware "github.com/jaas/jaas/internal/identity/middleware"
	identityServices "github.com/jaas/jaas/internal/identity/services"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"gorm.io/gorm"
)

type port struct {
	m        *identity.Module
	db       *gorm.DB
	resolver gin.HandlerFunc
	authn    gin.HandlerFunc
}

// New returns the in-process Port over the identity module; db is used for route-level audit writes.
func New(m *identity.Module, db *gorm.DB) identityport.Port {
	return &port{m: m, db: db, resolver: m.TenantResolver(), authn: m.AuthMiddleware()}
}

func (p *port) TenantResolver() gin.HandlerFunc { return p.resolver }
func (p *port) Authenticate() gin.HandlerFunc   { return p.authn }

func (p *port) Require(resource, action string) gin.HandlerFunc {
	return identityMiddleware.RequirePermission(p.m.Authorizer(), resource, action)
}

func (p *port) RequireScoped(resource, action string, min access.Scope) gin.HandlerFunc {
	return identityMiddleware.RequireScopedPermission(p.m.Authorizer(), resource, action, min)
}

func (p *port) Flag(resource, action, key string) gin.HandlerFunc {
	return identityMiddleware.PermissionFlag(p.m.Authorizer(), resource, action, key)
}

func (p *port) AuditTrail(action, resource string) gin.HandlerFunc {
	return identityMiddleware.AuditLog(p.db, p.m.AuditService(), action, resource)
}

func (p *port) Users() identityport.Users   { return users{svc: p.m.UserService()} }
func (p *port) Audit() identityport.Auditor { return p.m.AuditService() }

func (p *port) RegisterPermissions(ctx context.Context, defs []access.PermissionDef) error {
	return p.m.RegisterCatalogue(ctx, defs)
}

func (p *port) OnUserDeactivated(h identityport.DeactivationHook) {
	identityServices.AddUserDeactivationConverger(h)
}

func (p *port) ProvideRoleFacts(f identityport.RoleFacts) { p.m.SetAutomaticRoleSource(f) }

type users struct{ svc identityServices.UserService }

func toUser(tenantID uuid.UUID, u *identityDTO.UserResponse) (*identityport.User, error) {
	id, err := uuid.Parse(u.ID)
	if err != nil {
		return nil, err
	}
	return &identityport.User{ID: id, TenantID: tenantID, Email: u.Email, Status: u.Status}, nil
}

func (u users) GetByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*identityport.User, error) {
	res, err := u.svc.GetByID(ctx, db, tenantID, id)
	if errors.Is(err, sharedErrors.ErrNotFound) || (err == nil && res == nil) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return toUser(tenantID, res)
}

func (u users) DeactivateUser(ctx context.Context, tx *gorm.DB, tenantID, id, correlationID uuid.UUID) error {
	return u.svc.DeactivateUser(ctx, tx, tenantID, id, correlationID)
}

func (u users) ActivateUser(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) error {
	_, err := u.svc.ActivateUser(ctx, tx, tenantID, id)
	return err
}

func (u users) InviteUser(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, in identityport.Invite, correlationID uuid.UUID) (*identityport.User, error) {
	res, err := u.svc.InviteUser(ctx, tx, tenantID, identityDTO.InviteUserRequest{Email: in.Email, FirstName: in.FirstName, LastName: in.LastName}, correlationID)
	if err != nil {
		return nil, err
	}
	return toUser(tenantID, res)
}

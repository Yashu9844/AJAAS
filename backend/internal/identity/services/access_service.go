package services

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/identity/access"
	"github.com/jaas/jaas/internal/identity/dto"
	"github.com/jaas/jaas/internal/identity/models"
	"github.com/jaas/jaas/internal/identity/repositories"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"gorm.io/gorm"
)

// AccessService manages per-user grants and blocks and explains effective access (docs/ARCHITECTURE.md §17).
type AccessService interface {
	ListGrants(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID) ([]dto.UserGrantResponse, error)
	Grant(ctx context.Context, tx *gorm.DB, tenantID, userID uuid.UUID, req dto.UserGrantRequest) (*dto.UserGrantResponse, error)
	RevokeGrant(ctx context.Context, tx *gorm.DB, tenantID, userID, permissionID uuid.UUID) error
	ListBlocks(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID) ([]dto.UserBlockResponse, error)
	Block(ctx context.Context, tx *gorm.DB, tenantID, userID uuid.UUID, req dto.UserBlockRequest) (*dto.UserBlockResponse, error)
	Unblock(ctx context.Context, tx *gorm.DB, tenantID, userID, permissionID uuid.UUID) error
	Explain(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID) (*dto.ExplainAccessResponse, error)
}

// AccessServiceDeps are the access service's collaborators.
type AccessServiceDeps struct {
	Users     repositories.UserRepository
	Perms     repositories.PermissionRepository
	UserRoles repositories.UserRoleRepository
	Access    repositories.AccessRepository
	Resolver  AccessResolver
	Audit     AuditService
	Now       func() time.Time
}

type accessService struct{ d AccessServiceDeps }

// NewAccessService builds an AccessService.
func NewAccessService(d AccessServiceDeps) AccessService {
	if d.Now == nil {
		d.Now = time.Now
	}
	return &accessService{d: d}
}

var errNotAssigned = &sharedErrors.AppError{Code: "NOT_FOUND", Message: "No such grant or block for this user", StatusCode: 404}

func (s *accessService) ensureUser(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID) error {
	u, err := s.d.Users.FindByID(ctx, db, tenantID, userID)
	if err != nil {
		return err
	}
	if u == nil {
		return sharedErrors.ErrNotFound
	}
	return nil
}

func (s *accessService) permission(ctx context.Context, db *gorm.DB, raw string) (*models.Permission, error) {
	id, err := uuid.Parse(raw)
	if err != nil {
		return nil, sharedErrors.ErrValidation
	}
	p, err := s.d.Perms.FindByID(ctx, db, id)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, &sharedErrors.AppError{Code: "NOT_FOUND", Message: "Permission not found", StatusCode: 404}
	}
	return p, nil
}

func callerID(ctx context.Context) *uuid.UUID {
	if c, ok := CallerFrom(ctx); ok {
		id := c.UserID
		return &id
	}
	return nil
}

func (s *accessService) Grant(ctx context.Context, tx *gorm.DB, tenantID, userID uuid.UUID, req dto.UserGrantRequest) (*dto.UserGrantResponse, error) {
	if err := s.ensureUser(ctx, tx, tenantID, userID); err != nil {
		return nil, err
	}
	perm, err := s.permission(ctx, tx, req.PermissionID)
	if err != nil {
		return nil, err
	}
	scope, err := access.ParseScope(req.Scope)
	if err != nil {
		return nil, validationError(err.Error())
	}
	if req.ExpiresAt != nil && !req.ExpiresAt.After(s.d.Now()) {
		return nil, validationError("expires_at must be in the future")
	}
	key := access.Key(perm.Resource, perm.Action)
	if err := ensureCanGrant(ctx, tx, s.d.Resolver, tenantID, []access.Grant{{Permission: key, Scope: scope}}); err != nil {
		return nil, err
	}
	g := &models.UserPermissionGrant{TenantID: tenantID, UserID: userID, PermissionID: perm.ID, Scope: string(scope),
		ExpiresAt: req.ExpiresAt, GrantedBy: callerID(ctx), CreatedAt: s.d.Now(), UpdatedAt: s.d.Now()}
	if req.Reason != "" {
		g.Reason = &req.Reason
	}
	if err := s.d.Access.UpsertUserGrant(ctx, tx, g); err != nil {
		return nil, err
	}
	s.audit(ctx, tx, tenantID, "access.granted", userID, map[string]string{"permission": key, "scope": string(scope)})
	g.Permission = perm
	return s.grantResponse(*g), nil
}

func (s *accessService) audit(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, action string, userID uuid.UUID, detail map[string]string) {
	actor := ""
	if id := callerID(ctx); id != nil {
		actor = id.String()
	}
	_ = s.d.Audit.Log(ctx, tx, tenantID.String(), actor, action, "user", userID.String(), detail, "", "")
}

func (s *accessService) RevokeGrant(ctx context.Context, tx *gorm.DB, tenantID, userID, permissionID uuid.UUID) error {
	if err := s.ensureUser(ctx, tx, tenantID, userID); err != nil {
		return err
	}
	ok, err := s.d.Access.DeleteUserGrant(ctx, tx, tenantID, userID, permissionID)
	if err != nil {
		return err
	}
	if !ok {
		return errNotAssigned
	}
	s.audit(ctx, tx, tenantID, "access.grant_revoked", userID, map[string]string{"permission_id": permissionID.String()})
	return nil
}

func (s *accessService) ListGrants(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID) ([]dto.UserGrantResponse, error) {
	if err := s.ensureUser(ctx, db, tenantID, userID); err != nil {
		return nil, err
	}
	rows, err := s.d.Access.UserGrants(ctx, db, tenantID, userID)
	if err != nil {
		return nil, err
	}
	out := make([]dto.UserGrantResponse, 0, len(rows))
	for _, g := range rows {
		out = append(out, *s.grantResponse(g))
	}
	return out, nil
}

func (s *accessService) grantResponse(g models.UserPermissionGrant) *dto.UserGrantResponse {
	res := &dto.UserGrantResponse{PermissionID: g.PermissionID.String(), Scope: g.Scope, ExpiresAt: g.ExpiresAt, Reason: g.Reason, CreatedAt: g.CreatedAt,
		Expired: g.ExpiresAt != nil && !g.ExpiresAt.After(s.d.Now())}
	if g.Permission != nil {
		res.Permission = access.Key(g.Permission.Resource, g.Permission.Action)
	}
	if g.GrantedBy != nil {
		by := g.GrantedBy.String()
		res.GrantedBy = &by
	}
	return res
}

func (s *accessService) Block(ctx context.Context, tx *gorm.DB, tenantID, userID uuid.UUID, req dto.UserBlockRequest) (*dto.UserBlockResponse, error) {
	if err := s.ensureNotTenantAdmin(ctx, tx, tenantID, userID); err != nil {
		return nil, err
	}
	perm, err := s.permission(ctx, tx, req.PermissionID)
	if err != nil {
		return nil, err
	}
	b := &models.UserPermissionBlock{TenantID: tenantID, UserID: userID, PermissionID: perm.ID, CreatedBy: callerID(ctx), CreatedAt: s.d.Now(), UpdatedAt: s.d.Now()}
	if req.Reason != "" {
		b.Reason = &req.Reason
	}
	if err := s.d.Access.UpsertUserBlock(ctx, tx, b); err != nil {
		return nil, err
	}
	key := access.Key(perm.Resource, perm.Action)
	s.audit(ctx, tx, tenantID, "access.blocked", userID, map[string]string{"permission": key})
	b.Permission = perm
	return blockResponse(*b), nil
}

// ensureNotTenantAdmin rejects blocks on tenant_admin holders: they bypass permission checks, so a block would
// silently do nothing.
func (s *accessService) ensureNotTenantAdmin(ctx context.Context, tx *gorm.DB, tenantID, userID uuid.UUID) error {
	if err := s.ensureUser(ctx, tx, tenantID, userID); err != nil {
		return err
	}
	urs, err := s.d.UserRoles.FindByUserID(ctx, tx, tenantID, userID)
	if err != nil {
		return err
	}
	for _, ur := range urs {
		if ur.Role != nil && ur.Role.IsSystem && ur.Role.Name == "tenant_admin" {
			return &sharedErrors.AppError{Code: "CONFLICT", StatusCode: 409,
				Message: "tenant_admin holds every permission and cannot be blocked; remove the role instead"}
		}
	}
	return nil
}

func (s *accessService) Unblock(ctx context.Context, tx *gorm.DB, tenantID, userID, permissionID uuid.UUID) error {
	if err := s.ensureUser(ctx, tx, tenantID, userID); err != nil {
		return err
	}
	ok, err := s.d.Access.DeleteUserBlock(ctx, tx, tenantID, userID, permissionID)
	if err != nil {
		return err
	}
	if !ok {
		return errNotAssigned
	}
	s.audit(ctx, tx, tenantID, "access.unblocked", userID, map[string]string{"permission_id": permissionID.String()})
	return nil
}

func (s *accessService) ListBlocks(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID) ([]dto.UserBlockResponse, error) {
	if err := s.ensureUser(ctx, db, tenantID, userID); err != nil {
		return nil, err
	}
	rows, err := s.d.Access.UserBlocks(ctx, db, tenantID, userID)
	if err != nil {
		return nil, err
	}
	out := make([]dto.UserBlockResponse, 0, len(rows))
	for _, b := range rows {
		out = append(out, *blockResponse(b))
	}
	return out, nil
}

func blockResponse(b models.UserPermissionBlock) *dto.UserBlockResponse {
	res := &dto.UserBlockResponse{PermissionID: b.PermissionID.String(), Reason: b.Reason, CreatedAt: b.CreatedAt}
	if b.Permission != nil {
		res.Permission = access.Key(b.Permission.Resource, b.Permission.Action)
	}
	if b.CreatedBy != nil {
		by := b.CreatedBy.String()
		res.CreatedBy = &by
	}
	return res
}

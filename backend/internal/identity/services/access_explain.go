package services

import (
	"context"
	"sort"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/identity/access"
	"github.com/jaas/jaas/internal/identity/dto"
	"github.com/jaas/jaas/internal/identity/models"
	"gorm.io/gorm"
)

// catalogueLimit bounds the permission catalogue read for explanations (the catalogue is small and global).
const catalogueLimit = 1000

// Explain lists every permission the user holds or is blocked from, with scope and the role or grant behind it.
func (s *accessService) Explain(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID) (*dto.ExplainAccessResponse, error) {
	if err := s.ensureUser(ctx, db, tenantID, userID); err != nil {
		return nil, err
	}
	urs, err := s.d.UserRoles.FindByUserID(ctx, db, tenantID, userID)
	if err != nil {
		return nil, err
	}
	res := &dto.ExplainAccessResponse{UserID: userID.String(), Roles: []dto.RoleSummary{}, Permissions: []dto.ExplainedPermission{}}
	for _, ur := range urs {
		if ur.Role != nil {
			res.Roles = append(res.Roles, dto.RoleSummary{ID: ur.Role.ID.String(), Name: ur.Role.Name, Source: ur.Source})
			res.IsTenantAdmin = res.IsTenantAdmin || ur.Role.Name == "tenant_admin"
		}
	}
	eff, err := s.d.Resolver.Effective(ctx, db, tenantID, userID)
	if err != nil {
		return nil, err
	}
	meta, err := s.catalogueMeta(ctx, db)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(eff))
	for k := range eff {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		res.Permissions = append(res.Permissions, explained(eff[k], meta[k]))
	}
	return res, nil
}

func (s *accessService) catalogueMeta(ctx context.Context, db *gorm.DB) (map[string]models.Permission, error) {
	perms, _, err := s.d.Perms.FindAll(ctx, db, 1, catalogueLimit)
	if err != nil {
		return nil, err
	}
	out := make(map[string]models.Permission, len(perms))
	for _, p := range perms {
		out[access.Key(p.Resource, p.Action)] = p
	}
	return out, nil
}

func explained(e *access.Entry, p models.Permission) dto.ExplainedPermission {
	out := dto.ExplainedPermission{Permission: e.Permission, Module: p.Module, Sensitive: p.IsSensitive, Scope: string(e.Scope),
		Blocked: e.Blocked, Sources: []dto.AccessSource{}}
	for _, src := range e.Sources {
		out.Sources = append(out.Sources, dto.AccessSource{Kind: string(src.Kind), RoleID: src.RoleID, RoleName: src.RoleName, Scope: string(src.Scope)})
	}
	return out
}

package services

import (
	"context"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/identity/access"
	"github.com/jaas/jaas/internal/identity/dto"
	"github.com/jaas/jaas/internal/identity/models"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"gorm.io/gorm"
)

// grantSpec is a validated permission + scope to store on a role.
type grantSpec struct {
	id    uuid.UUID
	key   string
	scope access.Scope
}

func grantsOf(specs []grantSpec) []access.Grant {
	out := make([]access.Grant, len(specs))
	for i, g := range specs {
		out[i] = access.Grant{Permission: g.key, Scope: g.scope}
	}
	return out
}

func validationError(msg string) error {
	return &sharedErrors.AppError{Code: "VALIDATION_ERROR", Message: msg, StatusCode: 400}
}

// parseGrants merges legacy permission_ids (tenant scope) and scoped grants, rejects conflicting duplicates, and
// verifies every permission exists.
func (s *roleService) parseGrants(ctx context.Context, db *gorm.DB, ids []string, grants []dto.GrantInput) ([]grantSpec, error) {
	scopes := map[uuid.UUID]access.Scope{}
	var order []uuid.UUID
	add := func(rawID, rawScope string) error {
		id, err := uuid.Parse(rawID)
		if err != nil {
			return sharedErrors.ErrValidation
		}
		scope, err := access.ParseScope(rawScope)
		if err != nil {
			return validationError(err.Error())
		}
		if prev, ok := scopes[id]; ok && prev != scope {
			return validationError("a permission appears twice with different scopes")
		} else if !ok {
			order = append(order, id)
		}
		scopes[id] = scope
		return nil
	}
	for _, id := range ids {
		if err := add(id, ""); err != nil {
			return nil, err
		}
	}
	for _, g := range grants {
		if err := add(g.PermissionID, g.Scope); err != nil {
			return nil, err
		}
	}
	return s.lookupGrants(ctx, db, order, scopes)
}

func (s *roleService) lookupGrants(ctx context.Context, db *gorm.DB, order []uuid.UUID, scopes map[uuid.UUID]access.Scope) ([]grantSpec, error) {
	if len(order) == 0 {
		return nil, nil
	}
	found, err := s.permRepo.FindByIDs(ctx, db, order)
	if err != nil {
		return nil, err
	}
	if len(found) != len(order) {
		return nil, &sharedErrors.AppError{Code: "NOT_FOUND", Message: "One or more permission IDs were not found", StatusCode: 404}
	}
	keys := make(map[uuid.UUID]string, len(found))
	for _, p := range found {
		keys[p.ID] = access.Key(p.Resource, p.Action)
	}
	out := make([]grantSpec, len(order))
	for i, id := range order {
		out[i] = grantSpec{id: id, key: keys[id], scope: scopes[id]}
	}
	return out, nil
}

// applyGrants adds new grants and changes the scope of existing ones; it returns the permissions changed.
func (s *roleService) applyGrants(ctx context.Context, tx *gorm.DB, role *models.Role, specs []grantSpec) ([]uuid.UUID, error) {
	if len(specs) == 0 {
		return nil, nil
	}
	existing, err := s.rolePermRepo.FindByRoleID(ctx, tx, role.TenantID, role.ID)
	if err != nil {
		return nil, err
	}
	current := map[uuid.UUID]string{}
	for _, rp := range existing {
		current[rp.PermissionID] = rp.Scope
	}
	var changed []uuid.UUID
	for _, g := range specs {
		scope, held := current[g.id]
		switch {
		case !held:
			err = s.rolePermRepo.Create(ctx, tx, &models.RolePermission{RoleID: role.ID, PermissionID: g.id, TenantID: role.TenantID, Scope: string(g.scope)})
		case scope != string(g.scope) && s.accessRepo != nil:
			err = s.accessRepo.SetRolePermissionScope(ctx, tx, role.TenantID, role.ID, g.id, string(g.scope))
		default:
			continue
		}
		if err != nil {
			return nil, err
		}
		changed = append(changed, g.id)
	}
	return changed, nil
}

// applyParent validates and sets (or with "" clears) the role's parent: same tenant, no cycle, depth limit, and the
// caller must already hold everything the parent grants.
func (s *roleService) applyParent(ctx context.Context, tx *gorm.DB, role *models.Role, raw *string) error {
	if raw == nil {
		return nil
	}
	if *raw == "" {
		role.ParentRoleID = nil
		return nil
	}
	if s.accessRepo == nil {
		return validationError("role inheritance is not available")
	}
	if role.IsSystem {
		return &sharedErrors.AppError{Code: "FORBIDDEN", Message: "System roles cannot inherit", StatusCode: 403}
	}
	parentID, err := uuid.Parse(*raw)
	if err != nil {
		return sharedErrors.ErrValidation
	}
	if _, err := s.findRole(ctx, tx, role.TenantID, parentID); err != nil {
		return err
	}
	if err := s.checkParent(ctx, tx, role, parentID); err != nil {
		return err
	}
	if err := ensureCanGrantRoles(ctx, tx, s.resolver, role.TenantID, []uuid.UUID{parentID}); err != nil {
		return err
	}
	role.ParentRoleID = &parentID
	return nil
}

func (s *roleService) checkParent(ctx context.Context, tx *gorm.DB, role *models.Role, parentID uuid.UUID) error {
	roles, err := s.accessRepo.TenantRoles(ctx, tx, role.TenantID)
	if err != nil {
		return err
	}
	nodes := make([]access.RoleNode, len(roles))
	for i, r := range roles {
		nodes[i] = access.RoleNode{ID: r.ID, ParentID: r.ParentRoleID}
	}
	if err := access.CheckParent(nodes, role.ID, parentID); err != nil {
		return &sharedErrors.AppError{Code: "CONFLICT", Message: err.Error(), StatusCode: 409}
	}
	return nil
}

func (s *roleService) CloneRole(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID, req dto.CloneRoleRequest) (*dto.RoleResponse, error) {
	src, err := s.findRole(ctx, tx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if src.IsSystem {
		return nil, &sharedErrors.AppError{Code: "FORBIDDEN", Message: "System roles cannot be cloned", StatusCode: 403}
	}
	rps, err := s.rolePermRepo.FindByRoleID(ctx, tx, tenantID, id)
	if err != nil {
		return nil, err
	}
	grants := make([]dto.GrantInput, len(rps))
	for i, rp := range rps {
		grants[i] = dto.GrantInput{PermissionID: rp.PermissionID.String(), Scope: rp.Scope}
	}
	create := dto.CreateRoleRequest{Name: req.Name, Grants: grants}
	if src.Description != nil {
		create.Description = *src.Description
	}
	if src.ParentRoleID != nil {
		parent := src.ParentRoleID.String()
		create.ParentRoleID = &parent
	}
	return s.CreateRole(ctx, tx, tenantID, create)
}

// loadPermissions attaches each role's permissions (one batched query).
func (s *roleService) loadPermissions(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, roles ...*models.Role) error {
	if len(roles) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, len(roles))
	for i, r := range roles {
		ids[i] = r.ID
	}
	rps, err := s.rolePermRepo.FindByRoleIDs(ctx, db, tenantID, ids)
	if err != nil {
		return err
	}
	byRole := make(map[uuid.UUID][]models.RolePermission, len(roles))
	for _, rp := range rps {
		byRole[rp.RoleID] = append(byRole[rp.RoleID], rp)
	}
	for _, r := range roles {
		r.Permissions = byRole[r.ID]
	}
	return nil
}

func mapRoleToResponse(r *models.Role) *dto.RoleResponse {
	var perms []dto.PermissionSummary
	for _, rp := range r.Permissions {
		if rp.Permission != nil {
			perms = append(perms, dto.PermissionSummary{ID: rp.Permission.ID.String(), Resource: rp.Permission.Resource, Action: rp.Permission.Action, Scope: rp.Scope})
		}
	}
	res := &dto.RoleResponse{
		ID: r.ID.String(), Name: r.Name, Description: r.Description, IsSystem: r.IsSystem, TemplateKey: r.TemplateKey,
		IsAutomatic: r.IsAutomatic, TemplateManaged: r.TemplateManaged, Permissions: perms, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
	if r.ParentRoleID != nil {
		p := r.ParentRoleID.String()
		res.ParentRoleID = &p
	}
	return res
}

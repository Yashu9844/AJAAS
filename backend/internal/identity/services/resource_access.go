package services

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/identity/access"
	"github.com/jaas/jaas/internal/identity/models"
	"github.com/jaas/jaas/internal/identity/repositories"
	"gorm.io/gorm"
)

// ResourceAccess answers and changes role assignments on resources (docs/modules/m0-identity-access/resource-roles.md).
// It is exactly the surface the identity port exposes to other services.
type ResourceAccess interface {
	// PermissionsOn returns the role and the permission keys the user holds on the resource through it (inheritance included).
	PermissionsOn(ctx context.Context, db *gorm.DB, ref access.ResourceRef, userID uuid.UUID) (access.ResourceRole, []string, error)
	// RolesFor maps each resource id to the role the user holds on it (ref.ID is ignored; resources without a role are absent).
	RolesFor(ctx context.Context, db *gorm.DB, ref access.ResourceRef, userID uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]access.ResourceRole, error)
	// AssignRole gives the user the role on the resource, replacing any earlier one.
	AssignRole(ctx context.Context, tx *gorm.DB, ref access.ResourceRef, a access.RoleAssignment) error
	// RevokeRole removes the user role on the resource; it is not an error when none exists.
	RevokeRole(ctx context.Context, tx *gorm.DB, ref access.ResourceRef, actorID, userID uuid.UUID) error
	// RolesOn lists every role assignment on the resource.
	RolesOn(ctx context.Context, db *gorm.DB, ref access.ResourceRef) ([]access.ResourceRole, error)
	// AvailableRoles lists the roles of the tenant that can be assigned on the resource type.
	AvailableRoles(ctx context.Context, db *gorm.DB, ref access.ResourceRef) ([]access.ResourceRole, error)
	// RoleByTemplate returns the id of a predefined role of the tenant.
	RoleByTemplate(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, templateKey string) (uuid.UUID, error)
	// CountWithTemplate counts assignments on the resource whose role came from the template.
	CountWithTemplate(ctx context.Context, db *gorm.DB, ref access.ResourceRef, templateKey string) (int64, error)
}

// ResourceAccessDeps are the service collaborators.
type ResourceAccessDeps struct {
	Repo     repositories.ResourceRoleRepository
	Roles    repositories.RoleRepository
	Access   repositories.AccessRepository
	Resolver AccessResolver
	Audit    AuditService
}

type resourceAccess struct{ d ResourceAccessDeps }

// NewResourceAccess builds a ResourceAccess.
func NewResourceAccess(d ResourceAccessDeps) ResourceAccess { return &resourceAccess{d: d} }

func checkResourceType(t string) error {
	if t != access.AppliesProject {
		return access.ErrResourceType
	}
	return nil
}

func (s *resourceAccess) PermissionsOn(ctx context.Context, db *gorm.DB, ref access.ResourceRef, userID uuid.UUID) (access.ResourceRole, []string, error) {
	if err := checkResourceType(ref.Type); err != nil {
		return access.ResourceRole{}, nil, err
	}
	a, err := s.d.Repo.Find(ctx, db, ref, userID)
	if err != nil || a == nil {
		return access.ResourceRole{}, nil, err
	}
	eff, err := s.d.Resolver.RoleGrants(ctx, db, ref.TenantID, []uuid.UUID{a.RoleID})
	if err != nil {
		return access.ResourceRole{}, nil, err
	}
	return roleOf(*a), eff.Permissions(), nil
}

func (s *resourceAccess) RolesFor(ctx context.Context, db *gorm.DB, ref access.ResourceRef, userID uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]access.ResourceRole, error) {
	if err := checkResourceType(ref.Type); err != nil {
		return nil, err
	}
	rows, err := s.d.Repo.ListForUser(ctx, db, ref, userID, ids)
	if err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID]access.ResourceRole, len(rows))
	for _, a := range rows {
		out[a.ResourceID] = roleOf(a)
	}
	return out, nil
}

func roleOf(a models.ResourceRoleAssignment) access.ResourceRole {
	rr := access.ResourceRole{UserID: a.UserID, RoleID: a.RoleID, Source: a.Source}
	if a.Role != nil {
		rr.RoleName = a.Role.Name
		if a.Role.TemplateKey != nil {
			rr.TemplateKey = *a.Role.TemplateKey
		}
	}
	return rr
}

func (s *resourceAccess) AssignRole(ctx context.Context, tx *gorm.DB, ref access.ResourceRef, in access.RoleAssignment) error {
	if err := checkResourceType(ref.Type); err != nil {
		return err
	}
	role, err := s.d.Roles.FindByID(ctx, tx, ref.TenantID, in.RoleID)
	if err != nil {
		return err
	}
	if role == nil {
		return access.ErrRoleNotFound
	}
	if role.AppliesTo != ref.Type {
		return access.ErrRoleNotApplicable
	}
	a := &models.ResourceRoleAssignment{TenantID: ref.TenantID, UserID: in.UserID, RoleID: in.RoleID, ResourceType: ref.Type, ResourceID: ref.ID, Source: in.Source}
	if in.ActorID != uuid.Nil {
		a.AssignedBy = &in.ActorID
	}
	if err := s.d.Repo.Assign(ctx, tx, a); err != nil {
		return err
	}
	meta := map[string]string{"user_id": in.UserID.String(), "role_id": in.RoleID.String(), "resource_type": ref.Type}
	return s.audit(ctx, tx, ref, in.ActorID, "resource_role.assigned", meta)
}

func (s *resourceAccess) RevokeRole(ctx context.Context, tx *gorm.DB, ref access.ResourceRef, actorID, userID uuid.UUID) error {
	if err := checkResourceType(ref.Type); err != nil {
		return err
	}
	removed, err := s.d.Repo.Revoke(ctx, tx, ref, userID)
	if err != nil || !removed {
		return err
	}
	return s.audit(ctx, tx, ref, actorID, "resource_role.revoked", map[string]string{"user_id": userID.String(), "resource_type": ref.Type})
}

func (s *resourceAccess) RolesOn(ctx context.Context, db *gorm.DB, ref access.ResourceRef) ([]access.ResourceRole, error) {
	if err := checkResourceType(ref.Type); err != nil {
		return nil, err
	}
	rows, err := s.d.Repo.ListByResource(ctx, db, ref)
	if err != nil {
		return nil, err
	}
	out := make([]access.ResourceRole, 0, len(rows))
	for _, a := range rows {
		out = append(out, roleOf(a))
	}
	return out, nil
}

func (s *resourceAccess) AvailableRoles(ctx context.Context, db *gorm.DB, ref access.ResourceRef) ([]access.ResourceRole, error) {
	if err := checkResourceType(ref.Type); err != nil {
		return nil, err
	}
	roles, err := s.d.Access.TenantRoles(ctx, db, ref.TenantID)
	if err != nil {
		return nil, err
	}
	var out []access.ResourceRole
	for _, r := range roles {
		if r.AppliesTo != ref.Type {
			continue
		}
		rr := access.ResourceRole{RoleID: r.ID, RoleName: r.Name}
		if r.TemplateKey != nil {
			rr.TemplateKey = *r.TemplateKey
		}
		out = append(out, rr)
	}
	return out, nil
}

func (s *resourceAccess) RoleByTemplate(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, templateKey string) (uuid.UUID, error) {
	roles, err := s.d.Access.TenantRoles(ctx, db, tenantID)
	if err != nil {
		return uuid.Nil, err
	}
	for _, r := range roles {
		if r.TemplateKey != nil && *r.TemplateKey == templateKey {
			return r.ID, nil
		}
	}
	return uuid.Nil, fmt.Errorf("predefined role %q is not provisioned for this tenant", templateKey)
}

func (s *resourceAccess) CountWithTemplate(ctx context.Context, db *gorm.DB, ref access.ResourceRef, templateKey string) (int64, error) {
	if err := checkResourceType(ref.Type); err != nil {
		return 0, err
	}
	return s.d.Repo.CountWithTemplate(ctx, db, ref, templateKey)
}

func (s *resourceAccess) audit(ctx context.Context, tx *gorm.DB, ref access.ResourceRef, actorID uuid.UUID, action string, meta any) error {
	actor := ""
	if actorID != uuid.Nil { // a system change (group rule refresh) has no actor
		actor = actorID.String()
	}
	return s.d.Audit.Log(ctx, tx, ref.TenantID.String(), actor, action, "project", ref.ID.String(), meta, "", "")
}

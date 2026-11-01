package services

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/identity/access"
	"github.com/jaas/jaas/internal/identity/models"
	"github.com/jaas/jaas/internal/identity/repositories"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"gorm.io/gorm"
)

// Errors returned for resource-scoped roles.
var (
	ErrResourceType = errors.New("unsupported resource type")
	ErrRoleScope    = errors.New("role does not apply to this resource type")
)

// ResourceRole is a role held on one resource.
type ResourceRole struct {
	UserID      uuid.UUID
	RoleID      uuid.UUID
	RoleName    string
	TemplateKey string
	Source      string
}

// ResourceAccess answers and changes role assignments on resources (docs/modules/m0-identity-access/resource-roles.md).
type ResourceAccess interface {
	// PermissionsOn returns the role and the permission keys the user holds on the resource through it (inheritance included).
	PermissionsOn(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID, resourceType string, resourceID uuid.UUID) (ResourceRole, []string, error)
	// RolesFor maps each resource to the role the user holds on it (resources without a role are absent).
	RolesFor(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID, resourceType string, ids []uuid.UUID) (map[uuid.UUID]ResourceRole, error)
	// AssignRole gives the user the role on the resource, replacing any earlier one.
	AssignRole(ctx context.Context, tx *gorm.DB, tenantID, actorID, userID, roleID uuid.UUID, resourceType string, resourceID uuid.UUID, source string) error
	// RevokeRole removes the user role on the resource; it is not an error when none exists.
	RevokeRole(ctx context.Context, tx *gorm.DB, tenantID, actorID, userID uuid.UUID, resourceType string, resourceID uuid.UUID) error
	// RolesOn lists every role assignment on the resource.
	RolesOn(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, resourceType string, resourceID uuid.UUID) ([]ResourceRole, error)
	// AvailableRoles lists the roles of the tenant that can be assigned on the resource type.
	AvailableRoles(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, resourceType string) ([]ResourceRole, error)
	// RoleByTemplate returns the id of a predefined role of the tenant.
	RoleByTemplate(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, templateKey string) (uuid.UUID, error)
	// CountWithTemplate counts assignments on the resource whose role came from the template.
	CountWithTemplate(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, resourceType string, resourceID uuid.UUID, templateKey string) (int64, error)
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
		return ErrResourceType
	}
	return nil
}

func (s *resourceAccess) PermissionsOn(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID, resourceType string, resourceID uuid.UUID) (ResourceRole, []string, error) {
	if err := checkResourceType(resourceType); err != nil {
		return ResourceRole{}, nil, err
	}
	a, err := s.d.Repo.Find(ctx, db, tenantID, userID, resourceType, resourceID)
	if err != nil || a == nil {
		return ResourceRole{}, nil, err
	}
	eff, err := s.d.Resolver.RoleGrants(ctx, db, tenantID, []uuid.UUID{a.RoleID})
	if err != nil {
		return ResourceRole{}, nil, err
	}
	return roleOf(*a), eff.Permissions(), nil
}

func (s *resourceAccess) RolesFor(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID, resourceType string, ids []uuid.UUID) (map[uuid.UUID]ResourceRole, error) {
	if err := checkResourceType(resourceType); err != nil {
		return nil, err
	}
	rows, err := s.d.Repo.ListForUser(ctx, db, tenantID, userID, resourceType, ids)
	if err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID]ResourceRole, len(rows))
	for _, a := range rows {
		out[a.ResourceID] = roleOf(a)
	}
	return out, nil
}

func roleOf(a models.ResourceRoleAssignment) ResourceRole {
	rr := ResourceRole{UserID: a.UserID, RoleID: a.RoleID, Source: a.Source}
	if a.Role != nil {
		rr.RoleName = a.Role.Name
		if a.Role.TemplateKey != nil {
			rr.TemplateKey = *a.Role.TemplateKey
		}
	}
	return rr
}

func (s *resourceAccess) AssignRole(ctx context.Context, tx *gorm.DB, tenantID, actorID, userID, roleID uuid.UUID, resourceType string, resourceID uuid.UUID, source string) error {
	if err := checkResourceType(resourceType); err != nil {
		return err
	}
	role, err := s.d.Roles.FindByID(ctx, tx, tenantID, roleID)
	if err != nil {
		return err
	}
	if role == nil {
		return &sharedErrors.AppError{Code: "NOT_FOUND", Message: "Role not found in this tenant", StatusCode: 404}
	}
	if role.AppliesTo != resourceType {
		return ErrRoleScope
	}
	a := &models.ResourceRoleAssignment{TenantID: tenantID, UserID: userID, RoleID: roleID, ResourceType: resourceType, ResourceID: resourceID, Source: source}
	if actorID != uuid.Nil {
		a.AssignedBy = &actorID
	}
	if err := s.d.Repo.Assign(ctx, tx, a); err != nil {
		return err
	}
	return s.audit(ctx, tx, tenantID, actorID, "resource_role.assigned", resourceID, map[string]string{"user_id": userID.String(), "role_id": roleID.String(), "resource_type": resourceType})
}

func (s *resourceAccess) RevokeRole(ctx context.Context, tx *gorm.DB, tenantID, actorID, userID uuid.UUID, resourceType string, resourceID uuid.UUID) error {
	if err := checkResourceType(resourceType); err != nil {
		return err
	}
	removed, err := s.d.Repo.Revoke(ctx, tx, tenantID, userID, resourceType, resourceID)
	if err != nil || !removed {
		return err
	}
	return s.audit(ctx, tx, tenantID, actorID, "resource_role.revoked", resourceID, map[string]string{"user_id": userID.String(), "resource_type": resourceType})
}

func (s *resourceAccess) RolesOn(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, resourceType string, resourceID uuid.UUID) ([]ResourceRole, error) {
	if err := checkResourceType(resourceType); err != nil {
		return nil, err
	}
	rows, err := s.d.Repo.ListByResource(ctx, db, tenantID, resourceType, resourceID)
	if err != nil {
		return nil, err
	}
	out := make([]ResourceRole, 0, len(rows))
	for _, a := range rows {
		out = append(out, roleOf(a))
	}
	return out, nil
}

func (s *resourceAccess) AvailableRoles(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, resourceType string) ([]ResourceRole, error) {
	if err := checkResourceType(resourceType); err != nil {
		return nil, err
	}
	roles, err := s.d.Access.TenantRoles(ctx, db, tenantID)
	if err != nil {
		return nil, err
	}
	var out []ResourceRole
	for _, r := range roles {
		if r.AppliesTo != resourceType {
			continue
		}
		rr := ResourceRole{RoleID: r.ID, RoleName: r.Name}
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

func (s *resourceAccess) CountWithTemplate(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, resourceType string, resourceID uuid.UUID, templateKey string) (int64, error) {
	if err := checkResourceType(resourceType); err != nil {
		return 0, err
	}
	return s.d.Repo.CountWithTemplate(ctx, db, tenantID, resourceType, resourceID, templateKey)
}

func (s *resourceAccess) audit(ctx context.Context, tx *gorm.DB, tenantID, actorID uuid.UUID, action string, resourceID uuid.UUID, meta any) error {
	actor := ""
	if actorID != uuid.Nil { // a system change (group rule refresh) has no actor
		actor = actorID.String()
	}
	return s.d.Audit.Log(ctx, tx, tenantID.String(), actor, action, "project", resourceID.String(), meta, "", "")
}

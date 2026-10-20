package services

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/identity/dto"
	"github.com/jaas/jaas/internal/identity/events"
	"github.com/jaas/jaas/internal/identity/models"
	"github.com/jaas/jaas/internal/identity/repositories"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"github.com/jaas/jaas/internal/shared/queue"
	"gorm.io/gorm"
)

// RoleService manages RBAC roles: grants with scopes, inheritance, predefined and automatic roles
// (docs/ARCHITECTURE.md §17).
type RoleService interface {
	CreateRole(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, req dto.CreateRoleRequest) (*dto.RoleResponse, error)
	GetRoleByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*dto.RoleResponse, error)
	ListRoles(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, page, perPage int) (*dto.RoleListResponse, error)
	UpdateRole(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID, req dto.UpdateRoleRequest) (*dto.RoleResponse, error)
	DeleteRole(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) error
	CloneRole(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID, req dto.CloneRoleRequest) (*dto.RoleResponse, error)
	AssignPermissions(ctx context.Context, tx *gorm.DB, tenantID, roleID uuid.UUID, req dto.AssignPermissionsRequest, correlationID uuid.UUID) error
	AssignRolesToUser(ctx context.Context, tx *gorm.DB, tenantID, userID uuid.UUID, req dto.AssignRoleRequest, correlationID uuid.UUID) error
	RemovePermission(ctx context.Context, tx *gorm.DB, tenantID, roleID, permissionID uuid.UUID) error
	RemoveRoleFromUser(ctx context.Context, tx *gorm.DB, tenantID, userID, roleID uuid.UUID) error
	GetMyAccess(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID) (*dto.AccessResponse, error)
}

type roleService struct {
	roleRepo     repositories.RoleRepository
	permRepo     repositories.PermissionRepository
	userRoleRepo repositories.UserRoleRepository
	rolePermRepo repositories.RolePermissionRepository
	userRepo     repositories.UserRepository
	accessRepo   repositories.AccessRepository // nil: no inheritance or scope changes (legacy wiring)
	resolver     AccessResolver
	publisher    queue.EventPublisher
	auditSvc     AuditService
}

// RoleOption customises a RoleService.
type RoleOption func(*roleService)

// WithAccessModel enables inheritance, scope changes and full access resolution for escalation checks.
func WithAccessModel(repo repositories.AccessRepository, resolver AccessResolver) RoleOption {
	return func(s *roleService) { s.accessRepo, s.resolver = repo, resolver }
}

// NewRoleService creates a new RoleService.
func NewRoleService(
	roleRepo repositories.RoleRepository,
	permRepo repositories.PermissionRepository,
	userRoleRepo repositories.UserRoleRepository,
	rolePermRepo repositories.RolePermissionRepository,
	userRepo repositories.UserRepository,
	publisher queue.EventPublisher,
	auditSvc AuditService,
	opts ...RoleOption,
) RoleService {
	s := &roleService{
		roleRepo: roleRepo, permRepo: permRepo, userRoleRepo: userRoleRepo, rolePermRepo: rolePermRepo,
		userRepo: userRepo, publisher: publisher, auditSvc: auditSvc,
	}
	for _, o := range opts {
		o(s)
	}
	if s.resolver == nil {
		s.resolver = NewAccessResolver(AccessResolverDeps{UserRoles: userRoleRepo, RolePerms: rolePermRepo})
	}
	return s
}

func (s *roleService) findRole(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Role, error) {
	role, err := s.roleRepo.FindByID(ctx, db, tenantID, id)
	if err != nil {
		return nil, err
	}
	if role == nil {
		return nil, sharedErrors.ErrNotFound
	}
	return role, nil
}

func (s *roleService) CreateRole(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, req dto.CreateRoleRequest) (*dto.RoleResponse, error) {
	// Validate everything BEFORE writing so a bad request never leaves a partial role behind.
	specs, err := s.parseGrants(ctx, tx, req.PermissionIDs, req.Grants)
	if err != nil {
		return nil, err
	}
	if err := ensureCanGrant(ctx, tx, s.resolver, tenantID, grantsOf(specs)); err != nil {
		return nil, err
	}
	if err := s.ensureNameFree(ctx, tx, tenantID, req.Name, nil); err != nil {
		return nil, err
	}
	role := &models.Role{TenantID: tenantID, Name: req.Name}
	if req.Description != "" {
		role.Description = &req.Description
	}
	role.ID = uuid.New()
	if err := s.applyParent(ctx, tx, role, req.ParentRoleID); err != nil {
		return nil, err
	}
	if err := s.roleRepo.Create(ctx, tx, role); err != nil {
		return nil, err
	}
	if _, err := s.applyGrants(ctx, tx, role, specs); err != nil {
		return nil, err
	}
	return s.respond(ctx, tx, role)
}

// ensureNameFree refuses a name another role uses; self is the role being renamed (nil on create).
func (s *roleService) ensureNameFree(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, name string, self *uuid.UUID) error {
	existing, err := s.roleRepo.FindByName(ctx, db, tenantID, name)
	if err != nil {
		return err
	}
	if existing != nil && (self == nil || existing.ID != *self) {
		return sharedErrors.ErrConflict
	}
	return nil
}

func (s *roleService) respond(ctx context.Context, db *gorm.DB, role *models.Role) (*dto.RoleResponse, error) {
	if err := s.loadPermissions(ctx, db, role.TenantID, role); err != nil {
		return nil, err
	}
	return mapRoleToResponse(role), nil
}

func (s *roleService) GetRoleByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*dto.RoleResponse, error) {
	role, err := s.findRole(ctx, db, tenantID, id)
	if err != nil {
		return nil, err
	}
	return s.respond(ctx, db, role)
}

func (s *roleService) ListRoles(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, page, perPage int) (*dto.RoleListResponse, error) {
	roles, total, err := s.roleRepo.FindAll(ctx, db, tenantID, page, perPage)
	if err != nil {
		return nil, err
	}
	ptrs := make([]*models.Role, len(roles))
	for i := range roles {
		ptrs[i] = &roles[i]
	}
	if err := s.loadPermissions(ctx, db, tenantID, ptrs...); err != nil {
		return nil, err
	}
	data := make([]dto.RoleResponse, len(roles))
	for i := range roles {
		data[i] = *mapRoleToResponse(&roles[i])
	}
	totalPages := int(total / int64(perPage))
	if total%int64(perPage) != 0 {
		totalPages++
	}
	return &dto.RoleListResponse{
		Data: data,
		Meta: dto.PaginationMeta{Page: page, PerPage: perPage, TotalItems: total, TotalPages: totalPages},
	}, nil
}

func (s *roleService) UpdateRole(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID, req dto.UpdateRoleRequest) (*dto.RoleResponse, error) {
	role, err := s.findRole(ctx, tx, tenantID, id)
	if err != nil {
		return nil, err
	}
	// RB-003: System roles cannot be renamed
	if role.IsSystem && req.Name != nil && *req.Name != role.Name {
		return nil, &sharedErrors.AppError{Code: "FORBIDDEN", Message: "System roles cannot be renamed", StatusCode: 403}
	}
	if req.Name != nil {
		if err := s.ensureNameFree(ctx, tx, tenantID, *req.Name, &id); err != nil {
			return nil, err
		}
		role.Name = *req.Name
	}
	if req.Description != nil {
		role.Description = req.Description
	}
	if req.ParentRoleID != nil {
		if err := s.applyParent(ctx, tx, role, req.ParentRoleID); err != nil {
			return nil, err
		}
		role.TemplateManaged = false
	}
	if err := s.roleRepo.Update(ctx, tx, role); err != nil {
		return nil, err
	}
	return s.respond(ctx, tx, role)
}

func (s *roleService) DeleteRole(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) error {
	role, err := s.findRole(ctx, tx, tenantID, id)
	if err != nil {
		return err
	}
	// RB-003: System and predefined roles cannot be deleted (predefined ones would be recreated at the next boot).
	if role.IsSystem || role.TemplateKey != nil {
		return &sharedErrors.AppError{Code: "FORBIDDEN", Message: "System and predefined roles cannot be deleted", StatusCode: 403}
	}
	// A role that is still held by users cannot be deleted (would silently strip their access).
	holders, err := s.userRoleRepo.CountByRoleID(ctx, tx, tenantID, id)
	if err != nil {
		return err
	}
	if holders > 0 {
		return &sharedErrors.AppError{Code: "CONFLICT", Message: fmt.Sprintf("Role is assigned to %d user(s); unassign it first", holders), StatusCode: 409}
	}
	if err := s.ensureNoChildren(ctx, tx, tenantID, id); err != nil {
		return err
	}
	if err := s.rolePermRepo.DeleteByRoleID(ctx, tx, tenantID, id); err != nil {
		return err
	}
	return s.roleRepo.Delete(ctx, tx, tenantID, id)
}

// ensureNoChildren refuses to delete a role other roles inherit from (their access would change silently).
func (s *roleService) ensureNoChildren(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) error {
	if s.accessRepo == nil {
		return nil
	}
	roles, err := s.accessRepo.TenantRoles(ctx, db, tenantID)
	if err != nil {
		return err
	}
	for _, r := range roles {
		if r.ParentRoleID != nil && *r.ParentRoleID == id {
			return &sharedErrors.AppError{Code: "CONFLICT", Message: "Other roles inherit from this role; change their parent first", StatusCode: 409}
		}
	}
	return nil
}

func (s *roleService) AssignPermissions(ctx context.Context, tx *gorm.DB, tenantID, roleID uuid.UUID, req dto.AssignPermissionsRequest, correlationID uuid.UUID) error {
	role, err := s.findRole(ctx, tx, tenantID, roleID)
	if err != nil {
		return err
	}
	if len(req.PermissionIDs) == 0 && len(req.Grants) == 0 {
		return &sharedErrors.AppError{Code: "VALIDATION_ERROR", Message: "permission_ids or grants is required", StatusCode: 400}
	}
	specs, err := s.parseGrants(ctx, tx, req.PermissionIDs, req.Grants)
	if err != nil {
		return err
	}
	if err := ensureCanGrant(ctx, tx, s.resolver, tenantID, grantsOf(specs)); err != nil {
		return err
	}
	changed, err := s.applyGrants(ctx, tx, role, specs)
	if err != nil || len(changed) == 0 {
		return err
	}
	if err := s.markEdited(ctx, tx, role); err != nil {
		return err
	}
	payload := events.PermissionAssignedPayload{RoleID: roleID, TenantID: tenantID, PermissionIDs: changed, AssignedAt: time.Now()}
	evt := events.NewEvent(events.TypePermissionAssigned, events.RoutingKeyPermissionAssigned, tenantID, correlationID, payload)
	_ = s.publisher.Publish(ctx, "jaas.identity.events", events.RoutingKeyPermissionAssigned, evt)
	_ = s.auditSvc.Log(ctx, tx, tenantID.String(), "", "permission.assigned", "role", roleID.String(), payload, "", "")
	return nil
}

// markEdited takes a predefined role out of template management after a tenant edit (never overwritten again).
func (s *roleService) markEdited(ctx context.Context, tx *gorm.DB, role *models.Role) error {
	if !role.TemplateManaged {
		return nil
	}
	role.TemplateManaged = false
	return s.roleRepo.Update(ctx, tx, role)
}

func (s *roleService) RemovePermission(ctx context.Context, tx *gorm.DB, tenantID, roleID, permissionID uuid.UUID) error {
	role, err := s.findRole(ctx, tx, tenantID, roleID)
	if err != nil {
		return err
	}
	if role.IsSystem && role.Name == "tenant_admin" {
		return &sharedErrors.AppError{Code: "FORBIDDEN", Message: "tenant_admin permissions cannot be changed", StatusCode: 403}
	}
	existing, err := s.rolePermRepo.FindByRoleID(ctx, tx, tenantID, roleID)
	if err != nil {
		return err
	}
	assigned := false
	for _, rp := range existing {
		assigned = assigned || rp.PermissionID == permissionID
	}
	if !assigned {
		return &sharedErrors.AppError{Code: "NOT_FOUND", Message: "Permission is not assigned to this role", StatusCode: 404}
	}
	if err := s.rolePermRepo.Delete(ctx, tx, tenantID, roleID, permissionID); err != nil {
		return err
	}
	if err := s.markEdited(ctx, tx, role); err != nil {
		return err
	}
	_ = s.auditSvc.Log(ctx, tx, tenantID.String(), "", "permission.removed", "role", roleID.String(), map[string]string{"permission_id": permissionID.String()}, "", "")
	return nil
}

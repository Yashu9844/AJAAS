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

// RoleService manages RBAC roles and privilege definitions.
type RoleService interface {
	CreateRole(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, req dto.CreateRoleRequest) (*dto.RoleResponse, error)
	GetRoleByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*dto.RoleResponse, error)
	ListRoles(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, page, perPage int) (*dto.RoleListResponse, error)
	UpdateRole(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID, req dto.UpdateRoleRequest) (*dto.RoleResponse, error)
	DeleteRole(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) error
	AssignPermissions(ctx context.Context, tx *gorm.DB, tenantID, roleID uuid.UUID, req dto.AssignPermissionsRequest, correlationID uuid.UUID) error
	AssignRolesToUser(ctx context.Context, tx *gorm.DB, tenantID, userID uuid.UUID, req dto.AssignRoleRequest, correlationID uuid.UUID) error
}

type roleService struct {
	roleRepo       repositories.RoleRepository
	permRepo       repositories.PermissionRepository
	userRoleRepo   repositories.UserRoleRepository
	rolePermRepo   repositories.RolePermissionRepository
	userRepo       repositories.UserRepository
	publisher      queue.EventPublisher
	auditSvc       AuditService
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
) RoleService {
	return &roleService{
		roleRepo:     roleRepo,
		permRepo:     permRepo,
		userRoleRepo: userRoleRepo,
		rolePermRepo: rolePermRepo,
		userRepo:     userRepo,
		publisher:    publisher,
		auditSvc:     auditSvc,
	}
}

func (s *roleService) CreateRole(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, req dto.CreateRoleRequest) (*dto.RoleResponse, error) {
	// Check duplicate name
	existing, err := s.roleRepo.FindByName(ctx, tx, tenantID, req.Name)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, sharedErrors.ErrConflict
	}

	role := &models.Role{
		TenantID: tenantID,
		Name:     req.Name,
		IsSystem: false,
	}
	if req.Description != "" {
		role.Description = &req.Description
	}

	if err := s.roleRepo.Create(ctx, tx, role); err != nil {
		return nil, err
	}

	// Verify and associate permissions if provided
	if len(req.PermissionIDs) > 0 {
		for _, permIDStr := range req.PermissionIDs {
			permID, err := uuid.Parse(permIDStr)
			if err != nil {
				return nil, sharedErrors.ErrValidation
			}
			rp := &models.RolePermission{
				RoleID:       role.ID,
				PermissionID: permID,
				TenantID:     tenantID,
			}
			if err := s.rolePermRepo.Create(ctx, tx, rp); err != nil {
				return nil, err
			}
		}
	}

	return mapRoleToResponse(role), nil
}

func (s *roleService) GetRoleByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*dto.RoleResponse, error) {
	role, err := s.roleRepo.FindByID(ctx, db, tenantID, id)
	if err != nil {
		return nil, err
	}
	if role == nil {
		return nil, sharedErrors.ErrNotFound
	}
	return mapRoleToResponse(role), nil
}

func (s *roleService) ListRoles(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, page, perPage int) (*dto.RoleListResponse, error) {
	roles, total, err := s.roleRepo.FindAll(ctx, db, tenantID, page, perPage)
	if err != nil {
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
		Meta: dto.PaginationMeta{
			Page:       page,
			PerPage:    perPage,
			TotalItems: total,
			TotalPages: totalPages,
		},
	}, nil
}

func (s *roleService) UpdateRole(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID, req dto.UpdateRoleRequest) (*dto.RoleResponse, error) {
	role, err := s.roleRepo.FindByID(ctx, tx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if role == nil {
		return nil, sharedErrors.ErrNotFound
	}

	// RB-003: System roles cannot be renamed
	if role.IsSystem && req.Name != nil && *req.Name != role.Name {
		return nil, &sharedErrors.AppError{
			Code:       "FORBIDDEN",
			Message:    "System roles cannot be renamed",
			StatusCode: 403,
		}
	}

	if req.Name != nil {
		// Check duplicate name
		existing, err := s.roleRepo.FindByName(ctx, tx, tenantID, *req.Name)
		if err != nil {
			return nil, err
		}
		if existing != nil && existing.ID != id {
			return nil, sharedErrors.ErrConflict
		}
		role.Name = *req.Name
	}
	if req.Description != nil {
		role.Description = req.Description
	}

	if err := s.roleRepo.Update(ctx, tx, role); err != nil {
		return nil, err
	}

	return mapRoleToResponse(role), nil
}

func (s *roleService) DeleteRole(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) error {
	role, err := s.roleRepo.FindByID(ctx, tx, tenantID, id)
	if err != nil {
		return err
	}
	if role == nil {
		return sharedErrors.ErrNotFound
	}

	// RB-003: System roles cannot be deleted
	if role.IsSystem {
		return &sharedErrors.AppError{
			Code:       "FORBIDDEN",
			Message:    "System roles cannot be deleted",
			StatusCode: 403,
		}
	}

	// Remove UserRole links first to avoid orphaned mapping
	// GORM cascade or explicit cleanup
	return s.roleRepo.Delete(ctx, tx, tenantID, id)
}

func (s *roleService) AssignPermissions(ctx context.Context, tx *gorm.DB, tenantID, roleID uuid.UUID, req dto.AssignPermissionsRequest, correlationID uuid.UUID) error {
	role, err := s.roleRepo.FindByID(ctx, tx, tenantID, roleID)
	if err != nil {
		return err
	}
	if role == nil {
		return sharedErrors.ErrNotFound
	}

	// Verify all permission IDs exist
	parsedPermIDs := make([]uuid.UUID, len(req.PermissionIDs))
	for i, permIDStr := range req.PermissionIDs {
		id, err := uuid.Parse(permIDStr)
		if err != nil {
			return sharedErrors.ErrValidation
		}
		parsedPermIDs[i] = id
	}

	foundPerms, err := s.permRepo.FindByIDs(ctx, tx, parsedPermIDs)
	if err != nil {
		return err
	}
	if len(foundPerms) != len(parsedPermIDs) {
		return &sharedErrors.AppError{
			Code:       "NOT_FOUND",
			Message:    "One or more permission IDs were not found",
			StatusCode: 404,
		}
	}

	// Fetch existing assignments to verify idempotency
	existing, err := s.rolePermRepo.FindByRoleID(ctx, tx, tenantID, roleID)
	if err != nil {
		return err
	}

	existingMap := make(map[uuid.UUID]bool)
	for _, rp := range existing {
		existingMap[rp.PermissionID] = true
	}

	var assignedIDs []uuid.UUID

	for _, pID := range parsedPermIDs {
		if !existingMap[pID] {
			rp := &models.RolePermission{
				RoleID:       roleID,
				PermissionID: pID,
				TenantID:     tenantID,
			}
			if err := s.rolePermRepo.Create(ctx, tx, rp); err != nil {
				return err
			}
			assignedIDs = append(assignedIDs, pID)
		}
	}

	// Publish Event & Audit Log
	if len(assignedIDs) > 0 {
		eventPayload := events.PermissionAssignedPayload{
			RoleID:        roleID,
			TenantID:      tenantID,
			PermissionIDs: assignedIDs,
			AssignedAt:    time.Now(),
		}
		evt := events.NewEvent(events.TypePermissionAssigned, events.RoutingKeyPermissionAssigned, tenantID, correlationID, eventPayload)
		_ = s.publisher.Publish(ctx, "jaas.identity.events", events.RoutingKeyPermissionAssigned, evt)

		_ = s.auditSvc.Log(ctx, tx, tenantID.String(), "", "permission.assigned", "role", roleID.String(), eventPayload, "", "")
	}

	return nil
}

func (s *roleService) AssignRolesToUser(ctx context.Context, tx *gorm.DB, tenantID, userID uuid.UUID, req dto.AssignRoleRequest, correlationID uuid.UUID) error {
	user, err := s.userRepo.FindByID(ctx, tx, tenantID, userID)
	if err != nil {
		return err
	}
	if user == nil {
		return sharedErrors.ErrNotFound
	}

	// Verify all role IDs exist inside this tenant
	parsedRoleIDs := make([]uuid.UUID, len(req.RoleIDs))
	for i, rIDStr := range req.RoleIDs {
		id, err := uuid.Parse(rIDStr)
		if err != nil {
			return sharedErrors.ErrValidation
		}

		role, err := s.roleRepo.FindByID(ctx, tx, tenantID, id)
		if err != nil {
			return err
		}
		if role == nil {
			return &sharedErrors.AppError{
				Code:       "NOT_FOUND",
				Message:    fmt.Sprintf("Role ID %s not found in this tenant", id.String()),
				StatusCode: 404,
			}
		}
		parsedRoleIDs[i] = id
	}

	// Fetch existing assignments
	existing, err := s.userRoleRepo.FindByUserID(ctx, tx, tenantID, userID)
	if err != nil {
		return err
	}

	existingMap := make(map[uuid.UUID]bool)
	for _, ur := range existing {
		existingMap[ur.RoleID] = true
	}

	var assignedIDs []uuid.UUID

	for _, rID := range parsedRoleIDs {
		// Idempotency: skip already assigned roles
		if !existingMap[rID] {
			ur := &models.UserRole{
				UserID:   userID,
				RoleID:   rID,
				TenantID: tenantID,
			}
			if err := s.userRoleRepo.Create(ctx, tx, ur); err != nil {
				return err
			}
			assignedIDs = append(assignedIDs, rID)
		}
	}

	// Publish Event & Audit Log
	if len(assignedIDs) > 0 {
		eventPayload := events.RoleAssignedPayload{
			UserID:     userID,
			TenantID:   tenantID,
			RoleIDs:    assignedIDs,
			AssignedBy: uuid.Nil, // Woven in controllers/routes context later
			AssignedAt: time.Now(),
		}
		evt := events.NewEvent(events.TypeRoleAssigned, events.RoutingKeyRoleAssigned, tenantID, correlationID, eventPayload)
		_ = s.publisher.Publish(ctx, "jaas.identity.events", events.RoutingKeyRoleAssigned, evt)

		_ = s.auditSvc.Log(ctx, tx, tenantID.String(), "", "role.assigned", "user", userID.String(), eventPayload, "", "")
	}

	return nil
}

func mapRoleToResponse(r *models.Role) *dto.RoleResponse {
	var desc *string
	if r.Description != nil {
		desc = r.Description
	}

	var perms []dto.PermissionSummary
	if len(r.Permissions) > 0 {
		perms = make([]dto.PermissionSummary, len(r.Permissions))
		for i, rp := range r.Permissions {
			if rp.Permission != nil {
				perms[i] = dto.PermissionSummary{
					ID:       rp.Permission.ID.String(),
					Resource: rp.Permission.Resource,
					Action:   rp.Permission.Action,
				}
			}
		}
	}

	return &dto.RoleResponse{
		ID:          r.ID.String(),
		Name:        r.Name,
		Description: desc,
		IsSystem:    r.IsSystem,
		Permissions: perms,
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
	}
}

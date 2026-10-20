package services

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/identity/access"
	"github.com/jaas/jaas/internal/identity/dto"
	"github.com/jaas/jaas/internal/identity/events"
	"github.com/jaas/jaas/internal/identity/models"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"gorm.io/gorm"
)

// errAutomaticRole refuses manual changes to roles held automatically from HR data.
var errAutomaticRole = &sharedErrors.AppError{Code: "ROLE_AUTOMATIC", StatusCode: 409,
	Message: "Automatic roles follow HR data (employee record, direct reports) and cannot be assigned or removed by hand"}

func (s *roleService) rolesForAssignment(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, raw []string) ([]*models.Role, error) {
	roles := make([]*models.Role, 0, len(raw))
	for _, str := range raw {
		id, err := uuid.Parse(str)
		if err != nil {
			return nil, sharedErrors.ErrValidation
		}
		role, err := s.roleRepo.FindByID(ctx, tx, tenantID, id)
		if err != nil {
			return nil, err
		}
		if role == nil {
			return nil, &sharedErrors.AppError{Code: "NOT_FOUND", Message: fmt.Sprintf("Role ID %s not found in this tenant", id), StatusCode: 404}
		}
		if role.IsAutomatic {
			return nil, errAutomaticRole
		}
		roles = append(roles, role)
	}
	return roles, nil
}

func (s *roleService) AssignRolesToUser(ctx context.Context, tx *gorm.DB, tenantID, userID uuid.UUID, req dto.AssignRoleRequest, correlationID uuid.UUID) error {
	user, err := s.userRepo.FindByID(ctx, tx, tenantID, userID)
	if err != nil {
		return err
	}
	if user == nil {
		return sharedErrors.ErrNotFound
	}
	roles, err := s.rolesForAssignment(ctx, tx, tenantID, req.RoleIDs)
	if err != nil {
		return err
	}
	if err := ensureCanAssignRoles(ctx, tx, s.resolver, tenantID, roles); err != nil {
		return err
	}
	existing, err := s.userRoleRepo.FindByUserID(ctx, tx, tenantID, userID)
	if err != nil {
		return err
	}
	held := make(map[uuid.UUID]bool, len(existing))
	for _, ur := range existing {
		held[ur.RoleID] = true
	}
	var assigned []uuid.UUID
	for _, role := range roles {
		if held[role.ID] { // idempotent
			continue
		}
		held[role.ID] = true
		ur := &models.UserRole{UserID: userID, RoleID: role.ID, TenantID: tenantID, Source: string(access.SourceManual)}
		if caller, ok := CallerFrom(ctx); ok {
			ur.AssignedBy = &caller.UserID
		}
		if err := s.userRoleRepo.Create(ctx, tx, ur); err != nil {
			return err
		}
		assigned = append(assigned, role.ID)
	}
	if len(assigned) > 0 {
		payload := events.RoleAssignedPayload{UserID: userID, TenantID: tenantID, RoleIDs: assigned, AssignedAt: time.Now()}
		evt := events.NewEvent(events.TypeRoleAssigned, events.RoutingKeyRoleAssigned, tenantID, correlationID, payload)
		if err := s.sink.Emit(ctx, tx, evt); err != nil {
			return err
		}
		_ = s.auditSvc.Log(ctx, tx, tenantID.String(), "", "role.assigned", "user", userID.String(), payload, "", "")
	}
	return nil
}

func (s *roleService) RemoveRoleFromUser(ctx context.Context, tx *gorm.DB, tenantID, userID, roleID uuid.UUID) error {
	user, err := s.userRepo.FindByID(ctx, tx, tenantID, userID)
	if err != nil {
		return err
	}
	if user == nil {
		return sharedErrors.ErrNotFound
	}
	role, err := s.roleRepo.FindByID(ctx, tx, tenantID, roleID)
	if err != nil {
		return err
	}
	if role == nil {
		return &sharedErrors.AppError{Code: "NOT_FOUND", Message: fmt.Sprintf("Role ID %s not found in this tenant", roleID), StatusCode: 404}
	}
	assignment, err := s.assignment(ctx, tx, tenantID, userID, roleID)
	if err != nil {
		return err
	}
	if role.IsAutomatic || assignment.Source == string(access.SourceAutomatic) {
		return errAutomaticRole
	}
	// Lock-out guard: a tenant must always keep at least one tenant_admin.
	if role.IsSystem && role.Name == "tenant_admin" {
		holders, err := s.userRoleRepo.CountByRoleID(ctx, tx, tenantID, roleID)
		if err != nil {
			return err
		}
		if holders <= 1 {
			return &sharedErrors.AppError{Code: "CONFLICT", Message: "Cannot remove the last tenant_admin of the tenant", StatusCode: 409}
		}
	}
	if err := s.userRoleRepo.Delete(ctx, tx, tenantID, userID, roleID); err != nil {
		return err
	}
	_ = s.auditSvc.Log(ctx, tx, tenantID.String(), "", "role.removed", "user", userID.String(), map[string]string{"role_id": roleID.String()}, "", "")
	return nil
}

func (s *roleService) assignment(ctx context.Context, tx *gorm.DB, tenantID, userID, roleID uuid.UUID) (*models.UserRole, error) {
	existing, err := s.userRoleRepo.FindByUserID(ctx, tx, tenantID, userID)
	if err != nil {
		return nil, err
	}
	for i := range existing {
		if existing[i].RoleID == roleID {
			return &existing[i], nil
		}
	}
	return nil, &sharedErrors.AppError{Code: "NOT_FOUND", Message: "Role is not assigned to this user", StatusCode: 404}
}

func (s *roleService) GetMyAccess(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID) (*dto.AccessResponse, error) {
	urs, err := s.userRoleRepo.FindByUserID(ctx, db, tenantID, userID)
	if err != nil {
		return nil, err
	}
	res := &dto.AccessResponse{UserID: userID.String(), Roles: []dto.RoleSummary{}, Permissions: []string{}, Grants: []dto.ScopedGrant{}}
	for _, ur := range urs {
		if ur.Role != nil {
			res.Roles = append(res.Roles, dto.RoleSummary{ID: ur.Role.ID.String(), Name: ur.Role.Name, Source: ur.Source})
			res.IsTenantAdmin = res.IsTenantAdmin || ur.Role.Name == "tenant_admin"
		}
	}
	eff, err := s.resolver.Effective(ctx, db, tenantID, userID)
	if err != nil {
		return nil, err
	}
	for _, key := range eff.Permissions() {
		res.Permissions = append(res.Permissions, key)
		res.Grants = append(res.Grants, dto.ScopedGrant{Permission: key, Scope: string(eff[key].Scope)})
	}
	return res, nil
}

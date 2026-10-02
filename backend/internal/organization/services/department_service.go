package services

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/organization/dto"
	"github.com/jaas/jaas/internal/organization/events"
	"github.com/jaas/jaas/internal/organization/models"
	"github.com/jaas/jaas/internal/organization/repositories"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"github.com/jaas/jaas/internal/shared/queue"
	"gorm.io/gorm"
)

// DepartmentService handles department lifecycle and hierarchy moves.
type DepartmentService interface {
	CreateDepartment(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, req dto.CreateDepartmentRequest, correlationID uuid.UUID) (*dto.DepartmentResponse, error)
	GetDepartment(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*dto.DepartmentResponse, error)
	ListDepartments(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, page, perPage int) (*dto.DepartmentListResponse, error)
	UpdateDepartment(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID, req dto.UpdateDepartmentRequest) (*dto.DepartmentResponse, error)
	DeactivateDepartment(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID, force bool, reason *string) (*dto.DepartmentResponse, error)
}

type departmentService struct {
	deptRepo    repositories.DepartmentRepository
	teamRepo    repositories.TeamRepository
	mappingRepo repositories.MappingRepository
	publisher   queue.EventPublisher
	audit       auditLogger
}

// NewDepartmentService creates a DepartmentService.
func NewDepartmentService(
	deptRepo repositories.DepartmentRepository,
	teamRepo repositories.TeamRepository,
	mappingRepo repositories.MappingRepository,
	publisher queue.EventPublisher,
	audit auditLogger,
) DepartmentService {
	return &departmentService{deptRepo: deptRepo, teamRepo: teamRepo, mappingRepo: mappingRepo, publisher: publisher, audit: audit}
}

func (s *departmentService) CreateDepartment(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, req dto.CreateDepartmentRequest, correlationID uuid.UUID) (*dto.DepartmentResponse, error) {
	existing, err := s.deptRepo.FindByName(ctx, tx, tenantID, req.Name)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, sharedErrors.ErrConflict
	}

	parentID, err := parseUUID(req.ParentDepartmentID)
	if err != nil {
		return nil, &sharedErrors.AppError{Code: "VALIDATION_ERROR", Message: "invalid parent_department_id", StatusCode: 400}
	}
	if parentID != nil {
		parent, err := s.deptRepo.FindByID(ctx, tx, tenantID, *parentID)
		if err != nil {
			return nil, err
		}
		if parent == nil {
			return nil, sharedErrors.ErrNotFound
		}
		if err := assertDeptAcyclic(ctx, tx, tenantID, s.deptRepo, parentID); err != nil {
			return nil, err
		}
	}

	dept := &models.Department{
		Name:               strings.TrimSpace(req.Name),
		Code:               req.Code,
		Description:        req.Description,
		ParentDepartmentID: parentID,
		Status:             "active",
	}
	dept.TenantID = tenantID
	if err := s.deptRepo.Create(ctx, tx, dept); err != nil {
		return nil, err
	}

	publishOrOutbox(ctx, s.publisher, events.Exchange, events.RoutingKeyDepartmentCreated,
		events.NewEvent(events.TypeDepartmentCreated, events.RoutingKeyDepartmentCreated, tenantID, correlationID,
			events.DepartmentPayload{DepartmentID: dept.ID, Name: dept.Name}))
	_ = s.audit.Log(ctx, tx, tenantID.String(), "", "department.created", "department", dept.ID.String(), map[string]string{"name": dept.Name}, "", "")

	return mapDeptToResponse(ctx, tx, tenantID, s.deptRepo, dept)
}

func (s *departmentService) GetDepartment(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*dto.DepartmentResponse, error) {
	dept, err := s.deptRepo.FindByID(ctx, db, tenantID, id)
	if err != nil {
		return nil, err
	}
	if dept == nil {
		return nil, sharedErrors.ErrNotFound
	}
	return mapDeptToResponse(ctx, db, tenantID, s.deptRepo, dept)
}

func (s *departmentService) ListDepartments(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, page, perPage int) (*dto.DepartmentListResponse, error) {
	depts, total, err := s.deptRepo.FindAll(ctx, db, tenantID, page, perPage)
	if err != nil {
		return nil, err
	}
	data := make([]dto.DepartmentResponse, len(depts))
	for i := range depts {
		res, err := mapDeptToResponse(ctx, db, tenantID, s.deptRepo, &depts[i])
		if err != nil {
			return nil, err
		}
		data[i] = *res
	}
	return &dto.DepartmentListResponse{Data: data, Meta: dto.NewPaginationMeta(page, perPage, total)}, nil
}

func (s *departmentService) UpdateDepartment(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID, req dto.UpdateDepartmentRequest) (*dto.DepartmentResponse, error) {
	dept, err := s.deptRepo.FindByID(ctx, tx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if dept == nil {
		return nil, sharedErrors.ErrNotFound
	}

	if req.Name != nil && strings.TrimSpace(*req.Name) != dept.Name {
		existing, err := s.deptRepo.FindByName(ctx, tx, tenantID, *req.Name)
		if err != nil {
			return nil, err
		}
		if existing != nil && existing.ID != dept.ID {
			return nil, sharedErrors.ErrConflict
		}
		dept.Name = strings.TrimSpace(*req.Name)
	}
	if req.Description != nil {
		dept.Description = req.Description
	}
	if req.ParentDepartmentID != nil {
		parentID, err := parseUUID(req.ParentDepartmentID)
		if err != nil {
			return nil, &sharedErrors.AppError{Code: "VALIDATION_ERROR", Message: "invalid parent_department_id", StatusCode: 400}
		}
		if parentID != nil && *parentID == dept.ID {
			return nil, &sharedErrors.AppError{Code: "VALIDATION_ERROR", Message: "department cannot be its own parent", StatusCode: 400}
		}
		if parentID != nil {
			parent, err := s.deptRepo.FindByID(ctx, tx, tenantID, *parentID)
			if err != nil {
				return nil, err
			}
			if parent == nil {
				return nil, sharedErrors.ErrNotFound
			}
			// Cycle check must include the moving node itself: walk from the new
			// parent and reject if the walk reaches this department.
			if err := s.assertMoveAcyclic(ctx, tx, tenantID, dept.ID, parentID); err != nil {
				return nil, err
			}
		}
		dept.ParentDepartmentID = parentID
	}

	if err := s.deptRepo.Update(ctx, tx, dept); err != nil {
		return nil, err
	}
	_ = s.audit.Log(ctx, tx, tenantID.String(), "", "department.updated", "department", dept.ID.String(), map[string]string{"name": dept.Name}, "", "")
	return mapDeptToResponse(ctx, tx, tenantID, s.deptRepo, dept)
}

// assertMoveAcyclic rejects moves that would cycle through the moving node.
func (s *departmentService) assertMoveAcyclic(ctx context.Context, db *gorm.DB, tenantID, movingID uuid.UUID, newParent *uuid.UUID) error {
	if err := assertDeptAcyclic(ctx, db, tenantID, s.deptRepo, newParent); err != nil {
		return err
	}
	cur := newParent
	for cur != nil {
		if *cur == movingID {
			return &sharedErrors.AppError{Code: "CONFLICT", Message: "hierarchy.cycle: move would create a cycle", StatusCode: 409}
		}
		parent, err := s.deptRepo.FindByID(ctx, db, tenantID, *cur)
		if err != nil || parent == nil {
			return err
		}
		cur = parent.ParentDepartmentID
	}
	return nil
}

func (s *departmentService) DeactivateDepartment(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID, force bool, reason *string) (*dto.DepartmentResponse, error) {
	dept, err := s.deptRepo.FindByID(ctx, tx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if dept == nil {
		return nil, sharedErrors.ErrNotFound
	}
	if dept.Status == "inactive" {
		return nil, &sharedErrors.AppError{Code: "CONFLICT", Message: "department is already inactive", StatusCode: 409}
	}

	teamCount, err := s.teamRepo.CountByDepartment(ctx, tx, tenantID, id)
	if err != nil {
		return nil, err
	}
	mapCount, err := s.mappingRepo.CountActiveByDepartment(ctx, tx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if (teamCount > 0 || mapCount > 0) && !force {
		return nil, &sharedErrors.AppError{Code: "CONFLICT", Message: "department has active teams or mappings; reassign first or force", StatusCode: 409}
	}
	if force {
		children, err := s.deptRepo.FindChildren(ctx, tx, tenantID, id)
		if err != nil {
			return nil, err
		}
		for i := range children {
			if children[i].Status == "active" {
				children[i].Status = "inactive"
				if err := s.deptRepo.Update(ctx, tx, &children[i]); err != nil {
					return nil, err
				}
			}
		}
	}

	dept.Status = "inactive"
	if err := s.deptRepo.Update(ctx, tx, dept); err != nil {
		return nil, err
	}
	_ = s.audit.Log(ctx, tx, tenantID.String(), "", "department.deactivated", "department", dept.ID.String(), map[string]interface{}{"force": force, "reason": reason}, "", "")
	return mapDeptToResponse(ctx, tx, tenantID, s.deptRepo, dept)
}

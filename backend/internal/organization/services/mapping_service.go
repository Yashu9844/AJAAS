package services

import (
	"context"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/organization/dto"
	"github.com/jaas/jaas/internal/organization/events"
	"github.com/jaas/jaas/internal/organization/models"
	"github.com/jaas/jaas/internal/organization/repositories"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"github.com/jaas/jaas/internal/shared/queue"
	"gorm.io/gorm"
)

// MappingService handles user→org mappings with hierarchy guards.
type MappingService interface {
	CreateMapping(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, req dto.CreateMappingRequest, correlationID uuid.UUID) (*dto.MappingResponse, error)
	GetMapping(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*dto.MappingResponse, error)
	ListUserMappings(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID, page, perPage int) (*dto.MappingListResponse, error)
	UpdateMapping(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID, req dto.UpdateMappingRequest) (*dto.MappingResponse, error)
	DeactivateMapping(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID, req dto.DeactivateMappingRequest) (*dto.MappingResponse, error)
	DeactivateUserMappings(ctx context.Context, tx *gorm.DB, tenantID, userID uuid.UUID, correlationID uuid.UUID) error
}

type mappingService struct {
	mappingRepo repositories.MappingRepository
	deptRepo    repositories.DepartmentRepository
	teamRepo    repositories.TeamRepository
	desigRepo   repositories.DesignationRepository
	users       userChecker
	publisher   queue.EventPublisher
	audit       auditLogger
}

// NewMappingService creates a MappingService.
func NewMappingService(
	mappingRepo repositories.MappingRepository,
	deptRepo repositories.DepartmentRepository,
	teamRepo repositories.TeamRepository,
	desigRepo repositories.DesignationRepository,
	users userChecker,
	publisher queue.EventPublisher,
	audit auditLogger,
) MappingService {
	return &mappingService{mappingRepo: mappingRepo, deptRepo: deptRepo, teamRepo: teamRepo, desigRepo: desigRepo, users: users, publisher: publisher, audit: audit}
}

func mapMapping(m *models.Mapping) *dto.MappingResponse {
	return &dto.MappingResponse{
		ID: m.ID.String(), TenantID: m.TenantID.String(), UserID: m.UserID.String(),
		DepartmentID: uuidToString(m.DepartmentID), TeamID: uuidToString(m.TeamID),
		DesignationID: uuidToString(m.DesignationID), IsPrimary: m.IsPrimary,
		ManagerUserID: uuidToString(m.ManagerUserID), Status: m.Status,
		CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}
}

// checkUser resolves a user through Module 0: must exist, same tenant, active. FR-M001.
func (s *mappingService) checkUser(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID) error {
	u, err := s.users.GetByID(ctx, db, tenantID, userID)
	if err != nil {
		return err
	}
	if u == nil || u.Status != "active" {
		return &sharedErrors.AppError{Code: "NOT_FOUND", Message: "user not found or inactive", StatusCode: 404}
	}
	return nil
}

// checkRefs validates department/team/designation/manager references. FR-M001/FR-M003/FR-M004.
func (s *mappingService) checkRefs(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, deptID, teamID, desigID, managerID *uuid.UUID, self uuid.UUID) error {
	if deptID != nil {
		dept, err := s.deptRepo.FindByID(ctx, db, tenantID, *deptID)
		if err != nil {
			return err
		}
		if dept == nil {
			return sharedErrors.ErrNotFound
		}
	}
	var team *models.Team
	if teamID != nil {
		t, err := s.teamRepo.FindByID(ctx, db, tenantID, *teamID)
		if err != nil {
			return err
		}
		if t == nil {
			return sharedErrors.ErrNotFound
		}
		team = t
	}
	if desigID != nil {
		d, err := s.desigRepo.FindByID(ctx, db, tenantID, *desigID)
		if err != nil {
			return err
		}
		if d == nil {
			return sharedErrors.ErrNotFound
		}
	}
	if deptID != nil && team != nil && team.DepartmentID != *deptID {
		return &sharedErrors.AppError{Code: "VALIDATION_ERROR", Message: "team does not belong to the given department", StatusCode: 400}
	}
	if managerID != nil {
		if *managerID == self {
			return &sharedErrors.AppError{Code: "VALIDATION_ERROR", Message: "user cannot manage themselves", StatusCode: 400}
		}
		if err := s.checkUser(ctx, db, tenantID, *managerID); err != nil {
			return err
		}
		if err := s.assertManagerAcyclic(ctx, db, tenantID, self, *managerID); err != nil {
			return err
		}
	}
	return nil
}

// assertManagerAcyclic walks the manager chain to reject cycles. FR-M003.
func (s *mappingService) assertManagerAcyclic(ctx context.Context, db *gorm.DB, tenantID, self, manager uuid.UUID) error {
	seen := map[uuid.UUID]bool{self: true}
	cur := manager
	for i := 0; i < 50; i++ {
		if seen[cur] {
			return &sharedErrors.AppError{Code: "CONFLICT", Message: "hierarchy.cycle: manager chain contains a cycle", StatusCode: 409}
		}
		seen[cur] = true
		primary, err := s.mappingRepo.FindPrimaryByUser(ctx, db, tenantID, cur)
		if err != nil || primary == nil || primary.ManagerUserID == nil {
			return err
		}
		cur = *primary.ManagerUserID
	}
	return &sharedErrors.AppError{Code: "CONFLICT", Message: "hierarchy.cycle: manager chain too deep or cyclic", StatusCode: 409}
}

func (s *mappingService) CreateMapping(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, req dto.CreateMappingRequest, correlationID uuid.UUID) (*dto.MappingResponse, error) {
	userID, err := uuid.Parse(req.UserID)
	if err != nil {
		return nil, &sharedErrors.AppError{Code: "VALIDATION_ERROR", Message: "invalid user_id", StatusCode: 400}
	}
	if err := s.checkUser(ctx, tx, tenantID, userID); err != nil {
		return nil, err
	}
	deptID, err := parseUUID(req.DepartmentID)
	if err != nil {
		return nil, &sharedErrors.AppError{Code: "VALIDATION_ERROR", Message: "invalid department_id", StatusCode: 400}
	}
	teamID, err := parseUUID(req.TeamID)
	if err != nil {
		return nil, &sharedErrors.AppError{Code: "VALIDATION_ERROR", Message: "invalid team_id", StatusCode: 400}
	}
	desigID, err := parseUUID(req.DesignationID)
	if err != nil {
		return nil, &sharedErrors.AppError{Code: "VALIDATION_ERROR", Message: "invalid designation_id", StatusCode: 400}
	}
	managerID, err := parseUUID(req.ManagerUserID)
	if err != nil {
		return nil, &sharedErrors.AppError{Code: "VALIDATION_ERROR", Message: "invalid manager_user_id", StatusCode: 400}
	}
	if teamID == nil && deptID == nil && desigID == nil {
		return nil, &sharedErrors.AppError{Code: "VALIDATION_ERROR", Message: "at least one of department, team, or designation is required", StatusCode: 400}
	}
	if err := s.checkRefs(ctx, tx, tenantID, deptID, teamID, desigID, managerID, userID); err != nil {
		return nil, err
	}
	if req.IsPrimary {
		existing, err := s.mappingRepo.FindPrimaryByUser(ctx, tx, tenantID, userID)
		if err != nil {
			return nil, err
		}
		if existing != nil {
			return nil, &sharedErrors.AppError{Code: "CONFLICT", Message: "user already has a primary mapping; use update to swap", StatusCode: 409}
		}
	}

	m := &models.Mapping{
		UserID: userID, DepartmentID: deptID, TeamID: teamID, DesignationID: desigID,
		IsPrimary: req.IsPrimary, ManagerUserID: managerID, Status: "active",
	}
	m.TenantID = tenantID
	if err := s.mappingRepo.Create(ctx, tx, m); err != nil {
		return nil, err
	}

	publishOrOutbox(ctx, s.publisher, events.Exchange, events.RoutingKeyMappingCreated,
		events.NewEvent(events.TypeMappingCreated, events.RoutingKeyMappingCreated, tenantID, correlationID,
			events.MappingPayload{MappingID: m.ID, UserID: userID}))
	_ = s.audit.Log(ctx, tx, tenantID.String(), "", "mapping.created", "mapping", m.ID.String(), map[string]string{"user_id": userID.String()}, "", "")
	return mapMapping(m), nil
}

func (s *mappingService) GetMapping(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*dto.MappingResponse, error) {
	m, err := s.mappingRepo.FindByID(ctx, db, tenantID, id)
	if err != nil {
		return nil, err
	}
	if m == nil {
		return nil, sharedErrors.ErrNotFound
	}
	return mapMapping(m), nil
}

func (s *mappingService) ListUserMappings(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID, page, perPage int) (*dto.MappingListResponse, error) {
	out, total, err := s.mappingRepo.FindByUser(ctx, db, tenantID, userID, page, perPage)
	if err != nil {
		return nil, err
	}
	data := make([]dto.MappingResponse, len(out))
	for i := range out {
		data[i] = *mapMapping(&out[i])
	}
	return &dto.MappingListResponse{Data: data, Meta: dto.NewPaginationMeta(page, perPage, total)}, nil
}

func (s *mappingService) UpdateMapping(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID, req dto.UpdateMappingRequest) (*dto.MappingResponse, error) {
	m, err := s.mappingRepo.FindByID(ctx, tx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if m == nil {
		return nil, sharedErrors.ErrNotFound
	}

	deptID := m.DepartmentID
	teamID := m.TeamID
	desigID := m.DesignationID
	managerID := m.ManagerUserID
	if req.DepartmentID != nil {
		deptID, err = parseUUID(req.DepartmentID)
		if err != nil {
			return nil, &sharedErrors.AppError{Code: "VALIDATION_ERROR", Message: "invalid department_id", StatusCode: 400}
		}
	}
	if req.TeamID != nil {
		teamID, err = parseUUID(req.TeamID)
		if err != nil {
			return nil, &sharedErrors.AppError{Code: "VALIDATION_ERROR", Message: "invalid team_id", StatusCode: 400}
		}
	}
	if req.DesignationID != nil {
		desigID, err = parseUUID(req.DesignationID)
		if err != nil {
			return nil, &sharedErrors.AppError{Code: "VALIDATION_ERROR", Message: "invalid designation_id", StatusCode: 400}
		}
	}
	if req.ManagerUserID != nil {
		managerID, err = parseUUID(req.ManagerUserID)
		if err != nil {
			return nil, &sharedErrors.AppError{Code: "VALIDATION_ERROR", Message: "invalid manager_user_id", StatusCode: 400}
		}
	}
	if err := s.checkRefs(ctx, tx, tenantID, deptID, teamID, desigID, managerID, m.UserID); err != nil {
		return nil, err
	}
	if req.IsPrimary != nil && *req.IsPrimary && !m.IsPrimary {
		existing, err := s.mappingRepo.FindPrimaryByUser(ctx, tx, tenantID, m.UserID)
		if err != nil {
			return nil, err
		}
		if existing != nil && existing.ID != m.ID {
			existing.IsPrimary = false
			if err := s.mappingRepo.Update(ctx, tx, existing); err != nil {
				return nil, err
			}
		}
		m.IsPrimary = true
	} else if req.IsPrimary != nil {
		m.IsPrimary = *req.IsPrimary
	}
	m.DepartmentID, m.TeamID, m.DesignationID, m.ManagerUserID = deptID, teamID, desigID, managerID

	if err := s.mappingRepo.Update(ctx, tx, m); err != nil {
		return nil, err
	}
	_ = s.audit.Log(ctx, tx, tenantID.String(), "", "mapping.updated", "mapping", m.ID.String(), map[string]string{"user_id": m.UserID.String()}, "", "")
	return mapMapping(m), nil
}

func (s *mappingService) DeactivateMapping(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID, req dto.DeactivateMappingRequest) (*dto.MappingResponse, error) {
	m, err := s.mappingRepo.FindByID(ctx, tx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if m == nil {
		return nil, sharedErrors.ErrNotFound
	}
	if m.Status == "inactive" {
		return nil, &sharedErrors.AppError{Code: "CONFLICT", Message: "mapping is already inactive", StatusCode: 409}
	}
	m.Status = "inactive"
	m.Reason = req.Reason
	if err := s.mappingRepo.Update(ctx, tx, m); err != nil {
		return nil, err
	}
	_ = s.audit.Log(ctx, tx, tenantID.String(), "", "mapping.deactivated", "mapping", m.ID.String(), map[string]interface{}{"reason": req.Reason}, "", "")
	return mapMapping(m), nil
}

// DeactivateUserMappings converges org state on identity UserDeactivated. FR-M005.
func (s *mappingService) DeactivateUserMappings(ctx context.Context, tx *gorm.DB, tenantID, userID uuid.UUID, correlationID uuid.UUID) error {
	out, _, err := s.mappingRepo.FindByUser(ctx, tx, tenantID, userID, 1, 1000)
	if err != nil {
		return err
	}
	for i := range out {
		if out[i].Status == "active" {
			out[i].Status = "inactive"
			if err := s.mappingRepo.Update(ctx, tx, &out[i]); err != nil {
				return err
			}
		}
	}
	reports, err := s.mappingRepo.FindReports(ctx, tx, tenantID, userID)
	if err != nil {
		return err
	}
	for i := range reports {
		reports[i].ManagerUserID = nil
		if err := s.mappingRepo.Update(ctx, tx, &reports[i]); err != nil {
			return err
		}
	}
	_ = s.audit.Log(ctx, tx, tenantID.String(), "", "mapping.user_deactivated", "user", userID.String(), map[string]int{"deactivated": len(out)}, "", "")
	return nil
}

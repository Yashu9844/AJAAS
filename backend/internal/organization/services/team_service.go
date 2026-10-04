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

// TeamService handles team lifecycle and moves.
type TeamService interface {
	CreateTeam(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, req dto.CreateTeamRequest, correlationID uuid.UUID) (*dto.TeamResponse, error)
	GetTeam(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*dto.TeamResponse, error)
	ListTeams(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, departmentID *uuid.UUID, page, perPage int) (*dto.TeamListResponse, error)
	UpdateTeam(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID, req dto.UpdateTeamRequest) (*dto.TeamResponse, error)
	DeactivateTeam(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID, reason *string) (*dto.TeamResponse, error)
}

type teamService struct {
	teamRepo    repositories.TeamRepository
	deptRepo    repositories.DepartmentRepository
	mappingRepo repositories.MappingRepository
	users       userChecker
	publisher   queue.EventPublisher
	audit       auditLogger
}

// NewTeamService creates a TeamService.
func NewTeamService(
	teamRepo repositories.TeamRepository,
	deptRepo repositories.DepartmentRepository,
	mappingRepo repositories.MappingRepository,
	users userChecker,
	publisher queue.EventPublisher,
	audit auditLogger,
) TeamService {
	return &teamService{teamRepo: teamRepo, deptRepo: deptRepo, mappingRepo: mappingRepo, users: users, publisher: publisher, audit: audit}
}

func (s *teamService) checkLead(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, leadID *string) (*uuid.UUID, error) {
	if leadID == nil || *leadID == "" {
		return nil, nil
	}
	id, err := uuid.Parse(*leadID)
	if err != nil {
		return nil, &sharedErrors.AppError{Code: "VALIDATION_ERROR", Message: "invalid lead_user_id", StatusCode: 400}
	}
	u, err := s.users.GetByID(ctx, db, tenantID, id)
	if err != nil {
		return nil, err
	}
	if u == nil || u.Status != "active" {
		return nil, &sharedErrors.AppError{Code: "NOT_FOUND", Message: "lead user not found or inactive", StatusCode: 404}
	}
	return &id, nil
}

func (s *teamService) toResponse(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, t *models.Team) (*dto.TeamResponse, error) {
	count, err := s.mappingRepo.CountActiveByTeam(ctx, db, tenantID, t.ID)
	if err != nil {
		return nil, err
	}
	return &dto.TeamResponse{
		ID: t.ID.String(), TenantID: t.TenantID.String(), DepartmentID: t.DepartmentID.String(),
		Name: t.Name, Code: t.Code, Description: t.Description,
		LeadUserID: uuidToString(t.LeadUserID), Status: t.Status,
		MemberCount: count, CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt,
	}, nil
}

func (s *teamService) CreateTeam(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, req dto.CreateTeamRequest, correlationID uuid.UUID) (*dto.TeamResponse, error) {
	deptID, err := uuid.Parse(req.DepartmentID)
	if err != nil {
		return nil, &sharedErrors.AppError{Code: "VALIDATION_ERROR", Message: "invalid department_id", StatusCode: 400}
	}
	dept, err := s.deptRepo.FindByID(ctx, tx, tenantID, deptID)
	if err != nil {
		return nil, err
	}
	if dept == nil {
		return nil, sharedErrors.ErrNotFound
	}
	existing, err := s.teamRepo.FindByName(ctx, tx, tenantID, deptID, strings.TrimSpace(req.Name))
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, sharedErrors.ErrConflict
	}
	leadID, err := s.checkLead(ctx, tx, tenantID, req.LeadUserID)
	if err != nil {
		return nil, err
	}

	team := &models.Team{
		DepartmentID: deptID, Name: strings.TrimSpace(req.Name),
		Code: req.Code, Description: req.Description, LeadUserID: leadID, Status: "active",
	}
	team.TenantID = tenantID
	if err := s.teamRepo.Create(ctx, tx, team); err != nil {
		return nil, err
	}

	publishOrOutbox(ctx, tx, s.publisher, events.Exchange,
		events.NewEvent(events.TypeTeamCreated, events.RoutingKeyTeamCreated, tenantID, correlationID,
			events.TeamPayload{TeamID: team.ID, DepartmentID: deptID, Name: team.Name}))
	_ = s.audit.Log(ctx, tx, tenantID.String(), "", "team.created", "team", team.ID.String(), map[string]string{"name": team.Name}, "", "")
	return s.toResponse(ctx, tx, tenantID, team)
}

func (s *teamService) GetTeam(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*dto.TeamResponse, error) {
	team, err := s.teamRepo.FindByID(ctx, db, tenantID, id)
	if err != nil {
		return nil, err
	}
	if team == nil {
		return nil, sharedErrors.ErrNotFound
	}
	return s.toResponse(ctx, db, tenantID, team)
}

func (s *teamService) ListTeams(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, departmentID *uuid.UUID, page, perPage int) (*dto.TeamListResponse, error) {
	teams, total, err := s.teamRepo.FindAll(ctx, db, tenantID, departmentID, page, perPage)
	if err != nil {
		return nil, err
	}
	data := make([]dto.TeamResponse, len(teams))
	for i := range teams {
		res, err := s.toResponse(ctx, db, tenantID, &teams[i])
		if err != nil {
			return nil, err
		}
		data[i] = *res
	}
	return &dto.TeamListResponse{Data: data, Meta: dto.NewPaginationMeta(page, perPage, total)}, nil
}

func (s *teamService) UpdateTeam(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID, req dto.UpdateTeamRequest) (*dto.TeamResponse, error) {
	team, err := s.teamRepo.FindByID(ctx, tx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if team == nil {
		return nil, sharedErrors.ErrNotFound
	}

	oldDept := team.DepartmentID
	if req.Description != nil {
		team.Description = req.Description
	}
	if req.DepartmentID != nil {
		deptID, err := uuid.Parse(*req.DepartmentID)
		if err != nil {
			return nil, &sharedErrors.AppError{Code: "VALIDATION_ERROR", Message: "invalid department_id", StatusCode: 400}
		}
		dept, err := s.deptRepo.FindByID(ctx, tx, tenantID, deptID)
		if err != nil {
			return nil, err
		}
		if dept == nil {
			return nil, sharedErrors.ErrNotFound
		}
		team.DepartmentID = deptID
	}
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name != team.Name {
			// Duplicate check runs against the final department (move + rename in one request).
			existing, err := s.teamRepo.FindByName(ctx, tx, tenantID, team.DepartmentID, name)
			if err != nil {
				return nil, err
			}
			if existing != nil && existing.ID != team.ID {
				return nil, sharedErrors.ErrConflict
			}
			team.Name = name
		}
	}
	if req.LeadUserID != nil {
		leadID, err := s.checkLead(ctx, tx, tenantID, req.LeadUserID)
		if err != nil {
			return nil, err
		}
		team.LeadUserID = leadID
	}

	if err := s.teamRepo.Update(ctx, tx, team); err != nil {
		return nil, err
	}
	_ = s.audit.Log(ctx, tx, tenantID.String(), "", "team.updated", "team", team.ID.String(),
		map[string]string{"old_department_id": oldDept.String(), "new_department_id": team.DepartmentID.String()}, "", "")
	return s.toResponse(ctx, tx, tenantID, team)
}

func (s *teamService) DeactivateTeam(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID, reason *string) (*dto.TeamResponse, error) {
	team, err := s.teamRepo.FindByID(ctx, tx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if team == nil {
		return nil, sharedErrors.ErrNotFound
	}
	if team.Status == "inactive" {
		return nil, &sharedErrors.AppError{Code: "CONFLICT", Message: "team is already inactive", StatusCode: 409}
	}
	count, err := s.mappingRepo.CountActiveByTeam(ctx, tx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if count > 0 {
		return nil, &sharedErrors.AppError{Code: "CONFLICT", Message: "team has active mappings; reassign first", StatusCode: 409}
	}
	team.Status = "inactive"
	if err := s.teamRepo.Update(ctx, tx, team); err != nil {
		return nil, err
	}
	_ = s.audit.Log(ctx, tx, tenantID.String(), "", "team.deactivated", "team", team.ID.String(), map[string]interface{}{"reason": reason}, "", "")
	return s.toResponse(ctx, tx, tenantID, team)
}

package services

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/organization/dto"
	"github.com/jaas/jaas/internal/organization/models"
	"github.com/jaas/jaas/internal/organization/repositories"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"github.com/jaas/jaas/internal/shared/queue"
	"gorm.io/gorm"
)

// DesignationService handles job title/level lifecycle.
type DesignationService interface {
	CreateDesignation(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, req dto.CreateDesignationRequest, correlationID uuid.UUID) (*dto.DesignationResponse, error)
	GetDesignation(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*dto.DesignationResponse, error)
	ListDesignations(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, page, perPage int) (*dto.DesignationListResponse, error)
	UpdateDesignation(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID, req dto.UpdateDesignationRequest) (*dto.DesignationResponse, error)
	DeactivateDesignation(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID, reason *string) (*dto.DesignationResponse, error)
}

type designationService struct {
	repo        repositories.DesignationRepository
	mappingRepo repositories.MappingRepository
	publisher   queue.EventPublisher
	audit       auditLogger
}

// NewDesignationService creates a DesignationService.
func NewDesignationService(repo repositories.DesignationRepository, mappingRepo repositories.MappingRepository, publisher queue.EventPublisher, audit auditLogger) DesignationService {
	return &designationService{repo: repo, mappingRepo: mappingRepo, publisher: publisher, audit: audit}
}

func mapDesignation(d *models.Designation) *dto.DesignationResponse {
	return &dto.DesignationResponse{
		ID: d.ID.String(), TenantID: d.TenantID.String(), Title: d.Title,
		Code: d.Code, Level: d.Level, Description: d.Description, Status: d.Status,
		CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt,
	}
}

func (s *designationService) CreateDesignation(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, req dto.CreateDesignationRequest, correlationID uuid.UUID) (*dto.DesignationResponse, error) {
	existing, err := s.repo.FindByTitle(ctx, tx, tenantID, req.Title)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, sharedErrors.ErrConflict
	}
	d := &models.Designation{
		Title: strings.TrimSpace(req.Title), Code: req.Code, Level: req.Level,
		Description: req.Description, Status: "active",
	}
	d.TenantID = tenantID
	if err := s.repo.Create(ctx, tx, d); err != nil {
		return nil, err
	}
	_ = s.audit.Log(ctx, tx, tenantID.String(), "", "designation.created", "designation", d.ID.String(), map[string]string{"title": d.Title}, "", "")
	return mapDesignation(d), nil
}

func (s *designationService) GetDesignation(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*dto.DesignationResponse, error) {
	d, err := s.repo.FindByID(ctx, db, tenantID, id)
	if err != nil {
		return nil, err
	}
	if d == nil {
		return nil, sharedErrors.ErrNotFound
	}
	return mapDesignation(d), nil
}

func (s *designationService) ListDesignations(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, page, perPage int) (*dto.DesignationListResponse, error) {
	out, total, err := s.repo.FindAll(ctx, db, tenantID, page, perPage)
	if err != nil {
		return nil, err
	}
	data := make([]dto.DesignationResponse, len(out))
	for i := range out {
		data[i] = *mapDesignation(&out[i])
	}
	return &dto.DesignationListResponse{Data: data, Meta: dto.NewPaginationMeta(page, perPage, total)}, nil
}

func (s *designationService) UpdateDesignation(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID, req dto.UpdateDesignationRequest) (*dto.DesignationResponse, error) {
	d, err := s.repo.FindByID(ctx, tx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if d == nil {
		return nil, sharedErrors.ErrNotFound
	}
	if req.Title != nil && strings.TrimSpace(*req.Title) != d.Title {
		existing, err := s.repo.FindByTitle(ctx, tx, tenantID, *req.Title)
		if err != nil {
			return nil, err
		}
		if existing != nil && existing.ID != d.ID {
			return nil, sharedErrors.ErrConflict
		}
		d.Title = strings.TrimSpace(*req.Title)
	}
	if req.Level != nil {
		d.Level = req.Level
	}
	if req.Description != nil {
		d.Description = req.Description
	}
	if err := s.repo.Update(ctx, tx, d); err != nil {
		return nil, err
	}
	_ = s.audit.Log(ctx, tx, tenantID.String(), "", "designation.updated", "designation", d.ID.String(), map[string]string{"title": d.Title}, "", "")
	return mapDesignation(d), nil
}

func (s *designationService) DeactivateDesignation(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID, reason *string) (*dto.DesignationResponse, error) {
	d, err := s.repo.FindByID(ctx, tx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if d == nil {
		return nil, sharedErrors.ErrNotFound
	}
	if d.Status == "inactive" {
		return nil, &sharedErrors.AppError{Code: "CONFLICT", Message: "designation is already inactive", StatusCode: 409}
	}
	// FR-DG004: deactivation blocked while active mappings reference the designation.
	refCount, err := s.mappingRepo.CountActiveByDesignation(ctx, tx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if refCount > 0 {
		return nil, &sharedErrors.AppError{Code: "CONFLICT", Message: "designation has active mappings; migrate them first", StatusCode: 409}
	}
	d.Status = "inactive"
	if err := s.repo.Update(ctx, tx, d); err != nil {
		return nil, err
	}
	_ = s.audit.Log(ctx, tx, tenantID.String(), "", "designation.deactivated", "designation", d.ID.String(), map[string]interface{}{"reason": reason}, "", "")
	return mapDesignation(d), nil
}

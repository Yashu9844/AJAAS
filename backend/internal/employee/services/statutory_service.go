package services

import (
	"context"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/employee/dto"
	"github.com/jaas/jaas/internal/employee/models"
	"github.com/jaas/jaas/internal/employee/repositories"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
)

type EmployeeStatutoryService interface {
	GetByProfileID(ctx context.Context, tenantID, profileID uuid.UUID, sensitive bool) (*dto.StatutoryResponse, error)
	Upsert(ctx context.Context, tenantID, profileID uuid.UUID, req dto.UpdateStatutoryRequest) (*dto.StatutoryResponse, error)
}

type employeeStatutoryService struct {
	statutoryRepo repositories.EmployeeStatutoryRepository
	profileRepo   repositories.EmployeeProfileRepository
}

func NewEmployeeStatutoryService(
	statutoryRepo repositories.EmployeeStatutoryRepository,
	profileRepo repositories.EmployeeProfileRepository,
) EmployeeStatutoryService {
	return &employeeStatutoryService{
		statutoryRepo: statutoryRepo,
		profileRepo:   profileRepo,
	}
}

func (s *employeeStatutoryService) GetByProfileID(ctx context.Context, tenantID, profileID uuid.UUID, sensitive bool) (*dto.StatutoryResponse, error) {
	stat, err := s.statutoryRepo.GetByProfileID(ctx, tenantID, profileID)
	if err != nil {
		return nil, err
	}
	if stat == nil {
		return nil, sharedErrors.ErrNotFound
	}

	res := &dto.StatutoryResponse{
		ID:                stat.ID,
		EmployeeProfileID: stat.EmployeeProfileID,
		BankName:          stat.BankName,
		BankRoutingSwift:  stat.BankRoutingSwift,
	}

	if sensitive {
		res.TaxID = stat.TaxID
		res.NationalID = stat.NationalID
		res.BankAccountNumber = stat.BankAccountNumber
	} else {
		res.TaxID = maskString(stat.TaxID)
		res.NationalID = maskString(stat.NationalID)
		res.BankAccountNumber = maskString(stat.BankAccountNumber)
	}

	return res, nil
}

func (s *employeeStatutoryService) Upsert(ctx context.Context, tenantID, profileID uuid.UUID, req dto.UpdateStatutoryRequest) (*dto.StatutoryResponse, error) {
	// Ensure employee exists
	profile, err := s.profileRepo.GetByID(ctx, tenantID, profileID)
	if err != nil {
		return nil, err
	}
	if profile == nil {
		return nil, sharedErrors.ErrNotFound
	}

	stat := &models.EmployeeStatutory{
		EmployeeProfileID: profileID,
		TaxID:             req.TaxID,
		NationalID:        req.NationalID,
		BankName:          req.BankName,
		BankAccountNumber: req.BankAccountNumber,
		BankRoutingSwift:  req.BankRoutingSwift,
	}
	stat.TenantID = tenantID

	if err := s.statutoryRepo.Upsert(ctx, nil, stat); err != nil {
		return nil, err
	}

	return &dto.StatutoryResponse{
		ID:                stat.ID,
		EmployeeProfileID: stat.EmployeeProfileID,
		TaxID:             stat.TaxID,
		NationalID:        stat.NationalID,
		BankName:          stat.BankName,
		BankAccountNumber: stat.BankAccountNumber,
		BankRoutingSwift:  stat.BankRoutingSwift,
	}, nil
}

func maskString(s string) string {
	if len(s) <= 4 {
		return "****"
	}
	return "****" + s[len(s)-4:]
}

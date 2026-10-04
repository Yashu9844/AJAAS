package services

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/employee/dto"
	"github.com/jaas/jaas/internal/employee/events"
	"github.com/jaas/jaas/internal/employee/models"
	"github.com/jaas/jaas/internal/employee/repositories"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"github.com/jaas/jaas/internal/shared/queue"
)

type EmployeeDocumentService interface {
	Upload(ctx context.Context, tenantID, profileID uuid.UUID, req dto.UploadDocumentRequest) (*dto.DocumentResponse, error)
	List(ctx context.Context, tenantID, profileID uuid.UUID) ([]dto.DocumentResponse, error)
	Verify(ctx context.Context, tenantID, docID, verifierID uuid.UUID) (*dto.DocumentResponse, error)
}

type employeeDocumentService struct {
	docRepo     repositories.EmployeeDocumentRepository
	profileRepo repositories.EmployeeProfileRepository
	publisher   queue.EventPublisher
}

func NewEmployeeDocumentService(
	docRepo repositories.EmployeeDocumentRepository,
	profileRepo repositories.EmployeeProfileRepository,
	publisher queue.EventPublisher,
) EmployeeDocumentService {
	return &employeeDocumentService{
		docRepo:     docRepo,
		profileRepo: profileRepo,
		publisher:   publisher,
	}
}

func (s *employeeDocumentService) Upload(ctx context.Context, tenantID, profileID uuid.UUID, req dto.UploadDocumentRequest) (*dto.DocumentResponse, error) {
	profile, err := s.profileRepo.GetByID(ctx, tenantID, profileID)
	if err != nil {
		return nil, err
	}
	if profile == nil {
		return nil, sharedErrors.ErrNotFound
	}

	doc := &models.EmployeeDocument{
		EmployeeProfileID: profileID,
		DocumentType:      req.DocumentType,
		FileName:          req.FileName,
		FileURL:           req.FileURL,
		FileSize:          req.FileSize,
		MimeType:          req.MimeType,
	}
	doc.TenantID = tenantID

	if err := s.docRepo.Create(ctx, nil, doc); err != nil {
		return nil, err
	}

	return toDocumentResponse(doc), nil
}

func (s *employeeDocumentService) List(ctx context.Context, tenantID, profileID uuid.UUID) ([]dto.DocumentResponse, error) {
	docs, err := s.docRepo.ListByProfileID(ctx, tenantID, profileID)
	if err != nil {
		return nil, err
	}

	res := make([]dto.DocumentResponse, len(docs))
	for i := range docs {
		res[i] = *toDocumentResponse(&docs[i])
	}
	return res, nil
}

func (s *employeeDocumentService) Verify(ctx context.Context, tenantID, docID, verifierID uuid.UUID) (*dto.DocumentResponse, error) {
	doc, err := s.docRepo.GetByID(ctx, tenantID, docID)
	if err != nil {
		return nil, err
	}
	if doc == nil {
		return nil, sharedErrors.ErrNotFound
	}

	now := time.Now().UTC()
	doc.VerifiedAt = &now
	doc.VerifiedBy = &verifierID

	if err := s.docRepo.Update(ctx, nil, doc); err != nil {
		return nil, err
	}

	if s.publisher != nil {
		evt := events.NewEventEnvelope(events.EventTypeEmployeeDocVerified, "employee.document.verified", tenantID, doc)
		_ = s.publisher.Publish(ctx, events.ExchangeName, "employee.document.verified", evt)
	}

	return toDocumentResponse(doc), nil
}

func toDocumentResponse(d *models.EmployeeDocument) *dto.DocumentResponse {
	if d == nil {
		return nil
	}
	return &dto.DocumentResponse{
		ID:                d.ID,
		EmployeeProfileID: d.EmployeeProfileID,
		DocumentType:      d.DocumentType,
		FileName:          d.FileName,
		FileURL:           d.FileURL,
		FileSize:          d.FileSize,
		MimeType:          d.MimeType,
		VerifiedAt:        d.VerifiedAt,
		VerifiedBy:        d.VerifiedBy,
		CreatedAt:         d.CreatedAt,
	}
}

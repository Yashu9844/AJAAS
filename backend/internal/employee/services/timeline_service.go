package services

import (
	"context"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/employee/dto"
	"github.com/jaas/jaas/internal/employee/models"
	"github.com/jaas/jaas/internal/employee/repositories"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
)

type EmployeeTimelineService interface {
	List(ctx context.Context, tenantID, profileID uuid.UUID) ([]dto.TimelineResponse, error)
}

type employeeTimelineService struct {
	timelineRepo repositories.EmployeeTimelineRepository
	profileRepo  repositories.EmployeeProfileRepository
}

func NewEmployeeTimelineService(
	timelineRepo repositories.EmployeeTimelineRepository,
	profileRepo repositories.EmployeeProfileRepository,
) EmployeeTimelineService {
	return &employeeTimelineService{
		timelineRepo: timelineRepo,
		profileRepo:  profileRepo,
	}
}

func (s *employeeTimelineService) List(ctx context.Context, tenantID, profileID uuid.UUID) ([]dto.TimelineResponse, error) {
	profile, err := s.profileRepo.GetByID(ctx, tenantID, profileID)
	if err != nil {
		return nil, err
	}
	if profile == nil {
		return nil, sharedErrors.ErrNotFound
	}

	timelines, err := s.timelineRepo.ListByProfileID(ctx, tenantID, profileID)
	if err != nil {
		return nil, err
	}

	res := make([]dto.TimelineResponse, len(timelines))
	for i := range timelines {
		res[i] = *toTimelineResponse(&timelines[i])
	}
	return res, nil
}

func toTimelineResponse(t *models.EmployeeTimeline) *dto.TimelineResponse {
	if t == nil {
		return nil
	}
	return &dto.TimelineResponse{
		ID:                t.ID,
		EmployeeProfileID: t.EmployeeProfileID,
		EventType:         t.EventType,
		EffectiveDate:     t.EffectiveDate,
		Notes:             t.Notes,
		Metadata:          t.Metadata,
		CreatedAt:         t.CreatedAt,
	}
}

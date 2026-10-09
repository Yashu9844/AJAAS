package services

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/recruitment/dto"
	"github.com/jaas/jaas/internal/recruitment/events"
	"github.com/jaas/jaas/internal/recruitment/models"
	"github.com/jaas/jaas/internal/recruitment/pipeline"
	"github.com/jaas/jaas/internal/recruitment/validators"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"gorm.io/gorm"
)

// InterviewService owns interviews and interviewer self-service (FR-IV).
type InterviewService struct{ base }

// NewInterviewService builds an InterviewService.
func NewInterviewService(d Deps) *InterviewService { return &InterviewService{base{d}} }

// Schedule is R12 (RC-004; interviewer must be a working employee, A6-01).
func (s *InterviewService) Schedule(ctx context.Context, a Actor, req dto.ScheduleInterviewRequest) (*dto.InterviewResponse, error) {
	candID, err := parseUUIDField("candidate_id", req.CandidateID)
	if err != nil {
		return nil, err
	}
	interviewer, err := parseUUIDField("interviewer_employee_id", req.InterviewerEmployeeID)
	if err != nil {
		return nil, err
	}
	if err := validators.FutureInstant(req.ScheduledAt, s.Now()); err != nil {
		return nil, invalid("scheduled_at", err.Error())
	}
	if err := s.checkInterviewer(ctx, a.TenantID, interviewer); err != nil {
		return nil, err
	}
	iv := &models.Interview{TenantID: a.TenantID, CandidateID: candID, RoundName: req.RoundName, InterviewerEmployeeID: interviewer,
		ScheduledAt: req.ScheduledAt.UTC(), DurationMins: req.DurationMins, MeetingLink: req.MeetingLink, Status: models.InterviewScheduled}
	var row *models.OutboxEvent
	err = s.Tx.InTx(ctx, func(tx *gorm.DB) error {
		c, err := s.lockCandidate(ctx, tx, a.TenantID, candID)
		if err != nil {
			return err
		}
		if c.Stage != pipeline.Interview {
			return ErrInvalidStage
		}
		if err := s.Repos.Interviews.Create(ctx, tx, iv); err != nil {
			return err
		}
		row, err = s.writeOutbox(ctx, tx, a, events.InterviewScheduled, events.Payload{JobID: c.JobID, CandidateID: &c.ID, InterviewID: &iv.ID, Stage: c.Stage})
		return err
	})
	if err != nil {
		return nil, err
	}
	s.afterCommit(ctx, a, auditEntry{"recruitment.interview.scheduled", "recruitment_interview", iv.ID.String(), map[string]string{"candidate_id": candID.String()}}, row)
	out := mapInterview(iv)
	return &out, nil
}

func (s *InterviewService) checkInterviewer(ctx context.Context, tenantID, id uuid.UUID) error {
	emp, err := s.Employees.GetByID(ctx, tenantID, id)
	if errors.Is(err, sharedErrors.ErrNotFound) || (err == nil && emp == nil) {
		return ErrEmployeeNotFound
	}
	if err != nil {
		return err
	}
	if !workingStatuses[emp.Status] {
		return ErrInterviewerState
	}
	return nil
}

// ListByCandidate is R13.
func (s *InterviewService) ListByCandidate(ctx context.Context, tenantID, candidateID uuid.UUID, page dto.Page) ([]dto.InterviewResponse, dto.PageMeta, error) {
	rows, total, err := s.Repos.Interviews.ListByCandidate(ctx, s.Tx.DB(), tenantID, candidateID, page)
	if err != nil {
		return nil, dto.PageMeta{}, err
	}
	return mapList(rows, mapInterview), page.Meta(total), nil
}

// ListMine is R14: the caller's own interviews (NFR-SEC002).
func (s *InterviewService) ListMine(ctx context.Context, a Actor, status string, page dto.Page) ([]dto.InterviewResponse, dto.PageMeta, error) {
	emp, err := s.employeeFor(ctx, a)
	if err != nil {
		return nil, dto.PageMeta{}, err
	}
	rows, total, err := s.Repos.Interviews.ListByInterviewer(ctx, s.Tx.DB(), a.TenantID, emp.ID, status, page)
	if err != nil {
		return nil, dto.PageMeta{}, err
	}
	return mapList(rows, mapInterview), page.Meta(total), nil
}

// Feedback is R15: only the assigned interviewer, only on scheduled interviews (RC-005).
func (s *InterviewService) Feedback(ctx context.Context, a Actor, id uuid.UUID, req dto.FeedbackRequest) (*dto.InterviewResponse, error) {
	emp, err := s.employeeFor(ctx, a)
	if err != nil {
		return nil, err
	}
	iv, err := s.transition(ctx, a.TenantID, id, &emp.ID, func(iv *models.Interview) {
		now := s.Now().UTC()
		iv.Status, iv.Rating, iv.Recommendation, iv.Feedback, iv.FeedbackAt = models.InterviewCompleted, &req.Rating, &req.Recommendation, req.Notes, &now
	})
	if err != nil {
		return nil, err
	}
	s.afterCommit(ctx, a, auditEntry{"recruitment.interview.feedback", "recruitment_interview", iv.ID.String(), map[string]string{"status": iv.Status}}, nil)
	out := mapInterview(iv)
	return &out, nil
}

// Cancel is R16.
func (s *InterviewService) Cancel(ctx context.Context, a Actor, id uuid.UUID) (*dto.InterviewResponse, error) {
	iv, err := s.transition(ctx, a.TenantID, id, nil, func(iv *models.Interview) { iv.Status = models.InterviewCancelled })
	if err != nil {
		return nil, err
	}
	s.afterCommit(ctx, a, auditEntry{"recruitment.interview.cancelled", "recruitment_interview", iv.ID.String(), nil}, nil)
	out := mapInterview(iv)
	return &out, nil
}

// transition loads a scheduled interview (owned by owner when set; others 404), applies fn and saves it in one transaction.
func (s *InterviewService) transition(ctx context.Context, tenantID, id uuid.UUID, owner *uuid.UUID, fn func(*models.Interview)) (*models.Interview, error) {
	var iv *models.Interview
	err := s.Tx.InTx(ctx, func(tx *gorm.DB) error {
		var err error
		if iv, err = s.Repos.Interviews.FindByID(ctx, tx, tenantID, id); err != nil || iv == nil {
			return orNotFound(err)
		}
		if owner != nil && iv.InterviewerEmployeeID != *owner {
			return ErrNotFound
		}
		if iv.Status != models.InterviewScheduled {
			return ErrInterviewState
		}
		fn(iv)
		return s.Repos.Interviews.Update(ctx, tx, iv)
	})
	return iv, err
}

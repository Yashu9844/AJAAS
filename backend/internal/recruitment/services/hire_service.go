package services

import (
	"context"
	"errors"

	"github.com/google/uuid"
	employeeDTO "github.com/jaas/jaas/internal/employee/dto"
	identityDTO "github.com/jaas/jaas/internal/identity/dto"
	"github.com/jaas/jaas/internal/recruitment/dto"
	"github.com/jaas/jaas/internal/recruitment/events"
	"github.com/jaas/jaas/internal/recruitment/models"
	"github.com/jaas/jaas/internal/recruitment/pipeline"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"gorm.io/gorm"
)

// HireService runs the three-step idempotent hire saga (FR-HR001, D6-04, RC-007/008/010).
type HireService struct{ base }

// NewHireService builds a HireService.
func NewHireService(d Deps) *HireService { return &HireService{base{d}} }

// hirePlan is what step 1 resolves for steps 2 and 3.
type hirePlan struct {
	candidate *models.Candidate
	job       *models.Job
	offer     *models.Offer
	userID    uuid.UUID
}

// Hire is R11.
func (s *HireService) Hire(ctx context.Context, a Actor, candidateID uuid.UUID, req dto.HireRequest) (*dto.HireResponse, error) {
	plan, err := s.reserveUser(ctx, a, candidateID)
	if err != nil {
		return nil, err
	}
	emp, err := s.ensureEmployee(ctx, a.TenantID, plan, req)
	if err != nil {
		return nil, err
	}
	job, row, err := s.complete(ctx, a, candidateID, emp.ID)
	if err != nil {
		return nil, err
	}
	s.afterCommit(ctx, a, auditEntry{"recruitment.candidate.hired", "recruitment_candidate", candidateID.String(),
		map[string]string{"employee_id": emp.ID.String(), "job_id": job.ID.String()}}, row)
	return &dto.HireResponse{CandidateID: candidateID, UserID: plan.userID, EmployeeID: emp.ID, JobStatus: job.Status}, nil
}

// reserveUser is step 1: validate hireability and invite the user once, storing its id on the candidate (RC-010).
func (s *HireService) reserveUser(ctx context.Context, a Actor, candidateID uuid.UUID) (*hirePlan, error) {
	plan := &hirePlan{}
	err := s.Tx.InTx(ctx, func(tx *gorm.DB) error {
		if err := s.loadHireable(ctx, tx, a.TenantID, candidateID, plan); err != nil {
			return err
		}
		c := plan.candidate
		if c.HiredUserID != nil {
			plan.userID = *c.HiredUserID
			return nil
		}
		u, err := s.Users.InviteUser(ctx, tx, a.TenantID, identityDTO.InviteUserRequest{Email: c.Email, FirstName: c.FirstName, LastName: c.LastName}, a.CorrelationID)
		if err != nil {
			return err
		}
		if plan.userID, err = uuid.Parse(u.ID); err != nil {
			return err
		}
		c.HiredUserID = &plan.userID
		return s.Repos.Candidates.Update(ctx, tx, c)
	})
	return plan, err
}

// loadHireable locks candidate and job and checks RC-007/RC-008.
func (s *HireService) loadHireable(ctx context.Context, tx *gorm.DB, tenantID, candidateID uuid.UUID, plan *hirePlan) error {
	c, err := s.lockCandidate(ctx, tx, tenantID, candidateID)
	if err != nil {
		return err
	}
	if c.Stage != pipeline.Offer {
		return ErrNotHireable
	}
	offer, err := s.Repos.Offers.LatestByStatus(ctx, tx, tenantID, c.ID, models.OfferAccepted)
	if err != nil {
		return err
	}
	if offer == nil {
		return ErrNotHireable
	}
	job, err := s.Repos.Jobs.LockByID(ctx, tx, tenantID, c.JobID)
	if err != nil || job == nil {
		return orNotFound(err)
	}
	if job.Status != pipeline.JobOpen && job.Status != pipeline.JobOnHold {
		return ErrJobNotOpen
	}
	plan.candidate, plan.job, plan.offer = c, job, offer
	return nil
}

// ensureEmployee is step 2: reuse the user's profile if a previous attempt created it, else create one in Module 2.
func (s *HireService) ensureEmployee(ctx context.Context, tenantID uuid.UUID, plan *hirePlan, req dto.HireRequest) (*employeeDTO.EmployeeResponse, error) {
	existing, err := s.Employees.GetByUserID(ctx, tenantID, plan.userID)
	if err == nil && existing != nil {
		return existing, nil
	}
	if err != nil && !errors.Is(err, sharedErrors.ErrNotFound) {
		return nil, err
	}
	empType := req.EmploymentType
	if empType == "" {
		empType = plan.job.EmploymentType
	}
	c := plan.candidate
	return s.Employees.CreateEmployee(ctx, tenantID, employeeDTO.CreateEmployeeRequest{UserID: plan.userID, EmployeeCode: req.EmployeeCode,
		FirstName: c.FirstName, LastName: c.LastName, EmploymentType: empType, JoiningDate: plan.offer.JoiningDate})
}

// complete is step 3: candidate hired, job count (+ filled), stage event and outbox in one transaction.
func (s *HireService) complete(ctx context.Context, a Actor, candidateID, employeeID uuid.UUID) (*models.Job, *models.OutboxEvent, error) {
	plan := &hirePlan{}
	var row *models.OutboxEvent
	err := s.Tx.InTx(ctx, func(tx *gorm.DB) error {
		if err := s.loadHireable(ctx, tx, a.TenantID, candidateID, plan); err != nil {
			return err
		}
		c, job := plan.candidate, plan.job
		c.HiredEmployeeID = &employeeID
		if err := s.moveStage(ctx, tx, a, c, stageChange{to: pipeline.Hired}); err != nil {
			return err
		}
		if job.HiredCount++; job.HiredCount >= job.Headcount {
			now := s.Now().UTC()
			job.Status, job.ClosedAt = pipeline.JobFilled, &now
		}
		if err := s.Repos.Jobs.Update(ctx, tx, job); err != nil {
			return err
		}
		var err error
		row, err = s.writeOutbox(ctx, tx, a, events.CandidateHired, events.Payload{JobID: job.ID, CandidateID: &c.ID, EmployeeID: &employeeID,
			Stage: pipeline.Hired, JobStatus: job.Status})
		return err
	})
	return plan.job, row, err
}

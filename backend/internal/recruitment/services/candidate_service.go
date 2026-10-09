package services

import (
	"context"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/recruitment/dto"
	"github.com/jaas/jaas/internal/recruitment/events"
	"github.com/jaas/jaas/internal/recruitment/models"
	"github.com/jaas/jaas/internal/recruitment/pipeline"
	"github.com/jaas/jaas/internal/recruitment/repositories"
	"github.com/jaas/jaas/internal/recruitment/validators"
	"gorm.io/gorm"
)

// CandidateService owns candidates and their pipeline (FR-CD).
type CandidateService struct{ base }

// NewCandidateService builds a CandidateService.
func NewCandidateService(d Deps) *CandidateService { return &CandidateService{base{d}} }

// Create is R6: a candidate on an open job, stage applied + first stage event (RC-001, RC-003).
func (s *CandidateService) Create(ctx context.Context, a Actor, req dto.CreateCandidateRequest) (*dto.CandidateResponse, error) {
	jobID, err := parseUUIDField("job_id", req.JobID)
	if err != nil {
		return nil, err
	}
	if req.ExpectedCTC != nil && *req.ExpectedCTC < 0 {
		return nil, invalid("expected_ctc", "must not be negative")
	}
	c := &models.Candidate{TenantID: a.TenantID, JobID: jobID, FirstName: req.FirstName, LastName: req.LastName, Email: validators.NormalizeEmail(req.Email),
		Phone: req.Phone, Source: req.Source, ResumeURL: req.ResumeURL, ExpectedCTC: req.ExpectedCTC, NoticePeriodDays: req.NoticePeriodDays, Stage: pipeline.Applied}
	var row *models.OutboxEvent
	err = s.Tx.InTx(ctx, func(tx *gorm.DB) error {
		var err error
		row, err = s.apply(ctx, tx, a, c)
		return err
	})
	if err != nil {
		return nil, err
	}
	s.afterCommit(ctx, a, auditEntry{"recruitment.candidate.created", "recruitment_candidate", c.ID.String(), map[string]string{"job_id": jobID.String()}}, row)
	out := mapCandidate(c)
	return &out, nil
}

func (s *CandidateService) apply(ctx context.Context, tx *gorm.DB, a Actor, c *models.Candidate) (*models.OutboxEvent, error) {
	job, err := s.Repos.Jobs.LockByID(ctx, tx, a.TenantID, c.JobID)
	if err != nil || job == nil {
		return nil, orNotFound(err)
	}
	if job.Status != pipeline.JobOpen {
		return nil, ErrJobNotOpen
	}
	if err := s.ensureUniqueEmail(ctx, tx, c.JobID, c.Email, uuid.Nil); err != nil {
		return nil, err
	}
	if err := s.Repos.Candidates.Create(ctx, tx, c); err != nil {
		return nil, err
	}
	err = s.Repos.Candidates.AddEvent(ctx, tx, &models.StageEvent{TenantID: c.TenantID, CandidateID: c.ID, ToStage: pipeline.Applied, ActorUserID: a.UserID})
	if err != nil {
		return nil, err
	}
	return s.writeOutbox(ctx, tx, a, events.CandidateApplied, events.Payload{JobID: c.JobID, CandidateID: &c.ID, Stage: pipeline.Applied})
}

// ensureUniqueEmail enforces FR-CD001 (case-insensitive per job); self is excluded on update.
func (s *CandidateService) ensureUniqueEmail(ctx context.Context, tx *gorm.DB, jobID uuid.UUID, email string, self uuid.UUID) error {
	dup, err := s.Repos.Candidates.FindByJobEmail(ctx, tx, jobID, email)
	if err != nil {
		return err
	}
	if dup != nil && dup.ID != self {
		return ErrDuplicateEmail
	}
	return nil
}

// Get is R8 with stage history.
func (s *CandidateService) Get(ctx context.Context, tenantID, id uuid.UUID) (*dto.CandidateResponse, error) {
	c, err := s.Repos.Candidates.FindByID(ctx, s.Tx.DB(), tenantID, id)
	if err != nil || c == nil {
		return nil, orNotFound(err)
	}
	history, err := s.Repos.Candidates.Events(ctx, s.Tx.DB(), tenantID, id)
	if err != nil {
		return nil, err
	}
	out := mapCandidate(c)
	out.History = mapList(history, mapEvent)
	return &out, nil
}

// List is R7.
func (s *CandidateService) List(ctx context.Context, tenantID uuid.UUID, f repositories.CandidateFilter, page dto.Page) ([]dto.CandidateResponse, dto.PageMeta, error) {
	rows, total, err := s.Repos.Candidates.List(ctx, s.Tx.DB(), tenantID, f, page)
	if err != nil {
		return nil, dto.PageMeta{}, err
	}
	return mapList(rows, mapCandidate), page.Meta(total), nil
}

// Update is R9: contact fields only, never for a hired candidate.
func (s *CandidateService) Update(ctx context.Context, a Actor, id uuid.UUID, req dto.UpdateCandidateRequest) (*dto.CandidateResponse, error) {
	if req.ExpectedCTC != nil && *req.ExpectedCTC < 0 {
		return nil, invalid("expected_ctc", "must not be negative")
	}
	var c *models.Candidate
	err := s.Tx.InTx(ctx, func(tx *gorm.DB) error {
		var err error
		if c, err = s.lockCandidate(ctx, tx, a.TenantID, id); err != nil {
			return err
		}
		if c.Stage == pipeline.Hired {
			return ErrInvalidStage
		}
		if req.Email != nil {
			email := validators.NormalizeEmail(*req.Email)
			if err := s.ensureUniqueEmail(ctx, tx, c.JobID, email, c.ID); err != nil {
				return err
			}
			c.Email = email
		}
		applyCandidatePatch(c, req)
		return s.Repos.Candidates.Update(ctx, tx, c)
	})
	if err != nil {
		return nil, err
	}
	s.afterCommit(ctx, a, auditEntry{"recruitment.candidate.updated", "recruitment_candidate", c.ID.String(), nil}, nil)
	out := mapCandidate(c)
	return &out, nil
}

// MoveStage is R10 along RC-002 only.
func (s *CandidateService) MoveStage(ctx context.Context, a Actor, id uuid.UUID, req dto.StageRequest) (*dto.CandidateResponse, error) {
	var c *models.Candidate
	var from string
	err := s.Tx.InTx(ctx, func(tx *gorm.DB) error {
		var err error
		if c, err = s.lockCandidate(ctx, tx, a.TenantID, id); err != nil {
			return err
		}
		if from = c.Stage; !pipeline.CanMove(from, req.Stage) {
			return ErrInvalidStage
		}
		if err := s.closeOpenOffer(ctx, tx, c); err != nil {
			return err
		}
		return s.moveStage(ctx, tx, a, c, stageChange{to: req.Stage, note: req.Note})
	})
	if err != nil {
		return nil, err
	}
	s.afterCommit(ctx, a, auditEntry{"recruitment.candidate.stage", "recruitment_candidate", c.ID.String(), map[string]string{"from": from, "to": req.Stage}}, nil)
	out := mapCandidate(c)
	return &out, nil
}

// closeOpenOffer withdraws an open offer when a candidate leaves the pipeline from `offer` (RC-006 frees the slot).
func (s *CandidateService) closeOpenOffer(ctx context.Context, tx *gorm.DB, c *models.Candidate) error {
	if c.Stage != pipeline.Offer {
		return nil
	}
	o, err := s.Repos.Offers.LatestByStatus(ctx, tx, c.TenantID, c.ID, models.OfferOffered)
	if err != nil || o == nil {
		return err
	}
	now := s.Now().UTC()
	o.Status, o.DecidedAt = models.OfferWithdrawn, &now
	return s.Repos.Offers.Update(ctx, tx, o)
}

func applyCandidatePatch(c *models.Candidate, req dto.UpdateCandidateRequest) {
	setIf(&c.FirstName, req.FirstName)
	setIf(&c.LastName, req.LastName)
	if req.Phone != nil {
		c.Phone = req.Phone
	}
	if req.ResumeURL != nil {
		c.ResumeURL = req.ResumeURL
	}
	if req.ExpectedCTC != nil {
		c.ExpectedCTC = req.ExpectedCTC
	}
	if req.NoticePeriodDays != nil {
		c.NoticePeriodDays = req.NoticePeriodDays
	}
}

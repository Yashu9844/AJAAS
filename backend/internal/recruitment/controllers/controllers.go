package controllers

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/recruitment/dto"
	"github.com/jaas/jaas/internal/recruitment/repositories"
	"github.com/jaas/jaas/internal/recruitment/services"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
)

// JobService is the R1–R5 surface the edge needs.
type JobService interface {
	Create(ctx context.Context, a services.Actor, req dto.CreateJobRequest) (*dto.JobResponse, error)
	Get(ctx context.Context, tenantID, id uuid.UUID) (*dto.JobResponse, error)
	List(ctx context.Context, tenantID uuid.UUID, f repositories.JobFilter, page dto.Page) ([]dto.JobResponse, dto.PageMeta, error)
	Update(ctx context.Context, a services.Actor, id uuid.UUID, req dto.UpdateJobRequest) (*dto.JobResponse, error)
	SetStatus(ctx context.Context, a services.Actor, id uuid.UUID, to string) (*dto.JobResponse, error)
}

// CandidateService is the R6–R10 surface.
type CandidateService interface {
	Create(ctx context.Context, a services.Actor, req dto.CreateCandidateRequest) (*dto.CandidateResponse, error)
	Get(ctx context.Context, tenantID, id uuid.UUID) (*dto.CandidateResponse, error)
	List(ctx context.Context, tenantID uuid.UUID, f repositories.CandidateFilter, page dto.Page) ([]dto.CandidateResponse, dto.PageMeta, error)
	Update(ctx context.Context, a services.Actor, id uuid.UUID, req dto.UpdateCandidateRequest) (*dto.CandidateResponse, error)
	MoveStage(ctx context.Context, a services.Actor, id uuid.UUID, req dto.StageRequest) (*dto.CandidateResponse, error)
}

// HireService is R11.
type HireService interface {
	Hire(ctx context.Context, a services.Actor, candidateID uuid.UUID, req dto.HireRequest) (*dto.HireResponse, error)
}

// InterviewService is R12–R16.
type InterviewService interface {
	Schedule(ctx context.Context, a services.Actor, req dto.ScheduleInterviewRequest) (*dto.InterviewResponse, error)
	ListByCandidate(ctx context.Context, tenantID, candidateID uuid.UUID, page dto.Page) ([]dto.InterviewResponse, dto.PageMeta, error)
	ListMine(ctx context.Context, a services.Actor, status string, page dto.Page) ([]dto.InterviewResponse, dto.PageMeta, error)
	Feedback(ctx context.Context, a services.Actor, id uuid.UUID, req dto.FeedbackRequest) (*dto.InterviewResponse, error)
	Cancel(ctx context.Context, a services.Actor, id uuid.UUID) (*dto.InterviewResponse, error)
}

// OfferService is R17–R20.
type OfferService interface {
	Create(ctx context.Context, a services.Actor, req dto.CreateOfferRequest) (*dto.OfferResponse, error)
	List(ctx context.Context, tenantID, candidateID uuid.UUID, page dto.Page) ([]dto.OfferResponse, dto.PageMeta, error)
	Decide(ctx context.Context, a services.Actor, id uuid.UUID, decision string) (*dto.OfferResponse, error)
	Withdraw(ctx context.Context, a services.Actor, id uuid.UUID) (*dto.OfferResponse, error)
}

// Controller serves all 20 Module 6 operations.
type Controller struct {
	jobs       JobService
	candidates CandidateService
	hire       HireService
	interviews InterviewService
	offers     OfferService
}

// Services bundles the controller's collaborators.
type Services struct {
	Jobs       JobService
	Candidates CandidateService
	Hire       HireService
	Interviews InterviewService
	Offers     OfferService
}

// New builds the Controller.
func New(s Services) *Controller {
	return &Controller{jobs: s.Jobs, candidates: s.Candidates, hire: s.Hire, interviews: s.Interviews, offers: s.Offers}
}

// queryUUID parses an optional UUID query parameter; malformed → 400.
func queryUUID(c *gin.Context, name string) (*uuid.UUID, bool) {
	raw := c.Query(name)
	if raw == "" {
		return nil, true
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		fail(c, &sharedErrors.AppError{Code: sharedErrors.ErrValidation.Code, Message: name + " must be a UUID", StatusCode: http.StatusBadRequest})
		return nil, false
	}
	return &id, true
}

// requiredQueryUUID is queryUUID for mandatory filters (R13, R18).
func requiredQueryUUID(c *gin.Context, name string) (uuid.UUID, bool) {
	id, ok := queryUUID(c, name)
	if ok && id == nil {
		fail(c, &sharedErrors.AppError{Code: sharedErrors.ErrValidation.Code, Message: name + " is required", StatusCode: http.StatusBadRequest})
		return uuid.Nil, false
	}
	if !ok {
		return uuid.Nil, false
	}
	return *id, true
}

// CreateJob handles POST /recruitment/jobs (R1).
func (ctl *Controller) CreateJob(c *gin.Context) {
	a, okA := actorFrom(c)
	var req dto.CreateJobRequest
	if !okA || !bindJSON(c, &req) {
		return
	}
	res, err := ctl.jobs.Create(c.Request.Context(), a, req)
	respond(c, http.StatusCreated, res, err)
}

// ListJobs handles GET /recruitment/jobs (R2).
func (ctl *Controller) ListJobs(c *gin.Context) {
	a, okA := actorFrom(c)
	if !okA {
		return
	}
	dept, okD := queryUUID(c, "department_id")
	if !okD {
		return
	}
	res, meta, err := ctl.jobs.List(c.Request.Context(), a.TenantID, repositories.JobFilter{Status: c.Query("status"), DepartmentID: dept}, page(c))
	respondList(c, res, meta, err)
}

// GetJob handles GET /recruitment/jobs/:id (R3).
func (ctl *Controller) GetJob(c *gin.Context) {
	a, okA := actorFrom(c)
	id, okID := pathIDIf(c, okA)
	if !okID {
		return
	}
	res, err := ctl.jobs.Get(c.Request.Context(), a.TenantID, id)
	respond(c, http.StatusOK, res, err)
}

// UpdateJob handles PATCH /recruitment/jobs/:id (R4).
func (ctl *Controller) UpdateJob(c *gin.Context) {
	a, okA := actorFrom(c)
	id, okID := pathIDIf(c, okA)
	var req dto.UpdateJobRequest
	if !okID || !bindJSON(c, &req) {
		return
	}
	res, err := ctl.jobs.Update(c.Request.Context(), a, id, req)
	respond(c, http.StatusOK, res, err)
}

// SetJobStatus handles POST /recruitment/jobs/:id/status (R5).
func (ctl *Controller) SetJobStatus(c *gin.Context) {
	a, okA := actorFrom(c)
	id, okID := pathIDIf(c, okA)
	var req dto.JobStatusRequest
	if !okID || !bindJSON(c, &req) {
		return
	}
	res, err := ctl.jobs.SetStatus(c.Request.Context(), a, id, req.Status)
	respond(c, http.StatusOK, res, err)
}

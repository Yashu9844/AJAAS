package controllers

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/hr/performance/dto"
	"github.com/jaas/jaas/internal/hr/performance/repositories"
	"github.com/jaas/jaas/internal/hr/performance/services"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
)

// CycleService is the C1–C8 surface the edge needs.
type CycleService interface {
	Create(ctx context.Context, a services.Actor, req dto.CreateCycleRequest) (*dto.CycleResponse, error)
	Get(ctx context.Context, tenantID, id uuid.UUID) (*dto.CycleResponse, error)
	List(ctx context.Context, tenantID uuid.UUID, status string, page dto.Page) ([]dto.CycleResponse, dto.PageMeta, error)
	Update(ctx context.Context, a services.Actor, id uuid.UUID, req dto.UpdateCycleRequest) (*dto.CycleResponse, error)
	Activate(ctx context.Context, a services.Actor, id uuid.UUID) (*dto.CycleResponse, error)
	LaunchReviews(ctx context.Context, a services.Actor, id uuid.UUID) (*dto.LaunchResponse, error)
	Complete(ctx context.Context, a services.Actor, id uuid.UUID) (*dto.CycleResponse, error)
	NineBox(ctx context.Context, tenantID, id uuid.UUID) (*dto.NineBoxResponse, error)
}

// GoalService is the G1–G8 surface.
type GoalService interface {
	Create(ctx context.Context, a services.Actor, req dto.CreateGoalRequest) (*dto.GoalResponse, error)
	Get(ctx context.Context, a services.Actor, id uuid.UUID) (*dto.GoalResponse, error)
	List(ctx context.Context, a services.Actor, f repositories.GoalFilter, page dto.Page) ([]dto.GoalResponse, dto.PageMeta, error)
	ListMine(ctx context.Context, a services.Actor, cycleID *uuid.UUID, page dto.Page) ([]dto.GoalResponse, dto.PageMeta, error)
	Update(ctx context.Context, a services.Actor, id uuid.UUID, req dto.UpdateGoalRequest) (*dto.GoalResponse, error)
	SetStatus(ctx context.Context, a services.Actor, id uuid.UUID, to string) (*dto.GoalResponse, error)
	AddKeyResult(ctx context.Context, a services.Actor, id uuid.UUID, req dto.KeyResultRequest) (*dto.GoalResponse, error)
	CheckIn(ctx context.Context, a services.Actor, id uuid.UUID, req dto.CheckInRequest) (*dto.GoalResponse, error)
}

// ReviewService is the V1–V6 surface.
type ReviewService interface {
	Assign(ctx context.Context, a services.Actor, req dto.AssignReviewerRequest) (*dto.ReviewResponse, error)
	List(ctx context.Context, tenantID uuid.UUID, f repositories.ReviewFilter, page dto.Page) ([]dto.ReviewResponse, dto.PageMeta, error)
	ListAssigned(ctx context.Context, a services.Actor, status string, page dto.Page) ([]dto.ReviewResponse, dto.PageMeta, error)
	ListMine(ctx context.Context, a services.Actor, cycleID *uuid.UUID) ([]dto.ReviewResponse, error)
	Get(ctx context.Context, a services.Actor, id uuid.UUID) (*dto.ReviewResponse, error)
	Submit(ctx context.Context, a services.Actor, id uuid.UUID, req dto.SubmitReviewRequest) (*dto.ReviewResponse, error)
}

// Controller serves all 22 HR/Performance operations.
type Controller struct {
	cycles  CycleService
	goals   GoalService
	reviews ReviewService
}

// Services bundles the controller's collaborators.
type Services struct {
	Cycles  CycleService
	Goals   GoalService
	Reviews ReviewService
}

// New builds the Controller.
func New(s Services) *Controller {
	return &Controller{cycles: s.Cycles, goals: s.Goals, reviews: s.Reviews}
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

// --- cycles (C1–C8) ---

func (ctl *Controller) CreateCycle(c *gin.Context) {
	a, okA := actorFrom(c)
	var req dto.CreateCycleRequest
	if !okA || !bindJSON(c, &req) {
		return
	}
	res, err := ctl.cycles.Create(c.Request.Context(), a, req)
	respond(c, http.StatusCreated, res, err)
}

func (ctl *Controller) ListCycles(c *gin.Context) {
	a, okA := actorFrom(c)
	if !okA {
		return
	}
	res, meta, err := ctl.cycles.List(c.Request.Context(), a.TenantID, c.Query("status"), page(c))
	respondList(c, res, meta, err)
}

func (ctl *Controller) GetCycle(c *gin.Context) {
	a, okA := actorFrom(c)
	id, okID := pathIDIf(c, okA)
	if !okID {
		return
	}
	res, err := ctl.cycles.Get(c.Request.Context(), a.TenantID, id)
	respond(c, http.StatusOK, res, err)
}

func (ctl *Controller) UpdateCycle(c *gin.Context) {
	a, okA := actorFrom(c)
	id, okID := pathIDIf(c, okA)
	var req dto.UpdateCycleRequest
	if !okID || !bindJSON(c, &req) {
		return
	}
	res, err := ctl.cycles.Update(c.Request.Context(), a, id, req)
	respond(c, http.StatusOK, res, err)
}

func (ctl *Controller) ActivateCycle(c *gin.Context) {
	a, okA := actorFrom(c)
	id, okID := pathIDIf(c, okA)
	if !okID {
		return
	}
	res, err := ctl.cycles.Activate(c.Request.Context(), a, id)
	respond(c, http.StatusOK, res, err)
}

func (ctl *Controller) LaunchReviews(c *gin.Context) {
	a, okA := actorFrom(c)
	id, okID := pathIDIf(c, okA)
	if !okID {
		return
	}
	res, err := ctl.cycles.LaunchReviews(c.Request.Context(), a, id)
	respond(c, http.StatusOK, res, err)
}

func (ctl *Controller) CompleteCycle(c *gin.Context) {
	a, okA := actorFrom(c)
	id, okID := pathIDIf(c, okA)
	if !okID {
		return
	}
	res, err := ctl.cycles.Complete(c.Request.Context(), a, id)
	respond(c, http.StatusOK, res, err)
}

func (ctl *Controller) NineBox(c *gin.Context) {
	a, okA := actorFrom(c)
	id, okID := pathIDIf(c, okA)
	if !okID {
		return
	}
	res, err := ctl.cycles.NineBox(c.Request.Context(), a.TenantID, id)
	respond(c, http.StatusOK, res, err)
}

// --- goals (G1–G8) ---

func (ctl *Controller) CreateGoal(c *gin.Context) {
	a, okA := actorFrom(c)
	var req dto.CreateGoalRequest
	if !okA || !bindJSON(c, &req) {
		return
	}
	res, err := ctl.goals.Create(c.Request.Context(), a, req)
	respond(c, http.StatusCreated, res, err)
}

func (ctl *Controller) ListGoals(c *gin.Context) {
	a, okA := actorFrom(c)
	if !okA {
		return
	}
	cycle, okC := queryUUID(c, "cycle_id")
	if !okC {
		return
	}
	emp, okE := queryUUID(c, "employee_id")
	if !okE {
		return
	}
	res, meta, err := ctl.goals.List(c.Request.Context(), a, repositories.GoalFilter{CycleID: cycle, EmployeeID: emp, Status: c.Query("status")}, page(c))
	respondList(c, res, meta, err)
}

func (ctl *Controller) MyGoals(c *gin.Context) {
	a, okA := actorFrom(c)
	if !okA {
		return
	}
	cycle, okC := queryUUID(c, "cycle_id")
	if !okC {
		return
	}
	res, meta, err := ctl.goals.ListMine(c.Request.Context(), a, cycle, page(c))
	respondList(c, res, meta, err)
}

func (ctl *Controller) GetGoal(c *gin.Context) {
	a, okA := actorFrom(c)
	id, okID := pathIDIf(c, okA)
	if !okID {
		return
	}
	res, err := ctl.goals.Get(c.Request.Context(), a, id)
	respond(c, http.StatusOK, res, err)
}

func (ctl *Controller) UpdateGoal(c *gin.Context) {
	a, okA := actorFrom(c)
	id, okID := pathIDIf(c, okA)
	var req dto.UpdateGoalRequest
	if !okID || !bindJSON(c, &req) {
		return
	}
	res, err := ctl.goals.Update(c.Request.Context(), a, id, req)
	respond(c, http.StatusOK, res, err)
}

func (ctl *Controller) SetGoalStatus(c *gin.Context) {
	a, okA := actorFrom(c)
	id, okID := pathIDIf(c, okA)
	var req dto.GoalStatusRequest
	if !okID || !bindJSON(c, &req) {
		return
	}
	res, err := ctl.goals.SetStatus(c.Request.Context(), a, id, req.Status)
	respond(c, http.StatusOK, res, err)
}

func (ctl *Controller) AddKeyResult(c *gin.Context) {
	a, okA := actorFrom(c)
	id, okID := pathIDIf(c, okA)
	var req dto.KeyResultRequest
	if !okID || !bindJSON(c, &req) {
		return
	}
	res, err := ctl.goals.AddKeyResult(c.Request.Context(), a, id, req)
	respond(c, http.StatusCreated, res, err)
}

func (ctl *Controller) CheckIn(c *gin.Context) {
	a, okA := actorFrom(c)
	id, okID := pathIDIf(c, okA)
	var req dto.CheckInRequest
	if !okID || !bindJSON(c, &req) {
		return
	}
	res, err := ctl.goals.CheckIn(c.Request.Context(), a, id, req)
	respond(c, http.StatusCreated, res, err)
}

// --- reviews (V1–V6) ---

func (ctl *Controller) AssignReviewer(c *gin.Context) {
	a, okA := actorFrom(c)
	var req dto.AssignReviewerRequest
	if !okA || !bindJSON(c, &req) {
		return
	}
	res, err := ctl.reviews.Assign(c.Request.Context(), a, req)
	respond(c, http.StatusCreated, res, err)
}

func (ctl *Controller) ListReviews(c *gin.Context) {
	a, okA := actorFrom(c)
	if !okA {
		return
	}
	cycle, okC := queryUUID(c, "cycle_id")
	if !okC {
		return
	}
	emp, okE := queryUUID(c, "employee_id")
	if !okE {
		return
	}
	res, meta, err := ctl.reviews.List(c.Request.Context(), a.TenantID, repositories.ReviewFilter{CycleID: cycle, EmployeeID: emp, Status: c.Query("status")}, page(c))
	respondList(c, res, meta, err)
}

func (ctl *Controller) AssignedReviews(c *gin.Context) {
	a, okA := actorFrom(c)
	if !okA {
		return
	}
	res, meta, err := ctl.reviews.ListAssigned(c.Request.Context(), a, c.Query("status"), page(c))
	respondList(c, res, meta, err)
}

func (ctl *Controller) MyReviews(c *gin.Context) {
	a, okA := actorFrom(c)
	if !okA {
		return
	}
	cycle, okC := queryUUID(c, "cycle_id")
	if !okC {
		return
	}
	res, err := ctl.reviews.ListMine(c.Request.Context(), a, cycle)
	respond(c, http.StatusOK, res, err)
}

func (ctl *Controller) GetReview(c *gin.Context) {
	a, okA := actorFrom(c)
	id, okID := pathIDIf(c, okA)
	if !okID {
		return
	}
	res, err := ctl.reviews.Get(c.Request.Context(), a, id)
	respond(c, http.StatusOK, res, err)
}

func (ctl *Controller) SubmitReview(c *gin.Context) {
	a, okA := actorFrom(c)
	id, okID := pathIDIf(c, okA)
	var req dto.SubmitReviewRequest
	if !okID || !bindJSON(c, &req) {
		return
	}
	res, err := ctl.reviews.Submit(c.Request.Context(), a, id, req)
	respond(c, http.StatusOK, res, err)
}

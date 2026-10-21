package controllers

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/hr/core/dto"
	"github.com/jaas/jaas/internal/hr/core/repositories"
	"github.com/jaas/jaas/internal/hr/core/services"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
)

// ArchitectureService is the job-architecture surface (CO-001..CO-003).
type ArchitectureService interface {
	CreateFamily(ctx context.Context, a services.Actor, req dto.CreateFamilyRequest) (*dto.FamilyResponse, error)
	Families(ctx context.Context, tenantID uuid.UUID, status string, page dto.Page) ([]dto.FamilyResponse, dto.PageMeta, error)
	UpdateFamily(ctx context.Context, a services.Actor, id uuid.UUID, req dto.UpdateArchitectureRequest) (*dto.FamilyResponse, error)
	CreateGrade(ctx context.Context, a services.Actor, req dto.CreateGradeRequest) (*dto.GradeResponse, error)
	Grades(ctx context.Context, tenantID uuid.UUID, status string, page dto.Page) ([]dto.GradeResponse, dto.PageMeta, error)
	UpdateGrade(ctx context.Context, a services.Actor, id uuid.UUID, req dto.UpdateArchitectureRequest) (*dto.GradeResponse, error)
	CreateProfile(ctx context.Context, a services.Actor, req dto.CreateProfileRequest) (*dto.ProfileResponse, error)
	Profiles(ctx context.Context, tenantID uuid.UUID, f repositories.ProfileFilter, page dto.Page) ([]dto.ProfileResponse, dto.PageMeta, error)
	Profile(ctx context.Context, tenantID, id uuid.UUID) (*dto.ProfileResponse, error)
	UpdateProfile(ctx context.Context, a services.Actor, id uuid.UUID, req dto.UpdateArchitectureRequest) (*dto.ProfileResponse, error)
}

// PositionService is the position surface (CO-010..CO-014).
type PositionService interface {
	Create(ctx context.Context, a services.Actor, req dto.CreatePositionRequest) (*dto.PositionResponse, error)
	Get(ctx context.Context, tenantID, id uuid.UUID, asOf string) (*dto.PositionResponse, error)
	List(ctx context.Context, tenantID uuid.UUID, f repositories.PositionFilter, vacant *bool, asOf string, page dto.Page) ([]dto.PositionResponse, dto.PageMeta, error)
	Update(ctx context.Context, a services.Actor, id uuid.UUID, req dto.UpdatePositionRequest) (*dto.PositionResponse, error)
	SetStatus(ctx context.Context, a services.Actor, id uuid.UUID, to string) (*dto.PositionResponse, error)
}

// ActionService is the HR-action surface (CO-030..CO-039).
type ActionService interface {
	Create(ctx context.Context, a services.Actor, req dto.CreateActionRequest) (*dto.ActionResponse, error)
	Get(ctx context.Context, tenantID, id uuid.UUID) (*dto.ActionResponse, error)
	List(ctx context.Context, tenantID uuid.UUID, f repositories.ActionFilter, page dto.Page) ([]dto.ActionResponse, dto.PageMeta, error)
	Update(ctx context.Context, a services.Actor, id uuid.UUID, req dto.UpdateActionRequest) (*dto.ActionResponse, error)
	Submit(ctx context.Context, a services.Actor, id uuid.UUID) (*dto.ActionResponse, error)
	Approve(ctx context.Context, a services.Actor, id uuid.UUID, req dto.DecisionRequest) (*dto.ActionResponse, error)
	Reject(ctx context.Context, a services.Actor, id uuid.UUID, req dto.DecisionRequest) (*dto.ActionResponse, error)
	Cancel(ctx context.Context, a services.Actor, id uuid.UUID) (*dto.ActionResponse, error)
	Rescind(ctx context.Context, a services.Actor, id uuid.UUID, req dto.DecisionRequest) (*dto.ActionResponse, error)
}

// WorkerService is the worker, headcount and back-fill surface (CO-021, CO-022, CO-042, CO-050, CO-051).
type WorkerService interface {
	Worker(ctx context.Context, tenantID, employeeID uuid.UUID, asOf string) (*dto.WorkerResponse, error)
	Me(ctx context.Context, a services.Actor, asOf string) (*dto.WorkerResponse, error)
	Assignments(ctx context.Context, tenantID uuid.UUID, asOf string, f repositories.AssignmentFilter, page dto.Page) ([]dto.AssignmentResponse, dto.PageMeta, error)
	CreatePlan(ctx context.Context, a services.Actor, req dto.CreatePlanRequest) (*dto.PlanResponse, error)
	Plans(ctx context.Context, tenantID uuid.UUID, dept *uuid.UUID, page dto.Page) ([]dto.PlanResponse, dto.PageMeta, error)
	Headcount(ctx context.Context, tenantID uuid.UUID, dept *uuid.UUID, asOf string) (*dto.HeadcountResponse, error)
	Backfill(ctx context.Context, a services.Actor) (*dto.RunResponse, error)
}

// EffectsService runs the effective-date sweep (CO-041).
type EffectsService interface {
	Sweep(ctx context.Context, tenant *uuid.UUID) (*dto.RunResponse, error)
}

// Services bundles the controller's collaborators.
type Services struct {
	Architecture ArchitectureService
	Positions    PositionService
	Actions      ActionService
	Workers      WorkerService
	Effects      EffectsService
}

// Controller serves all 32 HR/Core operations.
type Controller struct{ s Services }

// New builds the Controller.
func New(s Services) *Controller { return &Controller{s: s} }

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

// create binds a body and calls fn for the 201 create endpoints.
func create[Req any, Res any](c *gin.Context, fn func(context.Context, services.Actor, Req) (Res, error)) {
	a, okA := actorFrom(c)
	var req Req
	if !okA || !bindJSON(c, &req) {
		return
	}
	res, err := fn(c.Request.Context(), a, req)
	respond(c, http.StatusCreated, res, err)
}

// patch binds a body and calls fn for :id endpoints answering 200.
func patch[Req any, Res any](c *gin.Context, fn func(context.Context, services.Actor, uuid.UUID, Req) (Res, error)) {
	a, okA := actorFrom(c)
	id, okID := pathIDIf(c, okA)
	var req Req
	if !okID || !bindJSON(c, &req) {
		return
	}
	res, err := fn(c.Request.Context(), a, id, req)
	respond(c, http.StatusOK, res, err)
}

// optionalBody binds a JSON body when one was sent (decisions may omit it).
func optionalBody(c *gin.Context, dst interface{}) bool {
	if c.Request.ContentLength == 0 {
		return true
	}
	return bindJSON(c, dst)
}

// --- job architecture ---

func (ctl *Controller) CreateFamily(c *gin.Context)  { create(c, ctl.s.Architecture.CreateFamily) }
func (ctl *Controller) UpdateFamily(c *gin.Context)  { patch(c, ctl.s.Architecture.UpdateFamily) }
func (ctl *Controller) CreateGrade(c *gin.Context)   { create(c, ctl.s.Architecture.CreateGrade) }
func (ctl *Controller) UpdateGrade(c *gin.Context)   { patch(c, ctl.s.Architecture.UpdateGrade) }
func (ctl *Controller) CreateProfile(c *gin.Context) { create(c, ctl.s.Architecture.CreateProfile) }
func (ctl *Controller) UpdateProfile(c *gin.Context) { patch(c, ctl.s.Architecture.UpdateProfile) }

func (ctl *Controller) ListFamilies(c *gin.Context) {
	if a, ok := actorFrom(c); ok {
		res, meta, err := ctl.s.Architecture.Families(c.Request.Context(), a.TenantID, c.Query("status"), page(c))
		respondList(c, res, meta, err)
	}
}

func (ctl *Controller) ListGrades(c *gin.Context) {
	if a, ok := actorFrom(c); ok {
		res, meta, err := ctl.s.Architecture.Grades(c.Request.Context(), a.TenantID, c.Query("status"), page(c))
		respondList(c, res, meta, err)
	}
}

func (ctl *Controller) ListProfiles(c *gin.Context) {
	a, ok := actorFrom(c)
	if !ok {
		return
	}
	fam, okF := queryUUID(c, "family_id")
	if !okF {
		return
	}
	res, meta, err := ctl.s.Architecture.Profiles(c.Request.Context(), a.TenantID,
		repositories.ProfileFilter{FamilyID: fam, Status: c.Query("status"), Category: c.Query("category")}, page(c))
	respondList(c, res, meta, err)
}

func (ctl *Controller) GetProfile(c *gin.Context) {
	a, okA := actorFrom(c)
	if id, ok := pathIDIf(c, okA); ok {
		res, err := ctl.s.Architecture.Profile(c.Request.Context(), a.TenantID, id)
		respond(c, http.StatusOK, res, err)
	}
}

// --- positions ---

func (ctl *Controller) CreatePosition(c *gin.Context) { create(c, ctl.s.Positions.Create) }
func (ctl *Controller) UpdatePosition(c *gin.Context) { patch(c, ctl.s.Positions.Update) }

func (ctl *Controller) GetPosition(c *gin.Context) {
	a, okA := actorFrom(c)
	if id, ok := pathIDIf(c, okA); ok {
		res, err := ctl.s.Positions.Get(c.Request.Context(), a.TenantID, id, c.Query("as_of"))
		respond(c, http.StatusOK, res, err)
	}
}

func (ctl *Controller) ListPositions(c *gin.Context) {
	a, ok := actorFrom(c)
	if !ok {
		return
	}
	dept, okD := queryUUID(c, "department_id")
	prof, okP := queryUUID(c, "job_profile_id")
	if !okD || !okP {
		return
	}
	var vacant *bool
	switch c.Query("vacant") {
	case "true":
		v := true
		vacant = &v
	case "false":
		v := false
		vacant = &v
	}
	res, meta, err := ctl.s.Positions.List(c.Request.Context(), a.TenantID,
		repositories.PositionFilter{DepartmentID: dept, JobProfileID: prof, Status: c.Query("status")}, vacant, c.Query("as_of"), page(c))
	respondList(c, res, meta, err)
}

func (ctl *Controller) SetPositionStatus(c *gin.Context) {
	patch(c, func(ctx context.Context, a services.Actor, id uuid.UUID, req dto.PositionStatusRequest) (*dto.PositionResponse, error) {
		return ctl.s.Positions.SetStatus(ctx, a, id, req.Status)
	})
}

// --- actions ---

func (ctl *Controller) CreateAction(c *gin.Context) { create(c, ctl.s.Actions.Create) }
func (ctl *Controller) UpdateAction(c *gin.Context) { patch(c, ctl.s.Actions.Update) }

func (ctl *Controller) GetAction(c *gin.Context) {
	a, okA := actorFrom(c)
	if id, ok := pathIDIf(c, okA); ok {
		res, err := ctl.s.Actions.Get(c.Request.Context(), a.TenantID, id)
		respond(c, http.StatusOK, res, err)
	}
}

func (ctl *Controller) ListActions(c *gin.Context) {
	a, ok := actorFrom(c)
	if !ok {
		return
	}
	emp, okE := queryUUID(c, "employee_id")
	if !okE {
		return
	}
	res, meta, err := ctl.s.Actions.List(c.Request.Context(), a.TenantID,
		repositories.ActionFilter{EmployeeID: emp, Status: c.Query("status"), Type: c.Query("type")}, page(c))
	respondList(c, res, meta, err)
}

// act runs a body-less (or optional-body) action command.
func (ctl *Controller) act(c *gin.Context, fn func(context.Context, services.Actor, uuid.UUID, dto.DecisionRequest) (*dto.ActionResponse, error)) {
	a, okA := actorFrom(c)
	id, okID := pathIDIf(c, okA)
	var req dto.DecisionRequest
	if !okID || !optionalBody(c, &req) {
		return
	}
	res, err := fn(c.Request.Context(), a, id, req)
	respond(c, http.StatusOK, res, err)
}

func (ctl *Controller) SubmitAction(c *gin.Context) {
	ctl.act(c, func(ctx context.Context, a services.Actor, id uuid.UUID, _ dto.DecisionRequest) (*dto.ActionResponse, error) {
		return ctl.s.Actions.Submit(ctx, a, id)
	})
}
func (ctl *Controller) ApproveAction(c *gin.Context) { ctl.act(c, ctl.s.Actions.Approve) }
func (ctl *Controller) RejectAction(c *gin.Context)  { ctl.act(c, ctl.s.Actions.Reject) }
func (ctl *Controller) RescindAction(c *gin.Context) { ctl.act(c, ctl.s.Actions.Rescind) }
func (ctl *Controller) CancelAction(c *gin.Context) {
	ctl.act(c, func(ctx context.Context, a services.Actor, id uuid.UUID, _ dto.DecisionRequest) (*dto.ActionResponse, error) {
		return ctl.s.Actions.Cancel(ctx, a, id)
	})
}

// --- workers, assignments, headcount ---

func (ctl *Controller) Worker(c *gin.Context) {
	a, okA := actorFrom(c)
	if !okA {
		return
	}
	id, err := uuid.Parse(c.Param("employee_id"))
	if err != nil {
		fail(c, &sharedErrors.AppError{Code: sharedErrors.ErrValidation.Code, Message: "employee_id must be a UUID", StatusCode: http.StatusBadRequest})
		return
	}
	res, err := ctl.s.Workers.Worker(c.Request.Context(), a.TenantID, id, c.Query("as_of"))
	respond(c, http.StatusOK, res, err)
}

func (ctl *Controller) MyWork(c *gin.Context) {
	if a, ok := actorFrom(c); ok {
		res, err := ctl.s.Workers.Me(c.Request.Context(), a, c.Query("as_of"))
		respond(c, http.StatusOK, res, err)
	}
}

func (ctl *Controller) Assignments(c *gin.Context) {
	a, ok := actorFrom(c)
	if !ok {
		return
	}
	dept, okD := queryUUID(c, "department_id")
	pos, okP := queryUUID(c, "position_id")
	if !okD || !okP {
		return
	}
	res, meta, err := ctl.s.Workers.Assignments(c.Request.Context(), a.TenantID, c.Query("as_of"),
		repositories.AssignmentFilter{DepartmentID: dept, PositionID: pos}, page(c))
	respondList(c, res, meta, err)
}

func (ctl *Controller) CreatePlan(c *gin.Context) { create(c, ctl.s.Workers.CreatePlan) }

func (ctl *Controller) ListPlans(c *gin.Context) {
	a, ok := actorFrom(c)
	if !ok {
		return
	}
	if dept, okD := queryUUID(c, "department_id"); okD {
		res, meta, err := ctl.s.Workers.Plans(c.Request.Context(), a.TenantID, dept, page(c))
		respondList(c, res, meta, err)
	}
}

func (ctl *Controller) Headcount(c *gin.Context) {
	a, ok := actorFrom(c)
	if !ok {
		return
	}
	if dept, okD := queryUUID(c, "department_id"); okD {
		res, err := ctl.s.Workers.Headcount(c.Request.Context(), a.TenantID, dept, c.Query("as_of"))
		respond(c, http.StatusOK, res, err)
	}
}

func (ctl *Controller) Backfill(c *gin.Context) {
	if a, ok := actorFrom(c); ok {
		res, err := ctl.s.Workers.Backfill(c.Request.Context(), a)
		respond(c, http.StatusOK, res, err)
	}
}

func (ctl *Controller) RunEffects(c *gin.Context) {
	if a, ok := actorFrom(c); ok {
		res, err := ctl.s.Effects.Sweep(c.Request.Context(), &a.TenantID)
		respond(c, http.StatusOK, res, err)
	}
}

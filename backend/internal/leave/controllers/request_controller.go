package controllers

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/leave/dto"
	"github.com/jaas/jaas/internal/leave/services"
)

// RequestController serves the leave request lifecycle (R1–R8).
type RequestController struct{ svc services.RequestService }

// NewRequestController builds a RequestController.
func NewRequestController(s services.RequestService) *RequestController {
	return &RequestController{svc: s}
}

// Preview handles POST /leave/requests/preview (R1).
func (ctl *RequestController) Preview(c *gin.Context) {
	a, okA := actorFrom(c)
	var req dto.ApplyLeaveRequest
	if !okA || !bindJSON(c, &req) {
		return
	}
	res, err := ctl.svc.Preview(c.Request.Context(), a, req)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, http.StatusOK, res)
}

// Apply handles POST /leave/requests (R2); the employee is the caller (NFR-SEC002).
func (ctl *RequestController) Apply(c *gin.Context) {
	a, okA := actorFrom(c)
	var req dto.ApplyLeaveRequest
	if !okA || !bindJSON(c, &req) {
		return
	}
	res, err := ctl.svc.Apply(c.Request.Context(), a, req)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, http.StatusCreated, res)
}

// ListMine handles GET /leave/requests/me (R3).
func (ctl *RequestController) ListMine(c *gin.Context) {
	a, okA := actorFrom(c)
	if !okA {
		return
	}
	res, meta, err := ctl.svc.ListMine(c.Request.Context(), a, c.Query("status"), page(c))
	if err != nil {
		fail(c, err)
		return
	}
	okList(c, res, meta)
}

// List handles GET /leave/requests (R5).
func (ctl *RequestController) List(c *gin.Context) {
	a, okA := actorFrom(c)
	if !okA {
		return
	}
	q := services.RequestQuery{EmployeeID: c.Query("employee_id"), Status: c.Query("status"), From: c.Query("from"), To: c.Query("to")}
	res, meta, err := ctl.svc.List(c.Request.Context(), a.TenantID, q, page(c))
	if err != nil {
		fail(c, err)
		return
	}
	okList(c, res, meta)
}

// Get handles GET /leave/requests/:id (R6).
func (ctl *RequestController) Get(c *gin.Context) {
	a, okA := actorFrom(c)
	if !okA {
		return
	}
	id, okID := pathID(c, "id")
	if !okID {
		return
	}
	res, err := ctl.svc.Get(c.Request.Context(), a.TenantID, id)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, http.StatusOK, res)
}

type reqAction func(ctx context.Context, a services.Actor, id uuid.UUID, req dto.ReviewRequest) (*dto.LeaveRequestResponse, error)

// act runs an id-addressed action with an optional review body.
func (ctl *RequestController) act(c *gin.Context, fn reqAction, withBody bool) {
	a, okA := actorFrom(c)
	if !okA {
		return
	}
	id, okID := pathID(c, "id")
	if !okID {
		return
	}
	var req dto.ReviewRequest
	if withBody && c.Request.ContentLength != 0 && !bindJSON(c, &req) {
		return
	}
	res, err := fn(c.Request.Context(), a, id, req)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, http.StatusOK, res)
}

// Cancel handles POST /leave/requests/:id/cancel (R4).
func (ctl *RequestController) Cancel(c *gin.Context) {
	ctl.act(c, func(ctx context.Context, a services.Actor, id uuid.UUID, _ dto.ReviewRequest) (*dto.LeaveRequestResponse, error) {
		return ctl.svc.Cancel(ctx, a, id)
	}, false)
}

// Approve handles POST /leave/requests/:id/approve (R7).
func (ctl *RequestController) Approve(c *gin.Context) { ctl.act(c, ctl.svc.Approve, true) }

// Reject handles POST /leave/requests/:id/reject (R8).
func (ctl *RequestController) Reject(c *gin.Context) { ctl.act(c, ctl.svc.Reject, true) }

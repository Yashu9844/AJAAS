package controllers

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/attendance/dto"
	"github.com/jaas/jaas/internal/attendance/services"
)

// RegularizationController serves correction requests (A4–A6, A10–A12).
type RegularizationController struct {
	svc services.RegularizationService
}

// NewRegularizationController builds a RegularizationController.
func NewRegularizationController(s services.RegularizationService) *RegularizationController {
	return &RegularizationController{svc: s}
}

// Create handles POST /attendance/regularizations (A4).
func (ctl *RegularizationController) Create(c *gin.Context) {
	a, okA := actorFrom(c)
	var req dto.CreateRegularizationRequest
	if !okA || !bindJSON(c, &req) {
		return
	}
	res, err := ctl.svc.Create(c.Request.Context(), a, req)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, http.StatusCreated, res)
}

// ListMine handles GET /attendance/regularizations/me (A5).
func (ctl *RegularizationController) ListMine(c *gin.Context) {
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

// List handles GET /attendance/regularizations (A10).
func (ctl *RegularizationController) List(c *gin.Context) {
	a, okA := actorFrom(c)
	if !okA {
		return
	}
	res, meta, err := ctl.svc.List(c.Request.Context(), a.TenantID, c.Query("status"), c.Query("employee_id"), page(c))
	if err != nil {
		fail(c, err)
		return
	}
	okList(c, res, meta)
}

type regAction func(ctx context.Context, a services.Actor, id uuid.UUID, req dto.ReviewRequest) (*dto.RegularizationResponse, error)

// act runs an id-addressed action with an optional review body.
func (ctl *RegularizationController) act(c *gin.Context, fn regAction, withBody bool) {
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

// Cancel handles POST /attendance/regularizations/:id/cancel (A6).
func (ctl *RegularizationController) Cancel(c *gin.Context) {
	ctl.act(c, func(ctx context.Context, a services.Actor, id uuid.UUID, _ dto.ReviewRequest) (*dto.RegularizationResponse, error) {
		return ctl.svc.Cancel(ctx, a, id)
	}, false)
}

// Approve handles POST /attendance/regularizations/:id/approve (A11).
func (ctl *RegularizationController) Approve(c *gin.Context) { ctl.act(c, ctl.svc.Approve, true) }

// Reject handles POST /attendance/regularizations/:id/reject (A12).
func (ctl *RegularizationController) Reject(c *gin.Context) { ctl.act(c, ctl.svc.Reject, true) }

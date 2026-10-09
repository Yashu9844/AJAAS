package controllers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jaas/jaas/internal/attendance/dto"
	"github.com/jaas/jaas/internal/attendance/services"
)

// ShiftController serves shifts and assignments (S1–S7).
type ShiftController struct {
	shifts  services.ShiftService
	assigns services.AssignmentService
}

// NewShiftController builds a ShiftController.
func NewShiftController(s services.ShiftService, a services.AssignmentService) *ShiftController {
	return &ShiftController{shifts: s, assigns: a}
}

// Create handles POST /shifts (S1).
func (ctl *ShiftController) Create(c *gin.Context) {
	a, okA := actorFrom(c)
	var req dto.CreateShiftRequest
	if !okA || !bindJSON(c, &req) {
		return
	}
	res, err := ctl.shifts.Create(c.Request.Context(), a, req)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, http.StatusCreated, res)
}

// List handles GET /shifts (S2).
func (ctl *ShiftController) List(c *gin.Context) {
	a, okA := actorFrom(c)
	if !okA {
		return
	}
	res, meta, err := ctl.shifts.List(c.Request.Context(), a.TenantID, c.Query("status"), page(c))
	if err != nil {
		fail(c, err)
		return
	}
	okList(c, res, meta)
}

// Get handles GET /shifts/:id (S3).
func (ctl *ShiftController) Get(c *gin.Context) {
	a, okA := actorFrom(c)
	if !okA {
		return
	}
	id, okID := pathID(c, "id")
	if !okID {
		return
	}
	res, err := ctl.shifts.Get(c.Request.Context(), a.TenantID, id)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, http.StatusOK, res)
}

// Update handles PATCH /shifts/:id (S4).
func (ctl *ShiftController) Update(c *gin.Context) {
	a, okA := actorFrom(c)
	if !okA {
		return
	}
	id, okID := pathID(c, "id")
	var req dto.UpdateShiftRequest
	if !okID || !bindJSON(c, &req) {
		return
	}
	res, err := ctl.shifts.Update(c.Request.Context(), a, id, req)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, http.StatusOK, res)
}

// Deactivate handles POST /shifts/:id/deactivate (S5).
func (ctl *ShiftController) Deactivate(c *gin.Context) {
	a, okA := actorFrom(c)
	if !okA {
		return
	}
	id, okID := pathID(c, "id")
	if !okID {
		return
	}
	res, err := ctl.shifts.Deactivate(c.Request.Context(), a, id)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, http.StatusOK, res)
}

// Assign handles POST /shifts/:id/assignments (S6).
func (ctl *ShiftController) Assign(c *gin.Context) {
	a, okA := actorFrom(c)
	if !okA {
		return
	}
	id, okID := pathID(c, "id")
	var req dto.AssignShiftRequest
	if !okID || !bindJSON(c, &req) {
		return
	}
	res, err := ctl.assigns.Assign(c.Request.Context(), a, id, req)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, http.StatusCreated, res)
}

// ListAssignments handles GET /shift-assignments?employee_id= (S7).
func (ctl *ShiftController) ListAssignments(c *gin.Context) {
	a, okA := actorFrom(c)
	if !okA {
		return
	}
	res, meta, err := ctl.assigns.List(c.Request.Context(), a.TenantID, c.Query("employee_id"), page(c))
	if err != nil {
		fail(c, err)
		return
	}
	okList(c, res, meta)
}

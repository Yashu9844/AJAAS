package controllers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jaas/jaas/internal/leave/dto"
	"github.com/jaas/jaas/internal/leave/services"
)

// PolicyController serves leave types and holidays (L1–L5, H1–H3).
type PolicyController struct {
	types    services.TypeService
	holidays services.HolidayService
}

// NewPolicyController builds a PolicyController.
func NewPolicyController(t services.TypeService, h services.HolidayService) *PolicyController {
	return &PolicyController{types: t, holidays: h}
}

// CreateType handles POST /leave/types (L1).
func (ctl *PolicyController) CreateType(c *gin.Context) {
	a, okA := actorFrom(c)
	var req dto.CreateLeaveTypeRequest
	if !okA || !bindJSON(c, &req) {
		return
	}
	res, err := ctl.types.Create(c.Request.Context(), a, req)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, http.StatusCreated, res)
}

// ListTypes handles GET /leave/types (L2).
func (ctl *PolicyController) ListTypes(c *gin.Context) {
	a, okA := actorFrom(c)
	if !okA {
		return
	}
	res, meta, err := ctl.types.List(c.Request.Context(), a.TenantID, c.Query("status"), page(c))
	if err != nil {
		fail(c, err)
		return
	}
	okList(c, res, meta)
}

// GetType handles GET /leave/types/:id (L3).
func (ctl *PolicyController) GetType(c *gin.Context) {
	a, okA := actorFrom(c)
	if !okA {
		return
	}
	id, okID := pathID(c, "id")
	if !okID {
		return
	}
	res, err := ctl.types.Get(c.Request.Context(), a.TenantID, id)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, http.StatusOK, res)
}

// UpdateType handles PATCH /leave/types/:id (L4).
func (ctl *PolicyController) UpdateType(c *gin.Context) {
	a, okA := actorFrom(c)
	if !okA {
		return
	}
	id, okID := pathID(c, "id")
	var req dto.UpdateLeaveTypeRequest
	if !okID || !bindJSON(c, &req) {
		return
	}
	res, err := ctl.types.Update(c.Request.Context(), a, id, req)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, http.StatusOK, res)
}

// DeactivateType handles POST /leave/types/:id/deactivate (L5).
func (ctl *PolicyController) DeactivateType(c *gin.Context) {
	a, okA := actorFrom(c)
	if !okA {
		return
	}
	id, okID := pathID(c, "id")
	if !okID {
		return
	}
	res, err := ctl.types.Deactivate(c.Request.Context(), a, id)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, http.StatusOK, res)
}

// CreateHoliday handles POST /leave/holidays (H1).
func (ctl *PolicyController) CreateHoliday(c *gin.Context) {
	a, okA := actorFrom(c)
	var req dto.CreateHolidayRequest
	if !okA || !bindJSON(c, &req) {
		return
	}
	res, err := ctl.holidays.Create(c.Request.Context(), a, req)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, http.StatusCreated, res)
}

// ListHolidays handles GET /leave/holidays?year= (H2).
func (ctl *PolicyController) ListHolidays(c *gin.Context) {
	a, okA := actorFrom(c)
	if !okA {
		return
	}
	res, err := ctl.holidays.List(c.Request.Context(), a.TenantID, c.Query("year"))
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, http.StatusOK, res)
}

// DeleteHoliday handles DELETE /leave/holidays/:id (H3).
func (ctl *PolicyController) DeleteHoliday(c *gin.Context) {
	a, okA := actorFrom(c)
	if !okA {
		return
	}
	id, okID := pathID(c, "id")
	if !okID {
		return
	}
	res, err := ctl.holidays.Delete(c.Request.Context(), a, id)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, http.StatusOK, res)
}

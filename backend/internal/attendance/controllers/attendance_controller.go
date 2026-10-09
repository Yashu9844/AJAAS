package controllers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jaas/jaas/internal/attendance/dto"
	"github.com/jaas/jaas/internal/attendance/services"
)

// AttendanceController serves punches and attendance reads (A1–A3, A7–A9).
type AttendanceController struct {
	punches services.PunchService
	query   services.QueryService
}

// NewAttendanceController builds an AttendanceController.
func NewAttendanceController(p services.PunchService, q services.QueryService) *AttendanceController {
	return &AttendanceController{punches: p, query: q}
}

// Punch handles POST /attendance/punch (A1).
func (ctl *AttendanceController) Punch(c *gin.Context) {
	a, okA := actorFrom(c)
	var req dto.PunchRequest
	if !okA || !bindJSON(c, &req) {
		return
	}
	res, err := ctl.punches.Punch(c.Request.Context(), a, req)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, http.StatusCreated, res)
}

// Today handles GET /attendance/me/today (A2).
func (ctl *AttendanceController) Today(c *gin.Context) {
	a, okA := actorFrom(c)
	if !okA {
		return
	}
	res, err := ctl.query.Today(c.Request.Context(), a)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, http.StatusOK, res)
}

// Mine handles GET /attendance/me (A3); no employee id is accepted (NFR-SEC002).
func (ctl *AttendanceController) Mine(c *gin.Context) {
	a, okA := actorFrom(c)
	if !okA {
		return
	}
	res, err := ctl.query.Mine(c.Request.Context(), a, c.Query("from"), c.Query("to"))
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, http.StatusOK, res)
}

// List handles GET /attendance/records (A7).
func (ctl *AttendanceController) List(c *gin.Context) {
	a, okA := actorFrom(c)
	if !okA {
		return
	}
	q := services.RecordQuery{EmployeeID: c.Query("employee_id"), From: c.Query("from"), To: c.Query("to"), Status: c.Query("status")}
	res, meta, err := ctl.query.List(c.Request.Context(), a.TenantID, q, page(c))
	if err != nil {
		fail(c, err)
		return
	}
	okList(c, res, meta)
}

// Detail handles GET /attendance/records/:id (A8).
func (ctl *AttendanceController) Detail(c *gin.Context) {
	a, okA := actorFrom(c)
	if !okA {
		return
	}
	id, okID := pathID(c, "id")
	if !okID {
		return
	}
	res, err := ctl.query.Detail(c.Request.Context(), a.TenantID, id)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, http.StatusOK, res)
}

// Summary handles GET /attendance/summary (A9).
func (ctl *AttendanceController) Summary(c *gin.Context) {
	a, okA := actorFrom(c)
	if !okA {
		return
	}
	res, err := ctl.query.Summary(c.Request.Context(), a.TenantID, c.Query("date"))
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, http.StatusOK, res)
}

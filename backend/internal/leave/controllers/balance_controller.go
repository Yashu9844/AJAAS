package controllers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jaas/jaas/internal/leave/dto"
	"github.com/jaas/jaas/internal/leave/services"
)

// BalanceController serves balances, adjustments and the ledger (B1–B4).
type BalanceController struct{ svc services.BalanceService }

// NewBalanceController builds a BalanceController.
func NewBalanceController(s services.BalanceService) *BalanceController {
	return &BalanceController{svc: s}
}

// Mine handles GET /leave/balances/me (B1); no employee id is accepted (NFR-SEC002).
func (ctl *BalanceController) Mine(c *gin.Context) {
	a, okA := actorFrom(c)
	if !okA {
		return
	}
	res, err := ctl.svc.Mine(c.Request.Context(), a)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, http.StatusOK, res)
}

// ForEmployee handles GET /leave/balances?employee_id= (B2).
func (ctl *BalanceController) ForEmployee(c *gin.Context) {
	a, okA := actorFrom(c)
	if !okA {
		return
	}
	res, err := ctl.svc.ForEmployee(c.Request.Context(), a, c.Query("employee_id"))
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, http.StatusOK, res)
}

// Adjust handles POST /leave/balances/adjust (B3).
func (ctl *BalanceController) Adjust(c *gin.Context) {
	a, okA := actorFrom(c)
	var req dto.AdjustBalanceRequest
	if !okA || !bindJSON(c, &req) {
		return
	}
	res, err := ctl.svc.Adjust(c.Request.Context(), a, req)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, http.StatusOK, res)
}

// Ledger handles GET /leave/ledger?employee_id=&leave_type_id= (B4).
func (ctl *BalanceController) Ledger(c *gin.Context) {
	a, okA := actorFrom(c)
	if !okA {
		return
	}
	q := services.LedgerQuery{EmployeeID: c.Query("employee_id"), LeaveTypeID: c.Query("leave_type_id")}
	res, meta, err := ctl.svc.Ledger(c.Request.Context(), a.TenantID, q, page(c))
	if err != nil {
		fail(c, err)
		return
	}
	okList(c, res, meta)
}

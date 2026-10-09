package controllers

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/payroll/dto"
	"github.com/jaas/jaas/internal/payroll/services"
)

// RunController serves payroll runs and payslips (P10–P20).
type RunController struct {
	runs     services.RunService
	payslips services.PayslipService
}

// NewRunController builds a RunController.
func NewRunController(r services.RunService, p services.PayslipService) *RunController {
	return &RunController{runs: r, payslips: p}
}

// CreateRun handles POST /payroll/runs (P10).
func (ctl *RunController) CreateRun(c *gin.Context) {
	a, okA := actorFrom(c)
	var req dto.CreateRunRequest
	if !okA || !bindJSON(c, &req) {
		return
	}
	res, err := ctl.runs.Create(c.Request.Context(), a, req)
	respond(c, http.StatusCreated, res, err)
}

// ListRuns handles GET /payroll/runs (P11).
func (ctl *RunController) ListRuns(c *gin.Context) {
	a, okA := actorFrom(c)
	if !okA {
		return
	}
	res, meta, err := ctl.runs.List(c.Request.Context(), a.TenantID, c.Query("status"), page(c))
	respondList(c, res, meta, err)
}

// GetRun handles GET /payroll/runs/:id (P12).
func (ctl *RunController) GetRun(c *gin.Context) {
	a, okA := actorFrom(c)
	id, okID := pathIDIf(c, okA)
	if !okID {
		return
	}
	res, err := ctl.runs.Get(c.Request.Context(), a.TenantID, id)
	respond(c, http.StatusOK, res, err)
}

type runAction func(ctx context.Context, a services.Actor, id uuid.UUID) (*dto.RunResponse, error)

func (ctl *RunController) act(c *gin.Context, fn runAction) {
	a, okA := actorFrom(c)
	id, okID := pathIDIf(c, okA)
	if !okID {
		return
	}
	res, err := fn(c.Request.Context(), a, id)
	respond(c, http.StatusOK, res, err)
}

// Calculate handles POST /payroll/runs/:id/calculate (P13).
func (ctl *RunController) Calculate(c *gin.Context) { ctl.act(c, ctl.runs.Calculate) }

// Approve handles POST /payroll/runs/:id/approve (P14).
func (ctl *RunController) Approve(c *gin.Context) { ctl.act(c, ctl.runs.Approve) }

// Finalize handles POST /payroll/runs/:id/finalize (P15).
func (ctl *RunController) Finalize(c *gin.Context) { ctl.act(c, ctl.runs.Finalize) }

// RunPayslips handles GET /payroll/runs/:id/payslips (P16).
func (ctl *RunController) RunPayslips(c *gin.Context) {
	a, okA := actorFrom(c)
	id, okID := pathIDIf(c, okA)
	if !okID {
		return
	}
	res, meta, err := ctl.payslips.ByRun(c.Request.Context(), a.TenantID, id, page(c))
	respondList(c, res, meta, err)
}

// PayoutCSV handles GET /payroll/runs/:id/payout.csv (P17).
func (ctl *RunController) PayoutCSV(c *gin.Context) {
	a, okA := actorFrom(c)
	id, okID := pathIDIf(c, okA)
	if !okID {
		return
	}
	body, err := ctl.payslips.PayoutCSV(c.Request.Context(), a.TenantID, id)
	if err != nil {
		fail(c, err)
		return
	}
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="payout-%s.csv"`, id))
	c.Data(http.StatusOK, "text/csv; charset=utf-8", body)
}

// GetPayslip handles GET /payroll/payslips/:id (P18).
func (ctl *RunController) GetPayslip(c *gin.Context) {
	a, okA := actorFrom(c)
	id, okID := pathIDIf(c, okA)
	if !okID {
		return
	}
	res, err := ctl.payslips.Get(c.Request.Context(), a.TenantID, id)
	respond(c, http.StatusOK, res, err)
}

// MyPayslips handles GET /payroll/payslips/me (P19); finalized only.
func (ctl *RunController) MyPayslips(c *gin.Context) {
	a, okA := actorFrom(c)
	if !okA {
		return
	}
	res, meta, err := ctl.payslips.Mine(c.Request.Context(), a, page(c))
	respondList(c, res, meta, err)
}

// MyPayslip handles GET /payroll/payslips/me/:id (P20).
func (ctl *RunController) MyPayslip(c *gin.Context) {
	a, okA := actorFrom(c)
	id, okID := pathIDIf(c, okA)
	if !okID {
		return
	}
	res, err := ctl.payslips.MineOne(c.Request.Context(), a, id)
	respond(c, http.StatusOK, res, err)
}

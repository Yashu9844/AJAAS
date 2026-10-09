// Package routes mounts Module 5 endpoints (specification.md §5) behind Module 0 middleware.
package routes

import (
	"github.com/gin-gonic/gin"
	"github.com/jaas/jaas/internal/payroll/controllers"
)

// Permission actions on resource "payroll" (security.md).
const (
	ActionRead    = "read"
	ActionManage  = "manage"
	ActionApprove = "approve"
)

// Middleware is the Module 0 middleware Module 5 depends on.
type Middleware struct {
	TenantResolver, Authenticate gin.HandlerFunc
	Require                      func(action string) gin.HandlerFunc
}

// Controllers groups Module 5 handlers.
type Controllers struct {
	Setup *controllers.SetupController
	Run   *controllers.RunController
}

// RegisterRoutes mounts all 20 Module 5 operations; self endpoints need authentication only (PY-013).
func RegisterRoutes(rg *gin.RouterGroup, mw Middleware, ctl Controllers) {
	read, manage, approve := mw.Require(ActionRead), mw.Require(ActionManage), mw.Require(ActionApprove)
	p := rg.Group("/payroll", mw.TenantResolver, mw.Authenticate)

	p.POST("/structures", manage, ctl.Setup.CreateStructure)
	p.GET("/structures", read, ctl.Setup.ListStructures)
	p.GET("/structures/:id", read, ctl.Setup.GetStructure)
	p.PATCH("/structures/:id", manage, ctl.Setup.UpdateStructure)
	p.POST("/structures/:id/deactivate", manage, ctl.Setup.DeactivateStructure)

	p.POST("/assignments", manage, ctl.Setup.Assign)
	p.GET("/assignments", read, ctl.Setup.ListAssignments)
	p.GET("/assignments/me", ctl.Setup.MyAssignment)
	p.POST("/assignments/preview", manage, ctl.Setup.Preview)

	p.POST("/runs", manage, ctl.Run.CreateRun)
	p.GET("/runs", read, ctl.Run.ListRuns)
	p.GET("/runs/:id", read, ctl.Run.GetRun)
	p.POST("/runs/:id/calculate", manage, ctl.Run.Calculate)
	p.POST("/runs/:id/approve", approve, ctl.Run.Approve)
	p.POST("/runs/:id/finalize", approve, ctl.Run.Finalize)
	p.GET("/runs/:id/payslips", read, ctl.Run.RunPayslips)
	p.GET("/runs/:id/payout.csv", read, ctl.Run.PayoutCSV)

	p.GET("/payslips/me", ctl.Run.MyPayslips)
	p.GET("/payslips/me/:id", ctl.Run.MyPayslip)
	p.GET("/payslips/:id", read, ctl.Run.GetPayslip)
}

// Package routes mounts Module 4 endpoints (specification.md §5) behind Module 0 middleware.
package routes

import (
	"github.com/gin-gonic/gin"
	"github.com/jaas/jaas/internal/leave/controllers"
)

// Permission actions on resource "leave" (security.md).
const (
	ActionRead    = "read"
	ActionManage  = "manage"
	ActionApprove = "approve"
)

// Middleware is the Module 0 middleware Module 4 depends on (connections C4).
type Middleware struct {
	TenantResolver gin.HandlerFunc
	Authenticate   gin.HandlerFunc
	// Require returns RBAC middleware for leave:<action>.
	Require func(action string) gin.HandlerFunc
}

// Controllers groups Module 4 handlers.
type Controllers struct {
	Policy  *controllers.PolicyController
	Balance *controllers.BalanceController
	Request *controllers.RequestController
}

// RegisterRoutes mounts all 20 Module 4 operations; self endpoints and type/holiday reads need authentication only.
func RegisterRoutes(rg *gin.RouterGroup, mw Middleware, ctl Controllers) {
	read, manage, approve := mw.Require(ActionRead), mw.Require(ActionManage), mw.Require(ActionApprove)
	lv := rg.Group("/leave", mw.TenantResolver, mw.Authenticate)

	lv.POST("/types", manage, ctl.Policy.CreateType)
	lv.GET("/types", ctl.Policy.ListTypes)
	lv.GET("/types/:id", ctl.Policy.GetType)
	lv.PATCH("/types/:id", manage, ctl.Policy.UpdateType)
	lv.POST("/types/:id/deactivate", manage, ctl.Policy.DeactivateType)

	lv.POST("/holidays", manage, ctl.Policy.CreateHoliday)
	lv.GET("/holidays", ctl.Policy.ListHolidays)
	lv.DELETE("/holidays/:id", manage, ctl.Policy.DeleteHoliday)

	lv.GET("/balances/me", ctl.Balance.Mine)
	lv.GET("/balances", read, ctl.Balance.ForEmployee)
	lv.POST("/balances/adjust", manage, ctl.Balance.Adjust)
	lv.GET("/ledger", read, ctl.Balance.Ledger)

	lv.POST("/requests/preview", ctl.Request.Preview)
	lv.POST("/requests", ctl.Request.Apply)
	lv.GET("/requests/me", ctl.Request.ListMine)
	lv.POST("/requests/:id/cancel", ctl.Request.Cancel)
	lv.GET("/requests", read, ctl.Request.List)
	lv.GET("/requests/:id", read, ctl.Request.Get)
	lv.POST("/requests/:id/approve", approve, ctl.Request.Approve)
	lv.POST("/requests/:id/reject", approve, ctl.Request.Reject)
}

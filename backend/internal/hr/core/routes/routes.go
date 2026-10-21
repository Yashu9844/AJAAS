// Package routes mounts HR/Core endpoints (specification.md §4) behind M0 middleware.
package routes

import (
	"github.com/gin-gonic/gin"
	"github.com/jaas/jaas/internal/hr/core/controllers"
)

// Permission actions on resource "core".
const (
	ActionRead    = "read"
	ActionManage  = "manage"
	ActionApprove = "approve"
)

// Middleware is the M0 middleware HR/Core depends on.
type Middleware struct {
	TenantResolver, Authenticate gin.HandlerFunc
	Require                      func(action string) gin.HandlerFunc
}

// RegisterRoutes mounts all 32 operations; only /workers/me is open to any authenticated worker.
func RegisterRoutes(rg *gin.RouterGroup, mw Middleware, ctl *controllers.Controller) {
	read, manage, approve := mw.Require(ActionRead), mw.Require(ActionManage), mw.Require(ActionApprove)
	r := rg.Group("/core", mw.TenantResolver, mw.Authenticate)

	r.POST("/job-families", manage, ctl.CreateFamily)
	r.GET("/job-families", read, ctl.ListFamilies)
	r.PATCH("/job-families/:id", manage, ctl.UpdateFamily)
	r.POST("/grades", manage, ctl.CreateGrade)
	r.GET("/grades", read, ctl.ListGrades)
	r.PATCH("/grades/:id", manage, ctl.UpdateGrade)
	r.POST("/job-profiles", manage, ctl.CreateProfile)
	r.GET("/job-profiles", read, ctl.ListProfiles)
	r.GET("/job-profiles/:id", read, ctl.GetProfile)
	r.PATCH("/job-profiles/:id", manage, ctl.UpdateProfile)

	r.POST("/positions", manage, ctl.CreatePosition)
	r.GET("/positions", read, ctl.ListPositions)
	r.GET("/positions/:id", read, ctl.GetPosition)
	r.PATCH("/positions/:id", manage, ctl.UpdatePosition)
	r.POST("/positions/:id/status", manage, ctl.SetPositionStatus)

	r.POST("/actions", manage, ctl.CreateAction)
	r.GET("/actions", read, ctl.ListActions)
	r.GET("/actions/:id", read, ctl.GetAction)
	r.PATCH("/actions/:id", manage, ctl.UpdateAction)
	r.POST("/actions/:id/submit", manage, ctl.SubmitAction)
	r.POST("/actions/:id/approve", approve, ctl.ApproveAction)
	r.POST("/actions/:id/reject", approve, ctl.RejectAction)
	r.POST("/actions/:id/cancel", manage, ctl.CancelAction)
	r.POST("/actions/:id/rescind", approve, ctl.RescindAction)

	r.GET("/workers/me", ctl.MyWork)
	r.GET("/workers/:employee_id", read, ctl.Worker)
	r.GET("/assignments", read, ctl.Assignments)

	r.POST("/headcount-plans", manage, ctl.CreatePlan)
	r.GET("/headcount-plans", read, ctl.ListPlans)
	r.GET("/headcount", read, ctl.Headcount)

	r.POST("/backfill", manage, ctl.Backfill)
	r.POST("/effects/run", manage, ctl.RunEffects)
}

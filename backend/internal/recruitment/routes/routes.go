// Package routes mounts Module 6 endpoints (specification.md §5) behind Module 0 middleware.
package routes

import (
	"github.com/gin-gonic/gin"
	"github.com/jaas/jaas/internal/recruitment/controllers"
)

// Permission actions on resource "recruitment" (security.md).
const (
	ActionRead   = "read"
	ActionManage = "manage"
	ActionHire   = "hire"
)

// Middleware is the Module 0 middleware Module 6 depends on.
type Middleware struct {
	TenantResolver, Authenticate gin.HandlerFunc
	Require                      func(action string) gin.HandlerFunc
}

// RegisterRoutes mounts all 20 Module 6 operations; interviewer self endpoints need authentication only (NFR-SEC002).
func RegisterRoutes(rg *gin.RouterGroup, mw Middleware, ctl *controllers.Controller) {
	read, manage, hire := mw.Require(ActionRead), mw.Require(ActionManage), mw.Require(ActionHire)
	r := rg.Group("/recruitment", mw.TenantResolver, mw.Authenticate)

	r.POST("/jobs", manage, ctl.CreateJob)
	r.GET("/jobs", read, ctl.ListJobs)
	r.GET("/jobs/:id", read, ctl.GetJob)
	r.PATCH("/jobs/:id", manage, ctl.UpdateJob)
	r.POST("/jobs/:id/status", manage, ctl.SetJobStatus)

	r.POST("/candidates", manage, ctl.CreateCandidate)
	r.GET("/candidates", read, ctl.ListCandidates)
	r.GET("/candidates/:id", read, ctl.GetCandidate)
	r.PATCH("/candidates/:id", manage, ctl.UpdateCandidate)
	r.POST("/candidates/:id/stage", manage, ctl.MoveStage)
	r.POST("/candidates/:id/hire", hire, ctl.Hire)

	r.POST("/interviews", manage, ctl.ScheduleInterview)
	r.GET("/interviews", read, ctl.ListInterviews)
	r.GET("/interviews/me", ctl.MyInterviews)
	r.POST("/interviews/:id/feedback", ctl.Feedback)
	r.POST("/interviews/:id/cancel", manage, ctl.CancelInterview)

	r.POST("/offers", manage, ctl.CreateOffer)
	r.GET("/offers", read, ctl.ListOffers)
	r.POST("/offers/:id/decision", manage, ctl.DecideOffer)
	r.POST("/offers/:id/withdraw", manage, ctl.WithdrawOffer)
}

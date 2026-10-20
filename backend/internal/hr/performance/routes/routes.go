// Package routes mounts HR/Performance endpoints (specification.md §5) behind M0 middleware.
package routes

import (
	"github.com/gin-gonic/gin"
	"github.com/jaas/jaas/internal/hr/performance/controllers"
)

// Permission actions on resource "performance".
const (
	ActionRead      = "read"
	ActionManage    = "manage"
	ActionCalibrate = "calibrate"
)

// Middleware is the M0 middleware HR/Performance depends on.
type Middleware struct {
	TenantResolver, Authenticate gin.HandlerFunc
	Require                      func(action string) gin.HandlerFunc
	// Flag stores whether the caller holds the action at tenant scope under key (never blocks).
	Flag func(action, key string) gin.HandlerFunc
}

// RegisterRoutes mounts all 33 operations. Self, manager and reviewer access is decided per request (PM-008, D7-05),
// so those routes only authenticate and carry the read/manage flags.
func RegisterRoutes(rg *gin.RouterGroup, mw Middleware, ctl *controllers.Controller) {
	read, manage, calibrate := mw.Require(ActionRead), mw.Require(ActionManage), mw.Require(ActionCalibrate)
	p := rg.Group("/performance", mw.TenantResolver, mw.Authenticate,
		mw.Flag(ActionRead, controllers.FlagRead), mw.Flag(ActionManage, controllers.FlagManage))

	p.POST("/cycles", manage, ctl.CreateCycle)
	p.GET("/cycles", ctl.ListCycles)
	p.GET("/cycles/:id", ctl.GetCycle)
	p.PATCH("/cycles/:id", manage, ctl.UpdateCycle)
	p.POST("/cycles/:id/activate", manage, ctl.ActivateCycle)
	p.POST("/cycles/:id/launch-reviews", manage, ctl.LaunchReviews)
	p.POST("/cycles/:id/complete", manage, ctl.CompleteCycle)
	p.GET("/cycles/:id/nine-box", read, ctl.NineBox)
	p.POST("/cycles/:id/calibrations", calibrate, ctl.Calibrate)
	p.GET("/cycles/:id/calibrations", read, ctl.ListCalibrations)

	p.POST("/goals", ctl.CreateGoal)
	p.GET("/goals", ctl.ListGoals)
	p.GET("/goals/me", ctl.MyGoals)
	p.GET("/goals/:id", ctl.GetGoal)
	p.PATCH("/goals/:id", ctl.UpdateGoal)
	p.POST("/goals/:id/status", ctl.SetGoalStatus)
	p.POST("/goals/:id/key-results", ctl.AddKeyResult)
	p.POST("/goals/:id/check-ins", ctl.CheckIn)

	p.POST("/reviews", manage, ctl.AssignReviewer)
	p.GET("/reviews", read, ctl.ListReviews)
	p.GET("/reviews/assigned", ctl.AssignedReviews)
	p.GET("/reviews/me", ctl.MyReviews)
	p.GET("/reviews/:id", ctl.GetReview)
	p.POST("/reviews/:id/submit", ctl.SubmitReview)

	p.POST("/feedback", ctl.GiveFeedback)
	p.GET("/feedback", ctl.FeedbackFor)
	p.GET("/feedback/received", ctl.FeedbackReceived)
	p.GET("/feedback/given", ctl.FeedbackGiven)
	p.GET("/feedback/public", ctl.PublicFeedback)
	p.POST("/feedback/requests", ctl.RequestFeedback)
	p.GET("/feedback/requests/assigned", ctl.AssignedFeedbackRequests)
	p.GET("/feedback/requests/mine", ctl.MyFeedbackRequests)
	p.POST("/feedback/requests/:id/decline", ctl.DeclineFeedbackRequest)
}

// Package routes mounts Module 3 endpoints (specification.md §5) behind Module 0 middleware.
package routes

import (
	"github.com/gin-gonic/gin"
	"github.com/jaas/jaas/internal/attendance/controllers"
)

// Permission actions on resource "attendance" (security.md).
const (
	ActionRead    = "read"
	ActionManage  = "manage"
	ActionApprove = "approve"
)

// Middleware is the Module 0 / shared middleware Module 3 depends on (connections C4).
type Middleware struct {
	TenantResolver gin.HandlerFunc
	Authenticate   gin.HandlerFunc
	// Require returns RBAC middleware for attendance:<action>.
	Require    func(action string) gin.HandlerFunc
	PunchLimit gin.HandlerFunc
}

// Controllers groups Module 3 handlers.
type Controllers struct {
	Attendance     *controllers.AttendanceController
	Regularization *controllers.RegularizationController
	Shift          *controllers.ShiftController
}

// RegisterRoutes mounts all 19 Module 3 routes; self endpoints need only authentication (NFR-SEC002).
func RegisterRoutes(rg *gin.RouterGroup, mw Middleware, ctl Controllers) {
	read, manage, approve := mw.Require(ActionRead), mw.Require(ActionManage), mw.Require(ActionApprove)

	att := rg.Group("/attendance", mw.TenantResolver, mw.Authenticate)
	att.POST("/punch", mw.PunchLimit, ctl.Attendance.Punch)
	att.GET("/me/today", ctl.Attendance.Today)
	att.GET("/me", ctl.Attendance.Mine)
	att.GET("/records", read, ctl.Attendance.List)
	att.GET("/records/:id", read, ctl.Attendance.Detail)
	att.GET("/summary", read, ctl.Attendance.Summary)

	att.POST("/regularizations", ctl.Regularization.Create)
	att.GET("/regularizations/me", ctl.Regularization.ListMine)
	att.GET("/regularizations", read, ctl.Regularization.List)
	att.POST("/regularizations/:id/cancel", ctl.Regularization.Cancel)
	att.POST("/regularizations/:id/approve", approve, ctl.Regularization.Approve)
	att.POST("/regularizations/:id/reject", approve, ctl.Regularization.Reject)

	shifts := rg.Group("/shifts", mw.TenantResolver, mw.Authenticate)
	shifts.POST("", manage, ctl.Shift.Create)
	shifts.GET("", read, ctl.Shift.List)
	shifts.GET("/:id", read, ctl.Shift.Get)
	shifts.PATCH("/:id", manage, ctl.Shift.Update)
	shifts.POST("/:id/deactivate", manage, ctl.Shift.Deactivate)
	shifts.POST("/:id/assignments", manage, ctl.Shift.Assign)

	rg.GET("/shift-assignments", mw.TenantResolver, mw.Authenticate, read, ctl.Shift.ListAssignments)
}

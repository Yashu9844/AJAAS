package collab

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
)

// timerStart is the body of POST /time-entries/timer/start.
type timerStart struct {
	TaskID uuid.UUID `json:"task_id"`
}

func (h handler) timeRoutes(g *gin.RouterGroup) {
	g.GET("/tasks/:id/time", withID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		pr := pageOf(c)
		rows, next, err := h.s.ListTime(c.Request.Context(), a, id, TimeFilter{Page: pr, Mine: c.Query("mine") == "true"})
		kernel.RespondCursor(c, kernel.NewCursorPage(rows, pr.Limit, next), err)
	}))
	g.POST("/tasks/:id/time", withID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		body(c, func(in ManualTimeInput) {
			e, err := h.s.LogTime(c.Request.Context(), a, id, in)
			kernel.Respond(c, http.StatusCreated, e, err)
		})
	}))
	g.PATCH("/time-entries/:id", withID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		body(c, func(in TimeUpdateInput) {
			e, err := h.s.UpdateTime(c.Request.Context(), a, id, in)
			kernel.Respond(c, http.StatusOK, e, err)
		})
	}))
	g.DELETE("/time-entries/:id", withID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		kernel.Respond(c, http.StatusOK, gin.H{"deleted": true}, h.s.DeleteTime(c.Request.Context(), a, id))
	}))
	g.GET("/projects/:id/time/report", withID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		r, err := h.s.TimeReport(c.Request.Context(), a, id, ReportQuery{From: c.Query("from"), To: c.Query("to"), GroupBy: c.Query("group_by")})
		kernel.Respond(c, http.StatusOK, r, err)
	}))
}

// timerRoutes use POST and GET only, so they never meet the PATCH and DELETE /time-entries/:id routes in one gin tree.
func (h handler) timerRoutes(g *gin.RouterGroup) {
	g.POST("/time-entries/timer/start", act(func(c *gin.Context, a api.Actor) {
		body(c, func(in timerStart) {
			e, err := h.s.StartTimer(c.Request.Context(), a, in.TaskID)
			kernel.Respond(c, http.StatusCreated, e, err)
		})
	}))
	g.POST("/time-entries/timer/stop", act(func(c *gin.Context, a api.Actor) {
		e, err := h.s.StopTimer(c.Request.Context(), a)
		kernel.Respond(c, http.StatusOK, e, err)
	}))
	g.GET("/time-entries/timer", act(func(c *gin.Context, a api.Actor) {
		e, err := h.s.RunningTimer(c.Request.Context(), a)
		kernel.Respond(c, http.StatusOK, e, err)
	}))
}

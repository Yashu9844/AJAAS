package planning

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
)

// handler adapts the use cases to gin: it resolves the actor and the :id path parameter.
type handler struct{ s *Service }

func act(fn func(c *gin.Context, a api.Actor)) gin.HandlerFunc {
	return func(c *gin.Context) {
		if a, ok := api.ActorFrom(c); ok {
			fn(c, a)
		}
	}
}

func withID(fn func(c *gin.Context, a api.Actor, id uuid.UUID)) gin.HandlerFunc {
	return act(func(c *gin.Context, a api.Actor) {
		if id, ok := kernel.ParamUUID(c, "id"); ok {
			fn(c, a, id)
		}
	})
}

func body[T any](c *gin.Context, run func(in T)) {
	var in T
	if kernel.Bind(c, &in) {
		run(in)
	}
}

// Register mounts the planning routes on g (already guarded by tenant resolution, authentication and the flag middleware).
func (s *Service) Register(g *gin.RouterGroup) {
	h := handler{s: s}
	h.sprintRoutes(g)
	h.planRoutes(g)
	h.viewRoutes(g)
	h.workRoutes(g)
	h.savedViewRoutes(g)
	h.tokenRoutes(g)
}

// RegisterPublic mounts the one route that has no bearer token: the iCalendar feed authenticated by a per-user token.
// The tenant is resolved from the token row, never from the client; the route is rate limited per IP.
func (s *Service) RegisterPublic(rg *gin.RouterGroup) {
	rg.GET("/calendar/feed.ics", s.limiter, func(c *gin.Context) {
		ics, err := s.FeedFor(c.Request.Context(), c.Query("token"))
		if err != nil {
			kernel.Fail(c, err)
			return
		}
		c.Header("Content-Disposition", `inline; filename="jaas-tasks.ics"`)
		c.Header("Cache-Control", "private, no-store")
		c.Data(http.StatusOK, "text/calendar; charset=utf-8", []byte(ics))
	})
}

func (h handler) sprintRoutes(g *gin.RouterGroup) {
	g.POST("/projects/:id/sprints", withID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		body(c, func(in SprintInput) {
			sp, err := h.s.CreateSprint(c.Request.Context(), a, id, in)
			kernel.Respond(c, http.StatusCreated, sp, err)
		})
	}))
	g.GET("/projects/:id/sprints", withID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		f := SprintFilter{Status: c.Query("status"), Cursor: c.Query("cursor"), Limit: kernel.ClampLimit(c.Query("limit"))}
		rows, next, err := h.s.ListSprints(c.Request.Context(), a, id, f)
		kernel.RespondCursor(c, kernel.NewCursorPage(rows, f.Limit, next), err)
	}))
	g.GET("/sprints/:id", withID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		sp, err := h.s.GetSprint(c.Request.Context(), a, id)
		kernel.Respond(c, http.StatusOK, sp, err)
	}))
	g.PATCH("/sprints/:id", withID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		body(c, func(in SprintInput) {
			if v := kernel.IfMatchVersion(c); v > 0 && in.Version == nil {
				in.Version = &v
			}
			sp, err := h.s.UpdateSprint(c.Request.Context(), a, id, in)
			kernel.Respond(c, http.StatusOK, sp, err)
		})
	}))
	g.DELETE("/sprints/:id", withID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		n, err := h.s.DeleteSprint(c.Request.Context(), a, id)
		kernel.Respond(c, http.StatusOK, gin.H{"deleted": true, "released_tasks": n}, err)
	}))
	h.sprintFlowRoutes(g)
}

func (h handler) sprintFlowRoutes(g *gin.RouterGroup) {
	g.POST("/sprints/:id/start", withID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		sp, err := h.s.StartSprint(c.Request.Context(), a, id)
		kernel.Respond(c, http.StatusOK, sp, err)
	}))
	g.POST("/sprints/:id/complete", withID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		body(c, func(in CompleteInput) {
			sp, err := h.s.CompleteSprint(c.Request.Context(), a, id, in)
			kernel.Respond(c, http.StatusOK, sp, err)
		})
	}))
	g.GET("/sprints/:id/report", withID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		r, err := h.s.SprintReport(c.Request.Context(), a, id)
		kernel.Respond(c, http.StatusOK, r, err)
	}))
}

func (h handler) planRoutes(g *gin.RouterGroup) {
	g.GET("/projects/:id/backlog", withID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		limit := kernel.ClampLimit(c.Query("limit"))
		rows, next, err := h.s.Backlog(c.Request.Context(), a, id, c.Query("cursor"), limit)
		kernel.RespondCursor(c, kernel.NewCursorPage(rows, limit, next), err)
	}))
	g.POST("/projects/:id/plan", withID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		body(c, func(in PlanInput) {
			res, err := h.s.PlanMove(c.Request.Context(), a, id, in)
			kernel.Respond(c, http.StatusOK, res, err)
		})
	}))
}

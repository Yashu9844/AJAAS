package recurrence

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
)

func act(fn func(c *gin.Context, a api.Actor, id uuid.UUID)) gin.HandlerFunc {
	return func(c *gin.Context) {
		a, ok := api.ActorFrom(c)
		if !ok {
			return
		}
		if id, ok := kernel.ParamUUID(c, "id"); ok {
			fn(c, a, id)
		}
	}
}

func onRecurrence(fn func(c *gin.Context, a api.Actor, id, rec uuid.UUID)) gin.HandlerFunc {
	return act(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		if rec, ok := kernel.ParamUUID(c, "recurrence_id"); ok {
			fn(c, a, id, rec)
		}
	})
}

// Register mounts the recurrence routes on g (already guarded by tenant resolution and authentication).
func (s *Service) Register(g *gin.RouterGroup) {
	g.GET("/projects/:id/recurrences", act(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		rows, err := s.List(c.Request.Context(), a, id)
		kernel.Respond(c, http.StatusOK, rows, err)
	}))
	g.POST("/projects/:id/recurrences", act(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		var in Input
		if kernel.Bind(c, &in) {
			r, err := s.Create(c.Request.Context(), a, id, in)
			kernel.Respond(c, http.StatusCreated, r, err)
		}
	}))
	g.PATCH("/projects/:id/recurrences/:recurrence_id", onRecurrence(func(c *gin.Context, a api.Actor, id, rec uuid.UUID) {
		var in Update
		if !kernel.Bind(c, &in) {
			return
		}
		if v := kernel.IfMatchVersion(c); v > 0 && in.Version == nil {
			in.Version = &v
		}
		r, err := s.Patch(c.Request.Context(), a, id, rec, in)
		kernel.Respond(c, http.StatusOK, r, err)
	}))
	g.DELETE("/projects/:id/recurrences/:recurrence_id", onRecurrence(func(c *gin.Context, a api.Actor, id, rec uuid.UUID) {
		kernel.Respond(c, http.StatusOK, gin.H{"deleted": true}, s.Delete(c.Request.Context(), a, id, rec))
	}))
	g.POST("/projects/:id/recurrences/:recurrence_id/run-now", onRecurrence(func(c *gin.Context, a api.Actor, id, rec uuid.UUID) {
		task, err := s.RunNow(c.Request.Context(), a, id, rec)
		kernel.Respond(c, http.StatusCreated, gin.H{"task_id": task}, err)
	}))
}

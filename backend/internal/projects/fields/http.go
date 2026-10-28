package fields

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

// Register mounts the custom-field routes on g (already guarded by tenant resolution and authentication).
func (s *Service) Register(g *gin.RouterGroup) {
	g.GET("/projects/:id/custom-fields", act(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		rows, err := s.Definitions(c.Request.Context(), a, id)
		kernel.Respond(c, http.StatusOK, rows, err)
	}))
	g.POST("/projects/:id/custom-fields", act(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		var in DefinitionInput
		if kernel.Bind(c, &in) {
			d, err := s.CreateDefinition(c.Request.Context(), a, id, in)
			kernel.Respond(c, http.StatusCreated, d, err)
		}
	}))
	g.PATCH("/projects/:id/custom-fields/:field_id", act(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		fid, ok := kernel.ParamUUID(c, "field_id")
		var in DefinitionUpdate
		if ok && kernel.Bind(c, &in) {
			d, err := s.UpdateDefinition(c.Request.Context(), a, id, fid, in)
			kernel.Respond(c, http.StatusOK, d, err)
		}
	}))
	g.DELETE("/projects/:id/custom-fields/:field_id", act(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		if fid, ok := kernel.ParamUUID(c, "field_id"); ok {
			kernel.Respond(c, http.StatusOK, gin.H{"deleted": true}, s.DeleteDefinition(c.Request.Context(), a, id, fid))
		}
	}))
	g.GET("/tasks/:id/custom-fields", act(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		rows, err := s.TaskFields(c.Request.Context(), a, id)
		kernel.Respond(c, http.StatusOK, rows, err)
	}))
	g.PUT("/tasks/:id/custom-fields", act(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		var in SetInput
		if !kernel.Bind(c, &in) {
			return
		}
		if v := kernel.IfMatchVersion(c); v > 0 && in.Version == nil {
			in.Version = &v
		}
		rows, err := s.SetTaskFields(c.Request.Context(), a, id, in)
		kernel.Respond(c, http.StatusOK, rows, err)
	}))
}

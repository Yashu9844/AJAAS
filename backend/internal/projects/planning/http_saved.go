package planning

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
)

func (h handler) savedViewRoutes(g *gin.RouterGroup) {
	g.POST("/saved-views", act(func(c *gin.Context, a api.Actor) {
		body(c, func(in ViewInput) {
			v, err := h.s.CreateView(c.Request.Context(), a, in)
			kernel.Respond(c, http.StatusCreated, v, err)
		})
	}))
	g.GET("/saved-views", act(func(c *gin.Context, a api.Actor) {
		project, ok := kernel.QueryUUID(c, "project_id")
		if !ok {
			return
		}
		f := ViewFilter{ProjectID: project, Cursor: c.Query("cursor"), Limit: kernel.ClampLimit(c.Query("limit"))}
		rows, next, err := h.s.ListViews(c.Request.Context(), a, f)
		kernel.RespondCursor(c, kernel.NewCursorPage(rows, f.Limit, next), err)
	}))
	g.PATCH("/saved-views/:id", withID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		body(c, func(in ViewInput) {
			if v := kernel.IfMatchVersion(c); v > 0 && in.Version == nil {
				in.Version = &v
			}
			v, err := h.s.UpdateView(c.Request.Context(), a, id, in)
			kernel.Respond(c, http.StatusOK, v, err)
		})
	}))
	g.DELETE("/saved-views/:id", withID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		kernel.Respond(c, http.StatusOK, gin.H{"deleted": true}, h.s.DeleteView(c.Request.Context(), a, id))
	}))
}

// TokenInput names a new calendar feed token.
type TokenInput struct {
	Label string `json:"label"`
}

func (h handler) tokenRoutes(g *gin.RouterGroup) {
	g.POST("/calendar-tokens", act(func(c *gin.Context, a api.Actor) {
		var in TokenInput
		if c.Request.ContentLength > 0 && !kernel.Bind(c, &in) {
			return
		}
		t, err := h.s.CreateToken(c.Request.Context(), a, in.Label)
		kernel.Respond(c, http.StatusCreated, t, err)
	}))
	g.GET("/calendar-tokens", act(func(c *gin.Context, a api.Actor) {
		rows, err := h.s.ListTokens(c.Request.Context(), a)
		kernel.Respond(c, http.StatusOK, rows, err)
	}))
	g.DELETE("/calendar-tokens/:id", withID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		kernel.Respond(c, http.StatusOK, gin.H{"revoked": true}, h.s.RevokeToken(c.Request.Context(), a, id))
	}))
}

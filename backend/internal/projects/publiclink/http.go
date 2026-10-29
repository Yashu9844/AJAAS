package publiclink

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

// Register mounts the link management routes on g (already guarded by tenant resolution and authentication).
func (s *Service) Register(g *gin.RouterGroup) {
	g.GET("/projects/:id/public-links", act(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		rows, err := s.List(c.Request.Context(), a, id)
		kernel.Respond(c, http.StatusOK, rows, err)
	}))
	g.POST("/projects/:id/public-links", act(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		var in Input
		if kernel.Bind(c, &in) {
			l, err := s.Create(c.Request.Context(), a, id, in)
			kernel.Respond(c, http.StatusCreated, l, err)
		}
	}))
	g.DELETE("/projects/:id/public-links/:link_id", act(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		if lid, ok := kernel.ParamUUID(c, "link_id"); ok {
			kernel.Respond(c, http.StatusOK, gin.H{"revoked": true}, s.Revoke(c.Request.Context(), a, id, lid))
		}
	}))
}

// noIndex marks every answer of the public route (found or not) as uncacheable, unindexable and referrer-free.
func noIndex(c *gin.Context) {
	h := c.Writer.Header()
	h.Set("Cache-Control", "no-store")
	h.Set("X-Robots-Tag", "noindex")
	h.Set("Referrer-Policy", "no-referrer")
	c.Next()
}

// RegisterPublic mounts the one route without a bearer token: the read-only project page authenticated by the secret in
// its path. The tenant and the project are resolved from the token row, never from the client; the route is rate limited
// per IP and its answers are never cached.
func (s *Service) RegisterPublic(rg *gin.RouterGroup) {
	rg.GET("/public/projects/:token", noIndex, s.limiter, func(c *gin.Context) {
		limit := kernel.ClampLimit(c.Query("limit"))
		p, err := s.View(c.Request.Context(), c.Param("token"), c.Query("cursor"), limit)
		if err != nil {
			kernel.Fail(c, err)
			return
		}
		var cur any
		if p.Next != "" {
			cur = p.Next
		}
		c.JSON(http.StatusOK, gin.H{"data": p, "meta": gin.H{"limit": p.Limit, "next_cursor": cur}})
	})
}

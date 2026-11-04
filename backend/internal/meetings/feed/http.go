package feed

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/meetings/api"
	"github.com/jaas/jaas/internal/meetings/kernel"
)

// Register mounts the token and download routes (bearer token required).
func (s *Service) Register(g *gin.RouterGroup) {
	g.POST("/meeting-feed-tokens", api.Act(func(c *gin.Context, a api.Actor) {
		api.Body(c, func(in TokenInput) {
			v, err := s.CreateToken(c.Request.Context(), a, in)
			kernel.Respond(c, http.StatusCreated, v, err)
		})
	}))
	g.GET("/meeting-feed-tokens", api.Act(func(c *gin.Context, a api.Actor) {
		v, err := s.ListTokens(c.Request.Context(), a)
		kernel.Respond(c, http.StatusOK, v, err)
	}))
	g.DELETE("/meeting-feed-tokens/:id", api.WithID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		kernel.NoContent(c, s.RevokeToken(c.Request.Context(), a, id))
	}))
	g.GET("/occurrences/:id/ics", api.WithID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		body, err := s.Occurrence(c.Request.Context(), a, id)
		writeICS(c, "meeting.ics", body, err)
	}))
	g.GET("/meetings/:id/ics", api.WithID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		body, err := s.Series(c.Request.Context(), a, id)
		writeICS(c, "meeting-series.ics", body, err)
	}))
}

// RegisterPublic mounts the personal feed, authenticated by its token only and rate limited per IP. The tenant comes
// from the token row, never from the client.
func (s *Service) RegisterPublic(rg *gin.RouterGroup, limiter gin.HandlerFunc) {
	rg.GET("/calendar/meetings.ics", limiter, func(c *gin.Context) {
		body, err := s.FeedFor(c.Request.Context(), c.Query("token"))
		writeICS(c, "jaas-meetings.ics", body, err)
	})
}

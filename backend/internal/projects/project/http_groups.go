package project

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
)

func (h handler) groupRoutes(g *gin.RouterGroup) {
	g.GET("/projects/:id/group-rules", withID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		rows, err := h.s.GroupRules(c.Request.Context(), a, id)
		kernel.Respond(c, http.StatusOK, rows, err)
	}))
	g.POST("/projects/:id/group-rules", withID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		body(c, func(in GroupRuleInput) {
			r, err := h.s.AddGroupRule(c.Request.Context(), a, id, in)
			kernel.Respond(c, http.StatusCreated, r, err)
		})
	}))
	g.DELETE("/projects/:id/group-rules/:rule_id", withSub("rule_id", func(c *gin.Context, a api.Actor, id, rid uuid.UUID) {
		kernel.Respond(c, http.StatusOK, gin.H{"deleted": true}, h.s.RemoveGroupRule(c.Request.Context(), a, id, rid))
	}))
	g.POST("/projects/:id/group-rules/refresh", withID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		kernel.Respond(c, http.StatusOK, gin.H{"refreshed": true}, h.s.RefreshProject(c.Request.Context(), a, id))
	}))
}

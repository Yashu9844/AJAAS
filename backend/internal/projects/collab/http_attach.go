package collab

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
)

func (h handler) attachmentRoutes(g *gin.RouterGroup) {
	g.POST("/tasks/:id/attachments", withID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		body(c, func(in InitInput) {
			t, err := h.s.InitAttachment(c.Request.Context(), a, id, in)
			kernel.Respond(c, http.StatusCreated, t, err)
		})
	}))
	g.GET("/tasks/:id/attachments", withID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		pr := pageOf(c)
		rows, next, err := h.s.ListAttachments(c.Request.Context(), a, id, pr)
		kernel.RespondCursor(c, kernel.NewCursorPage(rows, pr.Limit, next), err)
	}))
	g.POST("/attachments/:id/complete", withID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		att, err := h.s.CompleteAttachment(c.Request.Context(), a, id)
		kernel.Respond(c, http.StatusOK, att, err)
	}))
	g.GET("/attachments/:id/download", withID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		l, err := h.s.DownloadAttachment(c.Request.Context(), a, id)
		kernel.Respond(c, http.StatusOK, l, err)
	}))
	g.DELETE("/attachments/:id", withID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		kernel.Respond(c, http.StatusOK, gin.H{"deleted": true}, h.s.DeleteAttachment(c.Request.Context(), a, id))
	}))
}

package projects

import (
	"github.com/gin-gonic/gin"
	"github.com/jaas/jaas/internal/projects/kernel"
)

// agentLabel copies a valid X-JAAS-Agent header into the request context, so every activity entry and event written
// while handling the request is labelled as an AI action (the user permissions still decide what is allowed). An invalid
// label is ignored rather than rejected: it never widens access, it only labels.
func agentLabel() gin.HandlerFunc {
	return func(c *gin.Context) {
		if name := c.GetHeader(kernel.AgentHeader); kernel.ValidAgent(name) {
			c.Request = c.Request.WithContext(kernel.WithAgent(c.Request.Context(), name))
		}
		c.Next()
	}
}

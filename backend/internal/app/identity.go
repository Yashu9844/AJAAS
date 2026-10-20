package app

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/identity"
	identityMiddleware "github.com/jaas/jaas/internal/identity/middleware"
)

// startIdentityBackground runs M0's outbox relay while background work is enabled.
func startIdentityBackground(o Options, id *identity.Module) {
	if o.Background != nil {
		go id.RunEventRelay(o.Background)
	}
}

// registerDevMailbox exposes one-time secrets (reset and invitation tokens) to developers and the test console in
// development and test only, behind the platform key: GET /api/v1/dev/mailbox?tenant_id=…&email=…
func registerDevMailbox(v1 *gin.RouterGroup, o Options, id *identity.Module) {
	if env := o.Config.Server.Env; env != "development" && env != "test" {
		return
	}
	box := id.EnableDevMailbox()
	v1.GET("/dev/mailbox", identityMiddleware.PlatformAdmin(o.Config.Platform.AdminKey), func(c *gin.Context) {
		tenantID, err := uuid.Parse(c.Query("tenant_id"))
		if err != nil || c.Query("email") == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "VALIDATION_ERROR", "message": "tenant_id and email are required"}})
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": box.For(tenantID, c.Query("email"))})
	})
}

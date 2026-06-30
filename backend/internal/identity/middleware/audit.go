package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/identity/services"
	"gorm.io/gorm"
)

// AuditLog captures API request context and logs actions via the AuditService post-execution.
func AuditLog(db *gorm.DB, auditSvc services.AuditService, action, resource string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Process request first
		c.Next()

		status := c.Writer.Status()
		// Only log successful operations (2xx statuses)
		if status < 200 || status >= 300 {
			return
		}

		tenantIDVal, ok1 := c.Get("tenant_id")
		if !ok1 {
			return // skip audit logging if no tenant context resolved
		}
		tenantID := tenantIDVal.(uuid.UUID)

		var userIDStr string
		if userIDVal, ok2 := c.Get("user_id"); ok2 {
			if uID, ok := userIDVal.(uuid.UUID); ok {
				userIDStr = uID.String()
			}
		}

		ipAddress := c.ClientIP()
		userAgent := c.Request.UserAgent()
		resourceID := c.Param("id")

		meta := map[string]interface{}{
			"status_code": status,
			"path":        c.Request.URL.Path,
			"method":      c.Request.Method,
		}

		// Log audit entry (best effort)
		_ = auditSvc.Log(
			c.Request.Context(),
			db,
			tenantID.String(),
			userIDStr,
			action,
			resource,
			resourceID,
			meta,
			ipAddress,
			userAgent,
		)
	}
}

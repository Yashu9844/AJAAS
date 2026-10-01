package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/shared/logger"
)

// RequestLogger outputs details of each incoming HTTP request.
func RequestLogger(log *logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		// Generate request tracking ID if not set
		requestID := c.GetHeader("X-Request-ID")
		if requestID == "" {
			requestID = uuid.New().String()
		}
		c.Header("X-Request-ID", requestID)
		c.Set("request_id", requestID)

		// Process request
		c.Next()

		duration := time.Since(start)
		status := c.Writer.Status()

		tenantIDStr := ""
		if tID, ok := c.Get("tenant_id"); ok {
			tenantIDStr = fmtString(tID)
		}

		userIDStr := ""
		if uID, ok := c.Get("user_id"); ok {
			userIDStr = fmtString(uID)
		}

		log.Info().
			Str("request_id", requestID).
			Str("tenant_id", tenantIDStr).
			Str("user_id", userIDStr).
			Str("method", c.Request.Method).
			Str("path", c.Request.URL.Path).
			Int("status", status).
			Dur("duration_ms", duration).
			Str("ip", c.ClientIP()).
			Msg("Processed HTTP request")
	}
}

func fmtString(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	if uuidVal, ok := v.(uuid.UUID); ok {
		return uuidVal.String()
	}
	return ""
}

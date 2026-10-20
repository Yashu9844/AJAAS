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

		// One validated correlation ID per request: echoed back, stored in the gin and request contexts, and carried
		// into audit rows and events (docs/ARCHITECTURE.md §3.4).
		corr := incomingCorrelation(c.GetHeader(CorrelationHeader), c.GetHeader(RequestIDHeader))
		requestID := corr.String()
		c.Header(CorrelationHeader, requestID)
		c.Header(RequestIDHeader, requestID)
		c.Set("request_id", requestID)
		c.Set("correlation_id", corr)
		c.Request = c.Request.WithContext(WithCorrelationID(c.Request.Context(), corr))

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

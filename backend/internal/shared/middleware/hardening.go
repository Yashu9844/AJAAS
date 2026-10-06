package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// DefaultMaxBodyBytes caps request bodies (JSON APIs here never need more).
const DefaultMaxBodyBytes int64 = 1 << 20 // 1 MiB

// BodyLimit rejects request bodies larger than max with 413 before handlers read them.
func BodyLimit(max int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.ContentLength > max {
			c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, gin.H{"error": gin.H{
				"code":    "PAYLOAD_TOO_LARGE",
				"message": "Request body is too large",
			}})
			return
		}
		// also covers chunked bodies with unknown length
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, max)
		c.Next()
	}
}

// SecurityHeaders sets conservative defaults for an API that never serves HTML.
func SecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.Writer.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cache-Control", "no-store")
		h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		c.Next()
	}
}

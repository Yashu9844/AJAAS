package middleware

import (
	"bytes"
	"errors"
	"io"
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
		// Also covers chunked bodies with unknown length. Those are read here (bounded by max) and given their real
		// length, because handlers decide "is there a body?" from ContentLength and would otherwise silently ignore it.
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, max)
		if c.Request.ContentLength < 0 {
			data, err := io.ReadAll(c.Request.Body)
			if err != nil {
				var tooBig *http.MaxBytesError
				status, code, msg := http.StatusBadRequest, "BAD_REQUEST", "Request body could not be read"
				if errors.As(err, &tooBig) {
					status, code, msg = http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", "Request body is too large"
				}
				c.AbortWithStatusJSON(status, gin.H{"error": gin.H{"code": code, "message": msg}})
				return
			}
			c.Request.ContentLength = int64(len(data))
			if len(data) == 0 {
				c.Request.Body = http.NoBody
			} else {
				c.Request.Body = io.NopCloser(bytes.NewReader(data))
			}
		}
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

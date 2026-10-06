package middleware

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"

	"github.com/gin-gonic/gin"
)

// PlatformKeyHeader carries the platform operator key on tenant-management requests.
const PlatformKeyHeader = "X-Platform-Key"

// PlatformAdmin guards platform-level (cross-tenant) routes such as tenant provisioning.
// It fails closed: with no configured key every request is refused (503), and a wrong/missing key is 401/403.
func PlatformAdmin(key string) gin.HandlerFunc {
	want := sha256.Sum256([]byte(key))
	return func(c *gin.Context) {
		if key == "" {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{
				"code":    "PLATFORM_ADMIN_DISABLED",
				"message": "Platform administration is not configured on this server",
			}})
			return
		}
		got := c.GetHeader(PlatformKeyHeader)
		if got == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": gin.H{
				"code":    "UNAUTHORIZED",
				"message": "Platform admin key is required",
			}})
			return
		}
		have := sha256.Sum256([]byte(got))
		if subtle.ConstantTimeCompare(have[:], want[:]) != 1 {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": gin.H{
				"code":    "FORBIDDEN",
				"message": "Invalid platform admin key",
			}})
			return
		}
		c.Next()
	}
}

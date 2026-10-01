package middleware

import (
	"net"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jaas/jaas/internal/identity/repositories"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"gorm.io/gorm"
)

// TenantResolver parses Host subdomains and binds tenant context to requests.
func TenantResolver(db *gorm.DB, tenantRepo repositories.TenantRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		host := c.Request.Host

		// Strip port if present
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}

		parts := strings.Split(host, ".")
		var slug string
		if len(parts) >= 2 {
			// e.g. "acme.localhost" (len 2) or "acme.jaas.com" (len 3)
			if parts[len(parts)-1] == "localhost" && len(parts) == 2 {
				slug = parts[0]
			} else if len(parts) >= 3 {
				slug = parts[0]
			}
		}

		if slug == "" {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"error": gin.H{
					"code":    "MISSING_SUBDOMAIN",
					"message": "Subdomain host is required to access this resource",
				},
			})
			return
		}

		// Resolve tenant slug in database
		tenant, err := tenantRepo.FindBySlug(c.Request.Context(), db, slug)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error": gin.H{
					"code":    "DATABASE_ERROR",
					"message": "Failed to resolve tenant subdomain context",
				},
			})
			return
		}

		if tenant == nil {
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{
				"error": gin.H{
					"code":    sharedErrors.ErrNotFound.Code,
					"message": "Tenant organization not found",
				},
			})
			return
		}

		// Check suspend status
		if tenant.Status == "suspended" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": gin.H{
					"code":    "TENANT_SUSPENDED",
					"message": "Tenant organization is suspended",
				},
			})
			return
		}

		// Inject resolved tenant ID into context
		c.Set("tenant_id", tenant.ID)
		c.Set("tenant_slug", tenant.Slug)

		c.Next()
	}
}

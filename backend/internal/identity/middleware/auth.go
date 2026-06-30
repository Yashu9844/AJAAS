package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/identity/services"
	"gorm.io/gorm"
)

// Authenticate verifies the Bearer access token and active session.
func Authenticate(db *gorm.DB, tokenSvc services.TokenService, sessionSvc services.SessionService) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": gin.H{
					"code":    "UNAUTHORIZED",
					"message": "Authorization header is required",
				},
			})
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": gin.H{
					"code":    "INVALID_TOKEN_FORMAT",
					"message": "Authorization header must be Bearer token",
				},
			})
			return
		}

		tokenStr := parts[1]

		// 1. Validate JWT
		claims, err := tokenSvc.ValidateAccessToken(tokenStr)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": gin.H{
					"code":    "INVALID_TOKEN",
					"message": "Access token is invalid or expired",
				},
			})
			return
		}

		// 2. Validate Session ID (not blacklisted/revoked)
		sessionID, err := uuid.Parse(claims.SessionID)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": gin.H{
					"code":    "INVALID_SESSION",
					"message": "Invalid session reference inside token",
				},
			})
			return
		}

		isValid, err := sessionSvc.ValidateSession(c.Request.Context(), db, sessionID)
		if err != nil || !isValid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": gin.H{
					"code":    "SESSION_EXPIRED",
					"message": "User session has expired or been revoked",
				},
			})
			return
		}

		// 3. Verify tenant isolation: JWT tid must match Resolved subdomain tenant_id
		tenantIDVal, exists := c.Get("tenant_id")
		if exists {
			resolvedTenantID := tenantIDVal.(uuid.UUID)
			claimsTenantID, err := uuid.Parse(claims.TenantID)
			if err != nil || resolvedTenantID != claimsTenantID {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
					"error": gin.H{
						"code":    "FORBIDDEN",
						"message": "Access to this tenant subdomain is denied",
					},
				})
				return
			}
		}

		userID, _ := uuid.Parse(claims.Subject)

		// Inject verified claims into request context
		c.Set("user_id", userID)
		c.Set("email", claims.Email)
		c.Set("roles", claims.Roles)
		c.Set("session_id", sessionID)

		c.Next()
	}
}

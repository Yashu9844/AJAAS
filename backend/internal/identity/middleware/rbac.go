package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/identity/repositories"
	"gorm.io/gorm"
)

// RequirePermission limits access to requests with a specific resource-action pair.
func RequirePermission(
	db *gorm.DB,
	resource string,
	action string,
	userRoleRepo repositories.UserRoleRepository,
	rolePermRepo repositories.RolePermissionRepository,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		rolesVal, ok := c.Get("roles")
		if !ok {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": gin.H{
					"code":    "FORBIDDEN",
					"message": "User roles are not mapped in request context",
				},
			})
			return
		}

		roles := rolesVal.([]string)
		for _, rName := range roles {
			// tenant_admin system role bypasses permission checks
			if rName == "tenant_admin" {
				c.Next()
				return
			}
		}

		tenantIDVal, ok1 := c.Get("tenant_id")
		userIDVal, ok2 := c.Get("user_id")
		if !ok1 || !ok2 {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": gin.H{
					"code":    "FORBIDDEN",
					"message": "Missing tenant or user context mapping",
				},
			})
			return
		}

		tenantID := tenantIDVal.(uuid.UUID)
		userID := userIDVal.(uuid.UUID)
		ctx := c.Request.Context()

		// Retrieve roles mapping for user
		userRoles, err := userRoleRepo.FindByUserID(ctx, db, tenantID, userID)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error": gin.H{
					"code":    "DATABASE_ERROR",
					"message": "Failed to look up user roles",
				},
			})
			return
		}

		hasPerm := false
		for _, ur := range userRoles {
			rolePerms, err := rolePermRepo.FindByRoleID(ctx, db, tenantID, ur.RoleID)
			if err != nil {
				continue
			}

			// Validate if any permission references the requested resource and action
			for _, rp := range rolePerms {
				if rp.Permission != nil && rp.Permission.Resource == resource && rp.Permission.Action == action {
					hasPerm = true
					break
				}
			}
			if hasPerm {
				break
			}
		}

		if !hasPerm {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": gin.H{
					"code":    "FORBIDDEN",
					"message": "You do not have permission to execute this action",
				},
			})
			return
		}

		c.Next()
	}
}

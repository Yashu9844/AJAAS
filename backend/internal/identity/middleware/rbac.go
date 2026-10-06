package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/identity/repositories"
	"gorm.io/gorm"
)

// permissionDecision is the outcome of evaluating resource:action for the authenticated caller.
type permissionDecision struct {
	allowed bool
	status  int    // HTTP status to use when !allowed
	code    string // error code to use when !allowed
	message string
}

func denied(status int, code, msg string) permissionDecision {
	return permissionDecision{status: status, code: code, message: msg}
}

// evaluatePermission resolves whether the caller (identified by the context set by TenantResolver/Authenticate)
// holds resource:action. The tenant_admin system role bypasses permission checks.
func evaluatePermission(
	c *gin.Context,
	db *gorm.DB,
	resource, action string,
	userRoleRepo repositories.UserRoleRepository,
	rolePermRepo repositories.RolePermissionRepository,
) permissionDecision {
	rolesVal, ok := c.Get("roles")
	if !ok {
		return denied(http.StatusForbidden, "FORBIDDEN", "User roles are not mapped in request context")
	}
	roles, _ := rolesVal.([]string)
	for _, rName := range roles {
		if rName == "tenant_admin" {
			return permissionDecision{allowed: true}
		}
	}

	tenantIDVal, ok1 := c.Get("tenant_id")
	userIDVal, ok2 := c.Get("user_id")
	if !ok1 || !ok2 {
		return denied(http.StatusForbidden, "FORBIDDEN", "Missing tenant or user context mapping")
	}
	tenantID := tenantIDVal.(uuid.UUID)
	userID := userIDVal.(uuid.UUID)
	ctx := c.Request.Context()

	userRoles, err := userRoleRepo.FindByUserID(ctx, db, tenantID, userID)
	if err != nil {
		return denied(http.StatusInternalServerError, "DATABASE_ERROR", "Failed to look up user roles")
	}

	roleIDs := make([]uuid.UUID, 0, len(userRoles))
	for _, ur := range userRoles {
		roleIDs = append(roleIDs, ur.RoleID)
	}
	rolePerms, err := rolePermRepo.FindByRoleIDs(ctx, db, tenantID, roleIDs)
	if err != nil {
		return denied(http.StatusInternalServerError, "DATABASE_ERROR", "Failed to look up role permissions")
	}
	for _, rp := range rolePerms {
		if rp.Permission != nil && rp.Permission.Resource == resource && rp.Permission.Action == action {
			return permissionDecision{allowed: true}
		}
	}
	return denied(http.StatusForbidden, "FORBIDDEN", "You do not have permission to execute this action")
}

// RequirePermission limits access to requests with a specific resource-action pair.
func RequirePermission(
	db *gorm.DB,
	resource string,
	action string,
	userRoleRepo repositories.UserRoleRepository,
	rolePermRepo repositories.RolePermissionRepository,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		d := evaluatePermission(c, db, resource, action, userRoleRepo, rolePermRepo)
		if !d.allowed {
			c.AbortWithStatusJSON(d.status, gin.H{
				"error": gin.H{
					"code":    d.code,
					"message": d.message,
				},
			})
			return
		}
		c.Next()
	}
}

// PermissionFlag never blocks: it evaluates resource:action and stores the boolean result in the gin context under
// key, so handlers can vary behaviour (e.g. unmasked PII) on a finer permission than the route requires.
func PermissionFlag(
	db *gorm.DB,
	resource, action, key string,
	userRoleRepo repositories.UserRoleRepository,
	rolePermRepo repositories.RolePermissionRepository,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		d := evaluatePermission(c, db, resource, action, userRoleRepo, rolePermRepo)
		c.Set(key, d.allowed)
		c.Next()
	}
}

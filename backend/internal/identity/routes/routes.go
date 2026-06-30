package routes

import (
	"github.com/gin-gonic/gin"
	"github.com/jaas/jaas/internal/identity/controllers"
)

// RegisterRoutes sets up routing groups and links controllers to REST paths.
func RegisterRoutes(
	router *gin.RouterGroup,
	tenantCtrl *controllers.TenantController,
	authCtrl *controllers.AuthController,
	userCtrl *controllers.UserController,
	roleCtrl *controllers.RoleController,
	permCtrl *controllers.PermissionController,
) {
	// Public Authentication Endpoints
	auth := router.Group("/auth")
	{
		auth.POST("/login", authCtrl.Login)
		auth.POST("/refresh", authCtrl.Refresh)
		auth.POST("/forgot-password", authCtrl.ForgotPassword)
		auth.POST("/reset-password", authCtrl.ResetPassword)
	}

	// Tenant Operations (Usually platform-owner / superadmin scoped)
	tenants := router.Group("/tenants")
	{
		tenants.POST("", tenantCtrl.Create)
		tenants.GET("", tenantCtrl.List)
		tenants.GET("/:id", tenantCtrl.GetByID)
		tenants.PATCH("/:id", tenantCtrl.Update)
		tenants.POST("/:id/activate", tenantCtrl.Activate)
		tenants.POST("/:id/suspend", tenantCtrl.Suspend)
	}

	// Scoped Session Logouts (Requires auth middleware applied in Phase 9)
	authProtected := router.Group("/auth")
	{
		authProtected.POST("/logout", authCtrl.Logout)
	}

	// User Profiles (Requires tenant resolver and auth middlewares)
	users := router.Group("/users")
	{
		users.POST("", userCtrl.Create)
		users.GET("", userCtrl.List)
		users.GET("/:id", userCtrl.GetByID)
		users.PATCH("/:id", userCtrl.Update)
		users.POST("/:id/deactivate", userCtrl.Deactivate)
		users.POST("/:id/roles", roleCtrl.AssignRolesToUser)
	}

	// RBAC Roles (Requires tenant resolver and auth/rbac middlewares)
	roles := router.Group("/roles")
	{
		roles.POST("", roleCtrl.Create)
		roles.GET("", roleCtrl.List)
		roles.GET("/:id", roleCtrl.GetByID)
		roles.PATCH("/:id", roleCtrl.Update)
		roles.DELETE("/:id", roleCtrl.Delete)
		roles.POST("/:id/permissions", roleCtrl.AssignPermissions)
	}

	// Global Privileges (Requires auth middleware)
	permissions := router.Group("/permissions")
	{
		permissions.GET("", permCtrl.List)
	}
}

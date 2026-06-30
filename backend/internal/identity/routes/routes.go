package routes

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jaas/jaas/internal/identity/controllers"
	identityMiddleware "github.com/jaas/jaas/internal/identity/middleware"
	"github.com/jaas/jaas/internal/identity/repositories"
	"github.com/jaas/jaas/internal/identity/services"
	sharedCache "github.com/jaas/jaas/internal/shared/cache"
	sharedMiddleware "github.com/jaas/jaas/internal/shared/middleware"
	"gorm.io/gorm"
)

// RegisterRoutes sets up routing groups, initializes middlewares, and binds controller paths.
func RegisterRoutes(
	router *gin.RouterGroup,
	db *gorm.DB,
	redisClient *sharedCache.RedisClient,
	tenantRepo repositories.TenantRepository,
	userRoleRepo repositories.UserRoleRepository,
	rolePermRepo repositories.RolePermissionRepository,
	tokenSvc services.TokenService,
	sessionSvc services.SessionService,
	auditSvc services.AuditService,
	tenantCtrl *controllers.TenantController,
	authCtrl *controllers.AuthController,
	userCtrl *controllers.UserController,
	roleCtrl *controllers.RoleController,
	permCtrl *controllers.PermissionController,
) {
	// Initialize Middleware Instances
	tenantResolver := identityMiddleware.TenantResolver(db, tenantRepo)
	authMiddleware := identityMiddleware.Authenticate(db, tokenSvc, sessionSvc)

	// Authentication Group (Rate limited & Public)
	auth := router.Group("/auth")
	{
		// Rate limiter: 10 logins per 15 minutes
		auth.POST("/login", sharedMiddleware.RateLimiter(redisClient, 10, 15*time.Minute), authCtrl.Login)
		auth.POST("/refresh", authCtrl.Refresh)

		// Rate limiter: 5 forgot passwords per 1 hour
		auth.POST("/forgot-password", sharedMiddleware.RateLimiter(redisClient, 5, 1*time.Hour), authCtrl.ForgotPassword)
		auth.POST("/reset-password", authCtrl.ResetPassword)
	}

	// Tenant Operations (Global / Admin operations)
	tenants := router.Group("/tenants")
	{
		tenants.POST("", tenantCtrl.Create)
		tenants.GET("", tenantCtrl.List)
		tenants.GET("/:id", tenantCtrl.GetByID)
		tenants.PATCH("/:id", tenantCtrl.Update)
		tenants.POST("/:id/activate", tenantCtrl.Activate)
		tenants.POST("/:id/suspend", tenantCtrl.Suspend)
	}

	// Scoped Session Logouts (Requires login)
	authProtected := router.Group("/auth")
	authProtected.Use(authMiddleware)
	{
		authProtected.POST("/logout", authCtrl.Logout)
	}

	// User Profiles Group (Requires Tenant Subdomain Context + JWT Auth)
	users := router.Group("/users")
	users.Use(tenantResolver, authMiddleware)
	{
		users.POST("",
			identityMiddleware.RequirePermission(db, "users", "create", userRoleRepo, rolePermRepo),
			identityMiddleware.AuditLog(db, auditSvc, "user.created", "user"),
			userCtrl.Create,
		)
		users.GET("",
			identityMiddleware.RequirePermission(db, "users", "read", userRoleRepo, rolePermRepo),
			userCtrl.List,
		)
		users.GET("/:id",
			identityMiddleware.RequirePermission(db, "users", "read", userRoleRepo, rolePermRepo),
			userCtrl.GetByID,
		)
		users.PATCH("/:id",
			identityMiddleware.RequirePermission(db, "users", "update", userRoleRepo, rolePermRepo),
			identityMiddleware.AuditLog(db, auditSvc, "user.updated", "user"),
			userCtrl.Update,
		)
		users.POST("/:id/deactivate",
			identityMiddleware.RequirePermission(db, "users", "delete", userRoleRepo, rolePermRepo),
			identityMiddleware.AuditLog(db, auditSvc, "user.deactivated", "user"),
			userCtrl.Deactivate,
		)
		users.POST("/:id/roles",
			identityMiddleware.RequirePermission(db, "users", "update", userRoleRepo, rolePermRepo),
			identityMiddleware.AuditLog(db, auditSvc, "role.assigned", "user"),
			roleCtrl.AssignRolesToUser,
		)
	}

	// RBAC Roles Group (Requires Tenant Subdomain Context + JWT Auth)
	roles := router.Group("/roles")
	roles.Use(tenantResolver, authMiddleware)
	{
		roles.POST("",
			identityMiddleware.RequirePermission(db, "roles", "create", userRoleRepo, rolePermRepo),
			identityMiddleware.AuditLog(db, auditSvc, "role.created", "role"),
			roleCtrl.Create,
		)
		roles.GET("",
			identityMiddleware.RequirePermission(db, "roles", "read", userRoleRepo, rolePermRepo),
			roleCtrl.List,
		)
		roles.GET("/:id",
			identityMiddleware.RequirePermission(db, "roles", "read", userRoleRepo, rolePermRepo),
			roleCtrl.GetByID,
		)
		roles.PATCH("/:id",
			identityMiddleware.RequirePermission(db, "roles", "update", userRoleRepo, rolePermRepo),
			identityMiddleware.AuditLog(db, auditSvc, "role.updated", "role"),
			roleCtrl.Update,
		)
		roles.DELETE("/:id",
			identityMiddleware.RequirePermission(db, "roles", "delete", userRoleRepo, rolePermRepo),
			identityMiddleware.AuditLog(db, auditSvc, "role.deleted", "role"),
			roleCtrl.Delete,
		)
		roles.POST("/:id/permissions",
			identityMiddleware.RequirePermission(db, "roles", "update", userRoleRepo, rolePermRepo),
			identityMiddleware.AuditLog(db, auditSvc, "permission.assigned", "role"),
			roleCtrl.AssignPermissions,
		)
	}

	// Global Privileges Group (Requires JWT Auth)
	permissions := router.Group("/permissions")
	permissions.Use(authMiddleware)
	{
		permissions.GET("",
			identityMiddleware.RequirePermission(db, "permissions", "read", userRoleRepo, rolePermRepo),
			permCtrl.List,
		)
	}
}

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
	authz identityMiddleware.Authorizer,
	tokenSvc services.TokenService,
	sessionSvc services.SessionService,
	auditSvc services.AuditService,
	tenantCtrl *controllers.TenantController,
	authCtrl *controllers.AuthController,
	userCtrl *controllers.UserController,
	roleCtrl *controllers.RoleController,
	permCtrl *controllers.PermissionController,
	auditCtrl *controllers.AuditController,
	accessCtrl *controllers.AccessController,
	inviteCtrl *controllers.InvitationController,
	platformAdminKey string,
) {
	// Initialize Middleware Instances
	tenantResolver := identityMiddleware.TenantResolver(db, tenantRepo)
	authMiddleware := identityMiddleware.Authenticate(db, tokenSvc, sessionSvc)

	// Authentication Group (Rate limited & Public)
	auth := router.Group("/auth")
	{
		// Rate limiter: 10 logins per 15 minutes
		auth.POST("/login", sharedMiddleware.FailureRateLimiter(redisClient, 10, 15*time.Minute), authCtrl.Login)
		auth.POST("/refresh", authCtrl.Refresh)

		// Rate limiter: 5 forgot passwords per 1 hour
		auth.POST("/forgot-password", sharedMiddleware.RateLimiter(redisClient, 5, 1*time.Hour), authCtrl.ForgotPassword)
		auth.POST("/reset-password", authCtrl.ResetPassword)
		auth.POST("/accept-invitation", sharedMiddleware.FailureRateLimiter(redisClient, 10, 15*time.Minute), inviteCtrl.Accept)
	}

	// Tenant Operations (Global / Admin operations)
	// Platform-level routes: guarded by the platform operator key (fail closed when unset).
	tenants := router.Group("/tenants")
	tenants.Use(identityMiddleware.PlatformAdmin(platformAdminKey))
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
		authProtected.GET("/me", roleCtrl.Me)
		authProtected.GET("/me/access", accessCtrl.ExplainMe)
	}

	// User Profiles Group (Requires Tenant Subdomain Context + JWT Auth)
	users := router.Group("/users")
	users.Use(tenantResolver, authMiddleware)
	{
		users.POST("",
			identityMiddleware.RequirePermission(authz, "users", "create"),
			identityMiddleware.AuditLog(db, auditSvc, "user.created", "user"),
			userCtrl.Create,
		)
		users.GET("",
			identityMiddleware.RequirePermission(authz, "users", "read"),
			userCtrl.List,
		)
		users.GET("/:id",
			identityMiddleware.RequirePermission(authz, "users", "read"),
			userCtrl.GetByID,
		)
		users.PATCH("/:id",
			identityMiddleware.RequirePermission(authz, "users", "update"),
			identityMiddleware.AuditLog(db, auditSvc, "user.updated", "user"),
			userCtrl.Update,
		)
		users.POST("/:id/deactivate",
			identityMiddleware.RequirePermission(authz, "users", "delete"),
			identityMiddleware.AuditLog(db, auditSvc, "user.deactivated", "user"),
			userCtrl.Deactivate,
		)
		users.POST("/:id/activate",
			identityMiddleware.RequirePermission(authz, "users", "update"),
			identityMiddleware.AuditLog(db, auditSvc, "user.activated", "user"),
			userCtrl.Activate,
		)
		users.DELETE("/:id/roles/:role_id",
			identityMiddleware.RequirePermission(authz, "users", "update"),
			identityMiddleware.AuditLog(db, auditSvc, "role.removed", "user"),
			roleCtrl.RemoveRoleFromUser,
		)
		users.GET("/:id/access",
			identityMiddleware.RequirePermission(authz, "access", "read"),
			accessCtrl.Explain,
		)
		users.GET("/:id/grants",
			identityMiddleware.RequirePermission(authz, "access", "read"),
			accessCtrl.ListGrants,
		)
		users.POST("/:id/grants",
			identityMiddleware.RequirePermission(authz, "access", "manage"),
			accessCtrl.Grant,
		)
		users.DELETE("/:id/grants/:permission_id",
			identityMiddleware.RequirePermission(authz, "access", "manage"),
			accessCtrl.RevokeGrant,
		)
		users.GET("/:id/blocks",
			identityMiddleware.RequirePermission(authz, "access", "read"),
			accessCtrl.ListBlocks,
		)
		users.POST("/:id/blocks",
			identityMiddleware.RequirePermission(authz, "access", "manage"),
			accessCtrl.Block,
		)
		users.DELETE("/:id/blocks/:permission_id",
			identityMiddleware.RequirePermission(authz, "access", "manage"),
			accessCtrl.Unblock,
		)
		users.POST("/:id/roles",
			identityMiddleware.RequirePermission(authz, "users", "update"),
			identityMiddleware.AuditLog(db, auditSvc, "role.assigned", "user"),
			roleCtrl.AssignRolesToUser,
		)
	}

	// RBAC Roles Group (Requires Tenant Subdomain Context + JWT Auth)
	roles := router.Group("/roles")
	roles.Use(tenantResolver, authMiddleware)
	{
		roles.POST("",
			identityMiddleware.RequirePermission(authz, "roles", "create"),
			identityMiddleware.AuditLog(db, auditSvc, "role.created", "role"),
			roleCtrl.Create,
		)
		roles.GET("",
			identityMiddleware.RequirePermission(authz, "roles", "read"),
			roleCtrl.List,
		)
		roles.GET("/:id",
			identityMiddleware.RequirePermission(authz, "roles", "read"),
			roleCtrl.GetByID,
		)
		roles.PATCH("/:id",
			identityMiddleware.RequirePermission(authz, "roles", "update"),
			identityMiddleware.AuditLog(db, auditSvc, "role.updated", "role"),
			roleCtrl.Update,
		)
		roles.DELETE("/:id",
			identityMiddleware.RequirePermission(authz, "roles", "delete"),
			identityMiddleware.AuditLog(db, auditSvc, "role.deleted", "role"),
			roleCtrl.Delete,
		)
		roles.POST("/:id/clone",
			identityMiddleware.RequirePermission(authz, "roles", "create"),
			identityMiddleware.AuditLog(db, auditSvc, "role.created", "role"),
			roleCtrl.Clone,
		)
		roles.DELETE("/:id/permissions/:permission_id",
			identityMiddleware.RequirePermission(authz, "roles", "update"),
			identityMiddleware.AuditLog(db, auditSvc, "permission.removed", "role"),
			roleCtrl.RemovePermission,
		)
		roles.POST("/:id/permissions",
			identityMiddleware.RequirePermission(authz, "roles", "update"),
			identityMiddleware.AuditLog(db, auditSvc, "permission.assigned", "role"),
			roleCtrl.AssignPermissions,
		)
	}

	// Invitations (tenant host + JWT + users:*)
	invitations := router.Group("/invitations")
	invitations.Use(tenantResolver, authMiddleware)
	{
		invitations.POST("", identityMiddleware.RequirePermission(authz, "users", "create"), inviteCtrl.Create)
		invitations.GET("", identityMiddleware.RequirePermission(authz, "users", "read"), inviteCtrl.List)
		invitations.POST("/:id/resend", identityMiddleware.RequirePermission(authz, "users", "create"), inviteCtrl.Resend)
		invitations.POST("/:id/revoke", identityMiddleware.RequirePermission(authz, "users", "update"), inviteCtrl.Revoke)
	}

	// Access administration (tenant host + JWT + access:manage)
	accessAdmin := router.Group("/access")
	accessAdmin.Use(tenantResolver, authMiddleware)
	{
		accessAdmin.POST("/automatic-roles/sync",
			identityMiddleware.RequirePermission(authz, "access", "manage"),
			identityMiddleware.AuditLog(db, auditSvc, "access.automatic_roles_synced", "tenant"),
			accessCtrl.SyncAutomaticRoles,
		)
	}

	// Audit trail (tenant host + JWT + audit:read)
	audit := router.Group("/audit-logs")
	audit.Use(tenantResolver, authMiddleware)
	{
		audit.GET("", identityMiddleware.RequirePermission(authz, "audit", "read"), auditCtrl.List)
	}

	// Global Privileges Group (Requires JWT Auth)
	permissions := router.Group("/permissions")
	permissions.Use(authMiddleware)
	{
		permissions.GET("",
			identityMiddleware.RequirePermission(authz, "permissions", "read"),
			permCtrl.List,
		)
	}
}

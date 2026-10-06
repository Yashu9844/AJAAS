// Package app wires Modules 0-2 into a single HTTP engine. cmd/main.go and the integration tests share it so the
// tested router is exactly the production router.
package app

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jaas/jaas/internal/employee"
	"github.com/jaas/jaas/internal/identity"
	identityServices "github.com/jaas/jaas/internal/identity/services"
	"github.com/jaas/jaas/internal/organization"
	"github.com/jaas/jaas/internal/shared/cache"
	"github.com/jaas/jaas/internal/shared/config"
	"github.com/jaas/jaas/internal/shared/logger"
	sharedMiddleware "github.com/jaas/jaas/internal/shared/middleware"
	"github.com/jaas/jaas/internal/shared/queue"
	"gorm.io/gorm"
)

// Options carries the infrastructure handles and runtime policy the engine needs.
type Options struct {
	Config         *config.Config
	DB             *gorm.DB
	Redis          *cache.RedisClient
	Publisher      queue.EventPublisher
	Logger         *logger.Logger
	CORSOrigins    []string // explicit allow-list; "*" only for development
	TrustedProxies []string // nil = trust none (X-Forwarded-For ignored)
}

// ValidateConfig rejects configurations that are unsafe to run.
func ValidateConfig(cfg *config.Config) error {
	if cfg.Server.Env == "production" {
		if len(cfg.JWT.Secret) < 32 {
			return errors.New("jwt.secret must be at least 32 characters in production (set JWT_SECRET)")
		}
	}
	if cfg.JWT.AccessTokenTTL <= 0 || cfg.JWT.RefreshTokenTTL <= 0 {
		return errors.New("jwt token TTLs must be positive")
	}
	return nil
}

// Build migrates the schema, seeds the permission catalogue and returns the fully wired engine.
func Build(o Options) (*gin.Engine, error) {
	if err := ValidateConfig(o.Config); err != nil {
		return nil, err
	}
	cfg := o.Config
	log := o.Logger

	identityModule := identity.NewModule(o.DB, o.Redis, o.Publisher, cfg.JWT.Secret, cfg.JWT.AccessTokenTTL, cfg.JWT.RefreshTokenTTL, &log.Logger)
	identityModule.SetPlatformAdminKey(cfg.Platform.AdminKey)
	if cfg.Platform.AdminKey == "" {
		log.Warn().Msg("platform.admin_key is empty: /api/v1/tenants routes are DISABLED (set PLATFORM_ADMIN_KEY)")
	}
	if err := o.DB.AutoMigrate(identityModule.RegisterModels()...); err != nil {
		return nil, fmt.Errorf("identity auto-migration failed: %w", err)
	}
	if err := identityModule.SeedPermissions(); err != nil {
		return nil, fmt.Errorf("seeding permission catalogue failed: %w", err)
	}

	orgModule := organization.NewModule(o.DB, o.Redis, o.Publisher, &log.Logger, identityModule.UserService(), identityModule.AuditService())
	// FR-M005: deactivating a user converges org mappings in the same transaction.
	identityServices.SetUserDeactivationConverger(orgModule.UserDeactivationConverger())
	if err := o.DB.AutoMigrate(orgModule.RegisterModels()...); err != nil {
		return nil, fmt.Errorf("organization auto-migration failed: %w", err)
	}

	employeeModule := employee.NewModule(o.DB, o.Redis, o.Publisher, &log.Logger, identityModule.UserService(), identityModule.AuditService())
	if err := o.DB.AutoMigrate(employeeModule.RegisterModels()...); err != nil {
		return nil, fmt.Errorf("employee auto-migration failed: %w", err)
	}

	if cfg.Server.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}
	router := gin.New()
	router.Use(gin.Recovery())
	// Client IPs drive rate limiting: never trust X-Forwarded-For unless proxies are explicitly listed.
	if err := router.SetTrustedProxies(o.TrustedProxies); err != nil {
		return nil, fmt.Errorf("invalid trusted proxies: %w", err)
	}
	router.Use(sharedMiddleware.RequestLogger(log))
	router.Use(sharedMiddleware.CORS(o.CORSOrigins))

	v1 := router.Group("/api/v1")
	identityModule.RegisterRoutes(v1)
	orgModule.RegisterRoutes(v1, identityModule.UserRoleRepository(), identityModule.RolePermissionRepository(), identityModule.TenantResolver(), identityModule.AuthMiddleware(), identityModule.AuditService())
	employeeModule.RegisterRoutes(v1, identityModule.UserRoleRepository(), identityModule.RolePermissionRepository(), identityModule.TenantResolver(), identityModule.AuthMiddleware(), identityModule.AuditService())

	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "UP", "timestamp": time.Now().Format(time.RFC3339)})
	})
	return router, nil
}

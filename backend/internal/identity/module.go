package identity

import (
	"github.com/gin-gonic/gin"
	"github.com/jaas/jaas/internal/identity/controllers"
	"github.com/jaas/jaas/internal/identity/models"
	"github.com/jaas/jaas/internal/identity/repositories"
	"github.com/jaas/jaas/internal/identity/routes"
	"github.com/jaas/jaas/internal/identity/services"
	sharedCache "github.com/jaas/jaas/internal/shared/cache"
	sharedConfig "github.com/jaas/jaas/internal/shared/config"
	sharedQueue "github.com/jaas/jaas/internal/shared/queue"
	"github.com/rs/zerolog"
	"gorm.io/gorm"
)

// Module encapsulates the Module 0 Identity and Access Management domain.
type Module struct {
	db           *gorm.DB
	redisClient  *sharedCache.RedisClient
	publisher    sharedQueue.EventPublisher
	logger       *zerolog.Logger

	// Repositories
	tenantRepo   repositories.TenantRepository
	userRepo     repositories.UserRepository
	roleRepo     repositories.RoleRepository
	permRepo     repositories.PermissionRepository
	userRoleRepo repositories.UserRoleRepository
	rolePermRepo repositories.RolePermissionRepository
	sessionRepo  repositories.SessionRepository
	tokenRepo    repositories.RefreshTokenRepository
	resetRepo    repositories.PasswordResetTokenRepository
	mfaRepo      repositories.MFAConfigRepository
	settingsRepo repositories.TenantSettingsRepository
	auditRepo    repositories.AuditLogRepository

	// Services
	auditSvc     services.AuditService
	tokenSvc     services.TokenService
	sessionSvc   services.SessionService
	tenantSvc    services.TenantService
	roleSvc      services.RoleService
	permSvc      services.PermissionService
	userSvc      services.UserService
	authSvc      services.AuthService

	// Controllers
	tenantCtrl   *controllers.TenantController
	authCtrl     *controllers.AuthController
	userCtrl     *controllers.UserController
	roleCtrl     *controllers.RoleController
	permCtrl     *controllers.PermissionController
}

// NewModule constructs a Module container injecting database and message queue handles.
func NewModule(
	db *gorm.DB,
	redisClient *sharedCache.RedisClient,
	publisher sharedQueue.EventPublisher,
	jwtSecret string,
	accessTokenTTL int,
	refreshTokenTTL int,
	logger *zerolog.Logger,
) *Module {
	m := &Module{
		db:          db,
		redisClient: redisClient,
		publisher:   publisher,
		logger:      logger,
	}

	// 1. Initialize Repositories
	m.tenantRepo = repositories.NewTenantRepository()
	m.userRepo = repositories.NewUserRepository()
	m.roleRepo = repositories.NewRoleRepository()
	m.permRepo = repositories.NewPermissionRepository()
	m.userRoleRepo = repositories.NewUserRoleRepository()
	m.rolePermRepo = repositories.NewRolePermissionRepository()
	m.sessionRepo = repositories.NewSessionRepository()
	m.tokenRepo = repositories.NewRefreshTokenRepository()
	m.resetRepo = repositories.NewPasswordResetTokenRepository()
	m.mfaRepo = repositories.NewMFAConfigRepository()
	m.settingsRepo = repositories.NewTenantSettingsRepository()
	m.auditRepo = repositories.NewAuditLogRepository()

	// 2. Initialize Services
	jwtConfig := &sharedConfig.JWTConfig{
		Secret:          jwtSecret,
		AccessTokenTTL:  accessTokenTTL,
		RefreshTokenTTL: refreshTokenTTL,
	}

	m.auditSvc = services.NewAuditService(m.auditRepo, logger)
	m.tokenSvc = services.NewTokenService(jwtConfig)
	m.sessionSvc = services.NewSessionService(m.sessionRepo, redisClient)

	m.tenantSvc = services.NewTenantService(m.tenantRepo, m.roleRepo, publisher, m.auditSvc)
	m.roleSvc = services.NewRoleService(m.roleRepo, m.permRepo, m.userRoleRepo, m.rolePermRepo, m.userRepo, publisher, m.auditSvc)
	m.permSvc = services.NewPermissionService(m.permRepo)
	m.userSvc = services.NewUserService(m.userRepo, m.roleRepo, m.userRoleRepo, m.sessionRepo, m.tokenRepo, publisher, m.auditSvc)
	m.authSvc = services.NewAuthService(
		m.tenantRepo,
		m.userRepo,
		m.userRoleRepo,
		m.sessionRepo,
		m.tokenRepo,
		m.resetRepo,
		m.tokenSvc,
		m.sessionSvc,
		m.publisher,
		m.auditSvc,
	)

	// 3. Initialize Controllers
	m.tenantCtrl = controllers.NewTenantController(db, m.tenantSvc)
	m.authCtrl = controllers.NewAuthController(db, m.authSvc)
	m.userCtrl = controllers.NewUserController(db, m.userSvc)
	m.roleCtrl = controllers.NewRoleController(db, m.roleSvc)
	m.permCtrl = controllers.NewPermissionController(db, m.permSvc)

	return m
}

// RegisterModels lists all domain entity models for migrations.
func (m *Module) RegisterModels() []interface{} {
	return []interface{}{
		&models.Tenant{},
		&models.TenantSettings{},
		&models.User{},
		&models.Role{},
		&models.Permission{},
		&models.UserRole{},
		&models.RolePermission{},
		&models.Session{},
		&models.RefreshToken{},
		&models.PasswordResetToken{},
		&models.MFAConfig{},
		&models.AuditLog{},
	}
}

// RegisterRoutes hooks endpoint paths to controller handlers.
func (m *Module) RegisterRoutes(router *gin.RouterGroup) {
	routes.RegisterRoutes(
		router,
		m.db,
		m.redisClient,
		m.tenantRepo,
		m.userRoleRepo,
		m.rolePermRepo,
		m.tokenSvc,
		m.sessionSvc,
		m.auditSvc,
		m.tenantCtrl,
		m.authCtrl,
		m.userCtrl,
		m.roleCtrl,
		m.permCtrl,
	)
}

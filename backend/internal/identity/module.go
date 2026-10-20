package identity

import (
	"context"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/identity/access"
	"github.com/jaas/jaas/internal/identity/controllers"
	"github.com/jaas/jaas/internal/identity/middleware"
	"github.com/jaas/jaas/internal/identity/models"
	"github.com/jaas/jaas/internal/identity/repositories"
	"github.com/jaas/jaas/internal/identity/routes"
	"github.com/jaas/jaas/internal/identity/services"
	sharedCache "github.com/jaas/jaas/internal/shared/cache"
	sharedConfig "github.com/jaas/jaas/internal/shared/config"
	"github.com/jaas/jaas/internal/shared/outbox"
	sharedQueue "github.com/jaas/jaas/internal/shared/queue"
	"github.com/rs/zerolog"
	"gorm.io/gorm"
	"time"
)

// Module encapsulates the Module 0 Identity and Access Management domain.
type Module struct {
	db          *gorm.DB
	redisClient *sharedCache.RedisClient
	publisher   sharedQueue.EventPublisher
	logger      *zerolog.Logger

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
	accessRepo   repositories.AccessRepository

	// Services
	auditSvc   services.AuditService
	tokenSvc   services.TokenService
	sessionSvc services.SessionService
	tenantSvc  services.TenantService
	roleSvc    services.RoleService
	permSvc    services.PermissionService
	userSvc    services.UserService
	authSvc    services.AuthService
	resolver   services.AccessResolver
	catalogue  services.Catalogue
	accessSvc  services.AccessService
	eventSink  services.EventSink
	relay      *outbox.Relay
	mailbox    *services.DevMailbox
	automatic  services.AutomaticRoles

	// Controllers
	tenantCtrl *controllers.TenantController
	authCtrl   *controllers.AuthController
	userCtrl   *controllers.UserController
	roleCtrl   *controllers.RoleController
	permCtrl   *controllers.PermissionController
	auditCtrl  *controllers.AuditController
	accessCtrl *controllers.AccessController

	platformAdminKey string
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
	m.accessRepo = repositories.NewAccessRepository()

	// 2. Initialize Services
	jwtConfig := &sharedConfig.JWTConfig{
		Secret:          jwtSecret,
		AccessTokenTTL:  accessTokenTTL,
		RefreshTokenTTL: refreshTokenTTL,
	}

	m.auditSvc = services.NewAuditService(m.auditRepo, logger)
	eventStore := outbox.NewStore(outboxTable)
	m.eventSink = services.NewOutboxSink(eventStore)
	m.relay = &outbox.Relay{DB: db, Store: eventStore, Publisher: publisher}
	m.tokenSvc = services.NewTokenService(jwtConfig)
	m.sessionSvc = services.NewSessionService(m.sessionRepo, redisClient, time.Duration(accessTokenTTL)*time.Minute)

	resolverDeps := services.AccessResolverDeps{DB: db, Access: m.accessRepo, UserRoles: m.userRoleRepo, RolePerms: m.rolePermRepo}
	if redisClient != nil {
		resolverDeps.Cache = redisClient
	}
	m.resolver = services.NewAccessResolver(resolverDeps)
	m.catalogue = services.NewCatalogue(services.CatalogueDeps{DB: db, Roles: m.roleRepo, RolePerms: m.rolePermRepo, Access: m.accessRepo})
	m.tenantSvc = services.NewTenantService(m.tenantRepo, m.roleRepo, publisher, m.auditSvc,
		services.WithAdminProvisioning(m.userRepo, m.userRoleRepo), services.WithRoleProvisioner(m.catalogue.ProvisionTenant),
		services.WithTenantEvents(m.eventSink))
	m.roleSvc = services.NewRoleService(m.roleRepo, m.permRepo, m.userRoleRepo, m.rolePermRepo, m.userRepo, publisher, m.auditSvc,
		services.WithAccessModel(m.accessRepo, m.resolver), services.WithRoleEvents(m.eventSink))
	m.permSvc = services.NewPermissionService(m.permRepo)
	m.accessSvc = services.NewAccessService(services.AccessServiceDeps{Users: m.userRepo, Perms: m.permRepo, UserRoles: m.userRoleRepo,
		Access: m.accessRepo, Resolver: m.resolver, Audit: m.auditSvc})
	m.automatic = services.NewAutomaticRoles(services.AutomaticRolesDeps{DB: db, Access: m.accessRepo, UserRoles: m.userRoleRepo,
		Catalogue: m.catalogue, Logger: logger})
	m.userSvc = services.NewUserService(m.userRepo, m.roleRepo, m.userRoleRepo, m.sessionRepo, m.tokenRepo, publisher, m.auditSvc, services.WithAccessResolver(m.resolver), services.WithUserEvents(m.eventSink))
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
		services.WithAuthEvents(m.eventSink),
		services.WithSecretDelivery(secretRouter{m: m}),
	)

	// 3. Initialize Controllers
	m.tenantCtrl = controllers.NewTenantController(db, m.tenantSvc)
	m.authCtrl = controllers.NewAuthController(db, m.authSvc)
	m.userCtrl = controllers.NewUserController(db, m.userSvc)
	m.roleCtrl = controllers.NewRoleController(db, m.roleSvc)
	m.permCtrl = controllers.NewPermissionController(db, m.permSvc)
	m.auditCtrl = controllers.NewAuditController(db, m.auditSvc)
	m.accessCtrl = controllers.NewAccessController(db, m.accessSvc, m.automatic)
	m.authCtrl.OnLogin(m.syncUserOnLogin)

	return m
}

// SetPlatformAdminKey configures the platform operator key guarding /tenants routes (empty = routes disabled).
func (m *Module) SetPlatformAdminKey(key string) { m.platformAdminKey = key }

// SeedPermissions registers M0's own permission catalogue (models.IdentityCatalogue).
func (m *Module) SeedPermissions() error {
	return m.catalogue.Register(context.Background(), models.IdentityCatalogue)
}

// RegisterCatalogue registers another service's permission definitions and their predefined-role defaults.
func (m *Module) RegisterCatalogue(ctx context.Context, defs []access.PermissionDef) error {
	return m.catalogue.Register(ctx, defs)
}

// SyncRoleTemplates provisions predefined roles in every tenant; call it once all catalogues are registered.
func (m *Module) SyncRoleTemplates(ctx context.Context) error { return m.catalogue.SyncAllTenants(ctx) }

// outboxTable is M0's transactional outbox (migration 000059).
const outboxTable = "identity_events_outbox"

// RunEventRelay publishes committed identity events until ctx is cancelled.
func (m *Module) RunEventRelay(ctx context.Context) { m.relay.Run(ctx) }

// EnableDevMailbox keeps one-time secrets (reset and invitation tokens) in memory for development tooling. Never call
// it outside development.
func (m *Module) EnableDevMailbox() *services.DevMailbox {
	m.mailbox = services.NewDevMailbox()
	return m.mailbox
}

// secretRouter delivers secrets to the dev mailbox when enabled; production delivery arrives with M10 Notifications.
type secretRouter struct{ m *Module }

func (r secretRouter) Deliver(ctx context.Context, msg services.SecretMessage) error {
	if r.m.mailbox != nil {
		return r.m.mailbox.Deliver(ctx, msg)
	}
	return nil
}

// CallerScope identifies the caller of a request for Idempotency-Key scoping: "platform" for a valid platform key,
// "tenant:user" for a valid access token, otherwise not scoped.
func (m *Module) CallerScope(c *gin.Context) (string, bool) {
	if m.platformAdminKey != "" && middleware.PlatformKeyValid(c.GetHeader("X-Platform-Key"), m.platformAdminKey) {
		return "platform", true
	}
	raw := strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
	if raw == "" || raw == c.GetHeader("Authorization") {
		return "", false
	}
	claims, err := m.tokenSvc.ValidateAccessToken(raw)
	if err != nil {
		return "", false
	}
	return claims.TenantID + ":" + claims.Subject, true
}

// SetAutomaticRoleSource installs the HR-backed source of the automatic employee/manager roles.
func (m *Module) SetAutomaticRoleSource(src services.AutomaticRoleSource) { m.automatic.SetSource(src) }

// RunAutomaticRoleSync reconciles automatic roles in every tenant each interval until ctx is cancelled.
func (m *Module) RunAutomaticRoleSync(ctx context.Context, interval time.Duration) {
	m.automatic.Run(ctx, interval)
}

// syncUserOnLogin refreshes the user's automatic roles at login; failures only delay the sync to the next sweep.
func (m *Module) syncUserOnLogin(ctx context.Context, tenantID, userID uuid.UUID) {
	if err := m.automatic.SyncUser(ctx, tenantID, userID); err != nil && m.logger != nil {
		m.logger.Warn().Err(err).Str("user_id", userID.String()).Msg("automatic role sync at login failed")
	}
}

// Authorizer exposes access resolution to the permission middleware of dependent modules.
func (m *Module) Authorizer() middleware.Authorizer { return m.resolver }

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
		&models.UserPermissionGrant{},
		&models.UserPermissionBlock{},
	}
}

// RegisterRoutes hooks endpoint paths to controller handlers.
func (m *Module) RegisterRoutes(router *gin.RouterGroup) {
	routes.RegisterRoutes(
		router,
		m.db,
		m.redisClient,
		m.tenantRepo,
		m.resolver,
		m.tokenSvc,
		m.sessionSvc,
		m.auditSvc,
		m.tenantCtrl,
		m.authCtrl,
		m.userCtrl,
		m.roleCtrl,
		m.permCtrl,
		m.auditCtrl,
		m.accessCtrl,
		m.platformAdminKey,
	)
}

// UserService exposes the user domain service to dependent modules (Module 1+).
func (m *Module) UserService() services.UserService { return m.userSvc }

// AuditService exposes the audit transport to dependent modules.
func (m *Module) AuditService() services.AuditService { return m.auditSvc }

// UserRoleRepository exposes user-role reads for RBAC middleware in dependent modules.
func (m *Module) UserRoleRepository() repositories.UserRoleRepository { return m.userRoleRepo }

// RolePermissionRepository exposes role-permission reads for RBAC middleware in dependent modules.
func (m *Module) RolePermissionRepository() repositories.RolePermissionRepository {
	return m.rolePermRepo
}

// TenantResolver builds the subdomain tenant-resolution middleware for dependent modules.
func (m *Module) TenantResolver() gin.HandlerFunc {
	return middleware.TenantResolver(m.db, m.tenantRepo)
}

// AuthMiddleware builds the JWT authentication middleware for dependent modules.
func (m *Module) AuthMiddleware() gin.HandlerFunc {
	return middleware.Authenticate(m.db, m.tokenSvc, m.sessionSvc)
}

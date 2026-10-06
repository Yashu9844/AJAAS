package employee

import (
	"context"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/employee/controllers"
	"github.com/jaas/jaas/internal/employee/models"
	"github.com/jaas/jaas/internal/employee/repositories"
	employeeRoutes "github.com/jaas/jaas/internal/employee/routes"
	"github.com/jaas/jaas/internal/employee/services"
	identityRepos "github.com/jaas/jaas/internal/identity/repositories"
	identityServices "github.com/jaas/jaas/internal/identity/services"
	sharedCache "github.com/jaas/jaas/internal/shared/cache"
	"github.com/jaas/jaas/internal/shared/queue"
	"github.com/rs/zerolog"
	"gorm.io/gorm"
)

// Module encapsulates the Module 2 Employee domain.
type Module struct {
	db          *gorm.DB
	redisClient *sharedCache.RedisClient
	publisher   queue.EventPublisher
	logger      *zerolog.Logger

	// Repositories
	profileRepo   repositories.EmployeeProfileRepository
	statutoryRepo repositories.EmployeeStatutoryRepository
	docRepo       repositories.EmployeeDocumentRepository
	timelineRepo  repositories.EmployeeTimelineRepository

	// Services
	empSvc       services.EmployeeService
	statutorySvc services.EmployeeStatutoryService
	docSvc       services.EmployeeDocumentService
	timelineSvc  services.EmployeeTimelineService
	consumer     services.EventConsumer

	// Controllers
	empCtrl  *controllers.EmployeeController
	statCtrl *controllers.StatutoryController
	docCtrl  *controllers.DocumentController
	tlCtrl   *controllers.TimelineController
}

func NewModule(
	db *gorm.DB,
	redisClient *sharedCache.RedisClient,
	publisher queue.EventPublisher,
	logger *zerolog.Logger,
	userSvc identityServices.UserService,
	auditSvc identityServices.AuditService,
) *Module {
	pRepo := repositories.NewEmployeeProfileRepository(db)
	sRepo := repositories.NewEmployeeStatutoryRepository(db)
	dRepo := repositories.NewEmployeeDocumentRepository(db)
	tRepo := repositories.NewEmployeeTimelineRepository(db)

	empSvc := services.NewEmployeeService(db, pRepo, tRepo, userSvc, auditSvc, publisher)
	statSvc := services.NewEmployeeStatutoryService(sRepo, pRepo)
	docSvc := services.NewEmployeeDocumentService(dRepo, pRepo, publisher)
	tlSvc := services.NewEmployeeTimelineService(tRepo, pRepo)
	consumer := services.NewEventConsumer(empSvc, logger)

	empCtrl := controllers.NewEmployeeController(empSvc)
	statCtrl := controllers.NewStatutoryController(statSvc)
	docCtrl := controllers.NewDocumentController(docSvc)
	tlCtrl := controllers.NewTimelineController(tlSvc)

	return &Module{
		db:            db,
		redisClient:   redisClient,
		publisher:     publisher,
		logger:        logger,
		profileRepo:   pRepo,
		statutoryRepo: sRepo,
		docRepo:       dRepo,
		timelineRepo:  tRepo,
		empSvc:        empSvc,
		statutorySvc:  statSvc,
		docSvc:        docSvc,
		timelineSvc:   tlSvc,
		consumer:      consumer,
		empCtrl:       empCtrl,
		statCtrl:      statCtrl,
		docCtrl:       docCtrl,
		tlCtrl:        tlCtrl,
	}
}

func (m *Module) RegisterModels() []interface{} {
	return []interface{}{
		&models.EmployeeProfile{},
		&models.EmploymentDetail{},
		&models.EmployeeContact{},
		&models.EmployeeStatutory{},
		&models.EmployeeDocument{},
		&models.EmployeeTimeline{},
		&models.EmployeeEventsOutbox{},
	}
}

func (m *Module) RegisterRoutes(
	router *gin.RouterGroup,
	userRoleRepo identityRepos.UserRoleRepository,
	rolePermRepo identityRepos.RolePermissionRepository,
	tenantResolver gin.HandlerFunc,
	authMiddleware gin.HandlerFunc,
	auditSvc identityServices.AuditService,
) {
	employeeRoutes.RegisterRoutes(
		router,
		m.db,
		userRoleRepo,
		rolePermRepo,
		tenantResolver,
		authMiddleware,
		m.empCtrl,
		m.statCtrl,
		m.docCtrl,
		m.tlCtrl,
		m.db,
		auditSvc,
	)
}

func (m *Module) EmployeeService() services.EmployeeService {
	return m.empSvc
}

func (m *Module) EventConsumer() services.EventConsumer {
	return m.consumer
}

// userDeactivationConverger adapts the employee service to the Module 0 deactivation hook (FR-EV001, synchronous).
type userDeactivationConverger struct{ svc services.EmployeeService }

func (c userDeactivationConverger) DeactivateUserMappings(ctx context.Context, tx *gorm.DB, tenantID, userID, _ uuid.UUID) error {
	return c.svc.ConvergeUserDeactivation(ctx, tx, tenantID, userID)
}

// UserDeactivationConverger returns the hook that keeps employee profiles in step with Module 0 user deactivation.
func (m *Module) UserDeactivationConverger() identityServices.UserDeactivationConverger {
	return userDeactivationConverger{svc: m.empSvc}
}

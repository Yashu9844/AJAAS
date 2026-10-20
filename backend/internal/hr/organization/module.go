package organization

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/hr/identityport"
	"github.com/jaas/jaas/internal/hr/organization/controllers"
	"github.com/jaas/jaas/internal/hr/organization/events"
	"github.com/jaas/jaas/internal/hr/organization/models"
	"github.com/jaas/jaas/internal/hr/organization/repositories"
	orgRoutes "github.com/jaas/jaas/internal/hr/organization/routes"
	"github.com/jaas/jaas/internal/hr/organization/services"
	sharedCache "github.com/jaas/jaas/internal/shared/cache"
	"github.com/jaas/jaas/internal/shared/queue"
	"github.com/rs/zerolog"
	"gorm.io/gorm"
)

// Module encapsulates the Module 1 Organization domain.
type Module struct {
	db          *gorm.DB
	redisClient *sharedCache.RedisClient
	publisher   queue.EventPublisher
	logger      *zerolog.Logger

	// Repositories
	deptRepo    repositories.DepartmentRepository
	teamRepo    repositories.TeamRepository
	desigRepo   repositories.DesignationRepository
	mappingRepo repositories.MappingRepository

	// Services
	deptSvc    services.DepartmentService
	teamSvc    services.TeamService
	desigSvc   services.DesignationService
	mappingSvc services.MappingService
	chartSvc   services.OrgChartService
	consumer   services.EventConsumer

	// Controllers
	deptCtrl    *controllers.DepartmentController
	teamCtrl    *controllers.TeamController
	desigCtrl   *controllers.DesignationController
	mappingCtrl *controllers.MappingController
	chartCtrl   *controllers.OrgChartController
}

// NewModule wires the organization module on shared handles plus Module 0 services.
func NewModule(
	db *gorm.DB,
	redisClient *sharedCache.RedisClient,
	publisher queue.EventPublisher,
	logger *zerolog.Logger,
	idp identityport.Port,
) *Module {
	m := &Module{db: db, redisClient: redisClient, publisher: publisher, logger: logger}

	m.deptRepo = repositories.NewDepartmentRepository()
	m.teamRepo = repositories.NewTeamRepository()
	m.desigRepo = repositories.NewDesignationRepository()
	m.mappingRepo = repositories.NewMappingRepository()

	users, audit := idp.Users(), idp.Audit()

	m.deptSvc = services.NewDepartmentService(m.deptRepo, m.teamRepo, m.mappingRepo, publisher, audit)
	m.teamSvc = services.NewTeamService(m.teamRepo, m.deptRepo, m.mappingRepo, users, publisher, audit)
	m.desigSvc = services.NewDesignationService(m.desigRepo, m.mappingRepo, publisher, audit)
	m.mappingSvc = services.NewMappingService(m.mappingRepo, m.deptRepo, m.teamRepo, m.desigRepo, users, publisher, audit)
	m.chartSvc = services.NewOrgChartService(m.deptRepo, m.teamRepo, m.mappingRepo, m.desigRepo, redisClient)
	m.consumer = services.NewEventConsumer(m.mappingSvc)

	m.deptCtrl = controllers.NewDepartmentController(db, m.deptSvc)
	m.teamCtrl = controllers.NewTeamController(db, m.teamSvc)
	m.desigCtrl = controllers.NewDesignationController(db, m.desigSvc)
	m.mappingCtrl = controllers.NewMappingController(db, m.mappingSvc)
	m.chartCtrl = controllers.NewOrgChartController(db, m.chartSvc)

	return m
}

// RegisterModels lists org entity models for migrations.
func (m *Module) RegisterModels() []interface{} {
	return []interface{}{
		&models.Department{},
		&models.Team{},
		&models.Designation{},
		&models.Mapping{},
		&models.OrgEventOutbox{},
	}
}

// Consumer exposes the identity-event consumer for broker subscription wiring.
func (m *Module) Consumer() services.EventConsumer { return m.consumer }

// ChartInvalidator exposes org-chart cache invalidation.
func (m *Module) ChartInvalidator() services.ChartInvalidator {
	inv, _ := m.chartSvc.(services.ChartInvalidator)
	return inv
}

// convergeAndInvalidate wraps the FR-M005 user-deactivation hook so the cached org chart is dropped too.
type convergeAndInvalidate struct {
	svc services.MappingService
	inv services.ChartInvalidator
}

func (c convergeAndInvalidate) DeactivateUserMappings(ctx context.Context, tx *gorm.DB, tenantID, userID, correlationID uuid.UUID) error {
	if err := c.svc.DeactivateUserMappings(ctx, tx, tenantID, userID, correlationID); err != nil {
		return err
	}
	if c.inv != nil {
		c.inv.InvalidateChart(ctx, tenantID)
	}
	return nil
}

// UserDeactivationConverger is the FR-M005 hook (mappings converge + chart cache invalidated).
func (m *Module) UserDeactivationConverger() identityport.DeactivationHook {
	return convergeAndInvalidate{svc: m.mappingSvc, inv: m.ChartInvalidator()}
}

// MappingService exposes the mapping domain service for cross-module
// convergence (FR-M005 hook registered in cmd/main.go).
func (m *Module) MappingService() services.MappingService { return m.mappingSvc }

// DepartmentService exposes read access to departments for dependent modules (Module 6 D6-03).
func (m *Module) DepartmentService() services.DepartmentService { return m.deptSvc }

// DesignationService exposes read access to designations for dependent modules (Module 6 D6-03).
func (m *Module) DesignationService() services.DesignationService { return m.desigSvc }

// RegisterRoutes hooks org endpoint groups onto the API router. The identity
// module owns tenantResolver/authMiddleware construction; org reuses them.
func (m *Module) RegisterRoutes(router *gin.RouterGroup, idp identityport.HTTP) {
	orgRoutes.RegisterRoutes(router, idp, orgRoutes.Controllers{Departments: m.deptCtrl, Teams: m.teamCtrl,
		Designations: m.desigCtrl, Mappings: m.mappingCtrl, Chart: m.chartCtrl}, m.ChartInvalidator())
}

// OrgEvents returns produced event type documentation for consumers.
func OrgEvents() []string {
	return []string{
		events.TypeDepartmentCreated,
		events.TypeTeamCreated,
		events.TypeTeamMoved,
		events.TypeMappingCreated,
	}
}

// Package recruitment wires Module 6 (Recruitment & Applicant Tracking) into the application.
package recruitment

import (
	"context"
	"errors"
	coreAPI "github.com/jaas/jaas/internal/hr/core/api"
	"github.com/jaas/jaas/internal/shared/identityport"
	recruitmentAPI "github.com/jaas/jaas/internal/hr/recruitment/api"
	"github.com/jaas/jaas/internal/hr/recruitment/plus"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	orgDTO "github.com/jaas/jaas/internal/hr/organization/dto"
	"github.com/jaas/jaas/internal/hr/recruitment/controllers"
	"github.com/jaas/jaas/internal/hr/recruitment/models"
	"github.com/jaas/jaas/internal/hr/recruitment/repositories"
	"github.com/jaas/jaas/internal/hr/recruitment/routes"
	"github.com/jaas/jaas/internal/hr/recruitment/services"
	"github.com/jaas/jaas/internal/identity/access"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"github.com/jaas/jaas/internal/shared/queue"
	"gorm.io/gorm"
)

// Module is the Module 6 DI container.
type Module struct {
	db         *gorm.DB
	controller *controllers.Controller
	relay      *services.OutboxRelay
	hooks      *services.Hooks
	plus       *plus.Module
	gate       *services.OfferGateSwitch
	positions  *positionsSwitch
}

// DepartmentReader is the Module 1 department lookup Module 6 needs (D6-03).
type DepartmentReader interface {
	GetDepartment(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*orgDTO.DepartmentResponse, error)
}

// DesignationReader is the Module 1 designation lookup Module 6 needs (D6-03).
type DesignationReader interface {
	GetDesignation(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*orgDTO.DesignationResponse, error)
}

// Ports are the cross-module collaborators Module 6 consumes (connections C2–C5).
type Ports struct {
	Employees    services.EmployeeDirectory // Module 2 EmployeeService
	Users        services.UserInviter       // Module 0 UserService
	Departments  DepartmentReader           // Module 1
	Designations DesignationReader          // Module 1
	Audit        services.AuditLogger       // Module 0 AuditService
	Publisher    queue.EventPublisher
}

// NewModule wires repositories, services and the controller.
func NewModule(db *gorm.DB, p Ports) *Module {
	gate, positions := &services.OfferGateSwitch{}, &positionsSwitch{}
	deps := services.Deps{
		Tx: services.NewTxRunner(db),
		Repos: services.Repos{Jobs: repositories.NewJobRepository(), Candidates: repositories.NewCandidateRepository(),
			Interviews: repositories.NewInterviewRepository(), Offers: repositories.NewOfferRepository(), Outbox: repositories.NewOutboxRepository()},
		Employees: p.Employees, Users: p.Users, Org: orgDirectory{db: db, depts: p.Departments, desigs: p.Designations},
		Audit: p.Audit, Publisher: p.Publisher, Now: time.Now, Hooks: &services.Hooks{}, OfferGate: gate,
	}
	parity := plus.NewModule(db, plus.Ports{Positions: positions, Employees: p.Employees, Audit: p.Audit})
	gate.Set(parity.Service())
	return &Module{
		db: db,
		controller: controllers.New(controllers.Services{Jobs: services.NewJobService(deps), Candidates: services.NewCandidateService(deps),
			Hire: services.NewHireService(deps), Interviews: services.NewInterviewService(deps), Offers: services.NewOfferService(deps)}),
		relay:     services.NewOutboxRelay(deps),
		hooks:     deps.Hooks,
		plus:      parity,
		gate:      gate,
		positions: positions,
	}
}

// orgDirectory adapts Module 1 services to services.OrgDirectory: not found → false, other errors propagate.
type orgDirectory struct {
	db     *gorm.DB
	depts  DepartmentReader
	desigs DesignationReader
}

func (o orgDirectory) DepartmentExists(ctx context.Context, tenantID, id uuid.UUID) (bool, error) {
	_, err := o.depts.GetDepartment(ctx, o.db, tenantID, id)
	return exists(err)
}

func (o orgDirectory) DesignationExists(ctx context.Context, tenantID, id uuid.UUID) (bool, error) {
	_, err := o.desigs.GetDesignation(ctx, o.db, tenantID, id)
	return exists(err)
}

func exists(err error) (bool, error) {
	if err == nil {
		return true, nil
	}
	if errors.Is(err, sharedErrors.ErrNotFound) {
		return false, nil
	}
	return false, err
}

// RegisterModels lists Module 6 models for legacy AutoMigrate (SQL 000045–000050 is the authority).
func (m *Module) RegisterModels() []interface{} {
	return append([]interface{}{&models.Job{}, &models.Candidate{}, &models.StageEvent{}, &models.Interview{}, &models.Offer{}, &models.OutboxEvent{}},
		m.plus.RegisterModels()...)
}

// RouteDeps is the Module 0 infrastructure the routes need.
type RouteDeps struct {
	Identity identityport.HTTP
}

// RegisterRoutes mounts Module 6 under rg.
func (m *Module) RegisterRoutes(rg *gin.RouterGroup, d RouteDeps) {
	routes.RegisterRoutes(rg, routes.Middleware{TenantResolver: d.Identity.TenantResolver(), Authenticate: d.Identity.Authenticate(),
		Require: func(action string) gin.HandlerFunc {
			return d.Identity.Require("recruitment", action)
		}}, m.controller)
	m.plus.RegisterRoutes(rg, d.Identity)
}

// Relay exposes the outbox relay.
func (m *Module) Relay() *services.OutboxRelay { return m.relay }

// OnHired registers a hook run inside the hire transaction (onboarding OB-03).
func (m *Module) OnHired(h recruitmentAPI.HireHook) { m.hooks.Add(h) }

// positionsSwitch is a core/api.Positions installed after construction (HR/Core is built after recruitment).
type positionsSwitch struct {
	mu  sync.RWMutex
	src coreAPI.Positions
}

func (p *positionsSwitch) PositionInfo(ctx context.Context, tenantID, positionID uuid.UUID) (coreAPI.PositionInfo, error) {
	p.mu.RLock()
	src := p.src
	p.mu.RUnlock()
	if src == nil {
		return coreAPI.PositionInfo{}, nil
	}
	return src.PositionInfo(ctx, tenantID, positionID)
}

// SetPositions installs the core position reader (requisitions RP-02).
func (m *Module) SetPositions(src coreAPI.Positions) {
	m.positions.mu.Lock()
	defer m.positions.mu.Unlock()
	m.positions.src = src
}

// ParityCatalogue is the permission HR/Recruitment parity adds (recruitment:approve).
func ParityCatalogue() []access.PermissionDef { return plus.Catalogue() }

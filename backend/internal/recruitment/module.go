// Package recruitment wires Module 6 (Recruitment & Applicant Tracking) into the application.
package recruitment

import (
	"context"
	"errors"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	identityMiddleware "github.com/jaas/jaas/internal/identity/middleware"
	identityModels "github.com/jaas/jaas/internal/identity/models"
	identityRepos "github.com/jaas/jaas/internal/identity/repositories"
	orgDTO "github.com/jaas/jaas/internal/organization/dto"
	"github.com/jaas/jaas/internal/recruitment/controllers"
	"github.com/jaas/jaas/internal/recruitment/models"
	"github.com/jaas/jaas/internal/recruitment/repositories"
	"github.com/jaas/jaas/internal/recruitment/routes"
	"github.com/jaas/jaas/internal/recruitment/services"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"github.com/jaas/jaas/internal/shared/queue"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Module is the Module 6 DI container.
type Module struct {
	db         *gorm.DB
	controller *controllers.Controller
	relay      *services.OutboxRelay
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
	deps := services.Deps{
		Tx: services.NewTxRunner(db),
		Repos: services.Repos{Jobs: repositories.NewJobRepository(), Candidates: repositories.NewCandidateRepository(),
			Interviews: repositories.NewInterviewRepository(), Offers: repositories.NewOfferRepository(), Outbox: repositories.NewOutboxRepository()},
		Employees: p.Employees, Users: p.Users, Org: orgDirectory{db: db, depts: p.Departments, desigs: p.Designations},
		Audit: p.Audit, Publisher: p.Publisher, Now: time.Now,
	}
	return &Module{
		db: db,
		controller: controllers.New(controllers.Services{Jobs: services.NewJobService(deps), Candidates: services.NewCandidateService(deps),
			Hire: services.NewHireService(deps), Interviews: services.NewInterviewService(deps), Offers: services.NewOfferService(deps)}),
		relay: services.NewOutboxRelay(deps),
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
	return []interface{}{&models.Job{}, &models.Candidate{}, &models.StageEvent{}, &models.Interview{}, &models.Offer{}, &models.OutboxEvent{}}
}

// SeedPermissions idempotently inserts recruitment:read|manage|hire (connections C7).
func (m *Module) SeedPermissions(ctx context.Context) error {
	for action, desc := range map[string]string{
		routes.ActionRead:   "View job openings, candidates, interviews and offers",
		routes.ActionManage: "Manage job openings, candidates, interviews and offers",
		routes.ActionHire:   "Hire candidates (creates a user invite and an employee profile)",
	} {
		d := desc
		err := m.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "resource"}, {Name: "action"}}, DoNothing: true}).
			Create(&identityModels.Permission{Resource: "recruitment", Action: action, Description: &d}).Error
		if err != nil {
			return err
		}
	}
	return nil
}

// RouteDeps is the Module 0 infrastructure the routes need.
type RouteDeps struct {
	TenantResolver, Authenticate gin.HandlerFunc
	UserRoles                    identityRepos.UserRoleRepository
	RolePerms                    identityRepos.RolePermissionRepository
}

// RegisterRoutes mounts Module 6 under rg.
func (m *Module) RegisterRoutes(rg *gin.RouterGroup, d RouteDeps) {
	routes.RegisterRoutes(rg, routes.Middleware{TenantResolver: d.TenantResolver, Authenticate: d.Authenticate,
		Require: func(action string) gin.HandlerFunc {
			return identityMiddleware.RequirePermission(m.db, "recruitment", action, d.UserRoles, d.RolePerms)
		}}, m.controller)
}

// Relay exposes the outbox relay.
func (m *Module) Relay() *services.OutboxRelay { return m.relay }

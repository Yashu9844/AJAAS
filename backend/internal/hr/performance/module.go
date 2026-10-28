// Package performance wires HR/Performance & OKRs (docs/modules/m1-hr-management/contexts/performance) into M1.
package performance

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jaas/jaas/internal/shared/identityport"
	"github.com/jaas/jaas/internal/hr/performance/api"
	"github.com/jaas/jaas/internal/hr/performance/controllers"
	"github.com/jaas/jaas/internal/hr/performance/models"
	"github.com/jaas/jaas/internal/hr/performance/repositories"
	"github.com/jaas/jaas/internal/hr/performance/routes"
	"github.com/jaas/jaas/internal/hr/performance/services"
	"github.com/jaas/jaas/internal/shared/queue"
	"gorm.io/gorm"
)

// Module is the HR/Performance DI container.
type Module struct {
	controller *controllers.Controller
	relay      *services.OutboxRelay
	ratings    *services.RatingsPort
}

// Ports are the collaborators HR/Performance consumes (design.md connections C2–C4).
type Ports struct {
	Employees services.EmployeeDirectory // HR/Employee directory
	Managers  services.ManagerDirectory  // HR/Organization direct manager (D7-03)
	Audit     services.AuditLogger       // M0 audit trail
	Publisher queue.EventPublisher
}

// NewModule wires repositories, services and the controller.
func NewModule(db *gorm.DB, p Ports) *Module {
	deps := services.Deps{
		Tx: services.NewTxRunner(db),
		Repos: services.Repos{Cycles: repositories.NewCycleRepository(), Goals: repositories.NewGoalRepository(),
			Reviews: repositories.NewReviewRepository(), Calibrations: repositories.NewCalibrationRepository(),
			Feedback: repositories.NewFeedbackRepository(), Outbox: repositories.NewOutboxRepository()},
		Employees: p.Employees, Managers: p.Managers, Audit: p.Audit, Publisher: p.Publisher, Now: time.Now,
	}
	return &Module{
		controller: controllers.New(controllers.Services{Cycles: services.NewCycleService(deps), Goals: services.NewGoalService(deps),
			Reviews: services.NewReviewService(deps), Calibrations: services.NewCalibrationService(deps), Feedback: services.NewFeedbackService(deps)}),
		relay:   services.NewOutboxRelay(deps),
		ratings: services.NewRatingsPort(deps),
	}
}

// RegisterModels lists HR/Performance models for legacy AutoMigrate (the SQL migrations are the authority).
func (m *Module) RegisterModels() []interface{} {
	return []interface{}{&models.Cycle{}, &models.Goal{}, &models.KeyResult{}, &models.CheckIn{}, &models.Review{}, &models.Calibration{},
		&models.FeedbackRequest{}, &models.Feedback{}, &models.OutboxEvent{}}
}

// RouteDeps is the M0 infrastructure the routes need.
type RouteDeps struct {
	Identity identityport.HTTP
}

// RegisterRoutes mounts HR/Performance under rg.
func (m *Module) RegisterRoutes(rg *gin.RouterGroup, d RouteDeps) {
	routes.RegisterRoutes(rg, routes.Middleware{TenantResolver: d.Identity.TenantResolver(), Authenticate: d.Identity.Authenticate(),
		Require: func(action string) gin.HandlerFunc { return d.Identity.Require("performance", action) },
		Flag:    func(action, key string) gin.HandlerFunc { return d.Identity.Flag("performance", action, key) },
	}, m.controller)
}

// Relay exposes the outbox relay.
func (m *Module) Relay() *services.OutboxRelay { return m.relay }

// Ratings is the final-rating port the other HR contexts use (performance/api).
func (m *Module) Ratings() api.Ratings { return m.ratings }

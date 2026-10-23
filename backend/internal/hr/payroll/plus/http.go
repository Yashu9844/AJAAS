package plus

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/hr/identityport"
	onboardingAPI "github.com/jaas/jaas/internal/hr/onboarding/api"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"gorm.io/gorm"
)

// Module is the payroll parity container.
type Module struct {
	svc *Service
	pt  *PT
}

// NewModule wires the store, PT source and service.
func NewModule(db *gorm.DB, pt *PT, p Ports) *Module {
	return &Module{svc: NewService(NewStore(db), p, pt, time.Now), pt: pt}
}

// NewPTFor builds the PT source over a database handle (payroll needs it before its services exist).
func NewPTFor(db *gorm.DB) *PT { return NewPT(NewStore(db)) }

// RegisterModels lists models for legacy AutoMigrate (the SQL migrations are the authority).
func (m *Module) RegisterModels() []interface{} {
	return []interface{}{&Settlement{}, &SettlementLine{}}
}

// SetClearance installs the exit-clearance port once HR/Onboarding exists (PP-07).
func (m *Module) SetClearance(c onboardingAPI.Clearance) { m.svc.p.Clearance = c }

// Service exposes the service (tests).
func (m *Module) Service() *Service { return m.svc }

func fail(c *gin.Context, err error) {
	err = sharedErrors.Normalize(err)
	var app *sharedErrors.AppError
	if errors.As(err, &app) {
		c.JSON(app.StatusCode, gin.H{"error": gin.H{"code": app.Code, "message": app.Message}})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": sharedErrors.ErrInternal.Code, "message": sharedErrors.ErrInternal.Message}})
}

func respond(c *gin.Context, status int, data interface{}, err error) {
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(status, gin.H{"data": data})
}

func actor(c *gin.Context) (Actor, bool) {
	t, okT := c.Get("tenant_id")
	u, okU := c.Get("user_id")
	tid, ok1 := t.(uuid.UUID)
	uid, ok2 := u.(uuid.UUID)
	if !okT || !okU || !ok1 || !ok2 {
		fail(c, sharedErrors.ErrUnauthorized)
		return Actor{}, false
	}
	return Actor{TenantID: tid, UserID: uid, IP: c.ClientIP(), UserAgent: c.Request.UserAgent()}, true
}

func badRequest(c *gin.Context, msg string) {
	fail(c, &sharedErrors.AppError{Code: sharedErrors.ErrValidation.Code, Message: msg, StatusCode: http.StatusBadRequest})
}

func withID(c *gin.Context) (Actor, uuid.UUID, bool) {
	a, ok := actor(c)
	if !ok {
		return a, uuid.Nil, false
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		badRequest(c, "id must be a UUID")
		return a, uuid.Nil, false
	}
	return a, id, true
}

func bind(c *gin.Context, dst interface{}) bool {
	if err := c.ShouldBindJSON(dst); err != nil {
		badRequest(c, "malformed JSON body")
		return false
	}
	return true
}

// RegisterRoutes mounts the 11 operations under rg (/payroll).
func (m *Module) RegisterRoutes(rg *gin.RouterGroup, idp identityport.HTTP) {
	s := m.svc
	read, manage, approve := idp.Require("payroll", "read"), idp.Require("payroll", "manage"), idp.Require("payroll", "approve")
	g := rg.Group("/payroll", idp.TenantResolver(), idp.Authenticate())

	g.GET("/pt-slabs", read, func(c *gin.Context) {
		if a, ok := actor(c); ok {
			res, err := s.PTSlabs(c.Request.Context(), a)
			respond(c, http.StatusOK, res, err)
		}
	})
	g.PUT("/pt-slabs/:state", manage, func(c *gin.Context) {
		a, ok := actor(c)
		var in struct {
			Slabs []SlabInput `json:"slabs"`
		}
		if ok && bind(c, &in) {
			res, err := s.SetPTSlabs(c.Request.Context(), a, c.Param("state"), in.Slabs)
			respond(c, http.StatusOK, res, err)
		}
	})
	g.DELETE("/pt-slabs/:state", manage, func(c *gin.Context) {
		if a, ok := actor(c); ok {
			if err := s.DeletePTSlabs(c.Request.Context(), a, c.Param("state")); err != nil {
				fail(c, err)
				return
			}
			c.Status(http.StatusNoContent)
		}
	})

	g.POST("/settlements", manage, func(c *gin.Context) {
		a, ok := actor(c)
		var in struct {
			EmployeeID string `json:"employee_id"`
		}
		if ok && bind(c, &in) {
			res, err := s.Calculate(c.Request.Context(), a, in.EmployeeID)
			respond(c, http.StatusCreated, res, err)
		}
	})
	g.GET("/settlements", read, func(c *gin.Context) {
		a, ok := actor(c)
		if !ok {
			return
		}
		var emp *uuid.UUID
		if raw := c.Query("employee_id"); raw != "" {
			id, err := uuid.Parse(raw)
			if err != nil {
				badRequest(c, "employee_id must be a UUID")
				return
			}
			emp = &id
		}
		res, err := s.List(c.Request.Context(), a, emp, c.Query("status"))
		respond(c, http.StatusOK, res, err)
	})
	g.GET("/settlements/me", func(c *gin.Context) {
		if a, ok := actor(c); ok {
			res, err := s.Mine(c.Request.Context(), a)
			respond(c, http.StatusOK, res, err)
		}
	})
	g.GET("/settlements/:id", read, func(c *gin.Context) {
		if a, id, ok := withID(c); ok {
			res, err := s.Get(c.Request.Context(), a, id)
			respond(c, http.StatusOK, res, err)
		}
	})
	g.POST("/settlements/:id/recalculate", manage, func(c *gin.Context) {
		if a, id, ok := withID(c); ok {
			res, err := s.Recalculate(c.Request.Context(), a, id)
			respond(c, http.StatusOK, res, err)
		}
	})
	g.POST("/settlements/:id/approve", approve, func(c *gin.Context) {
		if a, id, ok := withID(c); ok {
			res, err := s.Approve(c.Request.Context(), a, id)
			respond(c, http.StatusOK, res, err)
		}
	})
	g.POST("/settlements/:id/pay", manage, func(c *gin.Context) {
		if a, id, ok := withID(c); ok {
			res, err := s.Pay(c.Request.Context(), a, id)
			respond(c, http.StatusOK, res, err)
		}
	})
	g.POST("/settlements/:id/cancel", manage, func(c *gin.Context) {
		if a, id, ok := withID(c); ok {
			res, err := s.Cancel(c.Request.Context(), a, id)
			respond(c, http.StatusOK, res, err)
		}
	})
}

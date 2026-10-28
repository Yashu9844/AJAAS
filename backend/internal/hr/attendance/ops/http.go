package ops

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/shared/identityport"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"gorm.io/gorm"
)

// flagApprove records whether the caller holds attendance:approve at tenant scope.
const flagApprove = "attendance_approve_all"

// Module is the parity container.
type Module struct{ svc *Service }

// NewModule wires the store and service.
func NewModule(db *gorm.DB, p Ports) *Module {
	return &Module{svc: NewService(NewStore(db), p, time.Now)}
}

// RegisterModels lists models for legacy AutoMigrate (the SQL migrations are the authority).
func (m *Module) RegisterModels() []interface{} {
	return []interface{}{&Policy{}, &Roster{}, &ImportLog{}, &JobRun{}}
}

// Service exposes the service (background sweep).
func (m *Module) Service() *Service { return m.svc }

// RunAutoAbsent runs the daily auto-absent sweep until ctx is cancelled (AP-08).
func (m *Module) RunAutoAbsent(ctx context.Context, onError func(error)) {
	m.svc.Run(ctx, sweepInterval, onError)
}

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
	return Actor{TenantID: tid, UserID: uid, IP: c.ClientIP(), UserAgent: c.Request.UserAgent(), CanApproveAll: c.GetBool(flagApprove)}, true
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

func optionalBody(c *gin.Context, dst interface{}) bool {
	return c.Request.ContentLength <= 0 || bind(c, dst)
}

// RegisterRoutes mounts the 15 operations under rg (/attendance).
func (m *Module) RegisterRoutes(rg *gin.RouterGroup, idp identityport.HTTP) {
	s := m.svc
	read, manage := idp.Require("attendance", "read"), idp.Require("attendance", "manage")
	g := rg.Group("/attendance", idp.TenantResolver(), idp.Authenticate(), idp.Flag("attendance", "approve", flagApprove))

	g.GET("/inbox", func(c *gin.Context) {
		if a, ok := actor(c); ok {
			res, err := s.Inbox(c.Request.Context(), a, c.Query("scope") == "all")
			respond(c, http.StatusOK, res, err)
		}
	})
	decide := func(approve bool) gin.HandlerFunc {
		return func(c *gin.Context) {
			a, id, ok := withID(c)
			var b struct {
				Comment *string `json:"comment"`
			}
			if ok && optionalBody(c, &b) {
				res, err := s.Decide(c.Request.Context(), a, id, approve, b.Comment)
				respond(c, http.StatusOK, res, err)
			}
		}
	}
	g.POST("/inbox/:id/approve", decide(true))
	g.POST("/inbox/:id/reject", decide(false))

	g.PUT("/policy", manage, func(c *gin.Context) {
		a, ok := actor(c)
		var in PolicyInput
		if ok && bind(c, &in) {
			res, err := s.SetPolicy(c.Request.Context(), a, in)
			respond(c, http.StatusOK, res, err)
		}
	})
	g.GET("/policy", read, func(c *gin.Context) {
		if a, ok := actor(c); ok {
			res, err := s.GetPolicy(c.Request.Context(), a)
			respond(c, http.StatusOK, res, err)
		}
	})
	g.GET("/overtime/report", read, func(c *gin.Context) {
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
		res, err := s.Overtime(c.Request.Context(), a, c.Query("month"), emp)
		respond(c, http.StatusOK, res, err)
	})

	g.POST("/rosters", manage, func(c *gin.Context) {
		a, ok := actor(c)
		var in RosterInput
		if ok && bind(c, &in) {
			res, err := s.CreateRoster(c.Request.Context(), a, in)
			respond(c, http.StatusCreated, res, err)
		}
	})
	g.GET("/rosters", read, func(c *gin.Context) {
		if a, ok := actor(c); ok {
			res, err := s.Rosters(c.Request.Context(), a, c.Query("status"))
			respond(c, http.StatusOK, res, err)
		}
	})
	g.GET("/rosters/:id", read, func(c *gin.Context) {
		if a, id, ok := withID(c); ok {
			res, err := s.GetRoster(c.Request.Context(), a, id)
			respond(c, http.StatusOK, res, err)
		}
	})
	g.PATCH("/rosters/:id", manage, func(c *gin.Context) {
		a, id, ok := withID(c)
		var in RosterInput
		if ok && bind(c, &in) {
			res, err := s.UpdateRoster(c.Request.Context(), a, id, in)
			respond(c, http.StatusOK, res, err)
		}
	})
	g.POST("/rosters/:id/apply", manage, func(c *gin.Context) {
		a, id, ok := withID(c)
		var in ApplyInput
		if ok && bind(c, &in) {
			res, err := s.ApplyRoster(c.Request.Context(), a, id, in)
			respond(c, http.StatusOK, res, err)
		}
	})

	g.POST("/jobs/auto-absent", manage, func(c *gin.Context) {
		a, ok := actor(c)
		var b struct {
			Date string `json:"date"`
		}
		if ok && optionalBody(c, &b) {
			res, err := s.AutoAbsent(c.Request.Context(), a, b.Date)
			respond(c, http.StatusOK, res, err)
		}
	})
	g.GET("/jobs/runs", read, func(c *gin.Context) {
		if a, ok := actor(c); ok {
			res, err := s.Runs(c.Request.Context(), a)
			respond(c, http.StatusOK, res, err)
		}
	})

	g.POST("/imports", manage, func(c *gin.Context) {
		a, ok := actor(c)
		var in ImportInput
		if ok && bind(c, &in) {
			res, err := s.Import(c.Request.Context(), a, in)
			respond(c, http.StatusOK, res, err)
		}
	})
	g.GET("/imports", read, func(c *gin.Context) {
		if a, ok := actor(c); ok {
			res, err := s.Imports(c.Request.Context(), a)
			respond(c, http.StatusOK, res, err)
		}
	})
}

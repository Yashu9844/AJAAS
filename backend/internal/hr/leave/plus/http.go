package plus

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/hr/identityport"
	"github.com/jaas/jaas/internal/hr/leave/api"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"gorm.io/gorm"
)

// Flag keys for leave:read / manage / approve at tenant scope.
const (
	flagRead    = "leave_plus_read"
	flagManage  = "leave_plus_manage"
	flagApprove = "leave_plus_approve"
)

// Module is the parity container.
type Module struct {
	svc *Service
	cal *Calendar
}

// NewModule wires the store, calendar and service.
func NewModule(db *gorm.DB, cal *Calendar, p Ports) *Module {
	st := NewStore(db)
	return &Module{svc: NewService(st, p, cal, time.Now), cal: cal}
}

// NewCalendarFor builds the calendar over a database handle (the leave module needs it before its services exist).
func NewCalendarFor(db *gorm.DB) *Calendar { return NewCalendar(NewStore(db)) }

// RegisterModels lists models for legacy AutoMigrate (the SQL migrations are the authority).
func (m *Module) RegisterModels() []interface{} {
	return []interface{}{&Policy{}, &LocationHoliday{}, &WeekOff{}, &CompOff{}, &EncashmentRule{}, &Encashment{}}
}

// Service exposes the service (background sweep and tests).
func (m *Module) Service() *Service { return m.svc }

// RunCompOffExpiry runs the daily comp-off expiry sweep until ctx is cancelled (LP-10).
func (m *Module) RunCompOffExpiry(ctx context.Context, onError func(error)) {
	m.svc.Run(ctx, compOffSweep, onError)
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
	return Actor{TenantID: tid, UserID: uid, IP: c.ClientIP(), UserAgent: c.Request.UserAgent(), CanRead: c.GetBool(flagRead),
		CanManage: c.GetBool(flagManage), CanApproveAll: c.GetBool(flagApprove)}, true
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

func optionalUUID(c *gin.Context, name string) (*uuid.UUID, bool) {
	raw := c.Query(name)
	if raw == "" {
		return nil, true
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		badRequest(c, name+" must be a UUID")
		return nil, false
	}
	return &id, true
}

type commentBody struct {
	Comment *string `json:"comment"`
}

// RegisterRoutes mounts the 27 operations under rg (/leave).
func (m *Module) RegisterRoutes(rg *gin.RouterGroup, idp identityport.HTTP) {
	s := m.svc
	read, manage := idp.Require("leave", "read"), idp.Require("leave", "manage")
	g := rg.Group("/leave", idp.TenantResolver(), idp.Authenticate(), idp.Flag("leave", "read", flagRead), idp.Flag("leave", "manage", flagManage),
		idp.Flag("leave", "approve", flagApprove))

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

	g.POST("/location-holidays", manage, func(c *gin.Context) {
		a, ok := actor(c)
		var in LocationHolidayInput
		if ok && bind(c, &in) {
			res, err := s.CreateLocationHoliday(c.Request.Context(), a, in)
			respond(c, http.StatusCreated, res, err)
		}
	})
	g.GET("/location-holidays", read, func(c *gin.Context) {
		if a, ok := actor(c); ok {
			res, err := s.LocationHolidays(c.Request.Context(), a, c.Query("location_id"), c.Query("year"))
			respond(c, http.StatusOK, res, err)
		}
	})
	g.DELETE("/location-holidays/:id", manage, func(c *gin.Context) {
		if a, id, ok := withID(c); ok {
			if err := s.DeleteLocationHoliday(c.Request.Context(), a, id); err != nil {
				fail(c, err)
				return
			}
			c.Status(http.StatusNoContent)
		}
	})
	g.PUT("/week-offs/:id", manage, func(c *gin.Context) {
		a, ok := actor(c)
		var in struct {
			Days []int `json:"week_off_days"`
		}
		if ok && bind(c, &in) {
			res, err := s.SetWeekOff(c.Request.Context(), a, c.Param("id"), in.Days)
			respond(c, http.StatusOK, res, err)
		}
	})
	g.GET("/week-offs", read, func(c *gin.Context) {
		if a, ok := actor(c); ok {
			res, err := s.WeekOffs(c.Request.Context(), a)
			respond(c, http.StatusOK, res, err)
		}
	})
	g.DELETE("/week-offs/:id", manage, func(c *gin.Context) {
		if a, ok := actor(c); ok {
			if err := s.DeleteWeekOff(c.Request.Context(), a, c.Param("id")); err != nil {
				fail(c, err)
				return
			}
			c.Status(http.StatusNoContent)
		}
	})

	g.GET("/inbox", func(c *gin.Context) {
		if a, ok := actor(c); ok {
			res, err := s.Inbox(c.Request.Context(), a, c.Query("scope") == "all")
			respond(c, http.StatusOK, res, err)
		}
	})
	decide := func(approve bool) gin.HandlerFunc {
		return func(c *gin.Context) {
			a, id, ok := withID(c)
			var b commentBody
			if ok && optionalBody(c, &b) {
				res, err := s.Decide(c.Request.Context(), a, id, approve, b.Comment)
				respond(c, http.StatusOK, res, err)
			}
		}
	}
	g.POST("/inbox/:id/approve", decide(true))
	g.POST("/inbox/:id/reject", decide(false))
	g.GET("/team-calendar", func(c *gin.Context) {
		if a, ok := actor(c); ok {
			res, err := s.Calendar(c.Request.Context(), a, c.Query("from"), c.Query("to"))
			respond(c, http.StatusOK, res, err)
		}
	})

	g.POST("/comp-off", func(c *gin.Context) {
		a, ok := actor(c)
		var in CompOffInput
		if ok && bind(c, &in) {
			res, err := s.RequestCompOff(c.Request.Context(), a, in)
			respond(c, http.StatusCreated, res, err)
		}
	})
	g.GET("/comp-off/me", func(c *gin.Context) {
		if a, ok := actor(c); ok {
			res, err := s.MyCompOffs(c.Request.Context(), a)
			respond(c, http.StatusOK, res, err)
		}
	})
	g.GET("/comp-off", read, func(c *gin.Context) {
		a, ok := actor(c)
		emp, okE := optionalUUID(c, "employee_id")
		if ok && okE {
			res, err := s.CompOffs(c.Request.Context(), a, emp, c.Query("status"))
			respond(c, http.StatusOK, res, err)
		}
	})
	g.POST("/comp-off/expire", manage, func(c *gin.Context) {
		if a, ok := actor(c); ok {
			res, err := s.Expire(c.Request.Context(), a)
			respond(c, http.StatusOK, res, err)
		}
	})
	decideCompOff := func(to string) gin.HandlerFunc {
		return func(c *gin.Context) {
			a, id, ok := withID(c)
			var b commentBody
			if ok && optionalBody(c, &b) {
				res, err := s.DecideCompOff(c.Request.Context(), a, id, to, b.Comment)
				respond(c, http.StatusOK, res, err)
			}
		}
	}
	g.POST("/comp-off/:id/approve", decideCompOff(StatusApproved))
	g.POST("/comp-off/:id/reject", decideCompOff(StatusRejected))
	g.POST("/comp-off/:id/cancel", decideCompOff(StatusCancelled))

	g.PUT("/encashment-rules/:id", manage, func(c *gin.Context) {
		a, ok := actor(c)
		var in RuleInput
		if ok && bind(c, &in) {
			res, err := s.SetRule(c.Request.Context(), a, c.Param("id"), in)
			respond(c, http.StatusOK, res, err)
		}
	})
	g.GET("/encashment-rules", read, func(c *gin.Context) {
		if a, ok := actor(c); ok {
			res, err := s.Rules(c.Request.Context(), a)
			respond(c, http.StatusOK, res, err)
		}
	})
	g.POST("/encashments", func(c *gin.Context) {
		a, ok := actor(c)
		var in EncashInput
		if ok && bind(c, &in) {
			res, err := s.RequestEncashment(c.Request.Context(), a, in)
			respond(c, http.StatusCreated, res, err)
		}
	})
	g.GET("/encashments/me", func(c *gin.Context) {
		if a, ok := actor(c); ok {
			res, err := s.MyEncashments(c.Request.Context(), a)
			respond(c, http.StatusOK, res, err)
		}
	})
	g.GET("/encashments", read, func(c *gin.Context) {
		a, ok := actor(c)
		emp, okE := optionalUUID(c, "employee_id")
		typ, okT := optionalUUID(c, "leave_type_id")
		if ok && okE && okT {
			res, err := s.Encashments(c.Request.Context(), a, emp, typ, c.Query("status"))
			respond(c, http.StatusOK, res, err)
		}
	})
	decideEncash := func(to string) gin.HandlerFunc {
		return func(c *gin.Context) {
			a, id, ok := withID(c)
			var b commentBody
			if ok && optionalBody(c, &b) {
				res, err := s.DecideEncashment(c.Request.Context(), a, id, to, b.Comment)
				respond(c, http.StatusOK, res, err)
			}
		}
	}
	g.POST("/encashments/:id/approve", decideEncash(StatusApproved))
	g.POST("/encashments/:id/reject", decideEncash(StatusRejected))
	g.POST("/encashments/:id/cancel", decideEncash(StatusCancelled))
}

// Settlement is the leave/api.Settlement port (payroll final settlement).
func (m *Module) Settlement() api.Settlement { return SettlementPort(m.svc.store) }

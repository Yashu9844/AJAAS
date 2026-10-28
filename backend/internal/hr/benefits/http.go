package benefits

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/identity/access"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"github.com/jaas/jaas/internal/shared/identityport"
	"github.com/jaas/jaas/internal/shared/utils"
	"gorm.io/gorm"
)

// Flag keys for benefits:read / manage / read_sensitive.
const (
	flagRead      = "benefits_read"
	flagManage    = "benefits_manage"
	flagSensitive = "benefits_read_sensitive"
)

// Catalogue is HR/Benefits' permission catalogue (docs/ARCHITECTURE.md §17).
func Catalogue() []access.PermissionDef {
	return []access.PermissionDef{
		{Module: "hr", Resource: "benefits", Action: "read", Description: "View benefit plans, windows, settings and the enrolment report",
			Defaults: map[string]access.Scope{access.TemplateHRManager: access.ScopeTenant, access.TemplateHRAdmin: access.ScopeTenant, access.TemplatePayrollAdmin: access.ScopeTenant}},
		{Module: "hr", Resource: "benefits", Action: "manage", Description: "Manage benefit plans, versions, windows and settings; end a worker's election",
			Defaults: map[string]access.Scope{access.TemplateHRAdmin: access.ScopeTenant}},
		{Module: "hr", Resource: "benefits", Action: "read_sensitive", Description: "View a worker's elections, dependants and life events", Sensitive: true,
			Defaults: map[string]access.Scope{access.TemplateHRAdmin: access.ScopeTenant}},
	}
}

// Module is the HR/Benefits container.
type Module struct{ svc *Service }

// NewModule wires the store and service.
func NewModule(db *gorm.DB, p Ports) *Module {
	return &Module{svc: NewService(NewStore(db), p, time.Now)}
}

// Service exposes the service (the core action hook, the payroll deduction source).
func (m *Module) Service() *Service { return m.svc }

// RegisterModels lists models for legacy AutoMigrate (the SQL migrations are the authority).
func (m *Module) RegisterModels() []interface{} {
	return []interface{}{&Plan{}, &PlanVersion{}, &Window{}, &Settings{}, &Dependant{}, &LifeEvent{}, &Election{}}
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
	return Actor{TenantID: tid, UserID: uid, IP: c.ClientIP(), UserAgent: c.Request.UserAgent(), CanRead: c.GetBool(flagRead), CanManage: c.GetBool(flagManage),
		CanSensitiveRead: c.GetBool(flagSensitive)}, true
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

func optionalUUIDQuery(c *gin.Context, name string) (*uuid.UUID, bool) {
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

func paged[T any](c *gin.Context, rows []T, page, perPage int, total int64, err error) {
	if err != nil {
		fail(c, err)
		return
	}
	if rows == nil {
		rows = []T{}
	}
	c.JSON(http.StatusOK, gin.H{"data": rows, "meta": gin.H{"page": page, "per_page": perPage, "total_items": total,
		"total_pages": int((total + int64(perPage) - 1) / int64(perPage))}})
}

// RegisterRoutes mounts the 29 operations under rg (/benefits).
func (m *Module) RegisterRoutes(rg *gin.RouterGroup, idp identityport.HTTP) {
	s := m.svc
	read, manage, sensitive := idp.Require("benefits", "read"), idp.Require("benefits", "manage"), idp.Require("benefits", "read_sensitive")
	g := rg.Group("/benefits", idp.TenantResolver(), idp.Authenticate(), idp.Flag("benefits", "read", flagRead), idp.Flag("benefits", "manage", flagManage),
		idp.Flag("benefits", "read_sensitive", flagSensitive))

	// plans
	g.POST("/plans", manage, func(c *gin.Context) {
		a, ok := actor(c)
		var in PlanInput
		if ok && bind(c, &in) {
			res, err := s.CreatePlan(c.Request.Context(), a, in)
			respond(c, http.StatusCreated, res, err)
		}
	})
	g.GET("/plans", read, func(c *gin.Context) {
		if a, ok := actor(c); ok {
			page, perPage := utils.ParsePagination(c)
			rows, total, err := s.Plans(c.Request.Context(), a, c.Query("status"), page, perPage)
			paged(c, rows, page, perPage, total, err)
		}
	})
	g.GET("/plans/:id", read, func(c *gin.Context) {
		if a, id, ok := withID(c); ok {
			res, err := s.GetPlan(c.Request.Context(), a, id)
			respond(c, http.StatusOK, res, err)
		}
	})
	g.PATCH("/plans/:id", manage, func(c *gin.Context) {
		a, id, ok := withID(c)
		var in PlanInput
		if ok && bind(c, &in) {
			res, err := s.UpdatePlan(c.Request.Context(), a, id, in)
			respond(c, http.StatusOK, res, err)
		}
	})
	g.POST("/plans/:id/versions", manage, func(c *gin.Context) {
		a, id, ok := withID(c)
		var in VersionInput
		if ok && bind(c, &in) {
			res, err := s.AddVersion(c.Request.Context(), a, id, in)
			respond(c, http.StatusCreated, res, err)
		}
	})
	g.POST("/plans/:id/activate", manage, func(c *gin.Context) {
		if a, id, ok := withID(c); ok {
			res, err := s.ActivatePlan(c.Request.Context(), a, id)
			respond(c, http.StatusOK, res, err)
		}
	})
	g.POST("/plans/:id/retire", manage, func(c *gin.Context) {
		if a, id, ok := withID(c); ok {
			res, err := s.RetirePlan(c.Request.Context(), a, id)
			respond(c, http.StatusOK, res, err)
		}
	})

	// windows and settings
	g.POST("/windows", manage, func(c *gin.Context) {
		a, ok := actor(c)
		var in WindowInput
		if ok && bind(c, &in) {
			res, err := s.CreateWindow(c.Request.Context(), a, in)
			respond(c, http.StatusCreated, res, err)
		}
	})
	g.GET("/windows", read, func(c *gin.Context) {
		if a, ok := actor(c); ok {
			res, err := s.Windows(c.Request.Context(), a)
			respond(c, http.StatusOK, res, err)
		}
	})
	g.POST("/windows/:id/cancel", manage, func(c *gin.Context) {
		if a, id, ok := withID(c); ok {
			res, err := s.CancelWindow(c.Request.Context(), a, id)
			respond(c, http.StatusOK, res, err)
		}
	})
	g.GET("/settings", read, func(c *gin.Context) {
		if a, ok := actor(c); ok {
			res, err := s.GetSettings(c.Request.Context(), a)
			respond(c, http.StatusOK, res, err)
		}
	})
	g.PUT("/settings", manage, func(c *gin.Context) {
		a, ok := actor(c)
		var in SettingsInput
		if ok && bind(c, &in) {
			res, err := s.PutSettings(c.Request.Context(), a, in)
			respond(c, http.StatusOK, res, err)
		}
	})

	// elections
	g.GET("/options", func(c *gin.Context) {
		if a, ok := actor(c); ok {
			res, err := s.Options(c.Request.Context(), a)
			respond(c, http.StatusOK, res, err)
		}
	})
	g.GET("/elections/me", func(c *gin.Context) {
		if a, ok := actor(c); ok {
			res, err := s.MyElections(c.Request.Context(), a)
			respond(c, http.StatusOK, res, err)
		}
	})
	g.POST("/elections", func(c *gin.Context) {
		a, ok := actor(c)
		var in ElectInput
		if ok && bind(c, &in) {
			res, err := s.Elect(c.Request.Context(), a, in)
			respond(c, http.StatusCreated, res, err)
		}
	})
	g.POST("/elections/waive", func(c *gin.Context) {
		a, ok := actor(c)
		var in struct {
			PlanID string `json:"plan_id"`
		}
		if ok && bind(c, &in) {
			res, err := s.Waive(c.Request.Context(), a, in.PlanID)
			respond(c, http.StatusCreated, res, err)
		}
	})
	g.POST("/elections/:id/change", func(c *gin.Context) {
		a, id, ok := withID(c)
		var in ChangeInput
		if ok && bind(c, &in) {
			res, err := s.Change(c.Request.Context(), a, id, in)
			respond(c, http.StatusCreated, res, err)
		}
	})
	g.POST("/elections/:id/end", func(c *gin.Context) {
		if a, id, ok := withID(c); ok {
			res, err := s.End(c.Request.Context(), a, id)
			respond(c, http.StatusOK, res, err)
		}
	})
	g.GET("/elections", sensitive, func(c *gin.Context) {
		a, ok := actor(c)
		emp, ok1 := optionalUUIDQuery(c, "employee_id")
		plan, ok2 := optionalUUIDQuery(c, "plan_id")
		if ok && ok1 && ok2 {
			page, perPage := utils.ParsePagination(c)
			rows, total, err := s.Elections(c.Request.Context(), a, ElectionFilter{EmployeeID: emp, PlanID: plan, Status: c.Query("status")}, page, perPage)
			paged(c, rows, page, perPage, total, err)
		}
	})
	g.GET("/deductions/me", func(c *gin.Context) {
		if a, ok := actor(c); ok {
			res, err := s.MyDeductions(c.Request.Context(), a, c.Query("month"))
			respond(c, http.StatusOK, res, err)
		}
	})

	// dependants and life events (sensitive)
	g.POST("/dependants", func(c *gin.Context) {
		a, ok := actor(c)
		var in DependantInput
		if ok && bind(c, &in) {
			res, err := s.CreateDependant(c.Request.Context(), a, in)
			respond(c, http.StatusCreated, res, err)
		}
	})
	g.GET("/dependants", func(c *gin.Context) {
		if a, ok := actor(c); ok {
			res, err := s.MyDependants(c.Request.Context(), a)
			respond(c, http.StatusOK, res, err)
		}
	})
	g.PATCH("/dependants/:id", func(c *gin.Context) {
		a, id, ok := withID(c)
		var in DependantInput
		if ok && bind(c, &in) {
			res, err := s.UpdateDependant(c.Request.Context(), a, id, in)
			respond(c, http.StatusOK, res, err)
		}
	})
	g.POST("/dependants/:id/remove", func(c *gin.Context) {
		if a, id, ok := withID(c); ok {
			res, err := s.RemoveDependant(c.Request.Context(), a, id)
			respond(c, http.StatusOK, res, err)
		}
	})
	g.GET("/employees/:id/dependants", sensitive, func(c *gin.Context) {
		if a, id, ok := withID(c); ok {
			res, err := s.EmployeeDependants(c.Request.Context(), a, id)
			respond(c, http.StatusOK, res, err)
		}
	})
	g.POST("/life-events", func(c *gin.Context) {
		a, ok := actor(c)
		var in LifeEventInput
		if ok && bind(c, &in) {
			res, err := s.RecordLifeEvent(c.Request.Context(), a, in)
			respond(c, http.StatusCreated, res, err)
		}
	})
	g.GET("/life-events", func(c *gin.Context) {
		if a, ok := actor(c); ok {
			res, err := s.MyLifeEvents(c.Request.Context(), a)
			respond(c, http.StatusOK, res, err)
		}
	})
	g.GET("/employees/:id/life-events", sensitive, func(c *gin.Context) {
		if a, id, ok := withID(c); ok {
			res, err := s.EmployeeLifeEvents(c.Request.Context(), a, id)
			respond(c, http.StatusOK, res, err)
		}
	})

	// reports
	g.GET("/reports/enrolment", read, func(c *gin.Context) {
		if a, ok := actor(c); ok {
			res, err := s.EnrolmentReport(c.Request.Context(), a)
			respond(c, http.StatusOK, res, err)
		}
	})
}

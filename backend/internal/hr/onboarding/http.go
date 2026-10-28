package onboarding

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/shared/identityport"
	"github.com/jaas/jaas/internal/hr/onboarding/api"
	"github.com/jaas/jaas/internal/identity/access"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"github.com/jaas/jaas/internal/shared/utils"
	"gorm.io/gorm"
)

// Flag keys for onboarding:read / manage / exit_interview.
const (
	flagRead   = "onboarding_read"
	flagManage = "onboarding_manage"
	flagEI     = "onboarding_exit_interview"
)

// Catalogue is HR/Onboarding's permission catalogue (docs/ARCHITECTURE.md §17).
func Catalogue() []access.PermissionDef {
	return []access.PermissionDef{
		{Module: "hr", Resource: "onboarding", Action: "read", Description: "View every onboarding and offboarding checklist and the progress report",
			Defaults: map[string]access.Scope{access.TemplateHRAdmin: access.ScopeTenant}},
		{Module: "hr", Resource: "onboarding", Action: "manage", Description: "Manage templates and checklists, pool tasks and exit clearance",
			Defaults: map[string]access.Scope{access.TemplateHRAdmin: access.ScopeTenant}},
		{Module: "hr", Resource: "onboarding", Action: "exit_interview", Description: "Read exit-interview answers", Sensitive: true,
			Defaults: map[string]access.Scope{access.TemplateHRAdmin: access.ScopeTenant}},
	}
}

// Module is the HR/Onboarding container.
type Module struct{ svc *Service }

// NewModule wires the store and service.
func NewModule(db *gorm.DB, p Ports) *Module {
	return &Module{svc: NewService(NewStore(db), p, time.Now)}
}

// RegisterModels lists models for legacy AutoMigrate (the SQL migrations are the authority).
func (m *Module) RegisterModels() []interface{} {
	return []interface{}{&Template{}, &TemplateTask{}, &Checklist{}, &Task{}, &Clearance{}, &ExitInterview{}}
}

// Clearance is the port payroll uses before final settlement (onboarding/api).
func (m *Module) Clearance() api.Clearance { return m.svc }

// CoreHook and RecruitmentHook start and cancel checklists (OB-03); the HR root registers them.
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
	return Actor{TenantID: tid, UserID: uid, IP: c.ClientIP(), UserAgent: c.Request.UserAgent(), CanRead: c.GetBool(flagRead),
		CanManage: c.GetBool(flagManage), CanReadEI: c.GetBool(flagEI)}, true
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

// optionalBody binds a body when one was sent.
func optionalBody(c *gin.Context, dst interface{}) bool {
	return c.Request.ContentLength <= 0 || bind(c, dst)
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

type reasonBody struct {
	Reason *string `json:"reason"`
	Note   *string `json:"note"`
}

// RegisterRoutes mounts the 19 operations under rg (/onboarding).
func (m *Module) RegisterRoutes(rg *gin.RouterGroup, idp identityport.HTTP) {
	s := m.svc
	read, manage := idp.Require("onboarding", "read"), idp.Require("onboarding", "manage")
	g := rg.Group("/onboarding", idp.TenantResolver(), idp.Authenticate(), idp.Flag("onboarding", "read", flagRead),
		idp.Flag("onboarding", "manage", flagManage), idp.Flag("onboarding", "exit_interview", flagEI))

	g.POST("/templates", manage, func(c *gin.Context) {
		a, ok := actor(c)
		var in TemplateInput
		if ok && bind(c, &in) {
			res, err := s.CreateTemplate(c.Request.Context(), a, in)
			respond(c, http.StatusCreated, res, err)
		}
	})
	g.GET("/templates", read, func(c *gin.Context) {
		if a, ok := actor(c); ok {
			res, err := s.Templates(c.Request.Context(), a, c.Query("kind"), c.Query("status"))
			respond(c, http.StatusOK, res, err)
		}
	})
	g.GET("/templates/:id", read, func(c *gin.Context) {
		if a, id, ok := withID(c); ok {
			res, err := s.GetTemplate(c.Request.Context(), a, id)
			respond(c, http.StatusOK, res, err)
		}
	})
	g.PATCH("/templates/:id", manage, func(c *gin.Context) {
		a, id, ok := withID(c)
		var in TemplateInput
		if ok && bind(c, &in) {
			res, err := s.UpdateTemplate(c.Request.Context(), a, id, in)
			respond(c, http.StatusOK, res, err)
		}
	})
	g.PUT("/templates/:id/tasks", manage, func(c *gin.Context) {
		a, id, ok := withID(c)
		var in []TaskInput
		if ok && bind(c, &in) {
			res, err := s.SetTasks(c.Request.Context(), a, id, in)
			respond(c, http.StatusOK, res, err)
		}
	})

	g.POST("/checklists", manage, func(c *gin.Context) {
		a, ok := actor(c)
		var in StartInput
		if ok && bind(c, &in) {
			res, err := s.Start(c.Request.Context(), a, in)
			respond(c, http.StatusCreated, res, err)
		}
	})
	g.GET("/checklists", func(c *gin.Context) {
		a, ok := actor(c)
		emp, okE := optionalUUIDQuery(c, "employee_id")
		dept, okD := optionalUUIDQuery(c, "department_id")
		if !ok || !okE || !okD {
			return
		}
		page, perPage := utils.ParsePagination(c)
		rows, total, err := s.Checklists(c.Request.Context(), a, ChecklistFilter{Kind: c.Query("kind"), Status: c.Query("status"), EmployeeID: emp,
			DepartmentID: dept}, page, perPage)
		if err != nil {
			fail(c, err)
			return
		}
		if rows == nil {
			rows = []Checklist{}
		}
		c.JSON(http.StatusOK, gin.H{"data": rows, "meta": gin.H{"page": page, "per_page": perPage, "total_items": total,
			"total_pages": int((total + int64(perPage) - 1) / int64(perPage))}})
	})
	g.GET("/checklists/:id", func(c *gin.Context) {
		if a, id, ok := withID(c); ok {
			res, err := s.Get(c.Request.Context(), a, id)
			respond(c, http.StatusOK, res, err)
		}
	})
	g.POST("/checklists/:id/complete", manage, func(c *gin.Context) {
		if a, id, ok := withID(c); ok {
			res, err := s.Complete(c.Request.Context(), a, id)
			respond(c, http.StatusOK, res, err)
		}
	})
	g.POST("/checklists/:id/cancel", manage, func(c *gin.Context) {
		a, id, ok := withID(c)
		var b reasonBody
		if ok && optionalBody(c, &b) {
			res, err := s.Cancel(c.Request.Context(), a, id, b.Reason)
			respond(c, http.StatusOK, res, err)
		}
	})
	g.POST("/checklists/:id/clearances/:area/clear", func(c *gin.Context) {
		a, id, ok := withID(c)
		var b reasonBody
		if ok && optionalBody(c, &b) {
			res, err := s.Clear(c.Request.Context(), a, id, c.Param("area"), b.Note)
			respond(c, http.StatusOK, res, err)
		}
	})
	g.PUT("/checklists/:id/exit-interview", func(c *gin.Context) {
		a, id, ok := withID(c)
		var b struct {
			Answers Answers `json:"answers"`
		}
		if ok && bind(c, &b) {
			res, err := s.SubmitExit(c.Request.Context(), a, id, b.Answers)
			respond(c, http.StatusCreated, res, err)
		}
	})
	g.GET("/checklists/:id/exit-interview", func(c *gin.Context) {
		if a, id, ok := withID(c); ok {
			res, err := s.GetExit(c.Request.Context(), a, id)
			respond(c, http.StatusOK, res, err)
		}
	})

	g.GET("/my-tasks", func(c *gin.Context) {
		if a, ok := actor(c); ok {
			res, err := s.MyTasks(c.Request.Context(), a)
			respond(c, http.StatusOK, res, err)
		}
	})
	g.POST("/tasks/:id/done", func(c *gin.Context) {
		a, id, ok := withID(c)
		var b reasonBody
		if ok && optionalBody(c, &b) {
			res, err := s.Done(c.Request.Context(), a, id, b.Note)
			respond(c, http.StatusOK, res, err)
		}
	})
	g.POST("/tasks/:id/skip", manage, func(c *gin.Context) {
		a, id, ok := withID(c)
		var b reasonBody
		if ok && optionalBody(c, &b) {
			res, err := s.Skip(c.Request.Context(), a, id, b.Reason)
			respond(c, http.StatusOK, res, err)
		}
	})
	g.POST("/tasks/:id/reopen", manage, func(c *gin.Context) {
		if a, id, ok := withID(c); ok {
			res, err := s.Reopen(c.Request.Context(), a, id)
			respond(c, http.StatusOK, res, err)
		}
	})

	g.GET("/me", func(c *gin.Context) {
		if a, ok := actor(c); ok {
			res, err := s.Me(c.Request.Context(), a)
			respond(c, http.StatusOK, res, err)
		}
	})
	g.GET("/reports/progress", read, func(c *gin.Context) {
		if a, ok := actor(c); ok {
			res, err := s.Progress(c.Request.Context(), a, c.Query("kind"))
			respond(c, http.StatusOK, res, err)
		}
	})
}

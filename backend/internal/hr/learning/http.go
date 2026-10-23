package learning

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/hr/identityport"
	"github.com/jaas/jaas/internal/identity/access"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"github.com/jaas/jaas/internal/shared/utils"
	"gorm.io/gorm"
)

// Flag keys for learning:read / learning:manage.
const (
	flagRead   = "learning_read"
	flagManage = "learning_manage"
)

// Catalogue is HR/Learning's permission catalogue (docs/ARCHITECTURE.md §17).
func Catalogue() []access.PermissionDef {
	return []access.PermissionDef{
		{Module: "hr", Resource: "learning", Action: "read", Description: "View every enrolment, assignment, plan and the compliance report",
			Defaults: map[string]access.Scope{access.TemplateHRManager: access.ScopeTenant, access.TemplateHRAdmin: access.ScopeTenant}},
		{Module: "hr", Resource: "learning", Action: "manage", Description: "Manage the course catalogue, plans, assignments, waivers and corrections",
			Defaults: map[string]access.Scope{access.TemplateHRAdmin: access.ScopeTenant}},
	}
}

// Module is the HR/Learning container.
type Module struct{ svc *Service }

// NewModule wires the store and service.
func NewModule(db *gorm.DB, p Ports) *Module {
	return &Module{svc: NewService(NewStore(db), p, time.Now)}
}

// Service exposes the service (the core action hook, recurrence).
func (m *Module) Service() *Service { return m.svc }

// RegisterModels lists models for legacy AutoMigrate (the SQL migrations are the authority).
func (m *Module) RegisterModels() []interface{} {
	return []interface{}{&Settings{}, &Course{}, &CourseVersion{}, &Plan{}, &PlanItem{}, &Assignment{}, &Enrolment{}, &Completion{}}
}

// RunRecurrence starts recurring cycles every hour until ctx ends (LN-C14).
func (m *Module) RunRecurrence(ctx context.Context, onErr func(error)) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		if _, err := m.svc.RunRecurrence(ctx, nil); err != nil && onErr != nil {
			onErr(err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
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
	return Actor{TenantID: tid, UserID: uid, IP: c.ClientIP(), UserAgent: c.Request.UserAgent(), CanRead: c.GetBool(flagRead), CanManage: c.GetBool(flagManage)}, true
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

// optionalBody binds a JSON body only when one was sent.
func optionalBody(c *gin.Context, dst interface{}) bool {
	return c.Request.ContentLength == 0 || bind(c, dst)
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

// RegisterRoutes mounts the 35 operations under rg (/learning).
func (m *Module) RegisterRoutes(rg *gin.RouterGroup, idp identityport.HTTP) {
	s := m.svc
	read, manage := idp.Require("learning", "read"), idp.Require("learning", "manage")
	g := rg.Group("/learning", idp.TenantResolver(), idp.Authenticate(), idp.Flag("learning", "read", flagRead), idp.Flag("learning", "manage", flagManage))

	// catalogue
	g.POST("/courses", manage, func(c *gin.Context) {
		a, ok := actor(c)
		var in CourseInput
		if ok && bind(c, &in) {
			res, err := s.CreateCourse(c.Request.Context(), a, in)
			respond(c, http.StatusCreated, res, err)
		}
	})
	g.GET("/courses", read, func(c *gin.Context) {
		if a, ok := actor(c); ok {
			page, perPage := utils.ParsePagination(c)
			rows, total, err := s.Courses(c.Request.Context(), a, c.Query("status"), page, perPage)
			paged(c, rows, page, perPage, total, err)
		}
	})
	g.GET("/courses/:id", read, func(c *gin.Context) {
		if a, id, ok := withID(c); ok {
			res, err := s.GetCourse(c.Request.Context(), a, id)
			respond(c, http.StatusOK, res, err)
		}
	})
	g.PATCH("/courses/:id", manage, func(c *gin.Context) {
		a, id, ok := withID(c)
		var in CourseInput
		if ok && bind(c, &in) {
			res, err := s.UpdateCourse(c.Request.Context(), a, id, in)
			respond(c, http.StatusOK, res, err)
		}
	})
	g.POST("/courses/:id/publish", manage, func(c *gin.Context) {
		if a, id, ok := withID(c); ok {
			res, err := s.PublishCourse(c.Request.Context(), a, id)
			respond(c, http.StatusOK, res, err)
		}
	})
	g.POST("/courses/:id/retire", manage, func(c *gin.Context) {
		if a, id, ok := withID(c); ok {
			res, err := s.RetireCourse(c.Request.Context(), a, id)
			respond(c, http.StatusOK, res, err)
		}
	})
	g.GET("/courses/:id/versions", read, func(c *gin.Context) {
		if a, id, ok := withID(c); ok {
			res, err := s.Versions(c.Request.Context(), a, id)
			respond(c, http.StatusOK, res, err)
		}
	})
	g.GET("/catalog", func(c *gin.Context) {
		if a, ok := actor(c); ok {
			page, perPage := utils.ParsePagination(c)
			rows, total, err := s.Catalog(c.Request.Context(), a, page, perPage)
			paged(c, rows, page, perPage, total, err)
		}
	})

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
	g.PUT("/plans/:id/items", manage, func(c *gin.Context) {
		a, id, ok := withID(c)
		var in struct {
			Items []ItemInput `json:"items"`
		}
		if ok && bind(c, &in) {
			res, err := s.SetItems(c.Request.Context(), a, id, in.Items)
			respond(c, http.StatusOK, res, err)
		}
	})
	g.GET("/plans/:id/progress", func(c *gin.Context) {
		a, id, ok := withID(c)
		emp, okE := optionalUUIDQuery(c, "employee_id")
		if ok && okE {
			res, err := s.Progress(c.Request.Context(), a, id, emp)
			respond(c, http.StatusOK, res, err)
		}
	})

	// assignments
	g.POST("/assignments", manage, func(c *gin.Context) {
		a, ok := actor(c)
		var in AssignmentInput
		if ok && bind(c, &in) {
			res, err := s.CreateAssignment(c.Request.Context(), a, in)
			respond(c, http.StatusCreated, res, err)
		}
	})
	g.GET("/assignments", read, func(c *gin.Context) {
		if a, ok := actor(c); ok {
			page, perPage := utils.ParsePagination(c)
			rows, total, err := s.Assignments(c.Request.Context(), a, c.Query("status"), page, perPage)
			paged(c, rows, page, perPage, total, err)
		}
	})
	g.GET("/assignments/:id", read, func(c *gin.Context) {
		if a, id, ok := withID(c); ok {
			res, err := s.GetAssignment(c.Request.Context(), a, id)
			respond(c, http.StatusOK, res, err)
		}
	})
	g.POST("/assignments/:id/cancel", manage, func(c *gin.Context) {
		if a, id, ok := withID(c); ok {
			res, err := s.CancelAssignment(c.Request.Context(), a, id)
			respond(c, http.StatusOK, res, err)
		}
	})

	// enrolments
	g.POST("/enrolments/self", func(c *gin.Context) {
		a, ok := actor(c)
		var in SelfEnrolInput
		if ok && bind(c, &in) {
			res, err := s.EnrolSelf(c.Request.Context(), a, in)
			respond(c, http.StatusCreated, res, err)
		}
	})
	g.GET("/enrolments/me", func(c *gin.Context) {
		if a, ok := actor(c); ok {
			page, perPage := utils.ParsePagination(c)
			rows, total, err := s.Mine(c.Request.Context(), a, c.Query("status"), page, perPage)
			paged(c, rows, page, perPage, total, err)
		}
	})
	g.GET("/enrolments/team", func(c *gin.Context) {
		if a, ok := actor(c); ok {
			page, perPage := utils.ParsePagination(c)
			rows, total, err := s.Team(c.Request.Context(), a, c.Query("status"), page, perPage)
			paged(c, rows, page, perPage, total, err)
		}
	})
	g.GET("/enrolments", read, func(c *gin.Context) {
		a, ok := actor(c)
		emp, ok1 := optionalUUIDQuery(c, "employee_id")
		course, ok2 := optionalUUIDQuery(c, "course_id")
		asn, ok3 := optionalUUIDQuery(c, "assignment_id")
		if ok && ok1 && ok2 && ok3 {
			page, perPage := utils.ParsePagination(c)
			rows, total, err := s.Enrolments(c.Request.Context(), a, EnrolFilter{EmployeeID: emp, CourseID: course, AssignmentID: asn, Status: c.Query("status")}, page, perPage)
			paged(c, rows, page, perPage, total, err)
		}
	})
	g.GET("/enrolments/:id", func(c *gin.Context) {
		if a, id, ok := withID(c); ok {
			res, err := s.GetEnrolment(c.Request.Context(), a, id)
			respond(c, http.StatusOK, res, err)
		}
	})
	g.POST("/enrolments/:id/start", func(c *gin.Context) {
		if a, id, ok := withID(c); ok {
			res, err := s.Start(c.Request.Context(), a, id)
			respond(c, http.StatusOK, res, err)
		}
	})
	g.POST("/enrolments/:id/complete", func(c *gin.Context) {
		a, id, ok := withID(c)
		var in CompleteInput
		if ok && optionalBody(c, &in) {
			res, err := s.Complete(c.Request.Context(), a, id, in)
			respond(c, http.StatusOK, res, err)
		}
	})
	g.POST("/enrolments/:id/waive", manage, func(c *gin.Context) {
		a, id, ok := withID(c)
		var in struct {
			Reason string `json:"reason"`
		}
		if ok && bind(c, &in) {
			res, err := s.Waive(c.Request.Context(), a, id, in.Reason)
			respond(c, http.StatusOK, res, err)
		}
	})
	g.POST("/enrolments/:id/correct", manage, func(c *gin.Context) {
		a, id, ok := withID(c)
		var in CorrectInput
		if ok && bind(c, &in) {
			res, err := s.Correct(c.Request.Context(), a, id, in)
			respond(c, http.StatusOK, res, err)
		}
	})
	g.GET("/transcript/me", func(c *gin.Context) {
		if a, ok := actor(c); ok {
			page, perPage := utils.ParsePagination(c)
			rows, total, err := s.Transcript(c.Request.Context(), a, page, perPage)
			paged(c, rows, page, perPage, total, err)
		}
	})

	// completions
	g.GET("/completions/pending", func(c *gin.Context) {
		if a, ok := actor(c); ok {
			res, err := s.Pending(c.Request.Context(), a)
			respond(c, http.StatusOK, res, err)
		}
	})
	g.POST("/completions/:id/verify", func(c *gin.Context) {
		if a, id, ok := withID(c); ok {
			res, err := s.Verify(c.Request.Context(), a, id)
			respond(c, http.StatusOK, res, err)
		}
	})
	g.POST("/completions/:id/reject", func(c *gin.Context) {
		a, id, ok := withID(c)
		var in struct {
			Comment string `json:"comment"`
		}
		if ok && optionalBody(c, &in) {
			res, err := s.Reject(c.Request.Context(), a, id, in.Comment)
			respond(c, http.StatusOK, res, err)
		}
	})

	// operations
	g.POST("/recurrence/run", manage, func(c *gin.Context) {
		if a, ok := actor(c); ok {
			res, err := s.RunRecurrence(c.Request.Context(), &a.TenantID)
			respond(c, http.StatusOK, res, err)
		}
	})
	g.GET("/reports/compliance", read, func(c *gin.Context) {
		a, ok := actor(c)
		dept, ok1 := optionalUUIDQuery(c, "department_id")
		course, ok2 := optionalUUIDQuery(c, "course_id")
		if !ok || !ok1 || !ok2 {
			return
		}
		f := ComplianceFilter{DepartmentID: dept, CourseID: course}
		switch c.Query("mandatory") {
		case "":
		case "true", "false":
			v := c.Query("mandatory") == "true"
			f.Mandatory = &v
		default:
			badRequest(c, "mandatory must be true or false")
			return
		}
		res, err := s.Compliance(c.Request.Context(), a, f)
		respond(c, http.StatusOK, res, err)
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
}

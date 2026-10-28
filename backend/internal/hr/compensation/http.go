package compensation

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

// Flag keys for compensation:read / compensation:manage.
const (
	flagRead   = "compensation_read"
	flagManage = "compensation_manage"
)

// Catalogue is HR/Compensation's permission catalogue (docs/ARCHITECTURE.md §17).
func Catalogue() []access.PermissionDef {
	return []access.PermissionDef{
		{Module: "hr", Resource: "compensation", Action: "read", Description: "View anyone's compensation, pay ranges and merit cycles", Sensitive: true,
			Defaults: map[string]access.Scope{access.TemplateHRAdmin: access.ScopeTenant, access.TemplatePayrollAdmin: access.ScopeTenant}},
		{Module: "hr", Resource: "compensation", Action: "manage", Description: "Manage pay ranges, off-cycle changes and merit cycles", Sensitive: true,
			Defaults: map[string]access.Scope{access.TemplateHRAdmin: access.ScopeTenant}},
		{Module: "hr", Resource: "compensation", Action: "approve", Description: "Approve compensation changes and merit cycles (never one's own)", Sensitive: true,
			Defaults: map[string]access.Scope{access.TemplateExecutive: access.ScopeTenant}},
	}
}

// Module is the HR/Compensation container.
type Module struct{ svc *Service }

// NewModule wires the store and service.
func NewModule(db *gorm.DB, p Ports) *Module {
	return &Module{svc: NewService(NewStore(db), p, time.Now)}
}

// RegisterModels lists models for legacy AutoMigrate (the SQL migrations are the authority).
func (m *Module) RegisterModels() []interface{} {
	return []interface{}{&PayRange{}, &Change{}, &Cycle{}, &Budget{}, &Proposal{}, &Record{}}
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

// withID reads the actor and :id.
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

// RegisterRoutes mounts the 24 operations under rg (/compensation).
func (m *Module) RegisterRoutes(rg *gin.RouterGroup, idp identityport.HTTP) {
	s := m.svc
	read, manage, approve := idp.Require("compensation", "read"), idp.Require("compensation", "manage"), idp.Require("compensation", "approve")
	g := rg.Group("/compensation", idp.TenantResolver(), idp.Authenticate(), idp.Flag("compensation", "read", flagRead),
		idp.Flag("compensation", "manage", flagManage))

	g.POST("/pay-ranges", manage, func(c *gin.Context) {
		a, ok := actor(c)
		var in RangeInput
		if ok && bind(c, &in) {
			res, err := s.CreateRange(c.Request.Context(), a, in)
			respond(c, http.StatusCreated, res, err)
		}
	})
	g.GET("/pay-ranges", read, func(c *gin.Context) {
		a, ok := actor(c)
		grade, okG := optionalUUIDQuery(c, "grade_id")
		if ok && okG {
			res, err := s.Ranges(c.Request.Context(), a, grade)
			respond(c, http.StatusOK, res, err)
		}
	})

	g.POST("/changes", manage, func(c *gin.Context) {
		a, ok := actor(c)
		var in ChangeInput
		if ok && bind(c, &in) {
			res, err := s.CreateChange(c.Request.Context(), a, in)
			respond(c, http.StatusCreated, res, err)
		}
	})
	g.GET("/changes", read, func(c *gin.Context) {
		a, ok := actor(c)
		emp, okE := optionalUUIDQuery(c, "employee_id")
		if ok && okE {
			page, perPage := utils.ParsePagination(c)
			rows, total, err := s.Changes(c.Request.Context(), a, ChangeFilter{EmployeeID: emp, Status: c.Query("status")}, page, perPage)
			paged(c, rows, page, perPage, total, err)
		}
	})
	g.GET("/changes/:id", read, func(c *gin.Context) {
		if a, id, ok := withID(c); ok {
			res, err := s.GetChange(c.Request.Context(), a, id)
			respond(c, http.StatusOK, res, err)
		}
	})
	decide := func(to string) gin.HandlerFunc {
		return func(c *gin.Context) {
			a, id, ok := withID(c)
			var body struct {
				Comment *string `json:"comment"`
			}
			if !ok || (c.Request.ContentLength > 0 && !bind(c, &body)) {
				return
			}
			res, err := s.DecideChange(c.Request.Context(), a, id, to, body.Comment)
			respond(c, http.StatusOK, res, err)
		}
	}
	g.POST("/changes/:id/approve", approve, decide(ChangeApproved))
	g.POST("/changes/:id/reject", approve, decide(ChangeRejected))
	g.POST("/changes/:id/cancel", decide(ChangeCancelled))

	g.GET("/workers/:id", read, func(c *gin.Context) {
		if a, id, ok := withID(c); ok {
			res, err := s.Worker(c.Request.Context(), a, id)
			respond(c, http.StatusOK, res, err)
		}
	})
	g.GET("/me", func(c *gin.Context) {
		if a, ok := actor(c); ok {
			res, err := s.Me(c.Request.Context(), a)
			respond(c, http.StatusOK, res, err)
		}
	})

	g.POST("/merit-cycles", manage, func(c *gin.Context) {
		a, ok := actor(c)
		var in CycleInput
		if ok && bind(c, &in) {
			res, err := s.CreateCycle(c.Request.Context(), a, in)
			respond(c, http.StatusCreated, res, err)
		}
	})
	g.GET("/merit-cycles", read, func(c *gin.Context) {
		if a, ok := actor(c); ok {
			page, perPage := utils.ParsePagination(c)
			rows, total, err := s.Cycles(c.Request.Context(), a, c.Query("status"), page, perPage)
			paged(c, rows, page, perPage, total, err)
		}
	})
	g.GET("/merit-cycles/:id", read, func(c *gin.Context) {
		if a, id, ok := withID(c); ok {
			res, err := s.GetCycle(c.Request.Context(), a, id)
			respond(c, http.StatusOK, res, err)
		}
	})
	g.PATCH("/merit-cycles/:id", manage, func(c *gin.Context) {
		a, id, ok := withID(c)
		var in CycleInput
		if ok && bind(c, &in) {
			res, err := s.UpdateCycle(c.Request.Context(), a, id, in)
			respond(c, http.StatusOK, res, err)
		}
	})
	g.PUT("/merit-cycles/:id/budgets", manage, func(c *gin.Context) {
		a, id, ok := withID(c)
		var in []BudgetInput
		if ok && bind(c, &in) {
			res, err := s.SetBudgets(c.Request.Context(), a, id, in)
			respond(c, http.StatusOK, res, err)
		}
	})
	g.GET("/merit-cycles/:id/budgets", read, func(c *gin.Context) {
		if a, id, ok := withID(c); ok {
			res, err := s.Budgets(c.Request.Context(), a, id)
			respond(c, http.StatusOK, res, err)
		}
	})
	move := func(to string) gin.HandlerFunc {
		return func(c *gin.Context) {
			if a, id, ok := withID(c); ok {
				res, err := s.Move(c.Request.Context(), a, id, to)
				respond(c, http.StatusOK, res, err)
			}
		}
	}
	g.POST("/merit-cycles/:id/open", manage, move(CycleOpen))
	g.POST("/merit-cycles/:id/close", manage, move(CycleInReview))
	g.POST("/merit-cycles/:id/approve", approve, move(CycleApproved))
	g.POST("/merit-cycles/:id/apply", manage, move(CycleApplied))
	g.GET("/merit-cycles/:id/usage", read, func(c *gin.Context) {
		if a, id, ok := withID(c); ok {
			res, err := s.Usage(c.Request.Context(), a, id)
			respond(c, http.StatusOK, res, err)
		}
	})
	g.GET("/merit-cycles/:id/proposals", func(c *gin.Context) {
		if a, id, ok := withID(c); ok {
			res, err := s.Proposals(c.Request.Context(), a, id, false)
			respond(c, http.StatusOK, res, err)
		}
	})
	g.GET("/merit-cycles/:id/proposals/mine", func(c *gin.Context) {
		if a, id, ok := withID(c); ok {
			res, err := s.Proposals(c.Request.Context(), a, id, true)
			respond(c, http.StatusOK, res, err)
		}
	})
	g.PATCH("/proposals/:id", func(c *gin.Context) {
		a, id, ok := withID(c)
		var in ProposalInput
		if ok && bind(c, &in) {
			res, err := s.UpdateProposal(c.Request.Context(), a, id, in)
			respond(c, http.StatusOK, res, err)
		}
	})
}

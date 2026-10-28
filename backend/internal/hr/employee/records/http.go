package records

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/shared/identityport"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"github.com/jaas/jaas/internal/shared/utils"
)

// sensitiveKey is where the employee:read_sensitive flag is stored for the request.
const sensitiveKey = "records_read_sensitive"

// Handler is the HTTP edge of FR-ER and FR-CR.
type Handler struct{ svc *Service }

// NewHandler builds the handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

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
	return Actor{TenantID: tid, UserID: uid, IP: c.ClientIP(), UserAgent: c.Request.UserAgent(), Sensitive: c.GetBool(sensitiveKey)}, true
}

func param(c *gin.Context, name string) (*uuid.UUID, bool) {
	raw := c.Param(name)
	if raw == "" {
		return nil, true
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		fail(c, &sharedErrors.AppError{Code: sharedErrors.ErrValidation.Code, Message: name + " must be a UUID", StatusCode: http.StatusBadRequest})
		return nil, false
	}
	return &id, true
}

func bind(c *gin.Context, dst interface{}) bool {
	if err := c.ShouldBindJSON(dst); err != nil {
		fail(c, &sharedErrors.AppError{Code: sharedErrors.ErrValidation.Code, Message: "malformed JSON body", StatusCode: http.StatusBadRequest})
		return false
	}
	return true
}

// ids reads the actor, the optional :id (employee) and the optional :rid (record).
func ids(c *gin.Context) (Actor, *uuid.UUID, *uuid.UUID, bool) {
	a, ok := actor(c)
	if !ok {
		return a, nil, nil, false
	}
	emp, ok := param(c, "id")
	if !ok {
		return a, nil, nil, false
	}
	rid, ok := param(c, "rid")
	return a, emp, rid, ok
}

// recordRoutes mounts list / create / replace / delete for one record kind under g.
func recordRoutes[In any, Out any](g gin.IRoutes, path string, readGuard, writeGuard []gin.HandlerFunc,
	list func(*gin.Context, Actor, *uuid.UUID) (interface{}, error),
	save func(*gin.Context, Actor, *uuid.UUID, *uuid.UUID, In) (Out, error),
	del func(*gin.Context, Actor, *uuid.UUID, uuid.UUID) error) {
	with := func(guard []gin.HandlerFunc, f gin.HandlerFunc) []gin.HandlerFunc {
		return append(append([]gin.HandlerFunc{}, guard...), f)
	}
	g.GET(path, with(readGuard, func(c *gin.Context) {
		if a, emp, _, ok := ids(c); ok {
			res, err := list(c, a, emp)
			respond(c, http.StatusOK, res, err)
		}
	})...)
	g.POST(path, with(writeGuard, func(c *gin.Context) {
		a, emp, _, ok := ids(c)
		var in In
		if ok && bind(c, &in) {
			res, err := save(c, a, emp, nil, in)
			respond(c, http.StatusCreated, res, err)
		}
	})...)
	g.PUT(path+"/:rid", with(writeGuard, func(c *gin.Context) {
		a, emp, rid, ok := ids(c)
		var in In
		if ok && bind(c, &in) {
			res, err := save(c, a, emp, rid, in)
			respond(c, http.StatusOK, res, err)
		}
	})...)
	g.DELETE(path+"/:rid", with(writeGuard, func(c *gin.Context) {
		if a, emp, rid, ok := ids(c); ok {
			if err := del(c, a, emp, *rid); err != nil {
				fail(c, err)
				return
			}
			c.Status(http.StatusNoContent)
		}
	})...)
}

// Register mounts the 31 operations: self-service under /employees/me, HR under /employees/:id (employee:read for
// reads, employee:update for writes) and the approval queue under /employee-change-requests (employee:approve_changes).
// The groups must already resolve the tenant and authenticate.
func (h *Handler) Register(me, byID, approvals *gin.RouterGroup, idp identityport.HTTP) {
	read, update := idp.Require("employee", "read"), idp.Require("employee", "update")
	sensitive := idp.Flag("employee", "read_sensitive", sensitiveKey)
	s := h.svc
	for _, m := range []struct {
		g     gin.IRoutes
		read  []gin.HandlerFunc
		write []gin.HandlerFunc
	}{{me, nil, nil}, {byID, []gin.HandlerFunc{read}, []gin.HandlerFunc{update}}} {
		recordRoutes(m.g, "/emergency-contacts", m.read, m.write,
			func(c *gin.Context, a Actor, emp *uuid.UUID) (interface{}, error) {
				return s.Contacts(c.Request.Context(), a, emp)
			},
			func(c *gin.Context, a Actor, emp, id *uuid.UUID, in ContactInput) (*EmergencyContact, error) {
				return s.SaveContact(c.Request.Context(), a, emp, id, in)
			},
			func(c *gin.Context, a Actor, emp *uuid.UUID, id uuid.UUID) error {
				return s.DeleteContact(c.Request.Context(), a, emp, id)
			})
		recordRoutes(m.g, "/education", m.read, m.write,
			func(c *gin.Context, a Actor, emp *uuid.UUID) (interface{}, error) {
				return s.Educations(c.Request.Context(), a, emp)
			},
			func(c *gin.Context, a Actor, emp, id *uuid.UUID, in EducationInput) (*Education, error) {
				return s.SaveEducation(c.Request.Context(), a, emp, id, in)
			},
			func(c *gin.Context, a Actor, emp *uuid.UUID, id uuid.UUID) error {
				return s.DeleteEducation(c.Request.Context(), a, emp, id)
			})
		recordRoutes(m.g, "/prior-employment", m.read, m.write,
			func(c *gin.Context, a Actor, emp *uuid.UUID) (interface{}, error) {
				return s.Priors(c.Request.Context(), a, emp)
			},
			func(c *gin.Context, a Actor, emp, id *uuid.UUID, in PriorInput) (*PriorEmployment, error) {
				return s.SavePrior(c.Request.Context(), a, emp, id, in)
			},
			func(c *gin.Context, a Actor, emp *uuid.UUID, id uuid.UUID) error {
				return s.DeletePrior(c.Request.Context(), a, emp, id)
			})
	}

	me.POST("/change-requests", sensitive, func(c *gin.Context) {
		a, ok := actor(c)
		var in ChangeInput
		if ok && bind(c, &in) {
			res, err := s.Submit(c.Request.Context(), a, in)
			respond(c, http.StatusCreated, res, err)
		}
	})
	me.GET("/change-requests", sensitive, func(c *gin.Context) {
		if a, ok := actor(c); ok {
			page, perPage := utils.ParsePagination(c)
			rows, total, err := s.Mine(c.Request.Context(), a, c.Query("status"), page, perPage)
			list(c, rows, page, perPage, total, err)
		}
	})
	me.POST("/change-requests/:rid/cancel", sensitive, func(c *gin.Context) {
		if a, _, rid, ok := ids(c); ok {
			res, err := s.Cancel(c.Request.Context(), a, *rid)
			respond(c, http.StatusOK, res, err)
		}
	})
	approvals.GET("", sensitive, func(c *gin.Context) {
		a, ok := actor(c)
		if !ok {
			return
		}
		f := RequestFilter{Status: c.Query("status"), Type: c.Query("type")}
		if raw := c.Query("employee_id"); raw != "" {
			id, err := uuid.Parse(raw)
			if err != nil {
				fail(c, &sharedErrors.AppError{Code: sharedErrors.ErrValidation.Code, Message: "employee_id must be a UUID", StatusCode: http.StatusBadRequest})
				return
			}
			f.EmployeeID = &id
		}
		page, perPage := utils.ParsePagination(c)
		rows, total, err := s.List(c.Request.Context(), a, f, page, perPage)
		list(c, rows, page, perPage, total, err)
	})
	approvals.GET("/:rid", sensitive, func(c *gin.Context) {
		if a, _, rid, ok := ids(c); ok {
			res, err := s.Get(c.Request.Context(), a, *rid)
			respond(c, http.StatusOK, res, err)
		}
	})
	decide := func(fn func(*gin.Context, Actor, uuid.UUID, *string) (*ChangeRequest, error)) gin.HandlerFunc {
		return func(c *gin.Context) {
			a, _, rid, ok := ids(c)
			var body struct {
				Comment *string `json:"comment"`
			}
			if !ok || (c.Request.ContentLength > 0 && !bind(c, &body)) {
				return
			}
			res, err := fn(c, a, *rid, body.Comment)
			respond(c, http.StatusOK, res, err)
		}
	}
	approvals.POST("/:rid/approve", sensitive, decide(func(c *gin.Context, a Actor, id uuid.UUID, cm *string) (*ChangeRequest, error) {
		return s.Approve(c.Request.Context(), a, id, cm)
	}))
	approvals.POST("/:rid/reject", sensitive, decide(func(c *gin.Context, a Actor, id uuid.UUID, cm *string) (*ChangeRequest, error) {
		return s.Reject(c.Request.Context(), a, id, cm)
	}))
}

func list(c *gin.Context, rows []ChangeRequest, page, perPage int, total int64, err error) {
	if err != nil {
		fail(c, err)
		return
	}
	pages := int((total + int64(perPage) - 1) / int64(perPage))
	c.JSON(http.StatusOK, gin.H{"data": rows, "meta": gin.H{"page": page, "per_page": perPage, "total_items": total, "total_pages": pages}})
}

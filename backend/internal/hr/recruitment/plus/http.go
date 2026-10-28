package plus

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/hr/recruitment/dto"
	"github.com/jaas/jaas/internal/hr/recruitment/models"
	"github.com/jaas/jaas/internal/identity/access"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"github.com/jaas/jaas/internal/shared/identityport"
	"gorm.io/gorm"
)

// Catalogue is the permission HR/Recruitment parity adds (docs/ARCHITECTURE.md Â§17).
func Catalogue() []access.PermissionDef {
	return []access.PermissionDef{
		{Module: "hr", Resource: "recruitment", Action: "approve", Description: "Approve requisitions and offers (never one's own)", Sensitive: true,
			Defaults: map[string]access.Scope{access.TemplateHRAdmin: access.ScopeTenant}},
	}
}

// Module is the recruitment parity container.
type Module struct{ svc *Service }

// NewModule wires the store and service.
func NewModule(db *gorm.DB, p Ports) *Module {
	return &Module{svc: NewService(NewStore(db), p, time.Now)}
}

// RegisterModels lists models for legacy AutoMigrate (the SQL migrations are the authority).
func (m *Module) RegisterModels() []interface{} {
	return []interface{}{&Requisition{}, &Scorecard{}, &JobScorecard{}, &InterviewScore{}, &OfferPolicy{}, &OfferApproval{}}
}

// Service exposes the service (the offer gate and tests).
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

func optionalBody(c *gin.Context, dst interface{}) bool {
	return c.Request.ContentLength <= 0 || bind(c, dst)
}

type commentBody struct {
	Comment *string `json:"comment"`
}

// RegisterRoutes mounts the 20 operations under rg (/recruitment).
func (m *Module) RegisterRoutes(rg *gin.RouterGroup, idp identityport.HTTP) {
	s := m.svc
	read, manage, hire, approve := idp.Require("recruitment", "read"), idp.Require("recruitment", "manage"), idp.Require("recruitment", "hire"),
		idp.Require("recruitment", "approve")
	g := rg.Group("/recruitment", idp.TenantResolver(), idp.Authenticate())

	g.POST("/candidates/:id/hire/release", hire, func(c *gin.Context) {
		if a, id, ok := withID(c); ok {
			if err := s.ReleaseHire(c.Request.Context(), a, id); err != nil {
				fail(c, err)
				return
			}
			c.Status(http.StatusNoContent)
		}
	})

	g.POST("/requisitions", manage, func(c *gin.Context) {
		a, ok := actor(c)
		var in RequisitionInput
		if ok && bind(c, &in) {
			res, err := s.CreateRequisition(c.Request.Context(), a, in)
			respond(c, http.StatusCreated, res, err)
		}
	})
	g.GET("/requisitions", read, func(c *gin.Context) {
		if a, ok := actor(c); ok {
			res, err := s.Requisitions(c.Request.Context(), a, c.Query("status"), c.Query("position_id"))
			respond(c, http.StatusOK, res, err)
		}
	})
	g.GET("/requisitions/:id", read, func(c *gin.Context) {
		if a, id, ok := withID(c); ok {
			res, err := s.GetRequisition(c.Request.Context(), a, id)
			respond(c, http.StatusOK, res, err)
		}
	})
	g.POST("/requisitions/:id/submit", manage, func(c *gin.Context) {
		if a, id, ok := withID(c); ok {
			res, err := s.SubmitRequisition(c.Request.Context(), a, id)
			respond(c, http.StatusOK, res, err)
		}
	})
	g.POST("/requisitions/:id/cancel", manage, func(c *gin.Context) {
		if a, id, ok := withID(c); ok {
			res, err := s.CancelRequisition(c.Request.Context(), a, id)
			respond(c, http.StatusOK, res, err)
		}
	})
	g.POST("/requisitions/:id/approve", approve, func(c *gin.Context) {
		a, id, ok := withID(c)
		var b commentBody
		if ok && optionalBody(c, &b) {
			res, err := s.ApproveRequisition(c.Request.Context(), a, id, b.Comment)
			respond(c, http.StatusOK, res, err)
		}
	})
	g.POST("/requisitions/:id/reject", approve, func(c *gin.Context) {
		a, id, ok := withID(c)
		var b commentBody
		if ok && optionalBody(c, &b) {
			res, err := s.RejectRequisition(c.Request.Context(), a, id, b.Comment)
			respond(c, http.StatusOK, res, err)
		}
	})

	g.POST("/scorecards", manage, func(c *gin.Context) {
		a, ok := actor(c)
		var in ScorecardInput
		if ok && bind(c, &in) {
			res, err := s.CreateScorecard(c.Request.Context(), a, in)
			respond(c, http.StatusCreated, res, err)
		}
	})
	g.GET("/scorecards", read, func(c *gin.Context) {
		if a, ok := actor(c); ok {
			res, err := s.Scorecards(c.Request.Context(), a, c.Query("status"))
			respond(c, http.StatusOK, res, err)
		}
	})
	g.GET("/scorecards/:id", read, func(c *gin.Context) {
		if a, id, ok := withID(c); ok {
			res, err := s.GetScorecard(c.Request.Context(), a, id)
			respond(c, http.StatusOK, res, err)
		}
	})
	g.PATCH("/scorecards/:id", manage, func(c *gin.Context) {
		a, id, ok := withID(c)
		var in ScorecardInput
		if ok && bind(c, &in) {
			res, err := s.UpdateScorecard(c.Request.Context(), a, id, in)
			respond(c, http.StatusOK, res, err)
		}
	})
	g.PUT("/jobs/:id/scorecard", manage, func(c *gin.Context) {
		a, id, ok := withID(c)
		var in struct {
			ScorecardID string `json:"scorecard_id"`
		}
		if ok && bind(c, &in) {
			res, err := s.AssignScorecard(c.Request.Context(), a, id, in.ScorecardID)
			respond(c, http.StatusOK, res, err)
		}
	})
	g.PUT("/interviews/:id/scores", func(c *gin.Context) {
		a, id, ok := withID(c)
		var in struct {
			Scores []ScoreInput `json:"scores"`
		}
		if ok && bind(c, &in) {
			res, err := s.SubmitScores(c.Request.Context(), a, id, in.Scores)
			respond(c, http.StatusOK, res, err)
		}
	})
	g.GET("/candidates/:id/scorecard", read, func(c *gin.Context) {
		if a, id, ok := withID(c); ok {
			res, err := s.Scorecard(c.Request.Context(), a, id)
			respond(c, http.StatusOK, res, err)
		}
	})

	g.PUT("/offer-policy", manage, func(c *gin.Context) {
		a, ok := actor(c)
		var in PolicyInput
		if ok && bind(c, &in) {
			res, err := s.SetOfferPolicy(c.Request.Context(), a, in)
			respond(c, http.StatusOK, res, err)
		}
	})
	g.GET("/offer-policy", read, func(c *gin.Context) {
		if a, ok := actor(c); ok {
			res, err := s.GetOfferPolicy(c.Request.Context(), a)
			respond(c, http.StatusOK, res, err)
		}
	})
	decide := func(decision string) gin.HandlerFunc {
		return func(c *gin.Context) {
			a, id, ok := withID(c)
			var b commentBody
			if ok && optionalBody(c, &b) {
				res, err := s.DecideOffer(c.Request.Context(), a, id, decision, b.Comment)
				if err != nil {
					fail(c, err)
					return
				}
				respond(c, http.StatusOK, offerView(res), nil)
			}
		}
	}
	g.POST("/offers/:id/approve", approve, decide(DecisionApproved))
	g.POST("/offers/:id/reject", approve, decide(DecisionRejected))
	g.GET("/offers/:id/approvals", read, func(c *gin.Context) {
		if a, id, ok := withID(c); ok {
			res, err := s.OfferApprovals(c.Request.Context(), a, id)
			respond(c, http.StatusOK, res, err)
		}
	})
}

// offerView is the public offer shape (the same fields as the base offer endpoints).
func offerView(o *models.Offer) dto.OfferResponse {
	out := dto.OfferResponse{ID: o.ID, CandidateID: o.CandidateID, OfferedCTC: o.OfferedCTC, JoiningDate: o.JoiningDate.Format("2006-01-02"),
		Status: o.Status, DecidedAt: o.DecidedAt, CreatedAt: o.CreatedAt}
	if o.ExpiresOn != nil {
		s := o.ExpiresOn.Format("2006-01-02")
		out.ExpiresOn = &s
	}
	return out
}

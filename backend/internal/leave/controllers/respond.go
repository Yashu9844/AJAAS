// Package controllers is Module 4's thin HTTP edge: parse → validate → service → envelope.
package controllers

import (
	"errors"
	"net/http"
	"strings"
	"unicode"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/leave/dto"
	"github.com/jaas/jaas/internal/leave/services"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
)

type errorBody struct {
	Code    string                               `json:"code"`
	Message string                               `json:"message"`
	Details []sharedErrors.ValidationErrorDetail `json:"details,omitempty"`
}

func ok(c *gin.Context, status int, data interface{}) {
	c.JSON(status, gin.H{"data": data})
}

func okList(c *gin.Context, data interface{}, meta dto.PageMeta) {
	c.JSON(http.StatusOK, gin.H{"data": data, "meta": meta})
}

// fail maps AppErrors to their status; anything else is an opaque 500 (no internals leaked).
func fail(c *gin.Context, err error) {
	var app *sharedErrors.AppError
	if errors.As(err, &app) {
		c.JSON(app.StatusCode, gin.H{"error": errorBody{Code: app.Code, Message: app.Message}})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": errorBody{Code: sharedErrors.ErrInternal.Code, Message: sharedErrors.ErrInternal.Message}})
}

// bindJSON decodes and validates a body; failures become VALIDATION_ERROR with field details.
func bindJSON(c *gin.Context, dst interface{}) bool {
	err := c.ShouldBindJSON(dst)
	if err == nil {
		return true
	}
	body := errorBody{Code: sharedErrors.ErrValidation.Code, Message: "request body is invalid"}
	var verrs validator.ValidationErrors
	if errors.As(err, &verrs) {
		for _, fe := range verrs {
			body.Details = append(body.Details, sharedErrors.ValidationErrorDetail{Field: snake(fe.Field()), Message: "failed rule: " + fe.Tag()})
		}
	} else {
		body.Message = "malformed JSON body"
	}
	c.JSON(http.StatusBadRequest, gin.H{"error": body})
	return false
}

func snake(s string) string {
	var b strings.Builder
	for i, r := range s {
		if unicode.IsUpper(r) {
			if i > 0 && (i+1 < len(s) && unicode.IsLower(rune(s[i+1])) || unicode.IsLower(rune(s[i-1]))) {
				b.WriteByte('_')
			}
			r = unicode.ToLower(r)
		}
		b.WriteRune(r)
	}
	return b.String()
}

// actorFrom builds the caller from Module 0 middleware context only (NFR-SEC002).
func actorFrom(c *gin.Context) (services.Actor, bool) {
	tenantID, okT := c.Get("tenant_id")
	userID, okU := c.Get("user_id")
	tid, okT2 := tenantID.(uuid.UUID)
	uid, okU2 := userID.(uuid.UUID)
	if !okT || !okU || !okT2 || !okU2 {
		fail(c, sharedErrors.ErrUnauthorized)
		return services.Actor{}, false
	}
	corr, err := uuid.Parse(c.GetString("request_id"))
	if err != nil {
		corr = uuid.New()
	}
	return services.Actor{TenantID: tid, UserID: uid, IP: c.ClientIP(), UserAgent: c.Request.UserAgent(), CorrelationID: corr}, true
}

// pathID parses a :param UUID; malformed ids are 400.
func pathID(c *gin.Context, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param(name))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": errorBody{Code: sharedErrors.ErrValidation.Code, Message: name + " must be a UUID"}})
		return uuid.Nil, false
	}
	return id, true
}

func page(c *gin.Context) dto.Page {
	return dto.ParsePage(c.Query("page"), c.Query("per_page"))
}

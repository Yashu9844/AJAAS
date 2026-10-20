package controllers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"github.com/jaas/jaas/internal/shared/utils"
)

type successEnvelope struct {
	Data interface{} `json:"data"`
	Meta interface{} `json:"meta,omitempty"`
}

type errorEnvelope struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code    string      `json:"code"`
	Message string      `json:"message"`
	Details interface{} `json:"details,omitempty"`
}

func respondSuccess(c *gin.Context, status int, data interface{}, meta interface{}) {
	c.JSON(status, successEnvelope{Data: data, Meta: meta})
}

func respondError(c *gin.Context, err error) {
	err = sharedErrors.Normalize(err)
	var appErr *sharedErrors.AppError
	if errors.As(err, &appErr) {
		c.JSON(appErr.StatusCode, errorEnvelope{
			Error: errorDetail{Code: appErr.Code, Message: appErr.Message},
		})
		return
	}
	c.JSON(http.StatusInternalServerError, errorEnvelope{
		Error: errorDetail{Code: sharedErrors.ErrInternal.Code, Message: sharedErrors.ErrInternal.Message},
	})
}

func getTenantID(c *gin.Context) (uuid.UUID, bool) {
	val, exists := c.Get("tenant_id")
	if !exists {
		respondError(c, sharedErrors.ErrUnauthorized)
		return uuid.Nil, false
	}
	id, ok := val.(uuid.UUID)
	if !ok {
		respondError(c, sharedErrors.ErrUnauthorized)
		return uuid.Nil, false
	}
	return id, true
}

func getUserID(c *gin.Context) (uuid.UUID, bool) {
	val, exists := c.Get("user_id")
	if !exists {
		respondError(c, sharedErrors.ErrUnauthorized)
		return uuid.Nil, false
	}
	id, ok := val.(uuid.UUID)
	if !ok {
		respondError(c, sharedErrors.ErrUnauthorized)
		return uuid.Nil, false
	}
	return id, true
}

// respondBadRequest returns a 400 in the standard error envelope.
func respondBadRequest(c *gin.Context, msg string) {
	c.JSON(http.StatusBadRequest, errorEnvelope{
		Error: errorDetail{Code: sharedErrors.ErrValidation.Code, Message: msg},
	})
}

// respondBindError converts a request-binding failure into field-level validation details.
func respondBindError(c *gin.Context, err error) {
	c.JSON(http.StatusBadRequest, errorEnvelope{
		Error: errorDetail{Code: sharedErrors.ErrValidation.Code, Message: sharedErrors.ErrValidation.Message, Details: utils.BindErrorDetails(err)},
	})
}

// respondValidationError returns field-level validation details.
func respondValidationError(c *gin.Context, details []sharedErrors.ValidationErrorDetail) {
	c.JSON(http.StatusBadRequest, errorEnvelope{
		Error: errorDetail{Code: sharedErrors.ErrValidation.Code, Message: sharedErrors.ErrValidation.Message, Details: details},
	})
}

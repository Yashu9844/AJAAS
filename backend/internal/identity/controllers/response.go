package controllers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
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

// respondSuccess returns a consistent success format envelope.
func respondSuccess(c *gin.Context, status int, data interface{}, meta interface{}) {
	c.JSON(status, successEnvelope{
		Data: data,
		Meta: meta,
	})
}

// respondError wraps error types (e.g. AppError, Validation errors) into client envelopes.
func respondError(c *gin.Context, err error) {
	var appErr *sharedErrors.AppError
	if errors.As(err, &appErr) {
		c.JSON(appErr.StatusCode, errorEnvelope{
			Error: errorDetail{
				Code:    appErr.Code,
				Message: appErr.Message,
			},
		})
		return
	}

	// Fallback to internal error for unexpected errors
	c.JSON(http.StatusInternalServerError, errorEnvelope{
		Error: errorDetail{
			Code:    sharedErrors.ErrInternal.Code,
			Message: sharedErrors.ErrInternal.Message,
		},
	})
}

// respondValidationError returns field-level validation failure details.
func respondValidationError(c *gin.Context, details []sharedErrors.ValidationErrorDetail) {
	c.JSON(http.StatusBadRequest, errorEnvelope{
		Error: errorDetail{
			Code:    sharedErrors.ErrValidation.Code,
			Message: sharedErrors.ErrValidation.Message,
			Details: details,
		},
	})
}

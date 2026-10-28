// Package kernel holds what every M2 context shares: error constructors, outbox events, cursors, fractional ranks and
// HTTP helpers. It imports no other M2 package.
package kernel

import (
	"errors"
	"fmt"
	"net/http"

	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
)

// Err builds an application error with an explicit status and a stable code.
func Err(status int, code, msg string) *sharedErrors.AppError {
	return &sharedErrors.AppError{Code: code, Message: msg, StatusCode: status}
}

// Invalid is a 400 VALIDATION_ERROR naming the field.
func Invalid(field, msg string) *sharedErrors.AppError {
	return Err(http.StatusBadRequest, sharedErrors.ErrValidation.Code, field+": "+msg)
}

// NotFound is a 404; foreign-tenant and no-access ids answer the same way so nothing leaks.
func NotFound(what string) *sharedErrors.AppError {
	return Err(http.StatusNotFound, sharedErrors.ErrNotFound.Code, what+" not found")
}

// Forbidden is a 403 for callers who can see the object but lack the permission.
func Forbidden(perm string) *sharedErrors.AppError {
	return Err(http.StatusForbidden, sharedErrors.ErrForbidden.Code, "missing permission "+perm)
}

// Conflict is a 409 with a stable code.
func Conflict(code, msg string) *sharedErrors.AppError { return Err(http.StatusConflict, code, msg) }

// VersionConflict is returned when an edit carries a stale version; Current is what the caller should re-read (TK-03).
type VersionConflict struct{ Current int }

func (e *VersionConflict) Error() string {
	return fmt.Sprintf("[VERSION_CONFLICT] the object is at version %d (status: 409)", e.Current)
}

// AsVersionConflict unwraps a VersionConflict.
func AsVersionConflict(err error) (*VersionConflict, bool) {
	var v *VersionConflict
	return v, errors.As(err, &v)
}

package services

import (
	"net/http"

	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
)

func appErr(status int, code, msg string) *sharedErrors.AppError {
	return &sharedErrors.AppError{Code: code, Message: msg, StatusCode: status}
}

// invalid builds a 400 VALIDATION_ERROR naming the offending field.
func invalid(field, msg string) *sharedErrors.AppError {
	return appErr(http.StatusBadRequest, sharedErrors.ErrValidation.Code, field+": "+msg)
}

// Module 3 domain errors (specification.md §6).
var (
	ErrEmployeeNotFound     = appErr(http.StatusNotFound, "EMPLOYEE_NOT_FOUND", "no employee profile for this user in the tenant")
	ErrEmployeeNotActive    = appErr(http.StatusForbidden, "EMPLOYEE_NOT_ACTIVE", "employee status does not allow attendance")
	ErrAlreadyPunchedIn     = appErr(http.StatusConflict, "ALREADY_PUNCHED_IN", "a session is already open; punch out first")
	ErrNotPunchedIn         = appErr(http.StatusConflict, "NOT_PUNCHED_IN", "no open session to punch out of")
	ErrDuplicatePunch       = appErr(http.StatusConflict, "DUPLICATE_PUNCH", "punches less than 60 seconds apart are rejected")
	ErrSessionExpired       = appErr(http.StatusConflict, "SESSION_EXPIRED", "the open session is older than 20 hours; request a regularization")
	ErrAssignmentOverlap    = appErr(http.StatusConflict, "ASSIGNMENT_OVERLAP", "assignment overlaps an existing assignment")
	ErrShiftInUse           = appErr(http.StatusConflict, "SHIFT_IN_USE", "shift has current or future assignments")
	ErrShiftInactive        = appErr(http.StatusConflict, "SHIFT_INACTIVE", "shift is inactive")
	ErrShiftAlreadyInactive = appErr(http.StatusConflict, "CONFLICT", "shift is already inactive")
	ErrShiftNameTaken       = appErr(http.StatusConflict, "CONFLICT", "shift name already exists")
	ErrShiftCodeTaken       = appErr(http.StatusConflict, "CONFLICT", "shift code already exists")
	ErrRegularizationWindow = appErr(http.StatusBadRequest, "REGULARIZATION_WINDOW", "attendance_date must be within the last 30 days and not in the future")
	ErrRegularizationPend   = appErr(http.StatusConflict, "REGULARIZATION_PENDING", "a pending regularization already exists for this date")
	ErrRegularizationState  = appErr(http.StatusConflict, "REGULARIZATION_NOT_PENDING", "regularization is not pending")
	ErrSelfApproval         = appErr(http.StatusForbidden, "SELF_APPROVAL_FORBIDDEN", "you cannot review your own regularization")
)

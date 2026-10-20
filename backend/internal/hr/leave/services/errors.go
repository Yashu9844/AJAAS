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

// Module 4 domain errors (specification.md §6).
var (
	ErrEmployeeNotFound   = appErr(http.StatusNotFound, "EMPLOYEE_NOT_FOUND", "no employee profile for this user in the tenant")
	ErrEmployeeNotActive  = appErr(http.StatusForbidden, "EMPLOYEE_NOT_ACTIVE", "employee status does not allow leave requests")
	ErrNotFound           = appErr(http.StatusNotFound, sharedErrors.ErrNotFound.Code, "resource not found")
	ErrTypeInactive       = appErr(http.StatusConflict, "LEAVE_TYPE_INACTIVE", "leave type is inactive")
	ErrTypeNotApplicable  = appErr(http.StatusForbidden, "LEAVE_TYPE_NOT_APPLICABLE", "leave type does not apply to this employee")
	ErrTypeAlreadyOff     = appErr(http.StatusConflict, "CONFLICT", "leave type is already inactive")
	ErrTypeNameTaken      = appErr(http.StatusConflict, "CONFLICT", "leave type name already exists")
	ErrTypeCodeTaken      = appErr(http.StatusConflict, "CONFLICT", "leave type code already exists")
	ErrHolidayTaken       = appErr(http.StatusConflict, "CONFLICT", "a holiday already exists on this date")
	ErrInvalidRange       = appErr(http.StatusBadRequest, "INVALID_LEAVE_RANGE", "start_date must not be after end_date and both must be in the current year")
	ErrNotice             = appErr(http.StatusBadRequest, "LEAVE_NOTICE", "start_date violates the leave type notice or the 30-day backdating window")
	ErrNoWorkingDays      = appErr(http.StatusBadRequest, "NO_WORKING_DAYS", "the range contains no working days")
	ErrTooLong            = appErr(http.StatusBadRequest, "LEAVE_TOO_LONG", "the range exceeds the leave type's max_consecutive_days")
	ErrOverlap            = appErr(http.StatusConflict, "LEAVE_OVERLAP", "the range overlaps an existing pending or approved request")
	ErrInsufficient       = appErr(http.StatusConflict, "INSUFFICIENT_BALANCE", "not enough available balance for this leave type")
	ErrNotPending         = appErr(http.StatusConflict, "LEAVE_NOT_PENDING", "leave request is not pending")
	ErrAlreadyStarted     = appErr(http.StatusConflict, "LEAVE_ALREADY_STARTED", "approved leave that has started cannot be cancelled")
	ErrNotCancellable     = appErr(http.StatusConflict, "LEAVE_NOT_CANCELLABLE", "leave request cannot be cancelled in its current status")
	ErrSelfApproval       = appErr(http.StatusForbidden, "SELF_APPROVAL_FORBIDDEN", "you cannot review your own leave request")
	ErrRejectNeedsComment = invalid("comment", "is required to reject")
)

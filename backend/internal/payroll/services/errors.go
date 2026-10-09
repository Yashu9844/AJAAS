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

// Module 5 domain errors (specification.md §6).
var (
	ErrNotFound          = appErr(http.StatusNotFound, sharedErrors.ErrNotFound.Code, "resource not found")
	ErrEmployeeNotFound  = appErr(http.StatusNotFound, "EMPLOYEE_NOT_FOUND", "no employee profile in this tenant")
	ErrStructureName     = appErr(http.StatusConflict, "CONFLICT", "a structure with this name already exists")
	ErrStructureInactive = appErr(http.StatusConflict, "STRUCTURE_INACTIVE", "structure is inactive")
	ErrAlreadyInactive   = appErr(http.StatusConflict, "CONFLICT", "structure is already inactive")
	ErrOverflow          = appErr(http.StatusBadRequest, "STRUCTURE_OVERFLOW", "structure earnings exceed the monthly CTC")
	ErrBackdated         = appErr(http.StatusConflict, "ASSIGNMENT_BACKDATED", "effective_from must be after the current assignment's effective_from")
	ErrRunExists         = appErr(http.StatusConflict, "RUN_EXISTS", "a payroll run already exists for this period")
	ErrPeriod            = appErr(http.StatusBadRequest, "INVALID_PERIOD", "payroll period must not be in the future")
	ErrRunState          = appErr(http.StatusConflict, "RUN_STATE", "the run's status does not allow this action")
	ErrSelfApproval      = appErr(http.StatusForbidden, "SELF_APPROVAL_FORBIDDEN", "the user who calculated a run cannot approve it")
)

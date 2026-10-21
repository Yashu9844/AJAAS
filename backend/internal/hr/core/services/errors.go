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

// HR/Core domain errors (specification.md §5).
var (
	ErrNotFound          = appErr(http.StatusNotFound, sharedErrors.ErrNotFound.Code, "resource not found")
	ErrEmployeeNotFound  = appErr(http.StatusNotFound, "EMPLOYEE_NOT_FOUND", "no employee profile in this tenant")
	ErrCodeTaken         = appErr(http.StatusConflict, "CONFLICT", "the code is already used in this tenant")
	ErrRankTaken         = appErr(http.StatusConflict, "CONFLICT", "another grade has this rank")
	ErrInvalidState      = appErr(http.StatusConflict, "INVALID_STATE", "the action's status does not allow this")
	ErrInactiveReference = appErr(http.StatusConflict, "INACTIVE_REFERENCE", "a referenced record is inactive")
	ErrSelfApproval      = appErr(http.StatusForbidden, "CORE_SELF_APPROVAL", "the initiator cannot approve their own action")
	ErrLaterChange       = appErr(http.StatusConflict, "LATER_CHANGE_EXISTS", "a change with a later or equal effective date already exists")
	ErrNoAssignment      = appErr(http.StatusConflict, "NO_ASSIGNMENT", "the worker has no open assignment on this date")
	ErrAlreadyAssigned   = appErr(http.StatusConflict, "ALREADY_ASSIGNED", "the worker already has an assignment")
	ErrPositionFilled    = appErr(http.StatusConflict, "POSITION_FILLED", "the position is already filled on these dates")
	ErrPositionState     = appErr(http.StatusConflict, "POSITION_STATE", "the position is not active")
	ErrPositionOccupied  = appErr(http.StatusConflict, "POSITION_OCCUPIED", "the position has current or future incumbents")
	ErrPositionOverfill  = appErr(http.StatusConflict, "POSITION_OVERFILLED", "the budgeted FTE would be below the filled FTE")
	ErrGradeRule         = appErr(http.StatusConflict, "GRADE_RULE", "the grade does not match the action (or give grade_override_reason)")
	ErrNoChange          = appErr(http.StatusConflict, "NO_CHANGE", "the action changes nothing the type requires")
	ErrProbationState    = appErr(http.StatusConflict, "PROBATION_STATE", "the worker has no open probation")
	ErrNotTerminated     = appErr(http.StatusConflict, "NOT_TERMINATED", "only terminated or resigned workers can be rehired")
	ErrNotLatest         = appErr(http.StatusConflict, "NOT_LATEST", "only the worker's latest completed action can be rescinded")
	ErrPlanOverlap       = appErr(http.StatusConflict, "CONFLICT", "a plan for this department already covers part of the period")
)

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

// HR/Performance domain errors (specification.md §6).
var (
	ErrNotFound         = appErr(http.StatusNotFound, sharedErrors.ErrNotFound.Code, "resource not found")
	ErrEmployeeNotFound = appErr(http.StatusNotFound, "EMPLOYEE_NOT_FOUND", "no employee profile in this tenant")
	ErrCycleState       = appErr(http.StatusConflict, "CYCLE_STATE", "the cycle's status does not allow this action")
	ErrGoalState        = appErr(http.StatusConflict, "GOAL_STATE", "the goal's status does not allow this action")
	ErrReviewState      = appErr(http.StatusConflict, "REVIEW_STATE", "the review is not pending")
	ErrWeightExceeded   = appErr(http.StatusConflict, "WEIGHT_EXCEEDED", "the owner's goal weights in this cycle would exceed 100")
	ErrGoalDepth        = appErr(http.StatusConflict, "GOAL_DEPTH", "goal cascading is limited to three levels")
	ErrHasKeyResults    = appErr(http.StatusConflict, "HAS_KEY_RESULTS", "progress of a goal with key results comes from its key results")
	ErrEmployeeState    = appErr(http.StatusConflict, "EMPLOYEE_STATE", "the employee is not working (active, probation or notice)")
	ErrDuplicateReview  = appErr(http.StatusConflict, "CONFLICT", "this reviewer is already assigned")
)

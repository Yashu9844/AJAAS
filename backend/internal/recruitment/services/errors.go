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

// Module 6 domain errors (specification.md §6).
var (
	ErrNotFound         = appErr(http.StatusNotFound, sharedErrors.ErrNotFound.Code, "resource not found")
	ErrEmployeeNotFound = appErr(http.StatusNotFound, "EMPLOYEE_NOT_FOUND", "no employee profile in this tenant")
	ErrJobNotOpen       = appErr(http.StatusConflict, "JOB_NOT_OPEN", "the job is not open")
	ErrJobState         = appErr(http.StatusConflict, "JOB_STATE", "the job's status does not allow this change")
	ErrDuplicateEmail   = appErr(http.StatusConflict, "CONFLICT", "a candidate with this email already applied to this job")
	ErrInvalidStage     = appErr(http.StatusConflict, "INVALID_STAGE", "the candidate's stage does not allow this action")
	ErrInterviewState   = appErr(http.StatusConflict, "INTERVIEW_STATE", "the interview is not scheduled")
	ErrOfferExists      = appErr(http.StatusConflict, "OFFER_EXISTS", "the candidate already has an open offer")
	ErrOfferState       = appErr(http.StatusConflict, "OFFER_STATE", "the offer is not open")
	ErrNotHireable      = appErr(http.StatusConflict, "NOT_HIREABLE", "hire needs stage offer and an accepted offer")
	ErrInterviewerState = appErr(http.StatusConflict, "CONFLICT", "the interviewer is not a working employee")
)

package errors

import "fmt"

// AppError represents a domain error that maps to an HTTP response.
type AppError struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	StatusCode int    `json:"-"`
}

func (e *AppError) Error() string {
	return fmt.Sprintf("[%s] %s (status: %d)", e.Code, e.Message, e.StatusCode)
}

// ValidationErrorDetail provides details on a specific field validation failure.
type ValidationErrorDetail struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// Predefined application-wide error sentinels.
var (
	ErrValidation         = &AppError{Code: "VALIDATION_ERROR", Message: "Validation failed", StatusCode: 400}
	ErrUnauthorized       = &AppError{Code: "UNAUTHORIZED", Message: "Unauthorized request", StatusCode: 401}
	ErrInvalidCredentials = &AppError{Code: "INVALID_CREDENTIALS", Message: "Invalid credentials provided", StatusCode: 401}
	ErrForbidden          = &AppError{Code: "FORBIDDEN", Message: "Permission denied", StatusCode: 403}
	ErrNotFound           = &AppError{Code: "NOT_FOUND", Message: "Requested resource not found", StatusCode: 404}
	ErrConflict           = &AppError{Code: "CONFLICT", Message: "Conflict with existing resource", StatusCode: 409}
	ErrRateLimited        = &AppError{Code: "RATE_LIMITED", Message: "Too many requests", StatusCode: 429}
	ErrInternal           = &AppError{Code: "INTERNAL_ERROR", Message: "Internal server error", StatusCode: 500}
)

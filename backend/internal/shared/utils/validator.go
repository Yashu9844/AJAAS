package utils

import (
	"errors"
	"fmt"

	"github.com/go-playground/validator/v10"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
)

var validate *validator.Validate

func init() {
	validate = validator.New()
}

// ValidateStruct checks validation tags of structs and returns mapped field errors.
func ValidateStruct(s interface{}) []sharedErrors.ValidationErrorDetail {
	err := validate.Struct(s)
	if err == nil {
		return nil
	}

	var valErrors validator.ValidationErrors
	if errors.As(err, &valErrors) {
		details := make([]sharedErrors.ValidationErrorDetail, len(valErrors))
		for i, fieldErr := range valErrors {
			details[i] = sharedErrors.ValidationErrorDetail{
				Field:   fieldErr.Field(),
				Message: formatValidationErrorMessage(fieldErr),
			}
		}
		return details
	}

	// Generic fallback validation error
	return []sharedErrors.ValidationErrorDetail{
		{
			Field:   "struct",
			Message: err.Error(),
		},
	}
}

// formatValidationErrorMessage structures a readable message based on the failure tag.
func formatValidationErrorMessage(fe validator.FieldError) string {
	switch fe.Tag() {
	case "required":
		return "This field is required"
	case "email":
		return "Must be a valid email address"
	case "min":
		return fmt.Sprintf("Must be at least %s characters or value", fe.Param())
	case "max":
		return fmt.Sprintf("Must not exceed %s characters or value", fe.Param())
	case "eqfield":
		return fmt.Sprintf("Must equal field %s", fe.Param())
	case "uuid":
		return "Must be a valid UUID"
	case "url":
		return "Must be a valid URL"
	default:
		return fmt.Sprintf("Failed validation on tag: %s", fe.Tag())
	}
}

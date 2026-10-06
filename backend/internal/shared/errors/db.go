package errors

import (
	stderrors "errors"

	"gorm.io/gorm"
)

// Normalize maps well-known persistence failures onto client-facing AppErrors so they never surface as HTTP 500.
// Unknown errors are returned unchanged.
func Normalize(err error) error {
	if err == nil {
		return nil
	}
	var appErr *AppError
	if stderrors.As(err, &appErr) {
		return err
	}
	switch {
	case stderrors.Is(err, gorm.ErrDuplicatedKey):
		return &AppError{Code: ErrConflict.Code, Message: "A resource with the same unique value already exists", StatusCode: ErrConflict.StatusCode}
	case stderrors.Is(err, gorm.ErrForeignKeyViolated):
		return &AppError{Code: ErrValidation.Code, Message: "A referenced resource does not exist", StatusCode: ErrValidation.StatusCode}
	case stderrors.Is(err, gorm.ErrRecordNotFound):
		return ErrNotFound
	}
	return err
}

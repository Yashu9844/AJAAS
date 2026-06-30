package utils

import (
	"testing"
)

type DummyStruct struct {
	Email    string `validate:"required,email"`
	Password string `validate:"required,min=6"`
}

func TestValidateStruct(t *testing.T) {
	// Case 1: Valid fields
	ds := DummyStruct{
		Email:    "test@example.com",
		Password: "password123",
	}
	errs := ValidateStruct(ds)
	if len(errs) > 0 {
		t.Errorf("expected zero validation errors, got %d: %v", len(errs), errs)
	}

	// Case 2: Invalid fields
	dsInvalid := DummyStruct{
		Email:    "not-an-email",
		Password: "short",
	}
	errs = ValidateStruct(dsInvalid)
	if len(errs) != 2 {
		t.Errorf("expected 2 validation errors, got %d", len(errs))
	}

	// Verify field names map correctly
	var hasEmail, hasPassword bool
	for _, e := range errs {
		if e.Field == "Email" {
			hasEmail = true
		}
		if e.Field == "Password" {
			hasPassword = true
		}
	}
	if !hasEmail || !hasPassword {
		t.Error("expected validation errors to identify Email and Password fields")
	}
}

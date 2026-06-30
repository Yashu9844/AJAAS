package errors

import (
	"strings"
	"testing"
)

func TestAppError_Error(t *testing.T) {
	err := &AppError{
		Code:       "TEST_ERROR",
		Message:    "This is a test",
		StatusCode: 418,
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "TEST_ERROR") {
		t.Errorf("expected error message to contain 'TEST_ERROR', got %q", errMsg)
	}
	if !strings.Contains(errMsg, "This is a test") {
		t.Errorf("expected error message to contain 'This is a test', got %q", errMsg)
	}
	if !strings.Contains(errMsg, "418") {
		t.Errorf("expected error message to contain status code '418', got %q", errMsg)
	}
}

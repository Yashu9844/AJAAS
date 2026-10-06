package errors

import (
	"fmt"
	"testing"

	"gorm.io/gorm"
)

func TestNormalize(t *testing.T) {
	if Normalize(nil) != nil {
		t.Fatal("nil must stay nil")
	}
	cases := []struct {
		name   string
		in     error
		status int
		code   string
	}{
		{"duplicate", fmt.Errorf("wrap: %w", gorm.ErrDuplicatedKey), 409, "CONFLICT"},
		{"fk", gorm.ErrForeignKeyViolated, 400, "VALIDATION_ERROR"},
		{"notfound", gorm.ErrRecordNotFound, 404, "NOT_FOUND"},
	}
	for _, c := range cases {
		got := Normalize(c.in)
		ae, ok := got.(*AppError)
		if !ok || ae.StatusCode != c.status || ae.Code != c.code {
			t.Errorf("%s: got %#v", c.name, got)
		}
	}
	app := &AppError{Code: "X", StatusCode: 418}
	if Normalize(app) != error(app) {
		t.Error("AppError must pass through")
	}
	plain := fmt.Errorf("boom")
	if Normalize(plain) != plain {
		t.Error("unknown error must pass through")
	}
}

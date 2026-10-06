package validators_test

import (
	"testing"

	"github.com/jaas/jaas/internal/employee/validators"
)

func TestValidateEmployeeCode(t *testing.T) {
	valid := []string{"EMP-001", "E1", "ENGINEER-99", "DEV-2026-X"}
	for _, c := range valid {
		if err := validators.ValidateEmployeeCode(c); err != nil {
			t.Errorf("expected %s to be valid, got %v", c, err)
		}
	}

	invalid := []string{"", "A", "emp_001", "EMP@123", "TOO-LONG-CODE-EXCEEDING-THIRTY-TWO-CHARS-ALLOWED"}
	for _, c := range invalid {
		if err := validators.ValidateEmployeeCode(c); err == nil {
			t.Errorf("expected %s to be invalid", c)
		}
	}
}

func TestValidateStatus(t *testing.T) {
	valid := []string{"active", "probation", "notice", "terminated", "resigned", "on_leave", "inactive"}
	for _, s := range valid {
		if err := validators.ValidateStatus(s); err != nil {
			t.Errorf("expected status %s to be valid, got %v", s, err)
		}
	}

	if err := validators.ValidateStatus("unknown_status"); err == nil {
		t.Errorf("expected error for unknown status")
	}
}

func TestValidateEmploymentType(t *testing.T) {
	valid := []string{"full_time", "part_time", "contract", "intern"}
	for _, e := range valid {
		if err := validators.ValidateEmploymentType(e); err != nil {
			t.Errorf("expected employment type %s to be valid, got %v", e, err)
		}
	}

	if err := validators.ValidateEmploymentType("freelance"); err == nil {
		t.Errorf("expected error for unsupported employment type")
	}
}

func TestValidateTransitionMatrix(t *testing.T) {
	ok := [][2]string{
		{"probation", "active"}, {"active", "notice"}, {"active", "terminated"}, {"on_leave", "active"}, {"notice", "active"},
		{"notice", "resigned"}, {"inactive", "active"}, {"active", "active"}, {"ACTIVE", "Notice"},
	}
	for _, c := range ok {
		if err := validators.ValidateTransition(c[0], c[1]); err != nil {
			t.Errorf("%s -> %s must be allowed: %v", c[0], c[1], err)
		}
	}
	bad := [][2]string{
		{"resigned", "active"}, {"terminated", "probation"}, {"resigned", "terminated"}, {"active", "probation"},
		{"inactive", "notice"}, {"unknown", "active"},
	}
	for _, c := range bad {
		if err := validators.ValidateTransition(c[0], c[1]); err == nil {
			t.Errorf("%s -> %s must be rejected", c[0], c[1])
		}
	}
}

func TestValidateEmergencyContacts(t *testing.T) {
	good := []string{"", "  ", "[]", `[{"name":"Mom","relation":"mother","phone":"+91"}]`, `[{"name":"A","phone":"1"},{"name":"B","phone":"2"}]`}
	for _, g := range good {
		if err := validators.ValidateEmergencyContacts(g); err != nil {
			t.Errorf("%q must be valid: %v", g, err)
		}
	}
	bad := []string{"free text", `{"name":"x"}`, `[{"name":"Mom"}]`, `[{"phone":"1"}]`, `[1,2]`, `[{"name":" ","phone":" "}]`}
	for _, b := range bad {
		if err := validators.ValidateEmergencyContacts(b); err == nil {
			t.Errorf("%q must be rejected", b)
		}
	}
}

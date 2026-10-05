package validators

import "testing"

func TestValidateOrgCode(t *testing.T) {
	for _, ok := range []string{"eng", "sales-2", "r-and-d", "dept01"} {
		if !ValidateOrgCode(ok) {
			t.Errorf("expected code %q to be valid", ok)
		}
	}
	for _, bad := range []string{"", "A", "Upper", "has space", "a", "x!", "this-code-is-way-too-long-for-org"} {
		if ValidateOrgCode(bad) {
			t.Errorf("expected code %q to be invalid", bad)
		}
	}
}

func TestIsReservedOrgCode(t *testing.T) {
	for _, r := range []string{"api", "admin", "root", "all"} {
		if !IsReservedOrgCode(r) {
			t.Errorf("expected %q to be reserved", r)
		}
	}
	if IsReservedOrgCode("engineering") {
		t.Error("expected engineering to be allowed")
	}
}

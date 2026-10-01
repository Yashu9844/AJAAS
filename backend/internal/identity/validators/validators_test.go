package validators

import "testing"

func TestValidateSlug(t *testing.T) {
	cases := []struct {
		slug  string
		valid bool
	}{
		{"acme", true},
		{"acme-corp", true},
		{"my-company-123", true},
		{"my--company", true}, // double hyphens matched by regex
		{"-starting", false},   // starts with hyphen
		{"ending-", false},     // ends with hyphen
		{"ACME", false},        // uppercase not allowed
		{"a", false},           // too short
		{"ab", false},          // too short (min 3 chars required by ^[a-z][a-z0-9-]{1,62}[a-z0-9]$)
		{"abc", true},          // length 3, valid
		{"very-long-slug-exceeding-sixty-four-characters-for-testing-purposes-only", false}, // > 64 chars
	}

	for _, tc := range cases {
		if res := ValidateSlug(tc.slug); res != tc.valid {
			t.Errorf("ValidateSlug(%q) = %t; expected %t", tc.slug, res, tc.valid)
		}
	}
}

func TestIsReservedSlug(t *testing.T) {
	if !IsReservedSlug("admin") {
		t.Error("expected 'admin' to be identified as a reserved slug")
	}
	if !IsReservedSlug("api") {
		t.Error("expected 'api' to be identified as a reserved slug")
	}
	if IsReservedSlug("my-tenant") {
		t.Error("expected 'my-tenant' to not be reserved")
	}
}

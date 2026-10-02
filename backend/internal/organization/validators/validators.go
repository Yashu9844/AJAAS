package validators

import (
	"regexp"
)

var (
	// orgCodeRegex matches lowercase slug-like codes, 2 to 32 chars.
	orgCodeRegex = regexp.MustCompile(`^[a-z0-9-]{2,32}$`)

	// orgReservedCodes mirrors identity reserved slugs where colliding.
	orgReservedCodes = map[string]bool{
		"api": true, "admin": true, "www": true, "mail": true, "app": true,
		"platform": true, "system": true, "root": true, "all": true, "none": true,
	}
)

// ValidateOrgCode checks an org code against the slug-like format.
func ValidateOrgCode(code string) bool {
	return orgCodeRegex.MatchString(code)
}

// IsReservedOrgCode checks platform-reserved org codes.
func IsReservedOrgCode(code string) bool {
	return orgReservedCodes[code]
}

// MaxHierarchyDepth caps department nesting (FR-H004).
const MaxHierarchyDepth = 10

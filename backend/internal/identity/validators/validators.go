package validators

import (
	"regexp"
)

var (
	// regex matches alphanumeric lowercase strings with hyphens, not starting/ending with hyphen, length 3 to 64.
	slugRegex = regexp.MustCompile(`^[a-z][a-z0-9-]{1,62}[a-z0-9]$`)

	// list of reserved subdomains/slugs that cannot be registered by tenants.
	reservedSlugs = map[string]bool{
		"api":        true,
		"admin":      true,
		"administrator": true,
		"www":        true,
		"mail":       true,
		"app":        true,
		"platform":   true,
		"system":     true,
		"portal":     true,
		"status":     true,
		"dev":        true,
		"staging":    true,
		"prod":       true,
		"test":       true,
		"jaas":       true,
		"support":    true,
		"billing":    true,
	}
)

// ValidateSlug checks if the slug conforms to subdomain routing constraints.
func ValidateSlug(slug string) bool {
	return slugRegex.MatchString(slug)
}

// IsReservedSlug checks if the slug is reserved by the platform.
func IsReservedSlug(slug string) bool {
	return reservedSlugs[slug]
}

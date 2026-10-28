package fields

import (
	"math"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/kernel"
)

// Limits.
const (
	maxDefinitions = 30
	maxOptions     = 50
	maxOptionLen   = 60
	maxText        = 1000
	maxURL         = 500
	maxNumber      = 1e12
)

var keySlug = regexp.MustCompile(`[^a-z0-9]+`)

// slugKey derives the stable key of a field from its name.
func slugKey(name string) string {
	k := strings.Trim(keySlug.ReplaceAllString(strings.ToLower(name), "_"), "_")
	if k == "" {
		k = "field"
	}
	if len(k) > 40 {
		k = k[:40]
	}
	return k
}

func validType(t string) bool {
	switch t {
	case TypeText, TypeNumber, TypeCheckbox, TypeDate, TypeURL, TypeSelect, TypeMultiselect, TypeUser:
		return true
	}
	return false
}

// cleanOptions validates the option list of a select or multiselect field.
func cleanOptions(fieldType string, in []string) ([]string, error) {
	if fieldType != TypeSelect && fieldType != TypeMultiselect {
		if len(in) > 0 {
			return nil, kernel.Invalid("options", "only select and multiselect fields have options")
		}
		return []string{}, nil
	}
	if len(in) == 0 || len(in) > maxOptions {
		return nil, kernel.Invalid("options", "needs between 1 and 50 options")
	}
	seen, out := map[string]bool{}, make([]string, 0, len(in))
	for _, o := range in {
		o = strings.TrimSpace(o)
		if n := utf8.RuneCountInString(o); n < 1 || n > maxOptionLen {
			return nil, kernel.Invalid("options", "each option is 1-60 characters")
		}
		if seen[strings.ToLower(o)] {
			return nil, kernel.Invalid("options", "options must be unique")
		}
		seen[strings.ToLower(o)] = true
		out = append(out, o)
	}
	return out, nil
}

// checkValue validates v for the field and returns the normalised value to store. isMember tells whether a user id is an
// active member of the project (user fields).
func checkValue(d Definition, v any, isMember func(uuid.UUID) (bool, error)) (any, error) {
	bad := func(msg string) (any, error) { return nil, kernel.Invalid(d.Key, msg) }
	switch d.Type {
	case TypeText:
		s, ok := v.(string)
		if !ok || utf8.RuneCountInString(s) > maxText {
			return bad("must be text of at most 1000 characters")
		}
		return strings.TrimSpace(s), nil
	case TypeNumber:
		f, ok := v.(float64)
		if !ok || math.IsNaN(f) || math.IsInf(f, 0) || math.Abs(f) > maxNumber {
			return bad("must be a finite number")
		}
		return f, nil
	case TypeCheckbox:
		b, ok := v.(bool)
		if !ok {
			return bad("must be true or false")
		}
		return b, nil
	case TypeDate:
		s, _ := v.(string)
		if _, err := kernel.ParseDate(s); err != nil {
			return bad("must be YYYY-MM-DD")
		}
		return s, nil
	case TypeURL:
		return checkURL(d, v)
	case TypeSelect:
		s, _ := v.(string)
		if !contains(d.Options, s) {
			return bad("must be one of the options")
		}
		return s, nil
	case TypeMultiselect:
		return checkMulti(d, v)
	default:
		return checkUser(d, v, isMember)
	}
}

// checkURL accepts https URLs only. Stored URLs are never fetched by the backend (SECURITY.md section 9).
func checkURL(d Definition, v any) (any, error) {
	s, _ := v.(string)
	u, err := url.Parse(s)
	if err != nil || len(s) > maxURL || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return nil, kernel.Invalid(d.Key, "must be an https URL of at most 500 characters")
	}
	return s, nil
}

func checkMulti(d Definition, v any) (any, error) {
	list, ok := v.([]any)
	if !ok || len(list) > maxOptions {
		return nil, kernel.Invalid(d.Key, "must be a list of options")
	}
	seen, out := map[string]bool{}, make([]string, 0, len(list))
	for _, item := range list {
		s, _ := item.(string)
		if !contains(d.Options, s) || seen[s] {
			return nil, kernel.Invalid(d.Key, "must be distinct values from the options")
		}
		seen[s] = true
		out = append(out, s)
	}
	return out, nil
}

func checkUser(d Definition, v any, isMember func(uuid.UUID) (bool, error)) (any, error) {
	s, _ := v.(string)
	id, err := uuid.Parse(s)
	if err != nil {
		return nil, kernel.Invalid(d.Key, "must be a user id")
	}
	ok, err := isMember(id)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, kernel.Invalid(d.Key, "must be an active member of the project")
	}
	return id.String(), nil
}

func contains(list []string, s string) bool {
	for _, o := range list {
		if o == s {
			return true
		}
	}
	return false
}

package collab

import (
	"strings"
	"unicode/utf8"

	"github.com/jaas/jaas/internal/projects/kernel"
)

// Limits (specification sections 3 and 4).
const (
	maxCommentBody = 10000
	maxNote        = 500
)

// cleanText trims s and checks its length in characters; NUL bytes and invalid UTF-8 are refused because the database
// cannot store them.
func cleanText(field, s string, min, max int) (string, error) {
	s = strings.TrimSpace(s)
	if !utf8.ValidString(s) || strings.ContainsRune(s, 0) {
		return "", kernel.Invalid(field, "must be valid text")
	}
	if n := utf8.RuneCountInString(s); n < min || n > max {
		return "", kernel.Invalid(field, "length must be between "+itoa(min)+" and "+itoa(max))
	}
	return s, nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}

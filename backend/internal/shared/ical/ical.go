// Package ical writes RFC 5545 text safely: escaping of TEXT values and folding of long content lines. Used by the M2 task
// feed and the M3 meeting feed.
package ical

import (
	"strings"
	"unicode/utf8"
)

const lineMax = 75

// Escape escapes TEXT per RFC 5545 section 3.3.11: backslash, semicolon and comma are escaped, line breaks become
// the two characters \n, and other control characters are dropped, so a title can never start a new property line.
func Escape(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '\\':
			b.WriteString(`\\`)
		case r == ';':
			b.WriteString(`\;`)
		case r == ',':
			b.WriteString(`\,`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\t':
			b.WriteByte(' ')
		case r < 0x20 || r == 0x7f || r == ' ' || r == ' ':
			// dropped (a bare CR, other C0 controls and Unicode line separators)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Fold folds a content line at 75 octets without splitting a UTF-8 sequence (RFC 5545 section 3.1).
func Fold(line string) string {
	if len(line) <= lineMax {
		return line
	}
	var b strings.Builder
	size, limit := 0, lineMax
	for _, r := range line {
		n := utf8.RuneLen(r)
		if size+n > limit {
			b.WriteString("\r\n ")
			size, limit = 1, lineMax
		}
		b.WriteRune(r)
		size += n
	}
	return b.String()
}

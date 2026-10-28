package planning

import (
	"strings"
	"time"
	"unicode/utf8"
)

const (
	icsStamp   = "20060102T150405Z"
	icsDate    = "20060102"
	icsLineMax = 75
)

// icsEscape escapes TEXT per RFC 5545 section 3.3.11: backslash, semicolon and comma are escaped, line breaks become
// the two characters \n, and other control characters are dropped, so a title can never start a new property line.
func icsEscape(s string) string {
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

// icsFold folds a content line at 75 octets without splitting a UTF-8 sequence (RFC 5545 section 3.1).
func icsFold(line string) string {
	if len(line) <= icsLineMax {
		return line
	}
	var b strings.Builder
	size, limit := 0, icsLineMax
	for _, r := range line {
		n := utf8.RuneLen(r)
		if size+n > limit {
			b.WriteString("\r\n ")
			size, limit = 1, icsLineMax
		}
		b.WriteRune(r)
		size += n
	}
	return b.String()
}

// buildICS renders cards as a VCALENDAR; the output uses CRLF line ends and contains no client-controlled raw line.
func buildICS(cards []Card, now time.Time) string {
	lines := []string{"BEGIN:VCALENDAR", "VERSION:2.0", "PRODID:-//JAAS//Projects//EN", "CALSCALE:GREGORIAN", "METHOD:PUBLISH",
		"X-WR-CALNAME:JAAS tasks"}
	for _, c := range cards {
		lines = append(lines, eventLines(c, now)...)
	}
	lines = append(lines, "END:VCALENDAR")
	var b strings.Builder
	for _, l := range lines {
		b.WriteString(icsFold(l))
		b.WriteString("\r\n")
	}
	return b.String()
}

func eventLines(c Card, now time.Time) []string {
	status := "CONFIRMED"
	if c.Category == "canceled" {
		status = "CANCELLED"
	}
	lines := []string{"BEGIN:VEVENT", "UID:" + c.ID.String() + "@jaas", "DTSTAMP:" + now.UTC().Format(icsStamp),
		"SUMMARY:" + icsEscape("["+c.Ticket+"] "+c.Title), "LAST-MODIFIED:" + c.UpdatedAt.UTC().Format(icsStamp), "STATUS:" + status}
	switch {
	case c.StartOn != nil && c.DueAt != nil:
		end := c.DueAt.UTC().AddDate(0, 0, 1)
		lines = append(lines, "DTSTART;VALUE=DATE:"+c.StartOn.Format(icsDate), "DTEND;VALUE=DATE:"+end.Format(icsDate))
	case c.DueAt != nil:
		lines = append(lines, "DTSTART:"+c.DueAt.UTC().Format(icsStamp))
	case c.StartOn != nil:
		lines = append(lines, "DTSTART;VALUE=DATE:"+c.StartOn.Format(icsDate), "DTEND;VALUE=DATE:"+c.StartOn.AddDate(0, 0, 1).Format(icsDate))
	}
	return append(lines, "END:VEVENT")
}

package planning

import (
	"strings"
	"time"

	"github.com/jaas/jaas/internal/shared/ical"
)

const (
	icsStamp = "20060102T150405Z"
	icsDate  = "20060102"
)

// icsEscape escapes a TEXT value (RFC 5545 section 3.3.11).
func icsEscape(s string) string { return ical.Escape(s) }

// icsFold folds a content line at 75 octets.
func icsFold(line string) string { return ical.Fold(line) }

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

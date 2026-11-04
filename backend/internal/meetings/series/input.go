package series

import (
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/meetings/api"
	"github.com/jaas/jaas/internal/meetings/kernel"
	"github.com/jaas/jaas/internal/meetings/rrule"
)

// LocalLayout is the wall-clock form of a series start ("2027-03-16T09:00").
const LocalLayout = "2006-01-02T15:04"

// ParticipantInput invites a user with a kind and a role (a predefined role key such as "meeting_attendee", or role_id).
type ParticipantInput struct {
	UserID uuid.UUID  `json:"user_id"`
	Kind   string     `json:"kind"`
	Role   string     `json:"role"`
	RoleID *uuid.UUID `json:"role_id"`
}

// CreateInput creates a series (SC-1, RC-1). Start is local wall-clock time in TZ.
type CreateInput struct {
	Title            string             `json:"title"`
	Description      string             `json:"description"`
	Start            string             `json:"start"`
	TZ               string             `json:"tz"`
	DurationMin      int                `json:"duration_min"`
	AllDay           bool               `json:"all_day"`
	RRule            string             `json:"rrule"`
	Visibility       string             `json:"visibility"`
	Confidential     bool               `json:"confidential"`
	Location         string             `json:"location"`
	VideoURL         string             `json:"video_url"`
	DefaultProjectID *uuid.UUID         `json:"default_project_id"`
	RecordingAllowed bool               `json:"recording_allowed"`
	Participants     []ParticipantInput `json:"participants"`
}

// timing is the validated schedule of a series.
type timing struct {
	loc      *time.Location
	local    time.Time // wall clock, as UTC fields
	start    time.Time // first start instant
	duration int
	rule     *rrule.Rule
}

func parseTiming(start, tz string, duration int, allDay bool, rule string) (timing, error) {
	loc, err := time.LoadLocation(strings.TrimSpace(tz))
	if err != nil || tz == "" || strings.EqualFold(tz, "local") {
		return timing{}, kernel.Invalid("tz", "must be an IANA time zone such as Europe/Berlin")
	}
	layout := LocalLayout
	if allDay {
		layout, duration = "2006-01-02", 1440
	}
	local, err := time.Parse(layout, strings.TrimSpace(start))
	if err != nil {
		return timing{}, kernel.Invalid("start", "must be local time "+layout)
	}
	if duration < 1 || duration > 1440 {
		return timing{}, kernel.Invalid("duration_min", "must be 1 to 1440")
	}
	t := timing{loc: loc, local: local, duration: duration}
	t.start = wall(local, loc)
	if strings.TrimSpace(rule) != "" {
		r, err := rrule.Parse(rule)
		if err == nil {
			err = r.CheckUntil(t.start)
		}
		if err != nil {
			return timing{}, kernel.Unprocessable("INVALID_RRULE", err.Error())
		}
		t.rule = &r
	}
	return t, nil
}

// wall turns a wall-clock time (UTC fields) into an instant of loc, with DST gaps resolved as RFC 5545 does.
func wall(local time.Time, loc *time.Location) time.Time { return rrule.WallClock(local, loc) }

// fields validates the descriptive fields of a series.
type fields struct {
	title, description, visibility, location, video string
}

func checkFields(title, description, visibility, location, video string) (fields, error) {
	var f fields
	var err error
	if f.title, err = kernel.Text("title", title, 1, 200); err != nil {
		return f, err
	}
	if f.description, err = kernel.Text("description", description, 0, 20000); err != nil {
		return f, err
	}
	if f.location, err = kernel.Text("location", location, 0, 300); err != nil {
		return f, err
	}
	if visibility == "" {
		visibility = api.VisibilityPrivate
	}
	if err = kernel.OneOf("visibility", visibility, api.VisibilityPrivate, api.VisibilityInternal, api.VisibilityBusyOnly); err != nil {
		return f, err
	}
	f.visibility = visibility
	f.video, err = checkVideoURL(video)
	return f, err
}

// checkVideoURL accepts an https link without credentials; it is stored and never fetched (MR-12, SECURITY.md section 9).
func checkVideoURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	u, err := url.Parse(raw)
	if err != nil || len(raw) > 2048 || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return "", kernel.Invalid("video_url", "must be an https link without credentials")
	}
	return u.String(), nil
}

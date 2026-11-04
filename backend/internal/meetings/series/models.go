package series

import (
	"time"

	"github.com/google/uuid"
)

// Series is one meeting series row; a single meeting is a series without a rule (MR-02).
type Series struct {
	ID               uuid.UUID  `gorm:"type:uuid;primaryKey"`
	TenantID         uuid.UUID  `gorm:"type:uuid"`
	Title            string     ``
	DescriptionMD    string     `gorm:"column:description_md"`
	OrganizerID      uuid.UUID  `gorm:"type:uuid"`
	TZ               string     `gorm:"column:tz"`
	DTStartLocal     time.Time  `gorm:"column:dtstart_local"` // wall-clock time, read and written as UTC fields
	DurationMin      int        ``
	AllDay           bool       ``
	RRule            *string    `gorm:"column:rrule"`
	WindowUntil      *time.Time ``
	Visibility       string     ``
	Confidential     bool       ``
	Location         string     ``
	VideoURL         string     `gorm:"column:video_url"`
	DefaultProjectID *uuid.UUID `gorm:"type:uuid"`
	RecordingAllowed bool       ``
	ICalUID          string     `gorm:"column:ical_uid"`
	Sequence         int        ``
	Status           string     ``
	SplitFrom        *uuid.UUID `gorm:"type:uuid"`
	Version          int        ``
	CreatedBy        uuid.UUID  `gorm:"type:uuid"`
	CreatedAt        time.Time  ``
	UpdatedAt        time.Time  ``
	DeletedAt        *time.Time ``
}

// TableName maps to meeting_series.
func (Series) TableName() string { return "meeting_series" }

// Occurrence is one materialised occurrence row (DMT-02).
type Occurrence struct {
	ID               uuid.UUID  `gorm:"type:uuid;primaryKey"`
	TenantID         uuid.UUID  `gorm:"type:uuid"`
	SeriesID         uuid.UUID  `gorm:"type:uuid"`
	RecurrenceID     time.Time  ``
	StartAt          time.Time  ``
	EndAt            time.Time  ``
	IsException      bool       ``
	TitleOverride    *string    ``
	LocationOverride *string    ``
	Status           string     ``
	StatusManual     bool       ``
	CancelReason     string     ``
	Orphaned         bool       ``
	ReminderSentAt   *time.Time ``
	Version          int        ``
	CreatedAt        time.Time  ``
	UpdatedAt        time.Time  ``
}

// TableName maps to meeting_occurrences.
func (Occurrence) TableName() string { return "meeting_occurrences" }

// Participant is a series member; the role lives in M0 (DMT-09).
type Participant struct {
	TenantID  uuid.UUID  `gorm:"type:uuid"`
	SeriesID  uuid.UUID  `gorm:"type:uuid;primaryKey"`
	UserID    uuid.UUID  `gorm:"type:uuid;primaryKey"`
	Kind      string     ``
	Active    bool       ``
	AddedBy   *uuid.UUID `gorm:"type:uuid"`
	CreatedAt time.Time  ``
	UpdatedAt time.Time  ``
}

// TableName maps to meeting_participants.
func (Participant) TableName() string { return "meeting_participants" }

// Occurrence statuses (MR-05).
const (
	StatusScheduled  = "scheduled"
	StatusInProgress = "in_progress"
	StatusEnded      = "ended"
	StatusPublished  = "minutes_published"
	StatusCancelled  = "cancelled"
	SeriesActive     = "active"
	SeriesCancelled  = "cancelled"
	KindRequired     = "required"
	KindOptional     = "optional"
)

// EffectiveStatus derives in_progress and ended from the clock unless the status was set by hand or is final.
func (o Occurrence) EffectiveStatus(now time.Time) string {
	if o.Status != StatusScheduled || o.StatusManual {
		return o.Status
	}
	switch {
	case !now.Before(o.EndAt):
		return StatusEnded
	case !now.Before(o.StartAt):
		return StatusInProgress
	}
	return StatusScheduled
}

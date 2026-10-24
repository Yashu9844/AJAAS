// Package benefits is HR/Benefits (docs/modules/m1-hr-management/contexts/benefits): tenant-defined plans with
// effective-dated cost versions, eligibility, enrolment windows, elections with dependants, life events and the
// monthly deduction hand-off to payroll.
package benefits

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/hr/kernel"
	"gorm.io/gorm"
)

// Statuses, kinds and reasons (BN-C01..BN-C09).
const (
	PlanDraft   = "draft"
	PlanActive  = "active"
	PlanRetired = "retired"

	ElActive = "active"
	ElWaived = "waived"
	ElEnded  = "ended"

	ReasonChanged     = "changed"
	ReasonEnded       = "ended"
	ReasonWaived      = "waived"
	ReasonReplaced    = "replaced"
	ReasonTermination = "termination"

	WindowActive    = "active"
	WindowCancelled = "cancelled"

	// Window reasons reported by GET /options.
	WindowOpenEnrolment = "open_enrolment"
	WindowNewHire       = "new_hire"
	WindowLifeEvent     = "life_event"
)

// DateLayout is the API calendar-date format.
const DateLayout = "2006-01-02"

// Date is a calendar date stored as DATE and exchanged as "YYYY-MM-DD".
type Date struct{ time.Time }

// MarshalJSON renders YYYY-MM-DD.
func (d Date) MarshalJSON() ([]byte, error) { return []byte(`"` + d.Format(DateLayout) + `"`), nil }

// Value stores the date.
func (d Date) Value() (driver.Value, error) { return d.Format(DateLayout), nil }

// Scan reads a DATE.
func (d *Date) Scan(src interface{}) error {
	switch v := src.(type) {
	case time.Time:
		d.Time = time.Date(v.Year(), v.Month(), v.Day(), 0, 0, 0, 0, time.UTC)
	case string:
		t, err := time.Parse(DateLayout, v[:min(len(v), 10)])
		d.Time = t
		return err
	default:
		return fmt.Errorf("benefits: cannot scan %T as date", src)
	}
	return nil
}

func scanJSON(src interface{}, dst interface{}) error {
	switch v := src.(type) {
	case string:
		return json.Unmarshal([]byte(v), dst)
	case []byte:
		return json.Unmarshal(v, dst)
	}
	return fmt.Errorf("benefits: cannot scan %T as JSON", src)
}

// Eligibility restricts who is offered a plan (BN-C03); an empty list means no restriction.
type Eligibility struct {
	WorkerTypes     []string    `json:"worker_types,omitempty"`
	EmploymentTypes []string    `json:"employment_types,omitempty"`
	GradeIDs        []uuid.UUID `json:"grade_ids,omitempty"`
	LocationIDs     []uuid.UUID `json:"location_ids,omitempty"`
	MinTenureDays   int         `json:"min_tenure_days,omitempty"`
}

// Value encodes the eligibility.
func (e Eligibility) Value() (driver.Value, error) {
	b, err := json.Marshal(e)
	return string(b), err
}

// Scan decodes the eligibility.
func (e *Eligibility) Scan(src interface{}) error { return scanJSON(src, e) }

// Level is one coverage level of a plan version; costs are per month (BN-C02).
type Level struct {
	Code          string       `json:"code"`
	Name          string       `json:"name"`
	EmployeeCost  kernel.Money `json:"employee_cost"`
	EmployerCost  kernel.Money `json:"employer_cost"`
	MaxDependants int          `json:"max_dependants"`
}

// Levels is a version's coverage levels, stored as JSON.
type Levels []Level

// Value encodes the levels.
func (l Levels) Value() (driver.Value, error) {
	if l == nil {
		l = Levels{}
	}
	b, err := json.Marshal([]Level(l))
	return string(b), err
}

// Scan decodes the levels.
func (l *Levels) Scan(src interface{}) error { return scanJSON(src, l) }

// IDs is a list of ids stored as JSON (an election's covered dependants).
type IDs []uuid.UUID

// Value encodes the ids.
func (i IDs) Value() (driver.Value, error) {
	if i == nil {
		i = IDs{}
	}
	b, err := json.Marshal([]uuid.UUID(i))
	return string(b), err
}

// Scan decodes the ids.
func (i *IDs) Scan(src interface{}) error { return scanJSON(src, i) }

func assignID(id *uuid.UUID) error {
	if *id == uuid.Nil {
		*id = uuid.New()
	}
	return nil
}

// Plan is a tenant-defined benefit plan (BN-C01).
type Plan struct {
	ID              uuid.UUID     `gorm:"type:uuid;primaryKey" json:"id"`
	TenantID        uuid.UUID     `gorm:"type:uuid;not null" json:"-"`
	Code            string        `gorm:"type:varchar(30);not null" json:"code"`
	Name            string        `gorm:"type:varchar(150);not null" json:"name"`
	Category        string        `gorm:"type:varchar(12);not null" json:"category"`
	Provider        *string       `gorm:"type:varchar(100)" json:"provider,omitempty"`
	PlanYear        int           `gorm:"not null" json:"plan_year"`
	Eligibility     Eligibility   `gorm:"type:jsonb;not null" json:"eligibility"`
	Status          string        `gorm:"type:varchar(10);not null" json:"status"`
	CreatedByUserID uuid.UUID     `gorm:"type:uuid;not null" json:"-"`
	CreatedAt       time.Time     `gorm:"not null" json:"created_at"`
	UpdatedAt       time.Time     `gorm:"not null" json:"updated_at"`
	Versions        []PlanVersion `gorm:"-" json:"versions,omitempty"`
}

// TableName maps to ben_plans.
func (Plan) TableName() string { return "ben_plans" }

// BeforeCreate assigns a UUID v4 when unset.
func (p *Plan) BeforeCreate(*gorm.DB) error { return assignID(&p.ID) }

// PlanVersion holds a plan's costs over [EffectiveFrom, EffectiveTo) (BN-C02).
type PlanVersion struct {
	TenantID        uuid.UUID `gorm:"type:uuid;not null" json:"-"`
	PlanID          uuid.UUID `gorm:"type:uuid;primaryKey" json:"plan_id"`
	Version         int       `gorm:"primaryKey" json:"version"`
	EffectiveFrom   Date      `gorm:"type:date;not null" json:"effective_from"`
	EffectiveTo     *Date     `gorm:"type:date" json:"effective_to,omitempty"`
	Levels          Levels    `gorm:"type:jsonb;not null" json:"levels"`
	CreatedByUserID uuid.UUID `gorm:"type:uuid;not null" json:"-"`
	CreatedAt       time.Time `gorm:"not null" json:"created_at"`
}

// TableName maps to ben_plan_versions.
func (PlanVersion) TableName() string { return "ben_plan_versions" }

// In reports whether the version is in force on the date.
func (v PlanVersion) In(on time.Time) bool {
	return !on.Before(v.EffectiveFrom.Time) && (v.EffectiveTo == nil || on.Before(v.EffectiveTo.Time))
}

// Level finds a coverage level by code.
func (v PlanVersion) Level(code string) (Level, bool) {
	for _, l := range v.Levels {
		if l.Code == code {
			return l, true
		}
	}
	return Level{}, false
}

// Window is a tenant-wide open-enrolment window (BN-C04).
type Window struct {
	ID              uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	TenantID        uuid.UUID `gorm:"type:uuid;not null" json:"-"`
	Name            string    `gorm:"type:varchar(100);not null" json:"name"`
	StartsOn        Date      `gorm:"type:date;not null" json:"starts_on"`
	EndsOn          Date      `gorm:"type:date;not null" json:"ends_on"`
	Status          string    `gorm:"type:varchar(10);not null" json:"status"`
	CreatedByUserID uuid.UUID `gorm:"type:uuid;not null" json:"-"`
	CreatedAt       time.Time `gorm:"not null" json:"created_at"`
}

// TableName maps to ben_windows.
func (Window) TableName() string { return "ben_windows" }

// BeforeCreate assigns a UUID v4 when unset.
func (w *Window) BeforeCreate(*gorm.DB) error { return assignID(&w.ID) }

// Settings are the tenant's window lengths (BN-C04).
type Settings struct {
	TenantID        uuid.UUID `gorm:"type:uuid;primaryKey" json:"-"`
	NewHireDays     int       `gorm:"not null" json:"new_hire_days"`
	LifeEventDays   int       `gorm:"not null" json:"life_event_days"`
	UpdatedByUserID uuid.UUID `gorm:"type:uuid;not null" json:"-"`
	UpdatedAt       time.Time `gorm:"not null" json:"updated_at"`
}

// TableName maps to ben_settings.
func (Settings) TableName() string { return "ben_settings" }

// Dependant is a person a worker may cover (BN-C09); sensitive.
type Dependant struct {
	ID                uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	TenantID          uuid.UUID `gorm:"type:uuid;not null" json:"-"`
	EmployeeProfileID uuid.UUID `gorm:"type:uuid;not null" json:"employee_id"`
	Name              string    `gorm:"type:varchar(100);not null" json:"name"`
	Relationship      string    `gorm:"type:varchar(10);not null" json:"relationship"`
	DateOfBirth       Date      `gorm:"type:date;not null" json:"date_of_birth"`
	Status            string    `gorm:"type:varchar(10);not null" json:"status"`
	CreatedAt         time.Time `gorm:"not null" json:"created_at"`
	UpdatedAt         time.Time `gorm:"not null" json:"updated_at"`
}

// TableName maps to ben_dependants.
func (Dependant) TableName() string { return "ben_dependants" }

// BeforeCreate assigns a UUID v4 when unset.
func (d *Dependant) BeforeCreate(*gorm.DB) error { return assignID(&d.ID) }

// LifeEvent opens a special enrolment window (BN-C04, BN-C09); sensitive.
type LifeEvent struct {
	ID                uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	TenantID          uuid.UUID `gorm:"type:uuid;not null" json:"-"`
	EmployeeProfileID uuid.UUID `gorm:"type:uuid;not null" json:"employee_id"`
	EventType         string    `gorm:"type:varchar(10);not null" json:"type"`
	EventDate         Date      `gorm:"type:date;not null" json:"event_date"`
	EvidenceRef       *string   `gorm:"type:varchar(300)" json:"evidence_ref,omitempty"`
	RecordedByUserID  uuid.UUID `gorm:"type:uuid;not null" json:"-"`
	CreatedAt         time.Time `gorm:"not null" json:"created_at"`
}

// TableName maps to ben_life_events.
func (LifeEvent) TableName() string { return "ben_life_events" }

// BeforeCreate assigns a UUID v4 when unset.
func (l *LifeEvent) BeforeCreate(*gorm.DB) error { return assignID(&l.ID) }

// Election is a worker's choice for a plan over [EffectiveFrom, EffectiveTo) (BN-C05, BN-C06). A waived election has no level.
type Election struct {
	ID                uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	TenantID          uuid.UUID  `gorm:"type:uuid;not null" json:"-"`
	EmployeeProfileID uuid.UUID  `gorm:"type:uuid;not null" json:"employee_id"`
	PlanID            uuid.UUID  `gorm:"type:uuid;not null" json:"plan_id"`
	LevelCode         *string    `gorm:"type:varchar(30)" json:"level_code,omitempty"`
	DependantIDs      IDs        `gorm:"type:jsonb;not null" json:"dependant_ids"`
	Status            string     `gorm:"type:varchar(10);not null" json:"status"`
	EffectiveFrom     Date       `gorm:"type:date;not null" json:"effective_from"`
	EffectiveTo       *Date      `gorm:"type:date" json:"effective_to,omitempty"`
	EndReason         *string    `gorm:"type:varchar(12)" json:"end_reason,omitempty"`
	EndActionID       *uuid.UUID `gorm:"type:uuid" json:"-"`
	CreatedByUserID   uuid.UUID  `gorm:"type:uuid;not null" json:"-"`
	CreatedAt         time.Time  `gorm:"not null" json:"created_at"`
	UpdatedAt         time.Time  `gorm:"not null" json:"updated_at"`
}

// TableName maps to ben_elections.
func (Election) TableName() string { return "ben_elections" }

// BeforeCreate assigns a UUID v4 when unset.
func (e *Election) BeforeCreate(*gorm.DB) error { return assignID(&e.ID) }

// CoversDay reports whether the election is in force on the date (waived elections cost and cover nothing).
func (e Election) CoversDay(on time.Time) bool {
	return e.Status != ElWaived && !on.Before(e.EffectiveFrom.Time) && (e.EffectiveTo == nil || on.Before(e.EffectiveTo.Time))
}

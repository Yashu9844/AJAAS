// Package models holds Module 7 (Performance) persistence models; migrations 000051–000056 are the SQL twin.
package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/hr/performance/score"
	"gorm.io/gorm"
)

// Review types and statuses (FR-RV, PM-013).
const (
	ReviewSelf    = "self"
	ReviewManager = "manager"
	ReviewPeer    = "peer"

	ReviewPending   = "pending"
	ReviewSubmitted = "submitted"
	ReviewExpired   = "expired"
)

// Criteria is a cycle's ordered rating criteria, stored as a JSON array (PM-001).
type Criteria []string

// Value encodes the criteria as JSON.
func (c Criteria) Value() (driver.Value, error) { return marshalJSON(c) }

// Scan decodes the criteria from JSON.
func (c *Criteria) Scan(src interface{}) error { return unmarshalJSON(src, c) }

// Rating is one criterion score in a review (PM-012).
type Rating struct {
	Criterion string `json:"criterion"`
	Score     int    `json:"score"`
}

// Ratings is a review's scores, stored as a JSON array.
type Ratings []Rating

// Value encodes the ratings as JSON.
func (r Ratings) Value() (driver.Value, error) { return marshalJSON(r) }

// Scan decodes the ratings from JSON.
func (r *Ratings) Scan(src interface{}) error { return unmarshalJSON(src, r) }

func marshalJSON(v interface{}) (driver.Value, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	if string(b) == "null" {
		return "[]", nil
	}
	return string(b), nil
}

func unmarshalJSON(src interface{}, dst interface{}) error {
	switch v := src.(type) {
	case nil:
		return nil
	case string:
		return json.Unmarshal([]byte(v), dst)
	case []byte:
		return json.Unmarshal(v, dst)
	default:
		return fmt.Errorf("models: cannot scan %T as JSON", src)
	}
}

// Cycle is an appraisal cycle (FR-CY).
type Cycle struct {
	ID              uuid.UUID `gorm:"type:uuid;primaryKey"`
	TenantID        uuid.UUID `gorm:"type:uuid;not null;index:idx_performance_cycles_tenant_status,priority:1"`
	Title           string    `gorm:"type:varchar(150);not null"`
	StartDate       time.Time `gorm:"type:date;not null"`
	EndDate         time.Time `gorm:"type:date;not null"`
	Criteria        Criteria  `gorm:"type:jsonb;not null"`
	Status          string    `gorm:"type:varchar(20);not null;index:idx_performance_cycles_tenant_status,priority:2"`
	ActivatedAt     *time.Time
	ReviewStartedAt *time.Time
	CompletedAt     *time.Time
	CreatedByUserID uuid.UUID `gorm:"type:uuid;not null"`
	CreatedAt       time.Time `gorm:"not null"`
	UpdatedAt       time.Time `gorm:"not null"`
}

// TableName maps to performance_cycles (migration 000051).
func (Cycle) TableName() string { return "performance_cycles" }

// BeforeCreate assigns a UUID v4 when unset.
func (c *Cycle) BeforeCreate(tx *gorm.DB) error { return assignID(&c.ID) }

// Goal is one employee objective in a cycle (FR-GL).
type Goal struct {
	ID              uuid.UUID   `gorm:"type:uuid;primaryKey"`
	TenantID        uuid.UUID   `gorm:"type:uuid;not null;index:idx_performance_goals_cycle_employee,priority:1"`
	CycleID         uuid.UUID   `gorm:"type:uuid;not null;index:idx_performance_goals_cycle_employee,priority:2"`
	EmployeeID      uuid.UUID   `gorm:"type:uuid;not null;index:idx_performance_goals_cycle_employee,priority:3"`
	ParentGoalID    *uuid.UUID  `gorm:"type:uuid;index:idx_performance_goals_parent"`
	Title           string      `gorm:"type:varchar(200);not null"`
	Description     *string     `gorm:"type:text"`
	Weight          score.Fixed `gorm:"type:numeric(5,2);not null"`
	Progress        score.Fixed `gorm:"type:numeric(5,2);not null"`
	Status          string      `gorm:"type:varchar(20);not null"`
	CreatedByUserID uuid.UUID   `gorm:"type:uuid;not null"`
	CreatedAt       time.Time   `gorm:"not null"`
	UpdatedAt       time.Time   `gorm:"not null"`
}

// TableName maps to performance_goals (migration 000052).
func (Goal) TableName() string { return "performance_goals" }

// BeforeCreate assigns a UUID v4 when unset.
func (g *Goal) BeforeCreate(tx *gorm.DB) error { return assignID(&g.ID) }

// KeyResult is a measurable target of a goal (FR-GL005).
type KeyResult struct {
	ID           uuid.UUID   `gorm:"type:uuid;primaryKey"`
	TenantID     uuid.UUID   `gorm:"type:uuid;not null"`
	GoalID       uuid.UUID   `gorm:"type:uuid;not null;index:idx_performance_key_results_goal"`
	Title        string      `gorm:"type:varchar(200);not null"`
	Unit         *string     `gorm:"type:varchar(20)"`
	StartValue   score.Fixed `gorm:"type:numeric(14,2);not null"`
	TargetValue  score.Fixed `gorm:"type:numeric(14,2);not null"`
	CurrentValue score.Fixed `gorm:"type:numeric(14,2);not null"`
	CreatedAt    time.Time   `gorm:"not null"`
	UpdatedAt    time.Time   `gorm:"not null"`
}

// TableName maps to performance_key_results (migration 000053).
func (KeyResult) TableName() string { return "performance_key_results" }

// BeforeCreate assigns a UUID v4 when unset.
func (k *KeyResult) BeforeCreate(tx *gorm.DB) error { return assignID(&k.ID) }

// CheckIn is one append-only progress update (PM-007).
type CheckIn struct {
	ID           uuid.UUID    `gorm:"type:uuid;primaryKey"`
	TenantID     uuid.UUID    `gorm:"type:uuid;not null"`
	GoalID       uuid.UUID    `gorm:"type:uuid;not null;index:idx_performance_checkins_goal"`
	KeyResultID  *uuid.UUID   `gorm:"type:uuid"`
	FromProgress score.Fixed  `gorm:"type:numeric(5,2);not null"`
	ToProgress   score.Fixed  `gorm:"type:numeric(5,2);not null"`
	Value        *score.Fixed `gorm:"type:numeric(14,2)"`
	Note         *string      `gorm:"type:varchar(1000)"`
	AuthorUserID uuid.UUID    `gorm:"type:uuid;not null"`
	CreatedAt    time.Time    `gorm:"not null"`
}

// TableName maps to performance_checkins (migration 000054).
func (CheckIn) TableName() string { return "performance_checkins" }

// BeforeCreate assigns a UUID v4 when unset.
func (c *CheckIn) BeforeCreate(tx *gorm.DB) error { return assignID(&c.ID) }

// Review is one reviewer's assessment of one employee in one cycle (FR-RV).
type Review struct {
	ID                 uuid.UUID    `gorm:"type:uuid;primaryKey"`
	TenantID           uuid.UUID    `gorm:"type:uuid;not null;index:idx_performance_reviews_reviewer,priority:1"`
	CycleID            uuid.UUID    `gorm:"type:uuid;not null;uniqueIndex:uq_performance_reviews_assignment,priority:1"`
	EmployeeID         uuid.UUID    `gorm:"type:uuid;not null;uniqueIndex:uq_performance_reviews_assignment,priority:2"`
	ReviewerEmployeeID uuid.UUID    `gorm:"type:uuid;not null;uniqueIndex:uq_performance_reviews_assignment,priority:3;index:idx_performance_reviews_reviewer,priority:2"`
	Type               string       `gorm:"type:varchar(10);not null;uniqueIndex:uq_performance_reviews_assignment,priority:4"`
	Status             string       `gorm:"type:varchar(20);not null"`
	Ratings            Ratings      `gorm:"type:jsonb;not null"`
	Strengths          *string      `gorm:"type:text"`
	Improvements       *string      `gorm:"type:text"`
	OverallScore       *score.Fixed `gorm:"type:numeric(3,2)"`
	Potential          *string      `gorm:"type:varchar(10)"`
	SubmittedAt        *time.Time
	CreatedAt          time.Time `gorm:"not null"`
	UpdatedAt          time.Time `gorm:"not null"`
}

// TableName maps to performance_reviews (migration 000055).
func (Review) TableName() string { return "performance_reviews" }

// BeforeCreate assigns a UUID v4 when unset.
func (r *Review) BeforeCreate(tx *gorm.DB) error { return assignID(&r.ID) }

// OutboxEvent is a transactional-outbox row (FR-EV001).
type OutboxEvent struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey"`
	TenantID    uuid.UUID `gorm:"type:uuid;not null;index:idx_performance_outbox_tenant"`
	EventID     uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:uq_performance_outbox_event_id"`
	EventType   string    `gorm:"type:varchar(100);not null"`
	RoutingKey  string    `gorm:"type:varchar(100);not null"`
	Payload     string    `gorm:"type:jsonb;not null"`
	Published   bool      `gorm:"not null;index:idx_performance_outbox_published,where:published = false"`
	PublishedAt *time.Time
	Attempts    int       `gorm:"type:integer;not null"`
	LastError   *string   `gorm:"type:text"`
	CreatedAt   time.Time `gorm:"not null"`
}

// TableName maps to performance_events_outbox (migration 000056).
func (OutboxEvent) TableName() string { return "performance_events_outbox" }

// BeforeCreate assigns a UUID v4 when unset.
func (o *OutboxEvent) BeforeCreate(tx *gorm.DB) error { return assignID(&o.ID) }

func assignID(id *uuid.UUID) error {
	if *id == uuid.Nil {
		*id = uuid.New()
	}
	return nil
}

// Package models holds Module 5 (Payroll) persistence models; migrations 000038–000044 are the SQL twin.
package models

import (
	"time"

	"github.com/google/uuid"
	leavecalc "github.com/jaas/jaas/internal/leave/calc"
	"github.com/jaas/jaas/internal/payroll/calc"
	"github.com/jaas/jaas/internal/shared/database"
	"gorm.io/gorm"
)

// Enumerations (specification.md §3).
const (
	StructureActive   = "active"
	StructureInactive = "inactive"

	KindEarning   = "earning"
	KindDeduction = "deduction"

	CalcFixed          = "fixed"
	CalcPercentOfCTC   = "percent_of_ctc"
	CalcPercentOfBasic = "percent_of_basic"
	CalcBalance        = "balance"

	BasicCode = "BASIC"

	RunDraft      = "draft"
	RunCalculated = "calculated"
	RunApproved   = "approved"
	RunFinalized  = "finalized"
)

// Structure is a salary template with statutory switches (FR-ST001).
type Structure struct {
	database.BaseModel
	TenantID    uuid.UUID `gorm:"type:uuid;not null;index:idx_payroll_structures_tenant_id;uniqueIndex:uq_payroll_structures_tenant_name,priority:1,where:deleted_at IS NULL"`
	Name        string    `gorm:"type:varchar(100);not null;uniqueIndex:uq_payroll_structures_tenant_name,priority:2,expression:lower(name)"`
	Description *string   `gorm:"type:varchar(500)"`
	PFEnabled   bool      `gorm:"column:pf_enabled;not null"`
	ESIEnabled  bool      `gorm:"column:esi_enabled;not null"`
	PTEnabled   bool      `gorm:"column:pt_enabled;not null"`
	TDSEnabled  bool      `gorm:"column:tds_enabled;not null"`
	Status      string    `gorm:"type:varchar(20);not null"`
}

// TableName maps to payroll_structures (migration 000038).
func (Structure) TableName() string { return "payroll_structures" }

// BeforeCreate assigns a UUID v4 when unset.
func (s *Structure) BeforeCreate(tx *gorm.DB) error { return assignID(&s.ID) }

// Component is one immutable line of a structure (FR-ST001, PY-015). Value is rupees (fixed) or percent (percent_*).
type Component struct {
	ID          uuid.UUID  `gorm:"type:uuid;primaryKey"`
	TenantID    uuid.UUID  `gorm:"type:uuid;not null"`
	StructureID uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex:uq_payroll_components_structure_code,priority:1"`
	Code        string     `gorm:"type:varchar(20);not null;uniqueIndex:uq_payroll_components_structure_code,priority:2"`
	Name        string     `gorm:"type:varchar(100);not null"`
	Kind        string     `gorm:"type:varchar(10);not null"`
	Calc        string     `gorm:"type:varchar(20);not null"`
	Value       calc.Money `gorm:"type:numeric(14,2);not null"`
	Taxable     bool       `gorm:"not null"`
	Position    int        `gorm:"type:integer;not null"`
}

// TableName maps to payroll_components (migration 000039).
func (Component) TableName() string { return "payroll_components" }

// BeforeCreate assigns a UUID v4 when unset.
func (c *Component) BeforeCreate(tx *gorm.DB) error { return assignID(&c.ID) }

// Assignment binds an employee to a structure and annual CTC from a date (FR-AS001, PY-014).
type Assignment struct {
	ID                uuid.UUID  `gorm:"type:uuid;primaryKey"`
	TenantID          uuid.UUID  `gorm:"type:uuid;not null;index:idx_payroll_assignments_employee,priority:1"`
	EmployeeProfileID uuid.UUID  `gorm:"type:uuid;not null;index:idx_payroll_assignments_employee,priority:2"`
	StructureID       uuid.UUID  `gorm:"type:uuid;not null"`
	AnnualCTC         calc.Money `gorm:"column:annual_ctc;type:numeric(14,2);not null"`
	EffectiveFrom     time.Time  `gorm:"type:date;not null;index:idx_payroll_assignments_employee,priority:3"`
	EffectiveTo       *time.Time `gorm:"type:date"`
	CreatedByUserID   uuid.UUID  `gorm:"type:uuid;not null"`
	CreatedAt         time.Time  `gorm:"not null"`
	UpdatedAt         time.Time  `gorm:"not null"`
}

// TableName maps to payroll_assignments (migration 000040).
func (Assignment) TableName() string { return "payroll_assignments" }

// BeforeCreate assigns a UUID v4 when unset.
func (a *Assignment) BeforeCreate(tx *gorm.DB) error { return assignID(&a.ID) }

// Run is one tenant payroll period (FR-RN, PY-010). Warnings is a JSON array of strings.
type Run struct {
	ID                 uuid.UUID  `gorm:"type:uuid;primaryKey"`
	TenantID           uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex:uq_payroll_runs_tenant_period,priority:1"`
	Year               int        `gorm:"type:integer;not null;uniqueIndex:uq_payroll_runs_tenant_period,priority:2"`
	Month              int        `gorm:"type:integer;not null;uniqueIndex:uq_payroll_runs_tenant_period,priority:3"`
	Status             string     `gorm:"type:varchar(20);not null"`
	EmployeeCount      int        `gorm:"type:integer;not null"`
	GrossTotal         calc.Money `gorm:"type:numeric(14,2);not null"`
	DeductionTotal     calc.Money `gorm:"type:numeric(14,2);not null"`
	NetTotal           calc.Money `gorm:"type:numeric(14,2);not null"`
	Warnings           string     `gorm:"type:jsonb;not null"`
	CreatedByUserID    uuid.UUID  `gorm:"type:uuid;not null"`
	CalculatedByUserID *uuid.UUID `gorm:"type:uuid"`
	ApprovedByUserID   *uuid.UUID `gorm:"type:uuid"`
	CalculatedAt       *time.Time
	ApprovedAt         *time.Time
	FinalizedAt        *time.Time
	CreatedAt          time.Time `gorm:"not null"`
	UpdatedAt          time.Time `gorm:"not null"`
}

// TableName maps to payroll_runs (migration 000041).
func (Run) TableName() string { return "payroll_runs" }

// BeforeCreate assigns a UUID v4 when unset.
func (r *Run) BeforeCreate(tx *gorm.DB) error { return assignID(&r.ID) }

// Payslip is one employee's result in a run, with an employee snapshot (FR-PS001, D5-09).
type Payslip struct {
	ID                uuid.UUID      `gorm:"type:uuid;primaryKey"`
	TenantID          uuid.UUID      `gorm:"type:uuid;not null;index:idx_payroll_payslips_employee,priority:1"`
	RunID             uuid.UUID      `gorm:"type:uuid;not null;uniqueIndex:uq_payroll_payslips_run_employee,priority:1"`
	EmployeeProfileID uuid.UUID      `gorm:"type:uuid;not null;uniqueIndex:uq_payroll_payslips_run_employee,priority:2;index:idx_payroll_payslips_employee,priority:2"`
	EmployeeCode      string         `gorm:"type:varchar(50);not null"`
	EmployeeName      string         `gorm:"type:varchar(200);not null"`
	StructureID       uuid.UUID      `gorm:"type:uuid;not null"`
	AnnualCTC         calc.Money     `gorm:"column:annual_ctc;type:numeric(14,2);not null"`
	DaysInMonth       int            `gorm:"type:integer;not null"`
	PayableDays       leavecalc.Days `gorm:"type:numeric(7,2);not null"`
	LopDays           leavecalc.Days `gorm:"type:numeric(7,2);not null"`
	Gross             calc.Money     `gorm:"type:numeric(14,2);not null"`
	Deductions        calc.Money     `gorm:"type:numeric(14,2);not null"`
	Net               calc.Money     `gorm:"type:numeric(14,2);not null"`
	Lines             []PayslipLine  `gorm:"foreignKey:PayslipID"`
	CreatedAt         time.Time      `gorm:"not null"`
}

// TableName maps to payroll_payslips (migration 000042).
func (Payslip) TableName() string { return "payroll_payslips" }

// BeforeCreate assigns a UUID v4 when unset.
func (p *Payslip) BeforeCreate(tx *gorm.DB) error { return assignID(&p.ID) }

// PayslipLine is one earning or deduction on a payslip, statutory ones included (code PF, ESI, PT, TDS).
type PayslipLine struct {
	ID        uuid.UUID  `gorm:"type:uuid;primaryKey"`
	PayslipID uuid.UUID  `gorm:"type:uuid;not null;index:idx_payroll_payslip_lines_payslip"`
	Code      string     `gorm:"type:varchar(20);not null"`
	Name      string     `gorm:"type:varchar(100);not null"`
	Kind      string     `gorm:"type:varchar(10);not null"`
	Amount    calc.Money `gorm:"type:numeric(14,2);not null"`
	Position  int        `gorm:"type:integer;not null"`
}

// TableName maps to payroll_payslip_lines (migration 000043).
func (PayslipLine) TableName() string { return "payroll_payslip_lines" }

// BeforeCreate assigns a UUID v4 when unset.
func (l *PayslipLine) BeforeCreate(tx *gorm.DB) error { return assignID(&l.ID) }

// OutboxEvent is a transactional-outbox row (FR-EV001).
type OutboxEvent struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey"`
	TenantID    uuid.UUID `gorm:"type:uuid;not null;index:idx_payroll_outbox_tenant"`
	EventID     uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:uq_payroll_outbox_event_id"`
	EventType   string    `gorm:"type:varchar(100);not null"`
	RoutingKey  string    `gorm:"type:varchar(100);not null"`
	Payload     string    `gorm:"type:jsonb;not null"`
	Published   bool      `gorm:"not null;index:idx_payroll_outbox_published,where:published = false"`
	PublishedAt *time.Time
	Attempts    int       `gorm:"type:integer;not null"`
	LastError   *string   `gorm:"type:text"`
	CreatedAt   time.Time `gorm:"not null"`
}

// TableName maps to payroll_events_outbox (migration 000044).
func (OutboxEvent) TableName() string { return "payroll_events_outbox" }

// BeforeCreate assigns a UUID v4 when unset.
func (o *OutboxEvent) BeforeCreate(tx *gorm.DB) error { return assignID(&o.ID) }

func assignID(id *uuid.UUID) error {
	if *id == uuid.Nil {
		*id = uuid.New()
	}
	return nil
}

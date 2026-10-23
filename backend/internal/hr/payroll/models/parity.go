package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/hr/payroll/calc"
	"gorm.io/gorm"
)

// KindEmployer is the line kind of employer contributions (PP-01); they never count towards gross, deductions or net.
const KindEmployer = calc.Employer

// SegmentRecord is one priced slice of a payslip (PP-03); From and To are inclusive dates.
type SegmentRecord struct {
	From        string     `json:"from"`
	To          string     `json:"to"`
	StructureID uuid.UUID  `json:"structure_id"`
	AnnualCTC   calc.Money `json:"annual_ctc"`
	Payable     int64      `json:"payable"` // payable days × 100
}

// SegmentRecords is a payslip's segments, stored as JSON.
type SegmentRecords []SegmentRecord

// Value encodes the segments.
func (s SegmentRecords) Value() (driver.Value, error) {
	if s == nil {
		return "[]", nil
	}
	b, err := json.Marshal([]SegmentRecord(s))
	return string(b), err
}

// Scan decodes the segments.
func (s *SegmentRecords) Scan(src interface{}) error {
	switch v := src.(type) {
	case string:
		return json.Unmarshal([]byte(v), s)
	case []byte:
		return json.Unmarshal(v, s)
	case nil:
		*s = nil
		return nil
	}
	return fmt.Errorf("payroll: cannot scan %T as segments", src)
}

// Arrear is the amount a run settled for an earlier finalized payslip (PP-04).
type Arrear struct {
	ID                uuid.UUID  `gorm:"type:uuid;primaryKey"`
	TenantID          uuid.UUID  `gorm:"type:uuid;not null"`
	RunID             uuid.UUID  `gorm:"type:uuid;not null"`
	EmployeeProfileID uuid.UUID  `gorm:"type:uuid;not null"`
	SourcePayslipID   uuid.UUID  `gorm:"type:uuid;not null"`
	Amount            calc.Money `gorm:"type:numeric(14,2);not null"`
	CreatedAt         time.Time  `gorm:"not null"`
}

// TableName maps to payroll_arrears (migration 000078).
func (Arrear) TableName() string { return "payroll_arrears" }

// BeforeCreate assigns a UUID v4 when unset.
func (a *Arrear) BeforeCreate(*gorm.DB) error { return assignID(&a.ID) }

// PTSlab is a tenant professional-tax slab of a state (PP-02, PP-09). Zero MaxGross and Month mean unbounded / every month.
type PTSlab struct {
	ID              uuid.UUID   `gorm:"type:uuid;primaryKey" json:"-"`
	TenantID        uuid.UUID   `gorm:"type:uuid;not null" json:"-"`
	StateCode       string      `gorm:"type:varchar(2);not null" json:"state_code"`
	MinGross        calc.Money  `gorm:"type:numeric(14,2);not null" json:"min_gross"`
	MaxGross        *calc.Money `gorm:"type:numeric(14,2)" json:"max_gross,omitempty"`
	Amount          calc.Money  `gorm:"type:numeric(14,2);not null" json:"amount"`
	Month           *int        `gorm:"type:smallint" json:"month,omitempty"`
	CreatedByUserID uuid.UUID   `gorm:"type:uuid;not null" json:"-"`
	CreatedAt       time.Time   `gorm:"not null" json:"created_at"`
}

// TableName maps to payroll_pt_slabs (migration 000079).
func (PTSlab) TableName() string { return "payroll_pt_slabs" }

// BeforeCreate assigns a UUID v4 when unset.
func (s *PTSlab) BeforeCreate(*gorm.DB) error { return assignID(&s.ID) }

// Calc converts the stored slab to the pure calculation type.
func (s PTSlab) Calc() calc.PTSlab {
	out := calc.PTSlab{MinGross: s.MinGross, Amount: s.Amount}
	if s.MaxGross != nil {
		out.MaxGross = *s.MaxGross
	}
	if s.Month != nil {
		out.Month = *s.Month
	}
	return out
}

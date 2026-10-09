// Package dto holds Module 5 request/response shapes (specification.md §5); never GORM models.
package dto

import (
	"strconv"
	"time"

	"github.com/google/uuid"
	leavecalc "github.com/jaas/jaas/internal/leave/calc"
	"github.com/jaas/jaas/internal/payroll/calc"
)

// Pagination bounds (spec §5).
const (
	DefaultPage    = 1
	DefaultPerPage = 20
	MaxPerPage     = 100
)

// ComponentRequest is one structure line (FR-ST001); Value is rupees (fixed) or a percent (percent_*).
type ComponentRequest struct {
	Code    string     `json:"code" binding:"required"`
	Name    string     `json:"name" binding:"required,min=1,max=100"`
	Kind    string     `json:"kind" binding:"required,oneof=earning deduction"`
	Calc    string     `json:"calc" binding:"required,oneof=fixed percent_of_ctc percent_of_basic balance"`
	Value   calc.Money `json:"value"`
	Taxable *bool      `json:"taxable"`
}

// CreateStructureRequest is P1; statutory flags default to true.
type CreateStructureRequest struct {
	Name        string             `json:"name" binding:"required,min=1,max=100"`
	Description *string            `json:"description" binding:"omitempty,max=500"`
	PFEnabled   *bool              `json:"pf_enabled"`
	ESIEnabled  *bool              `json:"esi_enabled"`
	PTEnabled   *bool              `json:"pt_enabled"`
	TDSEnabled  *bool              `json:"tds_enabled"`
	Components  []ComponentRequest `json:"components" binding:"required,min=1,max=40,dive"`
}

// UpdateStructureRequest is P4: components are immutable (PY-015).
type UpdateStructureRequest struct {
	Name        *string `json:"name" binding:"omitempty,min=1,max=100"`
	Description *string `json:"description" binding:"omitempty,max=500"`
	PFEnabled   *bool   `json:"pf_enabled"`
	ESIEnabled  *bool   `json:"esi_enabled"`
	PTEnabled   *bool   `json:"pt_enabled"`
	TDSEnabled  *bool   `json:"tds_enabled"`
}

// AssignRequest is P6.
type AssignRequest struct {
	EmployeeID    string     `json:"employee_id" binding:"required,uuid"`
	StructureID   string     `json:"structure_id" binding:"required,uuid"`
	AnnualCTC     calc.Money `json:"annual_ctc" binding:"required"`
	EffectiveFrom string     `json:"effective_from" binding:"required"`
}

// PreviewRequest is P9.
type PreviewRequest struct {
	StructureID string     `json:"structure_id" binding:"required,uuid"`
	AnnualCTC   calc.Money `json:"annual_ctc" binding:"required"`
}

// CreateRunRequest is P10.
type CreateRunRequest struct {
	Month int `json:"month" binding:"required,min=1,max=12"`
	Year  int `json:"year" binding:"required,min=2000,max=2200"`
}

// ComponentResponse serializes a component.
type ComponentResponse struct {
	Code     string     `json:"code"`
	Name     string     `json:"name"`
	Kind     string     `json:"kind"`
	Calc     string     `json:"calc"`
	Value    calc.Money `json:"value"`
	Taxable  bool       `json:"taxable"`
	Position int        `json:"position"`
}

// StructureResponse serializes a structure with its components.
type StructureResponse struct {
	ID          uuid.UUID           `json:"id"`
	Name        string              `json:"name"`
	Description *string             `json:"description,omitempty"`
	PFEnabled   bool                `json:"pf_enabled"`
	ESIEnabled  bool                `json:"esi_enabled"`
	PTEnabled   bool                `json:"pt_enabled"`
	TDSEnabled  bool                `json:"tds_enabled"`
	Status      string              `json:"status"`
	Components  []ComponentResponse `json:"components"`
	CreatedAt   time.Time           `json:"created_at"`
	UpdatedAt   time.Time           `json:"updated_at"`
}

// AssignmentResponse serializes an assignment.
type AssignmentResponse struct {
	ID            uuid.UUID  `json:"id"`
	EmployeeID    uuid.UUID  `json:"employee_id"`
	StructureID   uuid.UUID  `json:"structure_id"`
	AnnualCTC     calc.Money `json:"annual_ctc"`
	EffectiveFrom string     `json:"effective_from"`
	EffectiveTo   *string    `json:"effective_to,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
}

// LineResponse is one breakdown or payslip line.
type LineResponse struct {
	Code   string     `json:"code"`
	Name   string     `json:"name"`
	Kind   string     `json:"kind"`
	Amount calc.Money `json:"amount"`
}

// BreakdownResponse is a full-month salary breakdown with statutory deductions (P8/P9).
type BreakdownResponse struct {
	MonthlyCTC calc.Money     `json:"monthly_ctc"`
	Lines      []LineResponse `json:"lines"`
	Gross      calc.Money     `json:"gross"`
	Deductions calc.Money     `json:"deductions"`
	Net        calc.Money     `json:"net"`
}

// MyAssignmentResponse is P8.
type MyAssignmentResponse struct {
	Assignment AssignmentResponse `json:"assignment"`
	Breakdown  BreakdownResponse  `json:"breakdown"`
}

// RunResponse serializes a payroll run.
type RunResponse struct {
	ID                 uuid.UUID  `json:"id"`
	Month              int        `json:"month"`
	Year               int        `json:"year"`
	Status             string     `json:"status"`
	EmployeeCount      int        `json:"employee_count"`
	GrossTotal         calc.Money `json:"gross_total"`
	DeductionTotal     calc.Money `json:"deduction_total"`
	NetTotal           calc.Money `json:"net_total"`
	Warnings           []string   `json:"warnings"`
	CalculatedByUserID *uuid.UUID `json:"calculated_by_user_id,omitempty"`
	ApprovedByUserID   *uuid.UUID `json:"approved_by_user_id,omitempty"`
	CalculatedAt       *time.Time `json:"calculated_at,omitempty"`
	ApprovedAt         *time.Time `json:"approved_at,omitempty"`
	FinalizedAt        *time.Time `json:"finalized_at,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
}

// PayslipResponse serializes a payslip.
type PayslipResponse struct {
	ID           uuid.UUID      `json:"id"`
	RunID        uuid.UUID      `json:"run_id"`
	Month        int            `json:"month"`
	Year         int            `json:"year"`
	EmployeeID   uuid.UUID      `json:"employee_id"`
	EmployeeCode string         `json:"employee_code"`
	EmployeeName string         `json:"employee_name"`
	StructureID  uuid.UUID      `json:"structure_id"`
	AnnualCTC    calc.Money     `json:"annual_ctc"`
	DaysInMonth  int            `json:"days_in_month"`
	PayableDays  leavecalc.Days `json:"payable_days"`
	LopDays      leavecalc.Days `json:"lop_days"`
	Gross        calc.Money     `json:"gross"`
	Deductions   calc.Money     `json:"deductions"`
	Net          calc.Money     `json:"net"`
	Lines        []LineResponse `json:"lines"`
	CreatedAt    time.Time      `json:"created_at"`
}

// PageMeta is the list envelope meta.
type PageMeta struct {
	Page       int   `json:"page"`
	PerPage    int   `json:"per_page"`
	TotalItems int64 `json:"total_items"`
	TotalPages int   `json:"total_pages"`
}

// Page is a clamped page request (G16).
type Page struct {
	Page    int
	PerPage int
}

// ParsePage clamps raw query values (spec §5).
func ParsePage(pageStr, perPageStr string) Page {
	page, err := strconv.Atoi(pageStr)
	if err != nil || page < 1 {
		page = DefaultPage
	}
	perPage, err := strconv.Atoi(perPageStr)
	if err != nil || perPage < 1 {
		perPage = DefaultPerPage
	}
	if perPage > MaxPerPage {
		perPage = MaxPerPage
	}
	return Page{Page: page, PerPage: perPage}
}

// Offset is the SQL offset.
func (p Page) Offset() int { return (p.Page - 1) * p.PerPage }

// Meta builds list metadata.
func (p Page) Meta(total int64) PageMeta {
	return PageMeta{Page: p.Page, PerPage: p.PerPage, TotalItems: total, TotalPages: int((total + int64(p.PerPage) - 1) / int64(p.PerPage))}
}

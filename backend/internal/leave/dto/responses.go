package dto

import (
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/leave/calc"
)

// Pagination bounds (spec §5).
const (
	DefaultPage    = 1
	DefaultPerPage = 20
	MaxPerPage     = 100
)

// LeaveTypeResponse serializes a leave type.
type LeaveTypeResponse struct {
	ID                 uuid.UUID `json:"id"`
	Name               string    `json:"name"`
	Code               string    `json:"code"`
	IsPaid             bool      `json:"is_paid"`
	AnnualAllowance    calc.Days `json:"annual_allowance"`
	Accrual            string    `json:"accrual"`
	CarryForwardLimit  calc.Days `json:"carry_forward_limit"`
	MaxConsecutiveDays *int      `json:"max_consecutive_days,omitempty"`
	MinNoticeDays      int       `json:"min_notice_days"`
	AllowHalfDay       bool      `json:"allow_half_day"`
	SandwichRule       bool      `json:"sandwich_rule"`
	ApplicableGender   string    `json:"applicable_gender"`
	Status             string    `json:"status"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// HolidayResponse serializes a holiday.
type HolidayResponse struct {
	ID         uuid.UUID `json:"id"`
	Date       string    `json:"date"`
	Name       string    `json:"name"`
	IsOptional bool      `json:"is_optional"`
	CreatedAt  time.Time `json:"created_at"`
}

// BalanceResponse serializes one balance projection.
type BalanceResponse struct {
	LeaveTypeID   uuid.UUID `json:"leave_type_id"`
	LeaveTypeCode string    `json:"leave_type_code"`
	LeaveTypeName string    `json:"leave_type_name"`
	Year          int       `json:"year"`
	Opening       calc.Days `json:"opening"`
	Accrued       calc.Days `json:"accrued"`
	Adjusted      calc.Days `json:"adjusted"`
	Used          calc.Days `json:"used"`
	Reserved      calc.Days `json:"reserved"`
	Available     calc.Days `json:"available"`
	IsPaid        bool      `json:"is_paid"`
}

// LedgerEntryResponse serializes one ledger row.
type LedgerEntryResponse struct {
	ID             uuid.UUID  `json:"id"`
	LeaveTypeID    uuid.UUID  `json:"leave_type_id"`
	Year           int        `json:"year"`
	Kind           string     `json:"kind"`
	Days           calc.Days  `json:"days"`
	LeaveRequestID *uuid.UUID `json:"leave_request_id,omitempty"`
	Note           *string    `json:"note,omitempty"`
	ActorUserID    *uuid.UUID `json:"actor_user_id,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

// LeaveRequestResponse serializes a leave request.
type LeaveRequestResponse struct {
	ID             uuid.UUID  `json:"id"`
	EmployeeID     uuid.UUID  `json:"employee_id"`
	LeaveTypeID    uuid.UUID  `json:"leave_type_id"`
	LeaveTypeCode  string     `json:"leave_type_code"`
	StartDate      string     `json:"start_date"`
	EndDate        string     `json:"end_date"`
	HalfDay        *string    `json:"half_day,omitempty"`
	TotalDays      calc.Days  `json:"total_days"`
	Reason         string     `json:"reason"`
	Status         string     `json:"status"`
	ReviewerUserID *uuid.UUID `json:"reviewer_user_id,omitempty"`
	ReviewedAt     *time.Time `json:"reviewed_at,omitempty"`
	ReviewComment  *string    `json:"review_comment,omitempty"`
	CancelledAt    *time.Time `json:"cancelled_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// PreviewResponse is R1: the price of a request without saving it.
type PreviewResponse struct {
	TotalDays      calc.Days `json:"total_days"`
	Available      calc.Days `json:"available"`
	AvailableAfter calc.Days `json:"available_after"`
	WorkingDates   []string  `json:"working_dates"`
}

// PageMeta is the list envelope meta.
type PageMeta struct {
	Page       int   `json:"page"`
	PerPage    int   `json:"per_page"`
	TotalItems int64 `json:"total_items"`
	TotalPages int   `json:"total_pages"`
}

// Page is a clamped page request (G15: bad input never errors).
type Page struct {
	Page    int
	PerPage int
}

// ParsePage clamps raw query values to valid bounds (spec §5, EC-18).
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

// Offset is the SQL offset for this page.
func (p Page) Offset() int { return (p.Page - 1) * p.PerPage }

// Meta builds list metadata for a total count.
func (p Page) Meta(total int64) PageMeta {
	pages := int((total + int64(p.PerPage) - 1) / int64(p.PerPage))
	return PageMeta{Page: p.Page, PerPage: p.PerPage, TotalItems: total, TotalPages: pages}
}

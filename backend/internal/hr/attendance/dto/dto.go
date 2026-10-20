// Package dto holds Module 3 request/response shapes (specification.md §5). Requests ≠ responses.
package dto

import (
	"strconv"
	"time"
)

// Pagination bounds (spec §5).
const (
	DefaultPage    = 1
	DefaultPerPage = 20
	MaxPerPage     = 100
)

// PunchRequest is A1. No time field exists: punch time is the server clock (AT-001).
type PunchRequest struct {
	Type      string   `json:"type" binding:"required,oneof=in out"`
	Latitude  *float64 `json:"latitude" binding:"omitempty,min=-90,max=90"`
	Longitude *float64 `json:"longitude" binding:"omitempty,min=-180,max=180"`
	DeviceID  *string  `json:"device_id" binding:"omitempty,max=100"`
	Source    string   `json:"source" binding:"omitempty,oneof=web mobile"`
}

// CreateShiftRequest is S1 (FR-SH001).
type CreateShiftRequest struct {
	Name              string  `json:"name" binding:"required,min=1,max=100"`
	Code              *string `json:"code" binding:"omitempty,min=2,max=32"`
	StartTime         string  `json:"start_time" binding:"required,len=5"`
	EndTime           string  `json:"end_time" binding:"required,len=5"`
	GracePeriodMins   *int    `json:"grace_period_mins" binding:"omitempty,min=0,max=240"`
	BreakDurationMins *int    `json:"break_duration_mins" binding:"omitempty,min=0,max=480"`
	FullDayMinutes    *int    `json:"full_day_minutes" binding:"omitempty,min=1,max=1440"`
	HalfDayMinutes    *int    `json:"half_day_minutes" binding:"omitempty,min=1,max=1440"`
	Timezone          string  `json:"timezone" binding:"omitempty,max=64"`
}

// UpdateShiftRequest is S4; code is immutable and therefore absent (FR-SH005).
type UpdateShiftRequest struct {
	Name              *string `json:"name" binding:"omitempty,min=1,max=100"`
	StartTime         *string `json:"start_time" binding:"omitempty,len=5"`
	EndTime           *string `json:"end_time" binding:"omitempty,len=5"`
	GracePeriodMins   *int    `json:"grace_period_mins" binding:"omitempty,min=0,max=240"`
	BreakDurationMins *int    `json:"break_duration_mins" binding:"omitempty,min=0,max=480"`
	FullDayMinutes    *int    `json:"full_day_minutes" binding:"omitempty,min=1,max=1440"`
	HalfDayMinutes    *int    `json:"half_day_minutes" binding:"omitempty,min=1,max=1440"`
	Timezone          *string `json:"timezone" binding:"omitempty,max=64"`
}

// AssignShiftRequest is S6 (FR-SA001). Dates are YYYY-MM-DD.
type AssignShiftRequest struct {
	EmployeeID    string  `json:"employee_id" binding:"required,uuid"`
	EffectiveFrom string  `json:"effective_from" binding:"required,len=10"`
	EffectiveTo   *string `json:"effective_to" binding:"omitempty,len=10"`
}

// CreateRegularizationRequest is A4 (FR-RG001).
type CreateRegularizationRequest struct {
	AttendanceDate    string    `json:"attendance_date" binding:"required,len=10"`
	RequestedPunchIn  time.Time `json:"requested_punch_in" binding:"required"`
	RequestedPunchOut time.Time `json:"requested_punch_out" binding:"required"`
	Reason            string    `json:"reason" binding:"required,min=1,max=500"`
}

// ReviewRequest is A11/A12; reject requires a comment (AT-016, enforced in service).
type ReviewRequest struct {
	Comment *string `json:"comment" binding:"omitempty,min=1,max=500"`
}

// ShiftResponse serializes a shift.
type ShiftResponse struct {
	ID                string    `json:"id"`
	Name              string    `json:"name"`
	Code              *string   `json:"code,omitempty"`
	StartTime         string    `json:"start_time"`
	EndTime           string    `json:"end_time"`
	GracePeriodMins   int       `json:"grace_period_mins"`
	BreakDurationMins int       `json:"break_duration_mins"`
	FullDayMinutes    *int      `json:"full_day_minutes,omitempty"`
	HalfDayMinutes    *int      `json:"half_day_minutes,omitempty"`
	Timezone          string    `json:"timezone"`
	IsNightShift      bool      `json:"is_night_shift"`
	ExpectedMinutes   int       `json:"expected_minutes"`
	Status            string    `json:"status"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// AssignmentResponse serializes a shift assignment.
type AssignmentResponse struct {
	ID            string    `json:"id"`
	EmployeeID    string    `json:"employee_id"`
	ShiftID       string    `json:"shift_id"`
	EffectiveFrom string    `json:"effective_from"`
	EffectiveTo   *string   `json:"effective_to,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

// PunchResponse serializes a punch; the IP address is never exposed (FR-EV003, security.md).
type PunchResponse struct {
	ID           string    `json:"id"`
	PunchType    string    `json:"punch_type"`
	PunchTime    time.Time `json:"punch_time"`
	Source       string    `json:"source"`
	Latitude     *float64  `json:"latitude,omitempty"`
	Longitude    *float64  `json:"longitude,omitempty"`
	DeviceID     *string   `json:"device_id,omitempty"`
	IsSuperseded bool      `json:"is_superseded"`
}

// RecordResponse serializes an attendance record.
type RecordResponse struct {
	ID                string     `json:"id"`
	EmployeeID        string     `json:"employee_id"`
	AttendanceDate    string     `json:"attendance_date"`
	ShiftID           *string    `json:"shift_id,omitempty"`
	FirstPunchIn      *time.Time `json:"first_punch_in,omitempty"`
	LastPunchOut      *time.Time `json:"last_punch_out,omitempty"`
	TotalWorkMinutes  int        `json:"total_work_minutes"`
	TotalBreakMinutes int        `json:"total_break_minutes"`
	LateMinutes       int        `json:"late_minutes"`
	OvertimeMinutes   int        `json:"overtime_minutes"`
	Status            string     `json:"status"`
	Source            string     `json:"source"`
	IsRegularized     bool       `json:"is_regularized"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

// PunchResult is A1's response.
type PunchResult struct {
	Punch  PunchResponse  `json:"punch"`
	Record RecordResponse `json:"record"`
}

// RecordDetailResponse is A8's response.
type RecordDetailResponse struct {
	Record  RecordResponse  `json:"record"`
	Punches []PunchResponse `json:"punches"`
}

// TodayResponse is A2's response.
type TodayResponse struct {
	AttendanceDate string          `json:"attendance_date"`
	Record         *RecordResponse `json:"record"`
	OpenSession    bool            `json:"open_session"`
	Shift          *ShiftResponse  `json:"shift"`
}

// RegularizationResponse serializes a regularization request.
type RegularizationResponse struct {
	ID                string     `json:"id"`
	EmployeeID        string     `json:"employee_id"`
	AttendanceDate    string     `json:"attendance_date"`
	RequestedPunchIn  time.Time  `json:"requested_punch_in"`
	RequestedPunchOut time.Time  `json:"requested_punch_out"`
	Reason            string     `json:"reason"`
	Status            string     `json:"status"`
	ReviewerUserID    *string    `json:"reviewer_user_id,omitempty"`
	ReviewedAt        *time.Time `json:"reviewed_at,omitempty"`
	ReviewComment     *string    `json:"review_comment,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
}

// SummaryResponse is A9's response (AT-020).
type SummaryResponse struct {
	Date           string `json:"date"`
	TotalEmployees int64  `json:"total_employees"`
	Present        int64  `json:"present"`
	Late           int64  `json:"late"`
	HalfDay        int64  `json:"half_day"`
	OnLeave        int64  `json:"on_leave"`
	Absent         int64  `json:"absent"`
}

// PageMeta is the list envelope meta.
type PageMeta struct {
	Page       int   `json:"page"`
	PerPage    int   `json:"per_page"`
	TotalItems int64 `json:"total_items"`
	TotalPages int   `json:"total_pages"`
}

// Page is a clamped page request (G13: bad input never errors).
type Page struct {
	Page    int
	PerPage int
}

// ParsePage clamps raw query values to valid bounds (spec §5, EC-16).
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

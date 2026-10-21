// Package dto holds HR/Core request and response shapes (specification.md §4).
package dto

import (
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/hr/core/models"
	"github.com/jaas/jaas/internal/hr/kernel"
)

// DateLayout is the API calendar-date format.
const DateLayout = "2006-01-02"

// --- job architecture (CO-001..CO-003) ---

// CreateFamilyRequest creates a job family.
type CreateFamilyRequest struct {
	Code        string  `json:"code" binding:"required"`
	Name        string  `json:"name" binding:"required"`
	Description *string `json:"description"`
}

// CreateGradeRequest creates a grade.
type CreateGradeRequest struct {
	Code string `json:"code" binding:"required"`
	Name string `json:"name" binding:"required"`
	Rank int    `json:"rank" binding:"required,min=1"`
}

// CreateProfileRequest creates a job profile.
type CreateProfileRequest struct {
	Code           string  `json:"code" binding:"required"`
	Title          string  `json:"title" binding:"required"`
	FamilyID       string  `json:"family_id" binding:"required"`
	DefaultGradeID *string `json:"default_grade_id"`
	Category       string  `json:"category" binding:"required,oneof=staff management executive"`
	ProbationDays  *int    `json:"probation_days" binding:"omitempty,min=0,max=365"`
	Description    *string `json:"description"`
}

// UpdateArchitectureRequest patches a family, grade or profile; fields that do not apply to the resource are ignored.
type UpdateArchitectureRequest struct {
	Name           *string `json:"name"`
	Title          *string `json:"title"`
	Description    *string `json:"description"`
	Rank           *int    `json:"rank" binding:"omitempty,min=1"`
	FamilyID       *string `json:"family_id"`
	DefaultGradeID *string `json:"default_grade_id"`
	Category       *string `json:"category" binding:"omitempty,oneof=staff management executive"`
	ProbationDays  *int    `json:"probation_days" binding:"omitempty,min=0,max=365"`
	Status         *string `json:"status" binding:"omitempty,oneof=active inactive"`
}

// FamilyResponse is a job family.
type FamilyResponse struct {
	ID          uuid.UUID `json:"id"`
	Code        string    `json:"code"`
	Name        string    `json:"name"`
	Description *string   `json:"description,omitempty"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// GradeResponse is a grade.
type GradeResponse struct {
	ID        uuid.UUID `json:"id"`
	Code      string    `json:"code"`
	Name      string    `json:"name"`
	Rank      int       `json:"rank"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ProfileResponse is a job profile.
type ProfileResponse struct {
	ID             uuid.UUID  `json:"id"`
	Code           string     `json:"code"`
	Title          string     `json:"title"`
	FamilyID       uuid.UUID  `json:"family_id"`
	DefaultGradeID *uuid.UUID `json:"default_grade_id,omitempty"`
	Category       string     `json:"category"`
	ProbationDays  int        `json:"probation_days"`
	Description    *string    `json:"description,omitempty"`
	Status         string     `json:"status"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// --- positions (CO-010..CO-014) ---

// CreatePositionRequest creates a position.
type CreatePositionRequest struct {
	Code         string        `json:"code" binding:"required"`
	Title        string        `json:"title" binding:"required"`
	JobProfileID string        `json:"job_profile_id" binding:"required"`
	DepartmentID string        `json:"department_id" binding:"required"`
	LocationID   *string       `json:"location_id"`
	BudgetedFTE  *kernel.Fixed `json:"budgeted_fte"`
	Shared       bool          `json:"shared"`
}

// UpdatePositionRequest patches a position.
type UpdatePositionRequest struct {
	Title        *string       `json:"title"`
	JobProfileID *string       `json:"job_profile_id"`
	DepartmentID *string       `json:"department_id"`
	LocationID   *string       `json:"location_id"`
	BudgetedFTE  *kernel.Fixed `json:"budgeted_fte"`
	Shared       *bool         `json:"shared"`
}

// PositionStatusRequest moves a position (CO-013).
type PositionStatusRequest struct {
	Status string `json:"status" binding:"required,oneof=active frozen closed"`
}

// PositionResponse is a position with its occupancy as of a date (CO-011).
type PositionResponse struct {
	ID           uuid.UUID    `json:"id"`
	Code         string       `json:"code"`
	Title        string       `json:"title"`
	JobProfileID uuid.UUID    `json:"job_profile_id"`
	DepartmentID uuid.UUID    `json:"department_id"`
	LocationID   *uuid.UUID   `json:"location_id,omitempty"`
	BudgetedFTE  kernel.Fixed `json:"budgeted_fte"`
	Shared       bool         `json:"shared"`
	Status       string       `json:"status"`
	AsOf         string       `json:"as_of"`
	FilledFTE    kernel.Fixed `json:"filled_fte"`
	OpenFTE      kernel.Fixed `json:"open_fte"`
	Vacant       bool         `json:"vacant"`
	Incumbents   []uuid.UUID  `json:"incumbents"`
	CreatedAt    time.Time    `json:"created_at"`
	UpdatedAt    time.Time    `json:"updated_at"`
}

// --- actions (CO-030..CO-039) ---

// CreateActionRequest creates a draft action.
type CreateActionRequest struct {
	Type          string         `json:"type" binding:"required"`
	EmployeeID    string         `json:"employee_id" binding:"required"`
	EffectiveDate string         `json:"effective_date" binding:"required"`
	ReasonCode    string         `json:"reason_code" binding:"required"`
	Comment       *string        `json:"comment"`
	Changes       models.Changes `json:"changes"`
}

// UpdateActionRequest edits a draft.
type UpdateActionRequest struct {
	EffectiveDate *string         `json:"effective_date"`
	ReasonCode    *string         `json:"reason_code"`
	Comment       *string         `json:"comment"`
	Changes       *models.Changes `json:"changes"`
}

// DecisionRequest carries a decision comment (reject) or a reason (rescind).
type DecisionRequest struct {
	Comment *string `json:"comment"`
	Reason  *string `json:"reason"`
}

// ActionResponse is an HR action.
type ActionResponse struct {
	ID                uuid.UUID      `json:"id"`
	Type              string         `json:"type"`
	EmployeeID        uuid.UUID      `json:"employee_id"`
	EffectiveDate     string         `json:"effective_date"`
	Status            string         `json:"status"`
	ReasonCode        string         `json:"reason_code"`
	Comment           *string        `json:"comment,omitempty"`
	Changes           models.Changes `json:"changes"`
	InitiatedByUserID uuid.UUID      `json:"initiated_by_user_id"`
	DecidedByUserID   *uuid.UUID     `json:"decided_by_user_id,omitempty"`
	DecisionComment   *string        `json:"decision_comment,omitempty"`
	SubmittedAt       *time.Time     `json:"submitted_at,omitempty"`
	CompletedAt       *time.Time     `json:"completed_at,omitempty"`
	RescindedAt       *time.Time     `json:"rescinded_at,omitempty"`
	RescindReason     *string        `json:"rescind_reason,omitempty"`
	NewAssignmentID   *uuid.UUID     `json:"new_assignment_id,omitempty"`
	EffectsApplied    bool           `json:"effects_applied"`
	CreatedAt         time.Time      `json:"created_at"`
}

// --- workers and assignments (CO-020..CO-022) ---

// AssignmentResponse is one effective-dated assignment.
type AssignmentResponse struct {
	ID                uuid.UUID    `json:"id"`
	EmployeeID        uuid.UUID    `json:"employee_id"`
	ActionID          uuid.UUID    `json:"action_id"`
	JobProfileID      *uuid.UUID   `json:"job_profile_id,omitempty"`
	PositionID        *uuid.UUID   `json:"position_id,omitempty"`
	DepartmentID      *uuid.UUID   `json:"department_id,omitempty"`
	LocationID        *uuid.UUID   `json:"location_id,omitempty"`
	ManagerEmployeeID *uuid.UUID   `json:"manager_employee_id,omitempty"`
	GradeID           *uuid.UUID   `json:"grade_id,omitempty"`
	WorkerType        string       `json:"worker_type"`
	EmploymentType    string       `json:"employment_type"`
	FTE               kernel.Fixed `json:"fte"`
	ProbationEndDate  *string      `json:"probation_end_date,omitempty"`
	EffectiveFrom     string       `json:"effective_from"`
	EffectiveTo       *string      `json:"effective_to,omitempty"`
	Status            string       `json:"status"`
}

// WorkerResponse is CO-021: the assignment in effect on as_of, the full history and the action log.
type WorkerResponse struct {
	EmployeeID uuid.UUID            `json:"employee_id"`
	AsOf       string               `json:"as_of"`
	Current    *AssignmentResponse  `json:"current"`
	History    []AssignmentResponse `json:"history"`
	Actions    []ActionResponse     `json:"actions"`
}

// --- headcount (CO-050, CO-051) ---

// CreatePlanRequest creates a headcount plan.
type CreatePlanRequest struct {
	DepartmentID string       `json:"department_id" binding:"required"`
	PeriodStart  string       `json:"period_start" binding:"required"`
	PeriodEnd    string       `json:"period_end" binding:"required"`
	PlannedFTE   kernel.Fixed `json:"planned_fte"`
}

// PlanResponse is a headcount plan.
type PlanResponse struct {
	ID           uuid.UUID    `json:"id"`
	DepartmentID uuid.UUID    `json:"department_id"`
	PeriodStart  string       `json:"period_start"`
	PeriodEnd    string       `json:"period_end"`
	PlannedFTE   kernel.Fixed `json:"planned_fte"`
	CreatedAt    time.Time    `json:"created_at"`
}

// HeadcountRow is one department's headcount as of a date (CO-051).
type HeadcountRow struct {
	DepartmentID  uuid.UUID     `json:"department_id"`
	PlannedFTE    *kernel.Fixed `json:"planned_fte,omitempty"`
	BudgetedFTE   kernel.Fixed  `json:"budgeted_fte"`
	FilledFTE     kernel.Fixed  `json:"filled_fte"`
	OpenPositions int           `json:"open_positions"`
	Workers       int           `json:"workers"`
}

// HeadcountResponse is CO-051.
type HeadcountResponse struct {
	AsOf        string         `json:"as_of"`
	Departments []HeadcountRow `json:"departments"`
}

// RunResponse reports how many items an operation processed (back-fill, effects sweep).
type RunResponse struct {
	Processed int `json:"processed"`
	Skipped   int `json:"skipped"`
}

// --- paging ---

// PageMeta is list metadata.
type PageMeta struct {
	Page       int   `json:"page"`
	PerPage    int   `json:"per_page"`
	TotalItems int64 `json:"total_items"`
	TotalPages int   `json:"total_pages"`
}

// Page is a clamped page request.
type Page struct{ Page, PerPage int }

// ParsePage clamps raw query values (page ≥ 1 default 1, per_page 1–100 default 20).
func ParsePage(pageStr, perPageStr string) Page {
	page, err := strconv.Atoi(pageStr)
	if err != nil || page < 1 {
		page = 1
	}
	perPage, err := strconv.Atoi(perPageStr)
	if err != nil || perPage < 1 {
		perPage = 20
	}
	if perPage > 100 {
		perPage = 100
	}
	return Page{Page: page, PerPage: perPage}
}

// Offset is the SQL offset.
func (p Page) Offset() int { return (p.Page - 1) * p.PerPage }

// Meta builds list metadata.
func (p Page) Meta(total int64) PageMeta {
	return PageMeta{Page: p.Page, PerPage: p.PerPage, TotalItems: total, TotalPages: int((total + int64(p.PerPage) - 1) / int64(p.PerPage))}
}

// Package dto holds Module 6 request/response shapes (specification.md §5); never GORM models.
package dto

import (
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/payroll/calc"
)

// CreateJobRequest is R1.
type CreateJobRequest struct {
	Title              string  `json:"title" binding:"required,min=1,max=150"`
	DepartmentID       *string `json:"department_id" binding:"omitempty,uuid"`
	DesignationID      *string `json:"designation_id" binding:"omitempty,uuid"`
	Headcount          int     `json:"headcount" binding:"required,min=1,max=1000"`
	Location           *string `json:"location" binding:"omitempty,max=100"`
	EmploymentType     string  `json:"employment_type" binding:"required,oneof=full_time part_time contract intern"`
	MinExperienceYears *int    `json:"min_experience_years" binding:"omitempty,min=0,max=50"`
	Description        string  `json:"description" binding:"required,min=1,max=10000"`
}

// UpdateJobRequest is R4 (pointer fields).
type UpdateJobRequest struct {
	Title              *string `json:"title" binding:"omitempty,min=1,max=150"`
	DepartmentID       *string `json:"department_id" binding:"omitempty,uuid"`
	DesignationID      *string `json:"designation_id" binding:"omitempty,uuid"`
	Headcount          *int    `json:"headcount" binding:"omitempty,min=1,max=1000"`
	Location           *string `json:"location" binding:"omitempty,max=100"`
	EmploymentType     *string `json:"employment_type" binding:"omitempty,oneof=full_time part_time contract intern"`
	MinExperienceYears *int    `json:"min_experience_years" binding:"omitempty,min=0,max=50"`
	Description        *string `json:"description" binding:"omitempty,min=1,max=10000"`
}

// JobStatusRequest is R5.
type JobStatusRequest struct {
	Status string `json:"status" binding:"required,oneof=open on_hold closed"`
}

// CreateCandidateRequest is R6.
type CreateCandidateRequest struct {
	JobID            string      `json:"job_id" binding:"required,uuid"`
	FirstName        string      `json:"first_name" binding:"required,min=1,max=100"`
	LastName         string      `json:"last_name" binding:"required,min=1,max=100"`
	Email            string      `json:"email" binding:"required,email,max=255"`
	Phone            *string     `json:"phone" binding:"omitempty,max=30"`
	Source           string      `json:"source" binding:"required,oneof=career_site referral agency linkedin other"`
	ResumeURL        *string     `json:"resume_url" binding:"omitempty,url,max=500"`
	ExpectedCTC      *calc.Money `json:"expected_ctc"`
	NoticePeriodDays *int        `json:"notice_period_days" binding:"omitempty,min=0,max=365"`
}

// UpdateCandidateRequest is R9 (contact fields only; stage changes go through R10/R11/R17).
type UpdateCandidateRequest struct {
	FirstName        *string     `json:"first_name" binding:"omitempty,min=1,max=100"`
	LastName         *string     `json:"last_name" binding:"omitempty,min=1,max=100"`
	Email            *string     `json:"email" binding:"omitempty,email,max=255"`
	Phone            *string     `json:"phone" binding:"omitempty,max=30"`
	ResumeURL        *string     `json:"resume_url" binding:"omitempty,url,max=500"`
	ExpectedCTC      *calc.Money `json:"expected_ctc"`
	NoticePeriodDays *int        `json:"notice_period_days" binding:"omitempty,min=0,max=365"`
}

// StageRequest is R10.
type StageRequest struct {
	Stage string  `json:"stage" binding:"required,oneof=screening interview rejected withdrawn"`
	Note  *string `json:"note" binding:"omitempty,max=500"`
}

// HireRequest is R11.
type HireRequest struct {
	EmployeeCode   string `json:"employee_code" binding:"required,min=2,max=32"`
	EmploymentType string `json:"employment_type" binding:"omitempty,oneof=full_time part_time contract intern"`
}

// ScheduleInterviewRequest is R12.
type ScheduleInterviewRequest struct {
	CandidateID           string    `json:"candidate_id" binding:"required,uuid"`
	RoundName             string    `json:"round_name" binding:"required,min=1,max=100"`
	InterviewerEmployeeID string    `json:"interviewer_employee_id" binding:"required,uuid"`
	ScheduledAt           time.Time `json:"scheduled_at" binding:"required"`
	DurationMins          int       `json:"duration_mins" binding:"required,min=15,max=480"`
	MeetingLink           *string   `json:"meeting_link" binding:"omitempty,url,max=500"`
}

// FeedbackRequest is R15.
type FeedbackRequest struct {
	Rating         int     `json:"rating" binding:"required,min=1,max=5"`
	Recommendation string  `json:"recommendation" binding:"required,oneof=strong_hire hire hold reject"`
	Notes          *string `json:"notes" binding:"omitempty,max=5000"`
}

// CreateOfferRequest is R17.
type CreateOfferRequest struct {
	CandidateID string     `json:"candidate_id" binding:"required,uuid"`
	OfferedCTC  calc.Money `json:"offered_ctc" binding:"required"`
	JoiningDate string     `json:"joining_date" binding:"required"`
	ExpiresOn   *string    `json:"expires_on"`
}

// OfferDecisionRequest is R19.
type OfferDecisionRequest struct {
	Decision string `json:"decision" binding:"required,oneof=accepted declined"`
}

// JobResponse serializes a job.
type JobResponse struct {
	ID                 uuid.UUID  `json:"id"`
	Title              string     `json:"title"`
	DepartmentID       *uuid.UUID `json:"department_id,omitempty"`
	DesignationID      *uuid.UUID `json:"designation_id,omitempty"`
	Headcount          int        `json:"headcount"`
	HiredCount         int        `json:"hired_count"`
	Location           *string    `json:"location,omitempty"`
	EmploymentType     string     `json:"employment_type"`
	MinExperienceYears int        `json:"min_experience_years"`
	Description        string     `json:"description"`
	Status             string     `json:"status"`
	OpenedAt           *time.Time `json:"opened_at,omitempty"`
	ClosedAt           *time.Time `json:"closed_at,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

// StageEventResponse serializes one pipeline change.
type StageEventResponse struct {
	FromStage   *string   `json:"from_stage,omitempty"`
	ToStage     string    `json:"to_stage"`
	ActorUserID uuid.UUID `json:"actor_user_id"`
	Note        *string   `json:"note,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// CandidateResponse serializes a candidate; History is set on R8 only.
type CandidateResponse struct {
	ID               uuid.UUID            `json:"id"`
	JobID            uuid.UUID            `json:"job_id"`
	FirstName        string               `json:"first_name"`
	LastName         string               `json:"last_name"`
	Email            string               `json:"email"`
	Phone            *string              `json:"phone,omitempty"`
	Source           string               `json:"source"`
	ResumeURL        *string              `json:"resume_url,omitempty"`
	ExpectedCTC      *calc.Money          `json:"expected_ctc,omitempty"`
	NoticePeriodDays *int                 `json:"notice_period_days,omitempty"`
	Stage            string               `json:"stage"`
	HiredEmployeeID  *uuid.UUID           `json:"hired_employee_id,omitempty"`
	History          []StageEventResponse `json:"history,omitempty"`
	CreatedAt        time.Time            `json:"created_at"`
	UpdatedAt        time.Time            `json:"updated_at"`
}

// InterviewResponse serializes an interview.
type InterviewResponse struct {
	ID                    uuid.UUID  `json:"id"`
	CandidateID           uuid.UUID  `json:"candidate_id"`
	RoundName             string     `json:"round_name"`
	InterviewerEmployeeID uuid.UUID  `json:"interviewer_employee_id"`
	ScheduledAt           time.Time  `json:"scheduled_at"`
	DurationMins          int        `json:"duration_mins"`
	MeetingLink           *string    `json:"meeting_link,omitempty"`
	Status                string     `json:"status"`
	Rating                *int       `json:"rating,omitempty"`
	Recommendation        *string    `json:"recommendation,omitempty"`
	Feedback              *string    `json:"feedback,omitempty"`
	FeedbackAt            *time.Time `json:"feedback_at,omitempty"`
	CreatedAt             time.Time  `json:"created_at"`
}

// OfferResponse serializes an offer.
type OfferResponse struct {
	ID          uuid.UUID  `json:"id"`
	CandidateID uuid.UUID  `json:"candidate_id"`
	OfferedCTC  calc.Money `json:"offered_ctc"`
	JoiningDate string     `json:"joining_date"`
	ExpiresOn   *string    `json:"expires_on,omitempty"`
	Status      string     `json:"status"`
	DecidedAt   *time.Time `json:"decided_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

// HireResponse is R11.
type HireResponse struct {
	CandidateID uuid.UUID `json:"candidate_id"`
	UserID      uuid.UUID `json:"user_id"`
	EmployeeID  uuid.UUID `json:"employee_id"`
	JobStatus   string    `json:"job_status"`
}

// PageMeta is the list envelope meta.
type PageMeta struct {
	Page       int   `json:"page"`
	PerPage    int   `json:"per_page"`
	TotalItems int64 `json:"total_items"`
	TotalPages int   `json:"total_pages"`
}

// Page is a clamped page request (G14).
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

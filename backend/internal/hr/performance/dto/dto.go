// Package dto holds HR/Performance request and response shapes (specification.md §5).
package dto

import (
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/hr/performance/models"
	"github.com/jaas/jaas/internal/hr/performance/score"
)

// CreateCycleRequest is C1 (FR-CY001); criteria default when omitted.
type CreateCycleRequest struct {
	Title     string   `json:"title" binding:"required"`
	StartDate string   `json:"start_date" binding:"required"`
	EndDate   string   `json:"end_date" binding:"required"`
	Criteria  []string `json:"criteria"`
}

// UpdateCycleRequest is C4 (FR-CY002, draft only).
type UpdateCycleRequest struct {
	Title     *string   `json:"title"`
	StartDate *string   `json:"start_date"`
	EndDate   *string   `json:"end_date"`
	Criteria  *[]string `json:"criteria"`
}

// CycleResponse is a cycle.
type CycleResponse struct {
	ID              uuid.UUID  `json:"id"`
	Title           string     `json:"title"`
	StartDate       string     `json:"start_date"`
	EndDate         string     `json:"end_date"`
	Criteria        []string   `json:"criteria"`
	Status          string     `json:"status"`
	ActivatedAt     *time.Time `json:"activated_at,omitempty"`
	ReviewStartedAt *time.Time `json:"review_started_at,omitempty"`
	CompletedAt     *time.Time `json:"completed_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// LaunchResponse is C6 (PM-010): the cycle plus how many reviews were created.
type LaunchResponse struct {
	Cycle          CycleResponse `json:"cycle"`
	Employees      int           `json:"employees"`
	SelfReviews    int           `json:"self_reviews_created"`
	ManagerReviews int           `json:"manager_reviews_created"`
	WithoutManager int           `json:"without_manager"`
}

// NineBoxEntry is one employee on the 9-box (PM-015); Box is "<performance>/<potential>" or "unrated".
type NineBoxEntry struct {
	EmployeeID   uuid.UUID    `json:"employee_id"`
	GoalScore    score.Fixed  `json:"goal_score"`
	OverallScore *score.Fixed `json:"overall_score,omitempty"`
	Performance  string       `json:"performance,omitempty"`
	Potential    string       `json:"potential,omitempty"`
	Box          string       `json:"box"`
}

// NineBoxResponse is C8.
type NineBoxResponse struct {
	CycleID uuid.UUID      `json:"cycle_id"`
	Entries []NineBoxEntry `json:"entries"`
	Counts  map[string]int `json:"counts"`
}

// CreateGoalRequest is G1 (FR-GL001); employee_id defaults to the caller's profile.
type CreateGoalRequest struct {
	CycleID      string      `json:"cycle_id" binding:"required"`
	EmployeeID   *string     `json:"employee_id"`
	Title        string      `json:"title" binding:"required"`
	Description  *string     `json:"description"`
	Weight       score.Fixed `json:"weight"`
	ParentGoalID *string     `json:"parent_goal_id"`
}

// UpdateGoalRequest is G5 (FR-GL003).
type UpdateGoalRequest struct {
	Title       *string      `json:"title"`
	Description *string      `json:"description"`
	Weight      *score.Fixed `json:"weight"`
}

// GoalStatusRequest is G6 (FR-GL004).
type GoalStatusRequest struct {
	Status string `json:"status" binding:"required,oneof=active completed cancelled"`
}

// KeyResultRequest is G7 (FR-GL005).
type KeyResultRequest struct {
	Title       string      `json:"title" binding:"required"`
	Unit        *string     `json:"unit"`
	StartValue  score.Fixed `json:"start_value"`
	TargetValue score.Fixed `json:"target_value"`
}

// CheckInRequest is G8 (FR-GL006): progress for a goal without key results, key_result_id + value otherwise.
type CheckInRequest struct {
	Progress    *score.Fixed `json:"progress"`
	KeyResultID *string      `json:"key_result_id"`
	Value       *score.Fixed `json:"value"`
	Note        *string      `json:"note"`
}

// KeyResultResponse is a key result with its derived progress (PM-006).
type KeyResultResponse struct {
	ID           uuid.UUID   `json:"id"`
	Title        string      `json:"title"`
	Unit         *string     `json:"unit,omitempty"`
	StartValue   score.Fixed `json:"start_value"`
	TargetValue  score.Fixed `json:"target_value"`
	CurrentValue score.Fixed `json:"current_value"`
	Progress     score.Fixed `json:"progress"`
}

// CheckInResponse is one progress update (PM-007).
type CheckInResponse struct {
	ID           uuid.UUID    `json:"id"`
	KeyResultID  *uuid.UUID   `json:"key_result_id,omitempty"`
	FromProgress score.Fixed  `json:"from_progress"`
	ToProgress   score.Fixed  `json:"to_progress"`
	Value        *score.Fixed `json:"value,omitempty"`
	Note         *string      `json:"note,omitempty"`
	AuthorUserID uuid.UUID    `json:"author_user_id"`
	CreatedAt    time.Time    `json:"created_at"`
}

// GoalResponse is a goal; detail reads add key results and the newest check-ins.
type GoalResponse struct {
	ID           uuid.UUID           `json:"id"`
	CycleID      uuid.UUID           `json:"cycle_id"`
	EmployeeID   uuid.UUID           `json:"employee_id"`
	ParentGoalID *uuid.UUID          `json:"parent_goal_id,omitempty"`
	Title        string              `json:"title"`
	Description  *string             `json:"description,omitempty"`
	Weight       score.Fixed         `json:"weight"`
	Progress     score.Fixed         `json:"progress"`
	Status       string              `json:"status"`
	CreatedAt    time.Time           `json:"created_at"`
	UpdatedAt    time.Time           `json:"updated_at"`
	KeyResults   []KeyResultResponse `json:"key_results,omitempty"`
	CheckIns     []CheckInResponse   `json:"check_ins,omitempty"`
}

// AssignReviewerRequest is V1 (FR-RV001).
type AssignReviewerRequest struct {
	CycleID            string `json:"cycle_id" binding:"required"`
	EmployeeID         string `json:"employee_id" binding:"required"`
	ReviewerEmployeeID string `json:"reviewer_employee_id" binding:"required"`
	Type               string `json:"type" binding:"required,oneof=peer manager"`
}

// SubmitReviewRequest is V6 (FR-RV003).
type SubmitReviewRequest struct {
	Ratings      models.Ratings `json:"ratings" binding:"required"`
	Strengths    *string        `json:"strengths"`
	Improvements *string        `json:"improvements"`
	Potential    *string        `json:"potential"`
}

// ReviewResponse is a review; reviewer_employee_id is omitted for peer reviews shown to the subject (PM-014).
type ReviewResponse struct {
	ID                 uuid.UUID      `json:"id"`
	CycleID            uuid.UUID      `json:"cycle_id"`
	EmployeeID         uuid.UUID      `json:"employee_id"`
	ReviewerEmployeeID *uuid.UUID     `json:"reviewer_employee_id,omitempty"`
	Type               string         `json:"type"`
	Status             string         `json:"status"`
	Ratings            models.Ratings `json:"ratings"`
	Strengths          *string        `json:"strengths,omitempty"`
	Improvements       *string        `json:"improvements,omitempty"`
	OverallScore       *score.Fixed   `json:"overall_score,omitempty"`
	Potential          *string        `json:"potential,omitempty"`
	SubmittedAt        *time.Time     `json:"submitted_at,omitempty"`
	CreatedAt          time.Time      `json:"created_at"`
}

// PageMeta is list metadata.
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

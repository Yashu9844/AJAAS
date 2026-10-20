// Package repositories is HR/Performance data access: tenant_id on every scoped query, `db`/`tx` passed per call.
package repositories

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/hr/performance/dto"
	"github.com/jaas/jaas/internal/hr/performance/models"
	"github.com/jaas/jaas/internal/hr/performance/score"
	"gorm.io/gorm"
)

// CycleRepository persists appraisal cycles (FR-CY).
type CycleRepository interface {
	Create(ctx context.Context, tx *gorm.DB, c *models.Cycle) error
	Update(ctx context.Context, tx *gorm.DB, c *models.Cycle) error
	FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Cycle, error)
	LockByID(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) (*models.Cycle, error)
	List(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, status string, page dto.Page) ([]models.Cycle, int64, error)
}

// GoalFilter narrows goal lists (G2); EmployeeID nil means any owner.
type GoalFilter struct {
	CycleID    *uuid.UUID
	EmployeeID *uuid.UUID
	Status     string
}

// GoalRepository persists goals, their key results and append-only check-ins (FR-GL, PM-007).
type GoalRepository interface {
	Create(ctx context.Context, tx *gorm.DB, g *models.Goal) error
	Update(ctx context.Context, tx *gorm.DB, g *models.Goal) error
	FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Goal, error)
	LockByID(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) (*models.Goal, error)
	List(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, f GoalFilter, page dto.Page) ([]models.Goal, int64, error)
	// LockOwner takes the transaction-scoped advisory lock for (cycle, owner) that serializes the weight cap (PM-004).
	LockOwner(ctx context.Context, tx *gorm.DB, cycleID, employeeID uuid.UUID) error
	// WeightSum sums the owner's non-cancelled goal weights in the cycle, excluding one goal (uuid.Nil: none).
	WeightSum(ctx context.Context, tx *gorm.DB, tenantID, cycleID, employeeID, exclude uuid.UUID) (score.Fixed, error)
	// Open lists the cycle's non-cancelled goals (launch reviews, 9-box goal scores).
	Open(ctx context.Context, db *gorm.DB, tenantID, cycleID uuid.UUID) ([]models.Goal, error)

	AddKeyResult(ctx context.Context, tx *gorm.DB, k *models.KeyResult) error
	UpdateKeyResult(ctx context.Context, tx *gorm.DB, k *models.KeyResult) error
	KeyResults(ctx context.Context, db *gorm.DB, tenantID, goalID uuid.UUID) ([]models.KeyResult, error)
	AddCheckIn(ctx context.Context, tx *gorm.DB, c *models.CheckIn) error
	// CheckIns returns the goal's newest check-ins first, at most limit.
	CheckIns(ctx context.Context, db *gorm.DB, tenantID, goalID uuid.UUID, limit int) ([]models.CheckIn, error)
}

// ReviewFilter narrows review lists (V2–V4).
type ReviewFilter struct {
	CycleID    *uuid.UUID
	EmployeeID *uuid.UUID
	ReviewerID *uuid.UUID
	Status     string
	Type       string
}

// ReviewRepository persists reviews (FR-RV).
type ReviewRepository interface {
	Create(ctx context.Context, tx *gorm.DB, r *models.Review) error
	Update(ctx context.Context, tx *gorm.DB, r *models.Review) error
	FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Review, error)
	LockByID(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) (*models.Review, error)
	// Exists reports whether the (cycle, employee, reviewer, type) assignment exists (PM-010, PM-011).
	Exists(ctx context.Context, tx *gorm.DB, tenantID, cycleID, employeeID, reviewerID uuid.UUID, typ string) (bool, error)
	List(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, f ReviewFilter, page dto.Page) ([]models.Review, int64, error)
	// ExpirePending marks the cycle's pending reviews expired (PM-013).
	ExpirePending(ctx context.Context, tx *gorm.DB, tenantID, cycleID uuid.UUID, at time.Time) (int64, error)
}

// CalibrationRepository persists append-only calibrations (PM-019).
type CalibrationRepository interface {
	Create(ctx context.Context, tx *gorm.DB, c *models.Calibration) error
	// Latest returns the employee's newest calibration in the cycle, or nil.
	Latest(ctx context.Context, db *gorm.DB, tenantID, cycleID, employeeID uuid.UUID) (*models.Calibration, error)
	// LatestPerEmployee returns each employee's newest calibration in the cycle.
	LatestPerEmployee(ctx context.Context, db *gorm.DB, tenantID, cycleID uuid.UUID) ([]models.Calibration, error)
	List(ctx context.Context, db *gorm.DB, tenantID, cycleID uuid.UUID, employeeID *uuid.UUID, page dto.Page) ([]models.Calibration, int64, error)
}

// FeedbackFilter narrows feedback lists (F2–F5).
type FeedbackFilter struct {
	ToID, FromID *uuid.UUID
	PublicOnly   bool
}

// RequestFilter narrows feedback-request lists (F7, F8).
type RequestFilter struct {
	ResponderID, RequesterID *uuid.UUID
	Status                   string
}

// FeedbackRepository persists continuous feedback and feedback requests (FB-001..FB-005).
type FeedbackRepository interface {
	Create(ctx context.Context, tx *gorm.DB, f *models.Feedback) error
	List(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, f FeedbackFilter, page dto.Page) ([]models.Feedback, int64, error)
	CreateRequest(ctx context.Context, tx *gorm.DB, r *models.FeedbackRequest) error
	UpdateRequest(ctx context.Context, tx *gorm.DB, r *models.FeedbackRequest) error
	LockRequest(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) (*models.FeedbackRequest, error)
	// PendingExists reports a pending request for (requester, responder, about) (FB-004).
	PendingExists(ctx context.Context, tx *gorm.DB, tenantID, requester, responder, about uuid.UUID) (bool, error)
	ListRequests(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, f RequestFilter, page dto.Page) ([]models.FeedbackRequest, int64, error)
}

// OutboxRepository persists outbox rows; relay methods are cross-tenant by design.
type OutboxRepository interface {
	Create(ctx context.Context, tx *gorm.DB, e *models.OutboxEvent) error
	MarkPublished(ctx context.Context, db *gorm.DB, id uuid.UUID, at time.Time) error
	RecordFailure(ctx context.Context, db *gorm.DB, id uuid.UUID, reason string) error
	FetchUnpublished(ctx context.Context, db *gorm.DB, limit, maxAttempts int) ([]models.OutboxEvent, error)
}

func first[T any](q *gorm.DB) (*T, error) {
	var out T
	err := q.Take(&out).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func paged[T any](q *gorm.DB, order string, page dto.Page) ([]T, int64, error) {
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var out []T
	err := q.Order(order).Offset(page.Offset()).Limit(page.PerPage).Find(&out).Error
	return out, total, err
}

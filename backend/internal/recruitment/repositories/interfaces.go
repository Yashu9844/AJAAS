// Package repositories is Module 6 data access: tenant_id on every scoped query, `db`/`tx` passed per call.
package repositories

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/recruitment/dto"
	"github.com/jaas/jaas/internal/recruitment/models"
	"gorm.io/gorm"
)

// JobFilter narrows job lists (R2).
type JobFilter struct {
	Status       string
	DepartmentID *uuid.UUID
}

// JobRepository persists job openings (FR-JB).
type JobRepository interface {
	Create(ctx context.Context, tx *gorm.DB, j *models.Job) error
	Update(ctx context.Context, tx *gorm.DB, j *models.Job) error
	FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Job, error)
	LockByID(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) (*models.Job, error)
	List(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, f JobFilter, page dto.Page) ([]models.Job, int64, error)
}

// CandidateFilter narrows candidate lists (R7).
type CandidateFilter struct {
	JobID *uuid.UUID
	Stage string
}

// CandidateRepository persists candidates and their append-only stage history (FR-CD, RC-003).
type CandidateRepository interface {
	Create(ctx context.Context, tx *gorm.DB, c *models.Candidate) error
	Update(ctx context.Context, tx *gorm.DB, c *models.Candidate) error
	FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Candidate, error)
	LockByID(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) (*models.Candidate, error)
	FindByJobEmail(ctx context.Context, db *gorm.DB, jobID uuid.UUID, email string) (*models.Candidate, error)
	List(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, f CandidateFilter, page dto.Page) ([]models.Candidate, int64, error)
	AddEvent(ctx context.Context, tx *gorm.DB, e *models.StageEvent) error
	Events(ctx context.Context, db *gorm.DB, tenantID, candidateID uuid.UUID) ([]models.StageEvent, error)
}

// InterviewRepository persists interviews (FR-IV).
type InterviewRepository interface {
	Create(ctx context.Context, tx *gorm.DB, i *models.Interview) error
	Update(ctx context.Context, tx *gorm.DB, i *models.Interview) error
	FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Interview, error)
	ListByCandidate(ctx context.Context, db *gorm.DB, tenantID, candidateID uuid.UUID, page dto.Page) ([]models.Interview, int64, error)
	ListByInterviewer(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID, status string, page dto.Page) ([]models.Interview, int64, error)
}

// OfferRepository persists offers (FR-OF).
type OfferRepository interface {
	Create(ctx context.Context, tx *gorm.DB, o *models.Offer) error
	Update(ctx context.Context, tx *gorm.DB, o *models.Offer) error
	FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Offer, error)
	// LatestByStatus returns the candidate's most recent offer in a status, or nil.
	LatestByStatus(ctx context.Context, db *gorm.DB, tenantID, candidateID uuid.UUID, status string) (*models.Offer, error)
	ListByCandidate(ctx context.Context, db *gorm.DB, tenantID, candidateID uuid.UUID, page dto.Page) ([]models.Offer, int64, error)
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

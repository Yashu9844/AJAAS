package repositories

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/recruitment/dto"
	"github.com/jaas/jaas/internal/recruitment/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type jobRepository struct{}

// NewJobRepository builds a JobRepository.
func NewJobRepository() JobRepository { return &jobRepository{} }

func (r *jobRepository) Create(ctx context.Context, tx *gorm.DB, j *models.Job) error {
	return tx.WithContext(ctx).Create(j).Error
}
func (r *jobRepository) Update(ctx context.Context, tx *gorm.DB, j *models.Job) error {
	return tx.WithContext(ctx).Where("tenant_id = ?", j.TenantID).Save(j).Error
}
func (r *jobRepository) FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Job, error) {
	return first[models.Job](db.WithContext(ctx).Where("tenant_id = ? AND id = ?", tenantID, id))
}
func (r *jobRepository) LockByID(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) (*models.Job, error) {
	return first[models.Job](tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_id = ? AND id = ?", tenantID, id))
}
func (r *jobRepository) List(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, f JobFilter, page dto.Page) ([]models.Job, int64, error) {
	q := db.WithContext(ctx).Model(&models.Job{}).Where("tenant_id = ?", tenantID)
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	if f.DepartmentID != nil {
		q = q.Where("department_id = ?", *f.DepartmentID)
	}
	return paged[models.Job](q, "created_at DESC", page)
}

type candidateRepository struct{}

// NewCandidateRepository builds a CandidateRepository.
func NewCandidateRepository() CandidateRepository { return &candidateRepository{} }

func (r *candidateRepository) Create(ctx context.Context, tx *gorm.DB, c *models.Candidate) error {
	return tx.WithContext(ctx).Create(c).Error
}
func (r *candidateRepository) Update(ctx context.Context, tx *gorm.DB, c *models.Candidate) error {
	return tx.WithContext(ctx).Where("tenant_id = ?", c.TenantID).Save(c).Error
}
func (r *candidateRepository) FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Candidate, error) {
	return first[models.Candidate](db.WithContext(ctx).Where("tenant_id = ? AND id = ?", tenantID, id))
}
func (r *candidateRepository) LockByID(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) (*models.Candidate, error) {
	return first[models.Candidate](tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_id = ? AND id = ?", tenantID, id))
}
func (r *candidateRepository) FindByJobEmail(ctx context.Context, db *gorm.DB, jobID uuid.UUID, email string) (*models.Candidate, error) {
	return first[models.Candidate](db.WithContext(ctx).Where("job_id = ? AND lower(email) = ?", jobID, strings.ToLower(email)))
}
func (r *candidateRepository) List(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, f CandidateFilter, page dto.Page) ([]models.Candidate, int64, error) {
	q := db.WithContext(ctx).Model(&models.Candidate{}).Where("tenant_id = ?", tenantID)
	if f.JobID != nil {
		q = q.Where("job_id = ?", *f.JobID)
	}
	if f.Stage != "" {
		q = q.Where("stage = ?", f.Stage)
	}
	return paged[models.Candidate](q, "created_at DESC", page)
}
func (r *candidateRepository) AddEvent(ctx context.Context, tx *gorm.DB, e *models.StageEvent) error {
	return tx.WithContext(ctx).Create(e).Error
}
func (r *candidateRepository) Events(ctx context.Context, db *gorm.DB, tenantID, candidateID uuid.UUID) ([]models.StageEvent, error) {
	var out []models.StageEvent
	err := db.WithContext(ctx).Where("tenant_id = ? AND candidate_id = ?", tenantID, candidateID).Order("created_at ASC, id ASC").Find(&out).Error
	return out, err
}

type interviewRepository struct{}

// NewInterviewRepository builds an InterviewRepository.
func NewInterviewRepository() InterviewRepository { return &interviewRepository{} }

func (r *interviewRepository) Create(ctx context.Context, tx *gorm.DB, i *models.Interview) error {
	return tx.WithContext(ctx).Create(i).Error
}
func (r *interviewRepository) Update(ctx context.Context, tx *gorm.DB, i *models.Interview) error {
	return tx.WithContext(ctx).Where("tenant_id = ?", i.TenantID).Save(i).Error
}
func (r *interviewRepository) FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Interview, error) {
	return first[models.Interview](db.WithContext(ctx).Where("tenant_id = ? AND id = ?", tenantID, id))
}
func (r *interviewRepository) ListByCandidate(ctx context.Context, db *gorm.DB, tenantID, candidateID uuid.UUID, page dto.Page) ([]models.Interview, int64, error) {
	q := db.WithContext(ctx).Model(&models.Interview{}).Where("tenant_id = ? AND candidate_id = ?", tenantID, candidateID)
	return paged[models.Interview](q, "scheduled_at ASC", page)
}
func (r *interviewRepository) ListByInterviewer(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID, status string, page dto.Page) ([]models.Interview, int64, error) {
	q := db.WithContext(ctx).Model(&models.Interview{}).Where("tenant_id = ? AND interviewer_employee_id = ?", tenantID, employeeID)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	return paged[models.Interview](q, "scheduled_at ASC", page)
}

type offerRepository struct{}

// NewOfferRepository builds an OfferRepository.
func NewOfferRepository() OfferRepository { return &offerRepository{} }

func (r *offerRepository) Create(ctx context.Context, tx *gorm.DB, o *models.Offer) error {
	return tx.WithContext(ctx).Create(o).Error
}
func (r *offerRepository) Update(ctx context.Context, tx *gorm.DB, o *models.Offer) error {
	return tx.WithContext(ctx).Where("tenant_id = ?", o.TenantID).Save(o).Error
}
func (r *offerRepository) FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Offer, error) {
	return first[models.Offer](db.WithContext(ctx).Where("tenant_id = ? AND id = ?", tenantID, id))
}
func (r *offerRepository) LatestByStatus(ctx context.Context, db *gorm.DB, tenantID, candidateID uuid.UUID, status string) (*models.Offer, error) {
	return first[models.Offer](db.WithContext(ctx).Where("tenant_id = ? AND candidate_id = ? AND status = ?", tenantID, candidateID, status).Order("created_at DESC"))
}
func (r *offerRepository) ListByCandidate(ctx context.Context, db *gorm.DB, tenantID, candidateID uuid.UUID, page dto.Page) ([]models.Offer, int64, error) {
	q := db.WithContext(ctx).Model(&models.Offer{}).Where("tenant_id = ? AND candidate_id = ?", tenantID, candidateID)
	return paged[models.Offer](q, "created_at DESC", page)
}

type outboxRepository struct{}

// NewOutboxRepository builds an OutboxRepository.
func NewOutboxRepository() OutboxRepository { return &outboxRepository{} }

func (r *outboxRepository) Create(ctx context.Context, tx *gorm.DB, e *models.OutboxEvent) error {
	return tx.WithContext(ctx).Create(e).Error
}
func (r *outboxRepository) MarkPublished(ctx context.Context, db *gorm.DB, id uuid.UUID, at time.Time) error {
	return db.WithContext(ctx).Model(&models.OutboxEvent{}).Where("id = ?", id).Updates(map[string]interface{}{"published": true, "published_at": at}).Error
}
func (r *outboxRepository) RecordFailure(ctx context.Context, db *gorm.DB, id uuid.UUID, reason string) error {
	return db.WithContext(ctx).Model(&models.OutboxEvent{}).Where("id = ?", id).
		Updates(map[string]interface{}{"attempts": gorm.Expr("attempts + 1"), "last_error": reason}).Error
}
func (r *outboxRepository) FetchUnpublished(ctx context.Context, db *gorm.DB, limit, maxAttempts int) ([]models.OutboxEvent, error) {
	var out []models.OutboxEvent
	err := db.WithContext(ctx).Where("published = ? AND attempts < ?", false, maxAttempts).Order("created_at ASC").Limit(limit).Find(&out).Error
	return out, err
}

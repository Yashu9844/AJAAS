package repositories

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/leave/dto"
	"github.com/jaas/jaas/internal/leave/models"
	"gorm.io/gorm"
)

type requestRepository struct{}

// NewRequestRepository builds a RequestRepository.
func NewRequestRepository() RequestRepository { return &requestRepository{} }

func (r *requestRepository) Create(ctx context.Context, tx *gorm.DB, req *models.Request) error {
	return tx.WithContext(ctx).Create(req).Error
}

func (r *requestRepository) Update(ctx context.Context, tx *gorm.DB, req *models.Request) error {
	return tx.WithContext(ctx).Where("tenant_id = ?", req.TenantID).Save(req).Error
}

func (r *requestRepository) FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Request, error) {
	return first[models.Request](db.WithContext(ctx).Where("tenant_id = ? AND id = ?", tenantID, id))
}

func (r *requestRepository) Overlapping(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID, from, to time.Time) ([]models.Request, error) {
	var out []models.Request
	err := db.WithContext(ctx).
		Where("tenant_id = ? AND employee_profile_id = ? AND status IN ?", tenantID, employeeID, []string{models.StatusPending, models.StatusApproved}).
		Where("start_date <= ?::date AND end_date >= ?::date", day(to), day(from)).
		Find(&out).Error
	return out, err
}

func (r *requestRepository) List(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, f RequestFilter, page dto.Page) ([]models.Request, int64, error) {
	q := db.WithContext(ctx).Model(&models.Request{}).Where("tenant_id = ?", tenantID)
	if f.EmployeeID != nil {
		q = q.Where("employee_profile_id = ?", *f.EmployeeID)
	}
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	if f.From != nil {
		q = q.Where("end_date >= ?::date", day(*f.From))
	}
	if f.To != nil {
		q = q.Where("start_date <= ?::date", day(*f.To))
	}
	return paged[models.Request](q, "start_date DESC, created_at DESC", page)
}

type outboxRepository struct{}

// NewOutboxRepository builds an OutboxRepository.
func NewOutboxRepository() OutboxRepository { return &outboxRepository{} }

func (r *outboxRepository) Create(ctx context.Context, tx *gorm.DB, e *models.OutboxEvent) error {
	return tx.WithContext(ctx).Create(e).Error
}

func (r *outboxRepository) MarkPublished(ctx context.Context, db *gorm.DB, id uuid.UUID, at time.Time) error {
	return db.WithContext(ctx).Model(&models.OutboxEvent{}).Where("id = ?", id).
		Updates(map[string]interface{}{"published": true, "published_at": at}).Error
}

func (r *outboxRepository) RecordFailure(ctx context.Context, db *gorm.DB, id uuid.UUID, reason string) error {
	return db.WithContext(ctx).Model(&models.OutboxEvent{}).Where("id = ?", id).
		Updates(map[string]interface{}{"attempts": gorm.Expr("attempts + 1"), "last_error": reason}).Error
}

func (r *outboxRepository) FetchUnpublished(ctx context.Context, db *gorm.DB, limit, maxAttempts int) ([]models.OutboxEvent, error) {
	var out []models.OutboxEvent
	err := db.WithContext(ctx).Where("published = ? AND attempts < ?", false, maxAttempts).
		Order("created_at ASC").Limit(limit).Find(&out).Error
	return out, err
}

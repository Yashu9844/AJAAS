package repositories

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/attendance/dto"
	"github.com/jaas/jaas/internal/attendance/models"
	"gorm.io/gorm"
)

type punchRepository struct{}

// NewPunchRepository returns a PunchRepository.
func NewPunchRepository() PunchRepository { return &punchRepository{} }

var _ PunchRepository = (*punchRepository)(nil)

func (r *punchRepository) Create(ctx context.Context, tx *gorm.DB, p *models.AttendancePunch) error {
	return tx.WithContext(ctx).Create(p).Error
}

func (r *punchRepository) LastForEmployee(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID) (*models.AttendancePunch, error) {
	var p models.AttendancePunch
	err := db.WithContext(ctx).
		Where("tenant_id = ? AND employee_profile_id = ? AND is_superseded = FALSE", tenantID, employeeID).
		Order("punch_time DESC").First(&p).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &p, nil
}

func (r *punchRepository) ActiveForRecord(ctx context.Context, db *gorm.DB, tenantID, recordID uuid.UUID) ([]models.AttendancePunch, error) {
	var out []models.AttendancePunch
	err := db.WithContext(ctx).
		Where("tenant_id = ? AND attendance_record_id = ? AND is_superseded = FALSE", tenantID, recordID).
		Order("punch_time ASC").Find(&out).Error
	return out, err
}

func (r *punchRepository) AllForRecord(ctx context.Context, db *gorm.DB, tenantID, recordID uuid.UUID) ([]models.AttendancePunch, error) {
	var out []models.AttendancePunch
	err := db.WithContext(ctx).
		Where("tenant_id = ? AND attendance_record_id = ?", tenantID, recordID).
		Order("punch_time ASC").Find(&out).Error
	return out, err
}

// SupersedeForRecord flags every punch of a record superseded (AT-017); nothing is deleted.
func (r *punchRepository) SupersedeForRecord(ctx context.Context, tx *gorm.DB, tenantID, recordID uuid.UUID) error {
	return tx.WithContext(ctx).Model(&models.AttendancePunch{}).
		Where("tenant_id = ? AND attendance_record_id = ?", tenantID, recordID).
		Update("is_superseded", true).Error
}

type regularizationRepository struct{}

// NewRegularizationRepository returns a RegularizationRepository.
func NewRegularizationRepository() RegularizationRepository { return &regularizationRepository{} }

var _ RegularizationRepository = (*regularizationRepository)(nil)

func (r *regularizationRepository) Create(ctx context.Context, tx *gorm.DB, g *models.Regularization) error {
	return tx.WithContext(ctx).Create(g).Error
}

func (r *regularizationRepository) Update(ctx context.Context, tx *gorm.DB, g *models.Regularization) error {
	return tx.WithContext(ctx).Where("tenant_id = ?", g.TenantID).Save(g).Error
}

func (r *regularizationRepository) first(ctx context.Context, db *gorm.DB, query string, args ...interface{}) (*models.Regularization, error) {
	var g models.Regularization
	if err := db.WithContext(ctx).Where(query, args...).First(&g).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &g, nil
}

func (r *regularizationRepository) FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Regularization, error) {
	return r.first(ctx, db, "tenant_id = ? AND id = ?", tenantID, id)
}

func (r *regularizationRepository) FindPending(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID, date time.Time) (*models.Regularization, error) {
	return r.first(ctx, db, "tenant_id = ? AND employee_profile_id = ? AND attendance_date = ?::date AND status = ?",
		tenantID, employeeID, day(date), models.RegPending)
}

func (r *regularizationRepository) List(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, f RegularizationFilter, page dto.Page) ([]models.Regularization, int64, error) {
	q := db.WithContext(ctx).Model(&models.Regularization{}).Where("tenant_id = ?", tenantID)
	if f.EmployeeID != nil {
		q = q.Where("employee_profile_id = ?", *f.EmployeeID)
	}
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var out []models.Regularization
	err := q.Order("created_at DESC").Offset(page.Offset()).Limit(page.PerPage).Find(&out).Error
	return out, total, err
}

type outboxRepository struct{}

// NewOutboxRepository returns an OutboxRepository.
func NewOutboxRepository() OutboxRepository { return &outboxRepository{} }

var _ OutboxRepository = (*outboxRepository)(nil)

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
	err := db.WithContext(ctx).Where("published = FALSE AND attempts < ?", maxAttempts).
		Order("created_at ASC").Limit(limit).Find(&out).Error
	return out, err
}

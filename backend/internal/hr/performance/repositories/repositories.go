package repositories

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/hr/performance/dto"
	"github.com/jaas/jaas/internal/hr/performance/models"
	"github.com/jaas/jaas/internal/hr/performance/score"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type cycleRepository struct{}

// NewCycleRepository builds a CycleRepository.
func NewCycleRepository() CycleRepository { return &cycleRepository{} }

func (r *cycleRepository) Create(ctx context.Context, tx *gorm.DB, c *models.Cycle) error {
	return tx.WithContext(ctx).Create(c).Error
}
func (r *cycleRepository) Update(ctx context.Context, tx *gorm.DB, c *models.Cycle) error {
	return tx.WithContext(ctx).Where("tenant_id = ?", c.TenantID).Save(c).Error
}
func (r *cycleRepository) FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Cycle, error) {
	return first[models.Cycle](db.WithContext(ctx).Where("tenant_id = ? AND id = ?", tenantID, id))
}
func (r *cycleRepository) LockByID(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) (*models.Cycle, error) {
	return first[models.Cycle](tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_id = ? AND id = ?", tenantID, id))
}
func (r *cycleRepository) List(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, status string, page dto.Page) ([]models.Cycle, int64, error) {
	q := db.WithContext(ctx).Model(&models.Cycle{}).Where("tenant_id = ?", tenantID)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	return paged[models.Cycle](q, "start_date DESC, id", page)
}

type goalRepository struct{}

// NewGoalRepository builds a GoalRepository.
func NewGoalRepository() GoalRepository { return &goalRepository{} }

func (r *goalRepository) Create(ctx context.Context, tx *gorm.DB, g *models.Goal) error {
	return tx.WithContext(ctx).Create(g).Error
}
func (r *goalRepository) Update(ctx context.Context, tx *gorm.DB, g *models.Goal) error {
	return tx.WithContext(ctx).Where("tenant_id = ?", g.TenantID).Save(g).Error
}
func (r *goalRepository) FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Goal, error) {
	return first[models.Goal](db.WithContext(ctx).Where("tenant_id = ? AND id = ?", tenantID, id))
}
func (r *goalRepository) LockByID(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) (*models.Goal, error) {
	return first[models.Goal](tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_id = ? AND id = ?", tenantID, id))
}
func (r *goalRepository) List(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, f GoalFilter, page dto.Page) ([]models.Goal, int64, error) {
	q := db.WithContext(ctx).Model(&models.Goal{}).Where("tenant_id = ?", tenantID)
	if f.CycleID != nil {
		q = q.Where("cycle_id = ?", *f.CycleID)
	}
	if f.EmployeeID != nil {
		q = q.Where("employee_id = ?", *f.EmployeeID)
	}
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	return paged[models.Goal](q, "created_at DESC, id", page)
}
func (r *goalRepository) LockOwner(ctx context.Context, tx *gorm.DB, cycleID, employeeID uuid.UUID) error {
	return tx.WithContext(ctx).Exec("SELECT pg_advisory_xact_lock(hashtextextended(?, 0))", "performance:weight:"+cycleID.String()+":"+employeeID.String()).Error
}
func (r *goalRepository) WeightSum(ctx context.Context, tx *gorm.DB, tenantID, cycleID, employeeID, exclude uuid.UUID) (score.Fixed, error) {
	var raw string
	err := tx.WithContext(ctx).Model(&models.Goal{}).Select("COALESCE(SUM(weight), 0)::text").
		Where("tenant_id = ? AND cycle_id = ? AND employee_id = ? AND status <> ? AND id <> ?", tenantID, cycleID, employeeID, "cancelled", exclude).
		Scan(&raw).Error
	if err != nil {
		return 0, err
	}
	return score.Parse(raw)
}
func (r *goalRepository) Open(ctx context.Context, db *gorm.DB, tenantID, cycleID uuid.UUID) ([]models.Goal, error) {
	var out []models.Goal
	err := db.WithContext(ctx).Where("tenant_id = ? AND cycle_id = ? AND status <> ?", tenantID, cycleID, "cancelled").
		Order("employee_id, created_at").Find(&out).Error
	return out, err
}
func (r *goalRepository) AddKeyResult(ctx context.Context, tx *gorm.DB, k *models.KeyResult) error {
	return tx.WithContext(ctx).Create(k).Error
}
func (r *goalRepository) UpdateKeyResult(ctx context.Context, tx *gorm.DB, k *models.KeyResult) error {
	return tx.WithContext(ctx).Where("tenant_id = ?", k.TenantID).Save(k).Error
}
func (r *goalRepository) KeyResults(ctx context.Context, db *gorm.DB, tenantID, goalID uuid.UUID) ([]models.KeyResult, error) {
	var out []models.KeyResult
	err := db.WithContext(ctx).Where("tenant_id = ? AND goal_id = ?", tenantID, goalID).Order("created_at, id").Find(&out).Error
	return out, err
}
func (r *goalRepository) AddCheckIn(ctx context.Context, tx *gorm.DB, c *models.CheckIn) error {
	return tx.WithContext(ctx).Create(c).Error
}
func (r *goalRepository) CheckIns(ctx context.Context, db *gorm.DB, tenantID, goalID uuid.UUID, limit int) ([]models.CheckIn, error) {
	var out []models.CheckIn
	err := db.WithContext(ctx).Where("tenant_id = ? AND goal_id = ?", tenantID, goalID).Order("created_at DESC, id").Limit(limit).Find(&out).Error
	return out, err
}

type reviewRepository struct{}

// NewReviewRepository builds a ReviewRepository.
func NewReviewRepository() ReviewRepository { return &reviewRepository{} }

func (r *reviewRepository) Create(ctx context.Context, tx *gorm.DB, rv *models.Review) error {
	return tx.WithContext(ctx).Create(rv).Error
}
func (r *reviewRepository) Update(ctx context.Context, tx *gorm.DB, rv *models.Review) error {
	return tx.WithContext(ctx).Where("tenant_id = ?", rv.TenantID).Save(rv).Error
}
func (r *reviewRepository) FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Review, error) {
	return first[models.Review](db.WithContext(ctx).Where("tenant_id = ? AND id = ?", tenantID, id))
}
func (r *reviewRepository) LockByID(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) (*models.Review, error) {
	return first[models.Review](tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_id = ? AND id = ?", tenantID, id))
}
func (r *reviewRepository) Exists(ctx context.Context, tx *gorm.DB, tenantID, cycleID, employeeID, reviewerID uuid.UUID, typ string) (bool, error) {
	var n int64
	err := tx.WithContext(ctx).Model(&models.Review{}).Where("tenant_id = ? AND cycle_id = ? AND employee_id = ? AND reviewer_employee_id = ? AND type = ?",
		tenantID, cycleID, employeeID, reviewerID, typ).Count(&n).Error
	return n > 0, err
}
func (r *reviewRepository) List(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, f ReviewFilter, page dto.Page) ([]models.Review, int64, error) {
	q := db.WithContext(ctx).Model(&models.Review{}).Where("tenant_id = ?", tenantID)
	if f.CycleID != nil {
		q = q.Where("cycle_id = ?", *f.CycleID)
	}
	if f.EmployeeID != nil {
		q = q.Where("employee_id = ?", *f.EmployeeID)
	}
	if f.ReviewerID != nil {
		q = q.Where("reviewer_employee_id = ?", *f.ReviewerID)
	}
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	if f.Type != "" {
		q = q.Where("type = ?", f.Type)
	}
	return paged[models.Review](q, "created_at DESC, id", page)
}
func (r *reviewRepository) ExpirePending(ctx context.Context, tx *gorm.DB, tenantID, cycleID uuid.UUID, at time.Time) (int64, error) {
	res := tx.WithContext(ctx).Model(&models.Review{}).Where("tenant_id = ? AND cycle_id = ? AND status = ?", tenantID, cycleID, models.ReviewPending).
		Updates(map[string]interface{}{"status": models.ReviewExpired, "updated_at": at})
	return res.RowsAffected, res.Error
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

type calibrationRepository struct{}

// NewCalibrationRepository builds a CalibrationRepository.
func NewCalibrationRepository() CalibrationRepository { return &calibrationRepository{} }

func (r *calibrationRepository) Create(ctx context.Context, tx *gorm.DB, c *models.Calibration) error {
	return tx.WithContext(ctx).Create(c).Error
}
func (r *calibrationRepository) Latest(ctx context.Context, db *gorm.DB, tenantID, cycleID, employeeID uuid.UUID) (*models.Calibration, error) {
	return first[models.Calibration](db.WithContext(ctx).Where("tenant_id = ? AND cycle_id = ? AND employee_id = ?", tenantID, cycleID, employeeID).
		Order("created_at DESC, id DESC"))
}
func (r *calibrationRepository) LatestPerEmployee(ctx context.Context, db *gorm.DB, tenantID, cycleID uuid.UUID) ([]models.Calibration, error) {
	var out []models.Calibration
	err := db.WithContext(ctx).Raw(`SELECT DISTINCT ON (employee_id) * FROM performance_calibrations
		WHERE tenant_id = ? AND cycle_id = ? ORDER BY employee_id, created_at DESC, id DESC`, tenantID, cycleID).Scan(&out).Error
	return out, err
}
func (r *calibrationRepository) List(ctx context.Context, db *gorm.DB, tenantID, cycleID uuid.UUID, employeeID *uuid.UUID, page dto.Page) ([]models.Calibration, int64, error) {
	q := db.WithContext(ctx).Model(&models.Calibration{}).Where("tenant_id = ? AND cycle_id = ?", tenantID, cycleID)
	if employeeID != nil {
		q = q.Where("employee_id = ?", *employeeID)
	}
	return paged[models.Calibration](q, "created_at DESC, id DESC", page)
}

type feedbackRepository struct{}

// NewFeedbackRepository builds a FeedbackRepository.
func NewFeedbackRepository() FeedbackRepository { return &feedbackRepository{} }

func (r *feedbackRepository) Create(ctx context.Context, tx *gorm.DB, f *models.Feedback) error {
	return tx.WithContext(ctx).Create(f).Error
}
func (r *feedbackRepository) List(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, f FeedbackFilter, page dto.Page) ([]models.Feedback, int64, error) {
	q := db.WithContext(ctx).Model(&models.Feedback{}).Where("tenant_id = ?", tenantID)
	if f.ToID != nil {
		q = q.Where("to_employee_id = ?", *f.ToID)
	}
	if f.FromID != nil {
		q = q.Where("from_employee_id = ?", *f.FromID)
	}
	if f.PublicOnly {
		q = q.Where("visibility = ?", models.VisibilityPublic)
	}
	return paged[models.Feedback](q, "created_at DESC, id DESC", page)
}
func (r *feedbackRepository) CreateRequest(ctx context.Context, tx *gorm.DB, fr *models.FeedbackRequest) error {
	return tx.WithContext(ctx).Create(fr).Error
}
func (r *feedbackRepository) UpdateRequest(ctx context.Context, tx *gorm.DB, fr *models.FeedbackRequest) error {
	return tx.WithContext(ctx).Where("tenant_id = ?", fr.TenantID).Save(fr).Error
}
func (r *feedbackRepository) LockRequest(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) (*models.FeedbackRequest, error) {
	return first[models.FeedbackRequest](tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_id = ? AND id = ?", tenantID, id))
}
func (r *feedbackRepository) PendingExists(ctx context.Context, tx *gorm.DB, tenantID, requester, responder, about uuid.UUID) (bool, error) {
	var n int64
	err := tx.WithContext(ctx).Model(&models.FeedbackRequest{}).Where(
		"tenant_id = ? AND requester_employee_id = ? AND responder_employee_id = ? AND about_employee_id = ? AND status = ?",
		tenantID, requester, responder, about, models.RequestPending).Count(&n).Error
	return n > 0, err
}
func (r *feedbackRepository) ListRequests(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, f RequestFilter, page dto.Page) ([]models.FeedbackRequest, int64, error) {
	q := db.WithContext(ctx).Model(&models.FeedbackRequest{}).Where("tenant_id = ?", tenantID)
	if f.ResponderID != nil {
		q = q.Where("responder_employee_id = ?", *f.ResponderID)
	}
	if f.RequesterID != nil {
		q = q.Where("requester_employee_id = ?", *f.RequesterID)
	}
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	return paged[models.FeedbackRequest](q, "created_at DESC, id DESC", page)
}

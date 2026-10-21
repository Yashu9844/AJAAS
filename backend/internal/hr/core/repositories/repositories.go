// Package repositories is HR/Core data access: tenant_id on every scoped query, `db`/`tx` passed per call.
package repositories

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/hr/core/dto"
	"github.com/jaas/jaas/internal/hr/core/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ProfileFilter narrows job-profile lists.
type ProfileFilter struct {
	FamilyID *uuid.UUID
	Status   string
	Category string
}

// PositionFilter narrows position lists.
type PositionFilter struct {
	DepartmentID *uuid.UUID
	JobProfileID *uuid.UUID
	Status       string
}

// AssignmentFilter narrows the "as of" view (CO-022).
type AssignmentFilter struct {
	DepartmentID *uuid.UUID
	PositionID   *uuid.UUID
}

// ActionFilter narrows action lists.
type ActionFilter struct {
	EmployeeID *uuid.UUID
	Status     string
	Type       string
}

// Repository is HR/Core's single data-access surface (one aggregate per method group).
type Repository interface {
	Create(ctx context.Context, tx *gorm.DB, row interface{}) error
	Save(ctx context.Context, tx *gorm.DB, row interface{}) error
	// CodeTaken reports whether another row of model's table in the tenant uses code.
	CodeTaken(ctx context.Context, db *gorm.DB, model interface{}, tenantID uuid.UUID, code string, except uuid.UUID) (bool, error)

	Family(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.JobFamily, error)
	Families(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, status string, page dto.Page) ([]models.JobFamily, int64, error)
	Grade(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Grade, error)
	Grades(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, status string, page dto.Page) ([]models.Grade, int64, error)
	RankTaken(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, rank int, except uuid.UUID) (bool, error)
	Profile(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.JobProfile, error)
	Profiles(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, f ProfileFilter, page dto.Page) ([]models.JobProfile, int64, error)

	Position(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Position, error)
	Positions(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, f PositionFilter, page dto.Page) ([]models.Position, int64, error)

	// LockWorker / LockPosition take transaction-scoped advisory locks (CO-020, CO-014).
	LockWorker(ctx context.Context, tx *gorm.DB, employeeID uuid.UUID) error
	LockPosition(ctx context.Context, tx *gorm.DB, positionID uuid.UUID) error
	Assignment(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Assignment, error)
	// WorkerAssignments returns every assignment of the worker (rescinded included), oldest first.
	WorkerAssignments(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID) ([]models.Assignment, error)
	// PositionAssignments returns the position's active assignments that overlap [from, to) (to nil = open).
	PositionAssignments(ctx context.Context, db *gorm.DB, tenantID, positionID uuid.UUID, from time.Time, to *time.Time) ([]models.Assignment, error)
	// AsOf returns the active assignments in effect on the date.
	AsOf(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, asOf time.Time, f AssignmentFilter, page dto.Page) ([]models.Assignment, int64, error)
	// AssignedWorkers returns the workers that have any active assignment.
	AssignedWorkers(ctx context.Context, db *gorm.DB, tenantID uuid.UUID) (map[uuid.UUID]bool, error)

	Action(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Action, error)
	LockAction(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) (*models.Action, error)
	Actions(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, f ActionFilter, page dto.Page) ([]models.Action, int64, error)
	// CompletedActions returns the worker's completed actions, latest effective first.
	CompletedActions(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID) ([]models.Action, error)
	// PendingEffects returns completed actions whose effects are due on or before the date (cross-tenant, sweep).
	PendingEffects(ctx context.Context, db *gorm.DB, today time.Time, limit int) ([]models.Action, error)

	Plans(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, departmentID *uuid.UUID, page dto.Page) ([]models.HeadcountPlan, int64, error)
	PlanOverlaps(ctx context.Context, db *gorm.DB, tenantID, departmentID uuid.UUID, start, end time.Time) (bool, error)

	MarkPublished(ctx context.Context, db *gorm.DB, id uuid.UUID, at time.Time) error
	RecordFailure(ctx context.Context, db *gorm.DB, id uuid.UUID, reason string) error
	FetchUnpublished(ctx context.Context, db *gorm.DB, limit, maxAttempts int) ([]models.OutboxEvent, error)
}

type repository struct{}

// New builds the HR/Core repository.
func New() Repository { return repository{} }

func (repository) Create(ctx context.Context, tx *gorm.DB, row interface{}) error {
	return tx.WithContext(ctx).Create(row).Error
}
func (repository) Save(ctx context.Context, tx *gorm.DB, row interface{}) error {
	return tx.WithContext(ctx).Save(row).Error
}
func (repository) CodeTaken(ctx context.Context, db *gorm.DB, model interface{}, tenantID uuid.UUID, code string, except uuid.UUID) (bool, error) {
	var n int64
	err := db.WithContext(ctx).Model(model).Where("tenant_id = ? AND code = ? AND id <> ?", tenantID, code, except).Count(&n).Error
	return n > 0, err
}

func scoped[T any](ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*T, error) {
	return first[T](db.WithContext(ctx).Where("tenant_id = ? AND id = ?", tenantID, id))
}

func (repository) Family(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.JobFamily, error) {
	return scoped[models.JobFamily](ctx, db, tenantID, id)
}
func (repository) Families(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, status string, page dto.Page) ([]models.JobFamily, int64, error) {
	return paged[models.JobFamily](withStatus(db.WithContext(ctx).Model(&models.JobFamily{}).Where("tenant_id = ?", tenantID), status), "name, id", page)
}
func (repository) Grade(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Grade, error) {
	return scoped[models.Grade](ctx, db, tenantID, id)
}
func (repository) Grades(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, status string, page dto.Page) ([]models.Grade, int64, error) {
	return paged[models.Grade](withStatus(db.WithContext(ctx).Model(&models.Grade{}).Where("tenant_id = ?", tenantID), status), "rank, id", page)
}
func (repository) RankTaken(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, rank int, except uuid.UUID) (bool, error) {
	var n int64
	err := db.WithContext(ctx).Model(&models.Grade{}).Where("tenant_id = ? AND rank = ? AND id <> ?", tenantID, rank, except).Count(&n).Error
	return n > 0, err
}
func (repository) Profile(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.JobProfile, error) {
	return scoped[models.JobProfile](ctx, db, tenantID, id)
}
func (repository) Profiles(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, f ProfileFilter, page dto.Page) ([]models.JobProfile, int64, error) {
	q := withStatus(db.WithContext(ctx).Model(&models.JobProfile{}).Where("tenant_id = ?", tenantID), f.Status)
	if f.FamilyID != nil {
		q = q.Where("family_id = ?", *f.FamilyID)
	}
	if f.Category != "" {
		q = q.Where("category = ?", f.Category)
	}
	return paged[models.JobProfile](q, "title, id", page)
}

func (repository) Position(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Position, error) {
	return scoped[models.Position](ctx, db, tenantID, id)
}
func (repository) Positions(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, f PositionFilter, page dto.Page) ([]models.Position, int64, error) {
	q := withStatus(db.WithContext(ctx).Model(&models.Position{}).Where("tenant_id = ?", tenantID), f.Status)
	if f.DepartmentID != nil {
		q = q.Where("department_id = ?", *f.DepartmentID)
	}
	if f.JobProfileID != nil {
		q = q.Where("job_profile_id = ?", *f.JobProfileID)
	}
	return paged[models.Position](q, "code, id", page)
}

func (repository) LockWorker(ctx context.Context, tx *gorm.DB, employeeID uuid.UUID) error {
	return tx.WithContext(ctx).Exec("SELECT pg_advisory_xact_lock(hashtextextended(?, 0))", "core:worker:"+employeeID.String()).Error
}
func (repository) LockPosition(ctx context.Context, tx *gorm.DB, positionID uuid.UUID) error {
	return tx.WithContext(ctx).Exec("SELECT pg_advisory_xact_lock(hashtextextended(?, 0))", "core:position:"+positionID.String()).Error
}
func (repository) Assignment(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Assignment, error) {
	return scoped[models.Assignment](ctx, db, tenantID, id)
}
func (repository) WorkerAssignments(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID) ([]models.Assignment, error) {
	var out []models.Assignment
	err := db.WithContext(ctx).Where("tenant_id = ? AND employee_id = ?", tenantID, employeeID).Order("effective_from, created_at").Find(&out).Error
	return out, err
}
func (repository) PositionAssignments(ctx context.Context, db *gorm.DB, tenantID, positionID uuid.UUID, from time.Time, to *time.Time) ([]models.Assignment, error) {
	q := db.WithContext(ctx).Where("tenant_id = ? AND position_id = ? AND status = ? AND (effective_to IS NULL OR effective_to > ?)",
		tenantID, positionID, models.AssignmentActive, from)
	if to != nil {
		q = q.Where("effective_from < ?", *to)
	}
	var out []models.Assignment
	err := q.Order("effective_from").Find(&out).Error
	return out, err
}
func (repository) AsOf(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, asOf time.Time, f AssignmentFilter, page dto.Page) ([]models.Assignment, int64, error) {
	q := db.WithContext(ctx).Model(&models.Assignment{}).Where("tenant_id = ? AND status = ? AND effective_from <= ? AND (effective_to IS NULL OR effective_to > ?)",
		tenantID, models.AssignmentActive, asOf, asOf)
	if f.DepartmentID != nil {
		q = q.Where("department_id = ?", *f.DepartmentID)
	}
	if f.PositionID != nil {
		q = q.Where("position_id = ?", *f.PositionID)
	}
	return paged[models.Assignment](q, "effective_from, id", page)
}
func (repository) AssignedWorkers(ctx context.Context, db *gorm.DB, tenantID uuid.UUID) (map[uuid.UUID]bool, error) {
	var ids []uuid.UUID
	err := db.WithContext(ctx).Model(&models.Assignment{}).Where("tenant_id = ? AND status = ?", tenantID, models.AssignmentActive).
		Distinct().Pluck("employee_id", &ids).Error
	out := make(map[uuid.UUID]bool, len(ids))
	for _, id := range ids {
		out[id] = true
	}
	return out, err
}

func (repository) Action(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Action, error) {
	return scoped[models.Action](ctx, db, tenantID, id)
}
func (repository) LockAction(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) (*models.Action, error) {
	return first[models.Action](tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_id = ? AND id = ?", tenantID, id))
}
func (repository) Actions(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, f ActionFilter, page dto.Page) ([]models.Action, int64, error) {
	q := withStatus(db.WithContext(ctx).Model(&models.Action{}).Where("tenant_id = ?", tenantID), f.Status)
	if f.EmployeeID != nil {
		q = q.Where("employee_id = ?", *f.EmployeeID)
	}
	if f.Type != "" {
		q = q.Where("type = ?", f.Type)
	}
	return paged[models.Action](q, "created_at DESC, id", page)
}
func (repository) CompletedActions(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID) ([]models.Action, error) {
	var out []models.Action
	err := db.WithContext(ctx).Where("tenant_id = ? AND employee_id = ? AND status = ?", tenantID, employeeID, "completed").
		Order("effective_date DESC, completed_at DESC").Find(&out).Error
	return out, err
}
func (repository) PendingEffects(ctx context.Context, db *gorm.DB, today time.Time, limit int) ([]models.Action, error) {
	var out []models.Action
	err := db.WithContext(ctx).Where("status = ? AND effects_applied_at IS NULL AND effective_date <= ?", "completed", today).
		Order("effective_date, completed_at").Limit(limit).Find(&out).Error
	return out, err
}

func (repository) Plans(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, departmentID *uuid.UUID, page dto.Page) ([]models.HeadcountPlan, int64, error) {
	q := db.WithContext(ctx).Model(&models.HeadcountPlan{}).Where("tenant_id = ?", tenantID)
	if departmentID != nil {
		q = q.Where("department_id = ?", *departmentID)
	}
	return paged[models.HeadcountPlan](q, "period_start DESC, id", page)
}
func (repository) PlanOverlaps(ctx context.Context, db *gorm.DB, tenantID, departmentID uuid.UUID, start, end time.Time) (bool, error) {
	var n int64
	err := db.WithContext(ctx).Model(&models.HeadcountPlan{}).Where("tenant_id = ? AND department_id = ? AND period_start <= ? AND period_end >= ?",
		tenantID, departmentID, end, start).Count(&n).Error
	return n > 0, err
}

func (repository) MarkPublished(ctx context.Context, db *gorm.DB, id uuid.UUID, at time.Time) error {
	return db.WithContext(ctx).Model(&models.OutboxEvent{}).Where("id = ?", id).Updates(map[string]interface{}{"published": true, "published_at": at}).Error
}
func (repository) RecordFailure(ctx context.Context, db *gorm.DB, id uuid.UUID, reason string) error {
	return db.WithContext(ctx).Model(&models.OutboxEvent{}).Where("id = ?", id).
		Updates(map[string]interface{}{"attempts": gorm.Expr("attempts + 1"), "last_error": reason}).Error
}
func (repository) FetchUnpublished(ctx context.Context, db *gorm.DB, limit, maxAttempts int) ([]models.OutboxEvent, error) {
	var out []models.OutboxEvent
	err := db.WithContext(ctx).Where("published = ? AND attempts < ?", false, maxAttempts).Order("created_at ASC").Limit(limit).Find(&out).Error
	return out, err
}

func withStatus(q *gorm.DB, status string) *gorm.DB {
	if status == "" {
		return q
	}
	return q.Where("status = ?", status)
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

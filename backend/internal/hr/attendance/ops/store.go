package ops

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	attModels "github.com/jaas/jaas/internal/hr/attendance/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// OTRow is one worker-day with overtime-relevant minutes (AP-04).
type OTRow struct {
	EmployeeID      uuid.UUID
	Date            time.Time
	WorkMinutes     int
	OvertimeMinutes int
}

// Store is parity data access (tenant-scoped; `tx` passed per call).
type Store interface {
	InTx(ctx context.Context, fn func(tx *gorm.DB) error) error
	DB() *gorm.DB
	Create(ctx context.Context, tx *gorm.DB, row interface{}) error
	Save(ctx context.Context, tx *gorm.DB, row interface{}) error

	Policy(ctx context.Context, db *gorm.DB, tenantID uuid.UUID) (*Policy, error)
	SavePolicy(ctx context.Context, tx *gorm.DB, p *Policy) error
	AutoAbsentTenants(ctx context.Context, db *gorm.DB) ([]uuid.UUID, error)

	Roster(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*Roster, error)
	Rosters(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, status string) ([]Roster, error)
	RosterNameTaken(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, name string, except *uuid.UUID) (bool, error)

	Regularization(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*attModels.Regularization, error)
	PendingRegularizations(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, employeeIDs []uuid.UUID, all bool, limit int) ([]attModels.Regularization, error)

	OvertimeRows(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, from, to time.Time, employeeID *uuid.UUID) ([]OTRow, error)

	Imports(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, limit int) ([]ImportLog, error)
	JobRuns(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, limit int) ([]JobRun, error)
	JobRan(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, kind string, date time.Time) (bool, error)
}

type gormStore struct{ db *gorm.DB }

// NewStore builds the gorm-backed Store.
func NewStore(db *gorm.DB) Store { return gormStore{db: db} }

func (s gormStore) InTx(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return s.db.WithContext(ctx).Transaction(fn)
}
func (s gormStore) DB() *gorm.DB { return s.db }
func (s gormStore) Create(ctx context.Context, tx *gorm.DB, row interface{}) error {
	return tx.WithContext(ctx).Create(row).Error
}
func (s gormStore) Save(ctx context.Context, tx *gorm.DB, row interface{}) error {
	return tx.WithContext(ctx).Save(row).Error
}

func (s gormStore) Policy(ctx context.Context, db *gorm.DB, t uuid.UUID) (*Policy, error) {
	var p Policy
	err := db.WithContext(ctx).Where("tenant_id = ?", t).Take(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &p, err
}
func (s gormStore) SavePolicy(ctx context.Context, tx *gorm.DB, p *Policy) error {
	return tx.WithContext(ctx).Clauses(clause.OnConflict{UpdateAll: true}).Create(p).Error
}
func (s gormStore) AutoAbsentTenants(ctx context.Context, db *gorm.DB) ([]uuid.UUID, error) {
	var ids []uuid.UUID
	return ids, db.WithContext(ctx).Model(&Policy{}).Where("auto_absent_enabled").Pluck("tenant_id", &ids).Error
}

func (s gormStore) Roster(ctx context.Context, db *gorm.DB, t, id uuid.UUID) (*Roster, error) {
	var r Roster
	err := db.WithContext(ctx).Where("tenant_id = ? AND id = ?", t, id).Take(&r).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &r, err
}
func (s gormStore) Rosters(ctx context.Context, db *gorm.DB, t uuid.UUID, status string) ([]Roster, error) {
	q := db.WithContext(ctx).Where("tenant_id = ?", t)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	var out []Roster
	return out, q.Order("lower(name), id").Find(&out).Error
}
func (s gormStore) RosterNameTaken(ctx context.Context, db *gorm.DB, t uuid.UUID, name string, except *uuid.UUID) (bool, error) {
	q := db.WithContext(ctx).Model(&Roster{}).Where("tenant_id = ? AND lower(name) = lower(?)", t, name)
	if except != nil {
		q = q.Where("id <> ?", *except)
	}
	var n int64
	return n > 0, q.Count(&n).Error
}

func (s gormStore) Regularization(ctx context.Context, db *gorm.DB, t, id uuid.UUID) (*attModels.Regularization, error) {
	var r attModels.Regularization
	err := db.WithContext(ctx).Where("tenant_id = ? AND id = ?", t, id).Take(&r).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &r, err
}
func (s gormStore) PendingRegularizations(ctx context.Context, db *gorm.DB, t uuid.UUID, ids []uuid.UUID, all bool, limit int) ([]attModels.Regularization, error) {
	var out []attModels.Regularization
	if !all && len(ids) == 0 {
		return out, nil
	}
	q := db.WithContext(ctx).Where("tenant_id = ? AND status = ?", t, attModels.RegPending)
	if !all {
		q = q.Where("employee_profile_id IN ?", ids)
	}
	return out, q.Order("attendance_date, created_at, id").Limit(limit).Find(&out).Error
}

func (s gormStore) OvertimeRows(ctx context.Context, db *gorm.DB, t uuid.UUID, from, to time.Time, emp *uuid.UUID) ([]OTRow, error) {
	q := db.WithContext(ctx).Model(&attModels.AttendanceRecord{}).
		Select("employee_profile_id AS employee_id, attendance_date AS date, total_work_minutes AS work_minutes, overtime_minutes").
		Where("tenant_id = ? AND attendance_date >= ? AND attendance_date <= ? AND total_work_minutes > 0", t, from.Format(DateLayout), to.Format(DateLayout))
	if emp != nil {
		q = q.Where("employee_profile_id = ?", *emp)
	}
	var out []OTRow
	return out, q.Order("employee_profile_id, attendance_date").Scan(&out).Error
}

func (s gormStore) Imports(ctx context.Context, db *gorm.DB, t uuid.UUID, limit int) ([]ImportLog, error) {
	var out []ImportLog
	return out, db.WithContext(ctx).Where("tenant_id = ?", t).Order("created_at DESC, id").Limit(limit).Find(&out).Error
}
func (s gormStore) JobRuns(ctx context.Context, db *gorm.DB, t uuid.UUID, limit int) ([]JobRun, error) {
	var out []JobRun
	return out, db.WithContext(ctx).Where("tenant_id = ?", t).Order("ran_at DESC, id").Limit(limit).Find(&out).Error
}
func (s gormStore) JobRan(ctx context.Context, db *gorm.DB, t uuid.UUID, kind string, date time.Time) (bool, error) {
	var n int64
	err := db.WithContext(ctx).Model(&JobRun{}).Where("tenant_id = ? AND kind = ? AND run_date = ?", t, kind, date.Format(DateLayout)).Count(&n).Error
	return n > 0, err
}

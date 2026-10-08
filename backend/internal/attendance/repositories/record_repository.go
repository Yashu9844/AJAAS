package repositories

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/attendance/dto"
	"github.com/jaas/jaas/internal/attendance/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type recordRepository struct{}

// NewRecordRepository returns a RecordRepository.
func NewRecordRepository() RecordRepository { return &recordRepository{} }

var _ RecordRepository = (*recordRepository)(nil)

// LockEmployee takes a transaction-scoped advisory lock keyed on tenant+employee (AT-022, D3-09).
func (r *recordRepository) LockEmployee(ctx context.Context, tx *gorm.DB, tenantID, employeeID uuid.UUID) error {
	return tx.WithContext(ctx).Exec("SELECT pg_advisory_xact_lock(hashtext(?))", tenantID.String()+":"+employeeID.String()).Error
}

// FindOrCreate inserts seed unless (tenant, employee, date) exists, then loads the stored row.
func (r *recordRepository) FindOrCreate(ctx context.Context, tx *gorm.DB, seed *models.AttendanceRecord) (*models.AttendanceRecord, error) {
	err := tx.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "tenant_id"}, {Name: "employee_profile_id"}, {Name: "attendance_date"}},
		DoNothing: true,
	}).Create(seed).Error
	if err != nil {
		return nil, err
	}
	rec, err := r.FindByDate(ctx, tx, seed.TenantID, seed.EmployeeProfileID, seed.AttendanceDate)
	if err == nil && rec == nil {
		return nil, gorm.ErrRecordNotFound
	}
	return rec, err
}

func (r *recordRepository) Update(ctx context.Context, tx *gorm.DB, rec *models.AttendanceRecord) error {
	return tx.WithContext(ctx).Where("tenant_id = ?", rec.TenantID).Save(rec).Error
}

func (r *recordRepository) first(ctx context.Context, db *gorm.DB, query string, args ...interface{}) (*models.AttendanceRecord, error) {
	var rec models.AttendanceRecord
	if err := db.WithContext(ctx).Where(query, args...).First(&rec).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &rec, nil
}

func (r *recordRepository) FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.AttendanceRecord, error) {
	return r.first(ctx, db, "tenant_id = ? AND id = ?", tenantID, id)
}

func (r *recordRepository) FindByDate(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID, date time.Time) (*models.AttendanceRecord, error) {
	return r.first(ctx, db, "tenant_id = ? AND employee_profile_id = ? AND attendance_date = ?::date", tenantID, employeeID, day(date))
}

func (r *recordRepository) ListForEmployee(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID, from, to time.Time) ([]models.AttendanceRecord, error) {
	var out []models.AttendanceRecord
	err := db.WithContext(ctx).
		Where("tenant_id = ? AND employee_profile_id = ?", tenantID, employeeID).
		Where("attendance_date BETWEEN ?::date AND ?::date", day(from), day(to)).
		Order("attendance_date DESC").Find(&out).Error
	return out, err
}

func (r *recordRepository) List(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, f RecordFilter, page dto.Page) ([]models.AttendanceRecord, int64, error) {
	q := db.WithContext(ctx).Model(&models.AttendanceRecord{}).
		Where("tenant_id = ?", tenantID).
		Where("attendance_date BETWEEN ?::date AND ?::date", day(f.From), day(f.To))
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
	var out []models.AttendanceRecord
	err := q.Order("attendance_date DESC, employee_profile_id ASC").Offset(page.Offset()).Limit(page.PerPage).Find(&out).Error
	return out, total, err
}

// CountByStatus aggregates one tenant-date in a single query (NFR-P003).
func (r *recordRepository) CountByStatus(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, date time.Time) (StatusCounts, error) {
	var rows []struct {
		Status string
		Total  int64
		Late   int64
	}
	err := db.WithContext(ctx).Model(&models.AttendanceRecord{}).
		Select("status, COUNT(*) AS total, SUM(CASE WHEN late_minutes > 0 THEN 1 ELSE 0 END) AS late").
		Where("tenant_id = ? AND attendance_date = ?::date", tenantID, day(date)).
		Group("status").Scan(&rows).Error
	counts := StatusCounts{ByStatus: map[string]int64{}}
	if err != nil {
		return counts, err
	}
	for _, row := range rows {
		counts.ByStatus[row.Status] = row.Total
		counts.Late += row.Late
	}
	return counts, nil
}

package plus

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/hr/leave/calc"
	leaveModels "github.com/jaas/jaas/internal/hr/leave/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// TenantHoliday is a tenant calendar entry shown on the team calendar.
type TenantHoliday struct {
	Date       Date   `json:"date"`
	Name       string `json:"name"`
	IsOptional bool   `json:"is_optional"`
}

// CompOffFilter and EncashFilter narrow list endpoints.
type CompOffFilter struct {
	EmployeeID *uuid.UUID
	Status     string
}

// EncashFilter narrows encashment lists.
type EncashFilter struct {
	EmployeeID  *uuid.UUID
	LeaveTypeID *uuid.UUID
	Status      string
}

// Store is parity data access (tenant-scoped; `tx` passed per call).
type Store interface {
	InTx(ctx context.Context, fn func(tx *gorm.DB) error) error
	DB() *gorm.DB
	Create(ctx context.Context, tx *gorm.DB, row interface{}) error
	Save(ctx context.Context, tx *gorm.DB, row interface{}) error
	Delete(ctx context.Context, tx *gorm.DB, row interface{}) error

	Policy(ctx context.Context, db *gorm.DB, tenantID uuid.UUID) (*Policy, error)
	SavePolicy(ctx context.Context, tx *gorm.DB, p *Policy) error
	BalancesExist(ctx context.Context, db *gorm.DB, tenantID uuid.UUID) (bool, error)
	TypeState(ctx context.Context, db *gorm.DB, tenantID, typeID uuid.UUID) (exists, active bool, err error)
	CompOffTenants(ctx context.Context, db *gorm.DB) ([]uuid.UUID, error)

	LocationHoliday(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*LocationHoliday, error)
	LocationHolidays(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, locationID *uuid.UUID, from, to time.Time) ([]LocationHoliday, error)
	HolidayTaken(ctx context.Context, db *gorm.DB, tenantID, locationID uuid.UUID, date time.Time) (bool, error)
	WeekOff(ctx context.Context, db *gorm.DB, tenantID, locationID uuid.UUID) (*WeekOff, error)
	WeekOffs(ctx context.Context, db *gorm.DB, tenantID uuid.UUID) ([]WeekOff, error)
	SaveWeekOff(ctx context.Context, tx *gorm.DB, w *WeekOff) error
	TenantHolidays(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, from, to time.Time) ([]TenantHoliday, error)

	PendingRequests(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, employeeIDs []uuid.UUID, all bool, limit int) ([]leaveModels.Request, error)
	Request(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*leaveModels.Request, error)
	RequestsOverlapping(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, employeeIDs []uuid.UUID, all bool, from, to time.Time) ([]leaveModels.Request, error)

	CompOff(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID, lock bool) (*CompOff, error)
	CompOffOnDay(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID, date time.Time) (*CompOff, error)
	CompOffs(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, f CompOffFilter, limit int) ([]CompOff, error)
	ExpiredCompOffs(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, today time.Time) ([]CompOff, error)

	EncashmentRule(ctx context.Context, db *gorm.DB, tenantID, typeID uuid.UUID) (*EncashmentRule, error)
	EncashmentRules(ctx context.Context, db *gorm.DB, tenantID uuid.UUID) ([]EncashmentRule, error)
	SaveEncashmentRule(ctx context.Context, tx *gorm.DB, r *EncashmentRule) error
	Encashment(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID, lock bool) (*Encashment, error)
	PendingEncashment(ctx context.Context, db *gorm.DB, tenantID, employeeID, typeID uuid.UUID) (*Encashment, error)
	EncashedDays(ctx context.Context, db *gorm.DB, tenantID, employeeID, typeID uuid.UUID, year int, statuses ...string) (calc.Days, error)
	Encashments(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, f EncashFilter, limit int) ([]Encashment, error)

	// Event stores an outbox event for the existing leave relay (ids and day counts only).
	Event(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, key string, payload map[string]string) error
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
func (s gormStore) Delete(ctx context.Context, tx *gorm.DB, row interface{}) error {
	return tx.WithContext(ctx).Delete(row).Error
}

func take[T any](q *gorm.DB) (*T, error) {
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

func scoped(ctx context.Context, db *gorm.DB, t, id uuid.UUID, lock bool) *gorm.DB {
	q := db.WithContext(ctx)
	if lock {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	return q.Where("tenant_id = ? AND id = ?", t, id)
}

func (s gormStore) Policy(ctx context.Context, db *gorm.DB, t uuid.UUID) (*Policy, error) {
	return take[Policy](db.WithContext(ctx).Where("tenant_id = ?", t))
}
func (s gormStore) SavePolicy(ctx context.Context, tx *gorm.DB, p *Policy) error {
	return tx.WithContext(ctx).Clauses(clause.OnConflict{UpdateAll: true}).Create(p).Error
}
func (s gormStore) BalancesExist(ctx context.Context, db *gorm.DB, t uuid.UUID) (bool, error) {
	var n int64
	err := db.WithContext(ctx).Model(&leaveModels.Balance{}).Where("tenant_id = ?", t).Limit(1).Count(&n).Error
	return n > 0, err
}
func (s gormStore) TypeState(ctx context.Context, db *gorm.DB, t, id uuid.UUID) (bool, bool, error) {
	lt, err := take[leaveModels.LeaveType](db.WithContext(ctx).Where("tenant_id = ? AND id = ?", t, id))
	if err != nil || lt == nil {
		return false, false, err
	}
	return true, lt.Status == leaveModels.TypeActive, nil
}
func (s gormStore) CompOffTenants(ctx context.Context, db *gorm.DB) ([]uuid.UUID, error) {
	var ids []uuid.UUID
	return ids, db.WithContext(ctx).Model(&Policy{}).Where("comp_off_leave_type_id IS NOT NULL").Pluck("tenant_id", &ids).Error
}

func (s gormStore) LocationHoliday(ctx context.Context, db *gorm.DB, t, id uuid.UUID) (*LocationHoliday, error) {
	return take[LocationHoliday](scoped(ctx, db, t, id, false))
}
func (s gormStore) LocationHolidays(ctx context.Context, db *gorm.DB, t uuid.UUID, loc *uuid.UUID, from, to time.Time) ([]LocationHoliday, error) {
	q := db.WithContext(ctx).Where("tenant_id = ? AND holiday_date >= ? AND holiday_date <= ?", t, from.Format(DateLayout), to.Format(DateLayout))
	if loc != nil {
		q = q.Where("location_id = ?", *loc)
	}
	var out []LocationHoliday
	return out, q.Order("holiday_date, location_id").Find(&out).Error
}
func (s gormStore) HolidayTaken(ctx context.Context, db *gorm.DB, t, loc uuid.UUID, date time.Time) (bool, error) {
	var n int64
	err := db.WithContext(ctx).Model(&LocationHoliday{}).Where("tenant_id = ? AND location_id = ? AND holiday_date = ?", t, loc, date.Format(DateLayout)).Count(&n).Error
	return n > 0, err
}
func (s gormStore) WeekOff(ctx context.Context, db *gorm.DB, t, loc uuid.UUID) (*WeekOff, error) {
	return take[WeekOff](db.WithContext(ctx).Where("tenant_id = ? AND location_id = ?", t, loc))
}
func (s gormStore) WeekOffs(ctx context.Context, db *gorm.DB, t uuid.UUID) ([]WeekOff, error) {
	var out []WeekOff
	return out, db.WithContext(ctx).Where("tenant_id = ?", t).Order("location_id").Find(&out).Error
}
func (s gormStore) SaveWeekOff(ctx context.Context, tx *gorm.DB, w *WeekOff) error {
	return tx.WithContext(ctx).Clauses(clause.OnConflict{UpdateAll: true}).Create(w).Error
}
func (s gormStore) TenantHolidays(ctx context.Context, db *gorm.DB, t uuid.UUID, from, to time.Time) ([]TenantHoliday, error) {
	var rows []leaveModels.Holiday
	err := db.WithContext(ctx).Where("tenant_id = ? AND holiday_date >= ? AND holiday_date <= ?", t, from.Format(DateLayout), to.Format(DateLayout)).
		Order("holiday_date").Find(&rows).Error
	out := make([]TenantHoliday, 0, len(rows))
	for _, h := range rows {
		out = append(out, TenantHoliday{Date: Date{h.HolidayDate}, Name: h.Name, IsOptional: h.IsOptional})
	}
	return out, err
}

func (s gormStore) PendingRequests(ctx context.Context, db *gorm.DB, t uuid.UUID, ids []uuid.UUID, all bool, limit int) ([]leaveModels.Request, error) {
	var out []leaveModels.Request
	if !all && len(ids) == 0 {
		return out, nil
	}
	q := db.WithContext(ctx).Where("tenant_id = ? AND status = ?", t, leaveModels.StatusPending)
	if !all {
		q = q.Where("employee_profile_id IN ?", ids)
	}
	return out, q.Order("start_date, created_at, id").Limit(limit).Find(&out).Error
}
func (s gormStore) Request(ctx context.Context, db *gorm.DB, t, id uuid.UUID) (*leaveModels.Request, error) {
	return take[leaveModels.Request](scoped(ctx, db, t, id, false))
}
func (s gormStore) RequestsOverlapping(ctx context.Context, db *gorm.DB, t uuid.UUID, ids []uuid.UUID, all bool, from, to time.Time) ([]leaveModels.Request, error) {
	var out []leaveModels.Request
	if !all && len(ids) == 0 {
		return out, nil
	}
	q := db.WithContext(ctx).Where("tenant_id = ? AND status IN ? AND start_date <= ? AND end_date >= ?", t,
		[]string{leaveModels.StatusPending, leaveModels.StatusApproved}, to.Format(DateLayout), from.Format(DateLayout))
	if !all {
		q = q.Where("employee_profile_id IN ?", ids)
	}
	return out, q.Order("start_date, employee_profile_id, id").Find(&out).Error
}

func (s gormStore) CompOff(ctx context.Context, db *gorm.DB, t, id uuid.UUID, lock bool) (*CompOff, error) {
	return take[CompOff](scoped(ctx, db, t, id, lock))
}
func (s gormStore) CompOffOnDay(ctx context.Context, db *gorm.DB, t, emp uuid.UUID, date time.Time) (*CompOff, error) {
	return take[CompOff](db.WithContext(ctx).Where("tenant_id = ? AND employee_profile_id = ? AND worked_date = ? AND status IN ?", t, emp,
		date.Format(DateLayout), []string{StatusPending, StatusApproved}))
}
func (s gormStore) CompOffs(ctx context.Context, db *gorm.DB, t uuid.UUID, f CompOffFilter, limit int) ([]CompOff, error) {
	q := db.WithContext(ctx).Where("tenant_id = ?", t)
	if f.EmployeeID != nil {
		q = q.Where("employee_profile_id = ?", *f.EmployeeID)
	}
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	var out []CompOff
	return out, q.Order("created_at DESC, id").Limit(limit).Find(&out).Error
}
func (s gormStore) ExpiredCompOffs(ctx context.Context, db *gorm.DB, t uuid.UUID, today time.Time) ([]CompOff, error) {
	var out []CompOff
	return out, db.WithContext(ctx).Where("tenant_id = ? AND status = ? AND expired_at IS NULL AND expires_on < ?", t, StatusApproved, today.Format(DateLayout)).
		Order("expires_on, id").Find(&out).Error
}

func (s gormStore) EncashmentRule(ctx context.Context, db *gorm.DB, t, typ uuid.UUID) (*EncashmentRule, error) {
	return take[EncashmentRule](db.WithContext(ctx).Where("tenant_id = ? AND leave_type_id = ?", t, typ))
}
func (s gormStore) EncashmentRules(ctx context.Context, db *gorm.DB, t uuid.UUID) ([]EncashmentRule, error) {
	var out []EncashmentRule
	return out, db.WithContext(ctx).Where("tenant_id = ?", t).Order("leave_type_id").Find(&out).Error
}
func (s gormStore) SaveEncashmentRule(ctx context.Context, tx *gorm.DB, r *EncashmentRule) error {
	return tx.WithContext(ctx).Clauses(clause.OnConflict{UpdateAll: true}).Create(r).Error
}
func (s gormStore) Encashment(ctx context.Context, db *gorm.DB, t, id uuid.UUID, lock bool) (*Encashment, error) {
	return take[Encashment](scoped(ctx, db, t, id, lock))
}
func (s gormStore) PendingEncashment(ctx context.Context, db *gorm.DB, t, emp, typ uuid.UUID) (*Encashment, error) {
	return take[Encashment](db.WithContext(ctx).Where("tenant_id = ? AND employee_profile_id = ? AND leave_type_id = ? AND status = ?", t, emp, typ, StatusPending))
}
func (s gormStore) EncashedDays(ctx context.Context, db *gorm.DB, t, emp, typ uuid.UUID, year int, statuses ...string) (calc.Days, error) {
	var sum calc.Days
	row := db.WithContext(ctx).Model(&Encashment{}).Select("COALESCE(SUM(days), 0)").
		Where("tenant_id = ? AND employee_profile_id = ? AND leave_type_id = ? AND year = ? AND status IN ?", t, emp, typ, year, statuses).Row()
	return sum, row.Scan(&sum)
}
func (s gormStore) Encashments(ctx context.Context, db *gorm.DB, t uuid.UUID, f EncashFilter, limit int) ([]Encashment, error) {
	q := db.WithContext(ctx).Where("tenant_id = ?", t)
	if f.EmployeeID != nil {
		q = q.Where("employee_profile_id = ?", *f.EmployeeID)
	}
	if f.LeaveTypeID != nil {
		q = q.Where("leave_type_id = ?", *f.LeaveTypeID)
	}
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	var out []Encashment
	return out, q.Order("created_at DESC, id").Limit(limit).Find(&out).Error
}

func (s gormStore) Event(ctx context.Context, tx *gorm.DB, t uuid.UUID, key string, payload map[string]string) error {
	id := uuid.New()
	raw, err := json.Marshal(map[string]interface{}{"event_id": id, "event_type": key, "routing_key": key, "version": "1.0", "event_version": 1,
		"tenant_id": t, "occurred_at": time.Now().UTC(), "producer": "leave-service", "payload": payload})
	if err != nil {
		return err
	}
	return tx.WithContext(ctx).Exec(`INSERT INTO leave_events_outbox (id, tenant_id, event_id, event_type, routing_key, payload) VALUES (?, ?, ?, ?, ?, ?)`,
		uuid.New(), t, id, key, key, string(raw)).Error
}

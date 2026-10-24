package benefits

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ElectionFilter narrows election lists.
type ElectionFilter struct {
	EmployeeID *uuid.UUID
	PlanID     *uuid.UUID
	Status     string
}

// CountRow is how many elections of a plan, level and status are in force on a date (BN-C10).
type CountRow struct {
	PlanID    uuid.UUID
	LevelCode *string
	Status    string
	N         int
}

// Store is benefits data access (tenant-scoped; `tx` passed per call).
type Store interface {
	InTx(ctx context.Context, fn func(tx *gorm.DB) error) error
	DB() *gorm.DB
	// Lock takes a transaction-scoped advisory lock on key.
	Lock(ctx context.Context, tx *gorm.DB, key string) error
	Create(ctx context.Context, tx *gorm.DB, row interface{}) error
	Save(ctx context.Context, tx *gorm.DB, row interface{}) error

	Settings(ctx context.Context, db *gorm.DB, tenantID uuid.UUID) (*Settings, error)
	SaveSettings(ctx context.Context, tx *gorm.DB, row *Settings) error

	Plan(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID, lock bool) (*Plan, error)
	PlanCodeTaken(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, code string) (bool, error)
	Plans(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, status string, page, perPage int) ([]Plan, int64, error)
	ActivePlans(ctx context.Context, db *gorm.DB, tenantID uuid.UUID) ([]Plan, error)
	Versions(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, planIDs []uuid.UUID) ([]PlanVersion, error)

	Window(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID, lock bool) (*Window, error)
	Windows(ctx context.Context, db *gorm.DB, tenantID uuid.UUID) ([]Window, error)
	OpenWindowOn(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, on Date) (bool, error)

	Dependant(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID, lock bool) (*Dependant, error)
	Dependants(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID) ([]Dependant, error)
	LifeEvents(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID) ([]LifeEvent, error)
	// LifeEventSince is the latest life event dated in [since, upTo], if any.
	LifeEventSince(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID, since, upTo Date) (*LifeEvent, error)

	Election(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID, lock bool) (*Election, error)
	// LiveElection is the open-ended (active or waived) election of the worker for the plan.
	LiveElection(ctx context.Context, db *gorm.DB, tenantID, employeeID, planID uuid.UUID, lock bool) (*Election, error)
	Elections(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, f ElectionFilter, page, perPage int) ([]Election, int64, error)
	// EmployeeElections lists all of the worker's elections, newest first.
	EmployeeElections(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID) ([]Election, error)
	// ElectionsOn lists the worker's non-waived elections in force on the date.
	ElectionsOn(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID, on Date) ([]Election, error)
	// EndForTermination ends the worker's open-ended active elections at `end` (exclusive), linked to the action, and
	// returns how many it ended; a waiver costs nothing and stays (BN-C08).
	EndForTermination(ctx context.Context, tx *gorm.DB, tenantID, employeeID uuid.UUID, end Date, actionID uuid.UUID) (int, error)
	// Reinstate reopens the elections that action ended and returns how many (BN-C08).
	Reinstate(ctx context.Context, tx *gorm.DB, tenantID, actionID uuid.UUID) (int, error)
	Counts(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, on Date) ([]CountRow, error)

	// Event stores an outbox event (ids and statuses only).
	Event(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, key string, payload map[string]string) error
}

type gormStore struct{ db *gorm.DB }

// NewStore builds the gorm-backed Store.
func NewStore(db *gorm.DB) Store { return gormStore{db: db} }

func (s gormStore) InTx(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return s.db.WithContext(ctx).Transaction(fn)
}
func (s gormStore) DB() *gorm.DB { return s.db }
func (s gormStore) Lock(ctx context.Context, tx *gorm.DB, key string) error {
	return tx.WithContext(ctx).Exec("SELECT pg_advisory_xact_lock(hashtextextended(?, 0))", "ben:"+key).Error
}
func (s gormStore) Create(ctx context.Context, tx *gorm.DB, row interface{}) error {
	return tx.WithContext(ctx).Create(row).Error
}
func (s gormStore) Save(ctx context.Context, tx *gorm.DB, row interface{}) error {
	return tx.WithContext(ctx).Save(row).Error
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

func paginate[T any](q *gorm.DB, order string, page, perPage int) ([]T, int64, error) {
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var out []T
	return out, total, q.Order(order).Offset((page - 1) * perPage).Limit(perPage).Find(&out).Error
}

func (s gormStore) Settings(ctx context.Context, db *gorm.DB, t uuid.UUID) (*Settings, error) {
	return take[Settings](db.WithContext(ctx).Where("tenant_id = ?", t))
}
func (s gormStore) SaveSettings(ctx context.Context, tx *gorm.DB, row *Settings) error {
	return tx.WithContext(ctx).Clauses(clause.OnConflict{UpdateAll: true}).Create(row).Error
}

func (s gormStore) Plan(ctx context.Context, db *gorm.DB, t, id uuid.UUID, lock bool) (*Plan, error) {
	return take[Plan](scoped(ctx, db, t, id, lock))
}
func (s gormStore) PlanCodeTaken(ctx context.Context, db *gorm.DB, t uuid.UUID, code string) (bool, error) {
	var n int64
	err := db.WithContext(ctx).Model(&Plan{}).Where("tenant_id = ? AND lower(code) = lower(?)", t, code).Count(&n).Error
	return n > 0, err
}
func (s gormStore) Plans(ctx context.Context, db *gorm.DB, t uuid.UUID, status string, page, perPage int) ([]Plan, int64, error) {
	q := db.WithContext(ctx).Model(&Plan{}).Where("tenant_id = ?", t)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	return paginate[Plan](q, "plan_year DESC, code", page, perPage)
}
func (s gormStore) ActivePlans(ctx context.Context, db *gorm.DB, t uuid.UUID) ([]Plan, error) {
	var out []Plan
	return out, db.WithContext(ctx).Where("tenant_id = ? AND status = ?", t, PlanActive).Order("category, code").Find(&out).Error
}
func (s gormStore) Versions(ctx context.Context, db *gorm.DB, t uuid.UUID, ids []uuid.UUID) ([]PlanVersion, error) {
	var out []PlanVersion
	if len(ids) == 0 {
		return out, nil
	}
	return out, db.WithContext(ctx).Where("tenant_id = ? AND plan_id IN ?", t, ids).Order("plan_id, version").Find(&out).Error
}

func (s gormStore) Window(ctx context.Context, db *gorm.DB, t, id uuid.UUID, lock bool) (*Window, error) {
	return take[Window](scoped(ctx, db, t, id, lock))
}
func (s gormStore) Windows(ctx context.Context, db *gorm.DB, t uuid.UUID) ([]Window, error) {
	var out []Window
	return out, db.WithContext(ctx).Where("tenant_id = ?", t).Order("starts_on DESC, id").Find(&out).Error
}
func (s gormStore) OpenWindowOn(ctx context.Context, db *gorm.DB, t uuid.UUID, on Date) (bool, error) {
	var n int64
	err := db.WithContext(ctx).Model(&Window{}).Where("tenant_id = ? AND status = ? AND starts_on <= ? AND ends_on >= ?", t, WindowActive, on, on).Count(&n).Error
	return n > 0, err
}

func (s gormStore) Dependant(ctx context.Context, db *gorm.DB, t, id uuid.UUID, lock bool) (*Dependant, error) {
	return take[Dependant](scoped(ctx, db, t, id, lock))
}
func (s gormStore) Dependants(ctx context.Context, db *gorm.DB, t, e uuid.UUID) ([]Dependant, error) {
	var out []Dependant
	return out, db.WithContext(ctx).Where("tenant_id = ? AND employee_profile_id = ? AND status = ?", t, e, "active").Order("created_at, id").Find(&out).Error
}
func (s gormStore) LifeEvents(ctx context.Context, db *gorm.DB, t, e uuid.UUID) ([]LifeEvent, error) {
	var out []LifeEvent
	return out, db.WithContext(ctx).Where("tenant_id = ? AND employee_profile_id = ?", t, e).Order("event_date DESC, id").Find(&out).Error
}
func (s gormStore) LifeEventSince(ctx context.Context, db *gorm.DB, t, e uuid.UUID, since, upTo Date) (*LifeEvent, error) {
	return take[LifeEvent](db.WithContext(ctx).Where("tenant_id = ? AND employee_profile_id = ? AND event_date >= ? AND event_date <= ?", t, e, since, upTo).Order("event_date DESC"))
}

func (s gormStore) Election(ctx context.Context, db *gorm.DB, t, id uuid.UUID, lock bool) (*Election, error) {
	return take[Election](scoped(ctx, db, t, id, lock))
}
func (s gormStore) LiveElection(ctx context.Context, db *gorm.DB, t, e, p uuid.UUID, lock bool) (*Election, error) {
	q := db.WithContext(ctx)
	if lock {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	return take[Election](q.Where("tenant_id = ? AND employee_profile_id = ? AND plan_id = ? AND effective_to IS NULL", t, e, p))
}
func (s gormStore) Elections(ctx context.Context, db *gorm.DB, t uuid.UUID, f ElectionFilter, page, perPage int) ([]Election, int64, error) {
	q := db.WithContext(ctx).Model(&Election{}).Where("tenant_id = ?", t)
	if f.EmployeeID != nil {
		q = q.Where("employee_profile_id = ?", *f.EmployeeID)
	}
	if f.PlanID != nil {
		q = q.Where("plan_id = ?", *f.PlanID)
	}
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	return paginate[Election](q, "effective_from DESC, id", page, perPage)
}
func (s gormStore) EmployeeElections(ctx context.Context, db *gorm.DB, t, e uuid.UUID) ([]Election, error) {
	var out []Election
	return out, db.WithContext(ctx).Where("tenant_id = ? AND employee_profile_id = ?", t, e).Order("effective_from DESC, created_at DESC, id").Find(&out).Error
}
func (s gormStore) ElectionsOn(ctx context.Context, db *gorm.DB, t, e uuid.UUID, on Date) ([]Election, error) {
	var out []Election
	return out, db.WithContext(ctx).Where("tenant_id = ? AND employee_profile_id = ? AND status <> ? AND effective_from <= ? AND (effective_to IS NULL OR effective_to > ?)",
		t, e, ElWaived, on, on).Order("plan_id").Find(&out).Error
}
func (s gormStore) EndForTermination(ctx context.Context, tx *gorm.DB, t, e uuid.UUID, end Date, action uuid.UUID) (int, error) {
	now := time.Now().UTC()
	// An election that would start only after the end date never takes effect: it ends on its own start date (an empty range).
	res := tx.WithContext(ctx).Exec(`UPDATE ben_elections
		SET effective_to = GREATEST(?::date, effective_from), status = ?, end_reason = ?, end_action_id = ?, updated_at = ?
		WHERE tenant_id = ? AND employee_profile_id = ? AND status = ? AND effective_to IS NULL`,
		end, ElEnded, ReasonTermination, action, now, t, e, ElActive)
	return int(res.RowsAffected), res.Error
}
func (s gormStore) Reinstate(ctx context.Context, tx *gorm.DB, t, action uuid.UUID) (int, error) {
	res := tx.WithContext(ctx).Exec(`UPDATE ben_elections
		SET effective_to = NULL, status = ?, end_reason = NULL, end_action_id = NULL, updated_at = ?
		WHERE tenant_id = ? AND end_action_id = ? AND end_reason = ?`, ElActive, time.Now().UTC(), t, action, ReasonTermination)
	return int(res.RowsAffected), res.Error
}
func (s gormStore) Counts(ctx context.Context, db *gorm.DB, t uuid.UUID, on Date) ([]CountRow, error) {
	var out []CountRow
	return out, db.WithContext(ctx).Table("ben_elections").Select("plan_id, level_code, status, count(*) AS n").
		Where("tenant_id = ? AND effective_from <= ? AND (effective_to IS NULL OR effective_to > ?)", t, on, on).
		Group("plan_id, level_code, status").Order("plan_id, level_code").Scan(&out).Error
}

func (s gormStore) Event(ctx context.Context, tx *gorm.DB, t uuid.UUID, key string, payload map[string]string) error {
	id := uuid.New()
	raw, err := json.Marshal(map[string]interface{}{"event_id": id, "event_type": key, "routing_key": key, "event_version": 1, "tenant_id": t,
		"occurred_at": time.Now().UTC(), "producer": "benefits-service", "payload": payload})
	if err != nil {
		return err
	}
	return tx.WithContext(ctx).Exec(`INSERT INTO ben_events_outbox (id, tenant_id, event_id, routing_key, payload) VALUES (?, ?, ?, ?, ?)`,
		uuid.New(), t, id, key, string(raw)).Error
}

package compensation

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ChangeFilter narrows change lists.
type ChangeFilter struct {
	EmployeeID *uuid.UUID
	Status     string
}

// Store is compensation data access (tenant-scoped; `tx` passed per call).
type Store interface {
	InTx(ctx context.Context, fn func(tx *gorm.DB) error) error
	DB() *gorm.DB
	// Lock takes a transaction-scoped advisory lock on key.
	Lock(ctx context.Context, tx *gorm.DB, key string) error
	Create(ctx context.Context, tx *gorm.DB, row interface{}) error
	Save(ctx context.Context, tx *gorm.DB, row interface{}) error

	Ranges(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, gradeID *uuid.UUID) ([]PayRange, error)
	Records(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID) ([]Record, error)

	Change(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID, lock bool) (*Change, error)
	Changes(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, f ChangeFilter, page, perPage int) ([]Change, int64, error)

	Cycle(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID, lock bool) (*Cycle, error)
	Cycles(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, status string, page, perPage int) ([]Cycle, int64, error)
	Budgets(ctx context.Context, db *gorm.DB, tenantID, cycleID uuid.UUID) ([]Budget, error)
	ReplaceBudgets(ctx context.Context, tx *gorm.DB, tenantID, cycleID uuid.UUID, rows []Budget) error
	Proposals(ctx context.Context, db *gorm.DB, tenantID, cycleID uuid.UUID, manager *uuid.UUID) ([]Proposal, error)
	Proposal(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID, lock bool) (*Proposal, error)

	// Event stores an outbox event (ids, dates and reasons only).
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
	return tx.WithContext(ctx).Exec("SELECT pg_advisory_xact_lock(hashtextextended(?, 0))", "comp:"+key).Error
}
func (s gormStore) Create(ctx context.Context, tx *gorm.DB, row interface{}) error {
	return tx.WithContext(ctx).Create(row).Error
}
func (s gormStore) Save(ctx context.Context, tx *gorm.DB, row interface{}) error {
	return tx.WithContext(ctx).Save(row).Error
}

func (s gormStore) Ranges(ctx context.Context, db *gorm.DB, t uuid.UUID, grade *uuid.UUID) ([]PayRange, error) {
	q := db.WithContext(ctx).Where("tenant_id = ?", t)
	if grade != nil {
		q = q.Where("grade_id = ?", *grade)
	}
	var out []PayRange
	return out, q.Order("grade_id, effective_from DESC").Find(&out).Error
}
func (s gormStore) Records(ctx context.Context, db *gorm.DB, t, e uuid.UUID) ([]Record, error) {
	var out []Record
	return out, db.WithContext(ctx).Where("tenant_id = ? AND employee_profile_id = ?", t, e).Order("effective_from").Find(&out).Error
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

func (s gormStore) Change(ctx context.Context, db *gorm.DB, t, id uuid.UUID, lock bool) (*Change, error) {
	return take[Change](scoped(ctx, db, t, id, lock))
}
func (s gormStore) Changes(ctx context.Context, db *gorm.DB, t uuid.UUID, f ChangeFilter, page, perPage int) ([]Change, int64, error) {
	q := db.WithContext(ctx).Model(&Change{}).Where("tenant_id = ?", t)
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
	var out []Change
	return out, total, q.Order("created_at DESC, id").Offset((page - 1) * perPage).Limit(perPage).Find(&out).Error
}

func (s gormStore) Cycle(ctx context.Context, db *gorm.DB, t, id uuid.UUID, lock bool) (*Cycle, error) {
	return take[Cycle](scoped(ctx, db, t, id, lock))
}
func (s gormStore) Cycles(ctx context.Context, db *gorm.DB, t uuid.UUID, status string, page, perPage int) ([]Cycle, int64, error) {
	q := db.WithContext(ctx).Model(&Cycle{}).Where("tenant_id = ?", t)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var out []Cycle
	return out, total, q.Order("effective_date DESC, id").Offset((page - 1) * perPage).Limit(perPage).Find(&out).Error
}
func (s gormStore) Budgets(ctx context.Context, db *gorm.DB, t, c uuid.UUID) ([]Budget, error) {
	var out []Budget
	return out, db.WithContext(ctx).Where("tenant_id = ? AND cycle_id = ?", t, c).Order("department_id").Find(&out).Error
}
func (s gormStore) ReplaceBudgets(ctx context.Context, tx *gorm.DB, t, c uuid.UUID, rows []Budget) error {
	if err := tx.WithContext(ctx).Where("tenant_id = ? AND cycle_id = ?", t, c).Delete(&Budget{}).Error; err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	return tx.WithContext(ctx).Create(&rows).Error
}
func (s gormStore) Proposals(ctx context.Context, db *gorm.DB, t, c uuid.UUID, manager *uuid.UUID) ([]Proposal, error) {
	q := db.WithContext(ctx).Where("tenant_id = ? AND cycle_id = ?", t, c)
	if manager != nil {
		q = q.Where("manager_employee_id = ?", *manager)
	}
	var out []Proposal
	return out, q.Order("department_id, employee_profile_id").Find(&out).Error
}
func (s gormStore) Proposal(ctx context.Context, db *gorm.DB, t, id uuid.UUID, lock bool) (*Proposal, error) {
	return take[Proposal](scoped(ctx, db, t, id, lock))
}

func (s gormStore) Event(ctx context.Context, tx *gorm.DB, t uuid.UUID, key string, payload map[string]string) error {
	id := uuid.New()
	raw, err := json.Marshal(map[string]interface{}{"event_id": id, "event_type": key, "routing_key": key, "event_version": 1, "tenant_id": t,
		"occurred_at": time.Now().UTC(), "producer": "compensation-service", "payload": payload})
	if err != nil {
		return err
	}
	return tx.WithContext(ctx).Exec(`INSERT INTO comp_events_outbox (id, tenant_id, event_id, routing_key, payload) VALUES (?, ?, ?, ?, ?)`,
		uuid.New(), t, id, key, string(raw)).Error
}

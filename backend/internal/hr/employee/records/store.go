package records

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Worker is the slice of an employee profile records need.
type Worker struct {
	ID, UserID uuid.UUID
	Status     string
}

// RequestFilter narrows change-request lists.
type RequestFilter struct {
	EmployeeID *uuid.UUID
	Status     string
	Type       string
}

// Store is the records data access (tenant-scoped; `tx` passed per call).
type Store interface {
	InTx(ctx context.Context, fn func(tx *gorm.DB) error) error
	DB() *gorm.DB
	WorkerByUser(ctx context.Context, tenantID, userID uuid.UUID) (*Worker, error)
	Worker(ctx context.Context, tenantID, id uuid.UUID) (*Worker, error)

	Contacts(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID) ([]EmergencyContact, error)
	Educations(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID) ([]Education, error)
	Priors(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID) ([]PriorEmployment, error)
	Create(ctx context.Context, tx *gorm.DB, row interface{}) error
	Save(ctx context.Context, tx *gorm.DB, row interface{}) error
	Delete(ctx context.Context, tx *gorm.DB, row interface{}) error
	// ClearPrimary unsets the worker's primary emergency contact (FR-ER001).
	ClearPrimary(ctx context.Context, tx *gorm.DB, tenantID, employeeID uuid.UUID) error

	Request(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*ChangeRequest, error)
	LockRequest(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) (*ChangeRequest, error)
	OpenRequest(ctx context.Context, tx *gorm.DB, tenantID, employeeID uuid.UUID, typ string) (bool, error)
	Requests(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, f RequestFilter, page, perPage int) ([]ChangeRequest, int64, error)
	// Apply writes an approved change to the profile, contact or statutory record and the timeline (FR-CR004).
	Apply(ctx context.Context, tx *gorm.DB, tenantID, employeeID uuid.UUID, typ string, p Payload) error
	// Event stores an outbox event (ids and type only, FR-CR005).
	Event(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, key string, payload map[string]string) error
}

type gormStore struct{ db *gorm.DB }

// NewStore builds the gorm-backed Store.
func NewStore(db *gorm.DB) Store { return gormStore{db: db} }

func (s gormStore) InTx(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return s.db.WithContext(ctx).Transaction(fn)
}
func (s gormStore) DB() *gorm.DB { return s.db }

func (s gormStore) worker(ctx context.Context, where string, args ...interface{}) (*Worker, error) {
	var w Worker
	q := s.db.WithContext(ctx).Table("employee_profiles").Select("id, user_id, status").Where(where+" AND deleted_at IS NULL", args...).Limit(1).Scan(&w)
	if q.Error != nil || q.RowsAffected == 0 {
		return nil, q.Error
	}
	return &w, nil
}
func (s gormStore) WorkerByUser(ctx context.Context, tenantID, userID uuid.UUID) (*Worker, error) {
	return s.worker(ctx, "tenant_id = ? AND user_id = ?", tenantID, userID)
}
func (s gormStore) Worker(ctx context.Context, tenantID, id uuid.UUID) (*Worker, error) {
	return s.worker(ctx, "tenant_id = ? AND id = ?", tenantID, id)
}

func ofWorker[T any](ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID, order string) ([]T, error) {
	var out []T
	err := db.WithContext(ctx).Where("tenant_id = ? AND employee_profile_id = ?", tenantID, employeeID).Order(order).Find(&out).Error
	return out, err
}
func (s gormStore) Contacts(ctx context.Context, db *gorm.DB, t, e uuid.UUID) ([]EmergencyContact, error) {
	return ofWorker[EmergencyContact](ctx, db, t, e, "is_primary DESC, created_at, id")
}
func (s gormStore) Educations(ctx context.Context, db *gorm.DB, t, e uuid.UUID) ([]Education, error) {
	return ofWorker[Education](ctx, db, t, e, "end_year DESC NULLS FIRST, created_at, id")
}
func (s gormStore) Priors(ctx context.Context, db *gorm.DB, t, e uuid.UUID) ([]PriorEmployment, error) {
	return ofWorker[PriorEmployment](ctx, db, t, e, "start_date DESC, id")
}
func (s gormStore) Create(ctx context.Context, tx *gorm.DB, row interface{}) error {
	return tx.WithContext(ctx).Create(row).Error
}
func (s gormStore) Save(ctx context.Context, tx *gorm.DB, row interface{}) error {
	return tx.WithContext(ctx).Save(row).Error
}
func (s gormStore) Delete(ctx context.Context, tx *gorm.DB, row interface{}) error {
	return tx.WithContext(ctx).Delete(row).Error
}
func (s gormStore) ClearPrimary(ctx context.Context, tx *gorm.DB, t, e uuid.UUID) error {
	return tx.WithContext(ctx).Model(&EmergencyContact{}).Where("tenant_id = ? AND employee_profile_id = ? AND is_primary", t, e).
		Update("is_primary", false).Error
}

func firstRequest(q *gorm.DB) (*ChangeRequest, error) {
	var r ChangeRequest
	err := q.Take(&r).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &r, err
}
func (s gormStore) Request(ctx context.Context, db *gorm.DB, t, id uuid.UUID) (*ChangeRequest, error) {
	return firstRequest(db.WithContext(ctx).Where("tenant_id = ? AND id = ?", t, id))
}
func (s gormStore) LockRequest(ctx context.Context, tx *gorm.DB, t, id uuid.UUID) (*ChangeRequest, error) {
	return firstRequest(tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_id = ? AND id = ?", t, id))
}
func (s gormStore) OpenRequest(ctx context.Context, tx *gorm.DB, t, e uuid.UUID, typ string) (bool, error) {
	var n int64
	err := tx.WithContext(ctx).Model(&ChangeRequest{}).Where("tenant_id = ? AND employee_profile_id = ? AND type = ? AND status = ?",
		t, e, typ, StatusSubmitted).Count(&n).Error
	return n > 0, err
}
func (s gormStore) Requests(ctx context.Context, db *gorm.DB, t uuid.UUID, f RequestFilter, page, perPage int) ([]ChangeRequest, int64, error) {
	q := db.WithContext(ctx).Model(&ChangeRequest{}).Where("tenant_id = ?", t)
	if f.EmployeeID != nil {
		q = q.Where("employee_profile_id = ?", *f.EmployeeID)
	}
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	if f.Type != "" {
		q = q.Where("type = ?", f.Type)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var out []ChangeRequest
	err := q.Order("created_at DESC, id").Offset((page - 1) * perPage).Limit(perPage).Find(&out).Error
	return out, total, err
}

func (s gormStore) Apply(ctx context.Context, tx *gorm.DB, t, e uuid.UUID, typ string, p Payload) error {
	db := tx.WithContext(ctx)
	now := time.Now().UTC()
	var err error
	switch typ {
	case TypeName:
		up := map[string]interface{}{"first_name": *p.FirstName, "last_name": *p.LastName, "updated_at": now}
		if p.DisplayName != nil {
			up["display_name"] = *p.DisplayName
		}
		err = db.Table("employee_profiles").Where("tenant_id = ? AND id = ?", t, e).Updates(up).Error
	case TypeAddress:
		up := map[string]interface{}{"updated_at": now}
		if p.CurrentAddress != nil {
			up["current_address"] = *p.CurrentAddress
		}
		if p.PermanentAddress != nil {
			up["permanent_address"] = *p.PermanentAddress
		}
		err = upsert(db, "employee_contacts", t, e, up)
	case TypeBank:
		err = upsert(db, "employee_statutory", t, e, map[string]interface{}{"bank_name": *p.BankName, "bank_account_number": *p.BankAccountNumber,
			"bank_routing_swift": *p.BankRoutingSwift, "updated_at": now})
	}
	if err != nil {
		return err
	}
	return db.Exec(`INSERT INTO employee_timelines (id, tenant_id, employee_profile_id, event_type, effective_date, notes, metadata, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, '{}', ?, ?)`, uuid.New(), t, e, "personal_data_changed:"+typ, now, "Approved change request", now, now).Error
}

// upsert updates the worker's row of a 1:1 table or creates it.
func upsert(db *gorm.DB, table string, t, e uuid.UUID, up map[string]interface{}) error {
	q := db.Table(table).Where("tenant_id = ? AND employee_profile_id = ? AND deleted_at IS NULL", t, e).Updates(up)
	if q.Error != nil || q.RowsAffected > 0 {
		return q.Error
	}
	up["id"], up["tenant_id"], up["employee_profile_id"], up["created_at"] = uuid.New(), t, e, up["updated_at"]
	return db.Table(table).Create(up).Error
}

func (s gormStore) Event(ctx context.Context, tx *gorm.DB, t uuid.UUID, key string, payload map[string]string) error {
	raw, err := json.Marshal(map[string]interface{}{"event_id": uuid.New(), "event_type": key, "routing_key": key, "event_version": 1,
		"tenant_id": t, "occurred_at": time.Now().UTC(), "payload": payload})
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	return tx.WithContext(ctx).Exec(`INSERT INTO employee_events_outbox (id, tenant_id, event_id, event_type, routing_key, payload, published, retry_count, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, FALSE, 0, ?, ?)`, uuid.New(), t, uuid.New(), key, key, string(raw), now, now).Error
}

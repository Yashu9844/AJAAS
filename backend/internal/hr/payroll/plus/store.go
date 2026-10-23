package plus

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/hr/payroll/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Store is parity data access (tenant-scoped; `tx` passed per call).
type Store interface {
	InTx(ctx context.Context, fn func(tx *gorm.DB) error) error
	DB() *gorm.DB
	Create(ctx context.Context, tx *gorm.DB, row interface{}) error
	Save(ctx context.Context, tx *gorm.DB, row interface{}) error

	PTSlabs(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, state string) ([]models.PTSlab, error)
	AllPTSlabs(ctx context.Context, db *gorm.DB, tenantID uuid.UUID) ([]models.PTSlab, error)
	ReplacePTSlabs(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, state string, rows []models.PTSlab) error
	DeletePTSlabs(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, state string) (int64, error)

	Settlement(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID, lock bool) (*Settlement, error)
	OpenSettlement(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID) (*Settlement, error)
	Settlements(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, employeeID *uuid.UUID, status string, limit int) ([]Settlement, error)
	ReplaceLines(ctx context.Context, tx *gorm.DB, settlementID uuid.UUID, lines []SettlementLine) error
	Lines(ctx context.Context, db *gorm.DB, settlementID uuid.UUID) ([]SettlementLine, error)

	// Event stores an outbox event for the payroll relay (ids only).
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
	return tx.WithContext(ctx).Omit("Lines").Save(row).Error
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

func (s gormStore) PTSlabs(ctx context.Context, db *gorm.DB, t uuid.UUID, state string) ([]models.PTSlab, error) {
	var out []models.PTSlab
	return out, db.WithContext(ctx).Where("tenant_id = ? AND state_code = ?", t, strings.ToUpper(state)).Order("month NULLS FIRST, min_gross").Find(&out).Error
}
func (s gormStore) AllPTSlabs(ctx context.Context, db *gorm.DB, t uuid.UUID) ([]models.PTSlab, error) {
	var out []models.PTSlab
	return out, db.WithContext(ctx).Where("tenant_id = ?", t).Order("state_code, month NULLS FIRST, min_gross").Find(&out).Error
}
func (s gormStore) ReplacePTSlabs(ctx context.Context, tx *gorm.DB, t uuid.UUID, state string, rows []models.PTSlab) error {
	if err := tx.WithContext(ctx).Where("tenant_id = ? AND state_code = ?", t, state).Delete(&models.PTSlab{}).Error; err != nil {
		return err
	}
	return tx.WithContext(ctx).Create(&rows).Error
}
func (s gormStore) DeletePTSlabs(ctx context.Context, tx *gorm.DB, t uuid.UUID, state string) (int64, error) {
	res := tx.WithContext(ctx).Where("tenant_id = ? AND state_code = ?", t, state).Delete(&models.PTSlab{})
	return res.RowsAffected, res.Error
}

func (s gormStore) Settlement(ctx context.Context, db *gorm.DB, t, id uuid.UUID, lock bool) (*Settlement, error) {
	q := db.WithContext(ctx)
	if lock {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	return take[Settlement](q.Where("tenant_id = ? AND id = ?", t, id))
}
func (s gormStore) OpenSettlement(ctx context.Context, db *gorm.DB, t, emp uuid.UUID) (*Settlement, error) {
	return take[Settlement](db.WithContext(ctx).Where("tenant_id = ? AND employee_profile_id = ? AND status IN ?", t, emp, []string{StatusDraft, StatusApproved}))
}
func (s gormStore) Settlements(ctx context.Context, db *gorm.DB, t uuid.UUID, emp *uuid.UUID, status string, limit int) ([]Settlement, error) {
	q := db.WithContext(ctx).Where("tenant_id = ?", t)
	if emp != nil {
		q = q.Where("employee_profile_id = ?", *emp)
	}
	if status != "" {
		q = q.Where("status = ?", status)
	}
	var out []Settlement
	return out, q.Order("created_at DESC, id").Limit(limit).Find(&out).Error
}
func (s gormStore) ReplaceLines(ctx context.Context, tx *gorm.DB, id uuid.UUID, lines []SettlementLine) error {
	if err := tx.WithContext(ctx).Where("settlement_id = ?", id).Delete(&SettlementLine{}).Error; err != nil {
		return err
	}
	if len(lines) == 0 {
		return nil
	}
	return tx.WithContext(ctx).Create(&lines).Error
}
func (s gormStore) Lines(ctx context.Context, db *gorm.DB, id uuid.UUID) ([]SettlementLine, error) {
	var out []SettlementLine
	return out, db.WithContext(ctx).Where("settlement_id = ?", id).Order("position").Find(&out).Error
}

func (s gormStore) Event(ctx context.Context, tx *gorm.DB, t uuid.UUID, key string, payload map[string]string) error {
	id := uuid.New()
	raw, err := json.Marshal(map[string]interface{}{"event_id": id, "event_type": key, "routing_key": key, "version": "1.0", "event_version": 1,
		"tenant_id": t, "occurred_at": time.Now().UTC(), "producer": "payroll-service", "payload": payload})
	if err != nil {
		return err
	}
	return tx.WithContext(ctx).Exec(`INSERT INTO payroll_events_outbox (id, tenant_id, event_id, event_type, routing_key, payload) VALUES (?, ?, ?, ?, ?, ?)`,
		uuid.New(), t, id, key, key, string(raw)).Error
}

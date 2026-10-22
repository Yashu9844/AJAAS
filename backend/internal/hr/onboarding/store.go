package onboarding

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ChecklistFilter narrows checklist lists; Involving restricts to checklists where the employee is the worker or the
// manager (OB-09).
type ChecklistFilter struct {
	Kind, Status string
	EmployeeID   *uuid.UUID
	DepartmentID *uuid.UUID
	Involving    *uuid.UUID
}

// ProgressRow is one department's line of the progress report (OB-10).
type ProgressRow struct {
	DepartmentID *uuid.UUID `json:"department_id"`
	Open         int64      `json:"open_checklists"`
	Completed    int64      `json:"completed_checklists"`
	OpenTasks    int64      `json:"open_tasks"`
	OverdueTasks int64      `json:"overdue_tasks"`
}

// Store is onboarding data access (tenant-scoped; `tx` passed per call).
type Store interface {
	InTx(ctx context.Context, fn func(tx *gorm.DB) error) error
	DB() *gorm.DB
	// Lock takes a transaction-scoped advisory lock on key.
	Lock(ctx context.Context, tx *gorm.DB, key string) error
	Create(ctx context.Context, tx *gorm.DB, row interface{}) error
	Save(ctx context.Context, tx *gorm.DB, row interface{}) error

	Template(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID, lock bool) (*Template, error)
	Templates(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, kind, status string) ([]Template, error)
	TemplateTasks(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, templateIDs []uuid.UUID) ([]TemplateTask, error)
	ReplaceTemplateTasks(ctx context.Context, tx *gorm.DB, tenantID, templateID uuid.UUID, rows []TemplateTask) error

	Checklist(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID, lock bool) (*Checklist, error)
	ChecklistBySource(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, kind string, sourceID uuid.UUID) (*Checklist, error)
	OpenChecklist(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID, kind string) (*Checklist, error)
	LatestOffboarding(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID) (*Checklist, error)
	Checklists(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, f ChecklistFilter, page, perPage int) ([]Checklist, int64, error)

	Tasks(ctx context.Context, db *gorm.DB, tenantID, checklistID uuid.UUID) ([]Task, error)
	Task(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID, lock bool) (*Task, error)
	// AssignedTasks lists open-checklist tasks assigned to the employee, plus pool tasks when pool is true.
	AssignedTasks(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, employeeID *uuid.UUID, pool bool) ([]Task, error)

	Clearances(ctx context.Context, db *gorm.DB, tenantID, checklistID uuid.UUID) ([]Clearance, error)
	ExitInterview(ctx context.Context, db *gorm.DB, tenantID, checklistID uuid.UUID) (*ExitInterview, error)
	Progress(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, kind string, today time.Time) ([]ProgressRow, error)

	// Event stores an outbox event (ids, kinds and dates only).
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
	return tx.WithContext(ctx).Exec("SELECT pg_advisory_xact_lock(hashtextextended(?, 0))", "onb:"+key).Error
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

func (s gormStore) Template(ctx context.Context, db *gorm.DB, t, id uuid.UUID, lock bool) (*Template, error) {
	return take[Template](scoped(ctx, db, t, id, lock))
}
func (s gormStore) Templates(ctx context.Context, db *gorm.DB, t uuid.UUID, kind, status string) ([]Template, error) {
	q := db.WithContext(ctx).Where("tenant_id = ?", t)
	if kind != "" {
		q = q.Where("kind = ?", kind)
	}
	if status != "" {
		q = q.Where("status = ?", status)
	}
	var out []Template
	return out, q.Order("created_at, id").Find(&out).Error
}
func (s gormStore) TemplateTasks(ctx context.Context, db *gorm.DB, t uuid.UUID, ids []uuid.UUID) ([]TemplateTask, error) {
	var out []TemplateTask
	if len(ids) == 0 {
		return out, nil
	}
	return out, db.WithContext(ctx).Where("tenant_id = ? AND template_id IN ?", t, ids).Order("position").Find(&out).Error
}
func (s gormStore) ReplaceTemplateTasks(ctx context.Context, tx *gorm.DB, t, id uuid.UUID, rows []TemplateTask) error {
	if err := tx.WithContext(ctx).Where("tenant_id = ? AND template_id = ?", t, id).Delete(&TemplateTask{}).Error; err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	return tx.WithContext(ctx).Create(&rows).Error
}

func (s gormStore) Checklist(ctx context.Context, db *gorm.DB, t, id uuid.UUID, lock bool) (*Checklist, error) {
	return take[Checklist](scoped(ctx, db, t, id, lock))
}
func (s gormStore) ChecklistBySource(ctx context.Context, db *gorm.DB, t uuid.UUID, kind string, src uuid.UUID) (*Checklist, error) {
	return take[Checklist](db.WithContext(ctx).Where("tenant_id = ? AND kind = ? AND source_id = ?", t, kind, src))
}
func (s gormStore) OpenChecklist(ctx context.Context, db *gorm.DB, t, e uuid.UUID, kind string) (*Checklist, error) {
	return take[Checklist](db.WithContext(ctx).Where("tenant_id = ? AND employee_profile_id = ? AND kind = ? AND status = ?", t, e, kind, ChecklistOpen))
}
func (s gormStore) LatestOffboarding(ctx context.Context, db *gorm.DB, t, e uuid.UUID) (*Checklist, error) {
	return take[Checklist](db.WithContext(ctx).Where("tenant_id = ? AND employee_profile_id = ? AND kind = ? AND status <> ?", t, e,
		KindOffboarding, ChecklistCancelled).Order("anchor_date DESC, created_at DESC"))
}
func (s gormStore) Checklists(ctx context.Context, db *gorm.DB, t uuid.UUID, f ChecklistFilter, page, perPage int) ([]Checklist, int64, error) {
	q := db.WithContext(ctx).Model(&Checklist{}).Where("tenant_id = ?", t)
	if f.Kind != "" {
		q = q.Where("kind = ?", f.Kind)
	}
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	if f.EmployeeID != nil {
		q = q.Where("employee_profile_id = ?", *f.EmployeeID)
	}
	if f.DepartmentID != nil {
		q = q.Where("department_id = ?", *f.DepartmentID)
	}
	if f.Involving != nil {
		q = q.Where("(employee_profile_id = ? OR manager_employee_id = ?)", *f.Involving, *f.Involving)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var out []Checklist
	return out, total, q.Order("anchor_date DESC, id").Offset((page - 1) * perPage).Limit(perPage).Find(&out).Error
}

func (s gormStore) Tasks(ctx context.Context, db *gorm.DB, t, c uuid.UUID) ([]Task, error) {
	var out []Task
	return out, db.WithContext(ctx).Where("tenant_id = ? AND checklist_id = ?", t, c).Order("position").Find(&out).Error
}
func (s gormStore) Task(ctx context.Context, db *gorm.DB, t, id uuid.UUID, lock bool) (*Task, error) {
	return take[Task](scoped(ctx, db, t, id, lock))
}
func (s gormStore) AssignedTasks(ctx context.Context, db *gorm.DB, t uuid.UUID, e *uuid.UUID, pool bool) ([]Task, error) {
	q := db.WithContext(ctx).Table("onb_tasks AS t").Select("t.*").
		Joins("JOIN onb_checklists c ON c.id = t.checklist_id AND c.status = ?", ChecklistOpen).Where("t.tenant_id = ?", t)
	switch {
	case e != nil && pool:
		q = q.Where("(t.assignee_employee_id = ? OR t.assignee_employee_id IS NULL)", *e)
	case e != nil:
		q = q.Where("t.assignee_employee_id = ?", *e)
	case pool:
		q = q.Where("t.assignee_employee_id IS NULL")
	default:
		return []Task{}, nil
	}
	var out []Task
	return out, q.Order("CASE WHEN t.status = 'open' THEN 0 ELSE 1 END, t.due_date, t.position").Find(&out).Error
}

func (s gormStore) Clearances(ctx context.Context, db *gorm.DB, t, c uuid.UUID) ([]Clearance, error) {
	var out []Clearance
	return out, db.WithContext(ctx).Where("tenant_id = ? AND checklist_id = ?", t, c).
		Order("CASE area WHEN 'manager' THEN 0 WHEN 'hr' THEN 1 WHEN 'it' THEN 2 ELSE 3 END").Find(&out).Error
}
func (s gormStore) ExitInterview(ctx context.Context, db *gorm.DB, t, c uuid.UUID) (*ExitInterview, error) {
	return take[ExitInterview](db.WithContext(ctx).Where("tenant_id = ? AND checklist_id = ?", t, c))
}
func (s gormStore) Progress(ctx context.Context, db *gorm.DB, t uuid.UUID, kind string, today time.Time) ([]ProgressRow, error) {
	var out []ProgressRow
	err := db.WithContext(ctx).Raw(`SELECT c.department_id,
		COUNT(DISTINCT c.id) FILTER (WHERE c.status = 'open') AS open,
		COUNT(DISTINCT c.id) FILTER (WHERE c.status = 'completed') AS completed,
		COUNT(t.id) FILTER (WHERE c.status = 'open' AND t.status = 'open') AS open_tasks,
		COUNT(t.id) FILTER (WHERE c.status = 'open' AND t.status = 'open' AND t.due_date < ?) AS overdue_tasks
		FROM onb_checklists c LEFT JOIN onb_tasks t ON t.checklist_id = c.id
		WHERE c.tenant_id = ? AND c.kind = ? GROUP BY c.department_id ORDER BY c.department_id NULLS LAST`,
		today.Format(DateLayout), t, kind).Scan(&out).Error
	return out, err
}

func (s gormStore) Event(ctx context.Context, tx *gorm.DB, t uuid.UUID, key string, payload map[string]string) error {
	id := uuid.New()
	raw, err := json.Marshal(map[string]interface{}{"event_id": id, "event_type": key, "routing_key": key, "event_version": 1, "tenant_id": t,
		"occurred_at": time.Now().UTC(), "producer": "onboarding-service", "payload": payload})
	if err != nil {
		return err
	}
	return tx.WithContext(ctx).Exec(`INSERT INTO onb_events_outbox (id, tenant_id, event_id, routing_key, payload) VALUES (?, ?, ?, ?, ?)`,
		uuid.New(), t, id, key, string(raw)).Error
}

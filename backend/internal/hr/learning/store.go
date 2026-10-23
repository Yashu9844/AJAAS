package learning

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// EnrolFilter narrows enrolment lists. Today is the tenant's calendar date, used by the overdue filter.
type EnrolFilter struct {
	EmployeeID   *uuid.UUID
	EmployeeIDs  []uuid.UUID // non-nil restricts to these workers (team view); an empty slice matches nobody
	CourseID     *uuid.UUID
	AssignmentID *uuid.UUID
	Status       string
	Today        Date
}

// ComplianceFilter narrows the compliance report.
type ComplianceFilter struct {
	DepartmentID *uuid.UUID
	CourseID     *uuid.UUID
	Mandatory    *bool
	Today        Date
}

// ComplianceRow is one department and course of the compliance report (LN-C16).
type ComplianceRow struct {
	DepartmentID *uuid.UUID `json:"department_id,omitempty"`
	CourseID     uuid.UUID  `json:"course_id"`
	Total        int        `json:"total"`
	NotStarted   int        `json:"not_started"`
	InProgress   int        `json:"in_progress"`
	Completed    int        `json:"completed"`
	Waived       int        `json:"waived"`
	Overdue      int        `json:"overdue"`
}

// Store is learning data access (tenant-scoped; `tx` passed per call).
type Store interface {
	InTx(ctx context.Context, fn func(tx *gorm.DB) error) error
	DB() *gorm.DB
	// Lock takes a transaction-scoped advisory lock on key.
	Lock(ctx context.Context, tx *gorm.DB, key string) error
	// Savepoint runs fn in a savepoint of tx: an error undoes only fn's work and leaves tx usable.
	Savepoint(ctx context.Context, tx *gorm.DB, fn func(sp *gorm.DB) error) error
	Create(ctx context.Context, tx *gorm.DB, row interface{}) error
	Save(ctx context.Context, tx *gorm.DB, row interface{}) error

	Settings(ctx context.Context, db *gorm.DB, tenantID uuid.UUID) (*Settings, error)
	SaveSettings(ctx context.Context, tx *gorm.DB, row *Settings) error

	Course(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID, lock bool) (*Course, error)
	Courses(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, status string, page, perPage int) ([]Course, int64, error)
	CoursesByIDs(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, ids []uuid.UUID) ([]Course, error)
	Published(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, page, perPage int) ([]Course, int64, error)
	Versions(ctx context.Context, db *gorm.DB, tenantID, courseID uuid.UUID) ([]CourseVersion, error)

	Plan(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID, lock bool) (*Plan, error)
	Plans(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, status string, page, perPage int) ([]Plan, int64, error)
	PlanNameTaken(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, name string, except uuid.UUID) (bool, error)
	PlanItems(ctx context.Context, db *gorm.DB, tenantID, planID uuid.UUID) ([]PlanItem, error)
	ReplaceItems(ctx context.Context, tx *gorm.DB, tenantID, planID uuid.UUID, rows []PlanItem) error

	Assignment(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID, lock bool) (*Assignment, error)
	Assignments(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, status string, page, perPage int) ([]Assignment, int64, error)
	// RuleAssignments lists the tenant's active rule assignments (every kind but employee).
	RuleAssignments(ctx context.Context, db *gorm.DB, tenantID uuid.UUID) ([]Assignment, error)
	// DueRecurring lists active recurring assignments whose next cycle is on or before upTo; a nil tenant means all.
	DueRecurring(ctx context.Context, db *gorm.DB, tenantID *uuid.UUID, upTo Date) ([]Assignment, error)

	// InsertEnrolments inserts, skipping rows whose dedupe key exists, and reports how many were created.
	InsertEnrolments(ctx context.Context, tx *gorm.DB, rows []Enrolment) (int, error)
	Enrolment(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID, lock bool) (*Enrolment, error)
	Enrolments(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, f EnrolFilter, page, perPage int) ([]Enrolment, int64, error)
	EnrolmentsByIDs(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, ids []uuid.UUID) ([]Enrolment, error)
	// CourseEnrolments lists the worker's enrolments in the courses, newest cycle first, whatever their source.
	CourseEnrolments(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID, courseIDs []uuid.UUID) ([]Enrolment, error)
	LiveSelfEnrolment(ctx context.Context, db *gorm.DB, tenantID, employeeID, courseID uuid.UUID) (*Enrolment, error)
	Compliance(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, f ComplianceFilter) ([]ComplianceRow, error)

	Completion(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID, lock bool) (*Completion, error)
	// LiveCompletion is the enrolment's pending or verified completion, if any.
	LiveCompletion(ctx context.Context, db *gorm.DB, tenantID, enrolmentID uuid.UUID, lock bool) (*Completion, error)
	LiveCompletions(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, enrolmentIDs []uuid.UUID) ([]Completion, error)
	Pending(ctx context.Context, db *gorm.DB, tenantID uuid.UUID) ([]Completion, error)

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
	return tx.WithContext(ctx).Exec("SELECT pg_advisory_xact_lock(hashtextextended(?, 0))", "lrn:"+key).Error
}
func (s gormStore) Savepoint(ctx context.Context, tx *gorm.DB, fn func(sp *gorm.DB) error) error {
	return tx.WithContext(ctx).Transaction(fn)
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

func (s gormStore) Course(ctx context.Context, db *gorm.DB, t, id uuid.UUID, lock bool) (*Course, error) {
	return take[Course](scoped(ctx, db, t, id, lock))
}
func (s gormStore) Courses(ctx context.Context, db *gorm.DB, t uuid.UUID, status string, page, perPage int) ([]Course, int64, error) {
	q := db.WithContext(ctx).Model(&Course{}).Where("tenant_id = ?", t)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	return paginate[Course](q, "title, id", page, perPage)
}
func (s gormStore) CoursesByIDs(ctx context.Context, db *gorm.DB, t uuid.UUID, ids []uuid.UUID) ([]Course, error) {
	var out []Course
	if len(ids) == 0 {
		return out, nil
	}
	return out, db.WithContext(ctx).Where("tenant_id = ? AND id IN ?", t, ids).Find(&out).Error
}
func (s gormStore) Published(ctx context.Context, db *gorm.DB, t uuid.UUID, page, perPage int) ([]Course, int64, error) {
	return paginate[Course](db.WithContext(ctx).Model(&Course{}).Where("tenant_id = ? AND status = ?", t, CoursePublished), "title, id", page, perPage)
}
func (s gormStore) Versions(ctx context.Context, db *gorm.DB, t, c uuid.UUID) ([]CourseVersion, error) {
	var out []CourseVersion
	return out, db.WithContext(ctx).Where("tenant_id = ? AND course_id = ?", t, c).Order("version DESC").Find(&out).Error
}

func (s gormStore) Plan(ctx context.Context, db *gorm.DB, t, id uuid.UUID, lock bool) (*Plan, error) {
	return take[Plan](scoped(ctx, db, t, id, lock))
}
func (s gormStore) Plans(ctx context.Context, db *gorm.DB, t uuid.UUID, status string, page, perPage int) ([]Plan, int64, error) {
	q := db.WithContext(ctx).Model(&Plan{}).Where("tenant_id = ?", t)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	return paginate[Plan](q, "name, id", page, perPage)
}
func (s gormStore) PlanNameTaken(ctx context.Context, db *gorm.DB, t uuid.UUID, name string, except uuid.UUID) (bool, error) {
	var n int64
	err := db.WithContext(ctx).Model(&Plan{}).Where("tenant_id = ? AND lower(name) = lower(?) AND id <> ?", t, name, except).Count(&n).Error
	return n > 0, err
}
func (s gormStore) PlanItems(ctx context.Context, db *gorm.DB, t, p uuid.UUID) ([]PlanItem, error) {
	var out []PlanItem
	return out, db.WithContext(ctx).Where("tenant_id = ? AND plan_id = ?", t, p).Order("position").Find(&out).Error
}
func (s gormStore) ReplaceItems(ctx context.Context, tx *gorm.DB, t, p uuid.UUID, rows []PlanItem) error {
	if err := tx.WithContext(ctx).Where("tenant_id = ? AND plan_id = ?", t, p).Delete(&PlanItem{}).Error; err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	return tx.WithContext(ctx).Create(&rows).Error
}

func (s gormStore) Assignment(ctx context.Context, db *gorm.DB, t, id uuid.UUID, lock bool) (*Assignment, error) {
	return take[Assignment](scoped(ctx, db, t, id, lock))
}
func (s gormStore) Assignments(ctx context.Context, db *gorm.DB, t uuid.UUID, status string, page, perPage int) ([]Assignment, int64, error) {
	q := db.WithContext(ctx).Model(&Assignment{}).Where("tenant_id = ?", t)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	return paginate[Assignment](q, "created_at DESC, id", page, perPage)
}
func (s gormStore) RuleAssignments(ctx context.Context, db *gorm.DB, t uuid.UUID) ([]Assignment, error) {
	var out []Assignment
	return out, db.WithContext(ctx).Where("tenant_id = ? AND status = ? AND target_kind <> ?", t, AssignActive, TargetEmployee).Order("created_at, id").Find(&out).Error
}
func (s gormStore) DueRecurring(ctx context.Context, db *gorm.DB, t *uuid.UUID, upTo Date) ([]Assignment, error) {
	q := db.WithContext(ctx).Where("status = ? AND recurrence_days IS NOT NULL AND next_cycle_on <= ?", AssignActive, upTo)
	if t != nil {
		q = q.Where("tenant_id = ?", *t)
	}
	var out []Assignment
	return out, q.Order("next_cycle_on, id").Find(&out).Error
}

func (s gormStore) InsertEnrolments(ctx context.Context, tx *gorm.DB, rows []Enrolment) (int, error) {
	if len(rows) == 0 {
		return 0, nil
	}
	now := time.Now().UTC()
	for i := range rows {
		_ = rows[i].BeforeCreate(nil)
		rows[i].CreatedAt, rows[i].UpdatedAt = now, now
	}
	res := tx.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "tenant_id"}, {Name: "employee_profile_id"}, {Name: "dedupe_key"}}, DoNothing: true}).Create(&rows)
	return int(res.RowsAffected), res.Error
}
func (s gormStore) Enrolment(ctx context.Context, db *gorm.DB, t, id uuid.UUID, lock bool) (*Enrolment, error) {
	return take[Enrolment](scoped(ctx, db, t, id, lock))
}
func (s gormStore) Enrolments(ctx context.Context, db *gorm.DB, t uuid.UUID, f EnrolFilter, page, perPage int) ([]Enrolment, int64, error) {
	q := db.WithContext(ctx).Model(&Enrolment{}).Where("tenant_id = ?", t)
	if f.EmployeeID != nil {
		q = q.Where("employee_profile_id = ?", *f.EmployeeID)
	}
	if f.EmployeeIDs != nil {
		if len(f.EmployeeIDs) == 0 {
			return []Enrolment{}, 0, nil
		}
		q = q.Where("employee_profile_id IN ?", f.EmployeeIDs)
	}
	if f.CourseID != nil {
		q = q.Where("course_id = ?", *f.CourseID)
	}
	if f.AssignmentID != nil {
		q = q.Where("assignment_id = ?", *f.AssignmentID)
	}
	switch f.Status {
	case "":
	case FilterOverdue:
		q = q.Where("status IN ? AND due_date < ?", []string{EnrolNotStarted, EnrolInProgress}, f.Today)
	default:
		q = q.Where("status = ?", f.Status)
	}
	return paginate[Enrolment](q, "due_date NULLS LAST, created_at DESC, id", page, perPage)
}
func (s gormStore) EnrolmentsByIDs(ctx context.Context, db *gorm.DB, t uuid.UUID, ids []uuid.UUID) ([]Enrolment, error) {
	var out []Enrolment
	if len(ids) == 0 {
		return out, nil
	}
	return out, db.WithContext(ctx).Where("tenant_id = ? AND id IN ?", t, ids).Find(&out).Error
}
func (s gormStore) CourseEnrolments(ctx context.Context, db *gorm.DB, t, e uuid.UUID, courses []uuid.UUID) ([]Enrolment, error) {
	var out []Enrolment
	if len(courses) == 0 {
		return out, nil
	}
	return out, db.WithContext(ctx).Where("tenant_id = ? AND employee_profile_id = ? AND course_id IN ?", t, e, courses).Order("cycle DESC, created_at DESC, id").Find(&out).Error
}
func (s gormStore) LiveSelfEnrolment(ctx context.Context, db *gorm.DB, t, e, c uuid.UUID) (*Enrolment, error) {
	return take[Enrolment](db.WithContext(ctx).Where("tenant_id = ? AND employee_profile_id = ? AND course_id = ? AND source = ? AND status IN ?",
		t, e, c, SourceSelf, []string{EnrolNotStarted, EnrolInProgress}))
}
func (s gormStore) Compliance(ctx context.Context, db *gorm.DB, t uuid.UUID, f ComplianceFilter) ([]ComplianceRow, error) {
	q := db.WithContext(ctx).Table("lrn_enrolments").Where("tenant_id = ?", t).
		Select(`department_id, course_id, count(*) AS total,
			count(*) FILTER (WHERE status = 'not_started') AS not_started,
			count(*) FILTER (WHERE status = 'in_progress') AS in_progress,
			count(*) FILTER (WHERE status = 'completed') AS completed,
			count(*) FILTER (WHERE status = 'waived') AS waived,
			count(*) FILTER (WHERE status IN ('not_started', 'in_progress') AND due_date < ?) AS overdue`, f.Today)
	if f.DepartmentID != nil {
		q = q.Where("department_id = ?", *f.DepartmentID)
	}
	if f.CourseID != nil {
		q = q.Where("course_id = ?", *f.CourseID)
	}
	if f.Mandatory != nil {
		q = q.Where("mandatory = ?", *f.Mandatory)
	}
	var out []ComplianceRow
	return out, q.Group("department_id, course_id").Order("department_id NULLS LAST, course_id").Scan(&out).Error
}

func (s gormStore) Completion(ctx context.Context, db *gorm.DB, t, id uuid.UUID, lock bool) (*Completion, error) {
	return take[Completion](scoped(ctx, db, t, id, lock))
}
func (s gormStore) LiveCompletion(ctx context.Context, db *gorm.DB, t, e uuid.UUID, lock bool) (*Completion, error) {
	q := db.WithContext(ctx)
	if lock {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	return take[Completion](q.Where("tenant_id = ? AND enrolment_id = ? AND status IN ?", t, e, []string{CompPending, CompVerified}))
}
func (s gormStore) LiveCompletions(ctx context.Context, db *gorm.DB, t uuid.UUID, ids []uuid.UUID) ([]Completion, error) {
	var out []Completion
	if len(ids) == 0 {
		return out, nil
	}
	return out, db.WithContext(ctx).Where("tenant_id = ? AND enrolment_id IN ? AND status IN ?", t, ids, []string{CompPending, CompVerified}).Find(&out).Error
}
func (s gormStore) Pending(ctx context.Context, db *gorm.DB, t uuid.UUID) ([]Completion, error) {
	var out []Completion
	return out, db.WithContext(ctx).Where("tenant_id = ? AND status = ?", t, CompPending).Order("created_at, id").Find(&out).Error
}

func (s gormStore) Event(ctx context.Context, tx *gorm.DB, t uuid.UUID, key string, payload map[string]string) error {
	id := uuid.New()
	raw, err := json.Marshal(map[string]interface{}{"event_id": id, "event_type": key, "routing_key": key, "event_version": 1, "tenant_id": t,
		"occurred_at": time.Now().UTC(), "producer": "learning-service", "payload": payload})
	if err != nil {
		return err
	}
	return tx.WithContext(ctx).Exec(`INSERT INTO lrn_events_outbox (id, tenant_id, event_id, routing_key, payload) VALUES (?, ?, ?, ?, ?)`,
		uuid.New(), t, id, key, string(raw)).Error
}

package services

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	employeeDTO "github.com/jaas/jaas/internal/hr/employee/dto"
	"github.com/jaas/jaas/internal/hr/leave/calc"
	"github.com/jaas/jaas/internal/hr/leave/events"
	"github.com/jaas/jaas/internal/hr/leave/models"
	"github.com/jaas/jaas/internal/hr/leave/repositories"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"gorm.io/gorm"
)

// workingStatuses may apply for leave (LV-001).
var workingStatuses = map[string]bool{"active": true, "probation": true, "notice": true}

// base carries Deps plus helpers shared by all services.
type base struct{ Deps }

func (b base) today() time.Time { return calc.DateOf(b.Now()) }

// employeeFor resolves the caller's profile (LV-001); requireWorking enforces status.
func (b base) employeeFor(ctx context.Context, a Actor, requireWorking bool) (*employeeDTO.EmployeeResponse, error) {
	emp, err := b.Employees.GetByUserID(ctx, a.TenantID, a.UserID)
	return checkEmployee(emp, err, requireWorking)
}

// employeeByID resolves a profile by id in the caller's tenant (B2/B3/B4).
func (b base) employeeByID(ctx context.Context, tenantID uuid.UUID, raw string) (*employeeDTO.EmployeeResponse, error) {
	id, err := parseUUIDField("employee_id", raw)
	if err != nil {
		return nil, err
	}
	emp, err := b.Employees.GetByID(ctx, tenantID, id)
	return checkEmployee(emp, err, false)
}

func checkEmployee(emp *employeeDTO.EmployeeResponse, err error, requireWorking bool) (*employeeDTO.EmployeeResponse, error) {
	if errors.Is(err, sharedErrors.ErrNotFound) || (err == nil && emp == nil) {
		return nil, ErrEmployeeNotFound
	}
	if err != nil {
		return nil, err
	}
	if requireWorking && !workingStatuses[emp.Status] {
		return nil, ErrEmployeeNotActive
	}
	return emp, nil
}

// joiningOf returns the employee's joining date, nil when unknown (A4-02).
func joiningOf(emp *employeeDTO.EmployeeResponse) *time.Time {
	if emp.Employment == nil || emp.Employment.JoiningDate.IsZero() {
		return nil
	}
	j := emp.Employment.JoiningDate
	return &j
}

// leaveYear labels the leave year of a date (LP-02).
func (b base) leaveYear(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, d time.Time) (int, error) {
	m, err := b.yearStartMonth(ctx, db, tenantID)
	if err != nil {
		return 0, err
	}
	return calc.YearOf(d, m), nil
}

// yearStartMonth is the tenant's leave-year start month (1 without a calendar source).
func (b base) yearStartMonth(ctx context.Context, db *gorm.DB, tenantID uuid.UUID) (int, error) {
	if b.Calendar == nil {
		return 1, nil
	}
	return b.Calendar.YearStartMonth(ctx, db, tenantID)
}

// calendarFor returns the blocked dates and weekly-off test for one employee's range (LP-05).
func (b base) calendarFor(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID, from, to time.Time) ([]time.Time, func(time.Time) bool, error) {
	holidays, err := b.blockingHolidays(ctx, db, tenantID, from, to)
	if err != nil || b.Calendar == nil {
		return holidays, nil, err
	}
	extra, weekOff, err := b.Calendar.For(ctx, db, tenantID, employeeID, from, to)
	return append(holidays, extra...), weekOff, err
}

// typeByID loads a leave type in the caller's tenant; unknown → 404.
func (b base) typeByID(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, raw string) (*models.LeaveType, error) {
	id, err := parseUUIDField("leave_type_id", raw)
	if err != nil {
		return nil, err
	}
	t, err := b.Repos.Types.FindByID(ctx, db, tenantID, id)
	if err == nil && t == nil {
		return nil, ErrNotFound
	}
	return t, err
}

// writeOutbox stores the event inside the business transaction (FR-EV002).
func (b base) writeOutbox(ctx context.Context, tx *gorm.DB, a Actor, routingKey string, payload interface{}) (*models.OutboxEvent, error) {
	env := events.New(routingKey, events.Source{TenantID: a.TenantID, CorrelationID: a.CorrelationID, OccurredAt: b.Now()}, payload)
	raw, err := json.Marshal(env)
	if err != nil {
		return nil, err
	}
	row := &models.OutboxEvent{TenantID: a.TenantID, EventID: env.EventID, EventType: env.EventType,
		RoutingKey: routingKey, Payload: string(raw)}
	return row, b.Repos.Outbox.Create(ctx, tx, row)
}

// auditEntry is one audit row: action, resource type, resource id, metadata (positional order).
type auditEntry struct {
	Action, Resource, ResourceID string
	Meta                         interface{}
}

// afterCommit audits and publishes best-effort; failures never surface (FR-EV002).
func (b base) afterCommit(ctx context.Context, a Actor, e auditEntry, rows []*models.OutboxEvent) {
	_ = b.Audit.Log(ctx, b.Tx.DB(), a.TenantID.String(), a.UserID.String(), e.Action, e.Resource, e.ResourceID, e.Meta, a.IP, a.UserAgent)
	for _, row := range rows {
		if row == nil {
			continue
		}
		if err := b.Publisher.Publish(ctx, events.Exchange, row.RoutingKey, json.RawMessage(row.Payload)); err == nil {
			_ = b.Repos.Outbox.MarkPublished(ctx, b.Tx.DB(), row.ID, b.Now())
		}
	}
}

func parseUUIDField(field, raw string) (uuid.UUID, error) {
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, invalid(field, "must be a UUID")
	}
	return id, nil
}

// keyFor builds the balance key for an employee/type/year.
func keyFor(tenantID, employeeID uuid.UUID, typ *models.LeaveType, year int) repositories.BalanceKey {
	return repositories.BalanceKey{TenantID: tenantID, EmployeeID: employeeID, LeaveTypeID: typ.ID, Year: year}
}

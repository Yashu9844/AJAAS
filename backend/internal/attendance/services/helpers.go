package services

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/attendance/calc"
	"github.com/jaas/jaas/internal/attendance/events"
	"github.com/jaas/jaas/internal/attendance/models"
	"github.com/jaas/jaas/internal/attendance/validators"
	employeeDTO "github.com/jaas/jaas/internal/employee/dto"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"gorm.io/gorm"
)

// workingStatuses are employee statuses that may record attendance (AT-002).
var workingStatuses = map[string]bool{"active": true, "probation": true, "notice": true}

// base carries Deps plus helpers shared by all services.
type base struct{ Deps }

// utcDay truncates an instant to its UTC calendar date.
func utcDay(t time.Time) time.Time {
	u := t.UTC()
	return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
}

// employeeFor resolves the caller's employee profile (AT-002); requireWorking enforces status.
func (b base) employeeFor(ctx context.Context, a Actor, requireWorking bool) (*employeeDTO.EmployeeResponse, error) {
	emp, err := b.Employees.GetByUserID(ctx, a.TenantID, a.UserID)
	if err != nil {
		if errors.Is(err, sharedErrors.ErrNotFound) {
			return nil, ErrEmployeeNotFound
		}
		return nil, err
	}
	if emp == nil {
		return nil, ErrEmployeeNotFound
	}
	if requireWorking && !workingStatuses[emp.Status] {
		return nil, ErrEmployeeNotActive
	}
	return emp, nil
}

// specFor converts a stored shift to calc input; nil shift → nil spec (AT-008 defaults).
func specFor(s *models.Shift) *calc.ShiftSpec {
	if s == nil {
		return nil
	}
	loc, err := validators.LoadTimezone(s.Timezone)
	if err != nil {
		loc = time.UTC
	}
	return &calc.ShiftSpec{StartMinute: s.StartMinute, EndMinute: s.EndMinute, GraceMins: s.GracePeriodMins,
		BreakMins: s.BreakDurationMins, FullDayMinutes: s.FullDayMinutes, HalfDayMinutes: s.HalfDayMinutes, Location: loc}
}

// shiftOn returns the shift assigned to an employee on a date, or nil.
func (b base) shiftOn(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID, date time.Time) (*models.Shift, error) {
	a, err := b.Repos.Assignments.ActiveFor(ctx, db, tenantID, employeeID, date)
	if err != nil || a == nil {
		return nil, err
	}
	return b.Repos.Shifts.FindByID(ctx, db, tenantID, a.ShiftID)
}

// shiftByID loads a record's shift; nil id → nil shift.
func (b base) shiftByID(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, id *uuid.UUID) (*models.Shift, error) {
	if id == nil {
		return nil, nil
	}
	return b.Repos.Shifts.FindByID(ctx, db, tenantID, *id)
}

// resolveDay picks the attendance date and shift for an instant (AT-005): probe the UTC day,
// attribute with that shift's zone/night rule, and re-resolve if attribution moved the date.
func (b base) resolveDay(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID, at time.Time) (time.Time, *models.Shift, error) {
	probe := utcDay(at)
	shift, err := b.shiftOn(ctx, db, tenantID, employeeID, probe)
	if err != nil {
		return time.Time{}, nil, err
	}
	date := calc.AttendanceDate(at, specFor(shift))
	if date.Equal(probe) {
		return date, shift, nil
	}
	moved, err := b.shiftOn(ctx, db, tenantID, employeeID, date)
	if err != nil {
		return time.Time{}, nil, err
	}
	if moved == nil {
		return calc.AttendanceDate(at, nil), nil, nil
	}
	return calc.AttendanceDate(at, specFor(moved)), moved, nil
}

// recompute rebuilds a record's totals from its active punches (AT-006..AT-010).
func (b base) recompute(ctx context.Context, tx *gorm.DB, rec *models.AttendanceRecord, shift *models.Shift, now time.Time) error {
	ps, err := b.Repos.Punches.ActiveForRecord(ctx, tx, rec.TenantID, rec.ID)
	if err != nil {
		return err
	}
	in := make([]calc.Punch, len(ps))
	for i, p := range ps {
		in[i] = calc.Punch{Time: p.PunchTime, Type: p.PunchType}
	}
	t := calc.ComputeTotals(in, specFor(shift), rec.AttendanceDate, now)
	rec.FirstPunchIn, rec.LastPunchOut = t.FirstIn, t.LastOut
	rec.TotalWorkMinutes, rec.TotalBreakMinutes = t.WorkMinutes, t.BreakMinutes
	rec.LateMinutes, rec.OvertimeMinutes, rec.Status = t.LateMinutes, t.OvertimeMinutes, t.Status
	return b.Repos.Records.Update(ctx, tx, rec)
}

// writeOutbox stores the event inside the business transaction (FR-EV002, D3-06).
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

// afterCommit audits and publishes best-effort; failures never surface (D3-06, D3-07).
func (b base) afterCommit(ctx context.Context, a Actor, e auditEntry, row *models.OutboxEvent) {
	_ = b.Audit.Log(ctx, b.Tx.DB(), a.TenantID.String(), a.UserID.String(), e.Action, e.Resource, e.ResourceID, e.Meta, a.IP, a.UserAgent)
	if row == nil {
		return
	}
	if err := b.Publisher.Publish(ctx, events.Exchange, row.RoutingKey, json.RawMessage(row.Payload)); err == nil {
		_ = b.Repos.Outbox.MarkPublished(ctx, b.Tx.DB(), row.ID, b.Now())
	}
}

func parseUUIDField(field, raw string) (uuid.UUID, error) {
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, invalid(field, "must be a UUID")
	}
	return id, nil
}

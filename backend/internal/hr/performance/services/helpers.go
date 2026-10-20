package services

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	employeeDTO "github.com/jaas/jaas/internal/hr/employee/dto"
	"github.com/jaas/jaas/internal/hr/performance/events"
	"github.com/jaas/jaas/internal/hr/performance/models"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"gorm.io/gorm"
)

// workingStatuses may own goals and review (A7-04).
var workingStatuses = map[string]bool{"active": true, "probation": true, "notice": true}

// Text limits on free-form fields.
const (
	maxCycleTitle = 150
	maxGoalTitle  = 200
	maxLongText   = 5000
	maxNote       = 1000
	maxUnit       = 20
	checkInWindow = 50 // newest check-ins returned with a goal (FR-GL002)
	reviewsOfMine = 200
)

// base carries Deps plus helpers shared by all services.
type base struct{ Deps }

// employeeFor resolves the caller's own employee profile (self endpoints, NFR-SEC002).
func (b base) employeeFor(ctx context.Context, a Actor) (*employeeDTO.EmployeeResponse, error) {
	emp, err := b.Employees.GetByUserID(ctx, a.TenantID, a.UserID)
	if errors.Is(err, sharedErrors.ErrNotFound) || (err == nil && emp == nil) {
		return nil, ErrEmployeeNotFound
	}
	return emp, err
}

// optionalSelf is employeeFor where "no profile" is not an error (callers acting only through permissions).
func (b base) optionalSelf(ctx context.Context, a Actor) (*employeeDTO.EmployeeResponse, error) {
	emp, err := b.employeeFor(ctx, a)
	if errors.Is(err, ErrEmployeeNotFound) {
		return nil, nil
	}
	return emp, err
}

// employee loads an employee of the tenant; unknown → EMPLOYEE_NOT_FOUND.
func (b base) employee(ctx context.Context, tenantID, id uuid.UUID) (*employeeDTO.EmployeeResponse, error) {
	emp, err := b.Employees.GetByID(ctx, tenantID, id)
	if errors.Is(err, sharedErrors.ErrNotFound) || (err == nil && emp == nil) {
		return nil, ErrEmployeeNotFound
	}
	return emp, err
}

// managerEmployee resolves an employee's direct manager's employee profile (D7-03); nil when there is none or the
// manager has no profile (A7-02).
func (b base) managerEmployee(ctx context.Context, tenantID uuid.UUID, emp *employeeDTO.EmployeeResponse) (*employeeDTO.EmployeeResponse, error) {
	mgr, err := b.Managers.DirectManager(ctx, tenantID, emp.UserID)
	if err != nil || mgr == nil || *mgr == emp.UserID {
		return nil, err
	}
	m, err := b.Employees.GetByUserID(ctx, tenantID, *mgr)
	if errors.Is(err, sharedErrors.ErrNotFound) {
		return nil, nil
	}
	return m, err
}

// managesEmployee reports whether the caller is the direct manager of the employee (PM-008).
func (b base) managesEmployee(ctx context.Context, a Actor, employeeID uuid.UUID) (bool, error) {
	emp, err := b.Employees.GetByID(ctx, a.TenantID, employeeID)
	if errors.Is(err, sharedErrors.ErrNotFound) || (err == nil && emp == nil) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	mgr, err := b.Managers.DirectManager(ctx, a.TenantID, emp.UserID)
	return err == nil && mgr != nil && *mgr == a.UserID, err
}

// canAccessEmployee is PM-008 for an owner: the permission flag, the owner themself, or the owner's manager.
// Denials are reported as false (callers answer 404, D7-05).
func (b base) canAccessEmployee(ctx context.Context, a Actor, employeeID uuid.UUID, write bool) (bool, error) {
	if a.CanManage || (!write && a.CanRead) {
		return true, nil
	}
	self, err := b.optionalSelf(ctx, a)
	if err != nil {
		return false, err
	}
	if self != nil && self.ID == employeeID {
		return true, nil
	}
	return b.managesEmployee(ctx, a, employeeID)
}

// writeOutbox stores the event inside the business transaction (FR-EV001).
func (b base) writeOutbox(ctx context.Context, tx *gorm.DB, a Actor, key string, payload events.Payload) (*models.OutboxEvent, error) {
	env := events.New(key, events.Source{TenantID: a.TenantID, CorrelationID: a.CorrelationID, OccurredAt: b.Now()}, payload)
	raw, err := json.Marshal(env)
	if err != nil {
		return nil, err
	}
	row := &models.OutboxEvent{TenantID: a.TenantID, EventID: env.EventID, EventType: key, RoutingKey: key, Payload: string(raw)}
	return row, b.Repos.Outbox.Create(ctx, tx, row)
}

// auditEntry is one audit row; metadata holds ids and statuses only (PM-017).
type auditEntry struct {
	Action, Resource, ResourceID string
	Meta                         interface{}
}

// afterCommit writes the audit row and tries to publish the event; the relay retries what fails.
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

func parseOptionalUUID(field string, raw *string) (*uuid.UUID, error) {
	if raw == nil || *raw == "" {
		return nil, nil
	}
	id, err := parseUUIDField(field, *raw)
	return &id, err
}

// text trims s and checks 1..max characters.
func text(field, s string, max int) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" || utf8.RuneCountInString(s) > max {
		return "", invalid(field, "must be 1–"+strconv.Itoa(max)+" characters")
	}
	return s, nil
}

// optionalText trims an optional field; blank → nil; longer than max → 400.
func optionalText(field string, s *string, max int) (*string, error) {
	if s == nil {
		return nil, nil
	}
	t := strings.TrimSpace(*s)
	if t == "" {
		return nil, nil
	}
	if utf8.RuneCountInString(t) > max {
		return nil, invalid(field, "must be at most "+strconv.Itoa(max)+" characters")
	}
	return &t, nil
}

func orNotFound(err error) error {
	if err != nil {
		return err
	}
	return ErrNotFound
}

func uuidPtr(id uuid.UUID) *uuid.UUID { return &id }

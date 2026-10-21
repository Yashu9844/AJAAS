package services

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/hr/core/dto"
	"github.com/jaas/jaas/internal/hr/core/events"
	"github.com/jaas/jaas/internal/hr/core/models"
	employeeDTO "github.com/jaas/jaas/internal/hr/employee/dto"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"gorm.io/gorm"
)

// workingStatuses may hold an assignment and manage others.
var workingStatuses = map[string]bool{"active": true, "probation": true, "notice": true}

// codeFormat is CO-002.
var codeFormat = regexp.MustCompile(`^[A-Z0-9_-]{2,32}$`)

// everything is the internal page for bounded whole-collection reads.
var everything = dto.Page{Page: 1, PerPage: 100000}

// base carries Deps plus helpers shared by all services.
type base struct{ Deps }

// today is the current calendar date (UTC midnight).
func (b base) today() time.Time { return dateOf(b.Now()) }

func dateOf(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func parseDate(field, s string) (time.Time, error) {
	d, err := time.Parse(dto.DateLayout, strings.TrimSpace(s))
	if err != nil {
		return time.Time{}, invalid(field, "must be YYYY-MM-DD")
	}
	return d, nil
}

func parseOptionalDate(field string, s *string) (*time.Time, error) {
	if s == nil || strings.TrimSpace(*s) == "" {
		return nil, nil
	}
	d, err := parseDate(field, *s)
	return &d, err
}

// asOf parses an optional as_of query value; empty means today.
func (b base) asOf(raw string) (time.Time, error) {
	if strings.TrimSpace(raw) == "" {
		return b.today(), nil
	}
	return parseDate("as_of", raw)
}

func fmtDate(t time.Time) string { return t.Format(dto.DateLayout) }

func fmtDatePtr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := fmtDate(*t)
	return &s
}

func parseUUIDField(field, raw string) (uuid.UUID, error) {
	id, err := uuid.Parse(strings.TrimSpace(raw))
	if err != nil {
		return uuid.Nil, invalid(field, "must be a UUID")
	}
	return id, nil
}

func parseOptionalUUID(field string, raw *string) (*uuid.UUID, error) {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return nil, nil
	}
	id, err := parseUUIDField(field, *raw)
	return &id, err
}

// code normalizes and checks CO-002.
func code(raw string) (string, error) {
	c := strings.ToUpper(strings.TrimSpace(raw))
	if !codeFormat.MatchString(c) {
		return "", invalid("code", "must be 2–32 characters of A–Z, 0–9, '_' or '-'")
	}
	return c, nil
}

// text trims s and checks 1..max characters.
func text(field, s string, max int) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" || utf8.RuneCountInString(s) > max {
		return "", invalid(field, "is required and at most "+strconv.Itoa(max)+" characters")
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

// employee loads a worker of the tenant; unknown → EMPLOYEE_NOT_FOUND.
func (b base) employee(ctx context.Context, tenantID, id uuid.UUID) (*employeeDTO.EmployeeResponse, error) {
	emp, err := b.Employees.GetByID(ctx, tenantID, id)
	if errors.Is(err, sharedErrors.ErrNotFound) || (err == nil && emp == nil) {
		return nil, ErrEmployeeNotFound
	}
	return emp, err
}

// self resolves the caller's own worker (self endpoints).
func (b base) self(ctx context.Context, a Actor) (*employeeDTO.EmployeeResponse, error) {
	emp, err := b.Employees.GetByUserID(ctx, a.TenantID, a.UserID)
	if errors.Is(err, sharedErrors.ErrNotFound) || (err == nil && emp == nil) {
		return nil, ErrEmployeeNotFound
	}
	return emp, err
}

// writeOutbox stores the event inside the business transaction.
func (b base) writeOutbox(ctx context.Context, tx *gorm.DB, a Actor, key string, payload interface{}) (*models.OutboxEvent, error) {
	env := events.New(key, events.Source{TenantID: a.TenantID, CorrelationID: a.CorrelationID, OccurredAt: b.Now()}, payload)
	raw, err := json.Marshal(env)
	if err != nil {
		return nil, err
	}
	row := &models.OutboxEvent{TenantID: a.TenantID, EventID: env.EventID, EventType: key, RoutingKey: key, Payload: string(raw), CreatedAt: b.Now().UTC()}
	return row, b.Repo.Create(ctx, tx, row)
}

// auditEntry is one audit row; metadata holds ids, types, statuses and dates only (HR-009).
type auditEntry struct {
	Action, Resource, ResourceID string
	Meta                         interface{}
}

// afterCommit writes the audit row and tries to publish the events; the relay retries what fails.
func (b base) afterCommit(ctx context.Context, a Actor, e auditEntry, rows ...*models.OutboxEvent) {
	_ = b.Audit.Log(ctx, b.Tx.DB(), a.TenantID.String(), a.UserID.String(), e.Action, e.Resource, e.ResourceID, e.Meta, a.IP, a.UserAgent)
	for _, row := range rows {
		if row == nil {
			continue
		}
		if err := b.Publisher.Publish(ctx, events.Exchange, row.RoutingKey, json.RawMessage(row.Payload)); err == nil {
			_ = b.Repo.MarkPublished(ctx, b.Tx.DB(), row.ID, b.Now())
		}
	}
}

func orNotFound(err error) error {
	if err != nil {
		return err
	}
	return ErrNotFound
}

// conflictOK swallows the 409 an idempotent login change returns when the login is already in the target state.
func conflictOK(err error) error {
	var app *sharedErrors.AppError
	if errors.As(err, &app) && app.StatusCode == 409 {
		return nil
	}
	return err
}

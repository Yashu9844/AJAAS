package services

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	employeeDTO "github.com/jaas/jaas/internal/employee/dto"
	"github.com/jaas/jaas/internal/payroll/calc"
	"github.com/jaas/jaas/internal/payroll/events"
	"github.com/jaas/jaas/internal/payroll/models"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"gorm.io/gorm"
)

// base carries Deps plus helpers shared by all services.
type base struct{ Deps }

func (b base) today() time.Time {
	n := b.Now().UTC()
	return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, time.UTC)
}

// employeeFor resolves the caller's own profile (self endpoints, NFR-SEC002).
func (b base) employeeFor(ctx context.Context, a Actor) (*employeeDTO.EmployeeResponse, error) {
	emp, err := b.Employees.GetByUserID(ctx, a.TenantID, a.UserID)
	return checkEmployee(emp, err)
}

// employeeByID resolves a profile id in the caller's tenant.
func (b base) employeeByID(ctx context.Context, tenantID uuid.UUID, id uuid.UUID) (*employeeDTO.EmployeeResponse, error) {
	emp, err := b.Employees.GetByID(ctx, tenantID, id)
	return checkEmployee(emp, err)
}

func checkEmployee(emp *employeeDTO.EmployeeResponse, err error) (*employeeDTO.EmployeeResponse, error) {
	if errors.Is(err, sharedErrors.ErrNotFound) || (err == nil && emp == nil) {
		return nil, ErrEmployeeNotFound
	}
	return emp, err
}

// loadSpec loads a structure and its components as calc input.
func (b base) loadSpec(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Structure, calc.StructureSpec, error) {
	s, err := b.Repos.Structures.FindByID(ctx, db, tenantID, id)
	if err != nil || s == nil {
		return nil, calc.StructureSpec{}, orNotFound(err)
	}
	comps, err := b.Repos.Structures.Components(ctx, db, tenantID, id)
	if err != nil {
		return nil, calc.StructureSpec{}, err
	}
	return s, specFor(s, comps), nil
}

func specFor(s *models.Structure, comps []models.Component) calc.StructureSpec {
	spec := calc.StructureSpec{PF: s.PFEnabled, ESI: s.ESIEnabled, PT: s.PTEnabled, TDS: s.TDSEnabled}
	for _, c := range comps {
		spec.Components = append(spec.Components, calc.ComponentSpec{Code: c.Code, Name: c.Name, Kind: c.Kind, Calc: c.Calc, Value: c.Value, Taxable: c.Taxable})
	}
	return spec
}

// writeOutbox stores the event inside the business transaction (FR-EV001).
func (b base) writeOutbox(ctx context.Context, tx *gorm.DB, a Actor, key string, payload interface{}) (*models.OutboxEvent, error) {
	env := events.New(key, events.Source{TenantID: a.TenantID, CorrelationID: a.CorrelationID, OccurredAt: b.Now()}, payload)
	raw, err := json.Marshal(env)
	if err != nil {
		return nil, err
	}
	row := &models.OutboxEvent{TenantID: a.TenantID, EventID: env.EventID, EventType: key, RoutingKey: key, Payload: string(raw)}
	return row, b.Repos.Outbox.Create(ctx, tx, row)
}

// auditEntry is one audit row: action, resource type, resource id, metadata (no salary amounts, NFR-SEC003).
type auditEntry struct {
	Action, Resource, ResourceID string
	Meta                         interface{}
}

// afterCommit audits and publishes best-effort.
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

func orNotFound(err error) error {
	if err != nil {
		return err
	}
	return ErrNotFound
}

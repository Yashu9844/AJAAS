package services

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	employeeDTO "github.com/jaas/jaas/internal/employee/dto"
	"github.com/jaas/jaas/internal/recruitment/events"
	"github.com/jaas/jaas/internal/recruitment/models"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"gorm.io/gorm"
)

// workingStatuses may interview candidates (A6-01).
var workingStatuses = map[string]bool{"active": true, "probation": true, "notice": true}

// base carries Deps plus helpers shared by all services.
type base struct{ Deps }

func (b base) today() time.Time {
	n := b.Now().UTC()
	return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, time.UTC)
}

// employeeFor resolves the caller's own employee profile (interviewer self endpoints, NFR-SEC002).
func (b base) employeeFor(ctx context.Context, a Actor) (*employeeDTO.EmployeeResponse, error) {
	emp, err := b.Employees.GetByUserID(ctx, a.TenantID, a.UserID)
	if errors.Is(err, sharedErrors.ErrNotFound) || (err == nil && emp == nil) {
		return nil, ErrEmployeeNotFound
	}
	return emp, err
}

// stageChange records RC-003: candidate stage update + append-only event in the caller's transaction.
type stageChange struct {
	to   string
	note *string
}

func (b base) moveStage(ctx context.Context, tx *gorm.DB, a Actor, c *models.Candidate, ch stageChange) error {
	from := c.Stage
	c.Stage = ch.to
	if err := b.Repos.Candidates.Update(ctx, tx, c); err != nil {
		return err
	}
	return b.Repos.Candidates.AddEvent(ctx, tx, &models.StageEvent{TenantID: c.TenantID, CandidateID: c.ID, FromStage: strPtr(from),
		ToStage: ch.to, ActorUserID: a.UserID, Note: ch.note})
}

// lockCandidate loads the candidate FOR UPDATE in the caller's tenant; unknown → 404.
func (b base) lockCandidate(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, id uuid.UUID) (*models.Candidate, error) {
	c, err := b.Repos.Candidates.LockByID(ctx, tx, tenantID, id)
	if err != nil || c == nil {
		return nil, orNotFound(err)
	}
	return c, nil
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

// auditEntry is one audit row; metadata holds ids/stages only (RC-011).
type auditEntry struct {
	Action, Resource, ResourceID string
	Meta                         interface{}
}

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

func orNotFound(err error) error {
	if err != nil {
		return err
	}
	return ErrNotFound
}

func strPtr(s string) *string { return &s }

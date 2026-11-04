package content

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/meetings/api"
	"github.com/jaas/jaas/internal/meetings/kernel"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Decision is a first-class decision row, searchable and reportable (DE-1).
type Decision struct {
	ID           uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	TenantID     uuid.UUID  `gorm:"type:uuid" json:"-"`
	OccurrenceID uuid.UUID  `gorm:"type:uuid" json:"occurrence_id"`
	AgendaItemID *uuid.UUID `gorm:"type:uuid" json:"agenda_item_id"`
	Text         string     `json:"text"`
	Status       string     `json:"status"`
	DecidedBy    *uuid.UUID `gorm:"type:uuid" json:"decided_by"`
	DecidedAt    time.Time  `json:"decided_at"`
	RecordedBy   uuid.UUID  `gorm:"type:uuid" json:"recorded_by"`
	Version      int        `json:"version"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// TableName maps to meeting_decisions.
func (Decision) TableName() string { return "meeting_decisions" }

// DecisionInput records or edits a decision; on edit absent fields stay.
type DecisionInput struct {
	Text         *string    `json:"text"`
	AgendaItemID *uuid.UUID `json:"agenda_item_id"`
	Status       *string    `json:"status"`
	DecidedBy    *uuid.UUID `json:"decided_by"`
	Version      int        `json:"version"`
}

func (in DecisionInput) apply(tx *gorm.DB, d *Decision) error {
	if in.Text != nil {
		t, err := kernel.Text("text", *in.Text, 1, 2000)
		if err != nil {
			return err
		}
		d.Text = t
	}
	if in.Status != nil {
		if err := kernel.OneOf("status", *in.Status, "proposed", "approved", "superseded"); err != nil {
			return err
		}
		d.Status = *in.Status
	}
	if in.DecidedBy != nil {
		d.DecidedBy = in.DecidedBy
	}
	if in.AgendaItemID != nil {
		var n int64
		if err := tx.Model(&AgendaItem{}).Where("id = ? AND occurrence_id = ?", *in.AgendaItemID, d.OccurrenceID).Count(&n).Error; err != nil {
			return err
		}
		if n == 0 {
			return kernel.Invalid("agenda_item_id", "must be an agenda item of this occurrence")
		}
		d.AgendaItemID = in.AgendaItemID
	}
	return nil
}

// Decisions lists the decisions of an occurrence (content readers, auditors included).
func (s *Service) Decisions(ctx context.Context, a api.Actor, occurrenceID uuid.UUID) ([]Decision, error) {
	if _, err := s.occurrence(ctx, s.db, a, occurrenceID); err != nil {
		return nil, err
	}
	var rows []Decision
	err := s.db.WithContext(ctx).Where("occurrence_id = ?", occurrenceID).Order("decided_at, id").Find(&rows).Error
	return rows, err
}

// RecordDecision adds a decision (J4 step 4).
func (s *Service) RecordDecision(ctx context.Context, a api.Actor, occurrenceID uuid.UUID, in DecisionInput) (*Decision, error) {
	if in.Text == nil {
		return nil, kernel.Invalid("text", "is required")
	}
	d := &Decision{ID: uuid.New(), TenantID: a.TenantID, OccurrenceID: occurrenceID, Status: "approved", RecordedBy: a.UserID,
		DecidedAt: s.now().UTC(), Version: 1}
	err := s.inTx(ctx, func(tx *gorm.DB) error {
		sc, err := s.writable(ctx, tx, a, occurrenceID, api.RecordDecision)
		if err != nil {
			return err
		}
		if err := in.apply(tx, d); err != nil {
			return err
		}
		if err := tx.Create(d).Error; err != nil {
			return err
		}
		return emit(ctx, tx, sc, "decision.recorded", map[string]any{"decision_id": d.ID})
	})
	return d, err
}

// UpdateDecision edits a decision; superseding is a status change.
func (s *Service) UpdateDecision(ctx context.Context, a api.Actor, id uuid.UUID, in DecisionInput) (*Decision, error) {
	var d Decision
	err := s.inTx(ctx, func(tx *gorm.DB) error {
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND tenant_id = ?", id, a.TenantID).Take(&d).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return kernel.NotFound("decision")
		}
		if err != nil {
			return err
		}
		if _, err := s.writable(ctx, tx, a, d.OccurrenceID, api.RecordDecision); err != nil {
			return err
		}
		if err := kernel.CheckVersion(in.Version, d.Version); err != nil {
			return err
		}
		if err := in.apply(tx, &d); err != nil {
			return err
		}
		d.Version++
		return tx.Save(&d).Error
	})
	return &d, err
}

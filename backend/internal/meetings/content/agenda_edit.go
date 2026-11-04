package content

import (
	"context"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/meetings/api"
	"github.com/jaas/jaas/internal/meetings/kernel"
	"github.com/jaas/jaas/internal/shared/rank"
	"gorm.io/gorm"
)

// UpdateAgendaItem edits an item: organizers any item, attendees the items they proposed (MR-07).
func (s *Service) UpdateAgendaItem(ctx context.Context, a api.Actor, id uuid.UUID, in AgendaInput) (*AgendaItem, error) {
	var out *AgendaItem
	err := s.inTx(ctx, func(tx *gorm.DB) error {
		it, _, err := s.loadItem(ctx, tx, a, id)
		if err != nil {
			return err
		}
		if err := kernel.CheckVersion(in.Version, it.Version); err != nil {
			return err
		}
		if err := in.apply(it); err != nil {
			return err
		}
		it.Version++
		out = it
		return tx.Save(it).Error
	})
	return out, err
}

// DeleteAgendaItem removes an item; its notes and decisions keep a reference-free history.
func (s *Service) DeleteAgendaItem(ctx context.Context, a api.Actor, id uuid.UUID) error {
	return s.inTx(ctx, func(tx *gorm.DB) error {
		it, _, err := s.loadItem(ctx, tx, a, id)
		if err != nil {
			return err
		}
		return tx.Delete(&AgendaItem{}, "id = ?", it.ID).Error
	})
}

// MoveInput places an item after one neighbour and/or before another ("" ids = the ends).
type MoveInput struct {
	AfterID  *uuid.UUID `json:"after_id"`
	BeforeID *uuid.UUID `json:"before_id"`
}

// MoveAgendaItem reorders an item with one rank write (organizers only).
func (s *Service) MoveAgendaItem(ctx context.Context, a api.Actor, id uuid.UUID, in MoveInput) (*AgendaItem, error) {
	var out *AgendaItem
	err := s.inTx(ctx, func(tx *gorm.DB) error {
		it, sc, err := s.loadItem(ctx, tx, a, id)
		if err != nil {
			return err
		}
		if err := sc.acc.Require(api.ManageAgenda); err != nil {
			return err
		}
		lo, hi, err := neighbours(tx, it, in)
		if err != nil {
			return err
		}
		r := rank.Between(lo, hi)
		if r == "" {
			return kernel.Invalid("after_id", "must come before before_id")
		}
		it.Rank, it.Version = r, it.Version+1
		out = it
		return tx.Save(it).Error
	})
	return out, err
}

func neighbours(tx *gorm.DB, it *AgendaItem, in MoveInput) (string, string, error) {
	get := func(id *uuid.UUID) (string, error) {
		if id == nil {
			return "", nil
		}
		var r []string
		if err := tx.Model(&AgendaItem{}).Where("id = ? AND occurrence_id = ? AND id <> ?", *id, it.OccurrenceID, it.ID).Pluck("rank", &r).Error; err != nil {
			return "", err
		}
		if len(r) == 0 {
			return "", kernel.Invalid("after_id", "neighbours must be other items of the same occurrence")
		}
		return r[0], nil
	}
	lo, err := get(in.AfterID)
	if err != nil {
		return "", "", err
	}
	hi, err := get(in.BeforeID)
	if err != nil || in.AfterID == nil || in.BeforeID != nil {
		return lo, hi, err
	}
	// Only after_id given: place between it and the next item.
	var next []string
	err = tx.Model(&AgendaItem{}).Where("occurrence_id = ? AND rank > ? AND id <> ?", it.OccurrenceID, lo, it.ID).Order("rank").Limit(1).Pluck("rank", &next).Error
	if len(next) > 0 {
		hi = next[0]
	}
	return lo, hi, err
}

package content

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/meetings/api"
	"github.com/jaas/jaas/internal/meetings/kernel"
	"github.com/jaas/jaas/internal/shared/rank"
	"gorm.io/gorm"
)

// AgendaItem is one ordered agenda item of an occurrence (AG-1).
type AgendaItem struct {
	ID           uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	TenantID     uuid.UUID  `gorm:"type:uuid" json:"-"`
	OccurrenceID uuid.UUID  `gorm:"type:uuid" json:"occurrence_id"`
	Rank         string     `json:"rank"`
	Title        string     `json:"title"`
	BodyMD       string     `gorm:"column:body_md" json:"body"`
	OwnerID      *uuid.UUID `gorm:"type:uuid" json:"owner_id"`
	TimeboxMin   *int       `json:"timebox_min"`
	Kind         string     `json:"kind"`
	State        string     `json:"state"`
	CreatedBy    uuid.UUID  `gorm:"type:uuid" json:"created_by"`
	Version      int        `json:"version"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// TableName maps to meeting_agenda_items.
func (AgendaItem) TableName() string { return "meeting_agenda_items" }

// AgendaInput creates or edits an agenda item; on edit absent fields stay.
type AgendaInput struct {
	Title      *string    `json:"title"`
	Body       *string    `json:"body"`
	OwnerID    *uuid.UUID `json:"owner_id"`
	TimeboxMin *int       `json:"timebox_min"`
	Kind       *string    `json:"kind"`
	State      *string    `json:"state"`
	Version    int        `json:"version"`
}

const maxAgendaItems = 100

func (in AgendaInput) apply(it *AgendaItem) error {
	if in.Title != nil {
		t, err := kernel.Text("title", *in.Title, 1, 200)
		if err != nil {
			return err
		}
		it.Title = t
	}
	if in.Body != nil {
		b, err := kernel.Text("body", *in.Body, 0, 20000)
		if err != nil {
			return err
		}
		it.BodyMD = b
	}
	if in.TimeboxMin != nil && (*in.TimeboxMin < 1 || *in.TimeboxMin > 480) {
		return kernel.Invalid("timebox_min", "must be 1 to 480")
	}
	if in.TimeboxMin != nil {
		it.TimeboxMin = in.TimeboxMin
	}
	if in.OwnerID != nil {
		it.OwnerID = in.OwnerID
	}
	if in.Kind != nil {
		if err := kernel.OneOf("kind", *in.Kind, "info", "discussion", "decision"); err != nil {
			return err
		}
		it.Kind = *in.Kind
	}
	if in.State != nil {
		if err := kernel.OneOf("state", *in.State, "open", "discussed", "deferred"); err != nil {
			return err
		}
		it.State = *in.State
	}
	return nil
}

// Agenda lists the items in order.
func (s *Service) Agenda(ctx context.Context, a api.Actor, occurrenceID uuid.UUID) ([]AgendaItem, error) {
	if _, err := s.occurrence(ctx, s.db, a, occurrenceID); err != nil {
		return nil, err
	}
	var rows []AgendaItem
	err := s.db.WithContext(ctx).Where("occurrence_id = ?", occurrenceID).Order("rank, id").Find(&rows).Error
	return rows, err
}

// AddAgendaItem appends an item (AG-2: attendees propose topics until the occurrence is over or published).
func (s *Service) AddAgendaItem(ctx context.Context, a api.Actor, occurrenceID uuid.UUID, in AgendaInput) (*AgendaItem, error) {
	if in.Title == nil {
		return nil, kernel.Invalid("title", "is required")
	}
	it := &AgendaItem{ID: uuid.New(), TenantID: a.TenantID, OccurrenceID: occurrenceID, Kind: "discussion", State: "open", CreatedBy: a.UserID, Version: 1}
	if err := in.apply(it); err != nil {
		return nil, err
	}
	err := s.inTx(ctx, func(tx *gorm.DB) error {
		sc, err := s.writable(ctx, tx, a, occurrenceID, api.ProposeAgenda)
		if err != nil {
			return err
		}
		if err := agendaOpen(sc); err != nil {
			return err
		}
		var last struct {
			N    int64
			Rank string
		}
		if err := tx.Raw(`SELECT count(*) AS n, coalesce(max(rank), '') AS rank FROM meeting_agenda_items WHERE occurrence_id = ?`,
			occurrenceID).Scan(&last).Error; err != nil {
			return err
		}
		if last.N >= maxAgendaItems {
			return kernel.Unprocessable("AGENDA_LIMIT", "an occurrence has at most 100 agenda items")
		}
		it.Rank = rank.Between(last.Rank, "")
		if err := tx.Create(it).Error; err != nil {
			return err
		}
		return emit(ctx, tx, sc, "agenda_item.added", map[string]any{"agenda_item_id": it.ID})
	})
	return it, err
}

// agendaOpen refuses topics on a published occurrence, and on an ended one for callers who cannot manage the agenda.
func agendaOpen(sc scope) error {
	switch {
	case sc.occ.Status == "minutes_published":
		return kernel.Conflict("OCCURRENCE_HAS_MINUTES", "the minutes of this occurrence are published")
	case sc.occ.Status == "ended" && !sc.acc.Can(api.ManageAgenda):
		return kernel.Conflict("OCCURRENCE_ENDED", "the occurrence is over")
	}
	return nil
}

// loadItem resolves an agenda item and its occurrence for a write.
func (s *Service) loadItem(ctx context.Context, tx *gorm.DB, a api.Actor, id uuid.UUID) (*AgendaItem, scope, error) {
	var it AgendaItem
	err := tx.WithContext(ctx).Where("id = ? AND tenant_id = ?", id, a.TenantID).Take(&it).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, scope{}, kernel.NotFound("agenda item")
	}
	if err != nil {
		return nil, scope{}, err
	}
	sc, err := s.writable(ctx, tx, a, it.OccurrenceID, api.ProposeAgenda)
	if err != nil {
		return nil, scope{}, err
	}
	if !sc.acc.Can(api.ManageAgenda) && it.CreatedBy != a.UserID {
		return nil, scope{}, kernel.Forbidden(string(api.ManageAgenda))
	}
	return &it, sc, agendaOpen(sc)
}

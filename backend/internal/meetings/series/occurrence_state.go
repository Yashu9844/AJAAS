package series

import (
	"context"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/meetings/api"
	"github.com/jaas/jaas/internal/meetings/kernel"
	"gorm.io/gorm"
)

// CancelInput cancels an occurrence or a series.
type CancelInput struct {
	Reason  string `json:"reason"`
	Version int    `json:"version"`
}

// CancelOccurrence cancels one occurrence (RC-4, J2 step 2); its data stays.
func (s *Service) CancelOccurrence(ctx context.Context, a api.Actor, id uuid.UUID, in CancelInput) (*OccurrenceDetail, error) {
	reason, err := kernel.Text("reason", in.Reason, 0, 500)
	if err != nil {
		return nil, err
	}
	err = s.inTx(ctx, func(tx *gorm.DB) error {
		o, row, acc, err := s.editableOccurrence(ctx, tx, a, id, in.Version)
		if err != nil {
			return err
		}
		if err := acc.Require(api.Cancel); err != nil {
			return err
		}
		o.Status, o.CancelReason, o.IsException = StatusCancelled, reason, true
		if err := s.saveOccurrence(ctx, tx, o); err != nil {
			return err
		}
		return s.occurrenceChanged(ctx, tx, a, occChange{row: row, o: o, key: "occurrence.cancelled", reason: reason})
	})
	if err != nil {
		return nil, err
	}
	return s.GetOccurrence(ctx, a, id)
}

// StatusInput sets an occurrence status by hand; "scheduled" returns it to the clock (MR-05).
type StatusInput struct {
	Status  string `json:"status"`
	Version int    `json:"version"`
}

// SetStatus starts or ends an occurrence early (note-takers and organizers).
func (s *Service) SetStatus(ctx context.Context, a api.Actor, id uuid.UUID, in StatusInput) (*OccurrenceDetail, error) {
	if err := kernel.OneOf("status", in.Status, StatusScheduled, StatusInProgress, StatusEnded); err != nil {
		return nil, err
	}
	err := s.inTx(ctx, func(tx *gorm.DB) error {
		o, row, acc, err := s.editableOccurrence(ctx, tx, a, id, in.Version)
		if err != nil {
			return err
		}
		if !acc.Can(api.EditMinutes) && !acc.Can(api.Update) {
			return kernel.Forbidden(string(api.EditMinutes))
		}
		o.Status, o.StatusManual = in.Status, in.Status != StatusScheduled
		if err := s.saveOccurrence(ctx, tx, o); err != nil {
			return err
		}
		key := ""
		if in.Status == StatusEnded {
			key = "occurrence.ended"
		}
		return s.occurrenceChanged(ctx, tx, a, occChange{row: row, o: o, key: key, action: "occurrence.status." + in.Status})
	})
	if err != nil {
		return nil, err
	}
	return s.GetOccurrence(ctx, a, id)
}

// MarkPublished records that the minutes of an occurrence were published (called by the minutes context in its tx).
func (s *Service) MarkPublished(ctx context.Context, tx *gorm.DB, occurrenceID uuid.UUID) error {
	return tx.WithContext(ctx).Exec(`UPDATE meeting_occurrences SET status = ?, status_manual = FALSE, version = version + 1, updated_at = ?
		WHERE id = ?`, StatusPublished, s.now().UTC(), occurrenceID).Error
}

type occChange struct {
	row    *Series
	o      *Occurrence
	key    string // event routing key; "" emits nothing
	action string // change-log action; defaults to key
	reason string
}

func (s *Service) occurrenceChanged(ctx context.Context, tx *gorm.DB, a api.Actor, c occChange) error {
	action := c.action
	if action == "" {
		action = c.key
	}
	fields := map[string]any{"start_at": c.o.StartAt, "end_at": c.o.EndAt, "status": c.o.Status}
	if err := s.logChange(ctx, tx, a, change{series: c.row.ID, occurrence: &c.o.ID, action: action, fields: fields, reason: c.reason}); err != nil {
		return err
	}
	if err := s.audit(ctx, tx, a, "meeting."+action, c.row.ID, map[string]string{"occurrence_id": c.o.ID.String()}); err != nil {
		return err
	}
	if c.key == "" {
		return nil
	}
	return kernel.Emit(ctx, tx, a.TenantID, c.key, map[string]any{"series_id": c.row.ID, "occurrence_id": c.o.ID, "version": c.o.Version})
}

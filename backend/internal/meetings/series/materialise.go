package series

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/meetings/kernel"
	"github.com/jaas/jaas/internal/meetings/rrule"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// farFuture bounds the expansion of rules that end (COUNT <= 500 or UNTIL <= 5 years).
var farFuture = time.Date(9000, 1, 1, 0, 0, 0, 0, time.UTC)

// firstStart is the instant of the series start in its zone.
func firstStart(row *Series) (time.Time, *time.Location, error) {
	loc, err := time.LoadLocation(row.TZ)
	if err != nil {
		return time.Time{}, nil, err
	}
	return wall(row.DTStartLocal, loc), loc, nil
}

// expansion lists the start instants of a series up to windowEnd; complete is true when the rule ends before it.
func expansion(row *Series, windowEnd time.Time) (starts []time.Time, complete bool, err error) {
	first, _, err := firstStart(row)
	if err != nil {
		return nil, false, err
	}
	if row.RRule == nil {
		return []time.Time{first}, true, nil
	}
	r, err := rrule.Parse(*row.RRule)
	if err != nil {
		return nil, false, err
	}
	if r.Bounded() {
		return r.Expand(first, farFuture, maxInTxOccurrence), true, nil
	}
	return r.Expand(first, windowEnd, maxInTxOccurrence), false, nil
}

// windowEnd is twelve months after the later of now and the first start (DMT-03).
func (s *Service) windowEnd(row *Series) time.Time {
	base := s.now().UTC()
	if first, _, err := firstStart(row); err == nil && first.After(base) {
		base = first
	}
	return base.AddDate(0, WindowMonths, 0)
}

// materialise inserts the occurrences of row that start at or after from, idempotently (natural key series_id +
// recurrence_id), and moves the series window. It refuses to pass the tenant cap.
func (s *Service) materialise(ctx context.Context, tx *gorm.DB, row *Series, from time.Time) (int, error) {
	end := s.windowEnd(row)
	starts, complete, err := expansion(row, end)
	if err != nil {
		return 0, err
	}
	dur := time.Duration(row.DurationMin) * time.Minute
	var rows []Occurrence
	for _, t := range starts {
		if t.Before(from) {
			continue
		}
		rows = append(rows, Occurrence{ID: uuid.New(), TenantID: row.TenantID, SeriesID: row.ID, RecurrenceID: t.UTC(), StartAt: t.UTC(),
			EndAt: t.Add(dur).UTC(), Status: StatusScheduled, Version: 1})
	}
	if err := s.checkCap(ctx, tx, row.TenantID, len(rows)); err != nil {
		return 0, err
	}
	if len(rows) > 0 {
		res := tx.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(rows, 500)
		if res.Error != nil {
			return 0, res.Error
		}
	}
	var window *time.Time
	if !complete {
		window = &end
	}
	return len(rows), tx.WithContext(ctx).Model(&Series{}).Where("id = ?", row.ID).Update("window_until", window).Error
}

func (s *Service) checkCap(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, adding int) error {
	if adding == 0 {
		return nil
	}
	var n int64
	if err := tx.WithContext(ctx).Model(&Occurrence{}).Where("tenant_id = ?", tenantID).Count(&n).Error; err != nil {
		return err
	}
	if n+int64(adding) > tenantOccurrences {
		return kernel.Unprocessable("OCCURRENCE_LIMIT", "the tenant has too many scheduled occurrences")
	}
	return nil
}

// ExtendWindows moves the rolling window of open-ended series that end within a day of it; it is idempotent and safe
// to run on several instances at once (rows are claimed with SKIP LOCKED). It returns the number of rows added.
func (s *Service) ExtendWindows(ctx context.Context) (int, error) {
	total := 0
	for {
		n, claimed, err := s.extendBatch(ctx)
		total += n
		if err != nil || claimed < 100 {
			return total, err
		}
	}
}

func (s *Service) extendBatch(ctx context.Context) (added, claimed int, err error) {
	horizon := s.now().UTC().AddDate(0, WindowMonths, -1)
	err = s.inTx(ctx, func(tx *gorm.DB) error {
		var rows []Series
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("rrule IS NOT NULL AND window_until IS NOT NULL AND window_until < ? AND status = ? AND deleted_at IS NULL", horizon, SeriesActive).
			Order("window_until").Limit(100).Find(&rows).Error; e != nil {
			return e
		}
		claimed = len(rows)
		for i := range rows {
			n, e := s.materialise(ctx, tx, &rows[i], *rows[i].WindowUntil)
			if e != nil {
				return e
			}
			added += n
		}
		return nil
	})
	return added, claimed, err
}

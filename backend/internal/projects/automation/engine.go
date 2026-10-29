package automation

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

// Engine defaults: the loop interval is chosen by the caller (module.go uses 5 s).
const (
	cursorName  = "rules"
	batchSize   = 200
	leaseFor    = 2 * time.Minute
	pruneEvery  = time.Hour
	runRetain   = 30 * 24 * time.Hour
	settleDelay = 3 * time.Second
)

// Engine reads new rows of the M2 outbox, independently of the broker relay, and runs the rules they trigger. Rows are
// read only after a short settle delay so a transaction that committed late is not skipped by the cursor.
type Engine struct {
	s         *Service
	batch     int
	settle    time.Duration
	lastPrune time.Time
}

// NewEngine builds the engine on a rule service.
func NewEngine(s *Service) *Engine { return &Engine{s: s, batch: batchSize, settle: settleDelay} }

// SetSettle changes how long an outbox row must have existed before the engine reads it (tests use zero).
func (e *Engine) SetSettle(d time.Duration) { e.settle = d }

// Run ticks until ctx ends; a failed tick is reported and retried on the next one.
func (e *Engine) Run(ctx context.Context, every time.Duration, onErr func(error)) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if _, err := e.Tick(ctx); err != nil && onErr != nil {
				onErr(err)
			}
		}
	}
}

type position struct {
	At time.Time
	ID uuid.UUID
}

// Tick handles one batch of outbox events and returns how many it read. Only one instance works at a time (a lease on
// the cursor row); the cursor moves past the events that were handled, so a failure retries from the failed event.
func (e *Engine) Tick(ctx context.Context) (int, error) {
	now := e.s.now().UTC()
	pos, horizon, ok, err := e.lease(ctx)
	if err != nil || !ok {
		return 0, err
	}
	events, err := e.events(ctx, pos, horizon)
	if err != nil {
		return 0, errors.Join(err, e.release(ctx, pos))
	}
	done, herr := e.handleAll(ctx, events)
	next := pos
	switch {
	case herr != nil && done > 0:
		next = position{events[done-1].CreatedAt, events[done-1].ID}
	case herr == nil && len(events) == e.batch:
		next = position{events[len(events)-1].CreatedAt, events[len(events)-1].ID}
	case herr == nil:
		next = position{horizon, uuid.Nil}
	}
	return done, errors.Join(herr, e.release(ctx, next), e.prune(ctx, now))
}

// lease takes the single-reader lease and returns the cursor position and the read horizon (database time minus the
// settle delay); ok is false when another instance holds the lease.
func (e *Engine) lease(ctx context.Context) (position, time.Time, bool, error) {
	var rows []struct {
		LastCreatedAt time.Time
		LastID        uuid.UUID
		Horizon       time.Time
	}
	err := e.s.db.WithContext(ctx).Raw(`UPDATE automation_cursor SET lease_until = now() + make_interval(secs => ?)
		WHERE name = ? AND (lease_until IS NULL OR lease_until < now())
		RETURNING last_created_at, last_id, now() - make_interval(secs => ?) AS horizon`,
		leaseFor.Seconds(), cursorName, e.settle.Seconds()).Scan(&rows).Error
	if err != nil || len(rows) == 0 {
		return position{}, time.Time{}, false, err
	}
	return position{rows[0].LastCreatedAt, rows[0].LastID}, rows[0].Horizon, true, nil
}

// release stores the new position and frees the lease.
func (e *Engine) release(ctx context.Context, p position) error {
	return e.s.db.WithContext(ctx).Exec(`UPDATE automation_cursor SET last_created_at = ?, last_id = ?, lease_until = NULL, updated_at = now() WHERE name = ?`,
		p.At, p.ID, cursorName).Error
}

// prune drops run history older than 30 days, at most once an hour (storage minimisation).
func (e *Engine) prune(ctx context.Context, now time.Time) error {
	if now.Sub(e.lastPrune) < pruneEvery {
		return nil
	}
	e.lastPrune = now
	return e.s.db.WithContext(ctx).Exec(`DELETE FROM workflow_rule_runs WHERE created_at < ?`, now.Add(-runRetain)).Error
}

// Rewind moves the cursor to a position (tests and operators replaying history; runs stay idempotent per rule and event).
func (e *Engine) Rewind(ctx context.Context, at time.Time) error {
	return e.release(ctx, position{at, uuid.Nil})
}

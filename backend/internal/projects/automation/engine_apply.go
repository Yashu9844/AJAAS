package automation

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"gorm.io/gorm"
)

// outcome is how one rule run ended.
type outcome struct {
	status  string
	code    string
	actions int
}

// apply runs one rule for one event: permission check, conditions, then an idempotent claim and the actions. A rule whose
// conditions do not hold leaves no trace (the check is read-only, so a redelivered event is simply evaluated again).
func (e *Engine) apply(ctx context.Context, r Rule, ev event) error {
	actor := api.Actor{TenantID: r.TenantID, UserID: r.CreatedBy}
	allowed, err := e.s.creatorCan(ctx, r)
	if err != nil {
		return err
	}
	if allowed {
		match, err := e.matches(ctx, r, actor, ev)
		if err != nil || !match {
			return err
		}
	}
	runID, claimed, err := e.claim(ctx, r, ev)
	if err != nil || !claimed {
		return err
	}
	out := outcome{status: StatusSkipped, code: CodeNotAllowed}
	if allowed {
		out = e.execute(ctx, r, actor, ev)
	}
	return e.finalize(ctx, r, runID, out)
}

// matches reads the task as the creator and tests the conditions; a task the creator cannot read matches nothing.
func (e *Engine) matches(ctx context.Context, r Rule, actor api.Actor, ev event) (bool, error) {
	st, err := e.s.tasks.State(ctx, actor, ev.task)
	if isNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return st.ProjectID == r.ProjectID && len(r.Conditions.failing(st, ev.facts, false)) == 0, nil
}

func isNotFound(err error) bool {
	var app *sharedErrors.AppError
	return errors.As(err, &app) && app.StatusCode == 404
}

// creatorCan re-checks, at run time, that the rule creator is still an active member of the project and holds the
// permission every action needs (a custom role may have lost it since the rule was written).
func (s *Service) creatorCan(ctx context.Context, r Rule) (bool, error) {
	acc, err := s.authz.Access(ctx, nil, api.Actor{TenantID: r.TenantID, UserID: r.CreatedBy}, r.ProjectID)
	if isNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !acc.Member {
		return false, nil
	}
	for _, a := range r.Actions {
		if !acc.Can(api.Perm(a.needs())) {
			return false, nil
		}
	}
	return true, nil
}

// claim records the run before its actions so a redelivered event is applied once per rule; a crash between the claim
// and the result leaves the run failed with INTERRUPTED rather than repeating actions.
func (e *Engine) claim(ctx context.Context, r Rule, ev event) (uuid.UUID, bool, error) {
	id := uuid.New()
	task := ev.task
	res := e.s.db.WithContext(ctx).Exec(`INSERT INTO workflow_rule_runs (id, tenant_id, rule_id, event_id, task_id, status, error_code, created_at)
		VALUES (?, ?, ?, ?, ?, 'failed', ?, ?) ON CONFLICT (rule_id, event_id) DO NOTHING`,
		id, r.TenantID, r.ID, ev.row.EventID, &task, CodeInterrupted, e.s.now().UTC())
	return id, res.RowsAffected == 1, res.Error
}

// execute applies the flood cap and runs the actions in order, stopping at the first failure.
func (e *Engine) execute(ctx context.Context, r Rule, actor api.Actor, ev event) outcome {
	recent, err := e.recentActions(ctx, r)
	if err != nil {
		return outcome{status: StatusFailed, code: errCode(err)}
	}
	if recent+len(r.Actions) > MaxActionsPerMinute {
		return outcome{status: StatusSkipped, code: CodeRateLimited}
	}
	st, err := e.s.tasks.State(ctx, actor, ev.task)
	if err != nil {
		return outcome{status: StatusFailed, code: errCode(err)}
	}
	ctx = kernel.WithAgent(ctx, agentOf(r.ID))
	done := 0
	for _, a := range r.Actions {
		if !a.noop(st) {
			if err := e.s.runAction(ctx, actor, ev.task, a); err != nil {
				return outcome{status: StatusFailed, code: errCode(err), actions: done}
			}
			done++
		}
		st = a.applied(st)
	}
	return outcome{status: StatusOK, actions: done}
}

// recentActions counts what the rule did in the last minute.
func (e *Engine) recentActions(ctx context.Context, r Rule) (int, error) {
	var n int
	err := e.s.db.WithContext(ctx).Raw(`SELECT COALESCE(SUM(action_count), 0) FROM workflow_rule_runs WHERE rule_id = ? AND tenant_id = ? AND created_at > ?`,
		r.ID, r.TenantID, e.s.now().UTC().Add(-time.Minute)).Scan(&n).Error
	return n, err
}

// errCode is the stable code stored for a failed action.
func errCode(err error) string {
	if _, ok := kernel.AsVersionConflict(err); ok {
		return "VERSION_CONFLICT"
	}
	var app *sharedErrors.AppError
	if errors.As(err, &app) {
		return app.Code
	}
	return CodeActionFailed
}

// finalize stores the outcome of a claimed run and updates the rule counters, flags and the auto-disable (20 in a row).
func (e *Engine) finalize(ctx context.Context, r Rule, runID uuid.UUID, out outcome) error {
	failure := out.status == StatusFailed || out.code == CodeNotAllowed
	ran := 0
	if out.status != StatusSkipped {
		ran = 1
	}
	return e.s.inTx(ctx, func(tx *gorm.DB) error {
		var code *string
		if out.code != "" {
			code = &out.code
		}
		if err := tx.Exec(`UPDATE workflow_rule_runs SET status = ?, error_code = ?, action_count = ? WHERE id = ? AND tenant_id = ?`,
			out.status, code, out.actions, runID, r.TenantID).Error; err != nil {
			return err
		}
		if err := tx.Exec(`UPDATE workflow_rules SET run_count = run_count + ?, last_run_at = ?,
			consecutive_failures = CASE WHEN ?::boolean THEN consecutive_failures + 1 WHEN ?::text = 'ok' THEN 0 ELSE consecutive_failures END,
			flagged_code = CASE WHEN ?::text = 'ok' THEN NULL WHEN ?::text = 'NOT_ALLOWED' THEN 'NOT_ALLOWED' ELSE flagged_code END
			WHERE id = ? AND tenant_id = ?`, ran, e.s.now().UTC(), failure, out.status, out.status, out.code, r.ID, r.TenantID).Error; err != nil {
			return err
		}
		return tx.Exec(`UPDATE workflow_rules SET active = FALSE, flagged_code = ? WHERE id = ? AND tenant_id = ? AND active AND consecutive_failures >= ?`,
			FlagTooManyFailures, r.ID, r.TenantID, MaxConsecutiveFailures).Error
	})
}

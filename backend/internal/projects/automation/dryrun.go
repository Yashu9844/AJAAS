package automation

import (
	"context"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
)

// TestAction says what one action would do in a dry run.
type TestAction struct {
	Type     string `json:"type"`
	WouldRun bool   `json:"would_run"`
	Reason   string `json:"reason,omitempty"` // already_set, conditions_not_met or creator_not_allowed
}

// TestResult is the outcome of a dry run: nothing was changed and nothing was recorded.
type TestResult struct {
	Matches           bool         `json:"matches"`
	FailedConditions  []string     `json:"failed_conditions"`
	AssumedConditions []string     `json:"assumed_conditions"` // event conditions cannot be checked without an event
	CreatorAllowed    bool         `json:"creator_allowed"`
	Actions           []TestAction `json:"actions"`
}

// Test checks a rule against a task of the project without running it (project.update): which conditions fail, whether
// the creator may still act, and which actions would change something. Conditions on the triggering event are assumed true.
func (s *Service) Test(ctx context.Context, a api.Actor, projectID, ruleID, taskID uuid.UUID) (*TestResult, error) {
	if err := s.require(ctx, a, projectID, api.ProjectUpdate); err != nil {
		return nil, err
	}
	r, err := s.load(ctx, a.TenantID, projectID, ruleID)
	if err != nil {
		return nil, err
	}
	st, err := s.tasks.State(ctx, a, taskID)
	if err != nil {
		return nil, err
	}
	if st.ProjectID != projectID {
		return nil, kernel.NotFound("task")
	}
	allowed, err := s.creatorCan(ctx, *r)
	if err != nil {
		return nil, err
	}
	failed := r.Conditions.failing(st, eventFacts{}, true)
	res := &TestResult{Matches: len(failed) == 0, FailedConditions: failed, AssumedConditions: r.Conditions.assumed(), CreatorAllowed: allowed}
	if res.FailedConditions == nil {
		res.FailedConditions = []string{}
	}
	res.Actions = planActions(*r, st, res)
	return res, nil
}

// planActions walks the actions with the state each would see, as a real run does.
func planActions(r Rule, st TaskState, res *TestResult) []TestAction {
	out := make([]TestAction, 0, len(r.Actions))
	for _, act := range r.Actions {
		ta := TestAction{Type: act.Type, WouldRun: true}
		switch {
		case !res.Matches:
			ta.WouldRun, ta.Reason = false, "conditions_not_met"
		case !res.CreatorAllowed:
			ta.WouldRun, ta.Reason = false, "creator_not_allowed"
		case act.noop(st):
			ta.WouldRun, ta.Reason = false, "already_set"
		}
		st = act.applied(st)
		out = append(out, ta)
	}
	return out
}

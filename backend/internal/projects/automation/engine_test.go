package automation

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
	"github.com/jaas/jaas/internal/projects/project"
)

// movedRule is "when a task is moved and its priority is urgent, move it to Done".
func (e *env) movedRule(f fixture) *Rule {
	done := e.col(f.p, "done")
	return e.rule(f.owner, f.p.ID, RuleInput{Name: "Urgent to done", Trigger: "task.moved",
		Conditions: Conditions{{Field: FieldPriority, Value: "urgent"}}, Actions: Actions{{Type: ActionMove, ColumnID: &done}}})
}

func TestRuleMovesUrgentTaskAndDoesNotCascade(t *testing.T) {
	e := newEnv(t)
	f := e.fixture("EN")
	r := e.movedRule(f)
	urgent := e.task(f.p, "todo", "urgent", f.owner.UserID)
	normal := e.task(f.p, "todo", "low", f.owner.UserID)
	todo := e.col(f.p, "todo")

	for _, id := range []uuid.UUID{urgent, normal} { // a person moves both tasks
		e.emit("task.moved", id, f.p.ID, map[string]any{"from_column_id": e.col(f.p, "backlog"), "to_column_id": todo})
	}
	if n := e.tick(); n != 2 {
		t.Fatalf("two events read, got %d", n)
	}
	st, _ := e.tasks.State(bg, f.owner, urgent)
	if st.ColumnCategory != "done" {
		t.Fatalf("the urgent task should be done, is %s", st.ColumnCategory)
	}
	if st2, _ := e.tasks.State(bg, f.owner, normal); st2.ColumnCategory != "todo" {
		t.Fatal("conditions leave the other task alone")
	}
	if len(e.tasks.calls) != 1 || e.tasks.calls[0].agent != agentOf(r.ID) || e.tasks.calls[0].user != f.owner.UserID {
		t.Fatalf("one call as the creator with the automation label: %+v", e.tasks.calls)
	}
	if !strings.HasPrefix(e.tasks.calls[0].agent, "automation.") || !kernel.ValidAgent(e.tasks.calls[0].agent) {
		t.Fatalf("agent label %q", e.tasks.calls[0].agent)
	}
	// The rule's own task.moved event carries the label: further ticks must not react (no cascade).
	if e.tick() != 1 {
		t.Fatal("the engine reads the rule's own event")
	}
	e.tick()
	if e.tasks.count(ActionMove) != 1 || e.runs(r.ID, StatusOK) != 1 {
		t.Fatalf("no cascade: moves=%d ok runs=%d", e.tasks.count(ActionMove), e.runs(r.ID, StatusOK))
	}
	var got Rule
	e.db.Take(&got, "id = ?", r.ID)
	if got.RunCount != 1 || got.LastRunAt == nil || got.ConsecutiveFailures != 0 {
		t.Fatalf("counters: %+v", got)
	}
}

func TestSelfTriggeringRuleStopsAfterOneRun(t *testing.T) {
	e := newEnv(t)
	f := e.fixture("LP")
	todo, prog := e.col(f.p, "todo"), e.col(f.p, "in-progress")
	e.rule(f.owner, f.p.ID, RuleInput{Name: "Ping", Trigger: "task.moved", Actions: Actions{{Type: ActionMove, ColumnID: &prog}}})
	e.rule(f.owner, f.p.ID, RuleInput{Name: "Pong", Trigger: "task.moved", Actions: Actions{{Type: ActionMove, ColumnID: &todo}}})
	id := e.task(f.p, "backlog", "none", f.owner.UserID)
	e.emit("task.moved", id, f.p.ID, nil)
	for i := 0; i < 5; i++ {
		e.tick()
	}
	if n := e.tasks.count(ActionMove); n != 2 {
		t.Fatalf("each rule runs once and the cascade stops, got %d moves", n)
	}
}

func TestRedeliveredEventIsAppliedOnce(t *testing.T) {
	e := newEnv(t)
	f := e.fixture("ID")
	r := e.movedRule(f)
	id := e.task(f.p, "todo", "urgent", f.owner.UserID)
	e.emit("task.moved", id, f.p.ID, nil)
	e.tick()
	if err := e.eng.Rewind(bg, time.Now().Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	e.tick()
	e.tick()
	if e.runs(r.ID, StatusOK) != 1 || e.tasks.count(ActionMove) != 1 {
		t.Fatalf("idempotent per (rule, event): runs=%d moves=%d", e.runs(r.ID, StatusOK), e.tasks.count(ActionMove))
	}
}

func TestAllActionsRunInOrder(t *testing.T) {
	e := newEnv(t)
	f := e.fixture("AA")
	label, _ := e.proj.CreateLabel(bg, f.owner, &f.p.ID, project.LabelInput{Name: "auto"})
	prog := e.col(f.p, "in-progress")
	r := e.rule(f.owner, f.p.ID, RuleInput{Name: "All", Trigger: "task.created", Conditions: Conditions{{Field: FieldCategory, Value: "todo"}},
		Actions: Actions{{Type: ActionMove, ColumnID: &prog}, {Type: ActionPriority, Priority: "high"}, {Type: ActionLabel, LabelID: &label.ID},
			{Type: ActionAssign, UserID: &f.member.UserID}, {Type: ActionComment, Text: "Triaged"}, {Type: ActionDue, Days: ptr(2)},
			{Type: ActionPriority, Priority: "high"}}})
	id := e.task(f.p, "todo", "none", f.owner.UserID)
	e.emit("task.created", id, f.p.ID, nil)
	e.tick()
	for _, k := range []string{ActionMove, ActionPriority, ActionLabel, ActionAssign, ActionComment, ActionDue} {
		if e.tasks.count(k) != 1 {
			t.Errorf("%s ran %d times", k, e.tasks.count(k))
		}
	}
	var run Run
	e.db.Take(&run, "rule_id = ?", r.ID)
	if run.Status != StatusOK || run.ActionCount != 6 {
		t.Fatalf("the repeated priority is a no-op: %+v", run)
	}
}

func TestConditionsAreAllOf(t *testing.T) {
	e := newEnv(t)
	f := e.fixture("CO")
	label, _ := e.proj.CreateLabel(bg, f.owner, &f.p.ID, project.LabelInput{Name: "x"})
	todo := e.col(f.p, "todo")
	r := e.rule(f.owner, f.p.ID, RuleInput{Name: "Strict", Trigger: "task.assigned", Conditions: Conditions{
		{Field: FieldColumn, Value: todo.String()}, {Field: FieldLabel, Value: label.ID.String()},
		{Field: FieldAssignee, Value: f.member.UserID.String()}, {Field: FieldAssignedUser, Value: f.member.UserID.String()}},
		Actions: Actions{{Type: ActionPriority, Priority: "urgent"}}})
	id := e.task(f.p, "todo", "none", f.owner.UserID)
	assigned := map[string]any{"user_ids": []uuid.UUID{f.member.UserID}}
	e.emit("task.assigned", id, f.p.ID, assigned)
	e.tick()
	if e.tasks.count(ActionPriority) != 0 {
		t.Fatal("label and assignee missing: no run")
	}
	e.db.Exec(`INSERT INTO task_labels (task_id, label_id, tenant_id) VALUES (?, ?, ?)`, id, label.ID, e.tenant)
	e.db.Exec(`INSERT INTO task_assignees (task_id, user_id, tenant_id, assigned_by) VALUES (?, ?, ?, ?)`, id, f.member.UserID, e.tenant, f.owner.UserID)
	e.emit("task.assigned", id, f.p.ID, map[string]any{"user_ids": []uuid.UUID{f.viewr.UserID}})
	e.tick()
	if e.tasks.count(ActionPriority) != 0 {
		t.Fatal("the assigned user differs: no run")
	}
	e.emit("task.assigned", id, f.p.ID, assigned)
	e.tick()
	if e.tasks.count(ActionPriority) != 1 || e.runs(r.ID, StatusOK) != 1 {
		t.Fatalf("all conditions hold: calls=%d", e.tasks.count(ActionPriority))
	}
}

func TestMovedEventConditions(t *testing.T) {
	e := newEnv(t)
	f := e.fixture("ME")
	todo, done := e.col(f.p, "todo"), e.col(f.p, "done")
	e.rule(f.owner, f.p.ID, RuleInput{Name: "To done", Trigger: "task.moved", Conditions: Conditions{
		{Field: FieldToColumn, Value: done.String()}, {Field: FieldFromColumn, Value: todo.String()}},
		Actions: Actions{{Type: ActionPriority, Priority: "low"}}})
	id := e.task(f.p, "done", "high", f.owner.UserID)
	e.emit("task.moved", id, f.p.ID, map[string]any{"from_column_id": e.col(f.p, "backlog"), "to_column_id": done})
	e.tick()
	if e.tasks.count(ActionPriority) != 0 {
		t.Fatal("from column differs")
	}
	e.emit("task.moved", id, f.p.ID, map[string]any{"from_column_id": todo, "to_column_id": done})
	e.tick()
	if e.tasks.count(ActionPriority) != 1 {
		t.Fatal("event conditions hold")
	}
}

func TestNotAllowedCreatorIsSkippedAndFlagged(t *testing.T) {
	e := newEnv(t)
	f := e.fixture("NA")
	creator := f.member
	prog := e.col(f.p, "in-progress")
	// a member can set priority; a viewer cannot.
	r := e.rule(f.owner, f.p.ID, RuleInput{Name: "Needs update", Trigger: "task.created", Actions: Actions{{Type: ActionPriority, Priority: "high"}}})
	e.db.Exec(`UPDATE workflow_rules SET created_by = ? WHERE id = ?`, creator.UserID, r.ID)
	id := e.task(f.p, "todo", "none", f.owner.UserID)
	e.emit("task.created", id, f.p.ID, nil)
	e.tick()
	if e.tasks.count(ActionPriority) != 1 {
		t.Fatal("the member may act")
	}
	if _, err := e.proj.ChangeMember(bg, f.owner, f.p.ID, creator.UserID, project.MemberInput{UserID: creator.UserID, Role: api.RoleViewer}); err != nil {
		t.Fatal(err)
	}
	e.emit("task.created", id, f.p.ID, map[string]any{"column_id": prog})
	e.tick()
	var got Rule
	e.db.Take(&got, "id = ?", r.ID)
	var run Run
	e.db.Where("rule_id = ? AND status = 'skipped'", r.ID).Take(&run)
	if e.tasks.count(ActionPriority) != 1 || run.ErrorCode == nil || *run.ErrorCode != CodeNotAllowed {
		t.Fatalf("a demoted creator is skipped with NOT_ALLOWED: calls=%d run=%+v", e.tasks.count(ActionPriority), run)
	}
	if got.FlaggedCode == nil || *got.FlaggedCode != CodeNotAllowed || got.ConsecutiveFailures != 1 {
		t.Fatalf("the rule is flagged: %+v", got)
	}
	if _, err := e.proj.ChangeMember(bg, f.owner, f.p.ID, creator.UserID, project.MemberInput{UserID: creator.UserID, Role: api.RoleMember}); err != nil {
		t.Fatal(err)
	}
	e.emit("task.created", id, f.p.ID, map[string]any{"again": true})
	e.tick()
	e.db.Take(&got, "id = ?", r.ID)
	if got.FlaggedCode != nil || got.ConsecutiveFailures != 0 {
		t.Fatalf("a successful run clears the flag: %+v", got)
	}
}

func TestRemovedCreatorIsNotAllowed(t *testing.T) {
	e := newEnv(t)
	f := e.fixture("RM")
	r := e.rule(f.owner, f.p.ID, RuleInput{Name: "x", Trigger: "task.created", Actions: Actions{{Type: ActionPriority, Priority: "high"}}})
	e.db.Exec(`UPDATE workflow_rules SET created_by = ? WHERE id = ?`, uuid.New(), r.ID) // a login that never was a member
	id := e.task(f.p, "todo", "none", f.owner.UserID)
	e.emit("task.created", id, f.p.ID, nil)
	e.tick()
	if e.runs(r.ID, StatusSkipped) != 1 || e.tasks.count(ActionPriority) != 0 {
		t.Fatal("a non-member creator never acts")
	}
}

func TestRuleDisablesAfterTwentyFailures(t *testing.T) {
	e := newEnv(t)
	f := e.fixture("TF")
	r := e.rule(f.owner, f.p.ID, RuleInput{Name: "Fails", Trigger: "task.created", Actions: Actions{{Type: ActionPriority, Priority: "high"}}})
	e.tasks.fail[ActionPriority] = kernel.Conflict("WIP_LIMIT", "full")
	id := e.task(f.p, "todo", "none", f.owner.UserID)
	for i := 0; i < MaxConsecutiveFailures; i++ {
		e.emit("task.created", id, f.p.ID, map[string]any{"n": i})
	}
	e.tick()
	var got Rule
	e.db.Take(&got, "id = ?", r.ID)
	if got.Active || got.FlaggedCode == nil || *got.FlaggedCode != FlagTooManyFailures || got.ConsecutiveFailures != MaxConsecutiveFailures {
		t.Fatalf("disabled after 20 failures: %+v", got)
	}
	var run Run
	e.db.Where("rule_id = ?", r.ID).Take(&run)
	if run.Status != StatusFailed || run.ErrorCode == nil || *run.ErrorCode != "WIP_LIMIT" {
		t.Fatalf("failure code is kept: %+v", run)
	}
	e.emit("task.created", id, f.p.ID, nil)
	e.tick()
	if e.count(`SELECT count(*) FROM workflow_rule_runs WHERE rule_id = ?`, r.ID) != MaxConsecutiveFailures {
		t.Fatal("a disabled rule does not run")
	}
	on := true
	if upd, err := e.svc.UpdateRule(bg, f.owner, f.p.ID, r.ID, RuleUpdate{Active: &on}); err != nil || !upd.Active || upd.FlaggedCode != nil {
		t.Fatalf("re-enabling clears the flag: %v %+v", err, upd)
	}
}

func TestErrorCodes(t *testing.T) {
	if errCode(&kernel.VersionConflict{Current: 2}) != "VERSION_CONFLICT" || errCode(errors.New("boom")) != CodeActionFailed {
		t.Fatal("error codes")
	}
	if errCode(kernel.Forbidden("task.update")) != "FORBIDDEN" {
		t.Fatal("app error code")
	}
}

func TestFloodCapSkipsRun(t *testing.T) {
	e := newEnv(t)
	f := e.fixture("FL")
	r := e.rule(f.owner, f.p.ID, RuleInput{Name: "Busy", Trigger: "task.created", Actions: Actions{{Type: ActionPriority, Priority: "high"}}})
	e.db.Exec(`INSERT INTO workflow_rule_runs (id, tenant_id, rule_id, event_id, status, action_count) VALUES (?, ?, ?, ?, 'ok', ?)`,
		uuid.New(), e.tenant, r.ID, uuid.New(), MaxActionsPerMinute)
	id := e.task(f.p, "todo", "none", f.owner.UserID)
	e.emit("task.created", id, f.p.ID, nil)
	e.tick()
	var run Run
	e.db.Where("rule_id = ? AND status = 'skipped'", r.ID).Take(&run)
	if run.ErrorCode == nil || *run.ErrorCode != CodeRateLimited || e.tasks.count(ActionPriority) != 0 {
		t.Fatalf("the cap stops the run: %+v", run)
	}
	var got Rule
	e.db.Take(&got, "id = ?", r.ID)
	if got.ConsecutiveFailures != 0 || !got.Active {
		t.Fatal("the cap is not a failure")
	}
}

func TestArchivedProjectAndDeletedTaskAreIgnored(t *testing.T) {
	e := newEnv(t)
	f := e.fixture("AR")
	r := e.rule(f.owner, f.p.ID, RuleInput{Name: "x", Trigger: "task.created", Actions: Actions{{Type: ActionPriority, Priority: "high"}}})
	gone := uuid.New()
	e.emit("task.created", gone, f.p.ID, nil) // no such task
	id := e.task(f.p, "todo", "none", f.owner.UserID)
	if _, err := e.proj.Archive(bg, f.owner, f.p.ID); err != nil {
		t.Fatal(err)
	}
	e.emit("task.created", id, f.p.ID, nil)
	e.tick()
	if e.count(`SELECT count(*) FROM workflow_rule_runs WHERE rule_id = ?`, r.ID) != 0 {
		t.Fatal("archived projects and missing tasks leave no runs")
	}
	e.db.Exec(`INSERT INTO projects_events_outbox (id, tenant_id, event_id, routing_key, payload) VALUES (?, ?, ?, 'task.created', '"junk"')`, uuid.New(), e.tenant, uuid.New())
	e.tick() // a malformed envelope is skipped, not retried forever
}

func TestEngineReportsTaskFailures(t *testing.T) {
	e := newEnv(t)
	f := e.fixture("TE")
	e.rule(f.owner, f.p.ID, RuleInput{Name: "x", Trigger: "task.created", Actions: Actions{{Type: ActionPriority, Priority: "high"}}})
	other := e.fixture("TF2")
	id := e.task(other.p, "todo", "none", other.owner.UserID)
	e.emit("task.created", id, f.p.ID, nil) // the task is not in the event's project
	e.tick()
	if e.tasks.count(ActionPriority) != 0 {
		t.Fatal("a task of another project never matches")
	}
}

func TestLeaseAllowsOneReaderAndRunStops(t *testing.T) {
	e := newEnv(t)
	if _, _, ok, err := e.eng.lease(bg); err != nil || !ok {
		t.Fatalf("first lease: %v %v", err, ok)
	}
	if n, err := e.eng.Tick(bg); err != nil || n != 0 {
		t.Fatalf("a second reader backs off: %d %v", n, err)
	}
	if err := e.eng.release(bg, position{time.Now(), uuid.Nil}); err != nil {
		t.Fatal(err)
	}
	f := e.fixture("LS")
	e.rule(f.owner, f.p.ID, RuleInput{Name: "x", Trigger: "task.created", Actions: Actions{{Type: ActionPriority, Priority: "high"}}})
	id := e.task(f.p, "todo", "none", f.owner.UserID)
	e.emit("task.created", id, f.p.ID, nil)
	ctx, cancel := context.WithTimeout(bg, 3*time.Second)
	defer cancel()
	errs := make(chan error, 4)
	go e.eng.Run(ctx, 50*time.Millisecond, func(err error) { errs <- err })
	for e.tasks.count(ActionPriority) == 0 && ctx.Err() == nil {
		time.Sleep(50 * time.Millisecond)
	}
	if e.tasks.count(ActionPriority) != 1 {
		t.Fatal("the loop picks up the event")
	}
	cancel()
}

func TestPruneDropsOldRuns(t *testing.T) {
	e := newEnv(t)
	f := e.fixture("PR")
	r := e.rule(f.owner, f.p.ID, RuleInput{Name: "x", Trigger: "task.created", Actions: Actions{{Type: ActionPriority, Priority: "high"}}})
	e.db.Exec(`INSERT INTO workflow_rule_runs (id, tenant_id, rule_id, event_id, status, created_at) VALUES (?, ?, ?, ?, 'ok', now() - interval '40 days')`,
		uuid.New(), e.tenant, r.ID, uuid.New())
	e.tick()
	if e.count(`SELECT count(*) FROM workflow_rule_runs WHERE rule_id = ?`, r.ID) != 0 {
		t.Fatal("runs older than 30 days are pruned")
	}
}

func TestRunsListing(t *testing.T) {
	e := newEnv(t)
	f := e.fixture("RU")
	r := e.rule(f.owner, f.p.ID, RuleInput{Name: "x", Trigger: "task.created", Actions: Actions{{Type: ActionPriority, Priority: "high"}}})
	id := e.task(f.p, "todo", "none", f.owner.UserID)
	for i := 0; i < 3; i++ {
		e.emit("task.created", id, f.p.ID, map[string]any{"n": i})
	}
	e.tick()
	page, next, err := e.svc.Runs(bg, f.owner, f.p.ID, r.ID, RunFilter{Limit: 2})
	if err != nil || len(page) != 2 || next == "" {
		t.Fatalf("first page: %v %d %q", err, len(page), next)
	}
	rest, next2, err := e.svc.Runs(bg, f.owner, f.p.ID, r.ID, RunFilter{Limit: 2, Cursor: next})
	if err != nil || len(rest) != 1 || next2 != "" {
		t.Fatalf("second page: %v %d", err, len(rest))
	}
	if rows, _, err := e.svc.Runs(bg, f.owner, f.p.ID, r.ID, RunFilter{Status: StatusFailed, Limit: 10}); err != nil || len(rows) != 0 {
		t.Fatalf("status filter: %v", err)
	}
	_, _, err = e.svc.Runs(bg, f.owner, f.p.ID, r.ID, RunFilter{Status: "weird", Limit: 10})
	wantCode(t, err, "VALIDATION_ERROR")
	_, _, err = e.svc.Runs(bg, f.owner, f.p.ID, r.ID, RunFilter{Cursor: "%%%", Limit: 10})
	wantCode(t, err, "VALIDATION_ERROR")
	_, _, err = e.svc.Runs(bg, f.member, f.p.ID, r.ID, RunFilter{Limit: 10})
	wantCode(t, err, "FORBIDDEN")
	_, _, err = e.svc.Runs(bg, f.owner, f.p.ID, uuid.New(), RunFilter{Limit: 10})
	wantCode(t, err, "NOT_FOUND")
}

func TestDryRunChangesNothing(t *testing.T) {
	e := newEnv(t)
	f := e.fixture("DR")
	done := e.col(f.p, "done")
	r := e.rule(f.owner, f.p.ID, RuleInput{Name: "Dry", Trigger: "task.moved", Conditions: Conditions{
		{Field: FieldPriority, Value: "urgent"}, {Field: FieldToColumn, Value: done.String()}},
		Actions: Actions{{Type: ActionMove, ColumnID: &done}, {Type: ActionPriority, Priority: "urgent"}}})
	urgent := e.task(f.p, "todo", "urgent", f.owner.UserID)
	low := e.task(f.p, "todo", "low", f.owner.UserID)
	events := e.count(`SELECT count(*) FROM projects_events_outbox`)

	res, err := e.svc.Test(bg, f.owner, f.p.ID, r.ID, urgent)
	if err != nil || !res.Matches || !res.CreatorAllowed || len(res.Actions) != 2 || !res.Actions[0].WouldRun || res.Actions[1].WouldRun ||
		res.Actions[1].Reason != "already_set" || len(res.AssumedConditions) != 1 {
		t.Fatalf("dry run on a matching task: %v %+v", err, res)
	}
	res, err = e.svc.Test(bg, f.owner, f.p.ID, r.ID, low)
	if err != nil || res.Matches || res.Actions[0].WouldRun || res.Actions[0].Reason != "conditions_not_met" || len(res.FailedConditions) != 1 {
		t.Fatalf("dry run on a non-matching task: %v %+v", err, res)
	}
	e.db.Exec(`UPDATE workflow_rules SET created_by = ? WHERE id = ?`, uuid.New(), r.ID)
	res, _ = e.svc.Test(bg, f.owner, f.p.ID, r.ID, urgent)
	if res.CreatorAllowed || res.Actions[0].Reason != "creator_not_allowed" {
		t.Fatalf("creator check: %+v", res)
	}
	if len(e.tasks.calls) != 0 || e.count(`SELECT count(*) FROM workflow_rule_runs`) != 0 || e.count(`SELECT count(*) FROM projects_events_outbox`) != events {
		t.Fatal("a dry run changes and records nothing")
	}
	other := e.fixture("DX")
	foreign := e.task(other.p, "todo", "urgent", other.owner.UserID)
	if _, err = e.svc.Test(bg, f.owner, f.p.ID, r.ID, foreign); err == nil {
		t.Fatal("a task of another project is a 404")
	}
	if _, err = e.svc.Test(bg, f.owner, f.p.ID, r.ID, uuid.New()); err == nil {
		t.Fatal("an unknown task is a 404")
	}
	if _, err = e.svc.Test(bg, f.member, f.p.ID, r.ID, urgent); err == nil {
		t.Fatal("members cannot test rules")
	}
	if _, err = e.svc.Test(bg, f.owner, f.p.ID, uuid.New(), urgent); err == nil {
		t.Fatal("unknown rule")
	}
}

func TestConditionEvaluation(t *testing.T) {
	id := uuid.New()
	st := TaskState{ColumnID: id, ColumnCategory: "done", Priority: "low", LabelIDs: []uuid.UUID{id}, AssigneeIDs: []uuid.UUID{id}}
	ev := eventFacts{toColumn: &id, fromColumn: &id, users: []uuid.UUID{id}}
	for _, f := range []string{FieldColumn, FieldLabel, FieldAssignee, FieldToColumn, FieldFromColumn, FieldAssignedUser} {
		if !(Condition{Field: f, Value: id.String()}).holds(st, ev) {
			t.Errorf("%s should hold", f)
		}
		if (Condition{Field: f, Value: uuid.NewString()}).holds(st, ev) || (Condition{Field: f, Value: id.String()}).holds(TaskState{}, eventFacts{}) {
			t.Errorf("%s should not hold", f)
		}
	}
	if (Condition{Field: "bogus", Value: "x"}).holds(st, ev) {
		t.Fatal("unknown fields never hold")
	}
	a := Action{Type: ActionLabel, LabelID: &id}
	if !a.noop(st) || (Action{Type: ActionAssign, UserID: &id}).noop(TaskState{}) {
		t.Fatal("noop detection")
	}
	if (Action{Type: ActionMove}).noop(st) || len((Action{Type: ActionAssign, UserID: &id}).applied(TaskState{}).AssigneeIDs) != 1 {
		t.Fatal("applied state")
	}
	if (Action{Type: ActionDue}).noop(st) {
		t.Fatal("due is never a noop")
	}
	_, err := (&Service{}).runActionCheck()
	if err == nil {
		t.Fatal("incomplete actions are refused")
	}
}

func (s *Service) runActionCheck() (struct{}, error) {
	return struct{}{}, s.runAction(bg, api.Actor{}, uuid.Nil, Action{Type: ActionMove})
}

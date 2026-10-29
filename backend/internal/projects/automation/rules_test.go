package automation

import (
	"strconv"
	"testing"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/project"
)

func TestRuleCrudAndPermissions(t *testing.T) {
	e := newEnv(t)
	f := e.fixture("RC")
	stranger := e.user(false)
	done := e.col(f.p, "done")
	in := RuleInput{Name: "Finish urgent", Trigger: "task.moved", Conditions: Conditions{{Field: FieldPriority, Value: "urgent"}},
		Actions: Actions{{Type: ActionMove, ColumnID: &done}}}

	_, err := e.svc.CreateRule(bg, f.member, f.p.ID, in)
	wantCode(t, err, "FORBIDDEN") // project.update is a manager permission
	_, err = e.svc.CreateRule(bg, stranger, f.p.ID, in)
	wantCode(t, err, "NOT_FOUND")
	r := e.rule(f.owner, f.p.ID, in)
	if !r.Active || r.Version != 1 || r.CreatedBy != f.owner.UserID {
		t.Fatalf("rule defaults: %+v", r)
	}
	_, err = e.svc.CreateRule(bg, f.owner, f.p.ID, in)
	wantCode(t, err, "RULE_NAME_TAKEN")

	rows, err := e.svc.Rules(bg, f.viewr, f.p.ID)
	if err != nil || len(rows) != 1 {
		t.Fatalf("members can read the rules: %v %d", err, len(rows))
	}
	if _, err = e.svc.Rules(bg, stranger, f.p.ID); err == nil {
		t.Fatal("strangers cannot list rules")
	}

	name, off := "Renamed", false
	upd, err := e.svc.UpdateRule(bg, f.owner, f.p.ID, r.ID, RuleUpdate{Name: &name, Active: &off, Version: ptr(1)})
	if err != nil || upd.Name != name || upd.Active || upd.Version != 2 {
		t.Fatalf("update: %v %+v", err, upd)
	}
	_, err = e.svc.UpdateRule(bg, f.owner, f.p.ID, r.ID, RuleUpdate{Name: &name, Version: ptr(1)})
	if _, ok := err.(interface{ Error() string }); !ok || err == nil {
		t.Fatalf("stale version must conflict: %v", err)
	}
	tr := "task.created"
	cs := Conditions{}
	acts := Actions{{Type: ActionPriority, Priority: "high"}}
	if upd, err = e.svc.UpdateRule(bg, f.owner, f.p.ID, r.ID, RuleUpdate{Trigger: &tr, Conditions: &cs, Actions: &acts}); err != nil || upd.Trigger != tr {
		t.Fatalf("update trigger: %v", err)
	}
	if _, err = e.svc.UpdateRule(bg, f.owner, f.p.ID, uuid.New(), RuleUpdate{Name: &name}); err == nil {
		t.Fatal("unknown rule is a 404")
	}
	if err = e.svc.DeleteRule(bg, f.member, f.p.ID, r.ID); err == nil {
		t.Fatal("members cannot delete")
	}
	if err = e.svc.DeleteRule(bg, f.owner, f.p.ID, r.ID); err != nil {
		t.Fatal(err)
	}
	wantCode(t, e.svc.DeleteRule(bg, f.owner, f.p.ID, r.ID), "NOT_FOUND")
	for _, a := range []string{"workflow_rule.created", "workflow_rule.updated", "workflow_rule.deleted"} {
		found := false
		for _, got := range e.audit.actions {
			found = found || got == a
		}
		if !found {
			t.Errorf("audit entry %s missing", a)
		}
	}
}

func TestRuleValidation(t *testing.T) {
	e := newEnv(t)
	f := e.fixture("RV")
	other := e.fixture("RW")
	done, foreign := e.col(f.p, "done"), e.col(other.p, "done")
	label, err := e.proj.CreateLabel(bg, f.owner, &f.p.ID, project.LabelInput{Name: "bug"})
	if err != nil {
		t.Fatal(err)
	}
	foreignLabel, err := e.proj.CreateLabel(bg, other.owner, &other.p.ID, project.LabelInput{Name: "bug"})
	if err != nil {
		t.Fatal(err)
	}
	str := func(s string) string { return s }
	ok := Actions{{Type: ActionPriority, Priority: "high"}}
	cases := map[string]RuleInput{
		"empty name":         {Name: " ", Trigger: "task.moved", Actions: ok},
		"long name":          {Name: string(make([]byte, 81)), Trigger: "task.moved", Actions: ok},
		"bad trigger":        {Name: "a", Trigger: "task.deleted", Actions: ok},
		"no actions":         {Name: "a", Trigger: "task.moved"},
		"too many actions":   {Name: "a", Trigger: "task.moved", Actions: make(Actions, 11)},
		"unknown field":      {Name: "a", Trigger: "task.moved", Actions: ok, Conditions: Conditions{{Field: "title", Value: "x"}}},
		"event field wrong":  {Name: "a", Trigger: "task.created", Actions: ok, Conditions: Conditions{{Field: FieldToColumn, Value: done.String()}}},
		"bad priority":       {Name: "a", Trigger: "task.moved", Actions: ok, Conditions: Conditions{{Field: FieldPriority, Value: "huge"}}},
		"bad category":       {Name: "a", Trigger: "task.moved", Actions: ok, Conditions: Conditions{{Field: FieldCategory, Value: "x"}}},
		"bad uuid":           {Name: "a", Trigger: "task.moved", Actions: ok, Conditions: Conditions{{Field: FieldColumn, Value: "nope"}}},
		"foreign column":     {Name: "a", Trigger: "task.moved", Actions: ok, Conditions: Conditions{{Field: FieldColumn, Value: foreign.String()}}},
		"foreign label":      {Name: "a", Trigger: "task.moved", Actions: ok, Conditions: Conditions{{Field: FieldLabel, Value: foreignLabel.ID.String()}}},
		"too many conds":     {Name: "a", Trigger: "task.moved", Actions: ok, Conditions: make(Conditions, 11)},
		"unknown action":     {Name: "a", Trigger: "task.moved", Actions: Actions{{Type: "delete_task"}}},
		"move no column":     {Name: "a", Trigger: "task.moved", Actions: Actions{{Type: ActionMove}}},
		"move foreign col":   {Name: "a", Trigger: "task.moved", Actions: Actions{{Type: ActionMove, ColumnID: &foreign}}},
		"move extra member":  {Name: "a", Trigger: "task.moved", Actions: Actions{{Type: ActionMove, ColumnID: &done, Text: "x"}}},
		"priority invalid":   {Name: "a", Trigger: "task.moved", Actions: Actions{{Type: ActionPriority, Priority: "x"}}},
		"label no id":        {Name: "a", Trigger: "task.moved", Actions: Actions{{Type: ActionLabel}}},
		"label foreign":      {Name: "a", Trigger: "task.moved", Actions: Actions{{Type: ActionLabel, LabelID: &foreignLabel.ID}}},
		"assign no user":     {Name: "a", Trigger: "task.moved", Actions: Actions{{Type: ActionAssign}}},
		"assign non member":  {Name: "a", Trigger: "task.moved", Actions: Actions{{Type: ActionAssign, UserID: &other.owner.UserID}}},
		"comment empty":      {Name: "a", Trigger: "task.moved", Actions: Actions{{Type: ActionComment, Text: "  "}}},
		"comment too long":   {Name: "a", Trigger: "task.moved", Actions: Actions{{Type: ActionComment, Text: str(string(make([]rune, 501)))}}},
		"due negative":       {Name: "a", Trigger: "task.moved", Actions: Actions{{Type: ActionDue, Days: ptr(-1)}}},
		"due missing":        {Name: "a", Trigger: "task.moved", Actions: Actions{{Type: ActionDue}}},
		"due too far":        {Name: "a", Trigger: "task.moved", Actions: Actions{{Type: ActionDue, Days: ptr(366)}}},
		"priority with text": {Name: "a", Trigger: "task.moved", Actions: Actions{{Type: ActionPriority, Priority: "low", Text: "x"}}},
	}
	for name, in := range cases {
		if _, err := e.svc.CreateRule(bg, f.owner, f.p.ID, in); err == nil {
			t.Errorf("%s: want a validation error", name)
		} else if !contains(err.Error(), "VALIDATION_ERROR") {
			t.Errorf("%s: want VALIDATION_ERROR, got %v", name, err)
		}
	}
	good := RuleInput{Name: "all kinds", Trigger: "task.assigned", Conditions: Conditions{
		{Field: FieldColumn, Value: done.String()}, {Field: FieldCategory, Value: "done"}, {Field: FieldPriority, Value: "low"},
		{Field: FieldLabel, Value: label.ID.String()}, {Field: FieldAssignee, Value: f.member.UserID.String()}, {Field: FieldAssignedUser, Value: f.member.UserID.String()}},
		Actions: Actions{{Type: ActionMove, ColumnID: &done}, {Type: ActionPriority, Priority: "high"}, {Type: ActionLabel, LabelID: &label.ID},
			{Type: ActionAssign, UserID: &f.member.UserID}, {Type: ActionComment, Text: "Auto note"}, {Type: ActionDue, Days: ptr(3)}}}
	if _, err := e.svc.CreateRule(bg, f.owner, f.p.ID, good); err != nil {
		t.Fatalf("every vocabulary item must be accepted: %v", err)
	}
	inactive := false
	good.Name, good.Active = "second", &inactive
	if r, err := e.svc.CreateRule(bg, f.owner, f.p.ID, good); err != nil || r.Active {
		t.Fatalf("inactive rule: %v", err)
	}
}

func TestRuleLimitPerProject(t *testing.T) {
	e := newEnv(t)
	f := e.fixture("RL")
	acts := Actions{{Type: ActionPriority, Priority: "high"}}
	for i := 0; i < maxRules; i++ {
		e.rule(f.owner, f.p.ID, RuleInput{Name: "r" + strconv.Itoa(i), Trigger: "task.created", Actions: acts})
	}
	_, err := e.svc.CreateRule(bg, f.owner, f.p.ID, RuleInput{Name: "one more", Trigger: "task.created", Actions: acts})
	wantCode(t, err, "RULE_LIMIT")
}

func TestArchivedProjectBlocksRuleWrites(t *testing.T) {
	e := newEnv(t)
	f := e.fixture("RA")
	if _, err := e.proj.Archive(bg, f.owner, f.p.ID); err != nil {
		t.Fatal(err)
	}
	_, err := e.svc.CreateRule(bg, f.owner, f.p.ID, RuleInput{Name: "x", Trigger: "task.created", Actions: Actions{{Type: ActionPriority, Priority: "low"}}})
	wantCode(t, err, "PROJECT_ARCHIVED")
	var _ api.Actor = f.owner
}

package fields

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/project"
)

func mustDefine(t *testing.T, e *env, owner api.Actor, p *project.Project, name, typ string, opts ...string) *Definition {
	t.Helper()
	d, err := e.svc.CreateDefinition(bg, owner, p.ID, DefinitionInput{Name: name, Type: typ, Options: opts})
	if err != nil {
		t.Fatalf("define %s: %v", name, err)
	}
	return d
}

func TestDefinitionsLifecycleAndValidation(t *testing.T) {
	e := newEnv(t)
	owner, viewer := e.user(false), e.user(false)
	p, _ := e.task(owner, "FLD")
	_, _ = e.proj.AddMember(bg, owner, p.ID, project.MemberInput{UserID: viewer.UserID, Role: "member"})

	d, err := e.svc.CreateDefinition(bg, owner, p.ID, DefinitionInput{Name: "Story points", Type: TypeNumber})
	if err != nil || d.Key != "story_points" || d.Position != 0 {
		t.Fatalf("create: %+v %v", d, err)
	}
	_, err = e.svc.CreateDefinition(bg, owner, p.ID, DefinitionInput{Name: "story POINTS", Type: TypeText})
	wantCode(t, err, "FIELD_NAME_TAKEN")
	bad := []DefinitionInput{{Name: "", Type: TypeText}, {Name: "x", Type: "blob"}, {Name: "s", Type: TypeSelect}, {Name: "n", Type: TypeNumber, Options: []string{"a"}},
		{Name: "dup", Type: TypeSelect, Options: []string{"a", "A"}}, {Name: "long", Type: TypeSelect, Options: []string{strings.Repeat("x", 61)}}}
	for _, in := range bad {
		if _, err = e.svc.CreateDefinition(bg, owner, p.ID, in); err == nil {
			t.Errorf("accepted %+v", in)
		}
	}
	if _, err = e.svc.CreateDefinition(bg, viewer, p.ID, DefinitionInput{Name: "Nope", Type: TypeText}); err == nil {
		t.Fatal("members cannot define fields (project.update)")
	}
	if _, err = e.svc.CreateDefinition(bg, e.user(false), p.ID, DefinitionInput{Name: "Nope", Type: TypeText}); err == nil {
		t.Fatal("a stranger gets 404")
	}
	list, err := e.svc.Definitions(bg, viewer, p.ID)
	if err != nil || len(list) != 1 {
		t.Fatalf("members read the definitions: %v %d", err, len(list))
	}
	nm := "Points"
	up, err := e.svc.UpdateDefinition(bg, owner, p.ID, d.ID, DefinitionUpdate{Name: &nm})
	if err != nil || up.Name != "Points" || up.Key != "story_points" {
		t.Fatalf("rename keeps the key: %+v %v", up, err)
	}
	if _, err = e.svc.UpdateDefinition(bg, owner, p.ID, d.ID, DefinitionUpdate{Options: &[]string{"x"}}); err == nil {
		t.Fatal("a number field has no options")
	}
	if _, err = e.svc.UpdateDefinition(bg, owner, p.ID, uuid.New(), DefinitionUpdate{Name: &nm}); err == nil {
		t.Fatal("unknown field")
	}
	if err = e.svc.DeleteDefinition(bg, owner, p.ID, d.ID); err != nil || e.svc.DeleteDefinition(bg, owner, p.ID, d.ID) == nil {
		t.Fatal("delete once, 404 twice")
	}
}

func TestDefinitionLimitIsThirty(t *testing.T) {
	e := newEnv(t)
	owner := e.user(false)
	p, _ := e.task(owner, "LIM")
	for i := 0; i < maxDefinitions; i++ {
		if _, err := e.svc.CreateDefinition(bg, owner, p.ID, DefinitionInput{Name: "F" + strings.Repeat("a", i+1), Type: TypeCheckbox}); err != nil {
			t.Fatalf("field %d: %v", i, err)
		}
	}
	_, err := e.svc.CreateDefinition(bg, owner, p.ID, DefinitionInput{Name: "One too many", Type: TypeText})
	wantCode(t, err, "FIELD_LIMIT")
}

func TestValuesByTypeAndVersion(t *testing.T) {
	e := newEnv(t)
	owner, member, outsider := e.user(false), e.user(false), e.user(false)
	p, tid := e.task(owner, "VAL")
	_, _ = e.proj.AddMember(bg, owner, p.ID, project.MemberInput{UserID: member.UserID, Role: "member"})
	mustDefine(t, e, owner, p, "Notes", TypeText)
	mustDefine(t, e, owner, p, "Estimate", TypeNumber)
	mustDefine(t, e, owner, p, "Blocked", TypeCheckbox)
	mustDefine(t, e, owner, p, "Due", TypeDate)
	mustDefine(t, e, owner, p, "Spec", TypeURL)
	mustDefine(t, e, owner, p, "Env", TypeSelect, "dev", "prod")
	mustDefine(t, e, owner, p, "Tags", TypeMultiselect, "a", "b", "c")
	mustDefine(t, e, owner, p, "Reviewer", TypeUser)

	good := map[string]any{"notes": "  hello ", "estimate": 3.5, "blocked": true, "due": "2026-12-31", "spec": "https://example.com/spec",
		"env": "prod", "tags": []any{"a", "c"}, "reviewer": member.UserID.String()}
	vals, err := e.svc.SetTaskFields(bg, member, tid, SetInput{Values: good})
	if err != nil || len(vals) != 8 {
		t.Fatalf("set: %v %v", err, vals)
	}
	got := map[string]any{}
	for _, v := range vals {
		got[v.Key] = v.Value
	}
	raw, _ := json.Marshal(got)
	if got["notes"] != "hello" || got["estimate"] != 3.5 || got["blocked"] != true || got["env"] != "prod" || got["reviewer"] != member.UserID.String() {
		t.Fatalf("stored values: %s", raw)
	}
	if e.count(`SELECT version FROM tasks WHERE id = ?`, tid) != 2 {
		t.Fatal("setting values bumps the task version")
	}

	badValues := []map[string]any{{"notes": 5}, {"estimate": "x"}, {"estimate": 1e13}, {"blocked": "yes"}, {"due": "31/12/2026"}, {"spec": "http://example.com"},
		{"spec": "https://user:pw@example.com"}, {"spec": "javascript:alert(1)"}, {"env": "staging"}, {"tags": []any{"a", "a"}}, {"tags": "a"},
		{"reviewer": uuid.NewString()}, {"reviewer": "nope"}, {"unknown": 1}, {"notes": strings.Repeat("x", maxText+1)}}
	for _, v := range badValues {
		if _, err = e.svc.SetTaskFields(bg, member, tid, SetInput{Values: v}); err == nil {
			t.Errorf("accepted %v", v)
		}
	}
	if _, err = e.svc.SetTaskFields(bg, member, tid, SetInput{Values: map[string]any{}}); err == nil {
		t.Fatal("empty set")
	}
	if _, err = e.svc.SetTaskFields(bg, outsider, tid, SetInput{Values: map[string]any{"notes": "x"}}); err == nil {
		t.Fatal("outsiders get 404")
	}

	stale := 1
	_, err = e.svc.SetTaskFields(bg, member, tid, SetInput{Values: map[string]any{"notes": "late"}, Version: &stale})
	if err == nil || !containsStr(err.Error(), "VERSION_CONFLICT") {
		t.Fatalf("stale version: %v", err)
	}
	cur := 2
	if _, err = e.svc.SetTaskFields(bg, member, tid, SetInput{Values: map[string]any{"notes": nil, "tags": nil}, Version: &cur}); err != nil {
		t.Fatalf("clearing with the right version: %v", err)
	}
	after, _ := e.svc.TaskFields(bg, member, tid)
	for _, v := range after {
		if (v.Key == "notes" || v.Key == "tags") && v.Value != nil {
			t.Fatalf("%s should be cleared", v.Key)
		}
	}
}

func TestOptionRemovalClearsValuesAndActivityHidesText(t *testing.T) {
	e := newEnv(t)
	owner := e.user(false)
	p, tid := e.task(owner, "OPT")
	sel := mustDefine(t, e, owner, p, "Env", TypeSelect, "dev", "prod")
	multi := mustDefine(t, e, owner, p, "Tags", TypeMultiselect, "a", "b")
	mustDefine(t, e, owner, p, "Secret note", TypeText)
	if _, err := e.svc.SetTaskFields(bg, owner, tid, SetInput{Values: map[string]any{"env": "prod", "tags": []any{"a", "b"}, "secret_note": "private text"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.UpdateDefinition(bg, owner, p.ID, sel.ID, DefinitionUpdate{Options: &[]string{"dev"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.UpdateDefinition(bg, owner, p.ID, multi.ID, DefinitionUpdate{Options: &[]string{"b"}}); err != nil {
		t.Fatal(err)
	}
	vals, _ := e.svc.TaskFields(bg, owner, tid)
	for _, v := range vals {
		switch v.Key {
		case "env":
			if v.Value != nil {
				t.Fatal("a value on a removed option is cleared")
			}
		case "tags":
			if l, _ := v.Value.([]any); len(l) != 1 || l[0] != "b" {
				t.Fatalf("a multiselect keeps only valid options: %v", v.Value)
			}
		}
	}
	var data string
	e.db.Raw(`SELECT data::text FROM activity WHERE type = 'custom_fields.updated' ORDER BY seq LIMIT 1`).Scan(&data)
	if strings.Contains(data, "private text") || !strings.Contains(data, "secret_note") {
		t.Fatalf("activity names a text field but never its value: %s", data)
	}
	var payload string
	e.db.Raw(`SELECT payload::text FROM projects_events_outbox WHERE routing_key = 'task.updated' ORDER BY created_at DESC LIMIT 1`).Scan(&payload)
	if strings.Contains(payload, "private text") || !strings.Contains(payload, "custom_field_ids") {
		t.Fatalf("events carry ids only: %s", payload)
	}
}

func TestArchivedProjectsFreezeValuesAndDeleteCascades(t *testing.T) {
	e := newEnv(t)
	owner := e.user(false)
	p, tid := e.task(owner, "ARC")
	d := mustDefine(t, e, owner, p, "Env", TypeSelect, "dev")
	if _, err := e.svc.SetTaskFields(bg, owner, tid, SetInput{Values: map[string]any{"env": "dev"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.proj.Archive(bg, owner, p.ID); err != nil {
		t.Fatal(err)
	}
	_, err := e.svc.SetTaskFields(bg, owner, tid, SetInput{Values: map[string]any{"env": nil}})
	wantCode(t, err, "PROJECT_ARCHIVED")
	if _, err = e.proj.Restore(bg, owner, p.ID); err != nil {
		t.Fatal(err)
	}
	if err = e.svc.DeleteDefinition(bg, owner, p.ID, d.ID); err != nil || e.count(`SELECT count(*) FROM custom_field_values`) != 0 {
		t.Fatalf("deleting a field deletes its values: %v", err)
	}
}

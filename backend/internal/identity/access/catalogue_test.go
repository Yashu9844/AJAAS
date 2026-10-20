package access

import (
	"errors"
	"testing"
)

func TestTemplatesAreOrderedAndChained(t *testing.T) {
	seen := map[string]bool{}
	for _, tpl := range Templates {
		if tpl.Parent != "" && !seen[tpl.Parent] {
			t.Fatalf("%s listed before its parent %s", tpl.Key, tpl.Parent)
		}
		seen[tpl.Key] = true
	}
	chain := []string{TemplateExecutive, TemplateHRAdmin, TemplateHRManager, TemplateManager, TemplateEmployee}
	for i := 0; i < len(chain)-1; i++ {
		tpl, ok := TemplateByKey(chain[i])
		if !ok || tpl.Parent != chain[i+1] {
			t.Fatalf("%s must inherit %s", chain[i], chain[i+1])
		}
	}
	for _, k := range []string{TemplateEmployee, TemplateManager} {
		if tpl, _ := TemplateByKey(k); !tpl.Automatic {
			t.Fatalf("%s is automatic", k)
		}
	}
	if _, ok := TemplateByKey("ceo"); ok {
		t.Fatal("unknown template")
	}
}

func TestValidateDefsAndDefaults(t *testing.T) {
	good := []PermissionDef{
		{Module: "hr", Resource: "leave", Action: "approve", Defaults: map[string]Scope{TemplateManager: ScopeDirectReports, TemplateHRManager: ScopeTenant}},
		{Module: "hr", Resource: "leave", Action: "read"},
	}
	if err := ValidateDefs(good); err != nil {
		t.Fatal(err)
	}
	if g := DefaultGrants(good, TemplateManager); len(g) != 1 || g[0] != (Grant{"leave:approve", ScopeDirectReports}) {
		t.Fatalf("manager defaults: %v", g)
	}
	if len(DefaultGrants(good, TemplateAuditor)) != 0 {
		t.Fatal("no defaults for auditor")
	}
	bad := map[string][]PermissionDef{
		"missing module":   {{Resource: "x", Action: "y"}},
		"duplicate":        {good[1], good[1]},
		"unknown template": {{Module: "m", Resource: "x", Action: "y", Defaults: map[string]Scope{"ceo": ScopeTenant}}},
		"bad scope":        {{Module: "m", Resource: "x", Action: "y", Defaults: map[string]Scope{TemplateManager: "galaxy"}}},
	}
	for name, defs := range bad {
		if err := ValidateDefs(defs); !errors.Is(err, ErrInvalidDefinition) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

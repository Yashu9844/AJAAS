package planning

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
)

func raw(s string) json.RawMessage { return json.RawMessage(s) }

func vin(name string) ViewInput { return ViewInput{Name: ptr(name), Layout: ptr("list")} }

// VW-06: only allow-listed filters, sort fields, groupings and layouts are stored, in canonical form.
func TestSavedViewValidation(t *testing.T) {
	e := newEnv(t)
	owner := e.user("create")
	p := e.scrum(owner, "VAL")
	good := vin("Good")
	good.Filters = raw(`{"priorities":["high","urgent"],"mine":true,"sprint":"active","due_before":"2026-12-31","q":"api"}`)
	good.Sort = raw(`[{"field":"due","dir":"desc"},{"field":"rank"}]`)
	good.Grouping = ptr("priority")
	v, err := e.svc.CreateView(bg, owner, good)
	if err != nil {
		t.Fatal(err)
	}
	if string(v.Sort) != `[{"field":"due","dir":"desc"},{"field":"rank","dir":"asc"}]` || !strings.Contains(string(v.Filters), `"due_before":"2026-12-31"`) {
		t.Fatalf("canonical form: %s %s", v.Filters, v.Sort)
	}
	bare, err := e.svc.CreateView(bg, owner, vin("Bare"))
	if err != nil || string(bare.Filters) != "{}" || string(bare.Sort) != "[]" {
		t.Fatalf("defaults: %s %s %v", bare.Filters, bare.Sort, err)
	}
	longList := `{"column_ids":[` + strings.Repeat(`"`+uuid.NewString()+`",`, 50) + `"` + uuid.NewString() + `"]}`
	cases := map[string]func(*ViewInput){
		"no name":          func(in *ViewInput) { in.Name = nil },
		"no layout":        func(in *ViewInput) { in.Layout = nil },
		"blank name":       func(in *ViewInput) { in.Name = ptr(" ") },
		"layout":           func(in *ViewInput) { in.Layout = ptr("pie") },
		"grouping":         func(in *ViewInput) { in.Grouping = ptr("mood") },
		"unknown filter":   func(in *ViewInput) { in.Filters = raw(`{"sql":"1=1"}`) },
		"filter type":      func(in *ViewInput) { in.Filters = raw(`{"mine":"yes"}`) },
		"bad uuid":         func(in *ViewInput) { in.Filters = raw(`{"column_ids":["x"]}`) },
		"bad priority":     func(in *ViewInput) { in.Filters = raw(`{"priorities":["epic"]}`) },
		"bad category":     func(in *ViewInput) { in.Filters = raw(`{"categories":["limbo"]}`) },
		"bad sprint":       func(in *ViewInput) { in.Filters = raw(`{"sprint":"soon"}`) },
		"date order":       func(in *ViewInput) { in.Filters = raw(`{"due_after":"2026-02-01","due_before":"2026-01-01"}`) },
		"long q":           func(in *ViewInput) { in.Filters = raw(`{"q":"` + strings.Repeat("a", 201) + `"}`) },
		"long list":        func(in *ViewInput) { in.Filters = raw(longList) },
		"trailing data":    func(in *ViewInput) { in.Filters = raw(`{} {}`) },
		"not an object":    func(in *ViewInput) { in.Filters = raw(`[1]`) },
		"too large":        func(in *ViewInput) { in.Filters = raw(`{"q":"` + strings.Repeat(" ", 9000) + `"}`) },
		"sort field":       func(in *ViewInput) { in.Sort = raw(`[{"field":"password","dir":"asc"}]`) },
		"sort dir":         func(in *ViewInput) { in.Sort = raw(`[{"field":"due","dir":"sideways"}]`) },
		"sort fragment":    func(in *ViewInput) { in.Sort = raw(`[{"field":"due; DROP TABLE tasks","dir":"asc"}]`) },
		"sort unknown key": func(in *ViewInput) { in.Sort = raw(`[{"field":"due","expr":"x"}]`) },
		"too many keys": func(in *ViewInput) {
			in.Sort = raw(`[{"field":"due"},{"field":"rank"},{"field":"title"},{"field":"number"}]`)
		},
		"shared no project": func(in *ViewInput) { in.Shared = ptr(true) },
	}
	for name, mutate := range cases {
		in := vin("Case " + name)
		mutate(&in)
		_, err := e.svc.CreateView(bg, owner, in)
		if err == nil {
			t.Errorf("%s: want a validation error", name)
		}
	}
	in := vin("Elsewhere")
	in.ProjectID = ptr(uuid.New())
	_, err = e.svc.CreateView(bg, owner, in)
	wantCode(t, err, "NOT_FOUND")
	in = vin("Good")
	_, err = e.svc.CreateView(bg, owner, in)
	wantCode(t, err, "VIEW_NAME_TAKEN")
	in.ProjectID = &p.ID // the same name in another scope is fine
	if _, err = e.svc.CreateView(bg, owner, in); err != nil {
		t.Fatalf("same name, different scope: %v", err)
	}
}

// VW-06: personal views are private; shared views need view.share and are visible to project readers only.
func TestSavedViewVisibility(t *testing.T) {
	e := newEnv(t)
	owner, manager, member, viewer, stranger := e.user("create"), e.user(), e.user(), e.user(), e.user()
	p := e.scrum(owner, "VIS")
	e.join(owner, p, manager, "manager")
	e.join(owner, p, member, "member")
	e.join(owner, p, viewer, "viewer")

	personal := vin("Mine")
	personal.ProjectID = &p.ID
	mine, err := e.svc.CreateView(bg, member, personal)
	if err != nil || mine.Shared {
		t.Fatalf("personal: %+v %v", mine, err)
	}
	shared := vin("Team board")
	shared.ProjectID, shared.Shared = &p.ID, ptr(true)
	_, err = e.svc.CreateView(bg, member, shared)
	wantCode(t, err, "FORBIDDEN")
	team, err := e.svc.CreateView(bg, manager, shared)
	if err != nil || !team.Shared {
		t.Fatalf("shared: %+v %v", team, err)
	}
	_, err = e.svc.CreateView(bg, stranger, shared)
	wantCode(t, err, "NOT_FOUND")

	list := func(who string) []string {
		var rows []SavedView
		var err error
		switch who {
		case "viewer":
			rows, _, err = e.svc.ListViews(bg, viewer, ViewFilter{Limit: 50})
		case "member":
			rows, _, err = e.svc.ListViews(bg, member, ViewFilter{Limit: 50, ProjectID: &p.ID})
		case "stranger":
			rows, _, err = e.svc.ListViews(bg, stranger, ViewFilter{Limit: 50})
		}
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, r := range rows {
			names = append(names, r.Name)
		}
		return names
	}
	if got := list("viewer"); len(got) != 1 || got[0] != "Team board" {
		t.Fatalf("a project reader sees the shared view only: %v", got)
	}
	if got := list("member"); len(got) != 2 {
		t.Fatalf("the owner sees personal and shared: %v", got)
	}
	if got := list("stranger"); len(got) != 0 {
		t.Fatalf("a stranger sees nothing: %v", got)
	}
	if _, _, err = e.svc.ListViews(bg, stranger, ViewFilter{Limit: 5, ProjectID: &p.ID}); err == nil {
		t.Fatal("filtering by an unreadable project must be a 404")
	}
	if _, _, err = e.svc.ListViews(bg, viewer, ViewFilter{Limit: 5, Cursor: "!"}); err == nil {
		t.Fatal("bad cursor")
	}
	// A personal view of someone else is a 404 (not a 403 that confirms it exists).
	_, err = e.svc.UpdateView(bg, viewer, mine.ID, ViewInput{Name: ptr("x")})
	wantCode(t, err, "NOT_FOUND")
	wantCode(t, e.svc.DeleteView(bg, viewer, mine.ID), "NOT_FOUND")
}

func TestSavedViewUpdateAndDelete(t *testing.T) {
	e := newEnv(t)
	owner, manager, member, viewer := e.user("create"), e.user(), e.user(), e.user()
	p := e.scrum(owner, "UPD")
	e.join(owner, p, manager, "manager")
	e.join(owner, p, member, "member")
	e.join(owner, p, viewer, "viewer")
	in := vin("Plan")
	in.ProjectID = &p.ID
	v, _ := e.svc.CreateView(bg, member, in)

	up, err := e.svc.UpdateView(bg, member, v.ID, ViewInput{Name: ptr("Plan B"), Layout: ptr("board"), Filters: raw(`{"mine":true}`), Version: ptr(1)})
	if err != nil || up.Name != "Plan B" || up.Layout != "board" || up.Version != 2 || string(up.Filters) != `{"mine":true}` {
		t.Fatalf("update: %+v %v", up, err)
	}
	_, err = e.svc.UpdateView(bg, member, v.ID, ViewInput{Name: ptr("stale"), Version: ptr(1)})
	if c, ok := kernel.AsVersionConflict(err); !ok || c.Current != 2 {
		t.Fatalf("stale version: %v", err)
	}
	_, err = e.svc.UpdateView(bg, member, v.ID, ViewInput{Layout: ptr("pie")})
	wantCode(t, err, "VALIDATION_ERROR")
	_, err = e.svc.UpdateView(bg, member, v.ID, ViewInput{Shared: ptr(true)})
	wantCode(t, err, "FORBIDDEN")
	shared, err := e.svc.UpdateView(bg, manager, mustOwn(t, e, manager, p.ID, "Mgr").ID, ViewInput{Shared: ptr(true)})
	if err != nil || !shared.Shared {
		t.Fatalf("a manager can share: %+v %v", shared, err)
	}
	// A shared view can be edited or removed by a view.share holder, not by a plain reader.
	if _, err = e.svc.UpdateView(bg, owner, shared.ID, ViewInput{Name: ptr("Renamed")}); err != nil {
		t.Fatalf("view.share holders manage shared views: %v", err)
	}
	_, err = e.svc.UpdateView(bg, viewer, shared.ID, ViewInput{Name: ptr("nope")})
	wantCode(t, err, "FORBIDDEN")
	wantCode(t, e.svc.DeleteView(bg, viewer, shared.ID), "FORBIDDEN")
	unshared, err := e.svc.UpdateView(bg, manager, shared.ID, ViewInput{Shared: ptr(false)})
	if err != nil || unshared.Shared {
		t.Fatalf("unshare: %+v %v", unshared, err)
	}
	// A cross-project personal view cannot become shared.
	global, _ := e.svc.CreateView(bg, member, vin("Everywhere"))
	_, err = e.svc.UpdateView(bg, member, global.ID, ViewInput{Shared: ptr(true)})
	wantCode(t, err, "VALIDATION_ERROR")
	taken, _ := e.svc.CreateView(bg, member, vin("Taken"))
	_, err = e.svc.UpdateView(bg, member, taken.ID, ViewInput{Name: ptr("everywhere")})
	wantCode(t, err, "VIEW_NAME_TAKEN")

	if err = e.svc.DeleteView(bg, member, v.ID); err != nil {
		t.Fatal(err)
	}
	wantCode(t, e.svc.DeleteView(bg, member, v.ID), "NOT_FOUND")
	if e.count(`SELECT count(*) FROM saved_views WHERE id = ?`, v.ID) != 0 {
		t.Fatal("deleted")
	}
}

func mustOwn(t *testing.T, e *env, who api.Actor, project uuid.UUID, name string) *SavedView {
	t.Helper()
	in := vin(name)
	in.ProjectID = &project
	v, err := e.svc.CreateView(bg, who, in)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

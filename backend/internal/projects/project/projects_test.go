package project

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
)

var bg = context.Background()

// PR-01: a new project has an owner, four columns and an event; the key is unique per tenant.
func TestCreateProjectDefaults(t *testing.T) {
	e := newEnv(t)
	owner := e.user("create")
	p := e.project(owner, "alpha")
	if p.Key != "ALPHA" || p.Status != "active" || p.Visibility != "private" || p.Mode != "kanban" || p.OwnerUserID != owner.UserID || p.Version != 1 {
		t.Fatalf("project: %+v", p)
	}
	if e.count(`SELECT count(*) FROM board_columns WHERE project_id = ?`, p.ID) != 4 ||
		e.count(`SELECT count(*) FROM board_columns WHERE project_id = ? AND is_default AND category = 'todo'`, p.ID) != 1 {
		t.Fatal("default columns")
	}
	if e.count(`SELECT count(*) FROM project_members WHERE project_id = ? AND role_key = 'owner' AND user_id = ?`, p.ID, owner.UserID) != 1 {
		t.Fatal("creator must be the owner")
	}
	if e.count(`SELECT count(*) FROM projects_events_outbox WHERE routing_key = 'project.created'`) != 1 || len(e.audit.actions) != 1 {
		t.Fatal("event and audit")
	}
	_, err := e.svc.Create(bg, owner, CreateInput{Key: "ALPHA", Name: "Again"})
	wantCode(t, err, "PROJECT_KEY_TAKEN")
	other := e.user("create")
	other.TenantID = uuid.New()
	if _, err := e.svc.Create(bg, other, CreateInput{Key: "ALPHA", Name: "Other tenant"}); err != nil {
		t.Fatalf("keys are per tenant: %v", err)
	}
}

func TestCreateProjectValidation(t *testing.T) {
	e := newEnv(t)
	owner := e.user("create")
	d := func(s string) *kernel.Date { v, _ := kernel.ParseDate(s); return &v }
	bad := map[string]CreateInput{
		"key short":   {Key: "A", Name: "x"},
		"key digit":   {Key: "1AB", Name: "x"},
		"key long":    {Key: "ABCDEFGHIJK", Name: "x"},
		"no name":     {Key: "AB", Name: "  "},
		"visibility":  {Key: "AB", Name: "x", Visibility: "public"},
		"mode":        {Key: "AB", Name: "x", Mode: "waterfall"},
		"color":       {Key: "AB", Name: "x", Color: "red"},
		"dates":       {Key: "AB", Name: "x", StartOn: d("2026-05-02"), DueOn: d("2026-05-01")},
		"description": {Key: "AB", Name: "x", Description: string(make([]rune, maxDescription+1))},
	}
	for name, in := range bad {
		if in.Description != "" {
			in.Description = string(repeat('a', maxDescription+1))
		}
		if _, err := e.svc.Create(bg, owner, in); err == nil {
			t.Errorf("%s: want validation error", name)
		}
	}
	if _, err := e.svc.Create(bg, e.user(), CreateInput{Key: "AB", Name: "x"}); err == nil {
		t.Fatal("creating needs projects:create")
	}
	if _, err := e.svc.Create(bg, e.user("admin"), CreateInput{Key: "AB", Name: "x"}); err != nil {
		t.Fatalf("admins can create: %v", err)
	}
}

func repeat(r rune, n int) []rune {
	out := make([]rune, n)
	for i := range out {
		out[i] = r
	}
	return out
}

// PG4 / specification section 8: private projects are invisible, tenant projects read-only, foreign tenants see nothing.
func TestAccessRules(t *testing.T) {
	e := newEnv(t)
	owner, member, viewer, stranger := e.user("create"), e.user(), e.user(), e.user()
	p := e.project(owner, "PRIV")
	for u, role := range map[uuid.UUID]string{member.UserID: "member", viewer.UserID: "viewer"} {
		if _, err := e.svc.AddMember(bg, owner, p.ID, MemberInput{UserID: u, Role: role}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := e.svc.Access(bg, nil, stranger, p.ID); err == nil {
		t.Fatal("a private project must be invisible to non-members (404)")
	}
	foreign := stranger
	foreign.TenantID = uuid.New()
	if _, err := e.svc.Access(bg, nil, foreign, p.ID); err == nil {
		t.Fatal("foreign tenant must get 404")
	}
	acc, err := e.svc.Access(bg, nil, member, p.ID)
	if err != nil || !acc.Can(api.TaskCreate) || acc.Can(api.ColumnManage) || acc.Role != "member" || !acc.Member {
		t.Fatalf("member access: %+v %v", acc, err)
	}
	if acc, _ = e.svc.Access(bg, nil, viewer, p.ID); acc.Can(api.TaskCreate) || !acc.Can(api.TaskRead) {
		t.Fatal("viewer reads only")
	}
	wantCode(t, acc.Require(api.TaskCreate), "FORBIDDEN")
	ra := e.user("readall")
	if acc, err = e.svc.Access(bg, nil, ra, p.ID); err != nil || !acc.Can(api.TaskRead) || acc.Can(api.TaskUpdate) || acc.Member {
		t.Fatalf("read_all reads without membership: %+v %v", acc, err)
	}
	if acc, _ = e.svc.Access(bg, nil, e.user("admin"), p.ID); !acc.Can(api.ProjectArchive) || acc.Role != "tenant_admin" {
		t.Fatal("manage_all holds every permission")
	}
}

func TestTenantVisibilityAndArchivedFreeze(t *testing.T) {
	e := newEnv(t)
	owner, reader := e.user("create"), e.user()
	p := e.project(owner, "OPEN", func(in *CreateInput) { in.Visibility = "tenant" })
	acc, err := e.svc.Access(bg, nil, reader, p.ID)
	if err != nil || !acc.Can(api.TaskRead) || acc.Can(api.TaskCreate) {
		t.Fatalf("tenant visibility is read-only: %+v %v", acc, err)
	}
	if _, err := e.svc.Archive(bg, owner, p.ID); err != nil {
		t.Fatal(err)
	}
	acc, _ = e.svc.Access(bg, nil, owner, p.ID)
	if acc.Can(api.TaskCreate) || !acc.Can(api.ProjectArchive) {
		t.Fatalf("archived keeps only reads and restore: %v", acc.Perms)
	}
	wantCode(t, acc.Require(api.TaskCreate), "PROJECT_ARCHIVED")
	ids, err := e.svc.ReadableProjectIDs(bg, nil, reader)
	if err != nil || len(ids) != 1 || ids[0] != p.ID {
		t.Fatalf("readable ids: %v %v", ids, err)
	}
}

func TestUpdateProjectVersioned(t *testing.T) {
	e := newEnv(t)
	owner, member := e.user("create"), e.user()
	p := e.project(owner, "UPD")
	_, _ = e.svc.AddMember(bg, owner, p.ID, MemberInput{UserID: member.UserID, Role: "member"})
	name, v1, v9 := "Renamed", 1, 9
	got, err := e.svc.Update(bg, owner, p.ID, UpdateInput{Name: &name, Version: &v1})
	if err != nil || got.Name != name || got.Version != 2 {
		t.Fatalf("update: %+v %v", got, err)
	}
	_, err = e.svc.Update(bg, owner, p.ID, UpdateInput{Name: &name, Version: &v9})
	if vc, ok := kernel.AsVersionConflict(err); !ok || vc.Current != 2 {
		t.Fatalf("stale version: %v", err)
	}
	if _, err = e.svc.Update(bg, member, p.ID, UpdateInput{Name: &name}); err == nil {
		t.Fatal("members cannot edit the project")
	}
	empty := " "
	if _, err = e.svc.Update(bg, owner, p.ID, UpdateInput{Name: &empty}); err == nil {
		t.Fatal("empty name")
	}
	scrum, off := "scrum", false
	got, err = e.svc.Update(bg, owner, p.ID, UpdateInput{Mode: &scrum, TimeTrackingEnabled: &off})
	if err != nil || got.Mode != "scrum" || got.TimeTrackingEnabled {
		t.Fatalf("mode and time tracking: %+v %v", got, err)
	}
}

func TestArchiveRestoreDeleteAndFavorite(t *testing.T) {
	e := newEnv(t)
	owner, admin, member := e.user("create"), e.user("admin"), e.user()
	p := e.project(owner, "LIFE")
	_, _ = e.svc.AddMember(bg, owner, p.ID, MemberInput{UserID: member.UserID, Role: "manager"})
	if _, err := e.svc.Archive(bg, member, p.ID); err == nil {
		t.Fatal("only owners archive")
	}
	if _, err := e.svc.Restore(bg, owner, p.ID); err == nil {
		t.Fatal("restoring an active project is a conflict")
	}
	if err := e.svc.Delete(bg, admin, p.ID); err == nil {
		t.Fatal("delete needs archive first")
	}
	got, err := e.svc.Archive(bg, owner, p.ID)
	if err != nil || got.Status != "archived" || got.ArchivedAt == nil {
		t.Fatalf("archive: %+v %v", got, err)
	}
	if _, err = e.svc.Archive(bg, owner, p.ID); err == nil {
		t.Fatal("double archive")
	}
	if got, err = e.svc.Restore(bg, owner, p.ID); err != nil || got.Status != "active" {
		t.Fatalf("restore: %+v %v", got, err)
	}
	_, _ = e.svc.Archive(bg, owner, p.ID)
	if err = e.svc.Delete(bg, owner, p.ID); err == nil {
		t.Fatal("only tenant administrators delete")
	}
	if err = e.svc.Delete(bg, admin, p.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = e.svc.Get(bg, owner, p.ID); err == nil {
		t.Fatal("a deleted project is gone")
	}
	q := e.project(owner, "FAVE")
	if err = e.svc.SetFavorite(bg, owner, q.ID, true); err != nil || e.svc.SetFavorite(bg, owner, q.ID, true) != nil {
		t.Fatal("favourite is idempotent")
	}
	if g, _ := e.svc.Get(bg, owner, q.ID); !g.Favorite || g.MyRole != "owner" {
		t.Fatalf("get: %+v", g)
	}
	_ = e.svc.SetFavorite(bg, owner, q.ID, false)
	if g, _ := e.svc.Get(bg, owner, q.ID); g.Favorite {
		t.Fatal("unfavourite")
	}
	if err = e.svc.SetFavorite(bg, e.user(), q.ID, true); err == nil {
		t.Fatal("strangers cannot favourite a private project")
	}
}

// PR-09: filters, recent-activity order and cursor pages.
func TestListProjects(t *testing.T) {
	e := newEnv(t)
	owner, other := e.user("create"), e.user("create")
	keys := []string{"AA", "BB", "CC", "DD", "EE"}
	for _, k := range keys {
		e.project(owner, k)
	}
	hidden := e.project(other, "ZZ")
	_ = hidden
	pub := e.project(other, "PUB", func(in *CreateInput) { in.Visibility = "tenant" })
	_ = pub
	rows, next, err := e.svc.List(bg, owner, ListFilter{Limit: 3})
	if err != nil || len(rows) != 3 || next == "" || rows[0].Key != "PUB" {
		t.Fatalf("page 1: %d %q %v", len(rows), next, err)
	}
	seen := map[string]bool{}
	for _, r := range rows {
		seen[r.Key] = true
	}
	rows, next, err = e.svc.List(bg, owner, ListFilter{Limit: 3, Cursor: next})
	if err != nil || next != "" {
		t.Fatalf("page 2: %v %q", err, next)
	}
	for _, r := range rows {
		if seen[r.Key] || r.Key == "ZZ" {
			t.Fatalf("duplicate or invisible project %s", r.Key)
		}
		seen[r.Key] = true
	}
	if len(seen) != 6 {
		t.Fatalf("saw %v", seen)
	}
	mine, _, _ := e.svc.List(bg, owner, ListFilter{Mine: true, Limit: 50})
	if len(mine) != 5 {
		t.Fatalf("mine: %d", len(mine))
	}
	_ = e.svc.SetFavorite(bg, owner, pub.ID, true)
	fav, _, _ := e.svc.List(bg, owner, ListFilter{Favorite: true, Limit: 50})
	byName, _, _ := e.svc.List(bg, owner, ListFilter{Query: "project c%", Limit: 50})
	if len(fav) != 1 || fav[0].Key != "PUB" || len(byName) != 0 {
		t.Fatalf("favorite %d, like-escaped name %d", len(fav), len(byName))
	}
	byName, _, _ = e.svc.List(bg, owner, ListFilter{Query: "project c", Limit: 50})
	if len(byName) != 1 {
		t.Fatalf("name search: %d", len(byName))
	}
	if _, _, err = e.svc.List(bg, owner, ListFilter{Status: "weird", Limit: 5}); err == nil {
		t.Fatal("bad status")
	}
	if _, _, err = e.svc.List(bg, owner, ListFilter{Cursor: "%%", Limit: 5}); err == nil {
		t.Fatal("bad cursor")
	}
}

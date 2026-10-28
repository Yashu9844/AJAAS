package project

import (
	"testing"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
)

func TestMembersLifecycle(t *testing.T) {
	e := newEnv(t)
	owner, mgr, alice, ghost := e.user("create"), e.user(), e.user(), uuid.New()
	p := e.project(owner, "MEM")
	if _, err := e.svc.AddMember(bg, owner, p.ID, MemberInput{UserID: mgr.UserID, Role: "manager"}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.AddMember(bg, mgr, p.ID, MemberInput{UserID: alice.UserID, Role: "member"}); err != nil {
		t.Fatalf("managers add members: %v", err)
	}
	_, err := e.svc.AddMember(bg, mgr, p.ID, MemberInput{UserID: alice.UserID, Role: "viewer"})
	wantCode(t, err, "ALREADY_MEMBER")
	if _, err = e.svc.AddMember(bg, owner, p.ID, MemberInput{UserID: ghost, Role: "member"}); err == nil {
		t.Fatal("unknown users are refused")
	}
	for _, in := range []MemberInput{{UserID: alice.UserID}, {UserID: alice.UserID, Role: "boss"}, {UserID: alice.UserID, Role: "member", CustomRoleID: &p.ID},
		{UserID: alice.UserID, CustomRoleID: &p.ID}} {
		if _, err = e.svc.ChangeMember(bg, owner, p.ID, alice.UserID, in); err == nil {
			t.Fatalf("invalid role input accepted: %+v", in)
		}
	}
	if _, err = e.svc.AddMember(bg, mgr, p.ID, MemberInput{UserID: e.user().UserID, Role: "owner"}); err == nil {
		t.Fatal("only owners grant ownership")
	}
	if _, err = e.svc.ChangeMember(bg, alice, p.ID, alice.UserID, MemberInput{Role: "owner"}); err == nil {
		t.Fatal("members cannot promote themselves")
	}
	views, err := e.svc.Members(bg, mgr, p.ID)
	if err != nil || len(views) != 3 || views[0].RoleName != "owner" {
		t.Fatalf("members: %+v %v", views, err)
	}
}

// PR-04: the last owner cannot be demoted or removed; a second owner can leave.
func TestLastOwnerGuard(t *testing.T) {
	e := newEnv(t)
	owner, second := e.user("create"), e.user()
	p := e.project(owner, "OWN")
	_, err := e.svc.ChangeMember(bg, owner, p.ID, owner.UserID, MemberInput{Role: "manager"})
	wantCode(t, err, "LAST_OWNER")
	wantCode(t, e.svc.RemoveMember(bg, owner, p.ID, owner.UserID), "LAST_OWNER")
	_, _ = e.svc.AddMember(bg, owner, p.ID, MemberInput{UserID: second.UserID, Role: "owner"})
	if _, err = e.svc.ChangeMember(bg, second, p.ID, owner.UserID, MemberInput{Role: "member"}); err != nil {
		t.Fatalf("owners can demote another owner: %v", err)
	}
	if err = e.svc.RemoveMember(bg, second, p.ID, owner.UserID); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if err = e.svc.RemoveMember(bg, second, p.ID, uuid.New()); err == nil {
		t.Fatal("removing a non-member is 404")
	}
	if _, err = e.svc.Access(bg, nil, owner, p.ID); err == nil {
		t.Fatal("a removed member loses access to a private project")
	}
	joiner := e.user()
	_, _ = e.svc.AddMember(bg, second, p.ID, MemberInput{UserID: joiner.UserID, Role: "viewer"})
	if err = e.svc.RemoveMember(bg, joiner, p.ID, joiner.UserID); err != nil {
		t.Fatalf("members may leave: %v", err)
	}
	if err = e.svc.RemoveMember(bg, joiner, p.ID, second.UserID); err == nil {
		t.Fatal("non-members cannot remove others")
	}
}

func TestCustomRoles(t *testing.T) {
	e := newEnv(t)
	admin, owner, user := e.user("roles"), e.user("create"), e.user()
	if _, err := e.svc.CreateRole(bg, user, RoleInput{Name: "X", Permissions: []string{"project.read", "task.read"}}); err == nil {
		t.Fatal("roles need project_roles:manage")
	}
	bad := []RoleInput{{Name: ""}, {Name: "Owner", Permissions: []string{"project.read", "task.read"}}, {Name: "Z", Permissions: []string{"nope"}},
		{Name: "Z", Permissions: []string{"task.create"}}, {Name: "Z", Description: string(repeat('a', 301)), Permissions: []string{"project.read", "task.read"}}}
	for _, in := range bad {
		if _, err := e.svc.CreateRole(bg, admin, in); err == nil {
			t.Errorf("accepted %+v", in)
		}
	}
	r, err := e.svc.CreateRole(bg, admin, RoleInput{Name: "Reviewer", Permissions: []string{"project.read", "task.read", "task.comment", "task.comment"}})
	if err != nil || len(r.Permissions) != 3 {
		t.Fatalf("create: %+v %v", r, err)
	}
	_, err = e.svc.CreateRole(bg, admin, RoleInput{Name: "reviewer", Permissions: []string{"project.read", "task.read"}})
	wantCode(t, err, "ROLE_NAME_TAKEN")
	p := e.project(owner, "ROLE")
	if _, err = e.svc.AddMember(bg, owner, p.ID, MemberInput{UserID: user.UserID, CustomRoleID: &r.ID}); err != nil {
		t.Fatal(err)
	}
	acc, _ := e.svc.Access(bg, nil, user, p.ID)
	if !acc.Can(api.TaskComment) || acc.Can(api.TaskCreate) || acc.Role != "Reviewer" {
		t.Fatalf("custom role access: %+v", acc)
	}
	r, err = e.svc.UpdateRole(bg, admin, r.ID, RoleInput{Name: "Reviewer", Permissions: []string{"project.read", "task.read", "task.create"}})
	if err != nil {
		t.Fatal(err)
	}
	if acc, _ = e.svc.Access(bg, nil, user, p.ID); !acc.Can(api.TaskCreate) || acc.Can(api.TaskComment) {
		t.Fatal("role edits apply to members at once")
	}
	wantCode(t, e.svc.DeleteRole(bg, admin, r.ID), "ROLE_IN_USE")
	_ = e.svc.RemoveMember(bg, owner, p.ID, user.UserID)
	if err = e.svc.DeleteRole(bg, admin, r.ID); err != nil {
		t.Fatal(err)
	}
	if err = e.svc.DeleteRole(bg, admin, r.ID); err == nil {
		t.Fatal("deleting twice is 404")
	}
	if roles, _ := e.svc.Roles(bg, admin); len(roles) != 0 {
		t.Fatal("roles list")
	}
	if _, err = e.svc.UpdateRole(bg, admin, uuid.New(), RoleInput{Name: "N", Permissions: []string{"project.read", "task.read"}}); err == nil {
		t.Fatal("unknown role")
	}
	if c := PermissionCatalogue(); len(c.Permissions) != len(api.AllPerms) || len(c.Roles) != 4 {
		t.Fatal("catalogue")
	}
}

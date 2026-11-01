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
	for _, in := range []MemberInput{{UserID: alice.UserID}, {UserID: alice.UserID, Role: "boss"}, {UserID: alice.UserID, Role: "member", RoleID: &p.ID},
		{UserID: alice.UserID, RoleID: &p.ID}} {
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
	if _, err = e.svc.AddMember(bg, owner, p.ID, MemberInput{UserID: second.UserID, Role: "owner"}); err != nil {
		t.Fatalf("add second owner: %v", err)
	}
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

// DPJ-49: roles live in M0. A member can hold a custom project role; its permissions and name come from M0 at once, and a role
// M0 does not know is refused.
func TestMemberWithCustomRoleFromIdentity(t *testing.T) {
	e := newEnv(t)
	owner, user := e.user("create"), e.user()
	p := e.project(owner, "ROLE")
	role := e.res.DefineRole("Reviewer", "project.read", "task.read", "task.comment")
	if _, err := e.svc.AddMember(bg, owner, p.ID, MemberInput{UserID: user.UserID, RoleID: &role}); err != nil {
		t.Fatal(err)
	}
	acc, _ := e.svc.Access(bg, nil, user, p.ID)
	if !acc.Can(api.TaskComment) || acc.Can(api.TaskCreate) || acc.Role != "Reviewer" {
		t.Fatalf("custom role access: %+v", acc)
	}
	members, err := e.svc.Members(bg, owner, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range members {
		if m.UserID == user.UserID && (m.RoleID != role || m.RoleName != "Reviewer" || m.RoleKey != "") {
			t.Fatalf("member view: %+v", m)
		}
	}
	unknown := uuid.New()
	_, err = e.svc.ChangeMember(bg, owner, p.ID, user.UserID, MemberInput{RoleID: &unknown})
	wantCode(t, err, "VALIDATION_ERROR")
	if _, err = e.svc.ChangeMember(bg, owner, p.ID, user.UserID, MemberInput{Role: "viewer"}); err != nil {
		t.Fatal(err)
	}
	if acc, _ = e.svc.Access(bg, nil, user, p.ID); acc.Can(api.TaskComment) || acc.Role != "viewer" {
		t.Fatalf("back to viewer: %+v", acc)
	}
}

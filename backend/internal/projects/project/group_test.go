package project

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
)

func roleOf(e *env, p *Project, u api.Actor) (string, string) {
	var m Member
	if err := e.db.Where("project_id = ? AND user_id = ? AND active", p.ID, u.UserID).Take(&m).Error; err != nil {
		return "", ""
	}
	if m.RoleKey == nil {
		return "", m.Source
	}
	return *m.RoleKey, m.Source
}

// PR-04: department, team and reports-of rules expand into members, the highest role wins, direct members stay.
func TestGroupRulesExpandAndReconcile(t *testing.T) {
	e := newEnv(t)
	owner, a, b, c, direct := e.user("create"), e.user(), e.user(), e.user(), e.user()
	p := e.project(owner, "GRP")
	dept, team, mgr := uuid.New(), uuid.New(), uuid.New()
	e.people.members[dept] = []uuid.UUID{a.UserID, b.UserID, direct.UserID}
	e.people.members[team] = []uuid.UUID{b.UserID}
	e.people.members[mgr] = []uuid.UUID{c.UserID}
	_, _ = e.svc.AddMember(bg, owner, p.ID, MemberInput{UserID: direct.UserID, Role: "viewer"})

	r1, err := e.svc.AddGroupRule(bg, owner, p.ID, GroupRuleInput{Kind: "department", RefID: dept, Role: "member"})
	if err != nil || r1.Kind != "department" {
		t.Fatalf("add rule: %+v %v", r1, err)
	}
	if role, src := roleOf(e, p, a); role != "member" || src != "group" {
		t.Fatalf("a: %s %s", role, src)
	}
	if role, src := roleOf(e, p, direct); role != "viewer" || src != "direct" {
		t.Fatalf("a direct member must keep its row: %s %s", role, src)
	}
	if _, err = e.svc.AddGroupRule(bg, owner, p.ID, GroupRuleInput{Kind: "team", RefID: team, Role: "manager"}); err != nil {
		t.Fatal(err)
	}
	if role, _ := roleOf(e, p, b); role != "manager" {
		t.Fatalf("highest role wins across rules: %s", role)
	}
	r3, _ := e.svc.AddGroupRule(bg, owner, p.ID, GroupRuleInput{Kind: "reports_of", RefID: mgr, Role: "viewer"})
	if role, _ := roleOf(e, p, c); role != "viewer" {
		t.Fatalf("reports_of: %s", role)
	}
	if acc, err := e.svc.Access(bg, nil, c, p.ID); err != nil || !acc.Can(api.TaskRead) || acc.Can(api.TaskCreate) {
		t.Fatalf("group members get access through their role: %+v %v", acc, err)
	}
	wantCode(t, e.svc.RemoveMember(bg, owner, p.ID, c.UserID), "GROUP_MEMBER")
	if _, err = e.svc.AddGroupRule(bg, owner, p.ID, GroupRuleInput{Kind: "reports_of", RefID: mgr, Role: "member"}); err == nil {
		t.Fatal("duplicate rule")
	}
	// HR changes: c leaves the manager, a joins nothing; refresh drops c and keeps the rest.
	e.people.members[mgr] = nil
	if err = e.svc.RefreshProject(bg, owner, p.ID); err != nil {
		t.Fatal(err)
	}
	if role, _ := roleOf(e, p, c); role != "" {
		t.Fatal("a user HR no longer places under the manager leaves the project")
	}
	// Removing the department rule removes a, keeps b through the team rule and direct stays.
	if err = e.svc.RemoveGroupRule(bg, owner, p.ID, r1.ID); err != nil {
		t.Fatal(err)
	}
	if role, _ := roleOf(e, p, a); role != "" {
		t.Fatal("a only came from the department rule")
	}
	if role, _ := roleOf(e, p, b); role != "manager" {
		t.Fatalf("b stays through the team rule: %s", role)
	}
	if role, src := roleOf(e, p, direct); role != "viewer" || src != "direct" {
		t.Fatal("direct member untouched")
	}
	// Pinning: an explicit role change turns a group member into a direct one that survives the rule.
	if _, err = e.svc.ChangeMember(bg, owner, p.ID, b.UserID, MemberInput{Role: "member"}); err != nil {
		t.Fatal(err)
	}
	if role, src := roleOf(e, p, b); role != "member" || src != "direct" {
		t.Fatalf("pinned: %s %s", role, src)
	}
	rules, err := e.svc.GroupRules(bg, owner, p.ID)
	if err != nil || len(rules) != 2 {
		t.Fatalf("rules: %d %v", len(rules), err)
	}
	if err = e.svc.RemoveGroupRule(bg, owner, p.ID, r3.ID); err != nil || e.svc.RemoveGroupRule(bg, owner, p.ID, r3.ID) == nil {
		t.Fatal("delete once, 404 twice")
	}
}

func TestGroupRuleGuards(t *testing.T) {
	e := newEnv(t)
	owner, member := e.user("create"), e.user()
	p := e.project(owner, "GRD")
	_, _ = e.svc.AddMember(bg, owner, p.ID, MemberInput{UserID: member.UserID, Role: "member"})
	ok := GroupRuleInput{Kind: "team", RefID: uuid.New(), Role: "member"}
	if _, err := e.svc.AddGroupRule(bg, member, p.ID, ok); err == nil {
		t.Fatal("members cannot add rules")
	}
	for _, in := range []GroupRuleInput{{Kind: "x", RefID: uuid.New(), Role: "member"}, {Kind: "team", Role: "member"}, {Kind: "team", RefID: uuid.New(), Role: "owner"}} {
		if _, err := e.svc.AddGroupRule(bg, owner, p.ID, in); err == nil {
			t.Fatalf("accepted %+v", in)
		}
	}
	e.people.err = errors.New("hr down")
	if _, err := e.svc.AddGroupRule(bg, owner, p.ID, ok); err == nil {
		t.Fatal("a failing people lookup must fail the request, not silently add nobody")
	}
	e.people.err = nil
	if _, err := e.svc.GroupRules(bg, member, p.ID); err == nil {
		t.Fatal("rules are visible to managers only")
	}
	noPeople := NewService(e.db, Ports{Users: e.users, Audit: e.audit})
	_, err := noPeople.AddGroupRule(bg, owner, p.ID, ok)
	wantCode(t, err, "PEOPLE_UNAVAILABLE")
	wantCode(t, noPeople.RemoveGroupRule(bg, owner, p.ID, uuid.New()), "PEOPLE_UNAVAILABLE")
	wantCode(t, noPeople.RefreshProject(bg, owner, p.ID), "PEOPLE_UNAVAILABLE")
	if err := noPeople.RefreshGroups(bg, e.tenant, p.ID); err != nil {
		t.Fatal("refresh without a people port is a no-op")
	}
}

func TestRefreshAllSweepsActiveProjectsWithRules(t *testing.T) {
	e := newEnv(t)
	owner, a := e.user("create"), e.user()
	p, q := e.project(owner, "SWA"), e.project(owner, "SWB")
	dept := uuid.New()
	e.people.members[dept] = []uuid.UUID{a.UserID}
	_, _ = e.svc.AddGroupRule(bg, owner, p.ID, GroupRuleInput{Kind: "department", RefID: dept, Role: "member"})
	_, _ = e.svc.AddGroupRule(bg, owner, q.ID, GroupRuleInput{Kind: "department", RefID: dept, Role: "viewer"})
	_, _ = e.svc.Archive(bg, owner, q.ID)
	e.people.members[dept] = nil
	if err := e.svc.RefreshAll(bg); err != nil {
		t.Fatal(err)
	}
	if role, _ := roleOf(e, p, a); role != "" {
		t.Fatal("active project was refreshed")
	}
	if role, _ := roleOf(e, q, a); role != "viewer" {
		t.Fatal("archived projects are not refreshed")
	}
	e.people.err = errors.New("down")
	if err := e.svc.RefreshAll(bg); err == nil {
		t.Fatal("a failing lookup must surface")
	}
}

// Specification section 7: a deactivated login leaves its projects and assignments, history stays; redelivery is a no-op.
func TestDeactivationAndInbox(t *testing.T) {
	e := newEnv(t)
	owner, leaver := e.user("create"), e.user()
	p, q := e.project(owner, "DEA"), e.project(owner, "DEB")
	for _, pr := range []*Project{p, q} {
		_, _ = e.svc.AddMember(bg, owner, pr.ID, MemberInput{UserID: leaver.UserID, Role: "member"})
	}
	todo := e.columnByKey(p, "todo")
	tid := e.insertTask(p, todo, 1, nil)
	e.db.Exec(`INSERT INTO task_assignees (task_id, user_id, tenant_id, assigned_by) VALUES (?, ?, ?, ?)`, tid, leaver.UserID, e.tenant, owner.UserID)

	envelope := func(key string, event uuid.UUID) []byte {
		raw, _ := json.Marshal(map[string]any{"event_id": event, "routing_key": key, "tenant_id": e.tenant, "payload": map[string]any{"user_id": leaver.UserID}})
		return raw
	}
	ev := uuid.New()
	if err := e.svc.HandleIdentityEvent(bg, envelope("identity.user.activated", uuid.New())); err != nil {
		t.Fatal("unknown event types are ignored")
	}
	if e.count(`SELECT count(*) FROM project_members WHERE user_id = ? AND active`, leaver.UserID) != 2 {
		t.Fatal("ignored event must change nothing")
	}
	if err := e.svc.HandleIdentityEvent(bg, envelope("identity.user.deactivated", ev)); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.HandleIdentityEvent(bg, envelope("identity.user.deactivated", ev)); err != nil {
		t.Fatalf("redelivery: %v", err)
	}
	if e.count(`SELECT count(*) FROM project_members WHERE user_id = ? AND active`, leaver.UserID) != 0 ||
		e.count(`SELECT count(*) FROM project_members WHERE user_id = ?`, leaver.UserID) != 2 {
		t.Fatal("memberships must be marked inactive, not deleted")
	}
	if e.count(`SELECT count(*) FROM task_assignees WHERE user_id = ? AND NOT active`, leaver.UserID) != 1 {
		t.Fatal("assignment kept but inactive")
	}
	if e.count(`SELECT count(*) FROM activity WHERE type = 'member.deactivated'`) != 2 ||
		e.count(`SELECT count(*) FROM projects_events_outbox WHERE payload->'payload'->>'reason' = 'deactivated'`) != 2 {
		t.Fatal("redelivery must not repeat the effect (PG10)")
	}
	if _, err := e.svc.Access(bg, nil, leaver, p.ID); err == nil {
		t.Fatal("a deactivated member loses access")
	}
	for _, bad := range [][]byte{[]byte("{"), []byte(`{"routing_key":"identity.user.deactivated"}`)} {
		if err := e.svc.HandleIdentityEvent(bg, bad); err == nil {
			t.Fatalf("bad envelope accepted: %s", bad)
		}
	}
}

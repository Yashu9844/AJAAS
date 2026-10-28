package project

import (
	"testing"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"gorm.io/gorm"
)

func (e *env) watch(taskID uuid.UUID, user uuid.UUID) {
	e.t.Helper()
	if err := e.db.Exec(`INSERT INTO task_watchers (task_id, user_id, tenant_id, reason) VALUES (?, ?, ?, 'explicit')`, taskID, user, e.tenant).Error; err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) watching(user uuid.UUID) int64 {
	return e.count(`SELECT count(*) FROM task_watchers WHERE user_id = ?`, user)
}

// CM-03: losing access to a project ends the right to follow its tasks - on removal, on a group rule change and on
// deactivation of the login.
func TestWatchesEndWhenAccessIsLost(t *testing.T) {
	e := newEnv(t)
	owner, removed, grouped, leaver := e.user("create"), e.user(), e.user(), e.user()
	p, q := e.project(owner, "WTA"), e.project(owner, "WTB")
	tp, tq := e.insertTask(p, e.columnByKey(p, "todo"), 1, nil), e.insertTask(q, e.columnByKey(q, "todo"), 1, nil)
	for _, u := range []api.Actor{removed, leaver} {
		for _, pr := range []*Project{p, q} {
			_, _ = e.svc.AddMember(bg, owner, pr.ID, MemberInput{UserID: u.UserID, Role: "member"})
		}
	}
	for _, u := range []uuid.UUID{removed.UserID, leaver.UserID} {
		e.watch(tp, u)
		e.watch(tq, u)
	}

	if err := e.svc.RemoveMember(bg, owner, p.ID, removed.UserID); err != nil {
		t.Fatal(err)
	}
	if e.count(`SELECT count(*) FROM task_watchers WHERE user_id = ? AND task_id = ?`, removed.UserID, tp) != 0 || e.count(`SELECT count(*) FROM task_watchers WHERE user_id = ? AND task_id = ?`, removed.UserID, tq) != 1 {
		t.Fatal("removal ends the watches of that project only")
	}

	dept := uuid.New()
	e.people.members[dept] = []uuid.UUID{grouped.UserID}
	rule, err := e.svc.AddGroupRule(bg, owner, p.ID, GroupRuleInput{Kind: "department", RefID: dept, Role: "member"})
	if err != nil {
		t.Fatal(err)
	}
	e.watch(tp, grouped.UserID)
	if err = e.svc.RemoveGroupRule(bg, owner, p.ID, rule.ID); err != nil {
		t.Fatal(err)
	}
	if e.watching(grouped.UserID) != 0 {
		t.Fatal("a group member who leaves with the rule stops watching")
	}

	err = e.svc.inTx(bg, func(tx *gorm.DB) error {
		return e.svc.DeactivateUserMappings(bg, tx, e.tenant, leaver.UserID, uuid.Nil)
	})
	if err != nil || e.watching(leaver.UserID) != 0 {
		t.Fatalf("deactivation ends every watch: %v %d", err, e.watching(leaver.UserID))
	}
}

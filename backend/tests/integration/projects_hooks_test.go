package integration

import (
	"fmt"
	"testing"
)

// CM-02 / CM-03 / PG11 across contexts: a task description mentions only people who can read the project (the others are
// dropped without a trace), the reporter, the assignee and the mentioned member become watchers automatically, and an
// edited description replaces the mention set.
func TestTaskDescriptionMentionsAndAutoWatchers(t *testing.T) {
	tc := newTenant(t)
	owner, _, ownerID := tc.limitedUser(t, [2]string{"projects", "create"})
	_, memberEmail := tc.newUser(t, "mem")
	memberID := tc.userIDByEmail(t, memberEmail)
	outsiderID, _ := tc.newUser(t, "out")
	assigneeID, _ := tc.newUser(t, "asg")

	pid := tc.asTok(t, owner, "POST", "/projects", body(map[string]any{"key": "MEN" + uniq()[:2], "name": "Mentions"})).str("data.id")
	for _, u := range []string{memberID, assigneeID} {
		expect(t, tc.asTok(t, owner, "POST", "/projects/"+pid+"/members", body(map[string]any{"user_id": u, "role": "member"})), 201, "member")
	}
	mention := func(id string) string { return fmt.Sprintf("@[Name](user:%s)", id) }

	created := tc.asTok(t, owner, "POST", "/projects/"+pid+"/tasks", body(map[string]any{
		"title": "Mentioning", "assignee_ids": []string{assigneeID},
		"description": "ping " + mention(memberID) + " and " + mention(outsiderID),
	}))
	expect(t, created, 201, "create task with mentions")
	tid := created.str("data.id")

	reasons := map[string]string{}
	for _, it := range tc.asTok(t, owner, "GET", "/tasks/"+tid+"/watchers").list("data") {
		w := it.(map[string]any)
		reasons[w["user_id"].(string)] = w["reason"].(string)
	}
	if reasons[ownerID] != "reporter" || reasons[assigneeID] != "assignee" || reasons[memberID] != "mentioned" {
		t.Fatalf("watchers: %v", reasons)
	}
	if _, leaked := reasons[outsiderID]; leaked {
		t.Fatalf("PG11: a user who cannot read the project must not be mentioned or watch: %v", reasons)
	}
	var stored int64
	testDB.Raw(`SELECT count(*) FROM mentions WHERE task_id = ? AND source_kind = 'task'`, tid).Scan(&stored)
	if stored != 1 {
		t.Fatalf("only the readable mention is stored, got %d", stored)
	}

	// Editing the description replaces the set.
	v := created.get("data.version")
	expect(t, tc.asTok(t, owner, "PATCH", "/tasks/"+tid, body(map[string]any{"description": "no mentions now", "version": v})), 200, "edit description")
	testDB.Raw(`SELECT count(*) FROM mentions WHERE task_id = ? AND source_kind = 'task'`, tid).Scan(&stored)
	if stored != 0 {
		t.Fatalf("an edited description replaces its mentions, got %d", stored)
	}
}

// userIDByEmail finds a user id by the e-mail newUser returned.
func (tc *tenantCtx) userIDByEmail(t *testing.T, email string) string {
	t.Helper()
	for _, it := range tc.do(t, "GET", "/users", query("per_page", "100")).list("data") {
		m := it.(map[string]any)
		if m["email"] == email {
			return m["id"].(string)
		}
	}
	t.Fatalf("user %s not found", email)
	return ""
}

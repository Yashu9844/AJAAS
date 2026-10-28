package task

import (
	"testing"

	"github.com/google/uuid"
)

// TestInactiveAssigneesAreHistory: a deactivated login stays on the task as history but no longer counts.
func TestInactiveAssigneesAreHistory(t *testing.T) {
	e := newEnv(t)
	owner := e.user()
	p := e.project(owner, "INA")
	gone := e.member(owner, p, "member")
	v := e.mk(owner, p, "t", func(in *CreateInput) { in.AssigneeIDs = []uuid.UUID{gone.UserID, owner.UserID} })
	e.db.Exec(`UPDATE task_assignees SET active = FALSE WHERE task_id = ? AND user_id = ?`, v.ID, gone.UserID)
	got, err := e.svc.Get(bg, owner, v.ID)
	if err != nil || len(got.AssigneeIDs) != 1 || got.AssigneeIDs[0] != owner.UserID || len(got.Assignees) != 2 {
		t.Fatalf("view: %+v %v", got, err)
	}
	active := map[uuid.UUID]bool{}
	for _, a := range got.Assignees {
		active[a.UserID] = a.Active
	}
	if active[gone.UserID] || !active[owner.UserID] {
		t.Fatalf("flags: %v", active)
	}
	rows, _, _ := e.svc.List(bg, owner, p.ID, ListFilter{AssigneeID: &gone.UserID})
	if len(rows) != 0 {
		t.Fatal("inactive assignees do not match the assignee filter")
	}
	dup, _ := e.svc.Duplicate(bg, owner, v.ID, DuplicateInput{CopyAssignees: true})
	if len(dup.Assignees) != 1 {
		t.Fatalf("copies carry active assignees only: %+v", dup.Assignees)
	}
	back, err := e.svc.AddAssignees(bg, owner, v.ID, UserIDsInput{UserIDs: []uuid.UUID{gone.UserID}})
	if err != nil || len(back.AssigneeIDs) != 2 || back.Version != got.Version+1 {
		t.Fatalf("assigning again reactivates: %+v %v", back, err)
	}
}

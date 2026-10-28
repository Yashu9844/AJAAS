package task

import (
	"testing"

	"github.com/google/uuid"
)

func TestMoveProject(t *testing.T) {
	e := newEnv(t)
	owner := e.user()
	src, dst := e.project(owner, "SRC"), e.project(owner, "DST")
	member := e.member(owner, src, "member")
	e.mk(owner, dst, "existing")
	bugSrc, keepSrc := e.label(owner, src, "bug"), e.label(owner, src, "only-src")
	bugDst := e.label(owner, dst, "BUG")
	msSrc, msDst := e.milestone(owner, src, "Beta"), e.milestone(owner, dst, "beta")
	prog := e.col(src, "in-progress")
	v := e.mk(owner, src, "mover", func(in *CreateInput) {
		in.ColumnID, in.MilestoneID, in.LabelIDs = &prog, &msSrc.ID, []uuid.UUID{bugSrc.ID, keepSrc.ID}
		in.AssigneeIDs = []uuid.UUID{member.UserID, owner.UserID}
	})
	got, err := e.svc.MoveProject(bg, owner, v.ID, MoveProjectInput{ProjectID: dst.ID, Version: num(1)})
	if err != nil {
		t.Fatal(err)
	}
	if got.ProjectID != dst.ID || got.Number != 2 || got.Key != "DST-2" || got.Version != 2 || got.ColumnID != e.col(dst, "in-progress") {
		t.Fatalf("moved: %+v", got)
	}
	if got.MilestoneID == nil || *got.MilestoneID != msDst.ID || got.SprintID != nil {
		t.Fatalf("milestone maps by name, sprint cleared: %+v", got)
	}
	if len(got.LabelIDs) != 1 || got.LabelIDs[0] != bugDst.ID {
		t.Fatalf("labels map by name and unmatched ones are dropped: %v", got.LabelIDs)
	}
	if len(got.AssigneeIDs) != 1 || got.AssigneeIDs[0] != owner.UserID {
		t.Fatalf("assignees who cannot read the target are dropped: %v", got.AssigneeIDs)
	}
	old, err := e.svc.ByKey(bg, owner, "src-1")
	if err != nil || old.ID != v.ID || old.Key != "DST-2" {
		t.Fatalf("the old ticket id keeps resolving: %+v %v", old, err)
	}
	if _, err = e.svc.ByKey(bg, owner, "DST-2"); err != nil {
		t.Fatalf("new id: %v", err)
	}
	if e.count(`SELECT count(*) FROM activity WHERE task_id = ? AND type IN ('moved_out', 'moved_in')`, v.ID) != 2 {
		t.Fatalf("history kept and extended: %v", e.activityTypes(v.ID))
	}
	back, err := e.svc.MoveProject(bg, owner, v.ID, MoveProjectInput{ProjectID: src.ID})
	if err != nil || back.Key != "SRC-2" {
		t.Fatalf("moving back gets a new number: %+v %v", back, err)
	}
	for _, key := range []string{"SRC-1", "DST-2", "SRC-2"} {
		if r, err := e.svc.ByKey(bg, owner, key); err != nil || r.ID != v.ID {
			t.Fatalf("%s: %v", key, err)
		}
	}
}

func TestMoveProjectRules(t *testing.T) {
	e := newEnv(t)
	owner, other := e.user(), e.user()
	src, dst, theirs := e.project(owner, "MPA"), e.project(owner, "MPB"), e.project(other, "MPC")
	member := e.member(owner, src, "member")
	v := e.mk(owner, src, "t")
	cases := []struct {
		name string
		in   MoveProjectInput
		want string
	}{
		{"same project", MoveProjectInput{ProjectID: src.ID}, "VALIDATION_ERROR"},
		{"unreadable target", MoveProjectInput{ProjectID: theirs.ID}, "NOT_FOUND"},
		{"unknown target", MoveProjectInput{ProjectID: uuid.New()}, "NOT_FOUND"},
		{"stale version", MoveProjectInput{ProjectID: dst.ID, Version: num(9)}, "VERSION_CONFLICT"},
		{"column of another project", MoveProjectInput{ProjectID: dst.ID, ColumnID: ptrID(e.col(src, "done"))}, "VALIDATION_ERROR"},
	}
	for _, tc := range cases {
		if _, err := e.svc.MoveProject(bg, owner, v.ID, tc.in); code(err) != tc.want {
			t.Errorf("%s: %v", tc.name, err)
		}
	}
	if _, err := e.svc.MoveProject(bg, member, v.ID, MoveProjectInput{ProjectID: dst.ID}); code(err) != "NOT_FOUND" {
		t.Fatalf("a member of the source only cannot see the target: %v", err)
	}
	if _, err := e.proj.AddMember(bg, owner, dst.ID, memberIn(member.UserID, "viewer")); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.MoveProject(bg, member, v.ID, MoveProjectInput{ProjectID: dst.ID}); code(err) != "FORBIDDEN" {
		t.Fatalf("task.create is needed in the target: %v", err)
	}
	parent := e.mk(owner, src, "p")
	kid := e.mk(owner, src, "k", func(in *CreateInput) { in.ParentTaskID = &parent.ID })
	for _, id := range []uuid.UUID{parent.ID, kid.ID} {
		if _, err := e.svc.MoveProject(bg, owner, id, MoveProjectInput{ProjectID: dst.ID}); code(err) != "TASK_HAS_HIERARCHY" {
			t.Errorf("hierarchy: %v", err)
		}
	}
	hard := e.addColumn(owner, dst, "Hard", "in_progress", num(1), true)
	e.mk(owner, dst, "fill", func(in *CreateInput) { in.ColumnID = &hard })
	if _, err := e.svc.MoveProject(bg, owner, v.ID, MoveProjectInput{ProjectID: dst.ID, ColumnID: &hard}); code(err) != "WIP_LIMIT" {
		t.Fatalf("the WIP limit of the target applies: %v", err)
	}
	blocker := e.mk(owner, src, "blocker")
	if _, err := e.svc.AddRelation(bg, owner, blocker.ID, RelationInput{TargetTaskID: v.ID, Type: "blocks"}); err != nil {
		t.Fatal(err)
	}
	done := e.col(dst, "done")
	if _, err := e.svc.MoveProject(bg, owner, v.ID, MoveProjectInput{ProjectID: dst.ID, ColumnID: &done}); code(err) != "TASK_BLOCKED" {
		t.Fatalf("the done rules apply: %v", err)
	}
	if got, err := e.svc.MoveProject(bg, owner, blocker.ID, MoveProjectInput{ProjectID: dst.ID, ColumnID: &done}); err != nil || got.CompletedAt == nil {
		t.Fatalf("moving into a done column completes: %+v %v", got, err)
	}
}

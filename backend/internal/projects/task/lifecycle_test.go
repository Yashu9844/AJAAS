package task

import (
	"testing"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/project"
)

func TestAssigneesAndLabels(t *testing.T) {
	e := newEnv(t)
	owner := e.user()
	p, q := e.project(owner, "ASG"), e.project(owner, "ASH")
	member, viewer := e.member(owner, p, "member"), e.member(owner, p, "viewer")
	bug, other := e.label(owner, p, "bug"), e.label(owner, q, "x")
	tenantLabel, err := e.proj.CreateLabel(bg, e.user("admin"), nil, project.LabelInput{Name: "global"})
	if err != nil {
		t.Fatal(err)
	}
	v := e.mk(member, p, "t")
	got, err := e.svc.AddAssignees(bg, member, v.ID, UserIDsInput{UserIDs: []uuid.UUID{member.UserID, viewer.UserID}})
	if err != nil || len(got.AssigneeIDs) != 2 || got.Version != 2 {
		t.Fatalf("assign: %+v %v", got, err)
	}
	again, _ := e.svc.AddAssignees(bg, member, v.ID, UserIDsInput{UserIDs: []uuid.UUID{member.UserID}})
	if again.Version != 2 {
		t.Fatalf("assigning an assignee again changes nothing: %d", again.Version)
	}
	if _, err = e.svc.AddAssignees(bg, member, v.ID, UserIDsInput{UserIDs: []uuid.UUID{uuid.New()}}); code(err) != "VALIDATION_ERROR" {
		t.Fatalf("non-member assignee: %v", err)
	}
	if _, err = e.svc.AddAssignees(bg, viewer, v.ID, UserIDsInput{UserIDs: []uuid.UUID{viewer.UserID}}); code(err) != "FORBIDDEN" {
		t.Fatalf("viewer cannot assign: %v", err)
	}
	if got, err = e.svc.RemoveAssignee(bg, member, v.ID, viewer.UserID); err != nil || len(got.AssigneeIDs) != 1 {
		t.Fatalf("unassign: %+v %v", got, err)
	}
	if noop, _ := e.svc.RemoveAssignee(bg, member, v.ID, uuid.New()); noop.Version != got.Version {
		t.Fatal("removing a non-assignee is a no-op")
	}
	if len(e.outbox("task.assigned")) != 1 || len(e.outbox("task.unassigned")) != 1 {
		t.Fatal("assignment events")
	}
	if got, err = e.svc.AddLabels(bg, member, v.ID, LabelIDsInput{LabelIDs: []uuid.UUID{bug.ID, tenantLabel.ID}}); err != nil || len(got.LabelIDs) != 2 {
		t.Fatalf("labels: %+v %v", got, err)
	}
	if _, err = e.svc.AddLabels(bg, member, v.ID, LabelIDsInput{LabelIDs: []uuid.UUID{other.ID}}); code(err) != "VALIDATION_ERROR" {
		t.Fatalf("label of another project: %v", err)
	}
	if got, err = e.svc.RemoveLabel(bg, member, v.ID, bug.ID); err != nil || len(got.LabelIDs) != 1 {
		t.Fatalf("remove label: %+v %v", got, err)
	}
	if _, err = e.svc.AddLabels(bg, viewer, v.ID, LabelIDsInput{LabelIDs: []uuid.UUID{bug.ID}}); code(err) != "FORBIDDEN" {
		t.Fatalf("viewer cannot label: %v", err)
	}
}

func TestTooManyAssignees(t *testing.T) {
	e := newEnv(t)
	owner := e.user()
	p := e.project(owner, "MAN")
	v := e.mk(owner, p, "t")
	ids := make([]uuid.UUID, 0, 21)
	for i := 0; i < 21; i++ {
		ids = append(ids, e.member(owner, p, "member").UserID)
	}
	if _, err := e.svc.AddAssignees(bg, owner, v.ID, UserIDsInput{UserIDs: ids}); code(err) != "VALIDATION_ERROR" {
		t.Fatalf("21 assignees: %v", err)
	}
	if _, err := e.svc.AddAssignees(bg, owner, v.ID, UserIDsInput{UserIDs: ids[:20]}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.AddAssignees(bg, owner, v.ID, UserIDsInput{UserIDs: ids[20:]}); code(err) != "VALIDATION_ERROR" {
		t.Fatalf("the 21st assignee: %v", err)
	}
}

func TestArchiveRestoreDelete(t *testing.T) {
	e := newEnv(t)
	owner := e.user()
	p := e.project(owner, "ARC")
	member := e.member(owner, p, "member")
	parent := e.mk(owner, p, "parent")
	kid := e.mk(owner, p, "kid", func(in *CreateInput) { in.ParentTaskID = &parent.ID })
	got, err := e.svc.Archive(bg, member, parent.ID)
	if err != nil || got.ArchivedAt == nil || got.Version != 2 {
		t.Fatalf("archive: %+v %v", got, err)
	}
	if again, _ := e.svc.Archive(bg, member, parent.ID); again.Version != 2 {
		t.Fatal("archiving twice changes nothing")
	}
	if _, err = e.svc.Update(bg, member, parent.ID, UpdateInput{Version: num(2), Title: str("x")}); code(err) != "TASK_ARCHIVED" {
		t.Fatalf("edit while archived: %v", err)
	}
	if rows, _, _ := e.svc.List(bg, member, p.ID, ListFilter{}); len(rows) != 1 {
		t.Fatalf("archived tasks leave the default list: %d", len(rows))
	}
	if got, err = e.svc.Restore(bg, member, parent.ID); err != nil || got.ArchivedAt != nil {
		t.Fatalf("restore: %+v %v", got, err)
	}
	if again, _ := e.svc.Restore(bg, member, parent.ID); again.Version != got.Version {
		t.Fatal("restoring a live task changes nothing")
	}
	if err = e.svc.Delete(bg, member, parent.ID); code(err) != "FORBIDDEN" {
		t.Fatalf("members cannot delete: %v", err)
	}
	if err = e.svc.Delete(bg, owner, parent.ID); err != nil {
		t.Fatal(err)
	}
	for _, id := range []uuid.UUID{parent.ID, kid.ID} {
		if _, err = e.svc.Get(bg, owner, id); code(err) != "NOT_FOUND" {
			t.Fatalf("deleted task %s is gone: %v", id, err)
		}
	}
	if e.count(`SELECT count(*) FROM tasks WHERE id IN (?, ?) AND deleted_at IS NOT NULL`, parent.ID, kid.ID) != 2 {
		t.Fatal("soft delete keeps the rows and cascades to subtasks")
	}
	if err = e.svc.Delete(bg, owner, parent.ID); code(err) != "NOT_FOUND" {
		t.Fatalf("delete twice: %v", err)
	}
	if len(e.outbox("task.archived")) != 2 {
		t.Fatalf("archive and delete events: %d", len(e.outbox("task.archived")))
	}
}

func TestDuplicate(t *testing.T) {
	e := newEnv(t)
	owner := e.user()
	p := e.project(owner, "DUP")
	member := e.member(owner, p, "member")
	bug := e.label(owner, p, "bug")
	done := e.col(p, "done")
	src := e.mk(owner, p, "Original", func(in *CreateInput) {
		in.Description, in.Priority, in.AssigneeIDs, in.LabelIDs = "body", "high", []uuid.UUID{member.UserID}, []uuid.UUID{bug.ID}
	})
	after := e.mk(owner, p, "After")
	cp, err := e.svc.Duplicate(bg, owner, src.ID, DuplicateInput{})
	if err != nil || cp.Title != "Original (copy)" || cp.Number != 3 || cp.Description != "body" || cp.Priority != "high" || len(cp.AssigneeIDs) != 0 || len(cp.LabelIDs) != 0 {
		t.Fatalf("plain copy: %+v %v", cp, err)
	}
	if order := e.order(e.col(p, "todo")); order[1] != cp.ID || order[2] != after.ID {
		t.Fatalf("the copy sits right after the original: %v", order)
	}
	full, err := e.svc.Duplicate(bg, owner, src.ID, DuplicateInput{CopyAssignees: true, CopyLabels: true})
	if err != nil || len(full.AssigneeIDs) != 1 || len(full.LabelIDs) != 1 {
		t.Fatalf("full copy: %+v %v", full, err)
	}
	long := e.mk(owner, p, repeat("x", 300))
	if cp, err = e.svc.Duplicate(bg, owner, long.ID, DuplicateInput{}); err != nil || runes(cp.Title) != 300 {
		t.Fatalf("title stays within 300 characters: %d %v", runes(cp.Title), err)
	}
	if _, err = e.svc.Move(bg, owner, src.ID, MoveInput{ColumnID: &done}); err != nil {
		t.Fatal(err)
	}
	if cp, err = e.svc.Duplicate(bg, owner, src.ID, DuplicateInput{}); err != nil || cp.CompletedAt == nil {
		t.Fatalf("a copy in a done column is completed: %+v %v", cp, err)
	}
	viewer := e.member(owner, p, "viewer")
	if _, err = e.svc.Duplicate(bg, viewer, src.ID, DuplicateInput{}); code(err) != "FORBIDDEN" {
		t.Fatalf("viewer: %v", err)
	}
	if _, err = e.svc.Duplicate(bg, e.user(), src.ID, DuplicateInput{}); code(err) != "NOT_FOUND" {
		t.Fatalf("stranger: %v", err)
	}
	e.svc.Archive(bg, owner, after.ID)
	if cp, err = e.svc.Duplicate(bg, owner, after.ID, DuplicateInput{}); err != nil || cp.ArchivedAt != nil {
		t.Fatalf("copying an archived task gives a live task at the end: %+v %v", cp, err)
	}
}

func repeat(s string, n int) string {
	out := ""
	for i := 0; i < n; i++ {
		out += s
	}
	return out
}

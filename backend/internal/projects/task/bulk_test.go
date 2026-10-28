package task

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestBulkUpdatePartialFailures(t *testing.T) {
	e := newEnv(t)
	owner := e.user()
	p, q := e.project(owner, "BLK"), e.project(owner, "BLQ")
	member := e.member(owner, p, "member")
	bug := e.label(owner, p, "bug")
	ms := e.milestone(owner, p, "M1")
	done, prog := e.col(p, "done"), e.col(p, "in-progress")
	a, b, c := e.mk(owner, p, "a"), e.mk(owner, p, "b"), e.mk(owner, p, "c")
	foreign := e.mk(owner, q, "foreign")
	archived := e.mk(owner, p, "archived")
	if _, err := e.svc.Archive(bg, owner, archived.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.AddRelation(bg, owner, a.ID, RelationInput{TargetTaskID: b.ID, Type: "blocks"}); err != nil {
		t.Fatal(err)
	}
	out, err := e.svc.Bulk(bg, owner, p.ID, BulkInput{
		TaskIDs:  []uuid.UUID{b.ID, a.ID, c.ID, foreign.ID, uuid.New(), archived.ID},
		ColumnID: &done, Priority: str("high"), AssigneesAdd: []uuid.UUID{member.UserID}, LabelsAdd: []uuid.UUID{bug.ID},
		MilestoneID: Opt[uuid.UUID]{Set: true, Val: &ms.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	wantErr := map[int]string{0: "TASK_BLOCKED", 3: "NOT_FOUND", 4: "NOT_FOUND", 5: "TASK_ARCHIVED"}
	if out.Succeeded != 2 || out.Failed != 4 || len(out.Results) != 6 {
		t.Fatalf("outcome: %+v", out)
	}
	for i, r := range out.Results {
		want := wantErr[i]
		if (want == "") != r.OK || (want != "" && r.Error.Code != want) {
			t.Errorf("result %d: %+v", i, r)
		}
	}
	for _, id := range []uuid.UUID{a.ID, c.ID} {
		v, _ := e.svc.Get(bg, owner, id)
		if v.ColumnCategory != "done" || v.Priority != "high" || len(v.AssigneeIDs) != 1 || len(v.LabelIDs) != 1 || v.MilestoneID == nil || v.Version != 2 {
			t.Errorf("task %s: %+v", id, v)
		}
	}
	if v, _ := e.svc.Get(bg, owner, b.ID); v.Priority != "none" || v.ColumnCategory != "todo" || v.Version != 1 {
		t.Fatalf("a failed task is untouched (its transaction rolled back): %+v", v)
	}
	if foreignV, _ := e.svc.Get(bg, owner, foreign.ID); foreignV.Version != 1 {
		t.Fatal("a task of another project is never touched")
	}

	out, err = e.svc.Bulk(bg, owner, p.ID, BulkInput{TaskIDs: []uuid.UUID{a.ID, c.ID}, ColumnID: &prog, AssigneesRemove: []uuid.UUID{member.UserID},
		LabelsRemove: []uuid.UUID{bug.ID}, MilestoneID: Opt[uuid.UUID]{Set: true}, Archive: boolp(true)})
	if err != nil || out.Succeeded != 2 {
		t.Fatalf("second round: %+v %v", out, err)
	}
	v, _ := e.svc.Get(bg, owner, a.ID)
	if v.ArchivedAt == nil || len(v.AssigneeIDs) != 0 || len(v.LabelIDs) != 0 || v.MilestoneID != nil || v.ColumnID != prog {
		t.Fatalf("combined changes: %+v", v)
	}
	out, _ = e.svc.Bulk(bg, owner, p.ID, BulkInput{TaskIDs: []uuid.UUID{a.ID, archived.ID}, Archive: boolp(false)})
	if out.Succeeded != 2 {
		t.Fatalf("restore: %+v", out)
	}
}

func TestBulkValidationAndPermissions(t *testing.T) {
	e := newEnv(t)
	owner := e.user()
	p := e.project(owner, "BVP")
	viewer := e.member(owner, p, "viewer")
	member := e.member(owner, p, "member")
	v := e.mk(owner, p, "t")
	many := make([]uuid.UUID, 101)
	for i := range many {
		many[i] = uuid.New()
	}
	cases := map[string]BulkInput{
		"no tasks":     {Priority: str("low")},
		"101 tasks":    {TaskIDs: many, Priority: str("low")},
		"no change":    {TaskIDs: []uuid.UUID{v.ID}},
		"bad priority": {TaskIDs: []uuid.UUID{v.ID}, Priority: str("max")},
	}
	for name, in := range cases {
		if _, err := e.svc.Bulk(bg, owner, p.ID, in); code(err) != "VALIDATION_ERROR" {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := e.svc.Bulk(bg, viewer, p.ID, BulkInput{TaskIDs: []uuid.UUID{v.ID}, Priority: str("low")}); code(err) != "FORBIDDEN" {
		t.Fatalf("viewer: %v", err)
	}
	if _, err := e.svc.Bulk(bg, e.user(), p.ID, BulkInput{TaskIDs: []uuid.UUID{v.ID}, Priority: str("low")}); code(err) != "NOT_FOUND" {
		t.Fatalf("stranger: %v", err)
	}
	custom, err := e.proj.CreateRole(bg, e.user("admin"), roleIn("Updater", "project.read", "task.read", "task.update"))
	if err != nil {
		t.Fatal(err)
	}
	cu := e.user()
	if _, err = e.proj.AddMember(bg, owner, p.ID, customMemberIn(cu.UserID, custom.ID)); err != nil {
		t.Fatal(err)
	}
	out, err := e.svc.Bulk(bg, cu, p.ID, BulkInput{TaskIDs: []uuid.UUID{v.ID}, AssigneesAdd: []uuid.UUID{member.UserID}})
	if err != nil || out.Failed != 1 || out.Results[0].Error.Code != "FORBIDDEN" {
		t.Fatalf("each task is checked for the right permission (task.assign): %+v %v", out, err)
	}
	out, _ = e.svc.Bulk(bg, cu, p.ID, BulkInput{TaskIDs: []uuid.UUID{v.ID, v.ID}, Priority: str("low")})
	if out.Succeeded != 1 || len(out.Results) != 1 {
		t.Fatalf("duplicate ids collapse: %+v", out)
	}
}

func TestBulkVersionConflictMessage(t *testing.T) {
	info := errInfo(&versionConflictForTest)
	if info.Code != "VERSION_CONFLICT" || strings.Contains(info.Message, "version 7") {
		t.Fatalf("%+v", info)
	}
	if got := errInfo(errPlain{}); got.Code != "INTERNAL_ERROR" {
		t.Fatalf("raw errors never reach clients: %+v", got)
	}
}

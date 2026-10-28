package project

import (
	"testing"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
)

func str(s string) *string { return &s }
func num(n int) *int       { return &n }

func (e *env) columnByKey(p *Project, key string) Column {
	var c Column
	e.db.Where("project_id = ? AND key = ?", p.ID, key).Take(&c)
	return c
}

// insertTask adds a minimal task row so column and milestone rules have something to act on.
func (e *env) insertTask(p *Project, col Column, n int, milestone *uuid.UUID) uuid.UUID {
	id := uuid.New()
	err := e.db.Exec(`INSERT INTO tasks (id, tenant_id, project_id, number, title, column_id, reporter_user_id, rank, milestone_id)
		VALUES (?, ?, ?, ?, 'T', ?, ?, ?, ?)`, id, p.TenantID, p.ID, n, col.ID, uuid.New(), kernel.RankBetween("", ""), milestone).Error
	if err != nil {
		e.t.Fatal(err)
	}
	return id
}

func TestColumnLifecycle(t *testing.T) {
	e := newEnv(t)
	owner, member := e.user("create"), e.user()
	p := e.project(owner, "COL")
	_, _ = e.svc.AddMember(bg, owner, p.ID, MemberInput{UserID: member.UserID, Role: "member"})
	if _, err := e.svc.CreateColumn(bg, member, p.ID, ColumnInput{Name: str("Review")}); err == nil {
		t.Fatal("members cannot manage columns")
	}
	review, err := e.svc.CreateColumn(bg, owner, p.ID, ColumnInput{Name: str("In Review"), Category: str("in_progress"), WipLimit: num(3), WipHard: boolp(true)})
	if err != nil || review.Key != "in-review" || review.Position != 4 || review.WipLimit == nil || !review.WipHard {
		t.Fatalf("create: %+v %v", review, err)
	}
	dup, _ := e.svc.CreateColumn(bg, owner, p.ID, ColumnInput{Name: str("In Review")})
	if dup.Key != "in-review-2" {
		t.Fatalf("key collision: %s", dup.Key)
	}
	for _, in := range []ColumnInput{{}, {Name: str("x"), Category: str("weird")}, {Name: str("x"), Color: str("blue")}, {Name: str("x"), WipLimit: num(0)}} {
		if _, err = e.svc.CreateColumn(bg, owner, p.ID, in); err == nil {
			t.Errorf("accepted %+v", in)
		}
	}
	got, err := e.svc.UpdateColumn(bg, owner, p.ID, review.ID, ColumnInput{Name: str("QA"), ClearWip: true, IsDefault: boolp(true)})
	if err != nil || got.Name != "QA" || got.WipLimit != nil || !got.IsDefault {
		t.Fatalf("update: %+v %v", got, err)
	}
	if e.count(`SELECT count(*) FROM board_columns WHERE project_id = ? AND is_default`, p.ID) != 1 {
		t.Fatal("exactly one default column")
	}
	done := e.columnByKey(p, "done")
	_, err = e.svc.UpdateColumn(bg, owner, p.ID, done.ID, ColumnInput{Category: str("canceled")})
	wantCode(t, err, "COLUMN_INVARIANT")
	if _, err = e.svc.UpdateColumn(bg, owner, p.ID, uuid.New(), ColumnInput{Name: str("x")}); err == nil {
		t.Fatal("unknown column")
	}
}

func boolp(b bool) *bool { return &b }

func TestReorderAndDeleteColumns(t *testing.T) {
	e := newEnv(t)
	owner := e.user("create")
	p := e.project(owner, "ORD")
	cols, _ := e.svc.Columns(bg, owner, p.ID)
	ids := make([]uuid.UUID, len(cols))
	for i, c := range cols {
		ids[len(cols)-1-i] = c.ID
	}
	re, err := e.svc.ReorderColumns(bg, owner, p.ID, ids)
	if err != nil || re[0].ID != ids[0] || re[3].ID != ids[3] {
		t.Fatalf("reorder: %v %v", re, err)
	}
	if _, err = e.svc.ReorderColumns(bg, owner, p.ID, ids[:2]); err == nil {
		t.Fatal("partial reorder")
	}
	if _, err = e.svc.ReorderColumns(bg, owner, p.ID, append(ids[:3:3], ids[0])); err == nil {
		t.Fatal("repeated id")
	}
	progress, todo, doneCol := e.columnByKey(p, "in-progress"), e.columnByKey(p, "todo"), e.columnByKey(p, "done")
	e.insertTask(p, progress, 1, nil)
	e.insertTask(p, progress, 2, nil)
	wantCode(t, e.svc.DeleteColumn(bg, owner, p.ID, progress.ID, nil), "COLUMN_NOT_EMPTY")
	wantCode(t, e.svc.DeleteColumn(bg, owner, p.ID, progress.ID, &progress.ID), "move_to")
	wantCode(t, e.svc.DeleteColumn(bg, owner, p.ID, todo.ID, nil), "DEFAULT_COLUMN")
	if err = e.svc.DeleteColumn(bg, owner, p.ID, progress.ID, &doneCol.ID); err != nil {
		t.Fatal(err)
	}
	if e.count(`SELECT count(*) FROM tasks WHERE column_id = ? AND completed_at IS NOT NULL AND version = 2`, doneCol.ID) != 2 {
		t.Fatal("moved tasks must complete in a done column and bump their version")
	}
	if err = e.svc.DeleteColumn(bg, owner, p.ID, progress.ID, nil); err == nil {
		t.Fatal("deleting twice is 404")
	}
	if err = e.svc.DeleteColumn(bg, owner, p.ID, doneCol.ID, &todo.ID); err == nil {
		t.Fatal("the last done column must stay")
	}
	cols, _ = e.svc.Columns(bg, owner, p.ID)
	if len(cols) != 3 {
		t.Fatalf("columns after failed delete: %d", len(cols))
	}
}

func TestLabelsAndMilestones(t *testing.T) {
	e := newEnv(t)
	owner, member, admin := e.user("create"), e.user(), e.user("admin")
	p := e.project(owner, "LBL")
	_, _ = e.svc.AddMember(bg, owner, p.ID, MemberInput{UserID: member.UserID, Role: "member"})
	l, err := e.svc.CreateLabel(bg, owner, &p.ID, LabelInput{Name: "Bug", Color: "#FF0000"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = e.svc.CreateLabel(bg, owner, &p.ID, LabelInput{Name: "bug"})
	wantCode(t, err, "LABEL_NAME_TAKEN")
	if _, err = e.svc.CreateLabel(bg, member, &p.ID, LabelInput{Name: "x"}); err == nil {
		t.Fatal("members cannot manage labels")
	}
	if _, err = e.svc.CreateLabel(bg, owner, nil, LabelInput{Name: "Global"}); err == nil {
		t.Fatal("tenant labels need manage_all")
	}
	g, err := e.svc.CreateLabel(bg, admin, nil, LabelInput{Name: "Bug"})
	if err != nil {
		t.Fatalf("a tenant label may share a project label name: %v", err)
	}
	all, _ := e.svc.Labels(bg, member, p.ID)
	tenantOnly, _ := e.svc.TenantLabels(bg, member)
	if len(all) != 2 || len(tenantOnly) != 1 || all[0].ID != g.ID {
		t.Fatalf("labels: %d %d", len(all), len(tenantOnly))
	}
	if _, err = e.svc.CreateLabel(bg, owner, &p.ID, LabelInput{Name: "x", Color: "red"}); err == nil {
		t.Fatal("bad color")
	}
	if l, err = e.svc.UpdateLabel(bg, owner, &p.ID, l.ID, LabelInput{Name: "Defect", Color: "#00FF00"}); err != nil || l.Name != "Defect" {
		t.Fatalf("update: %v", err)
	}
	if _, err = e.svc.UpdateLabel(bg, admin, nil, l.ID, LabelInput{Name: "z"}); err == nil {
		t.Fatal("a project label is not reachable through the tenant scope")
	}
	if err = e.svc.DeleteLabel(bg, owner, &p.ID, l.ID); err != nil || e.svc.DeleteLabel(bg, owner, &p.ID, l.ID) == nil {
		t.Fatal("delete once, 404 twice")
	}

	due, _ := kernel.ParseDate("2026-12-01")
	ms, err := e.svc.CreateMilestone(bg, owner, p.ID, MilestoneInput{Name: str("Beta"), DueOn: &due})
	if err != nil || ms.Status != "open" {
		t.Fatalf("milestone: %+v %v", ms, err)
	}
	if _, err = e.svc.CreateMilestone(bg, owner, p.ID, MilestoneInput{Name: str("beta")}); err == nil {
		t.Fatal("duplicate milestone name")
	}
	if _, err = e.svc.CreateMilestone(bg, owner, p.ID, MilestoneInput{}); err == nil {
		t.Fatal("name required")
	}
	if _, err = e.svc.CreateMilestone(bg, member, p.ID, MilestoneInput{Name: str("x")}); err == nil {
		t.Fatal("members cannot manage milestones")
	}
	todo, done := e.columnByKey(p, "todo"), e.columnByKey(p, "done")
	e.insertTask(p, todo, 1, &ms.ID)
	e.insertTask(p, done, 2, &ms.ID)
	list, err := e.svc.Milestones(bg, member, p.ID)
	if err != nil || len(list) != 1 || list[0].Total != 2 || list[0].Done != 1 {
		t.Fatalf("progress: %+v %v", list, err)
	}
	ms, err = e.svc.UpdateMilestone(bg, owner, p.ID, ms.ID, MilestoneInput{Status: str("reached")})
	if err != nil || ms.Status != "reached" {
		t.Fatalf("update: %v", err)
	}
	if _, err = e.svc.UpdateMilestone(bg, owner, p.ID, ms.ID, MilestoneInput{Status: str("later")}); err == nil {
		t.Fatal("bad status")
	}
	if err = e.svc.DeleteMilestone(bg, owner, p.ID, ms.ID); err != nil || e.count(`SELECT count(*) FROM tasks WHERE milestone_id IS NOT NULL`) != 0 {
		t.Fatalf("delete releases tasks: %v", err)
	}
	if e.svc.DeleteMilestone(bg, owner, p.ID, ms.ID) == nil {
		t.Fatal("404 twice")
	}
	_ = api.TaskRead
}

package task

import (
	"testing"

	"github.com/google/uuid"
)

// breakTable renames a table so every statement touching it fails, and returns the function that restores it.
func (e *env) breakTable(name string) func() {
	e.t.Helper()
	if err := e.db.Exec("ALTER TABLE " + name + " RENAME TO " + name + "_broken").Error; err != nil {
		e.t.Fatalf("break %s: %v", name, err)
	}
	return func() { e.db.Exec("ALTER TABLE " + name + "_broken RENAME TO " + name) }
}

func nonNil(name string, fn func() error) func(t *testing.T) {
	return func(t *testing.T) {
		if err := fn(); err == nil {
			t.Errorf("%s: want an error when its tables are unavailable", name)
		} else if c := code(err); c == "" {
			t.Errorf("%s: empty code for %v", name, err)
		}
	}
}

// TestStorageFailuresSurfaceAsErrors breaks one table at a time and checks that the operations that need it fail
// cleanly (an error, nothing half-written) instead of panicking or reporting success.
func TestStorageFailuresSurfaceAsErrors(t *testing.T) {
	e := newEnv(t)
	owner := e.user()
	p, q := e.project(owner, "FLT"), e.project(owner, "FLQ")
	member := e.member(owner, p, "member")
	bug := e.label(owner, p, "bug")
	a, b := e.mk(owner, p, "a"), e.mk(owner, p, "b")
	child := e.mk(owner, p, "child", func(in *CreateInput) { in.ParentTaskID = &a.ID })
	done := e.col(p, "done")
	one := uuid.New()
	ops := map[string]map[string]func() error{
		"activity": {
			"create": func() error { _, err := e.svc.Create(bg, owner, p.ID, CreateInput{Title: "x"}); return err },
			"update": func() error {
				_, err := e.svc.Update(bg, owner, a.ID, UpdateInput{Version: num(1), Title: str("y")})
				return err
			},
			"archive": func() error { _, err := e.svc.Archive(bg, owner, b.ID); return err },
			"relation": func() error {
				_, err := e.svc.AddRelation(bg, owner, a.ID, RelationInput{TargetTaskID: b.ID, Type: "related"})
				return err
			},
			"read": func() error { _, _, err := e.svc.ProjectActivity(bg, owner, p.ID, ActivityFilter{}); return err },
		},
		"projects_events_outbox": {
			"create": func() error { _, err := e.svc.Create(bg, owner, p.ID, CreateInput{Title: "x"}); return err },
			"delete": func() error { return e.svc.Delete(bg, owner, child.ID) },
		},
		"task_assignees": {
			"get": func() error { _, err := e.svc.Get(bg, owner, a.ID); return err },
			"assign": func() error {
				_, err := e.svc.AddAssignees(bg, owner, a.ID, UserIDsInput{UserIDs: []uuid.UUID{member.UserID}})
				return err
			},
			"unassign": func() error { _, err := e.svc.RemoveAssignee(bg, owner, a.ID, member.UserID); return err },
			"create": func() error {
				_, err := e.svc.Create(bg, owner, p.ID, CreateInput{Title: "x", AssigneeIDs: []uuid.UUID{member.UserID}})
				return err
			},
			"duplicate": func() error {
				_, err := e.svc.Duplicate(bg, owner, a.ID, DuplicateInput{CopyAssignees: true})
				return err
			},
			"list": func() error { _, _, err := e.svc.List(bg, owner, p.ID, ListFilter{}); return err },
		},
		"task_labels": {
			"add": func() error {
				_, err := e.svc.AddLabels(bg, owner, a.ID, LabelIDsInput{LabelIDs: []uuid.UUID{bug.ID}})
				return err
			},
			"remove": func() error { _, err := e.svc.RemoveLabel(bg, owner, a.ID, bug.ID); return err },
			"create": func() error {
				_, err := e.svc.Create(bg, owner, p.ID, CreateInput{Title: "x", LabelIDs: []uuid.UUID{bug.ID}})
				return err
			},
			"duplicate": func() error { _, err := e.svc.Duplicate(bg, owner, a.ID, DuplicateInput{CopyLabels: true}); return err },
		},
		"task_relations": {
			"add": func() error {
				_, err := e.svc.AddRelation(bg, owner, a.ID, RelationInput{TargetTaskID: b.ID, Type: "blocks"})
				return err
			},
			"list":   func() error { _, err := e.svc.Relations(bg, owner, a.ID); return err },
			"remove": func() error { return e.svc.RemoveRelation(bg, owner, a.ID, one) },
			"done":   func() error { _, err := e.svc.Move(bg, owner, b.ID, MoveInput{ColumnID: &done}); return err },
		},
		"board_columns": {
			"get":    func() error { _, err := e.svc.Get(bg, owner, a.ID); return err },
			"create": func() error { _, err := e.svc.Create(bg, owner, p.ID, CreateInput{Title: "x"}); return err },
			"move":   func() error { _, err := e.svc.Move(bg, owner, a.ID, MoveInput{ColumnID: &done}); return err },
			"mproj": func() error {
				_, err := e.svc.MoveProject(bg, owner, b.ID, MoveProjectInput{ProjectID: q.ID})
				return err
			},
		},
		"milestones": {
			"create": func() error {
				_, err := e.svc.Create(bg, owner, p.ID, CreateInput{Title: "x", MilestoneID: &one})
				return err
			},
			"update": func() error {
				_, err := e.svc.Update(bg, owner, a.ID, UpdateInput{Version: num(1), MilestoneID: Opt[uuid.UUID]{Set: true, Val: &one}})
				return err
			},
		},
		"sprints": {
			"create": func() error {
				_, err := e.svc.Create(bg, owner, p.ID, CreateInput{Title: "x", SprintID: &one})
				return err
			},
			"update": func() error {
				_, err := e.svc.Update(bg, owner, a.ID, UpdateInput{Version: num(1), SprintID: Opt[uuid.UUID]{Set: true, Val: &one}})
				return err
			},
		},
		"labels": {
			"create": func() error {
				_, err := e.svc.Create(bg, owner, p.ID, CreateInput{Title: "x", LabelIDs: []uuid.UUID{bug.ID}})
				return err
			},
		},
		"project_members": {
			"assign": func() error {
				_, err := e.svc.AddAssignees(bg, owner, a.ID, UserIDsInput{UserIDs: []uuid.UUID{member.UserID}})
				return err
			},
		},
		"task_ticket_alias": {
			"ticket": func() error { _, err := e.svc.ByKey(bg, owner, "FLT-999"); return err },
			"mproj": func() error {
				_, err := e.svc.MoveProject(bg, owner, b.ID, MoveProjectInput{ProjectID: q.ID})
				return err
			},
		},
		"tasks": {
			"get":     func() error { _, err := e.svc.Get(bg, owner, a.ID); return err },
			"ticket":  func() error { _, err := e.svc.ByKey(bg, owner, "FLT-1"); return err },
			"list":    func() error { _, _, err := e.svc.List(bg, owner, p.ID, ListFilter{}); return err },
			"resolve": func() error { _, _, err := e.svc.Task(bg, nil, owner, a.ID); return err },
		},
	}
	for table, set := range ops {
		restore := e.breakTable(table)
		for name, fn := range set {
			t.Run(table+"/"+name, nonNil(table+"/"+name, fn))
		}
		restore()
	}
	if got, err := e.svc.Get(bg, owner, a.ID); err != nil || got.Version != 1 {
		t.Fatalf("nothing half-written: %+v %v", got, err)
	}
}

func TestMisconfiguredProjectsAndSprints(t *testing.T) {
	e := newEnv(t)
	owner := e.user()
	p := e.project(owner, "MIS")
	e.db.Exec(`UPDATE board_columns SET is_default = false WHERE project_id = ?`, p.ID)
	if _, err := e.svc.Create(bg, owner, p.ID, CreateInput{Title: "x"}); code(err) != "NO_DEFAULT_COLUMN" {
		t.Fatalf("no default column: %v", err)
	}
	e.db.Exec(`UPDATE board_columns SET is_default = true WHERE project_id = ? AND key = 'todo'`, p.ID)
	v := e.mk(owner, p, "t")
	sprintID := uuid.New()
	e.db.Exec(`INSERT INTO sprints (id, tenant_id, project_id, name, created_by) VALUES (?, ?, ?, 'S1', ?)`, sprintID, e.tenant, p.ID, owner.UserID)
	got, err := e.svc.Update(bg, owner, v.ID, UpdateInput{Version: num(1), SprintID: Opt[uuid.UUID]{Set: true, Val: &sprintID}})
	if err != nil || got.SprintID == nil {
		t.Fatalf("sprint of the project: %+v %v", got, err)
	}
	out, err := e.svc.Bulk(bg, owner, p.ID, BulkInput{TaskIDs: []uuid.UUID{v.ID}, SprintID: Opt[uuid.UUID]{Set: true}})
	if err != nil || out.Succeeded != 1 {
		t.Fatalf("bulk clears the sprint: %+v %v", out, err)
	}
	// no audit port: mutations still work
	bare := NewService(e.db, e.proj, Ports{})
	if _, err := bare.Create(bg, owner, p.ID, CreateInput{Title: "no audit"}); err != nil {
		t.Fatal(err)
	}
}

func TestMoveProjectColumnFallbacks(t *testing.T) {
	e := newEnv(t)
	owner := e.user()
	src, dst := e.project(owner, "FBA"), e.project(owner, "FBB")
	qa := e.addColumn(owner, src, "QA", "in_progress", nil, false)
	drop := e.addColumn(owner, src, "Dropped", "canceled", nil, false)
	byCategory := e.mk(owner, src, "qa task", func(in *CreateInput) { in.ColumnID = &qa })
	got, err := e.svc.MoveProject(bg, owner, byCategory.ID, MoveProjectInput{ProjectID: dst.ID})
	if err != nil || got.ColumnID != e.col(dst, "in-progress") {
		t.Fatalf("falls back to a column of the same category: %+v %v", got, err)
	}
	dropped := e.mk(owner, src, "dropped", func(in *CreateInput) { in.ColumnID = &drop })
	got, err = e.svc.MoveProject(bg, owner, dropped.ID, MoveProjectInput{ProjectID: dst.ID})
	if err != nil || got.ColumnID != e.col(dst, "todo") {
		t.Fatalf("falls back to the default column: %+v %v", got, err)
	}
}

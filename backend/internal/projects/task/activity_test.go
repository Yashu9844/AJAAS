package task

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
)

func TestActivityReads(t *testing.T) {
	e := newEnv(t)
	owner := e.user()
	p := e.project(owner, "ACT")
	v := e.mk(owner, p, "t")
	for i := 0; i < 4; i++ {
		got, _ := e.svc.Get(bg, owner, v.ID)
		prio := []string{"low", "medium", "high", "urgent"}[i]
		if _, err := e.svc.Update(bg, owner, v.ID, UpdateInput{Version: &got.Version, Priority: &prio}); err != nil {
			t.Fatal(err)
		}
	}
	other := e.mk(owner, p, "other")
	rows, next, err := e.svc.TaskActivity(bg, owner, v.ID, ActivityFilter{Limit: 3})
	if err != nil || len(rows) != 3 || next == "" || rows[0].Type != "updated" || rows[0].Seq <= rows[1].Seq {
		t.Fatalf("newest first: %+v %v", rows, err)
	}
	var data map[string]any
	if err := json.Unmarshal(rows[0].Data, &data); err != nil || data["changes"] == nil {
		t.Fatalf("data: %s %v", rows[0].Data, err)
	}
	rest, next2, err := e.svc.TaskActivity(bg, owner, v.ID, ActivityFilter{Limit: 3, Cursor: next})
	if err != nil || len(rest) != 2 || next2 != "" || rest[1].Type != "created" {
		t.Fatalf("second page: %+v %v", rest, err)
	}
	asc, _, _ := e.svc.TaskActivity(bg, owner, v.ID, ActivityFilter{Asc: true, Limit: 100})
	if len(asc) != 5 || asc[0].Type != "created" || asc[0].ActorID == nil || *asc[0].ActorID != owner.UserID {
		t.Fatalf("ascending: %+v", asc)
	}
	all, _, err := e.svc.ProjectActivity(bg, owner, p.ID, ActivityFilter{Limit: 100})
	if err != nil || len(all) != 6 {
		t.Fatalf("project activity covers every task: %d %v", len(all), err)
	}
	if _, _, err = e.svc.TaskActivity(bg, owner, v.ID, ActivityFilter{Cursor: "%%"}); code(err) != "VALIDATION_ERROR" {
		t.Fatalf("bad cursor: %v", err)
	}
	if _, _, err = e.svc.TaskActivity(bg, e.user(), other.ID, ActivityFilter{}); code(err) != "NOT_FOUND" {
		t.Fatalf("stranger: %v", err)
	}
	if _, _, err = e.svc.ProjectActivity(bg, e.user(), p.ID, ActivityFilter{}); code(err) != "NOT_FOUND" {
		t.Fatalf("stranger project: %v", err)
	}
	if err := e.db.Exec(`UPDATE activity SET type = 'x' WHERE task_id = ?`, v.ID).Error; err == nil {
		t.Fatal("activity is append-only")
	}
}

func TestTicketLookup(t *testing.T) {
	e := newEnv(t)
	owner := e.user()
	p := e.project(owner, "TKT")
	v := e.mk(owner, p, "t")
	for _, key := range []string{"TKT-1", "tkt-1", " TKT-1 "} {
		got, err := e.svc.ByKey(bg, owner, key)
		if err != nil || got.ID != v.ID {
			t.Fatalf("%q: %v", key, err)
		}
	}
	for key, want := range map[string]string{"TKT-9": "NOT_FOUND", "NOPE-1": "NOT_FOUND", "bad": "VALIDATION_ERROR", "TKT-": "VALIDATION_ERROR",
		"1-1": "VALIDATION_ERROR", "TKT-99999999999": "VALIDATION_ERROR"} {
		if _, err := e.svc.ByKey(bg, owner, key); code(err) != want {
			t.Errorf("%q: want %s, got %v", key, want, err)
		}
	}
	if _, err := e.svc.ByKey(bg, e.user(), "TKT-1"); code(err) != "NOT_FOUND" {
		t.Fatalf("a private project's tickets do not resolve for strangers: %v", err)
	}
	foreign := e.user()
	foreign.TenantID = uuid.New()
	if _, err := e.svc.ByKey(bg, foreign, "TKT-1"); code(err) != "NOT_FOUND" {
		t.Fatalf("ticket ids resolve inside the tenant only: %v", err)
	}
	if err := e.svc.Delete(bg, owner, v.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.ByKey(bg, owner, "TKT-1"); code(err) != "NOT_FOUND" {
		t.Fatalf("deleted: %v", err)
	}
}

// TestTenantAndProjectIsolation is PG4 for tasks: every operation answers 404 for unseen tasks.
func TestTenantAndProjectIsolation(t *testing.T) {
	e := newEnv(t)
	owner := e.user()
	p := e.project(owner, "ISO")
	v := e.mk(owner, p, "secret")
	done := e.col(p, "done")
	stranger := e.user()
	foreign := e.user()
	foreign.TenantID = uuid.New()
	for name, actor := range map[string]api.Actor{"stranger": stranger, "foreign tenant": foreign} {
		checks := map[string]error{}
		_, checks["get"] = e.svc.Get(bg, actor, v.ID)
		_, checks["update"] = e.svc.Update(bg, actor, v.ID, UpdateInput{Version: num(1), Title: str("x")})
		_, checks["move"] = e.svc.Move(bg, actor, v.ID, MoveInput{ColumnID: &done})
		_, checks["archive"] = e.svc.Archive(bg, actor, v.ID)
		_, checks["assign"] = e.svc.AddAssignees(bg, actor, v.ID, UserIDsInput{UserIDs: []uuid.UUID{actor.UserID}})
		_, checks["labels"] = e.svc.AddLabels(bg, actor, v.ID, LabelIDsInput{})
		_, checks["relations"] = e.svc.Relations(bg, actor, v.ID)
		_, checks["movep"] = e.svc.MoveProject(bg, actor, v.ID, MoveProjectInput{ProjectID: p.ID})
		checks["delete"] = e.svc.Delete(bg, actor, v.ID)
		_, _, checks["list"] = e.svc.List(bg, actor, p.ID, ListFilter{})
		_, checks["bulk"] = e.svc.Bulk(bg, actor, p.ID, BulkInput{TaskIDs: []uuid.UUID{v.ID}, Priority: str("low")})
		for op, err := range checks {
			if code(err) != "NOT_FOUND" {
				t.Errorf("%s %s: want NOT_FOUND, got %v", name, op, err)
			}
		}
	}
	if got, _ := e.svc.Get(bg, owner, v.ID); got.Version != 1 || got.Title != "secret" {
		t.Fatalf("nothing changed: %+v", got)
	}
}

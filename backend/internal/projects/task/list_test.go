package task

import (
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func titles(rows []View) string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.Title
	}
	return strings.Join(out, ",")
}

func sorted(s string) string {
	parts := strings.Split(s, ",")
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

func TestListFilters(t *testing.T) {
	e := newEnv(t)
	owner := e.user()
	p := e.project(owner, "FLT")
	member := e.member(owner, p, "member")
	bug, ms := e.label(owner, p, "bug"), e.milestone(owner, p, "M1")
	done, prog := e.col(p, "done"), e.col(p, "in-progress")
	soon, past := time.Now().Add(72*time.Hour), time.Now().Add(-72*time.Hour)
	from, to := time.Now().Add(24*time.Hour), time.Now().Add(-24*time.Hour)
	a := e.mk(owner, p, "alpha urgent", func(in *CreateInput) {
		in.Priority, in.AssigneeIDs, in.LabelIDs, in.DueAt = "urgent", []uuid.UUID{member.UserID}, []uuid.UUID{bug.ID}, &past
	})
	b := e.mk(owner, p, "beta", func(in *CreateInput) { in.ColumnID, in.MilestoneID, in.DueAt = &prog, &ms.ID, &soon })
	c := e.mk(owner, p, "gamma 50%_off", func(in *CreateInput) { in.ColumnID, in.DueAt = &done, &past })
	if _, err := e.svc.AddRelation(bg, owner, b.ID, RelationInput{TargetTaskID: a.ID, Type: "blocks"}); err != nil {
		t.Fatal(err)
	}
	child := e.mk(owner, p, "child", func(in *CreateInput) { in.ParentTaskID = &c.ID })
	if _, err := e.svc.Archive(bg, owner, child.ID); err != nil {
		t.Fatal(err)
	}
	yes, no := true, false
	cases := map[string]struct {
		f    ListFilter
		want string
	}{
		"all live":        {ListFilter{}, "alpha urgent,beta,gamma 50%_off"},
		"column":          {ListFilter{ColumnID: &prog}, "beta"},
		"category":        {ListFilter{Category: "done"}, "gamma 50%_off"},
		"priority":        {ListFilter{Priority: "urgent"}, "alpha urgent"},
		"assignee":        {ListFilter{AssigneeID: &member.UserID}, "alpha urgent"},
		"label":           {ListFilter{LabelID: &bug.ID}, "alpha urgent"},
		"milestone":       {ListFilter{MilestoneID: &ms.ID}, "beta"},
		"due from":        {ListFilter{DueFrom: &from}, "beta"},
		"due to":          {ListFilter{DueTo: &to}, "alpha urgent,gamma 50%_off"},
		"overdue":         {ListFilter{Overdue: &yes}, "alpha urgent"},
		"not overdue":     {ListFilter{Overdue: &no}, "beta,gamma 50%_off"},
		"blocked":         {ListFilter{Blocked: &yes}, "alpha urgent"},
		"not blocked":     {ListFilter{Blocked: &no}, "beta,gamma 50%_off"},
		"text":            {ListFilter{Query: "alpha"}, "alpha urgent"},
		"text is escaped": {ListFilter{Query: "50%_"}, "gamma 50%_off"},
		"title substring": {ListFilter{Query: "amm"}, "gamma 50%_off"},
		"archived":        {ListFilter{Archived: true}, "child"},
		"parent":          {ListFilter{ParentID: &c.ID, Archived: true}, "child"},
		"top level":       {ListFilter{TopLevel: true, Archived: true}, ""},
	}
	for name, tc := range cases {
		rows, _, err := e.svc.List(bg, owner, p.ID, tc.f)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got := titles(rows); sorted(got) != sorted(tc.want) {
			t.Errorf("%s: got %q want %q", name, got, tc.want)
		}
	}
	rows, _, _ := e.svc.List(bg, owner, p.ID, ListFilter{Sort: "number"})
	if titles(rows) != "gamma 50%_off,beta,alpha urgent" {
		t.Errorf("number desc: %s", titles(rows))
	}
	if rows[0].SubtaskTotal != 1 || rows[0].Progress != 0 || !rows[2].Blocked || !rows[2].Overdue || len(rows[2].AssigneeIDs) != 1 || len(rows[2].LabelIDs) != 1 {
		t.Errorf("derived fields: %+v", rows)
	}
}

func TestListValidationAndAccess(t *testing.T) {
	e := newEnv(t)
	owner := e.user()
	p := e.project(owner, "LVA")
	e.mk(owner, p, "x")
	for name, f := range map[string]ListFilter{"sort": {Sort: "bogus"}, "dir": {Dir: "sideways"}, "category": {Category: "x"},
		"priority": {Priority: "x"}, "cursor": {Cursor: "!!"}} {
		if _, _, err := e.svc.List(bg, owner, p.ID, f); code(err) != "VALIDATION_ERROR" {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, _, err := e.svc.List(bg, e.user(), p.ID, ListFilter{}); code(err) != "NOT_FOUND" {
		t.Fatalf("stranger: %v", err)
	}
	tenantVisible, err := e.proj.Create(bg, owner, projectIn("TEN", "tenant"))
	if err != nil {
		t.Fatal(err)
	}
	e.mk(owner, tenantVisible, "open")
	reader := e.user()
	if rows, _, err := e.svc.List(bg, reader, tenantVisible.ID, ListFilter{}); err != nil || len(rows) != 1 {
		t.Fatalf("tenant-visible projects are readable: %v", err)
	}
	if _, err := e.svc.Create(bg, reader, tenantVisible.ID, CreateInput{Title: "no"}); code(err) != "FORBIDDEN" {
		t.Fatalf("but not writable by non-members: %v", err)
	}
}

// TestKeysetPaginationAllSorts is PG7 in miniature: every sort and direction pages without gaps or repeats.
func TestKeysetPaginationAllSorts(t *testing.T) {
	e := newEnv(t)
	owner := e.user()
	p := e.project(owner, "PAG")
	base := time.Now().Add(24 * time.Hour)
	prios := []string{"none", "low", "medium", "high", "urgent"}
	for i := 0; i < 23; i++ {
		i := i
		e.mk(owner, p, fmt.Sprintf("t%02d", i), func(in *CreateInput) {
			in.Priority = prios[i%5]
			if i%3 != 0 {
				d := base.Add(time.Duration(i%7) * time.Hour)
				in.DueAt = &d
			}
		})
	}
	for _, key := range []string{"rank", "created", "updated", "due", "priority", "number"} {
		for _, dir := range []string{"asc", "desc"} {
			full, _, err := e.svc.List(bg, owner, p.ID, ListFilter{Sort: key, Dir: dir, Limit: 100})
			if err != nil || len(full) != 23 {
				t.Fatalf("%s %s full: %d %v", key, dir, len(full), err)
			}
			var paged []View
			cursor := ""
			for pages := 0; pages < 10; pages++ {
				rows, next, err := e.svc.List(bg, owner, p.ID, ListFilter{Sort: key, Dir: dir, Limit: 5, Cursor: cursor})
				if err != nil {
					t.Fatalf("%s %s page: %v", key, dir, err)
				}
				paged = append(paged, rows...)
				if cursor = next; next == "" {
					break
				}
			}
			if titles(paged) != titles(full) {
				t.Fatalf("%s %s: paged order differs\n%s\n%s", key, dir, titles(paged), titles(full))
			}
		}
	}
	_, next, _ := e.svc.List(bg, owner, p.ID, ListFilter{Sort: "due", Limit: 2})
	if _, _, err := e.svc.List(bg, owner, p.ID, ListFilter{Sort: "number", Cursor: next}); code(err) != "VALIDATION_ERROR" {
		t.Fatalf("a cursor from another sort must be refused: %v", err)
	}
	first, _, _ := e.svc.List(bg, owner, p.ID, ListFilter{Sort: "priority", Limit: 1})
	if first[0].Priority != "urgent" {
		t.Fatalf("priority sorts urgent first by default: %s", first[0].Priority)
	}
	nulls, _, _ := e.svc.List(bg, owner, p.ID, ListFilter{Sort: "due", Limit: 100})
	if nulls[len(nulls)-1].DueAt != nil || nulls[0].DueAt == nil {
		t.Fatal("tasks without a due date sort last")
	}
}

func TestSubtasksListAndRollup(t *testing.T) {
	e := newEnv(t)
	owner := e.user()
	p := e.project(owner, "SUB")
	parent := e.mk(owner, p, "parent")
	for i := 0; i < 3; i++ {
		e.mk(owner, p, fmt.Sprintf("kid%d", i), func(in *CreateInput) { in.ParentTaskID = &parent.ID })
	}
	rows, next, err := e.svc.Subtasks(bg, owner, parent.ID, ListFilter{Limit: 2})
	if err != nil || len(rows) != 2 || next == "" || rows[0].Title != "kid0" {
		t.Fatalf("subtasks: %v %v", rows, err)
	}
	done := e.col(p, "done")
	if _, err = e.svc.Move(bg, owner, rows[0].ID, MoveInput{ColumnID: &done}); err != nil {
		t.Fatal(err)
	}
	got, _ := e.svc.Get(bg, owner, parent.ID)
	if got.SubtaskTotal != 3 || got.SubtaskDone != 1 || got.Progress != 33 {
		t.Fatalf("roll-up: %+v", got)
	}
	if _, _, err = e.svc.Subtasks(bg, e.user(), parent.ID, ListFilter{}); code(err) != "NOT_FOUND" {
		t.Fatalf("stranger: %v", err)
	}
}

package collab

import (
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/project"
)

func mention(id uuid.UUID) string { return fmt.Sprintf("@[Someone](user:%s)", id) }

// commentFixture is a private project with an owner, a member, a viewer and one task.
type commentFixture struct {
	*env
	owner, member, viewer, stranger api.Actor
	p                               *project.Project
	pid, task                       uuid.UUID
}

func newCommentFixture(t *testing.T) *commentFixture {
	e := newEnv(t)
	f := &commentFixture{env: e, owner: e.actor(), member: e.actor(), viewer: e.actor(), stranger: e.actor()}
	p := e.project(f.owner, "CMT", "private")
	e.join(f.owner, p, f.member, "member")
	e.join(f.owner, p, f.viewer, "viewer")
	f.p, f.pid, f.task = p, p.ID, e.task(p, nil)
	return f
}

func TestCommentLifecycleAndTombstone(t *testing.T) {
	f := newCommentFixture(t)
	c, err := f.svc.CreateComment(bg, f.member, f.task, CommentInput{Body: "  first **note**  "})
	must(t, err)
	if c.Body != "first **note**" || c.AuthorUserID != f.member.UserID || c.EditedAt != nil || c.Deleted {
		t.Fatalf("created: %+v", c)
	}
	// CM-01: only the author edits; edited_at is set.
	_, err = f.svc.UpdateComment(bg, f.owner, c.ID, CommentInput{Body: "hijack"})
	wantStatus(t, err, "403")
	up, err := f.svc.UpdateComment(bg, f.member, c.ID, CommentInput{Body: "second"})
	must(t, err)
	if up.Body != "second" || up.EditedAt == nil {
		t.Fatalf("updated: %+v", up)
	}
	// Viewers cannot comment (task.comment), strangers see nothing.
	_, err = f.svc.CreateComment(bg, f.viewer, f.task, CommentInput{Body: "x"})
	wantStatus(t, err, "403")
	_, err = f.svc.CreateComment(bg, f.stranger, f.task, CommentInput{Body: "x"})
	wantStatus(t, err, "404")
	// A member cannot delete someone else's comment, the owner (comment.moderate) can.
	other := f.actor()
	f.join(f.owner, f.p, other, "member")
	_, err = f.svc.CreateComment(bg, other, f.task, CommentInput{Body: "other's"})
	must(t, err)
	var others Comment
	f.db.Where("author_user_id = ?", other.UserID).Take(&others)
	wantStatus(t, f.svc.DeleteComment(bg, f.member, others.ID), "403")
	must(t, f.svc.DeleteComment(bg, f.owner, others.ID))
	// Own delete leaves a tombstone with an empty body; the row stays.
	must(t, f.svc.DeleteComment(bg, f.member, c.ID))
	rows, _, err := f.svc.ListComments(bg, f.owner, f.task, PageRequest{Limit: 50})
	must(t, err)
	if len(rows) != 2 || !rows[0].Deleted || rows[0].Body != "" || rows[0].DeletedAt == nil {
		t.Fatalf("tombstones: %+v", rows)
	}
	wantStatus(t, f.svc.DeleteComment(bg, f.member, c.ID), "404")
	_, err = f.svc.UpdateComment(bg, f.member, c.ID, CommentInput{Body: "again"})
	wantStatus(t, err, "404")
	if f.count(`SELECT count(*) FROM comments WHERE task_id = ? AND body <> ''`, f.task) != 0 {
		t.Fatal("deleted bodies must be cleared in the database")
	}
}

func TestCommentValidationAndUnknownIDs(t *testing.T) {
	f := newCommentFixture(t)
	for _, body := range []string{"", "   ", strings.Repeat("a", 10001), "bad\x00nul", "\xff\xfe"} {
		_, err := f.svc.CreateComment(bg, f.member, f.task, CommentInput{Body: body})
		wantStatus(t, err, "400")
	}
	if _, err := f.svc.CreateComment(bg, f.member, f.task, CommentInput{Body: strings.Repeat("é", 10000)}); err != nil {
		t.Fatalf("10000 characters are allowed: %v", err)
	}
	_, err := f.svc.CreateComment(bg, f.member, uuid.New(), CommentInput{Body: "x"})
	wantStatus(t, err, "404")
	_, err = f.svc.UpdateComment(bg, f.member, uuid.New(), CommentInput{Body: "x"})
	wantStatus(t, err, "404")
	wantStatus(t, f.svc.DeleteComment(bg, f.member, uuid.New()), "404")
	_, _, err = f.svc.ListComments(bg, f.member, f.task, PageRequest{Cursor: "!!"})
	wantStatus(t, err, "400")
	c, _ := f.svc.CreateComment(bg, f.member, f.task, CommentInput{Body: "ok"})
	_, err = f.svc.UpdateComment(bg, f.member, c.ID, CommentInput{Body: ""})
	wantStatus(t, err, "400")
	// A foreign tenant never reaches the comment (PG4).
	foreign := f.actor()
	foreign.TenantID = uuid.New()
	_, err = f.svc.UpdateComment(bg, foreign, c.ID, CommentInput{Body: "x"})
	wantStatus(t, err, "404")
}

func TestCommentPagination(t *testing.T) {
	f := newCommentFixture(t)
	for i := 0; i < 5; i++ {
		_, err := f.svc.CreateComment(bg, f.member, f.task, CommentInput{Body: fmt.Sprintf("c%d", i)})
		must(t, err)
	}
	seen := []string{}
	cursor := ""
	for pages := 0; pages < 10; pages++ {
		rows, next, err := f.svc.ListComments(bg, f.viewer, f.task, PageRequest{Cursor: cursor, Limit: 2})
		must(t, err)
		for _, r := range rows {
			seen = append(seen, r.Body)
		}
		if next == "" {
			break
		}
		cursor = next
	}
	if strings.Join(seen, ",") != "c0,c1,c2,c3,c4" {
		t.Fatalf("paging order: %v", seen)
	}
}

func TestCommentArchivedProjectIsFrozen(t *testing.T) {
	f := newCommentFixture(t)
	c, _ := f.svc.CreateComment(bg, f.member, f.task, CommentInput{Body: "before"})
	_, err := f.proj.Archive(bg, f.owner, f.pid)
	must(t, err)
	_, err = f.svc.CreateComment(bg, f.member, f.task, CommentInput{Body: "after"})
	wantCode(t, err, "PROJECT_ARCHIVED")
	_, err = f.svc.UpdateComment(bg, f.member, c.ID, CommentInput{Body: "after"})
	wantCode(t, err, "PROJECT_ARCHIVED")
	wantCode(t, f.svc.DeleteComment(bg, f.member, c.ID), "PROJECT_ARCHIVED")
	if rows, _, err := f.svc.ListComments(bg, f.member, f.task, PageRequest{}); err != nil || len(rows) != 1 {
		t.Fatalf("archived projects stay readable: %v %v", rows, err)
	}
}

// TestMentionsNeverLeakATask is PG11: a mention of a user who cannot read the project is dropped, stored nowhere and not
// announced; a member and (in a tenant-visible project) any active tenant user is kept.
func TestMentionsNeverLeakATask(t *testing.T) {
	f := newCommentFixture(t)
	outsider, ghost := f.actor(), uuid.New()
	body := fmt.Sprintf("hi %s %s %s %s %s", mention(f.viewer.UserID), mention(outsider.UserID), mention(ghost), mention(f.member.UserID), mention(f.viewer.UserID))
	c, err := f.svc.CreateComment(bg, f.member, f.task, CommentInput{Body: body})
	must(t, err)
	if len(c.Mentions) != 1 || c.Mentions[0] != f.viewer.UserID {
		t.Fatalf("only the member who can read stays (the author is dropped): %v", c.Mentions)
	}
	if f.count(`SELECT count(*) FROM mentions WHERE user_id IN (?, ?)`, outsider.UserID, ghost) != 0 {
		t.Fatal("unreadable mentions must not be stored")
	}
	ev := f.events("task.mentioned")
	if len(ev) != 1 {
		t.Fatalf("one mention event: %v", ev)
	}
	ids, _ := ev[0]["user_ids"].([]any)
	if len(ids) != 1 || ids[0] != f.viewer.UserID.String() || ev[0]["task_id"] != f.task.String() {
		t.Fatalf("event payload: %v", ev[0])
	}
	// Mentioned users watch; the commenter does too.
	if f.count(`SELECT count(*) FROM task_watchers WHERE task_id = ? AND reason = 'mentioned' AND user_id = ?`, f.task, f.viewer.UserID) != 1 ||
		f.count(`SELECT count(*) FROM task_watchers WHERE task_id = ? AND reason = 'commenter' AND user_id = ?`, f.task, f.member.UserID) != 1 {
		t.Fatal("auto watchers missing")
	}
	// Listing shows the stored mentions only.
	rows, _, _ := f.svc.ListComments(bg, f.owner, f.task, PageRequest{})
	if len(rows[0].Mentions) != 1 {
		t.Fatalf("listed mentions: %v", rows[0].Mentions)
	}
}

func TestMentionsInTenantVisibleProjectAndEdits(t *testing.T) {
	e := newEnv(t)
	owner, author, reader := e.actor(), e.actor(), e.actor()
	p := e.project(owner, "VIS", "tenant")
	e.join(owner, p, author, "member")
	task := e.task(p, nil)
	other := e.actor()
	c, err := e.svc.CreateComment(bg, author, task, CommentInput{Body: mention(reader.UserID)})
	must(t, err)
	if len(c.Mentions) != 1 {
		t.Fatalf("any active tenant user can read a tenant-visible project: %v", c.Mentions)
	}
	// A user of another tenant and a deactivated user are dropped.
	foreign := uuid.New()
	e.users.active[foreign] = false
	c2, err := e.svc.CreateComment(bg, author, task, CommentInput{Body: mention(foreign)})
	must(t, err)
	if len(c2.Mentions) != 0 {
		t.Fatalf("inactive user kept: %v", c2.Mentions)
	}
	// Editing replaces the mention set and announces only the newly added users.
	up, err := e.svc.UpdateComment(bg, author, c.ID, CommentInput{Body: mention(reader.UserID) + " " + mention(other.UserID)})
	must(t, err)
	if len(up.Mentions) != 2 {
		t.Fatalf("edited mentions: %v", up.Mentions)
	}
	ev := e.events("task.mentioned")
	last := ev[len(ev)-1]["user_ids"].([]any)
	if len(ev) != 2 || len(last) != 1 || last[0] != other.UserID.String() {
		t.Fatalf("only the new mention is announced: %v", ev)
	}
	up, err = e.svc.UpdateComment(bg, author, c.ID, CommentInput{Body: "no mentions now"})
	must(t, err)
	if e.count(`SELECT count(*) FROM mentions WHERE source_id = ?`, c.ID) != 0 || len(e.events("task.mentioned")) != 2 {
		t.Fatal("removing mentions deletes them and announces nothing")
	}
	// Deleting a comment removes its mentions.
	_, _ = e.svc.UpdateComment(bg, author, c.ID, CommentInput{Body: mention(reader.UserID)})
	must(t, e.svc.DeleteComment(bg, author, c.ID))
	if e.count(`SELECT count(*) FROM mentions WHERE source_id = ?`, c.ID) != 0 {
		t.Fatal("tombstones hold no mentions")
	}
}

func TestCommentEventsAndAuditHoldIdsOnly(t *testing.T) {
	f := newCommentFixture(t)
	secret := "TOP-SECRET-COMMENT-TEXT"
	c, err := f.svc.CreateComment(bg, f.member, f.task, CommentInput{Body: secret + " " + mention(f.viewer.UserID)})
	must(t, err)
	_, err = f.svc.UpdateComment(bg, f.member, c.ID, CommentInput{Body: secret + " edited"})
	must(t, err)
	must(t, f.svc.DeleteComment(bg, f.owner, c.ID))
	f.noLeak(secret, "Someone")
	ev := f.events("task.commented")
	if len(ev) != 1 || ev[0]["comment_id"] != c.ID.String() || ev[0]["task_version"] != float64(1) {
		t.Fatalf("task.commented payload: %v", ev)
	}
	if f.count(`SELECT count(*) FROM activity WHERE task_id = ? AND type = 'commented'`, f.task) != 1 {
		t.Fatal("one commented activity entry")
	}
	if len(f.audit.actions) != 3 {
		t.Fatalf("audit actions: %v", f.audit.actions)
	}
}

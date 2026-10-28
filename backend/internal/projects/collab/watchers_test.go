package collab

import (
	"fmt"
	"testing"

	"github.com/google/uuid"
)

func TestParseMentions(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	text := fmt.Sprintf("x @[A](user:%s) y @[B b](user:%s) again @[A](user:%s) @[Bad](user:not-a-uuid-not-a-uuid-not-a-uuid-xx) @[](user:%s) plain @nobody", a, b, a, a)
	got := ParseMentions(text)
	if len(got) != 2 || got[0] != a || got[1] != b {
		t.Fatalf("parsed: %v", got)
	}
	if len(ParseMentions("")) != 0 || len(ParseMentions("@[x](user:)")) != 0 {
		t.Fatal("no tokens, no mentions")
	}
	var many string
	for i := 0; i < maxMentions+10; i++ {
		many += mention(uuid.New()) + " "
	}
	if n := len(ParseMentions(many)); n != maxMentions {
		t.Fatalf("cap: %d", n)
	}
}

func TestWatchUnwatchAndList(t *testing.T) {
	f := newCommentFixture(t)
	st, err := f.svc.WatchStatus(bg, f.member, f.task)
	must(t, err)
	if st.Watching {
		t.Fatal("not watching yet")
	}
	st, err = f.svc.Watch(bg, f.member, f.task)
	must(t, err)
	if !st.Watching || st.Reason != ReasonExplicit {
		t.Fatalf("watch: %+v", st)
	}
	_, err = f.svc.Watch(bg, f.member, f.task) // idempotent
	must(t, err)
	// A commenter who already watches explicitly keeps one row; a viewer can watch too.
	_, err = f.svc.CreateComment(bg, f.member, f.task, CommentInput{Body: "hello"})
	must(t, err)
	_, err = f.svc.Watch(bg, f.viewer, f.task)
	must(t, err)
	if f.count(`SELECT count(*) FROM task_watchers WHERE task_id = ?`, f.task) != 2 {
		t.Fatal("one row per watcher")
	}
	if st, _ = f.svc.WatchStatus(bg, f.member, f.task); !st.Watching || st.Reason != ReasonExplicit {
		t.Fatalf("status: %+v", st)
	}
	// Strangers and foreign ids get 404 on every verb.
	_, err = f.svc.Watch(bg, f.stranger, f.task)
	wantStatus(t, err, "404")
	wantStatus(t, f.svc.Unwatch(bg, f.stranger, f.task), "404")
	_, err = f.svc.WatchStatus(bg, f.stranger, f.task)
	wantStatus(t, err, "404")
	_, _, err = f.svc.Watchers(bg, f.stranger, f.task, PageRequest{})
	wantStatus(t, err, "404")
	// Listing is cursor-paged by user id.
	first, next, err := f.svc.Watchers(bg, f.owner, f.task, PageRequest{Limit: 1})
	must(t, err)
	if len(first) != 1 || next == "" {
		t.Fatalf("page one: %v %q", first, next)
	}
	second, next, err := f.svc.Watchers(bg, f.owner, f.task, PageRequest{Limit: 1, Cursor: next})
	must(t, err)
	if len(second) != 1 || next != "" || second[0].UserID == first[0].UserID {
		t.Fatalf("page two: %v %q", second, next)
	}
	if _, _, err = f.svc.Watchers(bg, f.owner, f.task, PageRequest{Cursor: "%%"}); err == nil {
		t.Fatal("bad cursor")
	}
	// Unwatching is idempotent.
	must(t, f.svc.Unwatch(bg, f.member, f.task))
	must(t, f.svc.Unwatch(bg, f.member, f.task))
	if st, _ = f.svc.WatchStatus(bg, f.member, f.task); st.Watching {
		t.Fatal("unwatched")
	}
}

func TestAddWatcherKeepsFirstReason(t *testing.T) {
	f := newCommentFixture(t)
	w := Watcher{TaskID: f.task, UserID: f.member.UserID, TenantID: f.tenant, Reason: ReasonAssignee}
	must(t, AddWatcher(bg, f.db, w))
	w.Reason = ReasonMentioned
	must(t, AddWatcher(bg, f.db, w))
	if f.count(`SELECT count(*) FROM task_watchers WHERE task_id = ? AND reason = 'assignee'`, f.task) != 1 {
		t.Fatal("an existing watch keeps its reason")
	}
}

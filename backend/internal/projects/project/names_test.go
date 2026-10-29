package project

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

type fakeNames struct {
	names map[uuid.UUID]string
	err   error
}

func (f fakeNames) DisplayNames(_ context.Context, _ uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]string, error) {
	out := map[uuid.UUID]string{}
	for _, id := range ids {
		if n, ok := f.names[id]; ok {
			out[id] = n
		}
	}
	return out, f.err
}

// Members carry display names (never e-mail addresses) when a names port is wired; a failing lookup degrades to ids only.
func TestMembersCarryDisplayNames(t *testing.T) {
	e := newEnv(t)
	owner, member := e.user("create"), e.user()
	p := e.project(owner, "NAM")
	_, _ = e.svc.AddMember(bg, owner, p.ID, MemberInput{UserID: member.UserID, Role: "member"})

	names := fakeNames{names: map[uuid.UUID]string{owner.UserID: "Olivia Owner"}}
	withNames := NewService(e.db, Ports{Users: e.users, Audit: e.audit, Names: names})
	rows, err := withNames.Members(bg, owner, p.ID)
	if err != nil || len(rows) != 2 {
		t.Fatalf("members: %v %d", err, len(rows))
	}
	got := map[uuid.UUID]string{}
	for _, r := range rows {
		got[r.UserID] = r.DisplayName
	}
	if got[owner.UserID] != "Olivia Owner" || got[member.UserID] != "" {
		t.Fatalf("known names are filled, unknown stay empty: %v", got)
	}
	failing := NewService(e.db, Ports{Users: e.users, Audit: e.audit, Names: fakeNames{err: errors.New("down")}})
	if rows, err = failing.Members(bg, owner, p.ID); err != nil || len(rows) != 2 {
		t.Fatalf("a failing names lookup must not fail the list: %v", err)
	}
	if rows, err = e.svc.Members(bg, owner, p.ID); err != nil || rows[0].DisplayName != "" {
		t.Fatal("without a names port members carry no name")
	}
}

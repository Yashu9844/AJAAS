package series

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/meetings/api"
	"github.com/jaas/jaas/internal/meetings/testdb"
)

// fixture is a recurring series with a note-taker, used to run every operation against an injected fault.
type fixture struct {
	e        *env
	v        *View
	occ, sec uuid.UUID
	guest    uuid.UUID
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	e := newEnv(t)
	guest := uuid.New()
	v := e.create(t, CreateInput{Title: "Ops", Start: "2027-01-12T09:00", TZ: "Europe/Berlin", DurationMin: 30, RRule: "FREQ=WEEKLY",
		Participants: []ParticipantInput{{UserID: guest, Role: api.RoleNoteTaker}}})
	var occs []uuid.UUID
	e.db.Raw("SELECT id FROM meeting_occurrences WHERE series_id = ? ORDER BY start_at LIMIT 2", v.ID).Scan(&occs)
	return &fixture{e: e, v: v, occ: occs[0], sec: occs[1], guest: guest}
}

// ops runs every series operation once and returns how many failed.
func (f *fixture) ops() int {
	ctx, s, a := context.Background(), f.e.svc, f.e.a
	title, start := "New", "2027-01-12T10:00"
	from := f.e.now
	errs := 0
	try := func(err error) {
		if err != nil {
			errs++
		}
	}
	_, err := s.Create(ctx, a, CreateInput{Title: "X", Start: "2027-02-01T09:00", TZ: "UTC", DurationMin: 30, Participants: []ParticipantInput{{UserID: f.guest}}})
	try(err)
	_, err = s.Get(ctx, a, f.v.ID)
	try(err)
	_, err = s.Participants(ctx, a, f.v.ID)
	try(err)
	_, err = s.AssignableRoles(ctx, a, f.v.ID)
	try(err)
	_, err = s.GetOccurrence(ctx, a, f.occ)
	try(err)
	_, err = s.List(ctx, a, ListFilter{From: from, To: from.AddDate(0, 0, 30)})
	try(err)
	_, err = s.Search(ctx, a, SearchFilter{Query: "ops"})
	try(err)
	try(s.Respond(ctx, a, f.v.ID, nil, ResponseInput{RSVP: RSVPAccepted}))
	try(s.RespondOccurrence(ctx, a, f.occ, ResponseInput{RSVP: RSVPDeclined}))
	_, err = s.SetConsent(ctx, a, f.v.ID, ConsentInput{State: ConsentGranted})
	try(err)
	_, err = s.Consents(ctx, a, f.v.ID)
	try(err)
	_, err = s.UpdateOccurrence(ctx, a, f.occ, OccurrenceInput{Title: &title, Version: 1})
	try(err)
	_, err = s.SetStatus(ctx, a, f.occ, StatusInput{Status: StatusInProgress, Version: 1})
	try(err)
	_, err = s.AddParticipant(ctx, a, f.v.ID, ParticipantInput{UserID: uuid.New()})
	try(err)
	_, err = s.UpdateParticipant(ctx, a, f.v.ID, ParticipantInput{UserID: f.guest, Role: api.RoleAttendee})
	try(err)
	try(s.RemoveParticipant(ctx, a, f.v.ID, f.guest))
	_, err = s.Update(ctx, a, f.v.ID, UpdateInput{Title: &title, Version: 1})
	try(err)
	_, err = s.Update(ctx, a, f.v.ID, UpdateInput{Start: &start, Version: 1})
	try(err)
	_, err = s.Split(ctx, a, f.v.ID, SplitInput{OccurrenceID: f.sec, Version: 1})
	try(err)
	_, err = s.CancelOccurrence(ctx, a, f.sec, CancelInput{Version: 1})
	try(err)
	_, err = s.Cancel(ctx, a, f.v.ID, CancelInput{Version: 1})
	try(err)
	try(s.Delete(ctx, a, f.v.ID, 1))
	try(s.DeactivateUserMappings(ctx, f.e.db, a.TenantID, f.guest, uuid.Nil))
	_, err = s.Purge(ctx)
	try(err)
	_, err = s.ExtendWindows(ctx)
	try(err)
	_, err = s.RemindStartingSoon(ctx)
	try(err)
	return errs
}

func TestFaultsSurfaceAsErrors(t *testing.T) {
	methods := []string{"PermissionsOn", "AssignRole", "RevokeRole", "RolesOn", "AvailableRoles", "RoleByTemplate", "CountWithTemplate"}
	for _, m := range methods {
		f := newFixture(t)
		f.e.res.Fail[m] = true
		if n := f.ops(); n == 0 {
			t.Errorf("a failing %s must surface as an error somewhere", m)
		}
	}
	f := newFixture(t)
	f.e.svc.p.Audit.(*testdb.Audit).Fail = true
	if n := f.ops(); n < 10 {
		t.Errorf("a failing audit fails the writes: %d errors", n)
	}
	f = newFixture(t)
	f.e.users.Fail = true
	if n := f.ops(); n == 0 {
		t.Error("a failing user directory surfaces")
	}
	f = newFixture(t)
	f.e.svc.p.Names = failingNames{}
	if _, err := f.e.svc.Participants(context.Background(), f.e.a, f.v.ID); err != nil {
		t.Fatalf("names are best effort: %v", err)
	}
	f = newFixture(t)
	if sql, err := f.e.db.DB(); err == nil {
		_ = sql.Close()
	}
	if n := f.ops(); n < 20 {
		t.Errorf("a closed database fails every operation: %d", n)
	}
}

type failingNames struct{}

func (failingNames) DisplayNames(context.Context, uuid.UUID, []uuid.UUID) (map[uuid.UUID]string, error) {
	return nil, testdb.ErrFault
}

func TestNewServiceNeedsItsPorts(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("missing ports must fail fast")
		}
	}()
	NewService(nil, Ports{})
}

func TestRunStopsWithItsContext(t *testing.T) {
	e := newEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	done := make(chan struct{})
	go func() { e.svc.Run(ctx, func(string, error) {}); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run returns when its context ends")
	}
}

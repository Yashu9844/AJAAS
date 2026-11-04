package content

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/meetings/api"
	"github.com/jaas/jaas/internal/meetings/series"
	"github.com/jaas/jaas/internal/meetings/testdb"
)

type env struct {
	svc   *Service
	audit *testdb.Audit
	a     api.Actor
	occ   uuid.UUID
	close func()
}

// newEnv creates a meeting that started ten minutes ago, organised by the actor.
func newEnv(t *testing.T) *env {
	t.Helper()
	db := testdb.Open(t)
	audit := &testdb.Audit{}
	ser := series.NewService(db, series.Ports{Users: &testdb.Users{}, Audit: audit, Resources: testdb.NewResources()})
	a := api.Actor{TenantID: uuid.New(), UserID: uuid.New(), Create: true}
	start := time.Now().UTC().Add(-10 * time.Minute).Format(series.LocalLayout)
	v, err := ser.Create(context.Background(), a, series.CreateInput{Title: "Sync", Start: start, TZ: "UTC", DurationMin: 60})
	if err != nil {
		t.Fatal(err)
	}
	svc := NewService(db, Ports{Authz: ser, Lifecycle: ser, Audit: audit, Names: names{}})
	svc.SetClock(time.Now)
	return &env{svc: svc, audit: audit, a: a, occ: v.NextOccurrence.ID, close: func() {
		if sql, e := db.DB(); e == nil {
			_ = sql.Close()
		}
	}}
}

type names struct{}

func (names) DisplayNames(_ context.Context, _ uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]string, error) {
	out := map[uuid.UUID]string{}
	for _, id := range ids {
		out[id] = "Person " + id.String()[:4]
	}
	return out, nil
}

// run exercises every content operation once and returns the number of errors.
func (e *env) run() int {
	ctx, a, s := context.Background(), e.a, e.svc
	errs := 0
	try := func(err error) {
		if err != nil {
			errs++
		}
	}
	title, body := "Topic", "Body"
	it, err := s.AddAgendaItem(ctx, a, e.occ, AgendaInput{Title: &title, Body: &body})
	try(err)
	id := uuid.New()
	if it != nil {
		id = it.ID
	}
	_, err = s.Agenda(ctx, a, e.occ)
	try(err)
	_, err = s.UpdateAgendaItem(ctx, a, id, AgendaInput{Title: &title, Version: 1})
	try(err)
	_, err = s.MoveAgendaItem(ctx, a, id, MoveInput{})
	try(err)
	_, err = s.SaveNotes(ctx, a, e.occ, GeneralSection, NotesInput{Body: "notes", Version: 0})
	try(err)
	_, err = s.Notes(ctx, a, e.occ)
	try(err)
	_, err = s.NoteVersions(ctx, a, e.occ, GeneralSection)
	try(err)
	_, err = s.MarkAttendance(ctx, a, e.occ, AttendanceInput{Entries: []AttendanceEntry{{UserID: a.UserID, State: "present"}}})
	try(err)
	_, err = s.AttendanceList(ctx, a, e.occ)
	try(err)
	text := "Decided"
	d, err := s.RecordDecision(ctx, a, e.occ, DecisionInput{Text: &text, AgendaItemID: &id})
	try(err)
	if d != nil {
		_, err = s.UpdateDecision(ctx, a, d.ID, DecisionInput{Text: &text, Version: 1})
		try(err)
	}
	_, err = s.Decisions(ctx, a, e.occ)
	try(err)
	p, err := s.AddPrivatePoint(ctx, a, e.occ, PointInput{Body: "private"})
	try(err)
	_, err = s.PrivatePoints(ctx, a, e.occ)
	try(err)
	if p != nil {
		_, err = s.PromotePrivatePoint(ctx, a, p.ID)
		try(err)
	}
	_, err = s.Publish(ctx, a, e.occ)
	try(err)
	_, err = s.Correct(ctx, a, e.occ, PublishInput{Reason: "fix"})
	try(err)
	_, err = s.MinutesVersions(ctx, a, e.occ)
	try(err)
	_, err = s.MinutesVersion(ctx, a, e.occ, 1)
	try(err)
	try(s.DeleteAgendaItem(ctx, a, id))
	return errs
}

func TestContentHappyPathRendersMinutes(t *testing.T) {
	e := newEnv(t)
	if n := e.run(); n != 1 { // deleting an item of a published occurrence is the one refusal
		t.Fatalf("errors on the happy path: %d", n)
	}
	m, err := e.svc.MinutesVersion(context.Background(), e.a, e.occ, 0)
	if err != nil || m.Version != 2 || !strings.Contains(m.BodyMD, "## Attendance") || !strings.Contains(m.BodyMD, "Person") ||
		!strings.Contains(m.BodyMD, "- Decided (approved)") {
		t.Fatalf("minutes: %+v %v", m, err)
	}
}

func TestContentFaultsSurface(t *testing.T) {
	e := newEnv(t)
	e.audit.Fail = true
	if n := e.run(); n < 3 {
		t.Fatalf("a failing audit fails publishing: %d", n)
	}
	e = newEnv(t)
	e.close()
	if n := e.run(); n < 15 {
		t.Fatalf("a closed database fails everything: %d", n)
	}
	defer func() {
		if recover() == nil {
			t.Fatal("missing ports fail fast")
		}
	}()
	NewService(nil, Ports{})
}

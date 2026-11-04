package series

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/meetings/api"
	"github.com/jaas/jaas/internal/meetings/testdb"
	"gorm.io/gorm"
)

type env struct {
	db    *gorm.DB
	svc   *Service
	res   *testdb.Resources
	users *testdb.Users
	now   time.Time
	a     api.Actor
}

func newEnv(t *testing.T) *env {
	t.Helper()
	db := testdb.Open(t)
	e := &env{db: db, res: testdb.NewResources(), users: &testdb.Users{Inactive: map[uuid.UUID]bool{}},
		now: time.Date(2027, 1, 4, 8, 0, 0, 0, time.UTC)}
	e.svc = NewService(db, Ports{Users: e.users, Audit: &testdb.Audit{}, Resources: e.res})
	e.svc.SetClock(func() time.Time { return e.now })
	e.a = api.Actor{TenantID: uuid.New(), UserID: uuid.New(), Create: true}
	return e
}

func (e *env) count(t *testing.T, q string, args ...any) int {
	t.Helper()
	var n int
	if err := e.db.Raw(q, args...).Scan(&n).Error; err != nil {
		t.Fatal(err)
	}
	return n
}

func (e *env) create(t *testing.T, in CreateInput) *View {
	t.Helper()
	v, err := e.svc.Create(context.Background(), e.a, in)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestExtendWindowsRollsOpenEndedSeriesIdempotently(t *testing.T) {
	e := newEnv(t)
	v := e.create(t, CreateInput{Title: "Standup", Start: "2027-01-05T09:00", TZ: "Europe/Berlin", DurationMin: 15, RRule: "FREQ=WEEKLY"})
	before := e.count(t, "SELECT count(*) FROM meeting_occurrences WHERE series_id = ?", v.ID)
	if before < 52 {
		t.Fatalf("12-month window: %d", before)
	}
	if n, err := e.svc.ExtendWindows(context.Background()); err != nil || n != 0 {
		t.Fatalf("nothing to extend yet: %d %v", n, err)
	}
	e.now = e.now.AddDate(0, 3, 0)
	n, err := e.svc.ExtendWindows(context.Background())
	if err != nil || n < 12 || n > 14 {
		t.Fatalf("three more months: %d %v", n, err)
	}
	if again, _ := e.svc.ExtendWindows(context.Background()); again != 0 {
		t.Fatalf("idempotent: %d", again)
	}
	bounded := e.create(t, CreateInput{Title: "Course", Start: "2027-01-05T09:00", TZ: "UTC", DurationMin: 60, RRule: "FREQ=DAILY;COUNT=10"})
	if e.count(t, "SELECT count(*) FROM meeting_series WHERE id = ? AND window_until IS NULL", bounded.ID) != 1 {
		t.Fatal("a bounded series is materialised completely and has no window")
	}
}

func TestRemindStartingSoonPublishesOnce(t *testing.T) {
	e := newEnv(t)
	v := e.create(t, CreateInput{Title: "Soon", Start: e.now.Add(5 * time.Minute).Format(LocalLayout), TZ: "UTC", DurationMin: 30})
	e.create(t, CreateInput{Title: "Later", Start: e.now.Add(2 * time.Hour).Format(LocalLayout), TZ: "UTC", DurationMin: 30})
	n, err := e.svc.RemindStartingSoon(context.Background())
	if err != nil || n != 1 {
		t.Fatalf("one reminder: %d %v", n, err)
	}
	if n, _ := e.svc.RemindStartingSoon(context.Background()); n != 0 {
		t.Fatalf("sent once: %d", n)
	}
	if e.count(t, `SELECT count(*) FROM meetings_events_outbox WHERE routing_key = 'occurrence.starting_soon' AND payload->'payload'->>'series_id' = ?`, v.ID.String()) != 1 {
		t.Fatal("the reminder event is in the outbox")
	}
}

func TestPurgeRemovesDeletedSeriesAndTheirRoles(t *testing.T) {
	e := newEnv(t)
	v := e.create(t, CreateInput{Title: "Old", Start: "2027-01-05T09:00", TZ: "UTC", DurationMin: 30, RRule: "FREQ=DAILY;COUNT=3"})
	if err := e.svc.Delete(context.Background(), e.a, v.ID, v.Version); err != nil {
		t.Fatal(err)
	}
	if n, _ := e.svc.Purge(context.Background()); n != 0 {
		t.Fatalf("kept for 30 days: %d", n)
	}
	e.now = e.now.Add(PurgeAfter + time.Hour)
	if n, err := e.svc.Purge(context.Background()); err != nil || n != 1 {
		t.Fatalf("purged: %d %v", n, err)
	}
	if e.count(t, "SELECT count(*) FROM meeting_occurrences WHERE series_id = ?", v.ID) != 0 {
		t.Fatal("occurrences go with the series")
	}
	if roles, _ := e.res.RolesOn(context.Background(), nil, ref(e.a.TenantID, v.ID)); len(roles) != 0 {
		t.Fatalf("M0 roles are revoked: %v", roles)
	}
}

func TestDeactivationEndsParticipationAndFeeds(t *testing.T) {
	e := newEnv(t)
	guest := uuid.New()
	v := e.create(t, CreateInput{Title: "Team", Start: "2027-01-05T09:00", TZ: "UTC", DurationMin: 30, RRule: "FREQ=WEEKLY",
		Participants: []ParticipantInput{{UserID: guest}}})
	if err := e.db.Exec(`INSERT INTO meeting_feed_tokens (id, tenant_id, user_id, token_hash) VALUES (?, ?, ?, ?)`, uuid.New(), e.a.TenantID, guest,
		"0000000000000000000000000000000000000000000000000000000000000000").Error; err != nil {
		t.Fatal(err)
	}
	if err := e.svc.DeactivateUserMappings(context.Background(), e.db, e.a.TenantID, guest, uuid.New()); err != nil {
		t.Fatal(err)
	}
	if e.count(t, "SELECT count(*) FROM meeting_participants WHERE user_id = ? AND active", guest) != 0 {
		t.Fatal("participation ends")
	}
	if r, _, _ := e.res.PermissionsOn(context.Background(), nil, ref(e.a.TenantID, v.ID), guest); r.RoleID != uuid.Nil {
		t.Fatal("role revoked")
	}
	if e.count(t, "SELECT count(*) FROM meeting_feed_tokens WHERE user_id = ? AND revoked_at IS NULL", guest) != 0 {
		t.Fatal("feed tokens revoked")
	}
	e.users.Inactive[guest] = true
	if _, err := e.svc.AddParticipant(context.Background(), e.a, v.ID, ParticipantInput{UserID: guest}); err == nil {
		t.Fatal("an inactive user cannot be invited again")
	}
}

func TestEffectiveStatusAndOccurrenceCap(t *testing.T) {
	now := time.Date(2027, 1, 1, 10, 0, 0, 0, time.UTC)
	o := Occurrence{Status: StatusScheduled, StartAt: now.Add(-time.Minute), EndAt: now.Add(time.Hour)}
	if o.EffectiveStatus(now) != StatusInProgress || o.EffectiveStatus(now.Add(2*time.Hour)) != StatusEnded || o.EffectiveStatus(now.Add(-time.Hour)) != StatusScheduled {
		t.Fatal("the clock drives scheduled occurrences")
	}
	o.StatusManual = true
	if o.EffectiveStatus(now.Add(2*time.Hour)) != StatusScheduled {
		t.Fatal("a manual status is kept")
	}
	e := newEnv(t)
	err := e.db.Transaction(func(tx *gorm.DB) error {
		return e.svc.checkCap(context.Background(), tx, e.a.TenantID, tenantOccurrences+1)
	})
	if err == nil {
		t.Fatal("the tenant cap refuses runaway materialisation")
	}
}

func TestSplitCountRuleAndRefusals(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	v := e.create(t, CreateInput{Title: "Course", Start: "2027-01-05T09:00", TZ: "UTC", DurationMin: 60, RRule: "FREQ=WEEKLY;COUNT=6"})
	var occs []uuid.UUID
	e.db.Raw("SELECT id FROM meeting_occurrences WHERE series_id = ? ORDER BY start_at", v.ID).Scan(&occs)
	next, err := e.svc.Split(ctx, e.a, v.ID, SplitInput{OccurrenceID: occs[2], Version: v.Version, Changes: UpdateInput{Title: strPtr("Course (part 2)")}})
	if err != nil {
		t.Fatal(err)
	}
	if *next.RRule != "FREQ=WEEKLY;COUNT=4" {
		t.Fatalf("the new series keeps the remaining count: %s", *next.RRule)
	}
	old, _ := e.svc.Get(ctx, e.a, v.ID)
	if *old.RRule != "FREQ=WEEKLY;COUNT=2" || e.count(t, "SELECT count(*) FROM meeting_occurrences WHERE series_id = ?", next.ID) != 4 {
		t.Fatalf("the old series ends before the split: %s", *old.RRule)
	}
	e.now = time.Date(2027, 3, 1, 0, 0, 0, 0, time.UTC)
	var later []uuid.UUID
	e.db.Raw("SELECT id FROM meeting_occurrences WHERE series_id = ? ORDER BY start_at", next.ID).Scan(&later)
	if _, err := e.svc.Split(ctx, e.a, next.ID, SplitInput{OccurrenceID: later[1], Version: next.Version}); err == nil {
		t.Fatal("a past occurrence cannot start a new schedule")
	}
	if _, err := e.svc.Split(ctx, e.a, next.ID, SplitInput{OccurrenceID: uuid.New(), Version: next.Version}); err == nil {
		t.Fatal("unknown occurrence")
	}
	long := strings.Repeat("x", 301)
	bad := []CreateInput{
		{Title: "x", Start: "2027-01-05T09:00", TZ: "UTC", DurationMin: 30, Location: long},
		{Title: "x", Start: "2027-01-05T09:00", TZ: "UTC", DurationMin: 30, Visibility: "public"},
		{Title: "x", Start: "2027-01-05T09:00", TZ: "UTC", DurationMin: 30, Description: strings.Repeat("d", 20001)},
		{Title: "x", Start: "2027-01-05 09:00", TZ: "UTC", DurationMin: 30},
		{Title: "x", Start: "2027-01-05T09:00", TZ: "", DurationMin: 30},
		{Title: "x", Start: "2027-01-05T09:00", TZ: "UTC", DurationMin: 30, RRule: "FREQ=DAILY;UNTIL=20400101"},
		{Title: "", Start: "2027-01-05T09:00", TZ: "UTC", DurationMin: 30},
	}
	for i, in := range bad {
		if _, err := e.svc.Create(ctx, e.a, in); err == nil {
			t.Errorf("case %d must be refused", i)
		}
	}
	if v, err := e.svc.Create(ctx, e.a, CreateInput{Title: "Offsite", Start: "2027-01-20", TZ: "UTC", AllDay: true}); err != nil || v.DurationMin != 1440 {
		t.Fatalf("all-day meeting: %v", err)
	}
	if _, err := e.svc.Create(ctx, api.Actor{TenantID: e.a.TenantID, UserID: e.a.UserID}, CreateInput{Title: "x", Start: "2027-01-05T09:00", TZ: "UTC", DurationMin: 30}); err == nil {
		t.Fatal("meetings:create is required")
	}
}

func strPtr(s string) *string { return &s }

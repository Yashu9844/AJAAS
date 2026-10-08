package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/attendance/models"
)

// TestGolden_G12_RelayDrainsOutbox — protected (G12, FR-EV002): rows left by a broker outage get published.
func TestGolden_G12_RelayDrainsOutbox(t *testing.T) {
	h := newHarness()
	user := uuid.New()
	h.emps.add(h.tenant, user, "active")
	h.pub.fail = true
	if _, err := punch(t, h, user, "in"); err != nil {
		t.Fatal(err)
	}
	relay := NewOutboxRelay(h.deps)
	n, err := relay.RelayOnce(context.Background())
	if err != nil || n != 0 {
		t.Fatalf("broker still down: published=%d err=%v", n, err)
	}
	for _, row := range h.st.outbox {
		if row.Attempts != 1 || row.LastError == nil || row.Published {
			t.Fatalf("failure must be recorded: %+v", row)
		}
	}
	h.pub.fail = false
	if n, err := relay.RelayOnce(context.Background()); err != nil || n != 1 {
		t.Fatalf("relay after recovery: published=%d err=%v", n, err)
	}
	for _, row := range h.st.outbox {
		if !row.Published || row.PublishedAt == nil {
			t.Fatalf("row must be published: %+v", row)
		}
	}
	if n, _ := relay.RelayOnce(context.Background()); n != 0 {
		t.Fatalf("published rows must not be re-sent, got %d", n)
	}
}

func TestRelay_SkipsExhaustedRowsAndPropagatesFetchErrors(t *testing.T) {
	h := newHarness()
	id := uuid.New()
	h.st.outbox[id] = models.OutboxEvent{ID: id, TenantID: h.tenant, RoutingKey: "attendance.punch.in", Payload: "{}", Attempts: relayMaxAttempts}
	relay := NewOutboxRelay(h.deps)
	if n, err := relay.RelayOnce(context.Background()); err != nil || n != 0 {
		t.Fatalf("exhausted row must be skipped: %d %v", n, err)
	}
	h.st.fail["outbox.fetch"] = errors.New("db down")
	if _, err := relay.RelayOnce(context.Background()); err == nil {
		t.Fatal("fetch error must propagate")
	}
}

func TestRelay_RunStopsOnContextCancel(t *testing.T) {
	h := newHarness()
	relay := NewOutboxRelay(h.deps)
	relay.interval = 5 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { relay.Run(ctx); close(done) }()
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run must return after context cancel")
	}
}

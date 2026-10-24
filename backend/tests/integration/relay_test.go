package integration

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/jaas/jaas/internal/hr/kernel"
)

type capturePublisher struct {
	mu   sync.Mutex
	keys []string
	fail bool
}

func (p *capturePublisher) Publish(_ context.Context, _ string, routingKey string, _ interface{}) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.fail {
		return errors.New("bus down")
	}
	p.keys = append(p.keys, routingKey)
	return nil
}

// TestOutboxRelay — the shared relay publishes the outbox rows of the single-package contexts, retries failures and never
// republishes (HR-009).
func TestOutboxRelay(t *testing.T) {
	tc := newTimeTenant(t)
	c := tc.do(t, "POST", "/hr/learning/courses", body(map[string]any{"title": "Relay " + uniq(), "content_kind": "link", "content_url": "https://learn.example.com/r"}))
	expect(t, c, 201, "course")
	expect(t, tc.do(t, "POST", "/hr/learning/courses/"+c.str("data.id")+"/publish"), 200, "publish writes an event")
	if n := sqlStr(t, "SELECT count(*) FROM lrn_events_outbox WHERE tenant_id = ? AND published = FALSE", tc.id); n != "1" {
		t.Fatalf("one pending event: %s", n)
	}

	down := &capturePublisher{fail: true}
	if n, err := kernel.NewOutboxRelay(testDB, "lrn_events_outbox", "jaas.learning.events", down).RelayOnce(context.Background()); err != nil || n != 0 {
		t.Fatalf("a failing bus publishes nothing: %d %v", n, err)
	}
	if s := sqlStr(t, "SELECT published::text||'/'||attempts||'/'||coalesce(last_error, '') FROM lrn_events_outbox WHERE tenant_id = ?", tc.id); s != "false/1/bus down" {
		t.Fatalf("the failure is recorded for retry: %s", s)
	}

	up := &capturePublisher{}
	relay := kernel.NewOutboxRelay(testDB, "lrn_events_outbox", "jaas.learning.events", up)
	if n, err := relay.RelayOnce(context.Background()); err != nil || n < 1 {
		t.Fatalf("the retry publishes: %d %v", n, err)
	}
	found := false
	for _, k := range up.keys {
		found = found || k == "learning.course.published"
	}
	if !found {
		t.Fatalf("routing key published: %v", up.keys)
	}
	if s := sqlStr(t, "SELECT published::text||'/'||attempts FROM lrn_events_outbox WHERE tenant_id = ?", tc.id); s != "true/2" {
		t.Fatalf("marked published: %s", s)
	}
	if n, err := relay.RelayOnce(context.Background()); err != nil || n != 0 {
		t.Fatalf("a published row is never republished: %d %v", n, err)
	}

	// The benefits, compensation and onboarding tables share the shape.
	for _, table := range []string{"ben_events_outbox", "comp_events_outbox", "onb_events_outbox"} {
		if _, err := kernel.NewOutboxRelay(testDB, table, "jaas.test.events", up).RelayOnce(context.Background()); err != nil {
			t.Fatalf("%s: %v", table, err)
		}
	}
}

func TestOutboxRelayRejectsBadTableNames(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("a table name with SQL in it must be refused")
		}
	}()
	kernel.NewOutboxRelay(testDB, "lrn_events_outbox; DROP TABLE users", "x", &capturePublisher{})
}

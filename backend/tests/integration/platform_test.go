package integration

import (
	"testing"

	"github.com/google/uuid"
)

// Platform primitives of restructure Phase B (M0 plan §4): health probes and correlation IDs.

func TestHealthLivenessAndReadiness(t *testing.T) {
	expect(t, call(t, "GET", "/health/live"), 200, "liveness")
	r := call(t, "GET", "/health/ready")
	expect(t, r, 200, "readiness")
	if r.str("data") != "" || r.get("checks.postgres") != "UP" || r.get("checks.redis") != "UP" {
		t.Fatalf("readiness reports each dependency: %s", r.Raw)
	}
}

func TestCorrelationIDReachesAuditTrail(t *testing.T) {
	tc := newTenant(t)
	corr := uuid.New().String()
	r := tc.do(t, "POST", "/roles", body(map[string]any{"name": "corr-" + uniq()}), header("X-Correlation-ID", corr))
	expect(t, r, 201, "create role")
	if r.Header.Get("X-Correlation-ID") != corr {
		t.Fatalf("correlation id echoed: %v", r.Header)
	}
	if got := sqlStr(t, "SELECT count(*) FROM audit_logs WHERE tenant_id = ? AND correlation_id = ?", tc.id, corr); got == "0" {
		t.Fatal("audit rows carry the request correlation id")
	}
	bad := tc.do(t, "GET", "/roles", header("X-Correlation-ID", "not-a-uuid"))
	if _, err := uuid.Parse(bad.Header.Get("X-Correlation-ID")); err != nil {
		t.Fatalf("invalid ids are replaced: %q", bad.Header.Get("X-Correlation-ID"))
	}
}

func TestIdentityEventsGoThroughTheOutbox(t *testing.T) {
	tc := newTenant(t)
	if n := sqlStr(t, "SELECT count(*) FROM identity_events_outbox WHERE tenant_id = ? AND routing_key = 'identity.tenant.created'", tc.id); n != "1" {
		t.Fatalf("tenant.created stored in the outbox: %s", n)
	}
	if v := sqlStr(t, "SELECT payload->>'event_version' FROM identity_events_outbox WHERE tenant_id = ? LIMIT 1", tc.id); v != "1" {
		t.Fatalf("versioned envelope: %s", v)
	}
	// password reset: the token reaches the user through the secret channel, never the event
	expect(t, call(t, "POST", "/auth/forgot-password", body(map[string]string{"tenant_slug": tc.slug, "email": tc.adminEmail})), 200, "forgot")
	if n := sqlStr(t, "SELECT count(*) FROM identity_events_outbox WHERE tenant_id = ? AND payload::text LIKE '%reset_token%'", tc.id); n != "0" {
		t.Fatal("reset token must not be stored in events")
	}
	box := call(t, "GET", "/dev/mailbox", platform(), query("tenant_id", tc.id, "email", tc.adminEmail))
	expect(t, box, 200, "dev mailbox")
	tok := box.str("data.0.token")
	if tok == "" || box.str("data.0.kind") != "password_reset" {
		t.Fatalf("reset secret delivered: %s", box.Raw)
	}
	newPw := "N3w-Passw0rd!"
	expect(t, call(t, "POST", "/auth/reset-password", body(map[string]string{"token": tok, "new_password": newPw, "confirm_password": newPw})), 200, "reset")
	login(t, tc.slug, tc.adminEmail, newPw)
	expectErr(t, call(t, "GET", "/dev/mailbox", query("tenant_id", tc.id, "email", tc.adminEmail)), 401, "", "mailbox needs the platform key")
}

func TestIdempotencyKeyReplaysAndRejectsReuse(t *testing.T) {
	tc := newTenant(t)
	name := "idem-" + uniq()
	first := tc.do(t, "POST", "/roles", body(map[string]any{"name": name}), header("Idempotency-Key", "k-"+name))
	expect(t, first, 201, "first create")
	again := tc.do(t, "POST", "/roles", body(map[string]any{"name": name}), header("Idempotency-Key", "k-"+name))
	expect(t, again, 201, "replayed create")
	if again.Header.Get("Idempotent-Replayed") != "true" || again.str("data.id") != first.str("data.id") {
		t.Fatalf("replay returns the original outcome: %s", again.Raw)
	}
	if n := sqlStr(t, "SELECT count(*) FROM roles WHERE tenant_id = ? AND name = ?", tc.id, name); n != "1" {
		t.Fatalf("executed once: %s roles", n)
	}
	expectErr(t, tc.do(t, "POST", "/roles", body(map[string]any{"name": name + "x"}), header("Idempotency-Key", "k-"+name)), 409, "IDEMPOTENCY_KEY_REUSED", "same key, different payload")
	// a failed request (4xx) is replayed as the same failure, not re-executed
	dup := tc.do(t, "POST", "/roles", body(map[string]any{"name": name}), header("Idempotency-Key", "dup-"+name))
	expectErr(t, dup, 409, "CONFLICT", "duplicate name")
	// tenant creation through the platform key is keyed too
	b := map[string]any{"name": "Idem " + name, "slug": "i" + uniq()}
	p1 := call(t, "POST", "/tenants", platform(), body(b), header("Idempotency-Key", "t-"+name))
	p2 := call(t, "POST", "/tenants", platform(), body(b), header("Idempotency-Key", "t-"+name))
	expect(t, p1, 201, "tenant")
	if p2.Status != 201 || p2.str("data.id") != p1.str("data.id") {
		t.Fatalf("tenant replay: %s", p2.Raw)
	}
}

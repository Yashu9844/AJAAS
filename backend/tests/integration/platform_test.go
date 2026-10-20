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

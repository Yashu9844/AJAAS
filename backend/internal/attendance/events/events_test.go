package events

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestNewEnvelope(t *testing.T) {
	tenant, corr := uuid.New(), uuid.New()
	at := time.Date(2026, 10, 9, 9, 0, 0, 0, time.FixedZone("IST", 19800))
	env := New(PunchIn, tenant, corr, at, PunchPayload{PunchType: "in"})
	if env.EventID == uuid.Nil || env.EventType != PunchIn || env.RoutingKey != PunchIn {
		t.Fatalf("bad envelope: %+v", env)
	}
	if env.Version != "1.0" || env.Producer != "attendance-service" || env.TenantID != tenant || env.CorrelationID != corr {
		t.Fatalf("bad envelope metadata: %+v", env)
	}
	if env.OccurredAt.Location() != time.UTC || !env.OccurredAt.Equal(at) {
		t.Fatalf("occurred_at must be UTC, got %v", env.OccurredAt)
	}
}

// TestPayloadsCarryNoLocationOrNetworkData guards FR-EV003 at the type level.
func TestPayloadsCarryNoLocationOrNetworkData(t *testing.T) {
	for _, p := range []interface{}{PunchPayload{}, RegularizationPayload{}} {
		raw, _ := json.Marshal(p)
		for _, banned := range []string{"ip", "device", "latitude", "longitude"} {
			if strings.Contains(string(raw), banned) {
				t.Errorf("%T exposes %q: %s", p, banned, raw)
			}
		}
	}
}

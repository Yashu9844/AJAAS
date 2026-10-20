package events

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestNew_Envelope(t *testing.T) {
	tenant, corr := uuid.New(), uuid.New()
	at := time.Date(2026, 10, 9, 10, 0, 0, 0, time.FixedZone("IST", 19800))
	env := New(Applied, Source{TenantID: tenant, CorrelationID: corr, OccurredAt: at}, RequestPayload{Status: "pending"})
	if env.EventID == uuid.Nil || env.RoutingKey != Applied || env.EventType != Applied || env.Version != "1.0" ||
		env.Producer != "leave-service" || env.TenantID != tenant || env.CorrelationID != corr || env.OccurredAt.Location() != time.UTC {
		t.Fatalf("envelope = %+v", env)
	}
}

// FR-EV003: payloads carry no free text.
func TestPayloads_NoFreeText(t *testing.T) {
	raw, _ := json.Marshal(RequestPayload{TotalDays: 150})
	for _, banned := range []string{"reason", "comment", "note"} {
		if strings.Contains(string(raw), banned) {
			t.Fatalf("payload must not carry %s: %s", banned, raw)
		}
	}
	if !strings.Contains(string(raw), `"total_days":1.50`) {
		t.Fatalf("days serialized as number: %s", raw)
	}
	raw, _ = json.Marshal(AccruedPayload{Days: 150, AccruedTotal: 450, Year: 2026})
	if !strings.Contains(string(raw), `"accrued_total":4.50`) {
		t.Fatalf("accrued payload: %s", raw)
	}
}

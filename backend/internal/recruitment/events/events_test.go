package events

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestNewEnvelope(t *testing.T) {
	cand := uuid.New()
	env := New(CandidateApplied, Source{TenantID: uuid.New(), CorrelationID: uuid.New(), OccurredAt: time.Now()}, Payload{JobID: uuid.New(), CandidateID: &cand, Stage: "applied"})
	if env.EventID == uuid.Nil || env.Producer != "recruitment-service" || env.RoutingKey != CandidateApplied || env.OccurredAt.Location() != time.UTC {
		t.Fatalf("%+v", env)
	}
	raw, _ := json.Marshal(env.Payload)
	for _, banned := range []string{"email", "name", "ctc", "feedback"} {
		if strings.Contains(string(raw), banned) {
			t.Fatalf("payload carries %s: %s", banned, raw)
		}
	}
}

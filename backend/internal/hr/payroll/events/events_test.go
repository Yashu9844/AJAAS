package events

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestNewAndPayload(t *testing.T) {
	src := Source{TenantID: uuid.New(), CorrelationID: uuid.New(), OccurredAt: time.Date(2026, 10, 9, 10, 0, 0, 0, time.FixedZone("IST", 19800))}
	env := New(RunFinalized, src, RunPayload{Year: 2026, Month: 10, NetTotal: 150000})
	if env.EventID == uuid.Nil || env.RoutingKey != RunFinalized || env.Producer != "payroll-service" || env.OccurredAt.Location() != time.UTC {
		t.Fatalf("envelope %+v", env)
	}
	raw, _ := json.Marshal(env.Payload)
	if !strings.Contains(string(raw), `"net_total":1500.00`) || strings.Contains(string(raw), "employee_id") {
		t.Fatalf("payload %s", raw)
	}
}

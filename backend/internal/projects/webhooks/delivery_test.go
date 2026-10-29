package webhooks

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/kernel"
	"gorm.io/gorm"
)

func TestBackoffSchedule(t *testing.T) {
	want := []time.Duration{time.Minute, 5 * time.Minute, 30 * time.Minute, 2 * time.Hour, 12 * time.Hour}
	for i, d := range want {
		if got := backoff(i+1, 0.5); got != d {
			t.Errorf("attempt %d: %v want %v", i+1, got, d)
		}
	}
	if backoff(0, 0.5) != time.Minute || backoff(9, 0.5) != 12*time.Hour {
		t.Error("out-of-range attempts clamp")
	}
	if lo, hi := backoff(1, 0), backoff(1, 0.999999); lo != time.Duration(0.8*float64(time.Minute)) || hi <= time.Minute || hi > 72*time.Second {
		t.Errorf("jitter spread: %v %v", lo, hi)
	}
}

func (e *env) taskEvent(key string, projectID, taskID uuid.UUID) {
	e.emit(key, map[string]any{"project_id": projectID, "task_id": taskID, "version": 1})
}

type deliveryRow struct {
	ID            uuid.UUID
	WebhookID     uuid.UUID
	Status        string
	Attempts      int
	NextAttemptAt *time.Time
	LastError     string
}

func (e *env) deliveries(webhookID uuid.UUID) []deliveryRow {
	var rows []deliveryRow
	e.db.Raw(`SELECT id, webhook_id, status, attempts, next_attempt_at, last_error FROM webhook_deliveries WHERE webhook_id = ? ORDER BY created_at, id`, webhookID).Scan(&rows)
	return rows
}

func TestDispatchMatchesProjectAndEventTypes(t *testing.T) {
	e := newEnv(t)
	owner := e.user(false)
	p, other := e.project(owner, "DSP"), e.project(owner, "OTH")
	tid := e.task(owner, p, 1)
	all := e.webhook(owner, p, WebhookInput{Name: "all"})
	moved := e.webhook(owner, p, WebhookInput{Name: "moved", EventTypes: []string{"task.moved"}})
	off := e.webhook(owner, p, WebhookInput{Name: "off"})
	otherHook := e.webhook(owner, other, WebhookInput{Name: "other"})
	f := false
	if _, err := e.svc.UpdateWebhook(bg, owner, p.ID, off.ID, WebhookUpdate{Active: &f}); err != nil {
		t.Fatal(err)
	}

	e.taskEvent("task.created", p.ID, tid)
	e.emit("project.member.removed", map[string]any{"user_id": uuid.New()}) // no project_id: skipped
	e.emit("sprint.started", map[string]any{"project_id": other.ID, "sprint_id": uuid.New()})

	n, err := e.svc.DispatchOnce(bg)
	if err != nil || n != 2 {
		t.Fatalf("dispatch queued %d, %v (want the all-webhook for task.created and the other project's hook for sprint.started)", n, err)
	}
	if len(e.deliveries(all.ID)) != 1 || len(e.deliveries(moved.ID)) != 0 || len(e.deliveries(off.ID)) != 0 || len(e.deliveries(otherHook.ID)) != 1 {
		t.Fatal("wrong fan-out")
	}
	if n, _ := e.svc.DispatchOnce(bg); n != 0 {
		t.Fatalf("a second pass re-queues nothing, got %d", n)
	}
	e.taskEvent("task.moved", p.ID, tid)
	if n, _ := e.svc.DispatchOnce(bg); n != 2 { // all + moved
		t.Fatalf("task.moved goes to the two matching webhooks, got %d", n)
	}
	// A webhook only sees events created after it.
	late := e.webhook(owner, p, WebhookInput{Name: "late"})
	if n, _ := e.svc.DispatchOnce(bg); n != 0 || len(e.deliveries(late.ID)) != 0 {
		t.Fatal("past events are not replayed to a new webhook")
	}
	// The broker relay's published flag does not matter.
	e.db.Exec(`UPDATE projects_events_outbox SET published = TRUE`)
	e.taskEvent("task.updated", p.ID, tid)
	if n, _ := e.svc.DispatchOnce(bg); n != 2 { // all + late
		t.Fatalf("dispatch is independent of the relay, got %d", n)
	}
}

// A row that becomes visible after a later one (created_at is the transaction start) is still picked up.
func TestDispatchLookbackCatchesLateCommits(t *testing.T) {
	e := newEnv(t)
	owner := e.user(false)
	p := e.project(owner, "LTE")
	w := e.webhook(owner, p, WebhookInput{})
	e.db.Exec(`UPDATE webhooks SET created_at = now() - interval '1 minute' WHERE id = ?`, w.ID)
	e.taskEvent("task.created", p.ID, e.task(owner, p, 1))
	first, _ := e.svc.DispatchOnce(bg)
	if first < 1 {
		t.Fatalf("first pass: %d", first)
	}
	eventID := uuid.New()
	env := `{"event_id":"` + eventID.String() + `","event_type":"task.updated","routing_key":"task.updated","payload":{"project_id":"` + p.ID.String() + `"}}`
	err := e.db.Exec(`INSERT INTO projects_events_outbox (id, tenant_id, event_id, routing_key, payload, created_at) VALUES (?, ?, ?, 'task.updated', ?::jsonb, now() - interval '10 seconds')`,
		uuid.New(), e.tenant, eventID, env).Error
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := e.svc.DispatchOnce(bg); n != 1 || len(e.deliveries(w.ID)) != first+1 {
		t.Fatalf("the late row must be dispatched, queued %d", n)
	}
}

func TestSendSignedGenericBody(t *testing.T) {
	e := newEnv(t)
	owner := e.user(false)
	p := e.project(owner, "SND")
	tid := e.task(owner, p, 7)
	w := e.webhook(owner, p, WebhookInput{URL: "https://hooks.example.com/in?token=abc"})
	if err := e.db.Transaction(func(tx *gorm.DB) error {
		return kernel.Emit(kernel.WithAgent(bg, "triage-bot"), tx, e.tenant, "task.created", map[string]any{"project_id": p.ID, "task_id": tid, "version": 1})
	}); err != nil {
		t.Fatal(err)
	}
	if n, _ := e.svc.DispatchOnce(bg); n != 1 {
		t.Fatal("not queued")
	}
	if n, err := e.svc.SendOnce(bg); n != 1 || err != nil {
		t.Fatalf("send: %d %v", n, err)
	}
	req := e.doer.last()
	if req.URL != "https://hooks.example.com/in?token=abc" {
		t.Fatalf("url %q", req.URL)
	}
	var body map[string]any
	if err := json.Unmarshal(req.Body, &body); err != nil {
		t.Fatal(err)
	}
	if body["event_type"] != "task.created" || body["agent"] != "triage-bot" || body["delivery_id"] != req.Headers["X-JAAS-Delivery"] || body["tenant_id"] != e.tenant.String() {
		t.Fatalf("envelope forwarded unchanged plus delivery id: %s", req.Body)
	}
	if strings.Contains(string(req.Body), "Secret title") {
		t.Fatal("bodies carry ids only, never titles")
	}
	if req.Headers["X-JAAS-Event"] != "task.created" || req.Headers["X-JAAS-Signature"] != Sign([]byte(w.Secret), e.clock.Unix(), req.Body) {
		t.Fatalf("headers: %v", req.Headers)
	}
	d := e.deliveries(w.ID)[0]
	if d.Status != StatusDelivered || d.Attempts != 1 || d.NextAttemptAt != nil || d.LastError != "" {
		t.Fatalf("delivery: %+v", d)
	}
	var code int
	e.db.Raw(`SELECT response_status FROM webhook_deliveries WHERE id = ?`, d.ID).Scan(&code)
	if code != 200 {
		t.Fatalf("response status %d", code)
	}
	if n, _ := e.svc.SendOnce(bg); n != 0 {
		t.Fatal("a delivered delivery is not sent again")
	}
}

func TestChatFormatsCarryOnlyKeys(t *testing.T) {
	e := newEnv(t)
	owner := e.user(false)
	p := e.project(owner, "CHT")
	tid := e.task(owner, p, 12)
	e.webhook(owner, p, WebhookInput{Name: "s", Format: "slack"})
	e.webhook(owner, p, WebhookInput{Name: "d", Format: "discord"})
	e.taskEvent("task.completed", p.ID, tid)
	e.emit("sprint.started", map[string]any{"project_id": p.ID, "sprint_id": uuid.New()})
	e.emit("task.updated", map[string]any{"project_id": p.ID, "task_id": uuid.New()}) // unknown task: project key only
	e.svc.DispatchOnce(bg)
	e.svc.SendOnce(bg)
	texts := map[string]bool{}
	for _, r := range e.doer.reqs {
		var m map[string]string
		if err := json.Unmarshal(r.Body, &m); err != nil {
			t.Fatal(err)
		}
		for k, v := range m {
			if k != "text" && k != "content" {
				t.Fatalf("only one text field is sent: %s", r.Body)
			}
			texts[k+"="+v] = true
		}
	}
	for _, want := range []string{"text=[JAAS] task.completed in CHT (CHT-12)", "content=[JAAS] task.completed in CHT (CHT-12)", "text=[JAAS] sprint.started in CHT", "content=[JAAS] task.updated in CHT"} {
		if !texts[want] {
			t.Errorf("missing %q in %v", want, texts)
		}
	}
	for text := range texts {
		if strings.Contains(text, "Secret title") || strings.Contains(text, "http") {
			t.Errorf("chat text holds free text: %q", text)
		}
	}
}

func TestRetriesBackOffThenDie(t *testing.T) {
	e := newEnv(t)
	owner := e.user(false)
	p := e.project(owner, "RTY")
	w := e.webhook(owner, p, WebhookInput{})
	e.doer.def = Result{Status: 503, Err: "http 503"}
	e.taskEvent("task.created", p.ID, e.task(owner, p, 1))
	e.svc.DispatchOnce(bg)

	waits := []time.Duration{time.Minute, 5 * time.Minute, 30 * time.Minute, 2 * time.Hour, 12 * time.Hour}
	for i, wait := range waits {
		if n, _ := e.svc.SendOnce(bg); n != 1 {
			t.Fatalf("attempt %d not sent", i+1)
		}
		d := e.deliveries(w.ID)[0]
		if d.Status != StatusFailed || d.Attempts != i+1 || d.NextAttemptAt == nil || d.LastError != "http 503" {
			t.Fatalf("after attempt %d: %+v", i+1, d)
		}
		if got := d.NextAttemptAt.Sub(e.clock); got < wait-time.Millisecond || got > wait+time.Millisecond {
			t.Fatalf("attempt %d waits %v, want %v", i+1, got, wait)
		}
		if n, _ := e.svc.SendOnce(bg); n != 0 {
			t.Fatalf("attempt %d: not due yet", i+1)
		}
		e.clock = e.clock.Add(wait)
	}
	if n, _ := e.svc.SendOnce(bg); n != 1 {
		t.Fatal("sixth attempt")
	}
	d := e.deliveries(w.ID)[0]
	if d.Status != StatusDead || d.Attempts != 6 || d.NextAttemptAt != nil {
		t.Fatalf("six attempts then dead: %+v", d)
	}
	e.clock = e.clock.Add(48 * time.Hour)
	if n, _ := e.svc.SendOnce(bg); n != 0 || e.doer.count() != 6 {
		t.Fatal("a dead delivery is never sent again")
	}
	var fc int
	e.db.Raw(`SELECT failure_count FROM webhooks WHERE id = ?`, w.ID).Scan(&fc)
	if fc != 6 {
		t.Fatalf("failure count %d", fc)
	}
	// A success resets the consecutive-failure counter.
	e.doer.def = Result{Status: 200}
	e.taskEvent("task.updated", p.ID, uuid.New())
	e.svc.DispatchOnce(bg)
	e.svc.SendOnce(bg)
	e.db.Raw(`SELECT failure_count FROM webhooks WHERE id = ?`, w.ID).Scan(&fc)
	if fc != 0 {
		t.Fatalf("success resets the counter, got %d", fc)
	}
}

func TestTwentyFailuresDisableTheWebhook(t *testing.T) {
	e := newEnv(t)
	owner := e.user(false)
	p := e.project(owner, "DIS")
	w := e.webhook(owner, p, WebhookInput{})
	e.doer.def = Result{Err: errConnect}
	e.db.Exec(`UPDATE webhooks SET failure_count = 19 WHERE id = ?`, w.ID)
	e.taskEvent("task.created", p.ID, uuid.New())
	e.taskEvent("task.updated", p.ID, uuid.New())
	e.svc.DispatchOnce(bg)
	if n, _ := e.svc.SendOnce(bg); n != 2 {
		t.Fatalf("both deliveries are sent in one pass, got %d", n)
	}
	var row struct {
		FailureCount int
		DisabledAt   *time.Time
	}
	e.db.Raw(`SELECT failure_count, disabled_at FROM webhooks WHERE id = ?`, w.ID).Scan(&row)
	if row.DisabledAt == nil || row.FailureCount != 21 {
		t.Fatalf("disabled after the 20th failure: %+v", row)
	}
	var sys *auditEntry
	for i := range e.audit.entries {
		if e.audit.entries[i].action == "webhook.auto_disabled" {
			sys = &e.audit.entries[i]
		}
	}
	if sys == nil || sys.userID != "" || strings.Contains(sys.meta, "http") {
		t.Fatalf("the automatic disable is audited as a system action: %+v", sys)
	}
	e.clock = e.clock.Add(2 * time.Minute)
	before := e.doer.count()
	e.taskEvent("task.moved", p.ID, uuid.New())
	if n, _ := e.svc.DispatchOnce(bg); n != 0 {
		t.Fatal("a disabled webhook receives no new deliveries")
	}
	if n, _ := e.svc.SendOnce(bg); n != 0 || e.doer.count() != before {
		t.Fatal("pending deliveries of a disabled webhook wait")
	}
	on := true
	if _, err := e.svc.UpdateWebhook(bg, owner, p.ID, w.ID, WebhookUpdate{Active: &on}); err != nil {
		t.Fatal(err)
	}
	e.clock = e.clock.Add(10 * time.Minute)
	if n, _ := e.svc.SendOnce(bg); n != 2 {
		t.Fatalf("after re-enabling, due retries resume: %d", n)
	}
}

func TestPingIsRecordedAndNeverRetried(t *testing.T) {
	e := newEnv(t)
	owner := e.user(false)
	p := e.project(owner, "PNG")
	w := e.webhook(owner, p, WebhookInput{Format: "slack"})
	e.doer.def = Result{Err: errTimeout}
	d, err := e.svc.TestWebhook(bg, owner, p.ID, w.ID)
	if err != nil || d.Status != StatusFailed || d.Attempts != 1 || d.LastError != errTimeout || d.NextAttemptAt != nil || d.EventType != pingEvent {
		t.Fatalf("failed ping: %+v %v", d, err)
	}
	var m map[string]string
	json.Unmarshal(e.doer.last().Body, &m)
	if m["text"] != "[JAAS] webhook.test in PNG" {
		t.Fatalf("ping text %v", m)
	}
	e.clock = e.clock.Add(time.Hour)
	if n, _ := e.svc.SendOnce(bg); n != 0 {
		t.Fatal("pings are not retried")
	}
	var fc int
	e.db.Raw(`SELECT failure_count FROM webhooks WHERE id = ?`, w.ID).Scan(&fc)
	if fc != 0 {
		t.Fatal("a failed ping does not count against the webhook")
	}
	e.doer.def = Result{Status: 204}
	if d, err = e.svc.TestWebhook(bg, owner, p.ID, w.ID); err != nil || d.Status != StatusDelivered || d.DeliveredAt == nil {
		t.Fatalf("ok ping: %+v %v", d, err)
	}
	for i := 0; i < maxPingsPerMinute; i++ {
		_, err = e.svc.TestWebhook(bg, owner, p.ID, w.ID)
	}
	wantCode(t, err, "RATE_LIMITED")
	_, err = e.svc.TestWebhook(bg, owner, p.ID, uuid.New())
	wantCode(t, err, "NOT_FOUND")
}

// With the real sender a ping to a name that cannot resolve is recorded as failed (no receiver is reachable in tests).
func TestPingWithRealSenderFailsSafely(t *testing.T) {
	e := newEnv(t)
	e.svc.SetSender(NewSender())
	owner := e.user(false)
	p := e.project(owner, "REA")
	w := e.webhook(owner, p, WebhookInput{URL: "https://receiver.invalid/hook"})
	d, err := e.svc.TestWebhook(bg, owner, p.ID, w.ID)
	if err != nil || d.Status != StatusFailed || d.LastError != errConnect || d.ResponseStatus != nil {
		t.Fatalf("%+v %v", d, err)
	}
	// A URL that was valid when stored but is refused now (rules changed, or the row was edited) fails at send time too.
	e.db.Exec(`UPDATE webhooks SET url = 'https://169.254.169.254/latest' WHERE id = ?`, w.ID)
	if d, err = e.svc.TestWebhook(bg, owner, p.ID, w.ID); err != nil || d.LastError != errBadURL {
		t.Fatalf("send-time validation: %+v %v", d, err)
	}
}

func TestRedeliver(t *testing.T) {
	e := newEnv(t)
	owner := e.user(false)
	p := e.project(owner, "RDL")
	w := e.webhook(owner, p, WebhookInput{})
	e.taskEvent("task.created", p.ID, uuid.New())
	e.svc.DispatchOnce(bg)
	id := e.deliveries(w.ID)[0].ID
	_, err := e.svc.Redeliver(bg, owner, p.ID, w.ID, id)
	wantCode(t, err, "DELIVERY_PENDING")
	e.svc.SendOnce(bg)
	d, err := e.svc.Redeliver(bg, owner, p.ID, w.ID, id)
	if err != nil || d.Status != StatusPending || d.Attempts != 0 || d.DeliveredAt != nil || d.NextAttemptAt == nil {
		t.Fatalf("redeliver: %+v %v", d, err)
	}
	if n, _ := e.svc.SendOnce(bg); n != 1 || e.doer.count() != 2 {
		t.Fatal("the redelivery is sent")
	}
	_, err = e.svc.Redeliver(bg, owner, p.ID, w.ID, uuid.New())
	wantCode(t, err, "NOT_FOUND")
	_, err = e.svc.Redeliver(bg, owner, p.ID, uuid.New(), id)
	wantCode(t, err, "NOT_FOUND")
	off := false
	e.svc.UpdateWebhook(bg, owner, p.ID, w.ID, WebhookUpdate{Active: &off})
	_, err = e.svc.Redeliver(bg, owner, p.ID, w.ID, id)
	wantCode(t, err, "WEBHOOK_DISABLED")
}

func TestDeliveryListing(t *testing.T) {
	e := newEnv(t)
	owner := e.user(false)
	p := e.project(owner, "LST")
	w := e.webhook(owner, p, WebhookInput{})
	e.doer.def = Result{Err: errConnect}
	for i := 0; i < 3; i++ {
		e.clock = e.clock.Add(time.Second)
		if _, err := e.svc.TestWebhook(bg, owner, p.ID, w.ID); err != nil {
			t.Fatal(err)
		}
	}
	rows, next, err := e.svc.Deliveries(bg, owner, p.ID, w.ID, DeliveryFilter{Limit: 2})
	if err != nil || len(rows) != 2 || next == "" {
		t.Fatalf("page 1: %d %q %v", len(rows), next, err)
	}
	rest, next2, err := e.svc.Deliveries(bg, owner, p.ID, w.ID, DeliveryFilter{Limit: 2, Cursor: next})
	if err != nil || len(rest) != 1 || next2 != "" || rest[0].ID == rows[0].ID || rest[0].ID == rows[1].ID {
		t.Fatalf("page 2: %d %v", len(rest), err)
	}
	if rows, _, _ := e.svc.Deliveries(bg, owner, p.ID, w.ID, DeliveryFilter{Limit: 10, Status: StatusDelivered}); len(rows) != 0 {
		t.Fatal("status filter")
	}
	_, _, err = e.svc.Deliveries(bg, owner, p.ID, w.ID, DeliveryFilter{Limit: 10, Status: "bogus"})
	wantCode(t, err, "VALIDATION_ERROR")
	_, _, err = e.svc.Deliveries(bg, owner, p.ID, w.ID, DeliveryFilter{Limit: 10, Cursor: "!!"})
	wantCode(t, err, "VALIDATION_ERROR")
	if toJSON(rows) == "" || strings.Contains(toJSON(rows), "hooks.example.com") {
		t.Fatal("deliveries never expose the URL")
	}
}

func TestMissingEventOrSecretFailsTheAttempt(t *testing.T) {
	e := newEnv(t)
	owner := e.user(false)
	p := e.project(owner, "MIS")
	w := e.webhook(owner, p, WebhookInput{})
	e.taskEvent("task.created", p.ID, uuid.New())
	e.svc.DispatchOnce(bg)
	e.db.Exec(`DELETE FROM projects_events_outbox`)
	e.svc.SendOnce(bg)
	if d := e.deliveries(w.ID)[0]; d.Status != StatusFailed || d.LastError != errNoEvent || e.doer.count() != 0 {
		t.Fatalf("missing event: %+v", d)
	}
	e.clock = e.clock.Add(2 * time.Minute)
	e.db.Exec(`UPDATE webhooks SET secret_enc = '\x0102' WHERE id = ?`, w.ID)
	e.svc.SendOnce(bg)
	if d := e.deliveries(w.ID)[0]; d.LastError != errSecret || e.doer.count() != 0 {
		t.Fatalf("unreadable secret: %+v", d)
	}
}

func TestParallelWorkersSendEachDeliveryOnce(t *testing.T) {
	e := newEnv(t)
	owner := e.user(false)
	p := e.project(owner, "PAR")
	e.webhook(owner, p, WebhookInput{})
	for i := 0; i < 12; i++ {
		e.taskEvent("task.updated", p.ID, uuid.New())
	}
	e.svc.DispatchOnce(bg)
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); e.svc.SendOnce(bg); e.svc.SendOnce(bg) }()
	}
	wg.Wait()
	if e.doer.count() != 12 {
		t.Fatalf("every delivery is sent exactly once, sent %d", e.doer.count())
	}
}

func TestPurgeAndRunOnce(t *testing.T) {
	e := newEnv(t)
	owner := e.user(false)
	p := e.project(owner, "PRG")
	w := e.webhook(owner, p, WebhookInput{})
	e.taskEvent("task.created", p.ID, uuid.New())
	var errs []error
	e.svc.RunOnce(bg, func(err error) { errs = append(errs, err) })
	if len(errs) != 0 || e.doer.count() != 1 {
		t.Fatalf("one pass dispatches and sends: %v %d", errs, e.doer.count())
	}
	e.db.Exec(`UPDATE webhook_deliveries SET created_at = now() - interval '40 days' WHERE webhook_id = ?`, w.ID)
	if err := e.svc.Purge(bg); err != nil || len(e.deliveries(w.ID)) != 0 {
		t.Fatalf("purge: %v", err)
	}
	ctx, cancel := contextWithCancel()
	cancel()
	e.svc.Run(ctx, func(error) {}) // returns at once on a cancelled context
}

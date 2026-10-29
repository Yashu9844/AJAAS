package webhooks

import (
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/project"
)

func TestWebhookLifecycle(t *testing.T) {
	e := newEnv(t)
	owner := e.user(false)
	p := e.project(owner, "WH1")

	w, err := e.svc.CreateWebhook(bg, owner, p.ID, WebhookInput{Name: " Build bot ", URL: "https://hooks.example.com/in", Format: "slack", EventTypes: []string{"task.created", "task.created", "task.moved"}})
	if err != nil {
		t.Fatal(err)
	}
	if w.Name != "Build bot" || w.Format != FormatSlack || len(w.EventTypes) != 2 || !w.Active || !strings.HasPrefix(w.Secret, "whsec_") || w.SecretHint != Hint(w.Secret) {
		t.Fatalf("created: %+v", w)
	}
	raw := toJSON(w.Webhook)
	if strings.Contains(raw, w.Secret) || strings.Contains(raw, "secret_enc") {
		t.Fatalf("only the creation response carries the secret: %s", raw)
	}
	var stored []byte
	e.db.Raw(`SELECT secret_enc FROM webhooks WHERE id = ?`, w.ID).Scan(&stored)
	if len(stored) == 0 || strings.Contains(string(stored), w.Secret) {
		t.Fatal("the secret is stored encrypted")
	}
	if !e.audit.has("webhook.created") {
		t.Fatal("creation is audited")
	}
	if strings.Contains(toJSON(e.audit.entries), "hooks.example.com") || strings.Contains(toJSON(e.audit.entries), w.Secret) {
		t.Fatal("audit never holds the URL or the secret")
	}

	list, err := e.svc.Webhooks(bg, owner, p.ID)
	if err != nil || len(list) != 1 || list[0].ID != w.ID {
		t.Fatalf("list: %v %v", list, err)
	}
	name, active := "Renamed", false
	up, err := e.svc.UpdateWebhook(bg, owner, p.ID, w.ID, WebhookUpdate{Name: &name, Active: &active, EventTypes: &[]string{"*", "task.created"}, Format: ptr("discord"), URL: ptr("https://discord.com/api/webhooks/1/x")})
	if err != nil || up.Name != "Renamed" || up.Active || up.Format != FormatDiscord || len(up.EventTypes) != 1 || up.EventTypes[0] != "*" {
		t.Fatalf("update: %+v %v", up, err)
	}
	// Re-enabling clears the automatic disable.
	e.db.Exec(`UPDATE webhooks SET failure_count = 20, disabled_at = now() WHERE id = ?`, w.ID)
	on := true
	up, err = e.svc.UpdateWebhook(bg, owner, p.ID, w.ID, WebhookUpdate{Active: &on})
	if err != nil || up.DisabledAt != nil || up.FailureCount != 0 || !up.Active {
		t.Fatalf("re-enable: %+v %v", up, err)
	}

	rot, err := e.svc.RotateWebhookSecret(bg, owner, p.ID, w.ID)
	if err != nil || rot.Secret == w.Secret || rot.SecretHint != Hint(rot.Secret) {
		t.Fatalf("rotate: %+v %v", rot, err)
	}
	if err := e.svc.DeleteWebhook(bg, owner, p.ID, w.ID); err != nil {
		t.Fatal(err)
	}
	wantCode(t, e.svc.DeleteWebhook(bg, owner, p.ID, w.ID), "NOT_FOUND")
	for _, a := range []string{"webhook.updated", "webhook.secret_rotated", "webhook.deleted"} {
		if !e.audit.has(a) {
			t.Errorf("missing audit %s", a)
		}
	}
}

func ptr[T any](v T) *T { return &v }

func TestWebhookValidation(t *testing.T) {
	e := newEnv(t)
	owner := e.user(false)
	p := e.project(owner, "WH2")
	bad := []WebhookInput{
		{Name: "", URL: "https://example.com/x"},
		{Name: strings.Repeat("n", 61), URL: "https://example.com/x"},
		{Name: "a", URL: "http://example.com/x"},
		{Name: "a", URL: "https://127.0.0.1/x"},
		{Name: "a", URL: "https://169.254.169.254/latest"},
		{Name: "a", URL: "https://[::ffff:127.0.0.1]/"},
		{Name: "a", URL: "https://example.com/x", Format: "teams"},
		{Name: "a", URL: "https://example.com/x", EventTypes: []string{"Task.Created"}},
		{Name: "a", URL: "https://example.com/x", EventTypes: []string{"nodots"}},
		{Name: "a", URL: "https://example.com/x", EventTypes: make([]string, 31)},
	}
	for i, in := range bad {
		if _, err := e.svc.CreateWebhook(bg, owner, p.ID, in); err == nil || !strings.Contains(err.Error(), "VALIDATION_ERROR") {
			t.Errorf("case %d must be a validation error, got %v", i, err)
		}
	}
	if e.count(`SELECT count(*) FROM webhooks`) != 0 {
		t.Fatal("nothing may be stored for a refused request")
	}
	w := e.webhook(owner, p, WebhookInput{Name: "Dup"})
	if len(w.EventTypes) != 1 || w.EventTypes[0] != "*" || w.Format != FormatGeneric {
		t.Fatalf("defaults: %+v", w.Webhook)
	}
	_, err := e.svc.CreateWebhook(bg, owner, p.ID, WebhookInput{Name: "dup", URL: "https://example.com/y"})
	wantCode(t, err, "WEBHOOK_NAME_TAKEN")
	bad2 := "http://insecure.example.com"
	_, err = e.svc.UpdateWebhook(bg, owner, p.ID, w.ID, WebhookUpdate{URL: &bad2})
	wantCode(t, err, "VALIDATION_ERROR")
	other := e.webhook(owner, p, WebhookInput{Name: "Other"})
	dupName := "DUP"
	_, err = e.svc.UpdateWebhook(bg, owner, p.ID, other.ID, WebhookUpdate{Name: &dupName})
	wantCode(t, err, "WEBHOOK_NAME_TAKEN")
	for i, in := range []WebhookUpdate{{Name: ptr("")}, {Format: ptr("x")}, {EventTypes: &[]string{"bad"}}} {
		if _, err := e.svc.UpdateWebhook(bg, owner, p.ID, w.ID, in); err == nil {
			t.Errorf("update case %d must fail", i)
		}
	}
}

func TestWebhookLimitPerProject(t *testing.T) {
	e := newEnv(t)
	owner := e.user(false)
	p := e.project(owner, "WH3")
	for i := 0; i < maxWebhooksPerProject; i++ {
		e.webhook(owner, p, WebhookInput{Name: "h" + strconv.Itoa(i)})
	}
	_, err := e.svc.CreateWebhook(bg, owner, p.ID, WebhookInput{Name: "one more", URL: "https://example.com/x"})
	wantCode(t, err, "WEBHOOK_LIMIT")
	p2 := e.project(owner, "WH4")
	e.webhook(owner, p2, WebhookInput{Name: "h0"})
}

// Permissions: project.update manages, project.read lists deliveries, outsiders and foreign tenants get 404.
func TestWebhookAccessRules(t *testing.T) {
	e := newEnv(t)
	owner := e.user(false)
	viewer, outsider := e.user(false), e.user(false)
	p := e.project(owner, "WH5")
	if _, err := e.proj.AddMember(bg, owner, p.ID, project.MemberInput{UserID: viewer.UserID, Role: api.RoleViewer}); err != nil {
		t.Fatal(err)
	}
	w := e.webhook(owner, p, WebhookInput{})

	_, err := e.svc.CreateWebhook(bg, viewer, p.ID, WebhookInput{Name: "x", URL: "https://example.com/x"})
	wantCode(t, err, "FORBIDDEN")
	_, err = e.svc.Webhooks(bg, viewer, p.ID)
	wantCode(t, err, "FORBIDDEN")
	if _, _, err = e.svc.Deliveries(bg, viewer, p.ID, w.ID, DeliveryFilter{Limit: 10}); err != nil {
		t.Fatalf("project.read lists deliveries: %v", err)
	}
	_, err = e.svc.TestWebhook(bg, viewer, p.ID, w.ID)
	wantCode(t, err, "FORBIDDEN")
	wantCode(t, e.svc.DeleteWebhook(bg, viewer, p.ID, w.ID), "FORBIDDEN")
	_, err = e.svc.Webhooks(bg, outsider, p.ID)
	wantCode(t, err, "NOT_FOUND")
	_, _, err = e.svc.Deliveries(bg, outsider, p.ID, w.ID, DeliveryFilter{Limit: 10})
	wantCode(t, err, "NOT_FOUND")

	foreign := api.Actor{TenantID: uuid.New(), UserID: uuid.New(), ManageAll: true}
	_, err = e.svc.Webhooks(bg, foreign, p.ID)
	wantCode(t, err, "NOT_FOUND")
	_, err = e.svc.RotateWebhookSecret(bg, owner, p.ID, uuid.New())
	wantCode(t, err, "NOT_FOUND")
	p2 := e.project(owner, "WH6")
	_, err = e.svc.UpdateWebhook(bg, owner, p2.ID, w.ID, WebhookUpdate{Name: ptr("x")})
	wantCode(t, err, "NOT_FOUND") // a webhook id from another project is not reachable through this one
	if _, err := e.proj.Archive(bg, owner, p2.ID); err != nil {
		t.Fatal(err)
	}
	_, err = e.svc.CreateWebhook(bg, owner, p2.ID, WebhookInput{Name: "x", URL: "https://example.com/x"})
	wantCode(t, err, "PROJECT_ARCHIVED")
}

func TestSecretsDisabledWithoutKey(t *testing.T) {
	e := newEnv(t)
	svc, err := NewService(e.db, e.proj, testResolver{e.proj}, Ports{Audit: e.audit})
	if err != nil {
		t.Fatal(err)
	}
	owner := e.user(false)
	p := e.project(owner, "WH7")
	_, err = svc.CreateWebhook(bg, owner, p.ID, WebhookInput{Name: "x", URL: "https://example.com/x"})
	wantCode(t, err, "SECRETS_UNAVAILABLE")
	_, err = svc.CreateIntegration(bg, owner, p.ID, IntegrationInput{Provider: "github", Name: "x"})
	wantCode(t, err, "SECRETS_UNAVAILABLE")
	if _, err := NewService(e.db, e.proj, testResolver{e.proj}, Ports{SecretsKey: "bad"}); err == nil {
		t.Fatal("a configured but invalid key is an error")
	}
}

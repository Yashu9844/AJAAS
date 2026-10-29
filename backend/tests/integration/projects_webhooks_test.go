package integration

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
)

// Webhooks, chat and git integrations through the production router: the SSRF guard refuses private destinations, the
// secret is shown once, a test ping to an unresolvable host is recorded as failed, a signed provider call links tasks of
// its own project only, and bad signatures answer the same 404 as unknown ids.
func TestWebhooksThroughTheRouter(t *testing.T) {
	tc := newTenant(t)
	owner, _, _ := tc.limitedUser(t, [2]string{"projects", "create"})
	stranger, _, _ := tc.limitedUser(t)

	pid := tc.asTok(t, owner, "POST", "/projects", body(map[string]any{"key": "WHK" + uniq()[:3], "name": "Hooks"})).str("data.id")
	base := "/projects/" + pid + "/webhooks"
	for _, url := range []string{"http://example.com/h", "https://127.0.0.1/h", "https://[::ffff:127.0.0.1]/h", "https://169.254.169.254/latest/meta-data/",
		"https://10.0.0.5/h", "https://0x7f.1/h", "https://user:pw@example.com/h", "https://example.com:8080/h"} {
		expectErr(t, tc.asTok(t, owner, "POST", base, body(map[string]any{"name": "bad", "url": url})), 400, "VALIDATION_ERROR", "refused "+url)
	}
	created := tc.asTok(t, owner, "POST", base, body(map[string]any{"name": "CI", "url": "https://receiver.invalid/hook", "event_types": []string{"task.created"}}))
	expect(t, created, 201, "create webhook")
	if !strings.HasPrefix(created.str("data.secret"), "whsec_") {
		t.Fatalf("the secret is shown once at creation: %s", created.Raw)
	}
	wid := created.str("data.id")
	list := tc.asTok(t, owner, "GET", base)
	expect(t, list, 200, "list")
	if strings.Contains(list.Raw, "whsec_") || strings.Contains(list.Raw, "secret_enc") {
		t.Fatalf("the list never carries a secret: %s", list.Raw)
	}
	expectErr(t, tc.asTok(t, stranger, "GET", base), 404, "NOT_FOUND", "outsiders see nothing")

	ping := tc.asTok(t, owner, "POST", base+"/"+wid+"/test")
	expect(t, ping, 200, "test ping")
	if ping.str("data.status") != "failed" || ping.get("data.next_attempt_at") != nil || ping.str("data.last_error") == "" || strings.Contains(ping.Raw, "receiver.invalid") {
		t.Fatalf("the ping is recorded as failed, never retried, without the URL: %s", ping.Raw)
	}
	deliveries := tc.asTok(t, owner, "GET", base+"/"+wid+"/deliveries")
	expect(t, deliveries, 200, "deliveries")
	if len(deliveries.list("data")) != 1 {
		t.Fatalf("one delivery: %s", deliveries.Raw)
	}
	expect(t, tc.asTok(t, owner, "POST", base+"/"+wid+"/deliveries/"+ping.str("data.id")+"/redeliver"), 200, "redeliver")
	rot := tc.asTok(t, owner, "POST", base+"/"+wid+"/rotate-secret")
	expect(t, rot, 200, "rotate")
	if rot.str("data.secret") == created.str("data.secret") {
		t.Fatal("rotation changes the secret")
	}
	expect(t, tc.asTok(t, owner, "DELETE", base+"/"+wid), 200, "delete")
	expectErr(t, tc.asTok(t, owner, "DELETE", base+"/"+wid), 404, "NOT_FOUND", "gone")
}

func TestGitIntegrationThroughTheRouter(t *testing.T) {
	tc := newTenant(t)
	owner, _, _ := tc.limitedUser(t, [2]string{"projects", "create"})
	stranger, _, _ := tc.limitedUser(t)

	key := "GIT" + strings.ToUpper(uniq()[:3])
	pid := tc.asTok(t, owner, "POST", "/projects", body(map[string]any{"key": key, "name": "Git"})).str("data.id")
	otherKey := "OTR" + strings.ToUpper(uniq()[:3])
	otherPid := tc.asTok(t, owner, "POST", "/projects", body(map[string]any{"key": otherKey, "name": "Other"})).str("data.id")
	task := tc.asTok(t, owner, "POST", "/projects/"+pid+"/tasks", body(map[string]any{"title": "Linked work"}))
	expect(t, task, 201, "task")
	tid := task.str("data.id")
	otherTask := tc.asTok(t, owner, "POST", "/projects/"+otherPid+"/tasks", body(map[string]any{"title": "Foreign"}))
	expect(t, otherTask, 201, "other task")

	base := "/projects/" + pid + "/integrations"
	expectErr(t, tc.asTok(t, owner, "POST", base, body(map[string]any{"provider": "svn", "name": "x"})), 400, "VALIDATION_ERROR", "provider")
	expectErr(t, tc.asTok(t, stranger, "POST", base, body(map[string]any{"provider": "github", "name": "x"})), 404, "NOT_FOUND", "outsider")
	it := tc.asTok(t, owner, "POST", base, body(map[string]any{"provider": "github", "name": "repo"}))
	expect(t, it, 201, "create integration")
	secret, id, path := it.str("data.secret"), it.str("data.id"), it.str("data.path")
	if !strings.HasPrefix(path, "/integrations/github/"+id) || secret == "" {
		t.Fatalf("path and secret are shown once: %s", it.Raw)
	}
	if l := tc.asTok(t, owner, "GET", base); strings.Contains(l.Raw, secret) {
		t.Fatal("the list never carries the secret")
	}

	payload, _ := json.Marshal(map[string]any{"action": "opened", "pull_request": map[string]any{"number": 5, "html_url": "https://github.com/o/r/pull/5",
		"title": key + "-1 add login and " + otherKey + "-1 too", "merged": false, "head": map[string]any{"ref": "feature/x"}}})
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	good := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	// Provider calls carry no bearer token and no tenant host.
	signed := func(sig string) resp {
		return call(t, "POST", path, rawBody(string(payload)), header("X-Hub-Signature-256", sig), header("X-GitHub-Event", "pull_request"))
	}
	ok := signed(good)
	expect(t, ok, 200, "signed call")
	if ok.get("data.linked") != float64(1) {
		t.Fatalf("exactly the task of THIS project links: %s", ok.Raw)
	}
	links := tc.asTok(t, owner, "GET", "/tasks/"+tid+"/links")
	expect(t, links, 200, "links")
	if len(links.list("data")) != 1 || !links.has("data", "kind", "pr") || !links.has("data", "state", "open") || strings.Contains(links.Raw, "add login") {
		t.Fatalf("one pr link, no title stored: %s", links.Raw)
	}
	otherLinks := tc.asTok(t, owner, "GET", "/tasks/"+otherTask.str("data.id")+"/links")
	if len(otherLinks.list("data")) != 0 {
		t.Fatal("a key of another project never links")
	}
	expectErr(t, tc.asTok(t, stranger, "GET", "/tasks/"+tid+"/links"), 404, "NOT_FOUND", "outsiders cannot read links")

	unknown := call(t, "POST", "/integrations/github/00000000-0000-4000-8000-000000000001", rawBody(string(payload)), header("X-Hub-Signature-256", good))
	for name, r := range map[string]resp{"unknown id": unknown, "bad signature": signed("sha256=" + strings.Repeat("0", 64)), "missing signature": signed(""), "wrong prefix": signed(strings.TrimPrefix(good, "sha256="))} {
		if r.Status != 404 || r.Raw != unknown.Raw {
			t.Errorf("%s must answer the identical 404: %d %s", name, r.Status, r.Raw)
		}
	}
	expect(t, tc.asTok(t, owner, "DELETE", base+"/"+id), 200, "delete integration")
	if r := signed(good); r.Status != 404 {
		t.Fatalf("a deleted integration answers 404, got %d", r.Status)
	}
}

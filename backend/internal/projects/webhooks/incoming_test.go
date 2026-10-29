package webhooks

import (
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/project"
)

type gitEnv struct {
	*httpEnv
	owner  api.Actor
	p      *project.Project
	other  *project.Project
	t1, t2 uuid.UUID
	o1     uuid.UUID
	id     string
	secret string
}

func newGitEnv(t *testing.T, provider string) *gitEnv {
	h := newHTTPEnv(t)
	g := &gitEnv{httpEnv: h, owner: h.user(false)}
	g.p, g.other = h.project(g.owner, "GIT"), h.project(g.owner, "OTH")
	g.t1, g.t2, g.o1 = h.task(g.owner, g.p, 1), h.task(g.owner, g.p, 2), h.task(g.owner, g.other, 1)
	it, err := h.svc.CreateIntegration(bg, g.owner, g.p.ID, IntegrationInput{Provider: provider, Name: "repo"})
	if err != nil {
		t.Fatal(err)
	}
	g.id, g.secret = it.ID.String(), it.Secret
	return g
}

func (g *gitEnv) post(provider, id string, body []byte, headers ...string) reply {
	req := g.httpEnv.doRaw("/integrations/"+provider+"/"+id, body, headers...)
	return req
}

func (g *gitEnv) sign(provider string, body []byte) []string {
	sig := hmacHex([]byte(g.secret), body)
	if provider == ProviderGitHub {
		return []string{"X-Hub-Signature-256", "sha256=" + sig, "X-GitHub-Event"}
	}
	return []string{"X-Gitea-Signature", sig, "X-Gitea-Event"}
}

func (g *gitEnv) send(provider, event string, body string) reply {
	h := g.sign(provider, []byte(body))
	return g.post(provider, g.id, []byte(body), h[0], h[1], h[2], event)
}

func prBody(action string, merged bool, title, branch string) string {
	return fmt.Sprintf(`{"action":%q,"pull_request":{"number":42,"html_url":"https://github.com/o/r/pull/42","title":%q,"merged":%v,"head":{"ref":%q}}}`, action, title, merged, branch)
}

func TestGitHubPullRequestFlow(t *testing.T) {
	g := newGitEnv(t, ProviderGitHub)
	// Title names task 1 and a task of ANOTHER project; the branch names task 2. Only this project's tasks link.
	r := g.send(ProviderGitHub, "pull_request", prBody("opened", false, "GIT-1 and OTH-1: Super private title", "feature/git-2-x"))
	if r.Code != 200 || fmt.Sprint(r.Body["data"]) != "map[ignored:false linked:2]" {
		t.Fatalf("opened: %d %s", r.Code, r.Raw)
	}
	if g.count(`SELECT count(*) FROM external_links WHERE tenant_id = ? AND state = 'open' AND kind = 'pr' AND external_id = '42'`, g.tenant) != 2 || g.count(`SELECT count(*) FROM external_links WHERE task_id = ?`, g.o1) != 0 {
		t.Fatal("links go to this project's tasks only")
	}
	if g.count(`SELECT count(*) FROM activity WHERE source = 'integration' AND type = 'external_linked' AND actor_user_id IS NULL`) != 2 {
		t.Fatal("activity rows with source integration")
	}
	var dump string
	g.db.Raw(`SELECT string_agg(l::text, ' ') FROM external_links l`).Scan(&dump)
	g.db.Raw(`SELECT ? || string_agg(a.data::text, ' ') || string_agg(o.payload::text, ' ') FROM activity a, projects_events_outbox o WHERE a.source = 'integration'`, dump).Scan(&dump)
	if strings.Contains(dump, "Super private") || strings.Contains(dump, "git-2-x") {
		t.Fatalf("titles and branch names must not be stored in links, activity or events: %s", dump)
	}
	if g.count(`SELECT count(*) FROM projects_events_outbox WHERE routing_key = 'task.external_linked'`) != 2 || g.count(`SELECT count(*) FROM projects_events_outbox WHERE routing_key = 'task.pr_merged'`) != 0 {
		t.Fatal("external_linked events only")
	}

	// A replay changes nothing and emits nothing.
	if r = g.send(ProviderGitHub, "pull_request", prBody("opened", false, "GIT-1", "")); r.Code != 200 || !strings.Contains(r.Raw, `"linked":0`) {
		t.Fatalf("replay: %s", r.Raw)
	}
	if g.count(`SELECT count(*) FROM projects_events_outbox WHERE routing_key = 'task.external_linked'`) != 2 {
		t.Fatal("a replay emits nothing")
	}

	// Merging moves the state and emits task.pr_merged once per task.
	if r = g.send(ProviderGitHub, "pull_request", prBody("closed", true, "GIT-1", "git-2")); r.Code != 200 || !strings.Contains(r.Raw, `"linked":2`) {
		t.Fatalf("merged: %s", r.Raw)
	}
	if g.count(`SELECT count(*) FROM external_links WHERE state = 'merged'`) != 2 || g.count(`SELECT count(*) FROM projects_events_outbox WHERE routing_key = 'task.pr_merged'`) != 2 {
		t.Fatal("merged links and pr_merged events")
	}
	g.send(ProviderGitHub, "pull_request", prBody("closed", true, "GIT-1", ""))
	if g.count(`SELECT count(*) FROM projects_events_outbox WHERE routing_key = 'task.pr_merged'`) != 2 {
		t.Fatal("pr_merged is emitted once")
	}
	var payload string
	g.db.Raw(`SELECT payload::text FROM projects_events_outbox WHERE routing_key = 'task.pr_merged' LIMIT 1`).Scan(&payload)
	for _, want := range []string{`"project_id"`, `"task_id"`, `"link_id"`, `"state": "merged"`} {
		if !strings.Contains(payload, want) {
			t.Errorf("pr_merged payload lacks %s: %s", want, payload)
		}
	}
	// Reopened and closed-unmerged.
	g.send(ProviderGitHub, "pull_request", prBody("reopened", false, "GIT-1", ""))
	g.send(ProviderGitHub, "pull_request", prBody("closed", false, "GIT-1", ""))
	if g.count(`SELECT count(*) FROM external_links WHERE task_id = ? AND state = 'closed'`, g.t1) != 1 {
		t.Fatal("closed without merge")
	}

	owner := g.owner
	links := g.do(owner, "GET", "/tasks/"+g.t1.String()+"/links", nil)
	if links.Code != 200 || !strings.Contains(links.Raw, `"kind":"pr"`) || !strings.Contains(links.Raw, "https://github.com/o/r/pull/42") || strings.Contains(links.Raw, "Super private") {
		t.Fatalf("links: %s", links.Raw)
	}
}

func TestGiteaPushFlow(t *testing.T) {
	g := newGitEnv(t, ProviderGitea)
	body := `{"ref":"refs/heads/feature/GIT-1-login","repository":{"html_url":"https://git.example.com/o/r"},
		"commits":[{"id":"abc1234def","url":"https://git.example.com/o/r/commit/abc1234def","message":"GIT-2 hook it up"}]}`
	r := g.send(ProviderGitea, "push", body)
	if r.Code != 200 || !strings.Contains(r.Raw, `"linked":2`) {
		t.Fatalf("push: %d %s", r.Code, r.Raw)
	}
	if g.count(`SELECT count(*) FROM external_links WHERE task_id = ? AND kind = 'branch' AND url = 'https://git.example.com/o/r/src/branch/feature/GIT-1-login'`, g.t1) != 1 ||
		g.count(`SELECT count(*) FROM external_links WHERE task_id = ? AND kind = 'commit' AND external_id = 'abc1234def'`, g.t2) != 1 {
		t.Fatal("branch and commit links")
	}
	if r = g.send(ProviderGitea, "ping", `{"zen":"x"}`); r.Code != 200 || !strings.Contains(r.Raw, `"ignored":true`) {
		t.Fatalf("ping: %s", r.Raw)
	}
	// Gitea also sends the GitHub-compatible event header.
	h := g.sign(ProviderGitea, []byte(body))
	if r = g.post(ProviderGitea, g.id, []byte(body), h[0], h[1], "X-GitHub-Event", "push"); r.Code != 200 {
		t.Fatalf("fallback event header: %d", r.Code)
	}
}

// Unknown ids, wrong providers, inactive integrations and bad signatures are the same 404 with the same body (no oracle).
func TestIncomingSignatureAndNoOracle(t *testing.T) {
	g := newGitEnv(t, ProviderGitHub)
	body := []byte(prBody("opened", false, "GIT-1", ""))
	good := g.sign(ProviderGitHub, body)
	forged := hmacHex([]byte("not-the-secret"), body)
	otherBody := hmacHex([]byte(g.secret), []byte("different body"))
	cases := map[string]reply{
		"unknown id":         g.post("github", uuid.NewString(), body, good[0], good[1], good[2], "pull_request"),
		"malformed id":       g.post("github", "nope", body, good[0], good[1], good[2], "pull_request"),
		"wrong provider":     g.post("gitea", g.id, body, "X-Gitea-Signature", hmacHex([]byte(g.secret), body), "X-Gitea-Event", "pull_request"),
		"unsupported":        g.post("gitlab", g.id, body, good[0], good[1], good[2], "pull_request"),
		"missing signature":  g.post("github", g.id, body, good[2], "pull_request"),
		"empty signature":    g.post("github", g.id, body, good[0], "", good[2], "pull_request"),
		"forged":             g.post("github", g.id, body, good[0], "sha256="+forged, good[2], "pull_request"),
		"signature of other": g.post("github", g.id, body, good[0], "sha256="+otherBody, good[2], "pull_request"),
		"no prefix":          g.post("github", g.id, body, good[0], hmacHex([]byte(g.secret), body), good[2], "pull_request"),
		"sha1 prefix":        g.post("github", g.id, body, good[0], "sha1="+hmacHex([]byte(g.secret), body), good[2], "pull_request"),
		"tampered body":      g.post("github", g.id, append([]byte(nil), append(body, ' ')...), good[0], good[1], good[2], "pull_request"),
	}
	want := cases["unknown id"].Raw
	for name, r := range cases {
		if r.Code != 404 || r.Raw != want || r.errCode() != "NOT_FOUND" {
			t.Errorf("%s: %d %s (want the identical 404 %s)", name, r.Code, r.Raw, want)
		}
	}
	if g.count(`SELECT count(*) FROM external_links`) != 0 {
		t.Fatal("nothing may be linked by a rejected call")
	}
	// Signed junk is a 400 only after the signature passed.
	junk := []byte("not json")
	h := g.sign(ProviderGitHub, junk)
	if r := g.post("github", g.id, junk, h[0], h[1], h[2], "push"); r.Code != 400 {
		t.Fatalf("signed malformed json: %d %s", r.Code, r.Raw)
	}
	// Inactive integrations answer like unknown ones.
	g.db.Exec(`UPDATE integrations SET active = FALSE WHERE id = ?`, g.id)
	if r := g.post("github", g.id, body, good[0], good[1], good[2], "pull_request"); r.Code != 404 || r.Raw != want {
		t.Fatalf("inactive: %d", r.Code)
	}
	g.db.Exec(`UPDATE integrations SET active = TRUE WHERE id = ?`, g.id)
	// A body over 1 MiB is refused before it is verified.
	big := []byte(strings.Repeat("x", MaxIncomingBody+1))
	if r := g.post("github", g.id, big, good[0], good[1], good[2], "push"); r.Code != 413 {
		t.Fatalf("oversized: %d", r.Code)
	}
	// Rotating the secret retires the old one.
	if _, err := g.svc.RotateIntegrationSecret(bg, g.owner, g.p.ID, uuid.MustParse(g.id)); err != nil {
		t.Fatal(err)
	}
	if r := g.post("github", g.id, body, good[0], good[1], good[2], "pull_request"); r.Code != 404 {
		t.Fatalf("old secret after rotation: %d", r.Code)
	}
	// An integration whose stored secret cannot be opened never authenticates.
	g.db.Exec(`UPDATE integrations SET secret_enc = '\x0102' WHERE id = ?`, g.id)
	if r := g.post("github", g.id, body, good[0], good[1], good[2], "pull_request"); r.Code != 404 {
		t.Fatalf("unreadable secret: %d", r.Code)
	}
}

func TestIncomingIgnoresArchivedAndDeletedProjects(t *testing.T) {
	g := newGitEnv(t, ProviderGitHub)
	if _, err := g.proj.Archive(bg, g.owner, g.p.ID); err != nil {
		t.Fatal(err)
	}
	r := g.send(ProviderGitHub, "pull_request", prBody("opened", false, "GIT-1", ""))
	if r.Code != 200 || !strings.Contains(r.Raw, `"ignored":true`) || g.count(`SELECT count(*) FROM external_links`) != 0 {
		t.Fatalf("archived: %d %s", r.Code, r.Raw)
	}
	if r = g.send(ProviderGitHub, "pull_request", prBody("opened", false, "no key in here", "")); !strings.Contains(r.Raw, `"ignored":true`) {
		t.Fatalf("no key: %s", r.Raw)
	}
}

func TestIncomingRateLimitPerIP(t *testing.T) {
	g := newGitEnv(t, ProviderGitHub)
	g.svc.SetLimiter(newLimiter(3, 60_000_000_000))
	g.httpEnv = newHTTPEnvFrom(g.env)
	var last reply
	for i := 0; i < 5; i++ {
		last = g.post("github", uuid.NewString(), []byte("{}"))
	}
	if last.Code != 429 {
		t.Fatalf("rate limited, got %d", last.Code)
	}
	if !strings.Contains(last.Raw, "RATE_LIMITED") {
		t.Fatal(last.Raw)
	}
}

func TestIntegrationManagement(t *testing.T) {
	e := newEnv(t)
	owner, viewer := e.user(false), e.user(false)
	p := e.project(owner, "MGT")
	if _, err := e.proj.AddMember(bg, owner, p.ID, project.MemberInput{UserID: viewer.UserID, Role: api.RoleViewer}); err != nil {
		t.Fatal(err)
	}
	_, err := e.svc.CreateIntegration(bg, viewer, p.ID, IntegrationInput{Provider: "github", Name: "x"})
	wantCode(t, err, "FORBIDDEN")
	_, err = e.svc.CreateIntegration(bg, owner, p.ID, IntegrationInput{Provider: "github", Name: ""})
	wantCode(t, err, "VALIDATION_ERROR")
	for i := 0; i < maxIntegrationsPerProject; i++ {
		if _, err := e.svc.CreateIntegration(bg, owner, p.ID, IntegrationInput{Provider: " GitHub ", Name: fmt.Sprint("r", i)}); err != nil {
			t.Fatal(err)
		}
	}
	_, err = e.svc.CreateIntegration(bg, owner, p.ID, IntegrationInput{Provider: "github", Name: "more"})
	wantCode(t, err, "INTEGRATION_LIMIT")
	list, err := e.svc.Integrations(bg, owner, p.ID)
	if err != nil || len(list) != maxIntegrationsPerProject || list[0].Provider != ProviderGitHub {
		t.Fatalf("list: %d %v", len(list), err)
	}
	_, err = e.svc.Integrations(bg, viewer, p.ID)
	wantCode(t, err, "FORBIDDEN")
	_, err = e.svc.RotateIntegrationSecret(bg, owner, p.ID, uuid.New())
	wantCode(t, err, "NOT_FOUND")
	wantCode(t, e.svc.DeleteIntegration(bg, owner, p.ID, uuid.New()), "NOT_FOUND")
	if err := e.svc.DeleteIntegration(bg, owner, p.ID, list[0].ID); err != nil {
		t.Fatal(err)
	}
	for _, a := range []string{"integration.created", "integration.deleted"} {
		if !e.audit.has(a) {
			t.Errorf("missing audit %s", a)
		}
	}
	if strings.Contains(toJSON(e.audit.entries), "whsec_") {
		t.Fatal("audit never holds secrets")
	}
	foreign := api.Actor{TenantID: uuid.New(), UserID: uuid.New(), ManageAll: true}
	_, err = e.svc.TaskLinks(bg, foreign, uuid.New())
	wantCode(t, err, "NOT_FOUND")
}

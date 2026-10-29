package publiclink

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/project"
	"github.com/jaas/jaas/internal/projects/testdb"
	"github.com/jaas/jaas/internal/shared/identityport"
	"gorm.io/gorm"
)

var bg = context.Background()

type fakeUsers struct{ active map[uuid.UUID]bool }

func (f fakeUsers) GetByID(_ context.Context, _ *gorm.DB, tenantID, id uuid.UUID) (*identityport.User, error) {
	if !f.active[id] {
		return nil, nil
	}
	return &identityport.User{ID: id, TenantID: tenantID, Status: "active"}, nil
}
func (fakeUsers) DeactivateUser(context.Context, *gorm.DB, uuid.UUID, uuid.UUID, uuid.UUID) error {
	return nil
}
func (fakeUsers) ActivateUser(context.Context, *gorm.DB, uuid.UUID, uuid.UUID) error { return nil }
func (fakeUsers) InviteUser(context.Context, *gorm.DB, uuid.UUID, identityport.Invite, uuid.UUID) (*identityport.User, error) {
	return nil, nil
}

type fakeAudit struct{ actions []string }

func (f *fakeAudit) Log(_ context.Context, _ *gorm.DB, _, _ string, action, _, _ string, _ interface{}, _, _ string) error {
	f.actions = append(f.actions, action)
	return nil
}

type env struct {
	t      *testing.T
	db     *gorm.DB
	proj   *project.Service
	svc    *Service
	audit  *fakeAudit
	users  fakeUsers
	tenant uuid.UUID
	n      int
}

func newEnv(t *testing.T) *env {
	t.Helper()
	db := testdb.Open(t)
	e := &env{t: t, db: db, audit: &fakeAudit{}, users: fakeUsers{active: map[uuid.UUID]bool{}}, tenant: uuid.New()}
	e.proj = project.NewService(db, project.Ports{Users: e.users, Audit: e.audit})
	e.svc = NewService(db, e.proj, Ports{Audit: e.audit})
	return e
}

func (e *env) user() api.Actor {
	id := uuid.New()
	e.users.active[id] = true
	return api.Actor{TenantID: e.tenant, UserID: id, CreateProject: true}
}

type fixture struct {
	p                    *project.Project
	owner, member, viewr api.Actor
}

func (e *env) fixture(key string) fixture {
	e.t.Helper()
	owner := e.user()
	p, err := e.proj.Create(bg, owner, project.CreateInput{Key: key, Name: "Public " + key})
	if err != nil {
		e.t.Fatal(err)
	}
	f := fixture{p: p, owner: owner, member: e.user(), viewr: e.user()}
	for a, role := range map[*api.Actor]string{&f.member: api.RoleMember, &f.viewr: api.RoleViewer} {
		if _, err := e.proj.AddMember(bg, owner, p.ID, project.MemberInput{UserID: a.UserID, Role: role}); err != nil {
			e.t.Fatal(err)
		}
	}
	return f
}

// task inserts a task with every personal side table filled: assignee, label, comment, watcher.
func (e *env) task(f fixture, title, desc, col string) uuid.UUID {
	e.t.Helper()
	e.n++
	id := uuid.New()
	err := e.db.Exec(`INSERT INTO tasks (id, tenant_id, project_id, number, title, description, column_id, priority, reporter_user_id, rank, due_at)
		VALUES (?, ?, ?, ?, ?, ?, (SELECT id FROM board_columns WHERE project_id = ? AND key = ?), 'high', ?, ?, now() + interval '2 days')`,
		id, e.tenant, f.p.ID, e.n, title, desc, f.p.ID, col, f.member.UserID, "V"+strconv.Itoa(e.n)).Error
	if err != nil {
		e.t.Fatal(err)
	}
	e.db.Exec(`INSERT INTO task_assignees (task_id, user_id, tenant_id, assigned_by) VALUES (?, ?, ?, ?)`, id, f.member.UserID, e.tenant, f.owner.UserID)
	e.db.Exec(`INSERT INTO comments (id, tenant_id, task_id, author_user_id, body) VALUES (?, ?, ?, ?, 'secret comment')`, uuid.New(), e.tenant, id, f.member.UserID)
	return id
}

func (e *env) label(f fixture, id uuid.UUID, name string) {
	lid := uuid.New()
	if err := e.db.Exec(`INSERT INTO labels (id, tenant_id, project_id, name) VALUES (?, ?, ?, ?)`, lid, e.tenant, f.p.ID, name).Error; err != nil {
		e.t.Fatal(err)
	}
	e.db.Exec(`INSERT INTO task_labels (task_id, label_id, tenant_id) VALUES (?, ?, ?)`, id, lid, e.tenant)
}

func (e *env) link(f fixture, desc bool) *Link {
	e.t.Helper()
	l, err := e.svc.Create(bg, f.owner, f.p.ID, Input{ShowDescriptions: desc})
	if err != nil {
		e.t.Fatal(err)
	}
	return l
}

func wantCode(t *testing.T, err error, code string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), code) {
		t.Fatalf("want error %s, got %v", code, err)
	}
}

// get calls the public route (no actor) and returns status, headers and raw body.
func get(e *env, token, query string) (int, http.Header, string) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	e.svc.RegisterPublic(r.Group("/api/v1"))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/public/projects/"+token+query, nil))
	return w.Code, w.Header(), w.Body.String()
}

func TestLinkLifecycleAndPermissions(t *testing.T) {
	e := newEnv(t)
	f := e.fixture("LL")
	stranger := e.user()
	_, err := e.svc.Create(bg, f.member, f.p.ID, Input{})
	wantCode(t, err, "FORBIDDEN")
	_, err = e.svc.Create(bg, stranger, f.p.ID, Input{})
	wantCode(t, err, "NOT_FOUND")
	l := e.link(f, false)
	if len(l.Token) != tokenLen || l.Path == "" || l.TokenHash == l.Token {
		t.Fatalf("the raw token is shown once: %+v", l)
	}
	var stored string
	e.db.Raw(`SELECT token_hash FROM public_links WHERE id = ?`, l.ID).Scan(&stored)
	if stored != hashToken(l.Token) || strings.Contains(stored, l.Token) {
		t.Fatal("only the hash is stored")
	}
	rows, err := e.svc.List(bg, f.owner, f.p.ID)
	if err != nil || len(rows) != 1 || rows[0].Token != "" {
		t.Fatalf("list never shows the secret: %v %+v", err, rows)
	}
	raw, _ := json.Marshal(rows)
	if strings.Contains(string(raw), l.Token) || strings.Contains(string(raw), stored) {
		t.Fatal("secret in list JSON")
	}
	if _, err = e.svc.List(bg, f.member, f.p.ID); err == nil {
		t.Fatal("members cannot list links")
	}
	e.link(f, true)
	e.link(f, false)
	_, err = e.svc.Create(bg, f.owner, f.p.ID, Input{})
	wantCode(t, err, "LINK_LIMIT")
	if err = e.svc.Revoke(bg, f.member, f.p.ID, l.ID); err == nil {
		t.Fatal("members cannot revoke")
	}
	if err = e.svc.Revoke(bg, f.owner, f.p.ID, l.ID); err != nil {
		t.Fatal(err)
	}
	wantCode(t, e.svc.Revoke(bg, f.owner, f.p.ID, l.ID), "NOT_FOUND")
	wantCode(t, e.svc.Revoke(bg, f.owner, f.p.ID, uuid.New()), "NOT_FOUND")
	if _, err = e.svc.Create(bg, f.owner, f.p.ID, Input{}); err != nil {
		t.Fatalf("a revoked link frees a slot: %v", err)
	}
	for _, a := range []string{"public_link.created", "public_link.revoked"} {
		found := false
		for _, got := range e.audit.actions {
			found = found || got == a
		}
		if !found {
			t.Errorf("audit %s missing", a)
		}
	}
}

// collectStrings gathers every string value and every key of a decoded JSON document.
func collectStrings(v any, out *[]string) {
	switch x := v.(type) {
	case map[string]any:
		for k, val := range x {
			*out = append(*out, k)
			collectStrings(val, out)
		}
	case []any:
		for _, val := range x {
			collectStrings(val, out)
		}
	case string:
		*out = append(*out, x)
	}
}

func TestPublicViewIsAStrictAllowList(t *testing.T) {
	e := newEnv(t)
	f := e.fixture("AL")
	mention := "@[Alice Smith](user:" + f.member.UserID.String() + ")"
	t1 := e.task(f, "First", "Please ask "+mention+" about it", "todo")
	e.label(f, t1, "bug")
	e.label(f, t1, "ui")
	sub := e.task(f, "Sub", "", "done")
	e.db.Exec(`UPDATE tasks SET parent_task_id = ? WHERE id = ?`, t1, sub)
	e.task(f, "Done one", "", "done")
	deleted := e.task(f, "Deleted", "", "todo")
	e.db.Exec(`UPDATE tasks SET deleted_at = now() WHERE id = ?`, deleted)
	archived := e.task(f, "Archived", "", "todo")
	e.db.Exec(`UPDATE tasks SET archived_at = now() WHERE id = ?`, archived)

	without := e.link(f, false)
	with := e.link(f, true)
	code, h, body := get(e, without.Token, "")
	if code != http.StatusOK {
		t.Fatalf("status %d %s", code, body)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(body), &doc); err != nil {
		t.Fatal(err)
	}
	var seen []string
	collectStrings(doc, &seen)
	for _, banned := range []string{f.owner.UserID.String(), f.member.UserID.String(), f.viewr.UserID.String(), e.tenant.String(), f.p.ID.String(),
		"secret comment", "Alice", "reporter", "assignee", "comment", "attachment", "member", "time", "activity", "user_id", "tenant"} {
		for _, s := range seen {
			if strings.Contains(strings.ToLower(s), strings.ToLower(banned)) {
				t.Errorf("the public JSON mentions %q in %q", banned, s)
			}
		}
	}
	data := doc["data"].(map[string]any)
	if len(data) != 3 || data["project"].(map[string]any)["key"] != "AL" || len(data["columns"].([]any)) != 4 {
		t.Fatalf("shape: %s", body)
	}
	tasks := data["tasks"].([]any)
	if len(tasks) != 3 {
		t.Fatalf("deleted and archived tasks are hidden, got %d", len(tasks))
	}
	first := tasks[0].(map[string]any)
	if first["key"] != "AL-1" || first["progress"].(float64) != 100 || len(first["labels"].([]any)) != 2 || first["labels"].([]any)[0] != "bug" {
		t.Fatalf("first task: %v", first)
	}
	if _, has := first["description"]; has {
		t.Fatal("descriptions are off by default")
	}
	for k := range first {
		switch k {
		case "key", "title", "column", "priority", "due_at", "labels", "progress":
		default:
			t.Errorf("task member %q is not in the allow-list", k)
		}
	}
	_, _, body = get(e, with.Token, "")
	if !strings.Contains(body, "Please ask @someone about it") || strings.Contains(body, f.member.UserID.String()) || strings.Contains(body, "Alice") {
		t.Fatalf("descriptions hide mentions: %s", body)
	}
	if h.Get("Cache-Control") != "no-store" || h.Get("X-Robots-Tag") != "noindex" || h.Get("Referrer-Policy") != "no-referrer" {
		t.Fatalf("headers: %v", h)
	}
}

func TestPublicViewPaging(t *testing.T) {
	e := newEnv(t)
	f := e.fixture("PG")
	for i := 0; i < 5; i++ {
		e.task(f, "T"+strconv.Itoa(i), "", "todo")
	}
	l := e.link(f, false)
	_, _, body := get(e, l.Token, "?limit=2")
	var doc struct {
		Data struct{ Tasks []map[string]any }
		Meta struct {
			Limit int
			Next  *string `json:"next_cursor"`
		}
	}
	_ = json.Unmarshal([]byte(body), &doc)
	if len(doc.Data.Tasks) != 2 || doc.Meta.Limit != 2 || doc.Meta.Next == nil {
		t.Fatalf("first page: %s", body)
	}
	_, _, body = get(e, l.Token, "?limit=100&cursor="+*doc.Meta.Next)
	var rest struct {
		Data struct{ Tasks []map[string]any }
		Meta struct {
			Next *string `json:"next_cursor"`
		}
	}
	_ = json.Unmarshal([]byte(body), &rest)
	if len(rest.Data.Tasks) != 3 || rest.Meta.Next != nil || rest.Data.Tasks[0]["key"] != "PG-3" {
		t.Fatalf("rest: %s", body)
	}
	if code, _, _ := get(e, l.Token, "?cursor=%25%25"); code != http.StatusBadRequest {
		t.Fatalf("bad cursor: %d", code)
	}
}

func TestUnknownRevokedMalformedAndHiddenProjectsAnswerIdentically(t *testing.T) {
	e := newEnv(t)
	f := e.fixture("NF")
	live := e.link(f, false)
	revoked := e.link(f, false)
	if err := e.svc.Revoke(bg, f.owner, f.p.ID, revoked.ID); err != nil {
		t.Fatal(err)
	}
	other := e.fixture("NG")
	hidden := e.link(other, false)
	if _, err := e.proj.Archive(bg, other.owner, other.p.ID); err != nil {
		t.Fatal(err)
	}
	gone := e.fixture("NH")
	deleted := e.link(gone, false)
	admin := gone.owner
	admin.ManageAll = true
	if _, err := e.proj.Archive(bg, admin, gone.p.ID); err != nil {
		t.Fatal(err)
	}
	if err := e.proj.Delete(bg, admin, gone.p.ID); err != nil {
		t.Fatal(err)
	}
	unknown, _ := newRawToken()
	var first string
	for name, tok := range map[string]string{"unknown": unknown, "revoked": revoked.Token, "archived": hidden.Token, "deleted": deleted.Token,
		"short": "abc", "long": strings.Repeat("a", 80), "chars": strings.Repeat("!", 43), "tampered": live.Token[:42] + "_"} {
		code, h, body := get(e, tok, "")
		if code != http.StatusNotFound {
			t.Errorf("%s: status %d", name, code)
		}
		if first == "" {
			first = body
		} else if body != first {
			t.Errorf("%s: body differs: %s vs %s", name, body, first)
		}
		if h.Get("Cache-Control") != "no-store" || h.Get("Referrer-Policy") != "no-referrer" {
			t.Errorf("%s: 404 headers: %v", name, h)
		}
	}
	if code, _, _ := get(e, live.Token, ""); code != http.StatusOK {
		t.Fatalf("the live link still works: %d", code)
	}
	if err := e.svc.Revoke(bg, f.owner, f.p.ID, live.ID); err != nil {
		t.Fatal(err)
	}
	if code, _, _ := get(e, live.Token, ""); code != http.StatusNotFound {
		t.Fatal("revocation is immediate")
	}
}

func TestOnlyTheTokenProjectIsReadable(t *testing.T) {
	e := newEnv(t)
	a, b := e.fixture("TA"), e.fixture("TB")
	e.task(a, "Visible", "", "todo")
	e.task(b, "Other project", "", "todo")
	_, _, body := get(e, e.link(a, false).Token, "")
	if strings.Contains(body, "Other project") || !strings.Contains(body, "Visible") {
		t.Fatalf("a link shows its own project only: %s", body)
	}
}

func TestPublicRouteIsRateLimited(t *testing.T) {
	e := newEnv(t)
	e.svc.SetLimiter(newLimiter(3, time.Minute))
	gin.SetMode(gin.TestMode)
	r := gin.New()
	e.svc.RegisterPublic(r.Group("/api/v1"))
	last := 0
	for i := 0; i < 5; i++ {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/public/projects/nope", nil))
		last = w.Code
		if i == 4 && w.Header().Get("Retry-After") == "" {
			t.Fatal("Retry-After on 429")
		}
	}
	if last != http.StatusTooManyRequests {
		t.Fatalf("limited after the budget: %d", last)
	}
}

func TestManagementRoutes(t *testing.T) {
	e := newEnv(t)
	f := e.fixture("MR")
	do := func(a api.Actor, method, path string, body any) (int, map[string]any) {
		gin.SetMode(gin.TestMode)
		r := gin.New()
		g := r.Group("", func(c *gin.Context) { c.Set("tenant_id", a.TenantID); c.Set("user_id", a.UserID) })
		e.svc.Register(g)
		var rd bytes.Buffer
		_ = json.NewEncoder(&rd).Encode(body)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(method, path, &rd))
		var out map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		return w.Code, out
	}
	base := "/projects/" + f.p.ID.String() + "/public-links"
	code, out := do(f.owner, "POST", base, map[string]any{"show_descriptions": true})
	if code != http.StatusCreated || out["data"].(map[string]any)["token"] == nil {
		t.Fatalf("create: %d %v", code, out)
	}
	id := out["data"].(map[string]any)["id"].(string)
	if code, _ = do(f.owner, "POST", base, "{"); code != http.StatusBadRequest {
		t.Fatalf("body: %d", code)
	}
	if code, out = do(f.owner, "GET", base, nil); code != http.StatusOK || out["data"].([]any)[0].(map[string]any)["token"] != nil {
		t.Fatalf("list: %d %v", code, out)
	}
	if code, _ = do(f.owner, "DELETE", base+"/bad", nil); code != http.StatusBadRequest {
		t.Fatalf("bad id: %d", code)
	}
	if code, _ = do(f.owner, "DELETE", base+"/"+id, nil); code != http.StatusOK {
		t.Fatalf("revoke: %d", code)
	}
	if code, _ = do(f.owner, "DELETE", base+"/"+id, nil); code != http.StatusNotFound {
		t.Fatalf("revoke twice: %d", code)
	}
	if code, _ = do(f.owner, "GET", "/projects/bad/public-links", nil); code != http.StatusBadRequest {
		t.Fatalf("project id: %d", code)
	}
	unauth := gin.New()
	e.svc.Register(unauth.Group(""))
	w := httptest.NewRecorder()
	unauth.ServeHTTP(w, httptest.NewRequest("GET", base, nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("no actor: %d", w.Code)
	}
}

func TestDescriptionIsCapped(t *testing.T) {
	long := strings.Repeat("é", MaxDescription+50)
	if got := cleanDescription(long); len([]rune(got)) != MaxDescription {
		t.Fatalf("cap: %d", len([]rune(got)))
	}
	if itoa(0) != "0" || itoa(1205) != "1205" {
		t.Fatal("itoa")
	}
	if progress(taskRow{Total: 4, Done: 1}) != 25 || progress(taskRow{Category: "todo"}) != 0 {
		t.Fatal("progress")
	}
}

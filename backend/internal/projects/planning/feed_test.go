package planning

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
)

func TestIcsEscapeAndFold(t *testing.T) {
	cases := map[string]string{
		"plain":                  "plain",
		`semi;comma,back\slash`:  `semi\;comma\,back\\slash`,
		"line1\nline2":           `line1\nline2`,
		"crlf\r\ninjected":       `crlf\ninjected`,
		"bare\rcr":               "barecr",
		"tab\there":              "tab here",
		"nul\x00bell\x07del\x7f": "nulbelldel",
		"sep para ":              "seppara",
		"unicode é中 \U0001F600":  "unicode é中 \U0001F600",
	}
	for in, want := range cases {
		if got := icsEscape(in); got != want {
			t.Errorf("icsEscape(%q) = %q, want %q", in, got, want)
		}
	}
	// A title cannot start a new property line.
	body := buildICS([]Card{{ID: uuid.New(), Ticket: "K-1", Title: "x\r\nBEGIN:VEVENT\r\nATTENDEE:evil", UpdatedAt: time.Now()}}, time.Now())
	events := 0
	for _, line := range strings.Split(body, "\r\n") {
		if line == "BEGIN:VEVENT" {
			events++
		}
		if strings.HasPrefix(line, "ATTENDEE") {
			t.Fatalf("injected property line: %q", body)
		}
	}
	if events != 1 {
		t.Fatalf("injection created %d events: %q", events, body)
	}
	long := icsFold("SUMMARY:" + strings.Repeat("é", 100))
	for _, line := range strings.Split(long, "\r\n") {
		if len(line) > 75 || !utf8.ValidString(line) {
			t.Fatalf("folded line %d octets (valid utf8: %v)", len(line), utf8.ValidString(line))
		}
	}
	if strings.ReplaceAll(long, "\r\n ", "") != "SUMMARY:"+strings.Repeat("é", 100) {
		t.Fatal("unfolding must restore the line")
	}
	if icsFold("short") != "short" {
		t.Fatal("short lines stay")
	}
}

// VW-03: a token is shown once, only its hash is stored, and revoking it ends the feed at once.
func TestCalendarTokenLifecycle(t *testing.T) {
	e := newEnv(t)
	me, other := e.user(), e.user()
	tok, err := e.svc.CreateToken(bg, me, "  phone ")
	if err != nil || len(tok.Token) != 43 || !strings.HasSuffix(tok.FeedPath, tok.Token) || tok.Label != "phone" {
		t.Fatalf("token: %+v %v", tok, err)
	}
	var stored []string
	e.db.Raw(`SELECT token_hash FROM calendar_tokens WHERE id = ?`, tok.ID).Scan(&stored)
	if len(stored) != 1 || stored[0] == tok.Token || stored[0] != hashToken(tok.Token) || len(stored[0]) != 64 {
		t.Fatalf("only the SHA-256 hash is stored: %v", stored)
	}
	list, err := e.svc.ListTokens(bg, me)
	if err != nil || len(list) != 1 || list[0].Token != "" || list[0].FeedPath != "" {
		t.Fatalf("list must never carry the secret: %+v %v", list, err)
	}
	if l, _ := e.svc.ListTokens(bg, other); len(l) != 0 {
		t.Fatal("tokens are per user")
	}
	if _, err = e.svc.CreateToken(bg, me, strings.Repeat("x", 61)); err == nil {
		t.Fatal("label length")
	}
	wantCode(t, e.svc.RevokeToken(bg, other, tok.ID), "NOT_FOUND")
	if _, err = e.svc.FeedFor(bg, tok.Token); err != nil {
		t.Fatalf("feed works before revocation: %v", err)
	}
	if err = e.svc.RevokeToken(bg, me, tok.ID); err != nil {
		t.Fatal(err)
	}
	_, err = e.svc.FeedFor(bg, tok.Token)
	wantCode(t, err, "NOT_FOUND")
	wantCode(t, e.svc.RevokeToken(bg, me, tok.ID), "NOT_FOUND")
	if l, _ := e.svc.ListTokens(bg, me); len(l) != 0 {
		t.Fatal("revoked tokens leave the list")
	}
	for i := 0; i < maxActiveTokens; i++ {
		if _, err = e.svc.CreateToken(bg, me, ""); err != nil {
			t.Fatal(err)
		}
	}
	_, err = e.svc.CreateToken(bg, me, "")
	wantCode(t, err, "TOKEN_LIMIT")
}

func (e *env) feedSetup() (api.Actor, *CalendarToken, map[string]uuid.UUID) {
	owner, me := e.user("create"), e.user()
	p, lost := e.scrum(owner, "FEED"), e.scrum(owner, "LOST")
	e.join(owner, p, me, "member")
	e.join(owner, lost, me, "member")
	soon := time.Now().UTC().AddDate(0, 0, 3).Truncate(time.Second)
	dropped := e.canceledCol(owner, p)
	t := map[string]uuid.UUID{
		"due":      e.task(p, tk{title: "Plan; review, ship\\now\nnext", col: e.col(p, "todo"), due: &soon, assignee: &me.UserID}),
		"span":     e.task(p, tk{title: "Span", col: e.col(p, "todo"), start: soon.Format("2006-01-02"), due: &soon, assignee: &me.UserID}),
		"start":    e.task(p, tk{title: "Starts", col: e.col(p, "todo"), start: soon.Format("2006-01-02"), assignee: &me.UserID}),
		"canceled": e.task(p, tk{title: "Dropped", col: dropped, due: &soon, assignee: &me.UserID}),
		"others":   e.task(p, tk{title: "Not mine", col: e.col(p, "todo"), due: &soon, assignee: &owner.UserID}),
		"far":      e.task(p, tk{title: "Far away", col: e.col(p, "todo"), due: at("2031-01-01T00:00:00Z"), assignee: &me.UserID}),
		"lost":     e.task(lost, tk{title: "Lost access", col: e.col(lost, "todo"), due: &soon, assignee: &me.UserID}),
	}
	e.db.Exec(`DELETE FROM project_members WHERE project_id = ? AND user_id = ?`, lost.ID, me.UserID)
	tok, _ := e.svc.CreateToken(bg, me, "cal")
	return me, tok, t
}

func TestFeedListsOnlyMyReadableAssignedTasks(t *testing.T) {
	e := newEnv(t)
	me, tok, tasks := e.feedSetup()
	// The same user id in another tenant must never leak in, even with a task assigned to it.
	other := newEnvOn(e)
	op := other.scrum(other.user("create"), "FEED")
	other.task(op, tk{title: "Foreign tenant", col: other.col(op, "todo"), due: at("2030-01-01T00:00:00Z"), assignee: &me.UserID})

	body, err := e.svc.FeedFor(bg, tok.Token)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(body, "BEGIN:VCALENDAR\r\n") || !strings.HasSuffix(body, "END:VCALENDAR\r\n") || strings.Contains(strings.ReplaceAll(body, "\r\n", ""), "\n") {
		t.Fatalf("calendar framing / CRLF: %q", body)
	}
	if strings.Count(body, "BEGIN:VEVENT") != 4 {
		t.Fatalf("want 4 events (mine, readable, in the window):\n%s", body)
	}
	for _, id := range []uuid.UUID{tasks["due"], tasks["span"], tasks["start"], tasks["canceled"]} {
		if !strings.Contains(body, "UID:"+id.String()+"@jaas") {
			t.Errorf("missing event for %s", id)
		}
	}
	for _, name := range []string{"others", "far", "lost"} {
		if strings.Contains(body, tasks[name].String()) {
			t.Errorf("%s must not be in the feed", name)
		}
	}
	if strings.Contains(body, "Foreign tenant") || strings.Contains(body, "Not mine") || strings.Contains(body, "Lost access") {
		t.Fatal("foreign titles leaked")
	}
	for _, want := range []string{`SUMMARY:[FEED-1] Plan\; review\, ship\\now\nnext`, "STATUS:CANCELLED", "DTSTART;VALUE=DATE:", "DTEND;VALUE=DATE:"} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
	var used *time.Time
	e.db.Raw(`SELECT last_used_at FROM calendar_tokens WHERE id = ?`, tok.ID).Row().Scan(&used)
	if used == nil {
		t.Fatal("last_used_at must be recorded")
	}
}

func TestFeedRefusesBadTokensAndInactiveOwners(t *testing.T) {
	e := newEnv(t)
	me, tok, _ := e.feedSetup()
	for _, bad := range []string{"", "nope", strings.Repeat("a", 65), tok.Token + "x", strings.ToUpper(tok.Token)} {
		_, err := e.svc.FeedFor(bg, bad)
		wantCode(t, err, "NOT_FOUND")
	}
	e.users.mu.Lock()
	e.users.active[me.UserID] = false
	e.users.mu.Unlock()
	_, err := e.svc.FeedFor(bg, tok.Token)
	wantCode(t, err, "NOT_FOUND")

	e.svc.p.Users = nil // without the port the feed cannot re-check the owner and relies on the token alone
	if _, err = e.svc.FeedFor(bg, tok.Token); err != nil {
		t.Fatalf("no users port: %v", err)
	}
}

func TestFeedRouteIsPublicAndRateLimited(t *testing.T) {
	e := newEnv(t)
	_, tok, _ := e.feedSetup()
	e.svc.SetFeedLimiter(newFeedLimiter(3, time.Minute))
	gin.SetMode(gin.TestMode)
	r := gin.New()
	e.svc.RegisterPublic(r.Group("/api/v1"))
	get := func(token string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/calendar/feed.ics?token="+token, nil)
		req.RemoteAddr = "203.0.113.9:4444"
		r.ServeHTTP(w, req)
		return w
	}
	ok := get(tok.Token)
	if ok.Code != 200 || !strings.HasPrefix(ok.Header().Get("Content-Type"), "text/calendar") || !strings.Contains(ok.Header().Get("Cache-Control"), "no-store") {
		t.Fatalf("feed: %d %v", ok.Code, ok.Header())
	}
	if bad := get("wrong"); bad.Code != 404 || !strings.Contains(bad.Body.String(), "NOT_FOUND") {
		t.Fatalf("unknown token: %d %s", bad.Code, bad.Body)
	}
	get("wrong")
	limited := get(tok.Token)
	if limited.Code != 429 || limited.Header().Get("Retry-After") == "" {
		t.Fatalf("rate limit: %d %v", limited.Code, limited.Header())
	}
}

func TestFeedLimiterWindowsExpire(t *testing.T) {
	now := time.Now()
	l := &feedLimiter{limit: 2, window: time.Minute, hits: map[string]*feedWindow{}, now: func() time.Time { return now }}
	for i := 0; i < 2; i++ {
		if ok, _ := l.allow("ip"); !ok {
			t.Fatal("within the limit")
		}
	}
	if ok, retry := l.allow("ip"); ok || retry < 1 {
		t.Fatal("over the limit")
	}
	now = now.Add(2 * time.Minute)
	if ok, _ := l.allow("ip"); !ok {
		t.Fatal("window reset")
	}
	for i := 0; i <= feedLimiterMaxKeys; i++ {
		l.hits[uuid.NewString()] = &feedWindow{resetAt: now.Add(-time.Hour)}
	}
	l.allow("ip")
	if len(l.hits) > 2 {
		t.Fatalf("expired windows must be evicted, %d left", len(l.hits))
	}
}

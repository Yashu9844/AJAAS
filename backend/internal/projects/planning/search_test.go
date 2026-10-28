package planning

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/project"
)

func hitIDs(h []SearchHit) []uuid.UUID {
	out := make([]uuid.UUID, len(h))
	for i, x := range h {
		out[i] = x.ID
	}
	return out
}

// TK-14: full text over title and description, ranked, plus ticket-id lookup.
func TestSearchTextRankingAndTicket(t *testing.T) {
	e := newEnv(t)
	owner := e.user("create")
	p := e.scrum(owner, "SRC")
	todo := e.col(p, "todo")
	inDesc := e.task(p, tk{title: "Plain chore", desc: "needs a quokka", col: todo})
	inTitle := e.task(p, tk{title: "Quokka habitat", col: todo})
	e.task(p, tk{title: "Unrelated", col: todo})
	e.task(p, tk{title: "Quokka archived", col: todo, archived: true})
	e.task(p, tk{title: "Quokka deleted", col: todo, deleted: true})

	hits, err := e.svc.Search(bg, owner, SearchParams{Query: "quokka", Limit: 10})
	if err != nil || len(hits) != 2 || hits[0].ID != inTitle || hits[1].ID != inDesc || hits[0].Score <= hits[1].Score || hits[0].Match != "text" {
		t.Fatalf("ranking (title outranks description): %+v %v", hits, err)
	}
	// websearch syntax: quoted phrase and negation.
	if h, _ := e.svc.Search(bg, owner, SearchParams{Query: "quokka -habitat", Limit: 10}); len(h) != 1 || h[0].ID != inDesc {
		t.Fatalf("negation: %+v", h)
	}
	// Ticket id lookup, case-insensitive, listed first and not repeated by the text match.
	hits, _ = e.svc.Search(bg, owner, SearchParams{Query: "src-2", Limit: 10})
	if len(hits) != 1 || hits[0].ID != inTitle || hits[0].Match != "ticket" || hits[0].Ticket != "SRC-2" {
		t.Fatalf("ticket lookup: %+v", hits)
	}
	if h, _ := e.svc.Search(bg, owner, SearchParams{Query: "SRC-99", Limit: 10}); len(h) != 0 {
		t.Fatalf("unknown ticket: %+v", h)
	}
	if h, _ := e.svc.Search(bg, owner, SearchParams{Query: "quokka", Limit: 1}); len(h) != 1 {
		t.Fatalf("limit: %+v", h)
	}
	for _, bad := range []string{"", "   ", strings.Repeat("x", 201)} {
		_, err = e.svc.Search(bg, owner, SearchParams{Query: bad, Limit: 10})
		wantCode(t, err, "VALIDATION_ERROR")
	}
	if ClampSearchLimit("") != 20 || ClampSearchLimit("500") != 50 || ClampSearchLimit("5") != 5 {
		t.Fatal("search limit clamp")
	}
}

// PG9: search never crosses tenants and never shows private projects the caller cannot read.
func TestPG9SearchIsTenantIsolatedAndPrivacyAware(t *testing.T) {
	e := newEnv(t)
	owner, member, outsider := e.user("create"), e.user(), e.user()
	open := e.scrum(owner, "OPEN")
	private := e.scrum(owner, "PRIV")
	e.db.Exec(`UPDATE projects SET visibility = 'tenant' WHERE id = ?`, open.ID)
	e.join(owner, private, member, "viewer")
	openTask := e.task(open, tk{title: "Shared zebra plan", col: e.col(open, "todo")})
	privTask := e.task(private, tk{title: "Private zebra plan", col: e.col(private, "todo")})

	// A second tenant with the same words and the same project key.
	other := newEnvOn(e)
	foreignOwner := other.user("create")
	foreign := other.scrum(foreignOwner, "OPEN")
	other.task(foreign, tk{title: "Foreign zebra plan", col: other.col(foreign, "todo")})

	got := func(a api.Actor, q string) map[uuid.UUID]bool {
		hits, err := e.svc.Search(bg, a, SearchParams{Query: q, Limit: 50})
		if err != nil {
			t.Fatal(err)
		}
		m := map[uuid.UUID]bool{}
		for _, id := range hitIDs(hits) {
			m[id] = true
		}
		return m
	}
	if r := got(member, "zebra"); !r[openTask] || !r[privTask] || len(r) != 2 {
		t.Fatalf("member sees both of the tenant projects it can read: %v", r)
	}
	if r := got(outsider, "zebra"); !r[openTask] || r[privTask] || len(r) != 1 {
		t.Fatalf("outsider sees the tenant-visible project only: %v", r)
	}
	if r := got(foreignOwner, "zebra"); len(r) != 1 || r[openTask] || r[privTask] {
		t.Fatalf("the other tenant sees only its own task: %v", r)
	}
	if r := got(outsider, "OPEN-1"); !r[openTask] || len(r) != 1 {
		t.Fatalf("ticket lookup stays in the tenant: %v", r)
	}
	if r := got(outsider, "PRIV-1"); len(r) != 0 {
		t.Fatalf("a private ticket id resolves to nothing: %v", r)
	}

	// project_id narrows; an unreadable project is a 404, never an empty hit list.
	if _, err := e.svc.Search(bg, outsider, SearchParams{Query: "zebra", ProjectID: &private.ID, Limit: 10}); err == nil {
		t.Fatal("unreadable project must be 404")
	}
	hits, err := e.svc.Search(bg, member, SearchParams{Query: "zebra", ProjectID: &private.ID, Limit: 10})
	if err != nil || len(hits) != 1 || hits[0].ID != privTask {
		t.Fatalf("project filter: %+v %v", hits, err)
	}
	if h, err := e.svc.Search(bg, e.user(), SearchParams{Query: "nothing-readable-here", Limit: 5}); err != nil || h == nil {
		t.Fatalf("a caller without projects gets an empty list: %v %v", h, err)
	}
}

// A custom role with project.read but not task.read must not search the tasks.
func TestSearchHonoursTaskReadOfCustomRoles(t *testing.T) {
	e := newEnv(t)
	owner, admin, guest := e.user("create"), e.user("admin"), e.user()
	p := e.scrum(owner, "CUS")
	e.task(p, tk{title: "Walrus roadmap", col: e.col(p, "todo")})
	role, err := e.proj.CreateRole(bg, admin, project.RoleInput{Name: "Peek", Permissions: []string{"project.read", "task.read"}})
	if err != nil {
		t.Fatal(err)
	}
	// The API refuses such a role; the database could still hold one (older data, manual change), so the read models cope.
	e.db.Exec(`UPDATE project_roles SET permissions = '["project.read"]' WHERE id = ?`, role.ID)
	if _, err = e.proj.AddMember(bg, owner, p.ID, project.MemberInput{UserID: guest.UserID, CustomRoleID: &role.ID}); err != nil {
		t.Fatal(err)
	}
	if h, _ := e.svc.Search(bg, guest, SearchParams{Query: "walrus", Limit: 10}); len(h) != 0 {
		t.Fatalf("project.read alone must not expose tasks: %+v", h)
	}
	if h, _ := e.svc.Search(bg, owner, SearchParams{Query: "walrus", Limit: 10}); len(h) != 1 {
		t.Fatal("the owner still finds it")
	}
}

// newEnvOn returns a second tenant on the same database.
func newEnvOn(e *env) *env {
	o := *e
	o.tenant = uuid.New()
	return &o
}

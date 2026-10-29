package pmclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// server is a tiny stand-in for the API: login and refresh issue tokens, every other route needs the current access token.
type server struct {
	t        *testing.T
	access   atomic.Value
	hosts    []string
	patched  Map
	expireOn atomic.Bool
}

func (s *server) handler() http.Handler {
	mux := http.NewServeMux()
	issue := func(w http.ResponseWriter, tok string) {
		s.access.Store(tok)
		_ = json.NewEncoder(w).Encode(Map{"data": Map{"access_token": tok, "refresh_token": "r-" + tok, "expires_at": time.Now().Add(time.Hour)}})
	}
	mux.HandleFunc("/api/v1/auth/login", func(w http.ResponseWriter, r *http.Request) {
		var in Map
		_ = json.NewDecoder(r.Body).Decode(&in)
		if in["password"] != "pw" {
			w.WriteHeader(401)
			_ = json.NewEncoder(w).Encode(Map{"error": Map{"code": "INVALID_CREDENTIALS", "message": "Invalid credentials provided"}})
			return
		}
		issue(w, "a1")
	})
	mux.HandleFunc("/api/v1/auth/refresh", func(w http.ResponseWriter, r *http.Request) { issue(w, "a2") })
	guard := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			s.hosts = append(s.hosts, r.Host)
			if s.expireOn.Load() || r.Header.Get("Authorization") != "Bearer "+s.access.Load().(string) {
				s.expireOn.Store(false)
				w.WriteHeader(401)
				_ = json.NewEncoder(w).Encode(Map{"error": Map{"code": "UNAUTHORIZED", "message": "Unauthorized request"}})
				return
			}
			h(w, r)
		}
	}
	mux.HandleFunc("/api/v1/projects", guard(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(Map{"data": []Map{{"id": "p1", "key": "ENG"}}, "meta": Map{"limit": 5, "next_cursor": "c2"}})
	}))
	mux.HandleFunc("/api/v1/tickets/ENG-1", guard(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(Map{"data": Map{"id": "t1", "version": 4}})
	}))
	mux.HandleFunc("/api/v1/tasks/t1", guard(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch {
			_ = json.NewDecoder(r.Body).Decode(&s.patched)
		}
		_ = json.NewEncoder(w).Encode(Map{"data": Map{"id": "t1", "version": 5}})
	}))
	mux.HandleFunc("/api/v1/gone", guard(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		_, _ = w.Write([]byte("<html>not json</html>"))
	}))
	return mux
}

func newTest(t *testing.T) (*Client, *server, func()) {
	s := &server{t: t}
	s.access.Store("")
	ts := httptest.NewServer(s.handler())
	c := New(ts.URL, "acme")
	return c, s, ts.Close
}

func TestLoginRefreshAndHostHeader(t *testing.T) {
	c, s, done := newTest(t)
	defer done()
	ctx := context.Background()
	if err := c.Login(ctx, "a@b.c", "wrong"); err == nil || !strings.Contains(err.Error(), "INVALID_CREDENTIALS") {
		t.Fatalf("bad login: %v", err)
	}
	if err := c.Login(ctx, "a@b.c", "pw"); err != nil || c.Tokens().Access != "a1" {
		t.Fatalf("login: %v %+v", err, c.Tokens())
	}
	page, err := c.Projects(ctx, "", "", "", 5)
	if err != nil || len(page.Items) != 1 || page.Next != "c2" {
		t.Fatalf("projects: %+v %v", page, err)
	}
	if !strings.HasPrefix(s.hosts[0], "acme.127.0.0.1") {
		t.Fatalf("the tenant is addressed through the Host header: %v", s.hosts)
	}
	s.expireOn.Store(true) // the next call answers 401 once: the client refreshes and retries
	if _, err = c.Projects(ctx, "", "", "", 5); err != nil || c.Tokens().Access != "a2" {
		t.Fatalf("refresh and retry: %v %+v", err, c.Tokens())
	}
}

func TestUpdateFetchesVersionAndKeyLookup(t *testing.T) {
	c, s, done := newTest(t)
	defer done()
	ctx := context.Background()
	_ = c.Login(ctx, "a@b.c", "pw")
	if _, err := c.UpdateTask(ctx, "ENG-1", Map{"title": "New"}); err != nil {
		t.Fatal(err)
	}
	if s.patched["version"] != float64(4) || s.patched["title"] != "New" {
		t.Fatalf("the current version is sent with the edit: %v", s.patched)
	}
	if _, err := c.UpdateTask(ctx, "t1", Map{"title": "x", "version": 9}); err != nil || s.patched["version"] != float64(9) {
		t.Fatalf("an explicit version wins: %v %v", err, s.patched)
	}
}

func TestErrorsAndNoCredentials(t *testing.T) {
	c, _, done := newTest(t)
	defer done()
	ctx := context.Background()
	if _, err := c.Projects(ctx, "", "", "", 1); err == nil {
		t.Fatal("unauthenticated calls fail")
	}
	_ = c.Login(ctx, "a@b.c", "pw")
	_, err := c.Call(ctx, "GET", "/gone", CallOpts{Query: url.Values{}}, nil)
	if ae, ok := err.(*APIError); !ok || ae.Status != 404 || ae.Code != "HTTP_ERROR" {
		t.Fatalf("non-JSON errors still become an APIError: %v", err)
	}
	c.SetTokens(Tokens{})
	if err = c.refresh(ctx); err == nil {
		t.Fatal("refresh without a refresh token")
	}
	c.Host = "custom.example"
	if c.hostHeader() != "custom.example" {
		t.Fatal("host override")
	}
}

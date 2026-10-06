// Package integration runs the Module 0-2 golden tests against the real production router (internal/app) in-process,
// backed by a real PostgreSQL and Redis. They are skipped when the infrastructure is unreachable unless
// JAAS_REQUIRE_INTEGRATION=1 (CI).
//
// Environment (defaults in parentheses):
//
//	JAAS_TEST_DB_HOST (localhost)  JAAS_TEST_DB_PORT (5432)  JAAS_TEST_DB_USER (postgres)  JAAS_TEST_DB_PASSWORD (postgres)
//	JAAS_TEST_REDIS_HOST (localhost)  JAAS_TEST_REDIS_PORT (6379)
//
// A throw-away database `jaas_itest` is dropped and recreated on every run; Redis DB 15 is used and flushed.
package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	neturl "net/url"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jaas/jaas/internal/app"
	"github.com/jaas/jaas/internal/shared/cache"
	"github.com/jaas/jaas/internal/shared/config"
	"github.com/jaas/jaas/internal/shared/logger"
	"github.com/jaas/jaas/internal/shared/queue"
	"github.com/rs/zerolog"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

const (
	platformKey = "itest-platform-key"
	password    = "Passw0rd!123"
	zeroUUID    = "00000000-0000-4000-8000-000000000000"
)

var (
	engine *gin.Engine
	testDB *gorm.DB
	rdb    *cache.RedisClient
	ipSeq  atomic.Int64
)

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func TestMain(m *testing.M) {
	code, err := setup(m)
	if err != nil {
		if os.Getenv("JAAS_REQUIRE_INTEGRATION") == "1" {
			fmt.Println("integration infrastructure required but unavailable:", err)
			os.Exit(1)
		}
		fmt.Println("SKIP integration tests (infrastructure unavailable):", err)
		os.Exit(0)
	}
	os.Exit(code)
}

func setup(m *testing.M) (int, error) {
	host, port := env("JAAS_TEST_DB_HOST", "localhost"), env("JAAS_TEST_DB_PORT", "5432")
	user, pass := env("JAAS_TEST_DB_USER", "postgres"), env("JAAS_TEST_DB_PASSWORD", "postgres")
	dsn := func(db string) string {
		return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable connect_timeout=3", host, port, user, pass, db)
	}
	quiet := &gorm.Config{TranslateError: true, Logger: gormlogger.Default.LogMode(gormlogger.Silent)}

	admin, err := gorm.Open(postgres.Open(dsn("postgres")), &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
	if err != nil {
		return 0, err
	}
	if err := admin.Exec("DROP DATABASE IF EXISTS jaas_itest WITH (FORCE)").Error; err != nil {
		return 0, err
	}
	if err := admin.Exec("CREATE DATABASE jaas_itest").Error; err != nil {
		return 0, err
	}
	if sqlDB, e := admin.DB(); e == nil {
		_ = sqlDB.Close()
	}

	testDB, err = gorm.Open(postgres.Open(dsn("jaas_itest")), quiet)
	if err != nil {
		return 0, err
	}

	rport, _ := strconv.Atoi(env("JAAS_TEST_REDIS_PORT", "6379"))
	rdb, err = cache.NewRedisClient(env("JAAS_TEST_REDIS_HOST", "localhost"), rport, "", 15)
	if err != nil {
		return 0, err
	}
	if err := rdb.DeleteByPrefix(context.Background(), ""); err != nil { // flush the test DB (index 15)
		return 0, err
	}

	cfg := &config.Config{
		Server:   config.ServerConfig{Env: "test", Port: 0},
		JWT:      config.JWTConfig{Secret: strings.Repeat("s", 40), AccessTokenTTL: 15, RefreshTokenTTL: 7},
		Platform: config.PlatformConfig{AdminKey: platformKey},
	}
	gin.SetMode(gin.ReleaseMode)
	engine, err = app.Build(app.Options{
		Config:      cfg,
		DB:          testDB,
		Redis:       rdb,
		Publisher:   queue.NewNoOpPublisher(),
		Logger:      &logger.Logger{Logger: zerolog.Nop()},
		CORSOrigins: []string{"https://app.example.com"},
	})
	if err != nil {
		return 0, err
	}
	return m.Run(), nil
}

// ---------------------------------------------------------------- request DSL

type request struct {
	method, path string
	host         string
	token        string
	body         any
	raw          string
	query        map[string]string
	headers      map[string]string
	ip           string
}

type opt func(*request)

func host(slug string) opt   { return func(r *request) { r.host = slug + ".localhost" } }
func token(t string) opt     { return func(r *request) { r.token = t } }
func body(b any) opt         { return func(r *request) { r.body = b } }
func rawBody(s string) opt   { return func(r *request) { r.raw = s } }
func header(k, v string) opt { return func(r *request) { r.headers[k] = v } }
func ip(a string) opt        { return func(r *request) { r.ip = a } }
func platform() opt          { return header("X-Platform-Key", platformKey) }
func query(kv ...string) opt {
	return func(r *request) {
		for i := 0; i+1 < len(kv); i += 2 {
			r.query[kv[i]] = kv[i+1]
		}
	}
}

type resp struct {
	Status int
	Body   map[string]any
	Raw    string
	Header http.Header
}

// call performs a request against the in-process engine. Every call gets a fresh client IP (unless ip() is given) so
// the login rate limiter never couples unrelated tests.
func call(t *testing.T, method, path string, opts ...opt) resp {
	t.Helper()
	r := &request{method: method, path: path, query: map[string]string{}, headers: map[string]string{}}
	for _, o := range opts {
		o(r)
	}
	url := "/api/v1" + path
	if strings.HasPrefix(path, "/health") {
		url = path
	}
	if len(r.query) > 0 {
		vals := neturl.Values{}
		for k, v := range r.query {
			vals.Set(k, v)
		}
		url += "?" + vals.Encode()
	}
	var rdr *bytes.Reader
	switch {
	case r.raw != "":
		rdr = bytes.NewReader([]byte(r.raw))
	case r.body != nil:
		b, err := json.Marshal(r.body)
		if err != nil {
			t.Fatal(err)
		}
		rdr = bytes.NewReader(b)
	default:
		rdr = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, url, rdr)
	if r.host != "" {
		req.Host = r.host
	}
	if r.ip == "" {
		n := ipSeq.Add(1)
		r.ip = fmt.Sprintf("10.%d.%d.%d", (n>>16)&255, (n>>8)&255, n&255)
	}
	req.RemoteAddr = r.ip + ":4000"
	if r.token != "" {
		req.Header.Set("Authorization", "Bearer "+r.token)
	}
	if r.body != nil || r.raw != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range r.headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	out := resp{Status: w.Code, Raw: w.Body.String(), Header: w.Header()}
	_ = json.Unmarshal(w.Body.Bytes(), &out.Body)
	return out
}

// get walks a dotted path ("data.roles.0.name") in the decoded JSON body.
func (r resp) get(path string) any {
	var cur any = r.Body
	for _, p := range strings.Split(path, ".") {
		switch v := cur.(type) {
		case map[string]any:
			cur = v[p]
		case []any:
			i, err := strconv.Atoi(p)
			if err != nil || i < 0 || i >= len(v) {
				return nil
			}
			cur = v[i]
		default:
			return nil
		}
	}
	return cur
}

func (r resp) str(path string) string {
	if s, ok := r.get(path).(string); ok {
		return s
	}
	return ""
}

func (r resp) list(path string) []any {
	l, _ := r.get(path).([]any)
	return l
}

// has reports whether the list at path contains an object whose field equals value.
func (r resp) has(path, field, value string) bool {
	for _, it := range r.list(path) {
		if m, ok := it.(map[string]any); ok && fmt.Sprint(m[field]) == value {
			return true
		}
	}
	return false
}

func (r resp) errCode() string { return r.str("error.code") }

func expect(t *testing.T, r resp, status int, what string) {
	t.Helper()
	if r.Status != status {
		t.Fatalf("%s: want HTTP %d, got %d: %s", what, status, r.Status, r.Raw)
	}
}

func expectErr(t *testing.T, r resp, status int, code, what string) {
	t.Helper()
	if r.Status != status || (code != "" && r.errCode() != code) {
		t.Fatalf("%s: want HTTP %d %s, got %d %q: %s", what, status, code, r.Status, r.errCode(), r.Raw)
	}
}

// ---------------------------------------------------------------- fixtures

type tenantCtx struct {
	slug, id, adminEmail, adminID, token, refresh string
}

var slugSeq atomic.Int64

func uniq() string {
	return strconv.FormatInt(slugSeq.Add(1), 36) + strconv.FormatInt(int64(os.Getpid()), 36)
}

// newTenant provisions a tenant (with its first tenant_admin) through the platform API and logs the admin in.
func newTenant(t *testing.T) *tenantCtx {
	t.Helper()
	u := uniq()
	slug := "t" + u
	email := "root@" + slug + ".test"
	r := call(t, "POST", "/tenants", platform(), body(map[string]any{
		"name": "Tenant " + u, "slug": slug,
		"admin": map[string]any{"email": email, "password": password, "first_name": "Root", "last_name": "Admin"},
	}))
	expect(t, r, 201, "create tenant")
	tc := &tenantCtx{slug: slug, id: r.str("data.id"), adminEmail: email, adminID: r.str("data.admin_user_id")}
	l := login(t, slug, email, password)
	tc.token, tc.refresh = l.str("data.access_token"), l.str("data.refresh_token")
	return tc
}

func login(t *testing.T, slug, email, pw string) resp {
	t.Helper()
	r := call(t, "POST", "/auth/login", body(map[string]string{"tenant_slug": slug, "email": email, "password": pw}))
	expect(t, r, 200, "login "+email)
	return r
}

func (tc *tenantCtx) do(t *testing.T, method, path string, opts ...opt) resp {
	t.Helper()
	return call(t, method, path, append([]opt{host(tc.slug), token(tc.token)}, opts...)...)
}

// roleID returns the id of a role by name.
func (tc *tenantCtx) roleID(t *testing.T, name string) string {
	t.Helper()
	r := tc.do(t, "GET", "/roles", query("per_page", "100"))
	for _, it := range r.list("data") {
		m := it.(map[string]any)
		if m["name"] == name {
			return m["id"].(string)
		}
	}
	t.Fatalf("role %s not found", name)
	return ""
}

// permID returns the id of resource:action from the seeded catalogue.
func (tc *tenantCtx) permID(t *testing.T, resource, action string) string {
	t.Helper()
	r := tc.do(t, "GET", "/permissions", query("per_page", "100"))
	for _, it := range r.list("data") {
		m := it.(map[string]any)
		if m["resource"] == resource && m["action"] == action {
			return m["id"].(string)
		}
	}
	t.Fatalf("permission %s:%s not seeded", resource, action)
	return ""
}

// newUser creates a user (optionally with roles) and returns its id and email.
func (tc *tenantCtx) newUser(t *testing.T, tag string, roleIDs ...string) (string, string) {
	t.Helper()
	email := fmt.Sprintf("%s.%s@%s.test", tag, uniq(), tc.slug)
	b := map[string]any{"email": email, "password": password, "first_name": "F" + tag, "last_name": "L" + tag}
	if len(roleIDs) > 0 {
		b["role_ids"] = roleIDs
	}
	r := tc.do(t, "POST", "/users", body(b))
	expect(t, r, 201, "create user "+tag)
	return r.str("data.id"), email
}

// limitedUser creates a role holding exactly the given resource:action permissions plus a user with it, and returns a
// token for that user together with the role id (so tests can grant more later).
func (tc *tenantCtx) limitedUser(t *testing.T, perms ...[2]string) (token, roleID, userID string) {
	t.Helper()
	ids := make([]string, 0, len(perms))
	for _, p := range perms {
		ids = append(ids, tc.permID(t, p[0], p[1]))
	}
	rr := tc.do(t, "POST", "/roles", body(map[string]any{"name": "lim-" + uniq(), "permission_ids": ids}))
	expect(t, rr, 201, "create limited role")
	roleID = rr.str("data.id")
	userID, email := tc.newUser(t, "lim", roleID)
	l := login(t, tc.slug, email, password)
	return l.str("data.access_token"), roleID, userID
}

func (tc *tenantCtx) grant(t *testing.T, roleID, resource, action string) {
	t.Helper()
	r := tc.do(t, "POST", "/roles/"+roleID+"/permissions", body(map[string]any{"permission_ids": []string{tc.permID(t, resource, action)}}))
	expect(t, r, 200, "grant "+resource+":"+action)
}

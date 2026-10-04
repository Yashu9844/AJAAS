// Package api holds Module 1 organization integration tests executed against a
// live backend (Docker PG + Redis + RabbitMQ). They require:
//   - `docker compose up -d postgres redis rabbitmq` from the repo root
//   - the backend running locally (DATABASE_PASSWORD/DATABASE_DBNAME env, port 8080)
//
// Run: go test ./tests/api/ -v. Set JAAS_BASE_URL to override the default.
package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"
)

func baseURL() string {
	if v := os.Getenv("JAAS_BASE_URL"); v != "" {
		return v
	}
	return "http://localhost:8080"
}

var client = &http.Client{Timeout: 10 * time.Second}

type envelope struct {
	Data  json.RawMessage `json:"data"`
	Meta  json.RawMessage `json:"meta"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func doReq(t *testing.T, method, path, host, token string, body interface{}) (int, envelope) {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		rdr = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, baseURL()+path, rdr)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if host != "" {
		req.Host = host
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("decode %s %s: %v (%s)", method, path, err, string(raw))
	}
	return resp.StatusCode, env
}

func strField(t *testing.T, raw json.RawMessage, field string) string {
	t.Helper()
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("decode data: %v", err)
	}
	v, _ := m[field].(string)
	return v
}

// nestedField reads data.outer.inner as a string (e.g. login user.id).
func nestedField(t *testing.T, raw json.RawMessage, outer, inner string) string {
	t.Helper()
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("decode data: %v", err)
	}
	return strField(t, m[outer], inner)
}

// bootstrapTenant creates a tenant and returns its slug + id.
func bootstrapTenant(t *testing.T, slug string) string {
	t.Helper()
	status, env := doReq(t, http.MethodPost, "/api/v1/tenants", "", "", map[string]string{
		"name": slug + " Corp " + randomSuffix(), "slug": slug,
	})
	if status == 409 {
		return slug // already exists from a previous run
	}
	if status != 200 && status != 201 {
		t.Fatalf("create tenant: status=%d err=%+v", status, env.Error)
	}
	return slug
}

// uniq prefixes a resource name with the tenant slug and a random suffix so
// reruns never collide with soft-deleted rows (codes/names stay reserved).
func uniq(slug, name string) string {
	return slug + "-" + name + "-" + randomSuffix()
}

// adminToken logs in with the seeded admin (see docs seeding notes) and returns the JWT.
func adminToken(t *testing.T, slug string) string {
	t.Helper()
	status, env := doReq(t, http.MethodPost, "/api/v1/auth/login", slug+".localhost", "", map[string]string{
		"email": "admin@" + slug + ".com", "password": "Secret123!", "tenant_slug": slug,
	})
	if status != 200 {
		t.Fatalf("admin login: status=%d err=%+v", status, env.Error)
	}
	return strField(t, env.Data, "access_token")
}

// memberEmail is unique per run (soft-deleted users keep emails reserved).
func memberEmail(slug string) string {
	return "member-" + randomSuffix() + "@" + slug + ".com"
}

func orgPost(t *testing.T, slug, token, path string, body interface{}) (int, envelope) {
	t.Helper()
	return doReq(t, http.MethodPost, path, slug+".localhost", token, body)
}

func orgGet(t *testing.T, slug, token, path string) (int, envelope) {
	t.Helper()
	return doReq(t, http.MethodGet, path, slug+".localhost", token, nil)
}

func requireStatus(t *testing.T, what string, got, want int, env envelope) {
	t.Helper()
	if got != want {
		msg := ""
		if env.Error != nil {
			msg = fmt.Sprintf(" code=%s msg=%s", env.Error.Code, env.Error.Message)
		}
		t.Fatalf("%s: want %d, got %d%s", what, want, got, msg)
	}
}

// deactivateAllPrimaries deactivates every active primary mapping in a list
// payload so reruns start clean (the admin user is reused across runs).
func deactivateAllPrimaries(t *testing.T, slug, token string, list json.RawMessage) {
	t.Helper()
	var items []struct {
		ID        string `json:"id"`
		IsPrimary bool   `json:"is_primary"`
		Status    string `json:"status"`
	}
	if err := json.Unmarshal(list, &items); err != nil {
		t.Fatalf("decode mappings list: %v", err)
	}
	for _, m := range items {
		if m.IsPrimary && m.Status == "active" {
			status, env := doReq(t, http.MethodPost, "/api/v1/mappings/"+m.ID+"/deactivate", slug+".localhost", token, map[string]string{"reason": "e2e rerun cleanup"})
			requireStatus(t, "cleanup deactivate mapping", status, 200, env)
		}
	}
}

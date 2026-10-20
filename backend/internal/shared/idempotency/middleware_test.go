package idempotency

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type memStore struct {
	mu       sync.Mutex
	rows     map[string]*Record
	claimErr error
	purged   int
}

func keyOf(r *Record) string { return r.Scope + "|" + r.Method + "|" + r.Path + "|" + r.Key }

func (m *memStore) Claim(_ context.Context, r *Record) (*Record, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.claimErr != nil {
		return nil, false, m.claimErr
	}
	if ex, ok := m.rows[keyOf(r)]; ok {
		cp := *ex
		return &cp, false, nil
	}
	m.rows[keyOf(r)] = r
	return nil, true, nil
}

func (m *memStore) Complete(_ context.Context, id uuid.UUID, status int, body []byte, ct string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.rows {
		if r.ID == id {
			r.Status, r.ResponseStatus, r.ResponseBody, r.ContentType = StatusCompleted, status, body, ct
		}
	}
	return nil
}

func (m *memStore) Release(_ context.Context, id uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, r := range m.rows {
		if r.ID == id {
			delete(m.rows, k)
		}
	}
	return nil
}

func (m *memStore) PurgeExpired(context.Context, time.Time) (int64, error) {
	m.purged++
	return 0, nil
}

func engine(store Store, calls *int, status int) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Middleware(store, func(c *gin.Context) (string, bool) {
		s := c.GetHeader("X-Scope")
		return s, s != ""
	}, 0))
	r.POST("/things", func(c *gin.Context) {
		*calls++
		c.JSON(status, gin.H{"n": *calls})
	})
	r.GET("/things", func(c *gin.Context) { *calls++; c.Status(http.StatusOK) })
	return r
}

func post(r *gin.Engine, key, scope, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/things", strings.NewReader(body))
	if key != "" {
		req.Header.Set(Header, key)
	}
	if scope != "" {
		req.Header.Set("X-Scope", scope)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestReplaySameKeySamePayload(t *testing.T) {
	calls := 0
	store := &memStore{rows: map[string]*Record{}}
	r := engine(store, &calls, http.StatusCreated)
	first := post(r, "k1", "t:u", `{"a":1}`)
	again := post(r, "k1", "t:u", `{"a":1}`)
	if calls != 1 || again.Code != http.StatusCreated || again.Body.String() != first.Body.String() || again.Header().Get(ReplayedHeader) != "true" {
		t.Fatalf("replay: calls=%d %d %s", calls, again.Code, again.Body)
	}
	if w := post(r, "k1", "t:u", `{"a":2}`); w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "IDEMPOTENCY_KEY_REUSED") {
		t.Fatalf("different payload: %d %s", w.Code, w.Body)
	}
	if post(r, "k1", "t:other", `{"a":1}`); calls != 2 {
		t.Fatal("keys are scoped per caller")
	}
	post(r, "", "t:u", `{}`)
	post(r, "", "t:u", `{}`)
	if calls != 4 {
		t.Fatal("no key means no deduplication")
	}
	if post(r, "k9", "", `{}`); calls != 5 {
		t.Fatal("unscoped callers bypass")
	}
	if w := post(r, strings.Repeat("x", 256), "t:u", `{}`); w.Code != http.StatusBadRequest {
		t.Fatal("overlong key")
	}
}

func TestInProgressExpiredAndServerErrors(t *testing.T) {
	calls := 0
	store := &memStore{rows: map[string]*Record{}}
	h := hash("POST", "/things", []byte(`{}`))
	store.rows["t:u|POST|/things|busy"] = &Record{ID: uuid.New(), Scope: "t:u", Method: "POST", Path: "/things", Key: "busy",
		RequestHash: h, Status: StatusProcessing, ExpiresAt: time.Now().Add(time.Hour)}
	store.rows["t:u|POST|/things|old"] = &Record{ID: uuid.New(), Scope: "t:u", Method: "POST", Path: "/things", Key: "old",
		RequestHash: h, Status: StatusCompleted, ExpiresAt: time.Now().Add(-time.Hour)}
	r := engine(store, &calls, http.StatusCreated)
	if w := post(r, "busy", "t:u", `{}`); w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "IN_PROGRESS") {
		t.Fatalf("in progress: %d %s", w.Code, w.Body)
	}
	if w := post(r, "old", "t:u", `{}`); w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "EXPIRED") {
		t.Fatalf("expired: %d", w.Code)
	}
	failing := engine(store, &calls, http.StatusInternalServerError)
	before := calls
	post(failing, "boom", "t:u", `{}`)
	post(failing, "boom", "t:u", `{}`)
	if calls != before+2 {
		t.Fatal("5xx releases the key so a retry runs again")
	}
	store.claimErr = errors.New("db down")
	if w := post(r, "x", "t:u", `{}`); w.Code != http.StatusInternalServerError {
		t.Fatalf("store failure: %d", w.Code)
	}
	store.claimErr = nil
	req := httptest.NewRequest(http.MethodGet, "/things", nil)
	req.Header.Set(Header, "k")
	req.Header.Set("X-Scope", "t:u")
	before = calls
	r.ServeHTTP(httptest.NewRecorder(), req)
	r.ServeHTTP(httptest.NewRecorder(), req)
	if calls != before+2 {
		t.Fatal("reads are never deduplicated")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	RunPurger(ctx, store, time.Hour)
	if store.purged != 1 {
		t.Fatal("purger runs once then stops")
	}
}

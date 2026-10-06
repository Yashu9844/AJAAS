package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

type fakeStore struct {
	counts map[string]int64
	failIn bool
	ttl    time.Duration
}

func (f *fakeStore) IncrWithExpire(_ context.Context, key string, _ time.Duration) (int64, error) {
	if f.failIn {
		return 0, errors.New("redis down")
	}
	f.counts[key]++
	return f.counts[key], nil
}
func (f *fakeStore) Decr(_ context.Context, key string) error {
	if f.counts[key] > 0 {
		f.counts[key]--
	}
	return nil
}
func (f *fakeStore) TTL(_ context.Context, _ string) (time.Duration, error) { return f.ttl, nil }

func hit(r *gin.Engine) *httptest.ResponseRecorder {
	req, _ := http.NewRequest(http.MethodPost, "/login", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func engine(mw gin.HandlerFunc, status *int) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/login", mw, func(c *gin.Context) { c.Status(*status) })
	return r
}

func TestRateLimiter_BlocksAfterLimitWithRetryAfter(t *testing.T) {
	st := &fakeStore{counts: map[string]int64{}, ttl: 30 * time.Second}
	code := 200
	r := engine(RateLimiter(st, 2, time.Minute), &code)
	if hit(r).Code != 200 || hit(r).Code != 200 {
		t.Fatal("first two requests must pass")
	}
	w := hit(r)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("third must be 429, got %d", w.Code)
	}
	if w.Header().Get("Retry-After") != "31" {
		t.Errorf("Retry-After from TTL expected 31, got %q", w.Header().Get("Retry-After"))
	}
}

func TestFailureRateLimiter_SuccessRefundsSlot(t *testing.T) {
	st := &fakeStore{counts: map[string]int64{}}
	code := 200
	r := engine(FailureRateLimiter(st, 2, time.Minute), &code)
	for i := 0; i < 10; i++ { // many successes never exhaust the budget
		if hit(r).Code != 200 {
			t.Fatalf("success #%d blocked", i)
		}
	}
	code = 401
	hit(r)
	hit(r)
	if hit(r).Code != http.StatusTooManyRequests {
		t.Fatal("failed attempts beyond the limit must be throttled")
	}
}

func TestRateLimiter_FailsOpenOnStoreError(t *testing.T) {
	st := &fakeStore{counts: map[string]int64{}, failIn: true}
	code := 200
	r := engine(RateLimiter(st, 1, time.Minute), &code)
	for i := 0; i < 3; i++ {
		if hit(r).Code != 200 {
			t.Fatal("must fail open when the store errors")
		}
	}
}

package middleware

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestBodyLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(BodyLimit(16))
	r.POST("/x", func(c *gin.Context) {
		if _, err := io.ReadAll(c.Request.Body); err != nil {
			c.Status(http.StatusRequestEntityTooLarge)
			return
		}
		c.Status(http.StatusOK)
	})
	do := func(body string, chunked bool) int {
		req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(body))
		if chunked {
			req.ContentLength = -1
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w.Code
	}
	if c := do("small", false); c != 200 {
		t.Fatalf("small body: %d", c)
	}
	if c := do(strings.Repeat("a", 100), false); c != http.StatusRequestEntityTooLarge {
		t.Fatalf("declared oversize must be 413, got %d", c)
	}
	if c := do(strings.Repeat("a", 100), true); c != http.StatusRequestEntityTooLarge {
		t.Fatalf("undeclared oversize must be 413, got %d", c)
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/x", bytes.NewReader([]byte(strings.Repeat("a", 100))))
	r.ServeHTTP(w, req)
	if !strings.Contains(w.Body.String(), "PAYLOAD_TOO_LARGE") {
		t.Fatalf("envelope: %s", w.Body.String())
	}
}

func TestSecurityHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(SecurityHeaders())
	r.GET("/x", func(c *gin.Context) { c.Status(200) })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))
	for _, h := range []string{"X-Content-Type-Options", "X-Frame-Options", "Referrer-Policy", "Cache-Control", "Content-Security-Policy"} {
		if w.Header().Get(h) == "" {
			t.Errorf("missing %s", h)
		}
	}
}

// A chunked body (no Content-Length) is read once and handed on with its real length, so handlers that treat an unknown
// length as "no body" still see it.
func TestBodyLimitGivesChunkedBodiesALength(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(BodyLimit(64))
	var seen int64
	var got string
	r.POST("/x", func(c *gin.Context) {
		seen = c.Request.ContentLength
		b, _ := io.ReadAll(c.Request.Body)
		got = string(b)
		c.Status(http.StatusOK)
	})
	for _, tc := range []struct {
		body string
		want int64
	}{{`{"comment":"late"}`, 18}, {"", 0}} {
		req := httptest.NewRequest(http.MethodPost, "/x", io.NopCloser(strings.NewReader(tc.body)))
		req.ContentLength = -1
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != 200 || seen != tc.want || got != tc.body {
			t.Fatalf("chunked %q: status %d length %d body %q", tc.body, w.Code, seen, got)
		}
	}
	req := httptest.NewRequest(http.MethodPost, "/x", io.NopCloser(strings.NewReader(strings.Repeat("a", 100))))
	req.ContentLength = -1
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusRequestEntityTooLarge || !strings.Contains(w.Body.String(), "PAYLOAD_TOO_LARGE") {
		t.Fatalf("oversize chunked body: %d %s", w.Code, w.Body.String())
	}
}

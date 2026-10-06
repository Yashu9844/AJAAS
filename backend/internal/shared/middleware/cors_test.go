package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func corsReq(allowed []string, method, origin string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(CORS(allowed))
	r.Any("/x", func(c *gin.Context) { c.Status(200) })
	req, _ := http.NewRequest(method, "/x", nil)
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestCORS(t *testing.T) {
	w := corsReq([]string{"*"}, "GET", "http://a.test")
	if w.Header().Get("Access-Control-Allow-Origin") != "*" || w.Header().Get("Access-Control-Allow-Credentials") != "" {
		t.Errorf("wildcard must not allow credentials: %v", w.Header())
	}
	w = corsReq([]string{"https://app.example.com"}, "GET", "https://app.example.com")
	if w.Header().Get("Access-Control-Allow-Origin") != "https://app.example.com" || w.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Errorf("exact origin must be echoed: %v", w.Header())
	}
	w = corsReq([]string{"https://app.example.com"}, "GET", "https://evil.test")
	if w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Error("unlisted origin must get no CORS headers")
	}
	w = corsReq([]string{"https://*.example.com"}, "GET", "https://acme.example.com")
	if w.Header().Get("Access-Control-Allow-Origin") != "https://acme.example.com" {
		t.Errorf("subdomain wildcard must match: %v", w.Header())
	}
	w = corsReq([]string{"https://*.example.com"}, "GET", "https://example.com.evil.test")
	if w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Error("suffix trick must not match")
	}
	w = corsReq(nil, "GET", "https://a.test")
	if w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Error("empty allow-list disables CORS")
	}
	w = corsReq([]string{"*"}, "OPTIONS", "http://a.test")
	if w.Code != http.StatusNoContent {
		t.Errorf("preflight must be 204, got %d", w.Code)
	}
}

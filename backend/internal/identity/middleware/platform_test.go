package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func platformStatus(key, header string) int {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(PlatformAdmin(key))
	r.GET("/t", func(c *gin.Context) { c.Status(http.StatusOK) })
	req, _ := http.NewRequest(http.MethodGet, "/t", nil)
	if header != "" {
		req.Header.Set(PlatformKeyHeader, header)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w.Code
}

func TestPlatformAdmin(t *testing.T) {
	if got := platformStatus("", "anything"); got != http.StatusServiceUnavailable {
		t.Errorf("unconfigured key must fail closed, got %d", got)
	}
	if got := platformStatus("secret", ""); got != http.StatusUnauthorized {
		t.Errorf("missing header: want 401, got %d", got)
	}
	if got := platformStatus("secret", "wrong"); got != http.StatusForbidden {
		t.Errorf("wrong key: want 403, got %d", got)
	}
	if got := platformStatus("secret", "secret"); got != http.StatusOK {
		t.Errorf("right key: want 200, got %d", got)
	}
}

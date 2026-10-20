package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/shared/logger"
)

func TestCorrelationIDIsValidatedEchoedAndPropagated(t *testing.T) {
	gin.SetMode(gin.TestMode)
	log := logger.NewLogger("test")
	r := gin.New()
	r.Use(RequestLogger(log))
	var seen uuid.UUID
	r.GET("/x", func(c *gin.Context) { seen, _ = CorrelationIDFrom(c.Request.Context()) })
	do := func(h map[string]string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		for k, v := range h {
			req.Header.Set(k, v)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	given := uuid.New()
	w := do(map[string]string{CorrelationHeader: given.String()})
	if seen != given || w.Header().Get(CorrelationHeader) != given.String() || w.Header().Get(RequestIDHeader) != given.String() {
		t.Fatalf("valid correlation id is kept and echoed: %s %s", seen, w.Header())
	}
	legacy := uuid.New()
	if do(map[string]string{RequestIDHeader: legacy.String()}); seen != legacy {
		t.Fatal("X-Request-ID still accepted")
	}
	w = do(map[string]string{CorrelationHeader: "drop table; --"})
	if seen == uuid.Nil || w.Header().Get(CorrelationHeader) != seen.String() {
		t.Fatal("invalid ids are replaced by a fresh UUID")
	}
	if _, ok := CorrelationIDFrom(context.Background()); ok {
		t.Fatal("no id on a bare context")
	}
	if CorrelationOrNew(context.Background()) == uuid.Nil || CorrelationOrNew(WithCorrelationID(context.Background(), given)) != given {
		t.Fatal("CorrelationOrNew")
	}
}

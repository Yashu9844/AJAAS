package routes

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"context"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/organization/services"
)

type fakeInv struct{ calls []uuid.UUID }

func (f *fakeInv) InvalidateChart(_ context.Context, id uuid.UUID) { f.calls = append(f.calls, id) }

func invOrNil(f *fakeInv) services.ChartInvalidator {
	if f == nil {
		return nil
	}
	return f
}

func run(method string, status int, inv *fakeInv, withTenant bool) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		if withTenant {
			c.Set("tenant_id", uuid.New())
		}
	})
	r.Use(chartInvalidation(invOrNil(inv)))
	h := func(c *gin.Context) { c.Status(status) }
	r.Handle(method, "/x", h)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(method, "/x", nil)
	r.ServeHTTP(w, req)
}

func TestChartInvalidation(t *testing.T) {
	inv := &fakeInv{}
	run("POST", 201, inv, true)
	run("PATCH", 200, inv, true)
	if len(inv.calls) != 2 {
		t.Fatalf("successful writes must invalidate, got %d", len(inv.calls))
	}
	run("GET", 200, inv, true)
	run("POST", 409, inv, true)
	run("POST", 201, inv, false)
	if len(inv.calls) != 2 {
		t.Fatalf("GET, failed writes and tenant-less requests must not invalidate, got %d", len(inv.calls))
	}
	// nil invalidator is a no-op
	run("POST", 201, nil, true)
}

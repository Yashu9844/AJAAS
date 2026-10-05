package controllers

import (
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestOrgChartController_ChartAndChain(t *testing.T) {
	ctrl := NewOrgChartController(nil, &stubChartSvc{})

	c, w := newTestContext(t, http.MethodGet, "/org-chart?max_depth=5", nil)
	ctrl.Chart(c)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"data"`) {
		t.Fatalf("expected 200 with data, got %d %s", w.Code, w.Body.String())
	}

	c2, w2 := newTestContext(t, http.MethodGet, "/org-chart/chain?user_id="+uuid.New().String(), nil)
	ctrl.UserChain(c2)
	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w2.Code)
	}

	// Missing user_id → 400.
	c3, w3 := newTestContext(t, http.MethodGet, "/org-chart/chain", nil)
	ctrl.UserChain(c3)
	if w3.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 without user_id, got %d", w3.Code)
	}
}

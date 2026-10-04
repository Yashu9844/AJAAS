package controllers

import (
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/organization/dto"
)

func TestDesignationController_CRUD(t *testing.T) {
	ctrl := NewDesignationController(nil, &stubDesigSvc{res: &dto.DesignationResponse{ID: uuid.New().String(), Title: "Lead", Status: "active"}})

	c, w := newTestContext(t, http.MethodPost, "/designations", strings.NewReader(`{"title":"Lead"}`))
	ctrl.Create(c)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	c2, w2 := newTestContext(t, http.MethodGet, "/designations/x", nil)
	c2.Params = gin.Params{{Key: "id", Value: uuid.New().String()}}
	ctrl.GetByID(c2)
	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w2.Code)
	}

	c3, w3 := newTestContext(t, http.MethodGet, "/designations", nil)
	ctrl.List(c3)
	if w3.Code != http.StatusOK || !strings.Contains(w3.Body.String(), `"meta"`) {
		t.Fatalf("expected 200 with meta, got %d", w3.Code)
	}

	// Deactivate with referenced conflict → 409 (FR-DG004).
	ctrlC := NewDesignationController(nil, &stubDesigSvc{err: errConflict})
	c4, w4 := newTestContext(t, http.MethodPost, "/designations/x/deactivate", nil)
	c4.Params = gin.Params{{Key: "id", Value: uuid.New().String()}}
	ctrlC.Deactivate(c4)
	if w4.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", w4.Code)
	}
}

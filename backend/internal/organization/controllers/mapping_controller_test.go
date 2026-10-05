package controllers

import (
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/organization/dto"
)

func TestMappingController_CreateAndList(t *testing.T) {
	ctrl := NewMappingController(nil, &stubMappingSvc{res: &dto.MappingResponse{ID: uuid.New().String(), Status: "active", IsPrimary: true}})

	c, w := newTestContext(t, http.MethodPost, "/mappings", strings.NewReader(`{"user_id":"`+uuid.New().String()+`","department_id":"`+uuid.New().String()+`","is_primary":true}`))
	ctrl.Create(c)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	// List requires user_id.
	c2, w2 := newTestContext(t, http.MethodGet, "/mappings?user_id="+uuid.New().String(), nil)
	ctrl.ListByUser(c2)
	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w2.Code)
	}

	// Missing user_id → 400.
	c3, w3 := newTestContext(t, http.MethodGet, "/mappings", nil)
	ctrl.ListByUser(c3)
	if w3.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 without user_id, got %d", w3.Code)
	}
}

func TestMappingController_UpdateAndDeactivate(t *testing.T) {
	ctrl := NewMappingController(nil, &stubMappingSvc{res: &dto.MappingResponse{Status: "inactive"}})

	c, w := newTestContext(t, http.MethodPatch, "/mappings/x", strings.NewReader(`{"is_primary":true}`))
	c.Params = gin.Params{{Key: "id", Value: uuid.New().String()}}
	ctrl.Update(c)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	c2, w2 := newTestContext(t, http.MethodPost, "/mappings/x/deactivate", strings.NewReader(`{"reason":"left team"}`))
	c2.Params = gin.Params{{Key: "id", Value: uuid.New().String()}}
	ctrl.Deactivate(c2)
	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w2.Code, w2.Body.String())
	}

	// Already inactive → 409 from service.
	ctrlC := NewMappingController(nil, &stubMappingSvc{err: errConflict})
	c3, w3 := newTestContext(t, http.MethodPost, "/mappings/x/deactivate", strings.NewReader(`{}`))
	c3.Params = gin.Params{{Key: "id", Value: uuid.New().String()}}
	ctrlC.Deactivate(c3)
	if w3.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", w3.Code)
	}
}

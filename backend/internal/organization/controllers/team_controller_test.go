package controllers

import (
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/organization/dto"
)

func TestTeamController_CreateAndGet(t *testing.T) {
	ctrl := NewTeamController(nil, &stubTeamSvc{res: &dto.TeamResponse{ID: uuid.New().String(), Name: "Platform", Status: "active"}})

	c, w := newTestContext(t, http.MethodPost, "/teams", strings.NewReader(`{"name":"Platform","department_id":"`+uuid.New().String()+`"}`))
	ctrl.Create(c)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	c2, w2 := newTestContext(t, http.MethodGet, "/teams/x", nil)
	c2.Params = gin.Params{{Key: "id", Value: uuid.New().String()}}
	ctrl.GetByID(c2)
	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w2.Code)
	}

	// Service conflict (duplicate name) → 409.
	ctrlC := NewTeamController(nil, &stubTeamSvc{err: errConflict})
	c3, w3 := newTestContext(t, http.MethodPost, "/teams", strings.NewReader(`{"name":"Platform","department_id":"`+uuid.New().String()+`"}`))
	ctrlC.Create(c3)
	if w3.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", w3.Code)
	}
}

func TestTeamController_ListAndDeactivate(t *testing.T) {
	ctrl := NewTeamController(nil, &stubTeamSvc{})

	c, w := newTestContext(t, http.MethodGet, "/teams?department_id="+uuid.New().String(), nil)
	ctrl.List(c)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	// Bad department_id filter → 400.
	c2, w2 := newTestContext(t, http.MethodGet, "/teams?department_id=bad", nil)
	ctrl.List(c2)
	if w2.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 on bad filter, got %d", w2.Code)
	}

	// Deactivate happy path (no body).
	ctrlD := NewTeamController(nil, &stubTeamSvc{res: &dto.TeamResponse{Status: "inactive"}})
	c3, w3 := newTestContext(t, http.MethodPost, "/teams/x/deactivate", nil)
	c3.Params = gin.Params{{Key: "id", Value: uuid.New().String()}}
	ctrlD.Deactivate(c3)
	if w3.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w3.Code, w3.Body.String())
	}

	// Deactivate with reason body.
	c4, w4 := newTestContext(t, http.MethodPost, "/teams/x/deactivate", strings.NewReader(`{"reason":"reorg"}`))
	c4.Params = gin.Params{{Key: "id", Value: uuid.New().String()}}
	ctrlD.Deactivate(c4)
	if w4.Code != http.StatusOK {
		t.Fatalf("expected 200 with reason body, got %d", w4.Code)
	}
}

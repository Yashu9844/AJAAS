package controllers

import (
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/organization/dto"
)

func TestDepartmentController_UpdateAndErrors(t *testing.T) {
	// Happy update.
	ctrl := NewDepartmentController(nil, &stubDeptSvc{res: &dto.DepartmentResponse{ID: uuid.New().String(), Name: "Eng", Status: "active"}})
	c, w := newTestContext(t, http.MethodPatch, "/departments/x", strings.NewReader(`{"name":"Eng"}`))
	c.Params = gin.Params{{Key: "id", Value: uuid.New().String()}}
	ctrl.Update(c)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Bad body.
	c2, w2 := newTestContext(t, http.MethodPatch, "/departments/x", strings.NewReader(`{bad`))
	c2.Params = gin.Params{{Key: "id", Value: uuid.New().String()}}
	ctrl.Update(c2)
	if w2.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 on bad body, got %d", w2.Code)
	}

	// Service validation error (400) propagates.
	ctrlV := NewDepartmentController(nil, &stubDeptSvc{err: errValidation})
	c3, w3 := newTestContext(t, http.MethodPatch, "/departments/x", strings.NewReader(`{"name":"Eng"}`))
	c3.Params = gin.Params{{Key: "id", Value: uuid.New().String()}}
	ctrlV.Update(c3)
	if w3.Code != http.StatusBadRequest || !strings.Contains(w3.Body.String(), "VALIDATION_ERROR") {
		t.Fatalf("expected 400 VALIDATION_ERROR, got %d %s", w3.Code, w3.Body.String())
	}

	// Missing tenant → 403.
	w4 := httptestRecorder()
	c4, _ := gin.CreateTestContext(w4)
	c4.Request = httptestRequest(http.MethodPatch, "/departments/x", strings.NewReader(`{"name":"Eng"}`))
	c4.Params = gin.Params{{Key: "id", Value: uuid.New().String()}}
	ctrl.Update(c4)
	if w4.Code != http.StatusForbidden {
		t.Fatalf("expected 403 without tenant, got %d", w4.Code)
	}
}

func TestTeamController_UpdateAndDeactivateErrors(t *testing.T) {
	ctrl := NewTeamController(nil, &stubTeamSvc{res: &dto.TeamResponse{ID: uuid.New().String(), Name: "A", Status: "active"}})
	c, w := newTestContext(t, http.MethodPatch, "/teams/x", strings.NewReader(`{"name":"A"}`))
	c.Params = gin.Params{{Key: "id", Value: uuid.New().String()}}
	ctrl.Update(c)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	// Conflict from service.
	ctrlC := NewTeamController(nil, &stubTeamSvc{err: errConflict})
	c2, w2 := newTestContext(t, http.MethodPatch, "/teams/x", strings.NewReader(`{"name":"A"}`))
	c2.Params = gin.Params{{Key: "id", Value: uuid.New().String()}}
	ctrlC.Update(c2)
	if w2.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", w2.Code)
	}

	// Deactivate conflict (active mappings) → 409.
	c3, w3 := newTestContext(t, http.MethodPost, "/teams/x/deactivate", nil)
	c3.Params = gin.Params{{Key: "id", Value: uuid.New().String()}}
	ctrlC.Deactivate(c3)
	if w3.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", w3.Code)
	}
}

func TestDesignationController_UpdatePath(t *testing.T) {
	ctrl := NewDesignationController(nil, &stubDesigSvc{res: &dto.DesignationResponse{ID: uuid.New().String(), Title: "Lead", Status: "active"}})
	c, w := newTestContext(t, http.MethodPatch, "/designations/x", strings.NewReader(`{"title":"Lead"}`))
	c.Params = gin.Params{{Key: "id", Value: uuid.New().String()}}
	ctrl.Update(c)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	// Bad UUID → 400.
	c2, w2 := newTestContext(t, http.MethodPatch, "/designations/bad", nil)
	c2.Params = gin.Params{{Key: "id", Value: "nope"}}
	ctrl.Update(c2)
	if w2.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 on bad uuid, got %d", w2.Code)
	}

	// Missing tenant.
	w3 := httptestRecorder()
	c3, _ := gin.CreateTestContext(w3)
	c3.Request = httptestRequest(http.MethodPatch, "/designations/x", strings.NewReader(`{"title":"Lead"}`))
	c3.Params = gin.Params{{Key: "id", Value: uuid.New().String()}}
	ctrl.Update(c3)
	if w3.Code != http.StatusForbidden {
		t.Fatalf("expected 403 without tenant, got %d", w3.Code)
	}
}

func TestControllers_ValidationEnvelope(t *testing.T) {
	// Missing required name → binding error path through respondError (non-AppError guard exercised by status).
	ctrl := NewDepartmentController(nil, &stubDeptSvc{})
	c, w := newTestContext(t, http.MethodPost, "/departments", strings.NewReader(`{}`))
	ctrl.Create(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 on missing name, got %d", w.Code)
	}
}

func TestRespondErrorFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptestRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptestRequest(http.MethodGet, "/x", nil)
	respondError(c, errOpaque)
	if w.Code != http.StatusInternalServerError || !strings.Contains(w.Body.String(), "INTERNAL") {
		t.Fatalf("expected 500 INTERNAL envelope, got %d %s", w.Code, w.Body.String())
	}

	w2 := httptestRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Request = httptestRequest(http.MethodGet, "/x", nil)
	respondValidationError(c2, validationDetails())
	if w2.Code != http.StatusBadRequest || !strings.Contains(w2.Body.String(), `"details"`) {
		t.Fatalf("expected 400 with details, got %d %s", w2.Code, w2.Body.String())
	}
}

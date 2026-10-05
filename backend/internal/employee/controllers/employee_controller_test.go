package controllers

import (
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/employee/dto"
)

func TestEmployeeController_Endpoints(t *testing.T) {
	empID := uuid.New()
	userID := uuid.New()

	empRes := &dto.EmployeeResponse{
		ID:           empID,
		UserID:       userID,
		EmployeeCode: "EMP-001",
		FirstName:    "Jane",
		LastName:     "Doe",
		Status:       "active",
		Contact: &dto.EmployeeContact{
			CurrentAddress: "123 Main St",
		},
	}
	stubSvc := &stubEmpSvc{
		res:   empRes,
		list:  []dto.EmployeeResponse{*empRes},
		total: 1,
	}
	ctrl := NewEmployeeController(stubSvc)

	// 1. Create
	c, w := newTestContext(t, http.MethodPost, "/employees", strings.NewReader(`{
		"user_id":"`+userID.String()+`",
		"employee_code":"EMP-001",
		"first_name":"Jane",
		"last_name":"Doe",
		"employment_type":"full_time",
		"joining_date":"2026-01-01T00:00:00Z"
	}`))
	ctrl.Create(c)
	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	// 2. GetByID
	c, w = newTestContext(t, http.MethodGet, "/employees/"+empID.String(), nil)
	c.Params = gin.Params{{Key: "id", Value: empID.String()}}
	ctrl.GetByID(c)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	// 3. GetMe
	c, w = newTestContext(t, http.MethodGet, "/employees/me", nil)
	c.Set("user_id", userID)
	ctrl.GetMe(c)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	// 4. UpdateMe
	c, w = newTestContext(t, http.MethodPatch, "/employees/me", strings.NewReader(`{"current_address":"456 Pine St"}`))
	c.Set("user_id", userID)
	ctrl.UpdateMe(c)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	// 5. List
	c, w = newTestContext(t, http.MethodGet, "/employees?page=1&per_page=10", nil)
	ctrl.List(c)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	// 6. Update
	c, w = newTestContext(t, http.MethodPatch, "/employees/"+empID.String(), strings.NewReader(`{"first_name":"Janet"}`))
	c.Params = gin.Params{{Key: "id", Value: empID.String()}}
	ctrl.Update(c)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	// 7. TransitionStatus
	c, w = newTestContext(t, http.MethodPost, "/employees/"+empID.String()+"/status", strings.NewReader(`{"status":"notice"}`))
	c.Params = gin.Params{{Key: "id", Value: empID.String()}}
	ctrl.TransitionStatus(c)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	// 8. Deactivate
	c, w = newTestContext(t, http.MethodPost, "/employees/"+empID.String()+"/deactivate", nil)
	c.Params = gin.Params{{Key: "id", Value: empID.String()}}
	ctrl.Deactivate(c)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

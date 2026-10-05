package controllers

import (
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/employee/dto"
)

func TestStatutoryController_Endpoints(t *testing.T) {
	empID := uuid.New()
	statRes := &dto.StatutoryResponse{
		ID:                uuid.New(),
		EmployeeProfileID: empID,
		TaxID:             "****1234",
		BankAccountNumber: "****5678",
	}
	stubSvc := &stubStatSvc{res: statRes}
	ctrl := NewStatutoryController(stubSvc)

	// 1. Get
	c, w := newTestContext(t, http.MethodGet, "/employees/"+empID.String()+"/statutory", nil)
	c.Params = gin.Params{{Key: "id", Value: empID.String()}}
	ctrl.Get(c)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	// 2. Upsert
	c, w = newTestContext(t, http.MethodPut, "/employees/"+empID.String()+"/statutory", strings.NewReader(`{
		"tax_id":"TAX-123",
		"bank_name":"Bank",
		"bank_account_number":"123456789"
	}`))
	c.Params = gin.Params{{Key: "id", Value: empID.String()}}
	ctrl.Upsert(c)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

package controllers

import (
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/employee/dto"
)

func TestDocumentController_Endpoints(t *testing.T) {
	empID := uuid.New()
	docID := uuid.New()
	docRes := &dto.DocumentResponse{
		ID:                docID,
		EmployeeProfileID: empID,
		FileName:          "contract.pdf",
		FileURL:           "https://storage.jaas.com/1.pdf",
	}
	stubSvc := &stubDocSvc{
		res:  docRes,
		list: []dto.DocumentResponse{*docRes},
	}
	ctrl := NewDocumentController(stubSvc)

	// 1. Upload
	c, w := newTestContext(t, http.MethodPost, "/employees/"+empID.String()+"/documents", strings.NewReader(`{
		"document_type":"contract",
		"file_name":"contract.pdf",
		"file_url":"https://storage.jaas.com/1.pdf",
		"file_size":1024,
		"mime_type":"application/pdf"
	}`))
	c.Params = gin.Params{{Key: "id", Value: empID.String()}}
	ctrl.Upload(c)
	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	// 2. List
	c, w = newTestContext(t, http.MethodGet, "/employees/"+empID.String()+"/documents", nil)
	c.Params = gin.Params{{Key: "id", Value: empID.String()}}
	ctrl.List(c)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	// 3. Verify
	c, w = newTestContext(t, http.MethodPost, "/employees/"+empID.String()+"/documents/"+docID.String()+"/verify", nil)
	c.Params = gin.Params{{Key: "id", Value: empID.String()}, {Key: "doc_id", Value: docID.String()}}
	ctrl.Verify(c)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

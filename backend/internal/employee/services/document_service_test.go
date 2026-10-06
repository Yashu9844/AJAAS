package services_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/employee/dto"
	"github.com/jaas/jaas/internal/employee/models"
	"github.com/jaas/jaas/internal/employee/services"
)

func TestEmployeeDocumentService(t *testing.T) {
	tenantID := uuid.New()
	profileID := uuid.New()
	verifierID := uuid.New()
	ctx := context.Background()

	pRepo := newMockProfileRepo()
	pRepo.byID[profileID] = &models.EmployeeProfile{EmployeeCode: "EMP-001"}
	pRepo.byID[profileID].ID = profileID
	pRepo.byID[profileID].TenantID = tenantID

	dRepo := newMockDocumentRepo()
	pub := &mockPublisher{}
	svc := services.NewEmployeeDocumentService(dRepo, pRepo, pub)

	// 1. Upload
	req := dto.UploadDocumentRequest{
		DocumentType: "contract",
		FileName:     "employment_contract.pdf",
		FileURL:      "https://storage.jaas.com/docs/123.pdf",
		FileSize:     2048,
		MimeType:     "application/pdf",
	}
	doc, err := svc.Upload(ctx, tenantID, profileID, req)
	if err != nil {
		t.Fatalf("unexpected error uploading doc: %v", err)
	}
	if doc.FileName != "employment_contract.pdf" {
		t.Errorf("expected employment_contract.pdf, got %s", doc.FileName)
	}

	// 2. List
	docs, err := svc.List(ctx, tenantID, profileID)
	if err != nil {
		t.Fatalf("unexpected error listing docs: %v", err)
	}
	if len(docs) != 1 {
		t.Errorf("expected 1 doc, got %d", len(docs))
	}

	// 3. Verify
	// 3a. a document cannot be verified through another employee's path
	if _, err := svc.Verify(ctx, tenantID, uuid.New(), doc.ID, verifierID); err == nil {
		t.Fatal("verify through a different employee must be refused")
	}
	verified, err := svc.Verify(ctx, tenantID, profileID, doc.ID, verifierID)
	if err != nil {
		t.Fatalf("unexpected error verifying doc: %v", err)
	}
	if verified.VerifiedAt == nil || *verified.VerifiedBy != verifierID {
		t.Errorf("expected verification timestamp and verifier ID")
	}
}

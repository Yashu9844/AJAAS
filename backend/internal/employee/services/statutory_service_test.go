package services_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/employee/dto"
	"github.com/jaas/jaas/internal/employee/models"
	"github.com/jaas/jaas/internal/employee/services"
)

func TestEmployeeStatutoryService(t *testing.T) {
	tenantID := uuid.New()
	profileID := uuid.New()
	ctx := context.Background()

	pRepo := newMockProfileRepo()
	pRepo.byID[profileID] = &models.EmployeeProfile{
		EmployeeCode: "EMP-001",
	}
	pRepo.byID[profileID].ID = profileID
	pRepo.byID[profileID].TenantID = tenantID

	sRepo := newMockStatutoryRepo()
	svc := services.NewEmployeeStatutoryService(sRepo, pRepo)

	// 1. Upsert Statutory
	req := dto.UpdateStatutoryRequest{
		TaxID:             "TAX-12345678",
		NationalID:        "NAT-98765432",
		BankName:          "Global Bank",
		BankAccountNumber: "9988776655",
		BankRoutingSwift:  "GLOBUS33",
	}
	upserted, err := svc.Upsert(ctx, tenantID, profileID, req)
	if err != nil {
		t.Fatalf("unexpected error upserting: %v", err)
	}
	if upserted.TaxID != "TAX-12345678" {
		t.Errorf("expected TAX-12345678, got %s", upserted.TaxID)
	}

	// 2. Get Sensitive (Unmasked)
	sensitive, err := svc.GetByProfileID(ctx, tenantID, profileID, true)
	if err != nil {
		t.Fatalf("unexpected error getting sensitive statutory: %v", err)
	}
	if sensitive.BankAccountNumber != "9988776655" {
		t.Errorf("expected unmasked bank account, got %s", sensitive.BankAccountNumber)
	}

	// 3. Get Masked
	masked, err := svc.GetByProfileID(ctx, tenantID, profileID, false)
	if err != nil {
		t.Fatalf("unexpected error getting masked statutory: %v", err)
	}
	if masked.BankAccountNumber != "****6655" {
		t.Errorf("expected ****6655, got %s", masked.BankAccountNumber)
	}
	if masked.TaxID != "****5678" {
		t.Errorf("expected ****5678, got %s", masked.TaxID)
	}
}

package models_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/employee/models"
)

func TestEmployeeModelsInstantiation(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()
	now := time.Now()

	profile := models.EmployeeProfile{
		UserID:       userID,
		EmployeeCode: "EMP-001",
		FirstName:    "Jane",
		LastName:     "Doe",
		DisplayName:  "Jane Doe",
		Gender:       "female",
		DateOfBirth:  &now,
		Status:       "active",
	}
	profile.TenantID = tenantID
	profile.ID = uuid.New()

	if profile.TableName() != "employee_profiles" {
		t.Errorf("expected employee_profiles, got %s", profile.TableName())
	}

	empDetail := models.EmploymentDetail{
		EmployeeProfileID: profile.ID,
		EmploymentType:    "full_time",
		JoiningDate:       now,
		NoticePeriodDays:  30,
	}
	empDetail.TenantID = tenantID
	if empDetail.TableName() != "employment_details" {
		t.Errorf("expected employment_details, got %s", empDetail.TableName())
	}

	contact := models.EmployeeContact{
		EmployeeProfileID: profile.ID,
		PersonalEmail:     "jane@example.com",
		WorkPhone:         "555-0199",
	}
	contact.TenantID = tenantID
	if contact.TableName() != "employee_contacts" {
		t.Errorf("expected employee_contacts, got %s", contact.TableName())
	}

	statutory := models.EmployeeStatutory{
		EmployeeProfileID: profile.ID,
		TaxID:             "TAX-9988",
		NationalID:        "NAT-1122",
		BankName:          "First Bank",
		BankAccountNumber: "123456789",
	}
	statutory.TenantID = tenantID
	if statutory.TableName() != "employee_statutory" {
		t.Errorf("expected employee_statutory, got %s", statutory.TableName())
	}

	doc := models.EmployeeDocument{
		EmployeeProfileID: profile.ID,
		DocumentType:      "contract",
		FileName:          "offer.pdf",
		FileURL:           "https://storage.jaas.com/docs/offer.pdf",
		FileSize:          1024,
		MimeType:          "application/pdf",
	}
	doc.TenantID = tenantID
	if doc.TableName() != "employee_documents" {
		t.Errorf("expected employee_documents, got %s", doc.TableName())
	}

	tl := models.EmployeeTimeline{
		EmployeeProfileID: profile.ID,
		EventType:         "hired",
		EffectiveDate:     now,
		Notes:             "Onboarded as Senior Engineer",
	}
	tl.TenantID = tenantID
	if tl.TableName() != "employee_timelines" {
		t.Errorf("expected employee_timelines, got %s", tl.TableName())
	}

	outbox := models.EmployeeEventsOutbox{
		EventID:    uuid.New(),
		EventType:  "employee.created",
		RoutingKey: "employee.profile.created",
		Payload:    "{}",
		Published:  false,
	}
	outbox.TenantID = tenantID
	if outbox.TableName() != "employee_events_outbox" {
		t.Errorf("expected employee_events_outbox, got %s", outbox.TableName())
	}
}

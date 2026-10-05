package services_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/employee/dto"
	"github.com/jaas/jaas/internal/employee/services"
	identityDto "github.com/jaas/jaas/internal/identity/dto"
)

func TestEmployeeServiceLifecycle(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()
	ctx := context.Background()

	pRepo := newMockProfileRepo()
	tRepo := newMockTimelineRepo()
	uSvc := newMockUserService()
	uSvc.users[userID] = &identityDto.UserResponse{
		ID:    userID.String(),
		Email: "emp@example.com",
	}
	pub := &mockPublisher{}

	svc := services.NewEmployeeService(nil, pRepo, tRepo, uSvc, nil, pub)

	now := time.Now().UTC()
	req := dto.CreateEmployeeRequest{
		UserID:           userID,
		EmployeeCode:     "EMP-001",
		FirstName:        "Jane",
		LastName:         "Doe",
		EmploymentType:   "full_time",
		JoiningDate:      now,
		NoticePeriodDays: 30,
		PersonalEmail:    "jane.doe@example.com",
		PersonalPhone:    "555-1234",
	}

	// 1. Create Employee
	created, err := svc.CreateEmployee(ctx, tenantID, req)
	if err != nil {
		t.Fatalf("unexpected error creating employee: %v", err)
	}
	if created.EmployeeCode != "EMP-001" {
		t.Errorf("expected EMP-001, got %s", created.EmployeeCode)
	}
	if created.Status != "active" {
		t.Errorf("expected active status, got %s", created.Status)
	}

	// 2. Duplicate Code Block
	_, err = svc.CreateEmployee(ctx, tenantID, req)
	if err == nil {
		t.Errorf("expected conflict on duplicate employee code")
	}

	// 3. Get By ID
	fetched, err := svc.GetByID(ctx, tenantID, created.ID)
	if err != nil {
		t.Fatalf("unexpected error getting by ID: %v", err)
	}
	if fetched.FirstName != "Jane" {
		t.Errorf("expected Jane, got %s", fetched.FirstName)
	}

	// 4. Get By User ID
	fetchedUser, err := svc.GetByUserID(ctx, tenantID, userID)
	if err != nil {
		t.Fatalf("unexpected error getting by user ID: %v", err)
	}
	if fetchedUser.ID != created.ID {
		t.Errorf("expected %s, got %s", created.ID, fetchedUser.ID)
	}

	// 5. Update Profile
	newLast := "Smith"
	updated, err := svc.Update(ctx, tenantID, created.ID, dto.UpdateEmployeeRequest{
		LastName: &newLast,
	})
	if err != nil {
		t.Fatalf("unexpected error updating: %v", err)
	}
	if updated.LastName != "Smith" {
		t.Errorf("expected Smith, got %s", updated.LastName)
	}

	// 6. Update Self Contact
	newAddr := "123 Main St"
	selfUpdated, err := svc.UpdateSelfContact(ctx, tenantID, userID, dto.UpdateSelfContactRequest{
		CurrentAddress: &newAddr,
	})
	if err != nil {
		t.Fatalf("unexpected error self updating: %v", err)
	}
	if selfUpdated.Contact.CurrentAddress != "123 Main St" {
		t.Errorf("expected 123 Main St, got %s", selfUpdated.Contact.CurrentAddress)
	}

	// 7. Transition Status
	notes := "Promoted out of probation"
	statusUpdated, err := svc.TransitionStatus(ctx, tenantID, created.ID, dto.TransitionStatusRequest{
		Status: "active",
		Notes:  notes,
	})
	if err != nil {
		t.Fatalf("unexpected error transitioning status: %v", err)
	}
	if statusUpdated.Status != "active" {
		t.Errorf("expected active, got %s", statusUpdated.Status)
	}

	// 8. List
	list, total, err := svc.List(ctx, tenantID, dto.EmployeeFilter{Page: 1, PerPage: 10})
	if err != nil {
		t.Fatalf("unexpected error listing: %v", err)
	}
	if total != 1 || len(list) != 1 {
		t.Errorf("expected 1 employee in list, got %d", total)
	}

	// 9. Deactivate
	err = svc.Deactivate(ctx, tenantID, created.ID)
	if err != nil {
		t.Fatalf("unexpected error deactivating: %v", err)
	}
	deactivated, _ := svc.GetByID(ctx, tenantID, created.ID)
	if deactivated.Status != "inactive" {
		t.Errorf("expected inactive status, got %s", deactivated.Status)
	}
}

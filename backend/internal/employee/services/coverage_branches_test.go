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

func TestEmployeeServiceEdgeCasesAndBranches(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()
	ctx := context.Background()

	pRepo := newMockProfileRepo()
	tRepo := newMockTimelineRepo()
	uSvc := newMockUserService()
	pub := &mockPublisher{}

	svc := services.NewEmployeeService(nil, pRepo, tRepo, uSvc, nil, pub)

	// 1. Invalid employee code error
	_, err := svc.CreateEmployee(ctx, tenantID, dto.CreateEmployeeRequest{
		UserID:         userID,
		EmployeeCode:   "x",
		FirstName:      "Jane",
		LastName:       "Doe",
		EmploymentType: "full_time",
		JoiningDate:    time.Now().UTC(),
	})
	if err == nil {
		t.Errorf("expected validation error on short employee code")
	}

	// 2. Invalid employment type error
	_, err = svc.CreateEmployee(ctx, tenantID, dto.CreateEmployeeRequest{
		UserID:         userID,
		EmployeeCode:   "EMP-002",
		FirstName:      "Jane",
		LastName:       "Doe",
		EmploymentType: "freelancer",
		JoiningDate:    time.Now().UTC(),
	})
	if err == nil {
		t.Errorf("expected validation error on invalid employment type")
	}

	// 3. User not found error
	_, err = svc.CreateEmployee(ctx, tenantID, dto.CreateEmployeeRequest{
		UserID:         userID,
		EmployeeCode:   "EMP-002",
		FirstName:      "Jane",
		LastName:       "Doe",
		EmploymentType: "full_time",
		JoiningDate:    time.Now().UTC(),
	})
	if err == nil {
		t.Errorf("expected not found error for non-existent user")
	}

	// Add user and create profile
	uSvc.users[userID] = &identityDto.UserResponse{ID: userID.String(), Email: "user@example.com"}
	probationEnd := time.Now().UTC().Add(30 * 24 * time.Hour)
	prof, err := svc.CreateEmployee(ctx, tenantID, dto.CreateEmployeeRequest{
		UserID:           userID,
		EmployeeCode:     "EMP-002",
		FirstName:        "Jane",
		LastName:         "Doe",
		EmploymentType:   "full_time",
		JoiningDate:      time.Now().UTC(),
		ProbationEndDate: &probationEnd,
	})
	if err != nil {
		t.Fatalf("unexpected error creating employee: %v", err)
	}
	if prof.Status != "probation" {
		t.Errorf("expected probation status, got %s", prof.Status)
	}

	// 4. Duplicate User Profile error
	_, err = svc.CreateEmployee(ctx, tenantID, dto.CreateEmployeeRequest{
		UserID:         userID,
		EmployeeCode:   "EMP-003",
		FirstName:      "Jane",
		LastName:       "Doe",
		EmploymentType: "full_time",
		JoiningDate:    time.Now().UTC(),
	})
	if err == nil {
		t.Errorf("expected conflict on duplicate user profile")
	}

	// 5. GetByID / GetByUserID not found
	_, err = svc.GetByID(ctx, tenantID, uuid.New())
	if err == nil {
		t.Errorf("expected not found error")
	}
	_, err = svc.GetByUserID(ctx, tenantID, uuid.New())
	if err == nil {
		t.Errorf("expected not found error")
	}

	// 6. Update not found
	_, err = svc.Update(ctx, tenantID, uuid.New(), dto.UpdateEmployeeRequest{})
	if err == nil {
		t.Errorf("expected not found error")
	}

	// 7. Update all fields branch test
	newFirst := "Janet"
	newDisplay := "Janet Doe"
	newGender := "female"
	dob := time.Date(1990, 1, 1, 0, 0, 0, 0, time.UTC)
	marital := "single"
	blood := "O+"
	avatar := "https://avatar.com/janet.png"
	updated, err := svc.Update(ctx, tenantID, prof.ID, dto.UpdateEmployeeRequest{
		FirstName:     &newFirst,
		DisplayName:   &newDisplay,
		Gender:        &newGender,
		DateOfBirth:   &dob,
		MaritalStatus: &marital,
		BloodGroup:    &blood,
		AvatarURL:     &avatar,
	})
	if err != nil {
		t.Fatalf("unexpected error updating: %v", err)
	}
	if updated.FirstName != "Janet" || updated.BloodGroup != "O+" {
		t.Errorf("expected updated fields")
	}

	// 8. UpdateSelfContact with nil existing contact and all fields
	u2ID := uuid.New()
	uSvc.users[u2ID] = &identityDto.UserResponse{ID: u2ID.String(), Email: "u2@example.com"}
	prof2, _ := svc.CreateEmployee(ctx, tenantID, dto.CreateEmployeeRequest{
		UserID:         u2ID,
		EmployeeCode:   "EMP-004",
		FirstName:      "Bob",
		LastName:       "Ross",
		EmploymentType: "contract",
		JoiningDate:    time.Now().UTC(),
	})
	pRepo.byID[prof2.ID].Contact = nil

	pPhone := "555-9999"
	currAddr := "456 Oak St"
	permAddr := "789 Pine St"
	emrg := `[{"name":"Alice","phone":"555-0000"}]`
	selfUp, err := svc.UpdateSelfContact(ctx, tenantID, u2ID, dto.UpdateSelfContactRequest{
		PersonalPhone:     &pPhone,
		CurrentAddress:    &currAddr,
		PermanentAddress:  &permAddr,
		EmergencyContacts: &emrg,
	})
	if err != nil {
		t.Fatalf("unexpected error updating self contact: %v", err)
	}
	if selfUp.Contact.PersonalPhone != "555-9999" {
		t.Errorf("expected 555-9999, got %s", selfUp.Contact.PersonalPhone)
	}

	// 9. Transition status validation error & not found error & same status
	_, err = svc.TransitionStatus(ctx, tenantID, prof.ID, dto.TransitionStatusRequest{Status: "invalid_status"})
	if err == nil {
		t.Errorf("expected validation error on status transition")
	}
	_, err = svc.TransitionStatus(ctx, tenantID, uuid.New(), dto.TransitionStatusRequest{Status: "active"})
	if err == nil {
		t.Errorf("expected not found on status transition")
	}
	sameStatus, err := svc.TransitionStatus(ctx, tenantID, prof.ID, dto.TransitionStatusRequest{Status: "probation"})
	if err != nil || sameStatus.Status != "probation" {
		t.Errorf("expected no-op on same status")
	}

	// 10. Transition status with all exit dates
	now := time.Now().UTC()
	confDate := now.Add(-10 * 24 * time.Hour)
	resigDate := now
	exitDate := now.Add(30 * 24 * time.Hour)
	exitReason := "Career growth"
	trans, err := svc.TransitionStatus(ctx, tenantID, prof.ID, dto.TransitionStatusRequest{
		Status:           "notice",
		ConfirmationDate: &confDate,
		ResignationDate:  &resigDate,
		ExitDate:         &exitDate,
		ExitReason:       exitReason,
		Notes:            "Serving 30-day notice",
	})
	if err != nil {
		t.Fatalf("unexpected error on notice transition: %v", err)
	}
	if trans.Status != "notice" {
		t.Errorf("expected notice status, got %s", trans.Status)
	}

	// 11. Deactivate not found error
	if err := svc.Deactivate(ctx, tenantID, uuid.New()); err == nil {
		t.Errorf("expected not found on deactivate non-existent employee")
	}
}

func TestStatutoryAndDocumentAndTimelineEdgeCases(t *testing.T) {
	tenantID := uuid.New()
	ctx := context.Background()

	pRepo := newMockProfileRepo()
	sRepo := newMockStatutoryRepo()
	dRepo := newMockDocumentRepo()
	tRepo := newMockTimelineRepo()
	pub := &mockPublisher{}

	statSvc := services.NewEmployeeStatutoryService(sRepo, pRepo)
	docSvc := services.NewEmployeeDocumentService(dRepo, pRepo, pub)
	tlSvc := services.NewEmployeeTimelineService(tRepo, pRepo)

	// Statutory not found
	_, err := statSvc.GetByProfileID(ctx, tenantID, uuid.New(), false)
	if err == nil {
		t.Errorf("expected not found error")
	}
	_, err = statSvc.Upsert(ctx, tenantID, uuid.New(), dto.UpdateStatutoryRequest{})
	if err == nil {
		t.Errorf("expected not found error for non-existent profile")
	}

	// Doc not found
	_, err = docSvc.Upload(ctx, tenantID, uuid.New(), dto.UploadDocumentRequest{})
	if err == nil {
		t.Errorf("expected not found on doc upload")
	}
	_, err = docSvc.Verify(ctx, tenantID, uuid.New(), uuid.New(), uuid.New())
	if err == nil {
		t.Errorf("expected not found on doc verify")
	}

	// Timeline not found
	_, err = tlSvc.List(ctx, tenantID, uuid.New())
	if err == nil {
		t.Errorf("expected not found on timeline list")
	}
}

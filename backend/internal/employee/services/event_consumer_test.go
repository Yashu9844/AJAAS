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

func TestEventConsumerHandleUserDeactivated(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()
	ctx := context.Background()

	pRepo := newMockProfileRepo()
	tRepo := newMockTimelineRepo()
	uSvc := newMockUserService()
	uSvc.users[userID] = &identityDto.UserResponse{
		ID:    userID.String(),
		Email: "deact@example.com",
	}
	pub := &mockPublisher{}

	empSvc := services.NewEmployeeService(nil, pRepo, tRepo, uSvc, nil, pub)

	// Create employee
	now := time.Now().UTC()
	_, err := empSvc.CreateEmployee(ctx, tenantID, dto.CreateEmployeeRequest{
		UserID:           userID,
		EmployeeCode:     "EMP-999",
		FirstName:        "Bob",
		LastName:         "Marley",
		EmploymentType:   "full_time",
		JoiningDate:      now,
		NoticePeriodDays: 30,
	})
	if err != nil {
		t.Fatalf("unexpected error creating employee: %v", err)
	}

	consumer := services.NewEventConsumer(empSvc, nil)

	// Consume user deactivation
	err = consumer.HandleUserDeactivated(ctx, tenantID, userID)
	if err != nil {
		t.Fatalf("unexpected error handling user deactivation: %v", err)
	}

	profile, err := empSvc.GetByUserID(ctx, tenantID, userID)
	if err != nil {
		t.Fatalf("unexpected error getting profile: %v", err)
	}
	if profile.Status != "inactive" {
		t.Errorf("expected inactive status, got %s", profile.Status)
	}
}

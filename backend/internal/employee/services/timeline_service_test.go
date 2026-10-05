package services_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/employee/models"
	"github.com/jaas/jaas/internal/employee/services"
)

func TestEmployeeTimelineService(t *testing.T) {
	tenantID := uuid.New()
	profileID := uuid.New()
	ctx := context.Background()

	pRepo := newMockProfileRepo()
	pRepo.byID[profileID] = &models.EmployeeProfile{EmployeeCode: "EMP-001"}
	pRepo.byID[profileID].ID = profileID
	pRepo.byID[profileID].TenantID = tenantID

	tRepo := newMockTimelineRepo()
	tRepo.timelines[profileID] = []models.EmployeeTimeline{
		{
			EmployeeProfileID: profileID,
			EventType:         "hired",
			EffectiveDate:     time.Now().UTC(),
			Notes:             "Initial hire",
		},
	}

	svc := services.NewEmployeeTimelineService(tRepo, pRepo)

	// 1. List
	list, err := svc.List(ctx, tenantID, profileID)
	if err != nil {
		t.Fatalf("unexpected error listing timeline: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("expected 1 timeline event, got %d", len(list))
	}
	if list[0].EventType != "hired" {
		t.Errorf("expected hired, got %s", list[0].EventType)
	}
}

package services

import (
	"context"
	"io"
	"testing"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/identity/models"
	"github.com/rs/zerolog"
	"gorm.io/gorm"
)

func TestAuditService_Log(t *testing.T) {
	auditRepo := &MockAuditLogRepository{}
	logger := zerolog.New(io.Discard)
	svc := NewAuditService(auditRepo, &logger)
	ctx := context.Background()

	tenantID := uuid.New().String()
	userID := uuid.New().String()

	var createdLog *models.AuditLog
	auditRepo.CreateFunc = func(ctx context.Context, tx *gorm.DB, log *models.AuditLog) error {
		createdLog = log
		return nil
	}

	metadata := map[string]string{"foo": "bar"}
	err := svc.Log(ctx, nil, tenantID, userID, "user.created", "user", "res-123", metadata, "127.0.0.1", "Chrome")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if createdLog == nil {
		t.Fatal("expected audit log to be created")
	}
	if createdLog.Action != "user.created" || createdLog.Resource != "user" {
		t.Errorf("unexpected audit log values: %+v", createdLog)
	}
}

package services

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/identity/models"
	"github.com/jaas/jaas/internal/identity/repositories"
	"github.com/rs/zerolog"
	"gorm.io/gorm"
)

// AuditService defines the interface to write immutable security logs.
type AuditService interface {
	Log(ctx context.Context, tx *gorm.DB, tenantID, userID string, action, resource, resourceID string, metadata interface{}, ipAddress, userAgent string) error
}

type auditService struct {
	auditRepo repositories.AuditLogRepository
	logger    *zerolog.Logger
}

// NewAuditService creates a new AuditService instance.
func NewAuditService(auditRepo repositories.AuditLogRepository, logger *zerolog.Logger) AuditService {
	return &auditService{
		auditRepo: auditRepo,
		logger:    logger,
	}
}

func (s *auditService) Log(ctx context.Context, tx *gorm.DB, tenantID, userID string, action, resource, resourceID string, metadata interface{}, ipAddress, userAgent string) error {
	tID, err := uuid.Parse(tenantID)
	if err != nil {
		s.logger.Warn().Msgf("Failed to parse TenantID for audit logging: %s", tenantID)
		return err
	}

	var uID *uuid.UUID
	if userID != "" {
		parsedUID, err := uuid.Parse(userID)
		if err == nil {
			uID = &parsedUID
		}
	}

	var metaRaw json.RawMessage
	if metadata != nil {
		bytes, err := json.Marshal(metadata)
		if err == nil {
			metaRaw = json.RawMessage(bytes)
		}
	}

	var ip, ua *string
	if ipAddress != "" {
		ip = &ipAddress
	}
	if userAgent != "" {
		ua = &userAgent
	}

	var resID *string
	if resourceID != "" {
		resID = &resourceID
	}

	log := &models.AuditLog{
		TenantID:   tID,
		UserID:     uID,
		Action:     action,
		Resource:   resource,
		ResourceID: resID,
		Metadata:   metaRaw,
		IPAddress:  ip,
		UserAgent:  ua,
	}

	err = s.auditRepo.Create(ctx, tx, log)
	if err != nil {
		s.logger.Error().Err(err).Msgf("Failed to persist audit log for action: %s", action)
		return err
	}

	return nil
}

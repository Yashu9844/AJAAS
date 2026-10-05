package services

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/employee/dto"
	"github.com/rs/zerolog"
)

type EventConsumer interface {
	HandleUserDeactivated(ctx context.Context, tenantID, userID uuid.UUID) error
}

type eventConsumer struct {
	empSvc EmployeeService
	logger *zerolog.Logger
}

func NewEventConsumer(empSvc EmployeeService, logger *zerolog.Logger) EventConsumer {
	return &eventConsumer{
		empSvc: empSvc,
		logger: logger,
	}
}

func (c *eventConsumer) HandleUserDeactivated(ctx context.Context, tenantID, userID uuid.UUID) error {
	profile, err := c.empSvc.GetByUserID(ctx, tenantID, userID)
	if err != nil || profile == nil {
		if c.logger != nil {
			c.logger.Debug().Msgf("no employee profile found for deactivated user %s", userID)
		}
		return nil
	}

	exitDate := time.Now().UTC()
	_, err = c.empSvc.TransitionStatus(ctx, tenantID, profile.ID, dto.TransitionStatusRequest{
		Status:     "inactive",
		ExitDate:   &exitDate,
		ExitReason: "Identity user account deactivated",
		Notes:      "Automatic status convergence on identity.user.deactivated",
	})
	return err
}

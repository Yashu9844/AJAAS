package services

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/hr/employee/api"
	"github.com/jaas/jaas/internal/hr/employee/models"
	"github.com/jaas/jaas/internal/hr/employee/repositories"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"gorm.io/gorm"
)

// coreStatuses are the statuses HR/Core may set (core DC-07): hire/rehire/confirm/terminate targets plus the statuses a
// rescinded termination restores.
var coreStatuses = map[string]bool{"probation": true, "active": true, "terminated": true, "notice": true, "on_leave": true, "inactive": true}

// CoreLifecycle implements api.Lifecycle: HR/Core projects worker status changes inside its own transaction. It may
// leave final statuses (rehire, rescinded termination), which the manual status matrix forbids.
type CoreLifecycle struct {
	repo repositories.EmployeeProfileRepository
}

// NewCoreLifecycle builds the lifecycle port over the HR database.
func NewCoreLifecycle(db *gorm.DB) *CoreLifecycle {
	return &CoreLifecycle{repo: repositories.NewEmployeeProfileRepository(db)}
}

// ApplyCoreStatus sets the status (and confirmation or exit dates), appends a timeline entry and returns the previous
// status. Setting the current status again only updates the dates.
func (l *CoreLifecycle) ApplyCoreStatus(ctx context.Context, tx *gorm.DB, tenantID, employeeID uuid.UUID, change api.StatusChange) (string, error) {
	if !coreStatuses[change.Status] {
		return "", &sharedErrors.AppError{Code: "VALIDATION_ERROR", Message: "unsupported core status " + change.Status, StatusCode: 400}
	}
	repo := l.repo.WithTx(tx)
	profile, err := repo.GetByID(ctx, tenantID, employeeID)
	if err != nil {
		return "", err
	}
	if profile == nil {
		return "", sharedErrors.ErrNotFound
	}
	previous := profile.Status
	profile.Status = change.Status
	if d := profile.EmploymentDetail; d != nil {
		switch change.Status {
		case "terminated":
			d.ExitDate, d.ExitReason = change.ExitDate, change.ExitReason
		default:
			d.ExitDate, d.ExitReason, d.ResignationDate = nil, "", nil
			if change.ConfirmationDate != nil {
				d.ConfirmationDate = change.ConfirmationDate
			}
		}
		if err := repo.UpdateEmploymentDetail(ctx, tx, d); err != nil {
			return "", err
		}
	}
	if err := repo.Update(ctx, tx, profile); err != nil {
		return "", err
	}
	note := change.Note
	if note == "" {
		note = "HR action: " + previous + " → " + change.Status
	}
	timeline := &models.EmployeeTimeline{EmployeeProfileID: profile.ID, EventType: "status_changed:" + change.Status,
		EffectiveDate: time.Now().UTC(), Notes: note, Metadata: "{}"}
	timeline.TenantID = tenantID
	if err := tx.WithContext(ctx).Create(timeline).Error; err != nil {
		return "", err
	}
	return previous, nil
}

var _ api.Lifecycle = (*CoreLifecycle)(nil)

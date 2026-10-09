package services

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/attendance/models"
	"gorm.io/gorm"
)

// LeaveSync is Module 3's contract C10 for Module 4: it marks or clears `on_leave` day records on the
// caller's transaction, under the Module 3 per-employee lock (AT-022, D3-17).
type LeaveSync interface {
	MarkLeave(ctx context.Context, tx *gorm.DB, tenantID, employeeID uuid.UUID, dates []time.Time) error
	ClearLeave(ctx context.Context, tx *gorm.DB, tenantID, employeeID uuid.UUID, dates []time.Time) error
}

type leaveSync struct{ base }

// NewLeaveSync builds the LeaveSync port.
func NewLeaveSync(d Deps) LeaveSync { return &leaveSync{base{d}} }

// MarkLeave sets each date to on_leave, creating leave-sourced records; punched days keep their status (D4-09).
func (s *leaveSync) MarkLeave(ctx context.Context, tx *gorm.DB, tenantID, employeeID uuid.UUID, dates []time.Time) error {
	if err := s.Repos.Records.LockEmployee(ctx, tx, tenantID, employeeID); err != nil {
		return err
	}
	for _, d := range dates {
		if err := s.markOne(ctx, tx, &models.AttendanceRecord{TenantID: tenantID, EmployeeProfileID: employeeID,
			AttendanceDate: d, Status: models.StatusOnLeave, Source: models.SourceLeave}); err != nil {
			return err
		}
	}
	return nil
}

func (s *leaveSync) markOne(ctx context.Context, tx *gorm.DB, seed *models.AttendanceRecord) error {
	rec, err := s.Repos.Records.FindOrCreate(ctx, tx, seed)
	if err != nil || rec.Status == models.StatusOnLeave {
		return err
	}
	punches, err := s.Repos.Punches.ActiveForRecord(ctx, tx, seed.TenantID, rec.ID)
	if err != nil || len(punches) > 0 {
		return err // a punched day keeps its attendance status (D4-09)
	}
	rec.Status = models.StatusOnLeave
	return s.Repos.Records.Update(ctx, tx, rec)
}

// ClearLeave removes leave-only records and recomputes any other record still marked on_leave.
func (s *leaveSync) ClearLeave(ctx context.Context, tx *gorm.DB, tenantID, employeeID uuid.UUID, dates []time.Time) error {
	if err := s.Repos.Records.LockEmployee(ctx, tx, tenantID, employeeID); err != nil {
		return err
	}
	for _, d := range dates {
		if err := s.clearOne(ctx, tx, tenantID, employeeID, d); err != nil {
			return err
		}
	}
	return nil
}

func (s *leaveSync) clearOne(ctx context.Context, tx *gorm.DB, tenantID, employeeID uuid.UUID, d time.Time) error {
	rec, err := s.Repos.Records.FindByDate(ctx, tx, tenantID, employeeID, d)
	if err != nil || rec == nil || rec.Status != models.StatusOnLeave {
		return err
	}
	punches, err := s.Repos.Punches.ActiveForRecord(ctx, tx, tenantID, rec.ID)
	if err != nil {
		return err
	}
	if len(punches) == 0 && rec.Source == models.SourceLeave {
		return s.Repos.Records.Delete(ctx, tx, tenantID, rec.ID)
	}
	shift, err := s.shiftByID(ctx, tx, tenantID, rec.ShiftID)
	if err != nil {
		return err
	}
	return s.recompute(ctx, tx, rec, shift, s.Now())
}

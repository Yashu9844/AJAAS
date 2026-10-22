package services

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/hr/attendance/models"
	"gorm.io/gorm"
)

// TimeWriter is the write port HR/Attendance's parity package (attendance/ops) uses; every method runs on the caller's
// transaction under the per-employee lock (AT-022).
type TimeWriter interface {
	// SetWindow replaces the day's punches with one in/out pair of the given source and recomputes totals (AT-017).
	SetWindow(ctx context.Context, tx *gorm.DB, tenantID, employeeID uuid.UUID, date, in, out time.Time, source string) error
	// MarkDay creates a punch-less record with the status when the day has none (AP-07); it reports whether it did.
	MarkDay(ctx context.Context, tx *gorm.DB, tenantID, employeeID uuid.UUID, date time.Time, status string) (bool, error)
	// AssignShift appends a shift assignment over [from, to]; overlap is ErrAssignmentOverlap (AT-011).
	AssignShift(ctx context.Context, tx *gorm.DB, tenantID, employeeID, shiftID uuid.UUID, from, to time.Time, actor uuid.UUID) error
	// ShiftState reports whether the shift exists in the tenant and is active.
	ShiftState(ctx context.Context, db *gorm.DB, tenantID, shiftID uuid.UUID) (exists, active bool, err error)
	// HasRecord reports whether the day has any record, and whether it carries punches.
	HasRecord(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID, date time.Time) (exists, punched bool, err error)
}

type timeWriter struct{ base }

// NewTimeWriter builds the TimeWriter port.
func NewTimeWriter(d Deps) TimeWriter { return &timeWriter{base{d}} }

// setWindow is AT-017 without the request bookkeeping: lock, find or create the record, supersede its punches, insert
// the pair, recompute and flag it regularized.
func (b base) setWindow(ctx context.Context, tx *gorm.DB, tenantID, employeeID uuid.UUID, date, in, out time.Time, source string) (*models.AttendanceRecord, error) {
	if err := b.Repos.Records.LockEmployee(ctx, tx, tenantID, employeeID); err != nil {
		return nil, err
	}
	shift, err := b.shiftOn(ctx, tx, tenantID, employeeID, date)
	if err != nil {
		return nil, err
	}
	seed := &models.AttendanceRecord{TenantID: tenantID, EmployeeProfileID: employeeID, AttendanceDate: date, Status: models.StatusAbsent, Source: source}
	if shift != nil {
		seed.ShiftID = &shift.ID
	}
	rec, err := b.Repos.Records.FindOrCreate(ctx, tx, seed)
	if err != nil {
		return nil, err
	}
	if err := b.Repos.Punches.SupersedeForRecord(ctx, tx, tenantID, rec.ID); err != nil {
		return nil, err
	}
	for _, p := range []*models.AttendancePunch{
		{TenantID: tenantID, AttendanceRecordID: rec.ID, EmployeeProfileID: employeeID, PunchTime: in, PunchType: models.PunchIn, Source: source},
		{TenantID: tenantID, AttendanceRecordID: rec.ID, EmployeeProfileID: employeeID, PunchTime: out, PunchType: models.PunchOut, Source: source},
	} {
		if err := b.Repos.Punches.Create(ctx, tx, p); err != nil {
			return nil, err
		}
	}
	rec.IsRegularized = true
	recShift, err := b.shiftByID(ctx, tx, tenantID, rec.ShiftID)
	if err != nil {
		return nil, err
	}
	return rec, b.recompute(ctx, tx, rec, recShift, b.Now().UTC())
}

func (t *timeWriter) SetWindow(ctx context.Context, tx *gorm.DB, tenantID, employeeID uuid.UUID, date, in, out time.Time, source string) error {
	_, err := t.setWindow(ctx, tx, tenantID, employeeID, date, in, out, source)
	return err
}

func (t *timeWriter) MarkDay(ctx context.Context, tx *gorm.DB, tenantID, employeeID uuid.UUID, date time.Time, status string) (bool, error) {
	if err := t.Repos.Records.LockEmployee(ctx, tx, tenantID, employeeID); err != nil {
		return false, err
	}
	if rec, err := t.Repos.Records.FindByDate(ctx, tx, tenantID, employeeID, date); err != nil || rec != nil {
		return false, err
	}
	shift, err := t.shiftOn(ctx, tx, tenantID, employeeID, date)
	if err != nil {
		return false, err
	}
	seed := &models.AttendanceRecord{TenantID: tenantID, EmployeeProfileID: employeeID, AttendanceDate: date, Status: status, Source: models.SourceSystem}
	if shift != nil {
		seed.ShiftID = &shift.ID
	}
	_, err = t.Repos.Records.FindOrCreate(ctx, tx, seed)
	return err == nil, err
}

func (t *timeWriter) AssignShift(ctx context.Context, tx *gorm.DB, tenantID, employeeID, shiftID uuid.UUID, from, to time.Time, actor uuid.UUID) error {
	overlap, err := t.Repos.Assignments.Overlapping(ctx, tx, tenantID, employeeID, from, &to)
	if err != nil {
		return err
	}
	if len(overlap) > 0 {
		return ErrAssignmentOverlap
	}
	return t.Repos.Assignments.Create(ctx, tx, &models.ShiftAssignment{TenantID: tenantID, EmployeeProfileID: employeeID, ShiftID: shiftID,
		EffectiveFrom: from, EffectiveTo: &to, CreatedByUserID: &actor})
}

func (t *timeWriter) ShiftState(ctx context.Context, db *gorm.DB, tenantID, shiftID uuid.UUID) (bool, bool, error) {
	s, err := t.Repos.Shifts.FindByID(ctx, db, tenantID, shiftID)
	if err != nil || s == nil {
		return false, false, err
	}
	return true, s.Status == models.ShiftActive, nil
}

func (t *timeWriter) HasRecord(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID, date time.Time) (bool, bool, error) {
	rec, err := t.Repos.Records.FindByDate(ctx, db, tenantID, employeeID, date)
	if err != nil || rec == nil {
		return false, false, err
	}
	ps, err := t.Repos.Punches.ActiveForRecord(ctx, db, tenantID, rec.ID)
	return true, len(ps) > 0, err
}

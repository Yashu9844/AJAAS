package services

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/attendance/calc"
	"github.com/jaas/jaas/internal/attendance/dto"
	"github.com/jaas/jaas/internal/attendance/events"
	"github.com/jaas/jaas/internal/attendance/models"
	"github.com/jaas/jaas/internal/attendance/validators"
	"gorm.io/gorm"
)

// punchDebounce rejects near-duplicate taps (AT-004).
const punchDebounce = 60 * time.Second

// PunchService records self-service IN/OUT punches (FR-PU001, AT-001..AT-006, AT-022).
type PunchService interface {
	Punch(ctx context.Context, a Actor, req dto.PunchRequest) (*dto.PunchResult, error)
}

type punchService struct{ base }

// NewPunchService builds a PunchService.
func NewPunchService(d Deps) PunchService { return &punchService{base{d}} }

// punchTx carries one punch's state through the transaction.
type punchTx struct {
	employeeID uuid.UUID
	req        dto.PunchRequest
	now        time.Time
	record     *models.AttendanceRecord
	shift      *models.Shift
	punch      *models.AttendancePunch
	outbox     *models.OutboxEvent
}

func (s *punchService) Punch(ctx context.Context, a Actor, req dto.PunchRequest) (*dto.PunchResult, error) {
	emp, err := s.employeeFor(ctx, a, true)
	if err != nil {
		return nil, err
	}
	now := s.Now().UTC() // AT-001: server clock only
	st := punchTx{employeeID: emp.ID, req: req, now: now}
	err = s.Tx.InTx(ctx, func(tx *gorm.DB) error {
		if err := s.Repos.Records.LockEmployee(ctx, tx, a.TenantID, emp.ID); err != nil {
			return err
		}
		if err := s.targetRecord(ctx, tx, a, &st); err != nil {
			return err
		}
		st.punch = newPunch(a, &st)
		if err := s.Repos.Punches.Create(ctx, tx, st.punch); err != nil {
			return err
		}
		if err := s.recompute(ctx, tx, st.record, st.shift, now); err != nil {
			return err
		}
		st.outbox, err = s.writeOutbox(ctx, tx, a, routingFor(req.Type), punchPayload(st.punch, st.record))
		return err
	})
	if err != nil {
		return nil, err
	}
	s.afterCommit(ctx, a, auditEntry{"attendance.punch_" + req.Type, "attendance_record", st.record.ID.String(),
		map[string]string{"punch_id": st.punch.ID.String(), "attendance_date": st.record.AttendanceDate.Format(validators.DateLayout)}}, st.outbox)
	return &dto.PunchResult{Punch: mapPunch(st.punch), Record: mapRecord(st.record)}, nil
}

// targetRecord applies AT-003..AT-006 and loads (or creates) the record the punch belongs to.
func (s *punchService) targetRecord(ctx context.Context, tx *gorm.DB, a Actor, st *punchTx) error {
	employeeID, req, now := st.employeeID, st.req, st.now
	last, err := s.Repos.Punches.LastForEmployee(ctx, tx, a.TenantID, employeeID)
	if err != nil {
		return err
	}
	if last != nil && now.Sub(last.PunchTime) < punchDebounce {
		return ErrDuplicatePunch
	}
	lastIsIn := last != nil && last.PunchType == models.PunchIn
	open := lastIsIn && now.Sub(last.PunchTime) <= calc.SessionAbandonAfter
	if req.Type == models.PunchOut {
		switch {
		case !lastIsIn:
			return ErrNotPunchedIn
		case !open:
			return ErrSessionExpired
		}
		if st.record, err = s.Repos.Records.FindByID(ctx, tx, a.TenantID, last.AttendanceRecordID); err != nil {
			return err
		}
		st.shift, err = s.shiftByID(ctx, tx, a.TenantID, st.record.ShiftID)
		return err
	}
	if open {
		return ErrAlreadyPunchedIn
	}
	date, shift, err := s.resolveDay(ctx, tx, a.TenantID, employeeID, now)
	if err != nil {
		return err
	}
	seed := &models.AttendanceRecord{TenantID: a.TenantID, EmployeeProfileID: employeeID, AttendanceDate: date,
		Status: models.StatusPresent, Source: sourceOf(req)}
	if shift != nil {
		seed.ShiftID = &shift.ID
	}
	if st.record, err = s.Repos.Records.FindOrCreate(ctx, tx, seed); err != nil {
		return err
	}
	if st.record.ShiftID == nil && shift != nil {
		st.record.ShiftID = &shift.ID
	}
	st.shift, err = s.shiftByID(ctx, tx, a.TenantID, st.record.ShiftID)
	return err
}

func sourceOf(req dto.PunchRequest) string {
	if req.Source == models.SourceMobile {
		return models.SourceMobile
	}
	return models.SourceWeb
}

func newPunch(a Actor, st *punchTx) *models.AttendancePunch {
	req := st.req
	p := &models.AttendancePunch{TenantID: a.TenantID, AttendanceRecordID: st.record.ID, EmployeeProfileID: st.employeeID,
		PunchTime: st.now, PunchType: req.Type, Source: sourceOf(req), Latitude: req.Latitude, Longitude: req.Longitude,
		DeviceID: req.DeviceID}
	if a.IP != "" {
		ip := a.IP
		p.IPAddress = &ip
	}
	return p
}

func routingFor(punchType string) string {
	if punchType == models.PunchOut {
		return events.PunchOut
	}
	return events.PunchIn
}

// punchPayload whitelists event fields (FR-EV003: no IP/device/coordinates).
func punchPayload(p *models.AttendancePunch, r *models.AttendanceRecord) events.PunchPayload {
	return events.PunchPayload{PunchID: p.ID, RecordID: r.ID, EmployeeID: r.EmployeeProfileID,
		AttendanceDate: r.AttendanceDate.Format(validators.DateLayout), PunchType: p.PunchType, PunchTime: p.PunchTime,
		Status: r.Status, WorkMinutes: r.TotalWorkMinutes, LateMinutes: r.LateMinutes}
}

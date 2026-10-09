package services

import (
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/attendance/calc"
	"github.com/jaas/jaas/internal/attendance/dto"
	"github.com/jaas/jaas/internal/attendance/models"
	"github.com/jaas/jaas/internal/attendance/validators"
)

func uuidStr(id *uuid.UUID) *string {
	if id == nil {
		return nil
	}
	s := id.String()
	return &s
}

func mapShift(s *models.Shift) *dto.ShiftResponse {
	if s == nil {
		return nil
	}
	return &dto.ShiftResponse{
		ID: s.ID.String(), Name: s.Name, Code: s.Code,
		StartTime: calc.FormatHHMM(s.StartMinute), EndTime: calc.FormatHHMM(s.EndMinute),
		GracePeriodMins: s.GracePeriodMins, BreakDurationMins: s.BreakDurationMins,
		FullDayMinutes: s.FullDayMinutes, HalfDayMinutes: s.HalfDayMinutes, Timezone: s.Timezone,
		IsNightShift: s.IsNightShift, ExpectedMinutes: calc.ExpectedMinutes(specFor(s)), Status: s.Status,
		CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt,
	}
}

func mapAssignment(a *models.ShiftAssignment) dto.AssignmentResponse {
	var to *string
	if a.EffectiveTo != nil {
		s := a.EffectiveTo.Format(validators.DateLayout)
		to = &s
	}
	return dto.AssignmentResponse{ID: a.ID.String(), EmployeeID: a.EmployeeProfileID.String(), ShiftID: a.ShiftID.String(),
		EffectiveFrom: a.EffectiveFrom.Format(validators.DateLayout), EffectiveTo: to, CreatedAt: a.CreatedAt}
}

func mapRecord(r *models.AttendanceRecord) dto.RecordResponse {
	return dto.RecordResponse{
		ID: r.ID.String(), EmployeeID: r.EmployeeProfileID.String(), AttendanceDate: r.AttendanceDate.Format(validators.DateLayout),
		ShiftID: uuidStr(r.ShiftID), FirstPunchIn: r.FirstPunchIn, LastPunchOut: r.LastPunchOut,
		TotalWorkMinutes: r.TotalWorkMinutes, TotalBreakMinutes: r.TotalBreakMinutes, LateMinutes: r.LateMinutes,
		OvertimeMinutes: r.OvertimeMinutes, Status: r.Status, Source: r.Source, IsRegularized: r.IsRegularized, UpdatedAt: r.UpdatedAt,
	}
}

// mapPunch never copies IPAddress (security.md AS-T7).
func mapPunch(p *models.AttendancePunch) dto.PunchResponse {
	return dto.PunchResponse{ID: p.ID.String(), PunchType: p.PunchType, PunchTime: p.PunchTime, Source: p.Source,
		Latitude: p.Latitude, Longitude: p.Longitude, DeviceID: p.DeviceID, IsSuperseded: p.IsSuperseded}
}

func mapRegularization(g *models.Regularization) dto.RegularizationResponse {
	return dto.RegularizationResponse{
		ID: g.ID.String(), EmployeeID: g.EmployeeProfileID.String(), AttendanceDate: g.AttendanceDate.Format(validators.DateLayout),
		RequestedPunchIn: g.RequestedPunchIn, RequestedPunchOut: g.RequestedPunchOut, Reason: g.Reason, Status: g.Status,
		ReviewerUserID: uuidStr(g.ReviewerUserID), ReviewedAt: g.ReviewedAt, ReviewComment: g.ReviewComment, CreatedAt: g.CreatedAt,
	}
}

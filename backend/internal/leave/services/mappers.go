package services

import (
	"github.com/jaas/jaas/internal/leave/dto"
	"github.com/jaas/jaas/internal/leave/events"
	"github.com/jaas/jaas/internal/leave/models"
	"github.com/jaas/jaas/internal/leave/validators"
)

func mapType(t *models.LeaveType) dto.LeaveTypeResponse {
	return dto.LeaveTypeResponse{ID: t.ID, Name: t.Name, Code: t.Code, IsPaid: t.IsPaid, AnnualAllowance: t.AnnualAllowance,
		Accrual: t.Accrual, CarryForwardLimit: t.CarryForwardLimit, MaxConsecutiveDays: t.MaxConsecutiveDays,
		MinNoticeDays: t.MinNoticeDays, AllowHalfDay: t.AllowHalfDay, SandwichRule: t.SandwichRule,
		ApplicableGender: t.ApplicableGender, Status: t.Status, CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt}
}

func mapHoliday(h *models.Holiday) dto.HolidayResponse {
	return dto.HolidayResponse{ID: h.ID, Date: h.HolidayDate.Format(validators.DateLayout), Name: h.Name,
		IsOptional: h.IsOptional, CreatedAt: h.CreatedAt}
}

func mapBalance(b *models.Balance, t *models.LeaveType) dto.BalanceResponse {
	return dto.BalanceResponse{LeaveTypeID: t.ID, LeaveTypeCode: t.Code, LeaveTypeName: t.Name, Year: b.Year,
		Opening: b.Opening, Accrued: b.Accrued, Adjusted: b.Adjusted, Used: b.Used, Reserved: b.Reserved,
		Available: b.Available(), IsPaid: t.IsPaid}
}

func mapLedger(e *models.LedgerEntry) dto.LedgerEntryResponse {
	return dto.LedgerEntryResponse{ID: e.ID, LeaveTypeID: e.LeaveTypeID, Year: e.Year, Kind: e.Kind, Days: e.Days,
		LeaveRequestID: e.LeaveRequestID, Note: e.Note, ActorUserID: e.ActorUserID, CreatedAt: e.CreatedAt}
}

func mapRequest(r *models.Request, code string) dto.LeaveRequestResponse {
	return dto.LeaveRequestResponse{ID: r.ID, EmployeeID: r.EmployeeProfileID, LeaveTypeID: r.LeaveTypeID,
		LeaveTypeCode: code, StartDate: r.StartDate.Format(validators.DateLayout), EndDate: r.EndDate.Format(validators.DateLayout),
		HalfDay: r.HalfDay, TotalDays: r.TotalDays, Reason: r.Reason, Status: r.Status, ReviewerUserID: r.ReviewerUserID,
		ReviewedAt: r.ReviewedAt, ReviewComment: r.ReviewComment, CancelledAt: r.CancelledAt,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
}

// requestPayload whitelists event fields (FR-EV003: no reason/comment).
func requestPayload(r *models.Request, t *models.LeaveType) events.RequestPayload {
	return events.RequestPayload{RequestID: r.ID, EmployeeID: r.EmployeeProfileID, LeaveTypeID: t.ID, LeaveTypeCode: t.Code,
		IsPaid: t.IsPaid, StartDate: r.StartDate.Format(validators.DateLayout), EndDate: r.EndDate.Format(validators.DateLayout),
		HalfDay: r.HalfDay, TotalDays: r.TotalDays, Status: r.Status}
}

// requestMeta is the audit metadata for request actions: ids, dates and days only (LS-T6).
func requestMeta(r *models.Request) map[string]string {
	return map[string]string{"employee_id": r.EmployeeProfileID.String(), "leave_type_id": r.LeaveTypeID.String(),
		"start_date": r.StartDate.Format(validators.DateLayout), "end_date": r.EndDate.Format(validators.DateLayout),
		"total_days": r.TotalDays.String()}
}

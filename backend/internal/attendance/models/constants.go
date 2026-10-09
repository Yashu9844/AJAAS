// Package models holds Module 3 attendance persistence models (specification.md §8).
package models

// Shift lifecycle statuses.
const (
	ShiftActive   = "active"
	ShiftInactive = "inactive"
)

// Punch types (AT-003).
const (
	PunchIn  = "in"
	PunchOut = "out"
)

// Punch / record sources.
const (
	SourceWeb            = "web"
	SourceMobile         = "mobile"
	SourceRegularization = "regularization"
	SourceLeave          = "leave"
)

// Attendance record statuses (AT-010). on_leave/holiday/week_off are reserved for Module 4 writers.
const (
	StatusPresent = "present"
	StatusHalfDay = "half_day"
	StatusAbsent  = "absent"
	StatusOnLeave = "on_leave"
	StatusHoliday = "holiday"
	StatusWeekOff = "week_off"
)

// Regularization statuses (AT-015..AT-018).
const (
	RegPending   = "pending"
	RegApproved  = "approved"
	RegRejected  = "rejected"
	RegCancelled = "cancelled"
)

// Package dto holds Module 4 request/response shapes (specification.md §5); never GORM models.
package dto

import "github.com/jaas/jaas/internal/leave/calc"

// CreateLeaveTypeRequest is L1. Day amounts are JSON numbers with ≤ 2 decimals; ranges checked in validators.
type CreateLeaveTypeRequest struct {
	Name               string    `json:"name" binding:"required,min=1,max=100"`
	Code               string    `json:"code" binding:"required"`
	IsPaid             *bool     `json:"is_paid"`
	AnnualAllowance    calc.Days `json:"annual_allowance"`
	Accrual            string    `json:"accrual" binding:"omitempty,oneof=annual monthly"`
	CarryForwardLimit  calc.Days `json:"carry_forward_limit"`
	MaxConsecutiveDays *int      `json:"max_consecutive_days" binding:"omitempty,min=1,max=365"`
	MinNoticeDays      *int      `json:"min_notice_days" binding:"omitempty,min=0,max=90"`
	AllowHalfDay       *bool     `json:"allow_half_day"`
	SandwichRule       *bool     `json:"sandwich_rule"`
	ApplicableGender   string    `json:"applicable_gender" binding:"omitempty,oneof=all male female"`
}

// UpdateLeaveTypeRequest is L4: pointer fields, code immutable; max_consecutive_days 0 clears the limit.
type UpdateLeaveTypeRequest struct {
	Name               *string    `json:"name" binding:"omitempty,min=1,max=100"`
	IsPaid             *bool      `json:"is_paid"`
	AnnualAllowance    *calc.Days `json:"annual_allowance"`
	Accrual            *string    `json:"accrual" binding:"omitempty,oneof=annual monthly"`
	CarryForwardLimit  *calc.Days `json:"carry_forward_limit"`
	MaxConsecutiveDays *int       `json:"max_consecutive_days" binding:"omitempty,min=0,max=365"`
	MinNoticeDays      *int       `json:"min_notice_days" binding:"omitempty,min=0,max=90"`
	AllowHalfDay       *bool      `json:"allow_half_day"`
	SandwichRule       *bool      `json:"sandwich_rule"`
	ApplicableGender   *string    `json:"applicable_gender" binding:"omitempty,oneof=all male female"`
}

// CreateHolidayRequest is H1.
type CreateHolidayRequest struct {
	Date       string `json:"date" binding:"required"`
	Name       string `json:"name" binding:"required,min=1,max=100"`
	IsOptional bool   `json:"is_optional"`
}

// AdjustBalanceRequest is B3 (LV-017); days is signed and non-zero.
type AdjustBalanceRequest struct {
	EmployeeID  string    `json:"employee_id" binding:"required,uuid"`
	LeaveTypeID string    `json:"leave_type_id" binding:"required,uuid"`
	Days        calc.Days `json:"days" binding:"required"`
	Reason      string    `json:"reason" binding:"required,min=1,max=500"`
}

// ApplyLeaveRequest is R1/R2. No employee field exists: the employee is the caller (NFR-SEC002).
type ApplyLeaveRequest struct {
	LeaveTypeID string `json:"leave_type_id" binding:"required,uuid"`
	StartDate   string `json:"start_date" binding:"required"`
	EndDate     string `json:"end_date" binding:"required"`
	HalfDay     string `json:"half_day" binding:"omitempty,oneof=first_half second_half"`
	Reason      string `json:"reason" binding:"required,min=1,max=500"`
}

// ReviewRequest is R7/R8; reject requires a comment (LV-010, enforced in service).
type ReviewRequest struct {
	Comment *string `json:"comment" binding:"omitempty,min=1,max=500"`
}

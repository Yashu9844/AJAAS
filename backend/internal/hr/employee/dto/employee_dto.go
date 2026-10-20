package dto

import (
	"time"

	"github.com/google/uuid"
)

type CreateEmployeeRequest struct {
	UserID           uuid.UUID  `json:"user_id" binding:"required"`
	EmployeeCode     string     `json:"employee_code" binding:"required,min=2,max=32"`
	FirstName        string     `json:"first_name" binding:"required,min=1,max=100"`
	LastName         string     `json:"last_name" binding:"required,min=1,max=100"`
	DisplayName      string     `json:"display_name"`
	Gender           string     `json:"gender"`
	DateOfBirth      *time.Time `json:"date_of_birth"`
	MaritalStatus    string     `json:"marital_status"`
	BloodGroup       string     `json:"blood_group"`
	AvatarURL        string     `json:"avatar_url"`
	EmploymentType   string     `json:"employment_type" binding:"required"`
	JoiningDate      time.Time  `json:"joining_date" binding:"required"`
	ProbationEndDate *time.Time `json:"probation_end_date"`
	NoticePeriodDays int        `json:"notice_period_days"`
	PersonalEmail    string     `json:"personal_email"`
	WorkPhone        string     `json:"work_phone"`
	PersonalPhone    string     `json:"personal_phone"`
	CurrentAddress   string     `json:"current_address"`
	PermanentAddress string     `json:"permanent_address"`
}

type UpdateEmployeeRequest struct {
	FirstName     *string    `json:"first_name,omitempty"`
	LastName      *string    `json:"last_name,omitempty"`
	DisplayName   *string    `json:"display_name,omitempty"`
	Gender        *string    `json:"gender,omitempty"`
	DateOfBirth   *time.Time `json:"date_of_birth,omitempty"`
	MaritalStatus *string    `json:"marital_status,omitempty"`
	BloodGroup    *string    `json:"blood_group,omitempty"`
	AvatarURL     *string    `json:"avatar_url,omitempty"`
}

type UpdateSelfContactRequest struct {
	PersonalPhone     *string `json:"personal_phone,omitempty"`
	CurrentAddress    *string `json:"current_address,omitempty"`
	PermanentAddress  *string `json:"permanent_address,omitempty"`
	EmergencyContacts *string `json:"emergency_contacts,omitempty"`
}

type TransitionStatusRequest struct {
	Status           string     `json:"status" binding:"required"`
	ConfirmationDate *time.Time `json:"confirmation_date,omitempty"`
	ResignationDate  *time.Time `json:"resignation_date,omitempty"`
	ExitDate         *time.Time `json:"exit_date,omitempty"`
	ExitReason       string     `json:"exit_reason,omitempty"`
	Notes            string     `json:"notes,omitempty"`
}

type EmployeeFilter struct {
	Status       string
	Search       string
	DepartmentID *uuid.UUID
	Page         int
	PerPage      int
}

type EmployeeResponse struct {
	ID            uuid.UUID         `json:"id"`
	TenantID      uuid.UUID         `json:"tenant_id"`
	UserID        uuid.UUID         `json:"user_id"`
	EmployeeCode  string            `json:"employee_code"`
	FirstName     string            `json:"first_name"`
	LastName      string            `json:"last_name"`
	DisplayName   string            `json:"display_name"`
	Gender        string            `json:"gender"`
	DateOfBirth   *time.Time        `json:"date_of_birth,omitempty"`
	MaritalStatus string            `json:"marital_status"`
	BloodGroup    string            `json:"blood_group"`
	AvatarURL     string            `json:"avatar_url,omitempty"`
	Status        string            `json:"status"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
	Employment    *EmploymentDetail `json:"employment,omitempty"`
	Contact       *EmployeeContact  `json:"contact,omitempty"`
}

type EmploymentDetail struct {
	EmploymentType   string     `json:"employment_type"`
	JoiningDate      time.Time  `json:"joining_date"`
	ProbationEndDate *time.Time `json:"probation_end_date,omitempty"`
	ConfirmationDate *time.Time `json:"confirmation_date,omitempty"`
	NoticePeriodDays int        `json:"notice_period_days"`
	ResignationDate  *time.Time `json:"resignation_date,omitempty"`
	ExitDate         *time.Time `json:"exit_date,omitempty"`
	ExitReason       string     `json:"exit_reason,omitempty"`
}

type EmployeeContact struct {
	PersonalEmail     string `json:"personal_email,omitempty"`
	WorkPhone         string `json:"work_phone,omitempty"`
	PersonalPhone     string `json:"personal_phone,omitempty"`
	CurrentAddress    string `json:"current_address,omitempty"`
	PermanentAddress  string `json:"permanent_address,omitempty"`
	EmergencyContacts string `json:"emergency_contacts,omitempty"`
}

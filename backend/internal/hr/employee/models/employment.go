package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/shared/database"
)

// EmploymentDetail tracks employment contracts, lifecycle dates, and notice periods.
type EmploymentDetail struct {
	database.TenantBaseModel
	EmployeeProfileID uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex:idx_emp_detail_tenant_profile" json:"employee_profile_id"`
	EmploymentType    string     `gorm:"type:varchar(30);not null;default:'full_time'" json:"employment_type"`
	JoiningDate       time.Time  `gorm:"type:date;not null" json:"joining_date"`
	ProbationEndDate  *time.Time `gorm:"type:date" json:"probation_end_date,omitempty"`
	ConfirmationDate  *time.Time `gorm:"type:date" json:"confirmation_date,omitempty"`
	NoticePeriodDays  int        `gorm:"type:int;not null;default:30" json:"notice_period_days"`
	ResignationDate   *time.Time `gorm:"type:date" json:"resignation_date,omitempty"`
	ExitDate          *time.Time `gorm:"type:date" json:"exit_date,omitempty"`
	ExitReason        string     `gorm:"type:text" json:"exit_reason,omitempty"`
}

func (EmploymentDetail) TableName() string {
	return "employment_details"
}

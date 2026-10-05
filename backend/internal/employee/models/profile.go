package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/shared/database"
)

// EmployeeProfile represents the core operational profile of an employee.
type EmployeeProfile struct {
	database.TenantBaseModel
	UserID        uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex:idx_emp_profile_tenant_user" json:"user_id"`
	EmployeeCode  string     `gorm:"type:varchar(32);not null;uniqueIndex:idx_emp_profile_tenant_code" json:"employee_code"`
	FirstName     string     `gorm:"type:varchar(100);not null" json:"first_name"`
	LastName      string     `gorm:"type:varchar(100);not null" json:"last_name"`
	DisplayName   string     `gorm:"type:varchar(200)" json:"display_name"`
	Gender        string     `gorm:"type:varchar(20)" json:"gender"`
	DateOfBirth   *time.Time `gorm:"type:date" json:"date_of_birth,omitempty"`
	MaritalStatus string     `gorm:"type:varchar(30)" json:"marital_status"`
	BloodGroup    string     `gorm:"type:varchar(10)" json:"blood_group"`
	AvatarURL     string     `gorm:"type:text" json:"avatar_url,omitempty"`
	Status        string     `gorm:"type:varchar(30);not null;default:'active';index" json:"status"`

	// Associations
	EmploymentDetail *EmploymentDetail  `gorm:"foreignKey:EmployeeProfileID;constraint:OnDelete:CASCADE" json:"employment_detail,omitempty"`
	Contact          *EmployeeContact   `gorm:"foreignKey:EmployeeProfileID;constraint:OnDelete:CASCADE" json:"contact,omitempty"`
	Statutory        *EmployeeStatutory `gorm:"foreignKey:EmployeeProfileID;constraint:OnDelete:CASCADE" json:"statutory,omitempty"`
	Documents        []EmployeeDocument `gorm:"foreignKey:EmployeeProfileID;constraint:OnDelete:CASCADE" json:"documents,omitempty"`
	Timelines        []EmployeeTimeline `gorm:"foreignKey:EmployeeProfileID;constraint:OnDelete:CASCADE" json:"timelines,omitempty"`
}

func (EmployeeProfile) TableName() string {
	return "employee_profiles"
}

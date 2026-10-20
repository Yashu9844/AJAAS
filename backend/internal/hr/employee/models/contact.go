package models

import (
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/shared/database"
)

// EmployeeContact stores personal contact details and emergency contacts.
type EmployeeContact struct {
	database.TenantBaseModel
	EmployeeProfileID uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_emp_contact_tenant_profile" json:"employee_profile_id"`
	PersonalEmail     string    `gorm:"type:varchar(255)" json:"personal_email"`
	WorkPhone         string    `gorm:"type:varchar(50)" json:"work_phone"`
	PersonalPhone     string    `gorm:"type:varchar(50)" json:"personal_phone"`
	CurrentAddress    string    `gorm:"type:text" json:"current_address"`
	PermanentAddress  string    `gorm:"type:text" json:"permanent_address"`
	EmergencyContacts string    `gorm:"type:jsonb" json:"emergency_contacts"`
}

func (EmployeeContact) TableName() string {
	return "employee_contacts"
}

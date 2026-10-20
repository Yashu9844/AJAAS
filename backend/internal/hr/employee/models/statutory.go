package models

import (
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/shared/database"
)

// EmployeeStatutory stores sensitive tax identifiers and banking coordinates.
type EmployeeStatutory struct {
	database.TenantBaseModel
	EmployeeProfileID uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:uq_emp_statutory_profile" json:"employee_profile_id"`
	TaxID             string    `gorm:"type:varchar(100)" json:"tax_id"`
	NationalID        string    `gorm:"type:varchar(100)" json:"national_id"`
	BankName          string    `gorm:"type:varchar(100)" json:"bank_name"`
	BankAccountNumber string    `gorm:"type:varchar(100)" json:"bank_account_number"`
	BankRoutingSwift  string    `gorm:"type:varchar(100)" json:"bank_routing_swift"`
}

func (EmployeeStatutory) TableName() string {
	return "employee_statutory"
}

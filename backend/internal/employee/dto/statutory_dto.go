package dto

import "github.com/google/uuid"

type UpdateStatutoryRequest struct {
	TaxID             string `json:"tax_id" binding:"max=100"`
	NationalID        string `json:"national_id" binding:"max=100"`
	BankName          string `json:"bank_name" binding:"max=100"`
	BankAccountNumber string `json:"bank_account_number" binding:"max=100"`
	BankRoutingSwift  string `json:"bank_routing_swift" binding:"max=100"`
}

type StatutoryResponse struct {
	ID                uuid.UUID `json:"id"`
	EmployeeProfileID uuid.UUID `json:"employee_profile_id"`
	TaxID             string    `json:"tax_id"`
	NationalID        string    `json:"national_id"`
	BankName          string    `json:"bank_name"`
	BankAccountNumber string    `json:"bank_account_number"`
	BankRoutingSwift  string    `json:"bank_routing_swift"`
}

package dto

// DeactivateRequest carries an optional reason for lifecycle deactivation
// (departments, teams, designations). Departments also accept ?force=true.
type DeactivateRequest struct {
	Reason *string `json:"reason" binding:"omitempty,max=500"`
}

package dto

import "time"

// CreateDepartmentRequest creates a department. FR-D001.
type CreateDepartmentRequest struct {
	Name               string  `json:"name" binding:"required,min=1,max=100"`
	Code               *string `json:"code" binding:"omitempty,min=2,max=32"`
	Description        *string `json:"description" binding:"omitempty,max=500"`
	ParentDepartmentID *string `json:"parent_department_id" binding:"omitempty,uuid"`
}

// UpdateDepartmentRequest patches a department; code is immutable and ignored. FR-D005.
type UpdateDepartmentRequest struct {
	Name               *string `json:"name" binding:"omitempty,min=1,max=100"`
	Description        *string `json:"description" binding:"omitempty,max=500"`
	ParentDepartmentID *string `json:"parent_department_id" binding:"omitempty,uuid"`
}

// DepartmentResponse serializes a department with hierarchy position.
type DepartmentResponse struct {
	ID                 string    `json:"id"`
	TenantID           string    `json:"tenant_id"`
	Name               string    `json:"name"`
	Code               *string   `json:"code,omitempty"`
	Description        *string   `json:"description,omitempty"`
	ParentDepartmentID *string   `json:"parent_department_id,omitempty"`
	Status             string    `json:"status"`
	Depth              int       `json:"depth"`
	Path               []string  `json:"path"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// DepartmentListResponse is a paginated department list.
type DepartmentListResponse struct {
	Data []DepartmentResponse `json:"data"`
	Meta PaginationMeta       `json:"meta"`
}

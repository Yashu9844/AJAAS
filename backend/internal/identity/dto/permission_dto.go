package dto

// PermissionResponse represents authorization permission records.
type PermissionResponse struct {
	ID          string  `json:"id"`
	Resource    string  `json:"resource"`
	Action      string  `json:"action"`
	Description *string `json:"description,omitempty"`
}

// PermissionListResponse holds paginated PermissionResponses.
type PermissionListResponse struct {
	Data []PermissionResponse `json:"data"`
	Meta PaginationMeta       `json:"meta"`
}

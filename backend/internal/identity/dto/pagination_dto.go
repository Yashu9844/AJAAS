package dto

// PaginationRequest holds paging query parameters.
type PaginationRequest struct {
	Page    int `form:"page" json:"page" binding:"omitempty,min=1"`
	PerPage int `form:"per_page" json:"per_page" binding:"omitempty,min=1,max=100"`
}

// GetPage returns the current page, defaulting to 1 if not set.
func (p *PaginationRequest) GetPage() int {
	if p.Page <= 0 {
		return 1
	}
	return p.Page
}

// GetPerPage returns the current per page limit, defaulting to 20 if not set.
func (p *PaginationRequest) GetPerPage() int {
	if p.PerPage <= 0 {
		return 20
	}
	return p.PerPage
}

// PaginationMeta contains pagination metadata returned in responses.
type PaginationMeta struct {
	Page       int   `json:"page"`
	PerPage    int   `json:"per_page"`
	TotalItems int64 `json:"total_items"`
	TotalPages int   `json:"total_pages"`
}

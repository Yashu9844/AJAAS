package dto

// PaginationMeta mirrors the identity pagination envelope for org list responses.
type PaginationMeta struct {
	Page       int   `json:"page"`
	PerPage    int   `json:"per_page"`
	TotalItems int64 `json:"total_items"`
	TotalPages int   `json:"total_pages"`
}

// NewPaginationMeta builds pagination metadata from a total count.
func NewPaginationMeta(page, perPage int, total int64) PaginationMeta {
	if page <= 0 {
		page = 1
	}
	if perPage <= 0 {
		perPage = 20
	}
	pages := int(total / int64(perPage))
	if total%int64(perPage) != 0 {
		pages++
	}
	return PaginationMeta{Page: page, PerPage: perPage, TotalItems: total, TotalPages: pages}
}

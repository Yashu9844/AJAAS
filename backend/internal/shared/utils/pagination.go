package utils

import (
	"strconv"

	"github.com/gin-gonic/gin"
)

const (
	DefaultPage    = 1
	DefaultPerPage = 20
	MaxPerPage     = 100
)

// NormalizePagination applies defaults and bounds: page >= 1, 1 <= per_page <= MaxPerPage.
func NormalizePagination(page, perPage int) (int, int) {
	if page < 1 {
		page = DefaultPage
	}
	if perPage < 1 {
		perPage = DefaultPerPage
	}
	if perPage > MaxPerPage {
		perPage = MaxPerPage
	}
	return page, perPage
}

// ParsePagination reads ?page and ?per_page, tolerating junk values (falls back to defaults) and clamping the range.
func ParsePagination(c *gin.Context) (int, int) {
	page, _ := strconv.Atoi(c.Query("page"))
	perPage, _ := strconv.Atoi(c.Query("per_page"))
	return NormalizePagination(page, perPage)
}

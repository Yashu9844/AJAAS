package kernel

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
)

// Fail writes the error envelope; unknown errors become a generic 500 so nothing internal leaks.
func Fail(c *gin.Context, err error) {
	if v, ok := AsVersionConflict(err); ok {
		c.JSON(http.StatusConflict, gin.H{"error": gin.H{"code": "VERSION_CONFLICT", "message": "the object changed since it was read", "current_version": v.Current}})
		return
	}
	err = sharedErrors.Normalize(err)
	var app *sharedErrors.AppError
	if errors.As(err, &app) {
		c.JSON(app.StatusCode, gin.H{"error": gin.H{"code": app.Code, "message": app.Message}})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": sharedErrors.ErrInternal.Code, "message": sharedErrors.ErrInternal.Message}})
}

// Respond writes {data} with status, or the error.
func Respond(c *gin.Context, status int, data any, err error) {
	if err != nil {
		Fail(c, err)
		return
	}
	c.JSON(status, gin.H{"data": data})
}

// CursorPage is one page of a cursor-paged list.
type CursorPage[T any] struct {
	Rows  []T
	Limit int
	Next  string
}

// NewCursorPage bundles a page for RespondCursor.
func NewCursorPage[T any](rows []T, limit int, next string) CursorPage[T] {
	return CursorPage[T]{Rows: rows, Limit: limit, Next: next}
}

// RespondCursor writes a cursor-paged list: {data, meta: {limit, next_cursor}}.
func RespondCursor[T any](c *gin.Context, p CursorPage[T], err error) {
	if err != nil {
		Fail(c, err)
		return
	}
	if p.Rows == nil {
		p.Rows = []T{}
	}
	var cur any
	if p.Next != "" {
		cur = p.Next
	}
	c.JSON(http.StatusOK, gin.H{"data": p.Rows, "meta": gin.H{"limit": p.Limit, "next_cursor": cur}})
}

// Bind decodes the JSON body into dst or answers 400.
func Bind(c *gin.Context, dst any) bool {
	if err := c.ShouldBindJSON(dst); err != nil {
		Fail(c, Invalid("body", "malformed JSON"))
		return false
	}
	return true
}

// ParamUUID reads a path parameter as a UUID or answers 400.
func ParamUUID(c *gin.Context, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param(name))
	if err != nil {
		Fail(c, Invalid(name, "must be a UUID"))
		return uuid.Nil, false
	}
	return id, true
}

// QueryUUID reads an optional UUID query parameter; ok is false (and 400 written) when it is present but malformed.
func QueryUUID(c *gin.Context, name string) (*uuid.UUID, bool) {
	raw := c.Query(name)
	if raw == "" {
		return nil, true
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		Fail(c, Invalid(name, "must be a UUID"))
		return nil, false
	}
	return &id, true
}

// IfMatchVersion reads the expected version from the If-Match header (an optional W/"3" or "3" form); 0 means absent.
func IfMatchVersion(c *gin.Context) int {
	raw := strings.Trim(strings.TrimPrefix(c.GetHeader("If-Match"), "W/"), `" `)
	n := 0
	for _, r := range raw {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int(r-'0')
	}
	return n
}

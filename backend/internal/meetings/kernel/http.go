package kernel

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
)

// Fail writes the error envelope; unknown errors become a generic 500 so nothing internal leaks.
func Fail(c *gin.Context, err error) {
	if v, ok := AsVersionConflict(err); ok {
		body := gin.H{"code": "VERSION_CONFLICT", "message": "the object changed since it was read", "current_version": v.Current}
		if v.Body != "" {
			body["current_body"] = v.Body
		}
		c.JSON(http.StatusConflict, gin.H{"error": body})
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

// NoContent answers 204 or the error.
func NoContent(c *gin.Context, err error) {
	if err != nil {
		Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// Page is a cursor-paged list.
type Page[T any] struct {
	Rows []T
	Next string
}

// RespondPage writes {data, meta: {next_cursor}}.
func RespondPage[T any](c *gin.Context, p Page[T], err error) {
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
	c.JSON(http.StatusOK, gin.H{"data": p.Rows, "meta": gin.H{"next_cursor": cur}})
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
	id, err := uuid.Parse(strings.TrimSuffix(c.Param(name), ".ics"))
	if err != nil {
		Fail(c, Invalid(name, "must be a UUID"))
		return uuid.Nil, false
	}
	return id, true
}

// QueryTime reads an optional RFC 3339 query parameter; ok is false (and 400 written) when it is malformed.
func QueryTime(c *gin.Context, name string) (*time.Time, bool) {
	raw := c.Query(name)
	if raw == "" {
		return nil, true
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		Fail(c, Invalid(name, "must be an RFC 3339 time"))
		return nil, false
	}
	return &t, true
}

// IfMatchVersion reads the expected version from the If-Match header (W/"3" or "3"); 0 means absent.
func IfMatchVersion(c *gin.Context) int {
	raw := strings.Trim(strings.TrimPrefix(c.GetHeader("If-Match"), "W/"), `" `)
	if raw == "" || len(raw) > 9 {
		return 0
	}
	n := 0
	for _, r := range raw {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int(r-'0')
	}
	return n
}

// Version returns the If-Match version, or the body version when the header is absent.
func Version(c *gin.Context, body int) int {
	if v := IfMatchVersion(c); v > 0 {
		return v
	}
	return body
}

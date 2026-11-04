// Package kernel holds what every M3 context shares: error constructors, the outbox writer, cursors, text checks and
// HTTP helpers. It imports no other M3 package.
package kernel

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"gorm.io/gorm"
)

// Module tables and names.
const (
	OutboxTable = "meetings_events_outbox"
	InboxTable  = "meetings_events_inbox"
	Producer    = "meetings-service"
)

// Err builds an application error with an explicit status and a stable code.
func Err(status int, code, msg string) *sharedErrors.AppError {
	return &sharedErrors.AppError{Code: code, Message: msg, StatusCode: status}
}

// Invalid is a 400 VALIDATION_ERROR naming the field.
func Invalid(field, msg string) *sharedErrors.AppError {
	return Err(http.StatusBadRequest, sharedErrors.ErrValidation.Code, field+": "+msg)
}

// Unprocessable is a 422 with a stable code for a well-formed request the rules refuse.
func Unprocessable(code, msg string) *sharedErrors.AppError {
	return Err(http.StatusUnprocessableEntity, code, msg)
}

// NotFound is a 404; foreign-tenant and no-access ids answer the same way so nothing leaks (PV-3).
func NotFound(what string) *sharedErrors.AppError {
	return Err(http.StatusNotFound, sharedErrors.ErrNotFound.Code, what+" not found")
}

// Forbidden is a 403 for callers who can see the object but lack the permission.
func Forbidden(perm string) *sharedErrors.AppError {
	return Err(http.StatusForbidden, sharedErrors.ErrForbidden.Code, "missing permission "+perm)
}

// Conflict is a 409 with a stable code.
func Conflict(code, msg string) *sharedErrors.AppError { return Err(http.StatusConflict, code, msg) }

// VersionConflict is returned when an edit carries a stale version; Current is what the caller should re-read.
type VersionConflict struct {
	Current int
	Body    string // the current text, for note sections (DMT-04)
}

// Error implements error.
func (e *VersionConflict) Error() string {
	return fmt.Sprintf("[VERSION_CONFLICT] the object is at version %d (status: 409)", e.Current)
}

// AsVersionConflict unwraps a VersionConflict.
func AsVersionConflict(err error) (*VersionConflict, bool) {
	var v *VersionConflict
	return v, errors.As(err, &v)
}

// CheckVersion compares the expected version (from If-Match or the body) with the stored one; 0 is "not sent".
func CheckVersion(expected, current int) error {
	if expected == 0 {
		return Err(http.StatusPreconditionRequired, "VERSION_REQUIRED", "send the version you read (If-Match or version)")
	}
	if expected != current {
		return &VersionConflict{Current: current}
	}
	return nil
}

// Text trims s and checks its length in characters; min 0 allows empty.
func Text(field, s string, min, max int) (string, error) {
	s = strings.TrimSpace(s)
	n := utf8.RuneCountInString(s)
	if n < min || n > max || !utf8.ValidString(s) {
		if min > 0 && n == 0 {
			return "", Invalid(field, "is required")
		}
		return "", Invalid(field, fmt.Sprintf("must be %d to %d characters", min, max))
	}
	return s, nil
}

// OneOf checks that v is one of the allowed values.
func OneOf(field, v string, allowed ...string) error {
	for _, a := range allowed {
		if v == a {
			return nil
		}
	}
	return Invalid(field, "must be one of "+strings.Join(allowed, ", "))
}

// Emit writes one event, version 1, in the caller transaction. Payloads hold ids, versions and codes only (DMT-18).
func Emit(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, key string, payload map[string]any) error {
	id := uuid.New()
	env := map[string]any{"event_id": id, "event_type": key, "routing_key": key, "event_version": 1,
		"tenant_id": tenantID, "occurred_at": time.Now().UTC(), "producer": Producer, "payload": payload}
	raw, err := json.Marshal(env)
	if err != nil {
		return err
	}
	return tx.WithContext(ctx).Exec(`INSERT INTO `+OutboxTable+` (id, tenant_id, event_id, routing_key, payload) VALUES (?, ?, ?, ?, ?)`,
		uuid.New(), tenantID, id, key, string(raw)).Error
}

// Default and maximum page sizes.
const (
	DefaultLimit = 50
	MaxLimit     = 200
)

// ClampLimit applies the default and the maximum.
func ClampLimit(raw string) int {
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || n < 1 {
		return DefaultLimit
	}
	if n > MaxLimit {
		return MaxLimit
	}
	return n
}

// EncodeCursor turns a position into an opaque token.
func EncodeCursor(v any) string {
	raw, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}

// DecodeCursor parses a token into dst; an empty token leaves dst untouched.
func DecodeCursor(token string, dst any) error {
	if token == "" {
		return nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || json.Unmarshal(raw, dst) != nil {
		return Invalid("cursor", "is not a valid cursor")
	}
	return nil
}

// InTx runs fn in a transaction on db.
func InTx(ctx context.Context, db *gorm.DB, fn func(tx *gorm.DB) error) error {
	return db.WithContext(ctx).Transaction(fn)
}

// IsUniqueViolation reports whether err is a unique-key violation (a lost race on a natural key).
func IsUniqueViolation(err error) bool {
	return err != nil && (errors.Is(err, gorm.ErrDuplicatedKey) || strings.Contains(err.Error(), "SQLSTATE 23505"))
}

package kernel

import (
	"encoding/base64"
	"encoding/json"
	"strconv"
	"strings"
)

// Default and maximum page sizes for cursor lists.
const (
	DefaultLimit = 50
	MaxLimit     = 100
)

// EncodeCursor turns a position into an opaque token.
func EncodeCursor(v any) string {
	raw, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}

// DecodeCursor parses a token into dst; an empty token means the first page and leaves dst untouched.
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

// Trim cuts the extra probe row fetched to learn whether another page exists.
func Trim[T any](rows []T, limit int) ([]T, bool) {
	if len(rows) > limit {
		return rows[:limit], true
	}
	return rows, false
}

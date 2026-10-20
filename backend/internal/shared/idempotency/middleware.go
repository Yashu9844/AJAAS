package idempotency

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Header names and limits.
const (
	Header         = "Idempotency-Key"
	ReplayedHeader = "Idempotent-Replayed"
	maxKeyLength   = 255
	maxBodyBytes   = 1 << 20
	// DefaultRetention is how long outcomes are replayable; it exceeds any reasonable client retry horizon.
	DefaultRetention = 24 * time.Hour
)

// Scoper returns who is calling (e.g. "tenant:user" from a verified token); ok=false skips idempotency handling.
type Scoper func(c *gin.Context) (scope string, ok bool)

// Middleware handles Idempotency-Key on POST/PUT/PATCH/DELETE requests that carry one.
func Middleware(store Store, scope Scoper, retention time.Duration) gin.HandlerFunc {
	if retention <= 0 {
		retention = DefaultRetention
	}
	return func(c *gin.Context) {
		key := c.GetHeader(Header)
		if key == "" || !isWrite(c.Request.Method) {
			c.Next()
			return
		}
		if len(key) > maxKeyLength {
			abort(c, http.StatusBadRequest, "VALIDATION_ERROR", "Idempotency-Key is longer than 255 characters")
			return
		}
		who, ok := scope(c)
		if !ok {
			c.Next()
			return
		}
		handle(c, store, who, key, retention)
	}
}

func isWrite(method string) bool {
	return method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch || method == http.MethodDelete
}

func abort(c *gin.Context, status int, code, msg string) {
	c.AbortWithStatusJSON(status, gin.H{"error": gin.H{"code": code, "message": msg}})
}

func handle(c *gin.Context, store Store, who, key string, retention time.Duration) {
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, maxBodyBytes))
	if err != nil {
		abort(c, http.StatusBadRequest, "VALIDATION_ERROR", "Unreadable request body")
		return
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	now := time.Now().UTC()
	rec := &Record{ID: uuid.New(), Scope: who, Method: c.Request.Method, Path: c.Request.URL.Path, Key: key,
		RequestHash: hash(c.Request.Method, c.Request.URL.Path, body), Status: StatusProcessing, CreatedAt: now, ExpiresAt: now.Add(retention)}
	existing, claimed, err := store.Claim(c.Request.Context(), rec)
	if err != nil {
		abort(c, http.StatusInternalServerError, "DATABASE_ERROR", "Failed to record the idempotency key")
		return
	}
	if !claimed {
		replay(c, store, existing, rec.RequestHash, now)
		return
	}
	run(c, store, rec.ID)
}

// replay answers a repeated key: same payload gives the stored outcome; a different payload or a request still
// running gives 409.
func replay(c *gin.Context, store Store, existing *Record, requestHash string, now time.Time) {
	switch {
	case existing.ExpiresAt.Before(now):
		_ = store.Release(c.Request.Context(), existing.ID)
		abort(c, http.StatusConflict, "IDEMPOTENCY_KEY_EXPIRED", "This Idempotency-Key expired; retry with a new key")
	case existing.RequestHash != requestHash:
		abort(c, http.StatusConflict, "IDEMPOTENCY_KEY_REUSED", "This Idempotency-Key was already used with a different request")
	case existing.Status != StatusCompleted:
		abort(c, http.StatusConflict, "IDEMPOTENCY_IN_PROGRESS", "A request with this Idempotency-Key is still being processed")
	default:
		c.Header(ReplayedHeader, "true")
		c.Data(existing.ResponseStatus, existing.ContentType, existing.ResponseBody)
		c.Abort()
	}
}

// run executes the handler, capturing its response; server errors release the key so the client can retry.
func run(c *gin.Context, store Store, id uuid.UUID) {
	w := &capture{ResponseWriter: c.Writer}
	c.Writer = w
	c.Next()
	ctx := context.WithoutCancel(c.Request.Context())
	status := w.Status()
	if status >= http.StatusInternalServerError {
		_ = store.Release(ctx, id)
		return
	}
	_ = store.Complete(ctx, id, status, w.buf.Bytes(), w.Header().Get("Content-Type"))
}

func hash(method, path string, body []byte) string {
	h := sha256.New()
	h.Write([]byte(method + " " + path + "\n"))
	h.Write(body)
	return hex.EncodeToString(h.Sum(nil))
}

// capture tees the response body.
type capture struct {
	gin.ResponseWriter
	buf bytes.Buffer
}

func (w *capture) Write(b []byte) (int, error) {
	w.buf.Write(b)
	return w.ResponseWriter.Write(b)
}

func (w *capture) WriteString(s string) (int, error) {
	w.buf.WriteString(s)
	return w.ResponseWriter.WriteString(s)
}

// RunPurger deletes expired records every interval until ctx is cancelled.
func RunPurger(ctx context.Context, store Store, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		_, _ = store.PurgeExpired(ctx, time.Now().UTC())
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

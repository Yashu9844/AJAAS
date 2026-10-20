package middleware

import (
	"context"

	"github.com/google/uuid"
)

// Correlation headers: X-Correlation-ID is canonical (docs/ARCHITECTURE.md §3.4); X-Request-ID is still accepted.
const (
	CorrelationHeader = "X-Correlation-ID"
	RequestIDHeader   = "X-Request-ID"
)

type correlationKey struct{}

// WithCorrelationID returns ctx carrying the request's correlation ID.
func WithCorrelationID(ctx context.Context, id uuid.UUID) context.Context {
	return context.WithValue(ctx, correlationKey{}, id)
}

// CorrelationIDFrom returns the correlation ID carried by ctx, if any.
func CorrelationIDFrom(ctx context.Context) (uuid.UUID, bool) {
	id, ok := ctx.Value(correlationKey{}).(uuid.UUID)
	return id, ok && id != uuid.Nil
}

// CorrelationOrNew returns the ctx correlation ID or a fresh one (system-initiated work).
func CorrelationOrNew(ctx context.Context) uuid.UUID {
	if id, ok := CorrelationIDFrom(ctx); ok {
		return id
	}
	return uuid.New()
}

// incomingCorrelation accepts a caller-supplied ID only when it is a valid UUID; anything else is replaced.
func incomingCorrelation(values ...string) uuid.UUID {
	for _, v := range values {
		if id, err := uuid.Parse(v); err == nil && id != uuid.Nil {
			return id
		}
	}
	return uuid.New()
}

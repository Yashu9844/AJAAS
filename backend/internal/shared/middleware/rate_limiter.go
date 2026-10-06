package middleware

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"context"
	"github.com/gin-gonic/gin"
)

// RateLimitStore is the atomic counter backend required by the limiter (satisfied by cache.RedisClient).
type RateLimitStore interface {
	// IncrWithExpire atomically increments key and, on first creation, sets its TTL to window.
	IncrWithExpire(ctx context.Context, key string, window time.Duration) (int64, error)
	// Decr atomically decrements key (never below zero).
	Decr(ctx context.Context, key string) error
	// TTL returns the remaining time-to-live of key.
	TTL(ctx context.Context, key string) (time.Duration, error)
}

// RateLimiter returns a Gin middleware that rate-limits requests by client IP (fixed window, atomic).
// Every request counts.
func RateLimiter(store RateLimitStore, limit int, window time.Duration) gin.HandlerFunc {
	return rateLimiter(store, limit, window, false)
}

// FailureRateLimiter is like RateLimiter but only FAILED attempts (HTTP >= 400) consume the budget: a successful
// request refunds its slot. Use it for credential endpoints so legitimate users are not locked out by their own
// successful logins while brute-force (failed) attempts are still throttled.
func FailureRateLimiter(store RateLimitStore, limit int, window time.Duration) gin.HandlerFunc {
	return rateLimiter(store, limit, window, true)
}

func rateLimiter(store RateLimitStore, limit int, window time.Duration, failuresOnly bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := fmt.Sprintf("ratelimit:%s:%s", c.FullPath(), c.ClientIP())
		ctx := c.Request.Context()

		count, err := store.IncrWithExpire(ctx, key, window)
		if err != nil {
			// Fail-open on Redis errors so an outage does not take authentication down.
			c.Next()
			return
		}

		if count > int64(limit) {
			retry := int(window.Seconds())
			if ttl, terr := store.TTL(ctx, key); terr == nil && ttl > 0 {
				retry = int(ttl.Seconds()) + 1
			}
			c.Header("Retry-After", strconv.Itoa(retry))
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": gin.H{
					"code":    "RATE_LIMITED",
					"message": "Too many requests, please retry later.",
				},
			})
			return
		}

		c.Next()

		if failuresOnly && c.Writer.Status() < http.StatusBadRequest {
			_ = store.Decr(ctx, key)
		}
	}
}

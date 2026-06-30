package middleware

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jaas/jaas/internal/shared/cache"
)

// RateLimiter returns a Gin middleware that rate-limits requests by client IP.
func RateLimiter(redisClient *cache.RedisClient, limit int, window time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		key := fmt.Sprintf("ratelimit:%s:%s", c.FullPath(), ip)

		ctx := c.Request.Context()

		// Retrieve key count from Redis
		val, exists, err := redisClient.Get(ctx, key)
		if err != nil {
			// Fail-open: log warning and continue on Redis errors
			c.Next()
			return
		}

		count := 0
		if exists {
			count, _ = strconv.Atoi(val)
		}

		if count >= limit {
			c.Header("Retry-After", strconv.Itoa(int(window.Seconds())))
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": gin.H{
					"code":    "RATE_LIMITED",
					"message": "Too many requests, please retry later.",
				},
			})
			return
		}

		// Increment count
		newCount := count + 1
		err = redisClient.Set(ctx, key, strconv.Itoa(newCount), window)
		if err != nil {
			// Fail-open fallback
			c.Next()
			return
		}

		c.Next()
	}
}

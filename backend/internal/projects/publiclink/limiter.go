package publiclink

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// publicRatePerMinute is deliberately low: the route needs no login, so each client IP gets a small budget.
const publicRatePerMinute = 60

const limiterMaxKeys = 10000

type window struct {
	count   int
	resetAt time.Time
}

// ipLimiter is a fixed-window per-IP limiter held in process memory; production swaps it for the shared Redis middleware.
type ipLimiter struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	hits   map[string]*window
	now    func() time.Time
}

func newLimiter(limit int, d time.Duration) gin.HandlerFunc {
	l := &ipLimiter{limit: limit, window: d, hits: map[string]*window{}, now: time.Now}
	return l.handle
}

// allow counts a hit for key and reports whether it is within the limit, plus the seconds until the window resets.
func (l *ipLimiter) allow(key string) (bool, int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if len(l.hits) > limiterMaxKeys {
		for k, w := range l.hits {
			if now.After(w.resetAt) {
				delete(l.hits, k)
			}
		}
	}
	w := l.hits[key]
	if w == nil || now.After(w.resetAt) {
		w = &window{resetAt: now.Add(l.window)}
		l.hits[key] = w
	}
	w.count++
	return w.count <= l.limit, int(w.resetAt.Sub(now).Seconds()) + 1
}

func (l *ipLimiter) handle(c *gin.Context) {
	ok, retry := l.allow(c.ClientIP())
	if ok {
		c.Next()
		return
	}
	c.Header("Retry-After", strconv.Itoa(retry))
	c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": gin.H{"code": "RATE_LIMITED", "message": "Too many requests, please retry later."}})
}

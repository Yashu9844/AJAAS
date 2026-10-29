package webhooks

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// incomingRatePerMinute caps requests per client IP on the public integration route (shared by every integration id,
// because an attacker guessing ids must not get a budget per id). Provider hook bursts are far below it.
const incomingRatePerMinute = 120

const limiterMaxKeys = 10000

type limitWindow struct {
	count   int
	resetAt time.Time
}

// ipLimiter is a fixed-window per-IP limiter in process memory; production swaps in the shared Redis limiter via
// Service.SetLimiter.
type ipLimiter struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	hits   map[string]*limitWindow
	now    func() time.Time
}

func newLimiter(limit int, window time.Duration) gin.HandlerFunc {
	l := &ipLimiter{limit: limit, window: window, hits: map[string]*limitWindow{}, now: time.Now}
	return l.handle
}

// allow counts a hit for key and reports whether it is within the limit plus the seconds until the window resets.
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
		w = &limitWindow{resetAt: now.Add(l.window)}
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

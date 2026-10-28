package planning

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// feedRatePerMinute is deliberately low: calendar clients poll every few minutes at most, and the feed is the one M2
// route reachable without a bearer token.
const feedRatePerMinute = 30

const feedLimiterMaxKeys = 10000

type feedWindow struct {
	count   int
	resetAt time.Time
}

// feedLimiter is a fixed-window per-IP limiter held in process memory. Production can swap it for the shared Redis
// middleware through Service.SetFeedLimiter.
type feedLimiter struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	hits   map[string]*feedWindow
	now    func() time.Time
}

func newFeedLimiter(limit int, window time.Duration) gin.HandlerFunc {
	l := &feedLimiter{limit: limit, window: window, hits: map[string]*feedWindow{}, now: time.Now}
	return l.handle
}

// allow counts a hit for key and reports whether it is within the limit, plus the seconds until the window resets.
func (l *feedLimiter) allow(key string) (bool, int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if len(l.hits) > feedLimiterMaxKeys {
		for k, w := range l.hits {
			if now.After(w.resetAt) {
				delete(l.hits, k)
			}
		}
	}
	w := l.hits[key]
	if w == nil || now.After(w.resetAt) {
		w = &feedWindow{resetAt: now.Add(l.window)}
		l.hits[key] = w
	}
	w.count++
	return w.count <= l.limit, int(w.resetAt.Sub(now).Seconds()) + 1
}

func (l *feedLimiter) handle(c *gin.Context) {
	ok, retry := l.allow(c.ClientIP())
	if ok {
		c.Next()
		return
	}
	c.Header("Retry-After", strconv.Itoa(retry))
	c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": gin.H{"code": "RATE_LIMITED", "message": "Too many requests, please retry later."}})
}

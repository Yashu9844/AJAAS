package app

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jaas/jaas/internal/shared/cache"
	"gorm.io/gorm"
)

// readinessTimeout bounds each dependency check of /health/ready.
const readinessTimeout = 2 * time.Second

// pinger is a dependency the service needs to serve traffic.
type pinger func(ctx context.Context) error

// registerHealth mounts liveness and readiness (docs/ARCHITECTURE.md §3.10). Liveness only says the process serves
// HTTP; readiness checks Postgres and Redis. RabbitMQ is deliberately not required: the outbox holds events while the
// broker is down.
func registerHealth(router *gin.Engine, db *gorm.DB, redis *cache.RedisClient) {
	live := func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "UP", "timestamp": time.Now().Format(time.RFC3339)})
	}
	router.GET("/health", live)
	router.GET("/health/live", live)
	checks := map[string]pinger{"postgres": func(ctx context.Context) error {
		sqlDB, err := db.DB()
		if err != nil {
			return err
		}
		return sqlDB.PingContext(ctx)
	}}
	if redis != nil {
		checks["redis"] = redis.Ping
	}
	router.GET("/health/ready", func(c *gin.Context) { ready(c, checks) })
}

func ready(c *gin.Context, checks map[string]pinger) {
	status, code := gin.H{}, http.StatusOK
	for name, check := range checks {
		ctx, cancel := context.WithTimeout(c.Request.Context(), readinessTimeout)
		err := check(ctx)
		cancel()
		if err != nil {
			status[name], code = "DOWN", http.StatusServiceUnavailable
			continue
		}
		status[name] = "UP"
	}
	overall := "UP"
	if code != http.StatusOK {
		overall = "DOWN"
	}
	c.JSON(code, gin.H{"status": overall, "checks": status, "timestamp": time.Now().Format(time.RFC3339)})
}

package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jaas/jaas/internal/app"
	"github.com/jaas/jaas/internal/shared/cache"
	"github.com/jaas/jaas/internal/shared/config"
	"github.com/jaas/jaas/internal/shared/database"
	"github.com/jaas/jaas/internal/shared/logger"
	"github.com/jaas/jaas/internal/shared/queue"
)

func main() {
	env := os.Getenv("APP_ENV")
	if env == "" {
		env = "development"
	}
	cfg, err := config.LoadConfig("configs", env)
	if err != nil {
		fmt.Printf("Fatal error loading config: %v\n", err)
		os.Exit(1)
	}
	log := logger.NewLogger(cfg.Server.Env)
	log.Info().Msgf("Initializing JAAS backend in %s environment", cfg.Server.Env)

	opts, redisClient := buildOptions(cfg, log)
	relayCtx, stopRelays := context.WithCancel(context.Background()) // outbox relays run until shutdown (FR-EV002)
	opts.Background = relayCtx
	router, err := app.Build(opts)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to build application")
	}
	log.Info().Msg("Application built and schema ready")

	srv := newServer(cfg, router)
	go func() {
		log.Info().Msgf("Server listening and serving HTTP on port %d", cfg.Server.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("HTTP server execution failed")
		}
	}()
	shutdown(log, srv, stopRelays)
	_ = redisClient.Close()
	log.Info().Msg("Server exited cleanly")
}

// buildOptions connects the infrastructure and assembles the application options.
func buildOptions(cfg *config.Config, log *logger.Logger) (app.Options, *cache.RedisClient) {
	db, err := database.NewDatabase(&cfg.Database)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to database")
	}
	log.Info().Msg("Connected to database successfully")
	redisClient, err := cache.NewRedisClient(cfg.Redis.Host, cfg.Redis.Port, cfg.Redis.Password, cfg.Redis.DB)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to Redis cache")
	}
	log.Info().Msg("Connected to Redis successfully")
	trusted, cors := proxiesAndOrigins(cfg.Server.Env)
	return app.Options{Config: cfg, DB: db, Redis: redisClient, Publisher: newPublisher(cfg, log), Logger: log, CORSOrigins: cors, TrustedProxies: trusted}, redisClient
}

// newPublisher returns the RabbitMQ publisher; the broker is optional, so a missing or unreachable one falls back to the no-op publisher.
func newPublisher(cfg *config.Config, log *logger.Logger) queue.EventPublisher {
	if cfg.RabbitMQ.Host == "" {
		log.Warn().Msg("RabbitMQ host not configured. Using NoOpPublisher.")
		return queue.NewNoOpPublisher()
	}
	pub, err := queue.NewRabbitMQPublisher(cfg.RabbitMQ.Host, cfg.RabbitMQ.Port, cfg.RabbitMQ.User, cfg.RabbitMQ.Password)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to connect to RabbitMQ broker. Falling back to NoOpPublisher.")
		return queue.NewNoOpPublisher()
	}
	log.Info().Msg("Connected to RabbitMQ successfully")
	return pub
}

// proxiesAndOrigins reads TRUSTED_PROXIES and CORS_ALLOWED_ORIGINS; "*" origins are a development convenience only.
func proxiesAndOrigins(env string) (trusted, cors []string) {
	if v := os.Getenv("TRUSTED_PROXIES"); v != "" {
		trusted = strings.Split(v, ",")
	}
	cors = strings.Split(os.Getenv("CORS_ALLOWED_ORIGINS"), ",")
	if os.Getenv("CORS_ALLOWED_ORIGINS") == "" && env != "production" {
		cors = []string{"*"}
	}
	return trusted, cors
}

func newServer(cfg *config.Config, h http.Handler) *http.Server {
	return &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
}

// shutdown waits for SIGINT/SIGTERM, stops the relays and drains the server.
func shutdown(log *logger.Logger, srv *http.Server, stopRelays context.CancelFunc) {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Info().Msg("Shutting down HTTP server...")
	stopRelays()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal().Err(err).Msg("HTTP server forced to shutdown")
	}
}

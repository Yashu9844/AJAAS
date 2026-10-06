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

	// RabbitMQ is optional: fall back to the NoOp publisher when it is not configured or unreachable.
	var publisher queue.EventPublisher
	if cfg.RabbitMQ.Host != "" {
		pub, perr := queue.NewRabbitMQPublisher(cfg.RabbitMQ.Host, cfg.RabbitMQ.Port, cfg.RabbitMQ.User, cfg.RabbitMQ.Password)
		if perr != nil {
			log.Warn().Err(perr).Msg("Failed to connect to RabbitMQ broker. Falling back to NoOpPublisher.")
			publisher = queue.NewNoOpPublisher()
		} else {
			publisher = pub
			log.Info().Msg("Connected to RabbitMQ successfully")
		}
	} else {
		log.Warn().Msg("RabbitMQ host not configured. Using NoOpPublisher.")
		publisher = queue.NewNoOpPublisher()
	}

	var trusted []string
	if v := os.Getenv("TRUSTED_PROXIES"); v != "" {
		trusted = strings.Split(v, ",")
	}
	cors := strings.Split(os.Getenv("CORS_ALLOWED_ORIGINS"), ",")
	if os.Getenv("CORS_ALLOWED_ORIGINS") == "" && cfg.Server.Env != "production" {
		cors = []string{"*"} // development convenience only
	}

	router, err := app.Build(app.Options{
		Config:         cfg,
		DB:             db,
		Redis:          redisClient,
		Publisher:      publisher,
		Logger:         log,
		CORSOrigins:    cors,
		TrustedProxies: trusted,
	})
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to build application")
	}
	log.Info().Msg("Application built and schema ready")

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		log.Info().Msgf("Server listening and serving HTTP on port %d", cfg.Server.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("HTTP server execution failed")
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Info().Msg("Shutting down HTTP server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal().Err(err).Msg("HTTP server forced to shutdown")
	}

	_ = redisClient.Close()
	log.Info().Msg("Server exited cleanly")
}

package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jaas/jaas/internal/identity"
	"github.com/jaas/jaas/internal/shared/cache"
	"github.com/jaas/jaas/internal/shared/config"
	"github.com/jaas/jaas/internal/shared/database"
	"github.com/jaas/jaas/internal/shared/logger"
	sharedMiddleware "github.com/jaas/jaas/internal/shared/middleware"
	"github.com/jaas/jaas/internal/shared/queue"
)

func main() {
	// Resolve environment name
	env := os.Getenv("APP_ENV")
	if env == "" {
		env = "development"
	}

	// 1. Load Configurations
	cfg, err := config.LoadConfig("configs", env)
	if err != nil {
		fmt.Printf("Fatal error loading config: %v\n", err)
		os.Exit(1)
	}

	// 2. Initialize Structured Logger
	log := logger.NewLogger(cfg.Server.Env)
	log.Info().Msgf("Initializing JAAS backend in %s environment", cfg.Server.Env)

	// 3. Connect to PostgreSQL
	db, err := database.NewDatabase(&cfg.Database)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to database")
	}
	log.Info().Msg("Connected to database successfully")

	// 4. Connect to Redis
	redisClient, err := cache.NewRedisClient(
		cfg.Redis.Host,
		cfg.Redis.Port,
		cfg.Redis.Password,
		cfg.Redis.DB,
	)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to Redis cache")
	}
	log.Info().Msg("Connected to Redis successfully")

	// 5. Connect to RabbitMQ (Fallback to NoOpPublisher if connections fail)
	var publisher queue.EventPublisher
	if cfg.RabbitMQ.Host != "" {
		pub, err := queue.NewRabbitMQPublisher(
			cfg.RabbitMQ.Host,
			cfg.RabbitMQ.Port,
			cfg.RabbitMQ.User,
			cfg.RabbitMQ.Password,
		)
		if err != nil {
			log.Warn().Err(err).Msg("Failed to connect to RabbitMQ broker. Falling back to NoOpPublisher.")
			publisher = queue.NewNoOpPublisher()
		} else {
			publisher = pub
			log.Info().Msg("Connected to RabbitMQ successfully")
		}
	} else {
		log.Warn().Msg("RabbitMQ host not configured. Using NoOpPublisher.")
		publisher = queue.NewNoOpPublisher()
	}

	// 6. Bootstrap Identity Module
	identityModule := identity.NewModule(
		db,
		redisClient,
		publisher,
		cfg.JWT.Secret,
		cfg.JWT.AccessTokenTTL,
		cfg.JWT.RefreshTokenTTL,
		&log.Logger,
	)

	// Run GORM migrations to auto-ensure tables structure
	log.Info().Msg("Running database schema AutoMigrations...")
	if err := db.AutoMigrate(identityModule.RegisterModels()...); err != nil {
		log.Fatal().Err(err).Msg("Database auto-migration failed")
	}
	log.Info().Msg("Database auto-migrations executed successfully")

	// 7. Initialize HTTP server engine
	if cfg.Server.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()
	router.Use(gin.Recovery())

	// Attach global logger and CORS middleware
	router.Use(sharedMiddleware.RequestLogger(log))
	router.Use(sharedMiddleware.CORS())

	// Group routing definitions under v1 API
	v1Group := router.Group("/api/v1")
	identityModule.RegisterRoutes(v1Group)

	// Health check route
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":    "UP",
			"timestamp": time.Now().Format(time.RFC3339),
		})
	})

	serverAddr := fmt.Sprintf(":%d", cfg.Server.Port)
	srv := &http.Server{
		Addr:    serverAddr,
		Handler: router,
	}

	// Graceful shutdowns handler
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

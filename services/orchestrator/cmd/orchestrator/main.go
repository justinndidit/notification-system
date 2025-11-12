package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/justinndidit/notificationSystem/orchestrator/internal/app"
	"github.com/justinndidit/notificationSystem/orchestrator/internal/config"
	"github.com/justinndidit/notificationSystem/orchestrator/internal/database"
	"github.com/justinndidit/notificationSystem/orchestrator/internal/handlers"
	"github.com/justinndidit/notificationSystem/orchestrator/internal/logger"
	"github.com/justinndidit/notificationSystem/orchestrator/internal/routers"
	"github.com/justinndidit/notificationSystem/orchestrator/internal/server"
	"github.com/justinndidit/notificationSystem/orchestrator/internal/services"
)

const DefaultContextTimeout = 30

func main() {
	logger := logger.NewLogger("orchestrator")
	logger.Info().Msg("Application starting...")

	// Load config
	cfg, err := config.LoadConfig()
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to load config")
	}
	logger.Info().Msg("Config loaded successfully")

	// Database migration
	migrateCtx, cancelMigrate := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelMigrate()

	logger.Info().Msg("Starting database migration...")
	if err = database.Migrate(migrateCtx, &logger, cfg.Database); err != nil {
		logger.Fatal().Err(err).Msg("failed to migrate database")
	}
	logger.Info().Msg("Database migration completed")

	// Database connection
	logger.Info().Msg("Connecting to database...")
	db, err := database.New(cfg.Database, &logger)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to initialize database")
	}
	defer db.Close()
	logger.Info().Msg("Database connected successfully")

	// Redis connection
	logger.Info().Msg("Connecting to Redis...")
	redisClient := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Address,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	defer redisClient.Close()

	// Test Redis connection
	if err = redisClient.Ping(context.Background()).Err(); err != nil {
		logger.Fatal().Err(err).Msg("failed to connect to Redis")
	}
	logger.Info().Msg("Redis connected successfully")

	// RabbitMQ connection
	logger.Info().Msg("Connecting to RabbitMQ...")
	rabbitChannel, err := config.SetupRabbitMQ(cfg.RabbitMQ)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to initialize RabbitMQ")
	}
	defer rabbitChannel.Close()
	logger.Info().Msg("RabbitMQ connected successfully")

	// Initialize service clients
	logger.Info().Msg("Initializing service clients...")
	templateClient := services.NewTemplateClient(&logger, "http://localhost:3003")
	userClient := services.NewUserClient(&logger, "http://localhost:3001")

	// Initialize orchestrator
	logger.Info().Msg("Initializing orchestrator...")
	orchestrator := services.NewOrchestrator(
		&logger,
		templateClient,
		userClient,
		redisClient,
		rabbitChannel,
		cfg.RabbitMQ.ExchangeName,
		db.Pool,
	)

	// Initialize handlers
	logger.Info().Msg("Initializing handlers...")
	notificationHandler := handlers.NewNotificationHandler(&logger, redisClient, orchestrator)
	healthHandler := handlers.NewHealthHandler(&logger, redisClient, db)

	// Initialize app
	app := app.NewApp(cfg, &logger, redisClient, db, notificationHandler, healthHandler)

	// Setup routes
	router := routers.SetupRoutes(app)

	// Initialize server
	srv, err := server.New(app)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to initialize server")
	}
	srv.SetupHTTPServer(router)

	// Context for graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	// Start server
	go func() {
		logger.Info().
			Str("port", cfg.Server.Port).
			Msg("Starting HTTP server...")
		if err = srv.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Fatal().Err(err).Msg("failed to start server")
		}
	}()

	logger.Info().Msg("Server is ready to accept connections")

	// Wait for interrupt signal
	<-ctx.Done()
	logger.Info().Msg("Shutdown signal received, starting graceful shutdown...")

	// Graceful shutdown
	shutdownCtx, cancel := context.WithTimeout(context.Background(), DefaultContextTimeout*time.Second)
	defer cancel()

	// Shutdown orchestrator (closes RabbitMQ and Redis connections)
	logger.Info().Msg("Shutting down orchestrator...")
	if err = orchestrator.Shutdown(shutdownCtx); err != nil {
		logger.Error().Err(err).Msg("error during orchestrator shutdown")
	}

	// Shutdown HTTP server
	logger.Info().Msg("Shutting down HTTP server...")
	if err = srv.Shutdown(shutdownCtx); err != nil {
		logger.Fatal().Err(err).Msg("server forced to shutdown")
	}

	logger.Info().Msg("Server exited properly")
}

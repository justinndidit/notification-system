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
	"github.com/justinndidit/notificationSystem/orchestrator/internal/repositories"
	"github.com/justinndidit/notificationSystem/orchestrator/internal/routers"
	"github.com/justinndidit/notificationSystem/orchestrator/internal/server"
)

const DefaultContextTimeout = 30

func main() {
	logger := logger.NewLogger("orchestrator")
	logger.Info().Msg("Application starting...")

	cfg, err := config.LoadConfig()
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to load config")
	}
	logger.Info().Msg("Config loaded successfully")

	// Migration with timeout
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
	logger.Info().Msg("Database connected successfully")

	redisClient := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Address,
		Password: cfg.Redis.Password,
		DB:       0,
	})

	nRepo := repositories.NewRepository(&logger, db)
	nHandler := handlers.NewNotificationHandler(&logger, *nRepo, redisClient)
	hHandler := handlers.NewHealthHandler(&logger, redisClient, db)

	// app := app.NewApp(cfg, &logger, db, &redisClient)
	app := app.NewApp(cfg, &logger, redisClient, db, nRepo, nHandler, hHandler)
	r := routers.SetupRoutes(app)

	srv, err := server.New(app)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to initialize server")
	}

	srv.SetupHTTPServer(r)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	// Start server
	go func() {
		logger.Info().Msg("Starting HTTP server...")
		if err = srv.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Fatal().Err(err).Msg("failed to start server")
		}
	}()

	logger.Info().Msg("Server is ready to accept connections")

	// Wait for interrupt signal
	<-ctx.Done()

	logger.Info().Msg("Shutdown signal received, starting graceful shutdown...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), DefaultContextTimeout*time.Second)
	defer cancel()

	if err = srv.Shutdown(shutdownCtx); err != nil {
		logger.Fatal().Err(err).Msg("server forced to shutdown")
	}

	logger.Info().Msg("server exited properly")
}

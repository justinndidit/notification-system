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
	"github.com/justinndidit/notificationSystem/orchestrator/internal/auth"
	"github.com/justinndidit/notificationSystem/orchestrator/internal/config"
	"github.com/justinndidit/notificationSystem/orchestrator/internal/database"
	"github.com/justinndidit/notificationSystem/orchestrator/internal/handlers"
	"github.com/justinndidit/notificationSystem/orchestrator/internal/logger"
	"github.com/justinndidit/notificationSystem/orchestrator/internal/routers"
	"github.com/justinndidit/notificationSystem/orchestrator/internal/server"
	"github.com/justinndidit/notificationSystem/orchestrator/internal/services"
	"github.com/justinndidit/notificationSystem/orchestrator/internal/tracing"
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

	// Tracing is optional: without a collector configured the service runs
	// untraced rather than refusing to start, so observability tooling can
	// never take down notification delivery.
	shutdownTracing, err := tracing.Init(context.Background(), &logger)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to initialise tracing")
	}

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

	// Ensure the partition window is open before accepting traffic, then keep it
	// open. The notifications table is range-partitioned by created_at; without
	// this, inserts start failing once the calendar passes the last partition.
	partitionCtx, cancelPartitions := context.WithTimeout(context.Background(), 30*time.Second)
	if err = database.EnsurePartitions(partitionCtx, db, &logger, database.PartitionMonthsAhead); err != nil {
		cancelPartitions()
		logger.Fatal().Err(err).Msg("failed to ensure notification partitions")
	}
	cancelPartitions()

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
	rabbitConn, rabbitChannel, err := config.SetupRabbitMQ(cfg.RabbitMQ)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to initialize RabbitMQ")
	}
	defer rabbitChannel.Close()
	defer rabbitConn.Close()
	logger.Info().Msg("RabbitMQ connected successfully")

	// Service identity: short-lived signed tokens rather than a static shared
	// secret, so a leaked credential expires on its own and cannot be replayed
	// indefinitely.
	issuer, err := auth.NewTokenIssuer(cfg.External.JWTSecret, "orchestrator")
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to create the service token issuer")
	}

	serviceVerifier, err := auth.NewVerifier(cfg.External.JWTSecret)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to create the service token verifier")
	}

	// Initialize service clients
	logger.Info().Msg("Initializing service clients...")
	templateClient := services.NewTemplateClient(
		&logger,
		cfg.External.TemplateServiceAddress,
		issuer.Token,
	)
	userClient := services.NewUserClient(
		&logger,
		cfg.External.UserServiceAddress,
		issuer.Token,
	)

	// Initialize orchestrator
	logger.Info().Msg("Initializing orchestrator...")
	orchestrator := services.NewOrchestrator(
		&logger,
		templateClient,
		userClient,
		redisClient,
		rabbitConn,
		rabbitChannel,
		cfg.RabbitMQ,
		db.Pool,
	)

	// Initialize handlers
	logger.Info().Msg("Initializing handlers...")
	notificationHandler := handlers.NewNotificationHandler(&logger, redisClient, orchestrator)
	healthHandler := handlers.NewHealthHandler(&logger, redisClient, db)

	// Initialize app
	app := app.NewApp(cfg, &logger, redisClient, db, notificationHandler, healthHandler, serviceVerifier)

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

	// Keep the partition window open for the life of the process.
	database.StartPartitionMaintainer(ctx, db, &logger)

	// Drain the outbox into RabbitMQ. Enrichment commits the intent to publish;
	// this is what actually publishes it, so a broker outage delays delivery
	// rather than losing it.
	orchestrator.StartOutboxPublisher(ctx)

	// Pick up notifications abandoned mid-enrichment by a process that died
	// before its outbox entry was committed.
	orchestrator.StartRecoverySweeper(ctx)

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

	// Flush pending spans before exit, or the last trace of a shutdown is lost.
	flushCtx, cancelFlush := context.WithTimeout(context.Background(), 5*time.Second)
	if err = shutdownTracing(flushCtx); err != nil {
		logger.Error().Err(err).Msg("error flushing traces")
	}
	cancelFlush()

	logger.Info().Msg("Server exited properly")
}

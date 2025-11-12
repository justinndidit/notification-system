package internal

import (
	"context"
	"fmt"
	"net/http"
	"push-service/internal/models"

	"github.com/justinndidit/notificationSystem/orchestrator/internal/config"
	"github.com/rs/zerolog"
)

type App struct {
	config      *Config
	logger      *zerolog.Logger
	pushService *service.PushService
	consumer    *rabbitmq.Consumer
	httpServer  *http.Server
}

func New(cfg *config.Config, log *zerolog.Logger) (*App, error) {
	// Initialize push service
	pushService, err := service.NewPushService(cfg, log)
	if err != nil {
		return nil, fmt.Errorf("failed to create push service: %w", err)
	}

	app := &App{
		config:      cfg,
		logger:      log,
		pushService: pushService,
	}

	// Initialize RabbitMQ consumer
	consumer, err := rabbitmq.NewConsumer(&cfg.RabbitMQ, log, app.handleNotification)
	if err != nil {
		return nil, fmt.Errorf("failed to create RabbitMQ consumer: %w", err)
	}
	app.consumer = consumer

	// Setup HTTP server for health checks
	app.setupHTTPServer()

	return app, nil
}

func (a *App) handleNotification(ctx context.Context, msg *models.PushNotificationMessage) error {
	return a.pushService.ProcessNotification(ctx, msg)
}

func (a *App) setupHTTPServer() {
	mux := http.NewServeMux()

	// Health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"healthy","service":"push-service"}`))
	})

	// Readiness check endpoint
	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ready","service":"push-service"}`))
	})

	// Delivery status endpoint
	mux.HandleFunc("/status/", func(w http.ResponseWriter, r *http.Request) {
		notificationID := r.URL.Path[len("/status/"):]
		if notificationID == "" {
			http.Error(w, "notification_id required", http.StatusBadRequest)
			return
		}

		status, err := a.pushService.GetDeliveryStatus(r.Context(), notificationID)
		if err != nil {
			a.logger.Error("Failed to get delivery status",
				"notification_id", notificationID,
				"error", err)
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		// Manual JSON marshaling to avoid importing encoding/json again
		fmt.Fprintf(w, `{"notification_id":"%s","total_tokens":%d,"success_count":%d,"failure_count":%d}`,
			status.NotificationID, status.TotalTokens, status.SuccessCount, status.FailureCount)
	})

	a.httpServer = &http.Server{
		Addr:    ":" + a.config.Service.Port,
		Handler: mux,
	}
}

func (a *App) Start(ctx context.Context) error {
	// Start HTTP server in background
	go func() {
		a.logger.Info("Starting HTTP server", "port", a.config.Service.Port)
		if err := a.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			a.logger.Error("HTTP server error", "error", err)
		}
	}()

	// Start consuming messages (blocking)
	a.logger.Info("Starting RabbitMQ consumer...")
	return a.consumer.Start(ctx)
}

func (a *App) Shutdown(ctx context.Context) error {
	a.logger.Info("Shutting down application...")

	// Shutdown HTTP server
	if a.httpServer != nil {
		if err := a.httpServer.Shutdown(ctx); err != nil {
			a.logger.Error("Error shutting down HTTP server", "error", err)
		}
	}

	// Close RabbitMQ consumer
	if a.consumer != nil {
		if err := a.consumer.Close(); err != nil {
			a.logger.Error("Error closing RabbitMQ consumer", "error", err)
		}
	}

	// Close push service
	if a.pushService != nil {
		if err := a.pushService.Close(); err != nil {
			a.logger.Error("Error closing push service", "error", err)
		}
	}

	a.logger.Info("Application shutdown complete")
	return nil
}

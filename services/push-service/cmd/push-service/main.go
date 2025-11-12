package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"push-service/internal/app"
	"push-service/internal/config"
	"push-service/internal/logger"
)

func main() {
	// Initialize logger
	log := logger.New()
	log.Info("Starting Push Notification Service...")

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatal("Failed to load configuration", "error", err)
	}

	// Create context that listens for termination signals
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize and start the application
	application, err := app.New(cfg, log)
	if err != nil {
		log.Fatal("Failed to initialize application", "error", err)
	}

	// Start the push service in a goroutine
	go func() {
		if err := application.Start(ctx); err != nil {
			log.Error("Application error", "error", err)
			cancel()
		}
	}()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)

	<-sigChan
	log.Info("Shutdown signal received, gracefully shutting down...")

	// Give services time to cleanup
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := application.Shutdown(shutdownCtx); err != nil {
		log.Error("Error during shutdown", "error", err)
	}

	log.Info("Push service stopped")
}

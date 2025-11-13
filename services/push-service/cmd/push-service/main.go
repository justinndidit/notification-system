package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/justinndidit/notificationSystem/push-service/internal"
)

func main() {
	// Initialize logger
	log := internal.NewLogger("push-service")
	log.Info().Msg("Starting Push Notification Service...")

	// Load configuration
	cfg, err := internal.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load configuration")
	}

	// Create context that listens for termination signals
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize and start the application
	application, err := internal.NewApp(cfg, &log)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize application")
	}

	// Start the push service in a goroutine
	go func() {
		if err := application.Start(ctx); err != nil {
			log.Error().Err(err).Msg("Application error")
			cancel()
		}
	}()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)

	<-sigChan
	log.Info().Msg("Shutdown signal received, gracefully shutting down...")

	// Give services time to cleanup
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := application.Shutdown(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("Error during shutdown")
	}

	log.Info().Msg("Push service stopped")
}

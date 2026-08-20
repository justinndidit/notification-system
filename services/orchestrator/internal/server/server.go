package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/justinndidit/notificationSystem/orchestrator/internal/app"
)

type Server struct {
	App        *app.App
	httpServer *http.Server
}

func New(app *app.App) (*Server, error) {

	server := &Server{
		App: app,
	}

	return server, nil
}

func (s *Server) SetupHTTPServer(handler http.Handler) {
	// These are already time.Duration values parsed from strings like "15s".
	// Multiplying by time.Second again would yield ~475 years.
	s.httpServer = &http.Server{
		Addr:         ":" + s.App.Config.Server.Port,
		Handler:      handler,
		ReadTimeout:  s.App.Config.Server.ReadTimeout,
		WriteTimeout: s.App.Config.Server.WriteTimeout,
		IdleTimeout:  s.App.Config.Server.IdleTimeout,
	}
}

func (s *Server) Start() error {
	if s.httpServer == nil {
		return errors.New("HTTP server not initialized")
	}

	s.App.Logger.Info().
		Str("port", s.App.Config.Server.Port).
		Msg("starting server")

	return s.httpServer.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	if err := s.httpServer.Shutdown(ctx); err != nil {
		return fmt.Errorf("failed to shutdown HTTP server: %w", err)
	}

	s.App.Logger.Info().
		Str("addr", s.httpServer.Addr).
		Msg("HTTP server configured")

	s.App.DB.Close()

	return nil
}

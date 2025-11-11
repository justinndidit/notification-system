package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

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
	s.httpServer = &http.Server{
		Addr:         ":" + s.App.Config.Server.Port,
		Handler:      handler,
		ReadTimeout:  time.Duration(s.App.Config.Server.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(s.App.Config.Server.WriteTimeout) * time.Second,
		IdleTimeout:  time.Duration(s.App.Config.Server.IdleTimeout) * time.Second,
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

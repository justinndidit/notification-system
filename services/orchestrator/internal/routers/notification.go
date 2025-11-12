package routers

import (
	chi "github.com/go-chi/chi/v5"
	"github.com/justinndidit/notificationSystem/orchestrator/internal/app"
)

func SetupRoutes(app *app.App) *chi.Mux {
	r := chi.NewRouter()
	r.Post("/notification", app.NHandler.HandleNotificationRequest)
	r.Get("/health", app.HHandler.HandleHealthCheck)

	return r
}

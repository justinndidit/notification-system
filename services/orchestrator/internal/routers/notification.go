package routers

import (
	chi "github.com/go-chi/chi/v5"
	"github.com/justinndidit/notificationSystem/orchestrator/internal/app"
)

func SetupRoutes(app *app.App) *chi.Mux {
	r := chi.NewRouter()
	// Path must match the API Gateway's mount prefix: the gateway forwards the
	// full original path (including the prefix) rather than stripping it.
	r.Post("/notifications", app.NHandler.HandleNotificationRequest)
	r.Get("/health", app.HHandler.HandleHealthCheck)

	return r
}

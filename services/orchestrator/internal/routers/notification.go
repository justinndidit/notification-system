package routers

import (
	chi "github.com/go-chi/chi/v5"
	"github.com/justinndidit/notificationSystem/orchestrator/internal/app"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func SetupRoutes(app *app.App) *chi.Mux {
	r := chi.NewRouter()

	// Paths must match the API Gateway's mount prefix: the gateway forwards the
	// full original path (including the prefix) rather than stripping it.
	r.Route("/notifications", func(r chi.Router) {
		r.Post("/", app.NHandler.HandleNotificationRequest)
		r.Get("/", app.NHandler.HandleListNotifications)

		// Workers report delivery outcomes here, calling the orchestrator
		// directly rather than through the gateway, so this route carries its
		// own service-token check. Declared before /{id} so the literal segment
		// wins over the wildcard.
		r.With(requireServiceToken(app.Config.External.InternalToken)).
			Post("/status", app.NHandler.HandleStatusCallback)
		r.Get("/correlation/{correlationID}", app.NHandler.HandleGetNotificationByCorrelation)

		r.Get("/{id}", app.NHandler.HandleGetNotification)
		r.Get("/{id}/events", app.NHandler.HandleGetNotificationEvents)
		r.Post("/{id}/retry", app.NHandler.HandleRetryNotification)
	})

	r.Get("/health", app.HHandler.HandleHealthCheck)

	// Scraped by Prometheus. Unauthenticated, like the health check: it exposes
	// operational counters, not notification content. It should not be routed
	// through the public gateway.
	r.Handle("/metrics", promhttp.Handler())

	return r
}

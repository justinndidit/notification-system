package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/justinndidit/notificationSystem/orchestrator/internal/database"
	"github.com/justinndidit/notificationSystem/orchestrator/internal/utils"
	"github.com/rs/zerolog"
)

type HealthHandler struct {
	logger      *zerolog.Logger
	redisClient *redis.Client
	db          *database.Database
}

func NewHealthHandler(log *zerolog.Logger, rdb *redis.Client, db *database.Database) *HealthHandler {
	return &HealthHandler{
		logger:      log,
		redisClient: rdb,
		db:          db,
	}
}

func (h *HealthHandler) HandleHealthCheck(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	response := map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now().UTC(),
		"checks":    make(map[string]interface{}),
	}

	checks := response["checks"].(map[string]interface{})
	isHealthy := true

	// Check database connectivity
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	dbStart := time.Now()
	if err := h.db.Pool.Ping(ctx); err != nil {
		checks["database"] = map[string]interface{}{
			"status":        "unhealthy",
			"response_time": time.Since(dbStart).String(),
			"error":         err.Error(),
		}
		isHealthy = false
		h.logger.Error().Err(err).Dur("response_time", time.Since(dbStart)).Msg("database health check failed")
		//TODO:observability
	} else {
		checks["database"] = map[string]interface{}{
			"status":        "healthy",
			"response_time": time.Since(dbStart).String(),
		}
		h.logger.Info().Dur("response_time", time.Since(dbStart)).Msg("database health check passed")
	}

	if h.redisClient != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		redisStart := time.Now()
		if err := h.redisClient.Ping(ctx).Err(); err != nil {
			checks["redis"] = map[string]interface{}{
				"status":        "unhealthy",
				"response_time": time.Since(redisStart).String(),
				"error":         err.Error(),
			}
			h.logger.Error().Err(err).Dur("response_time", time.Since(redisStart)).Msg("redis health check failed")
		} else {
			checks["redis"] = map[string]interface{}{
				"status":        "healthy",
				"response_time": time.Since(redisStart).String(),
			}
			h.logger.Info().Dur("response_time", time.Since(redisStart)).Msg("redis health check passed")
		}
	}

	if !isHealthy {
		response["status"] = "unhealthy"
		h.logger.Warn().
			Dur("total_duration", time.Since(start)).
			Msg("health check failed")
		utils.WriteJsonHealthCheck(w, http.StatusServiceUnavailable, response)
		return
	}

	h.logger.Info().
		Dur("total_duration", time.Since(start)).
		Msg("health check passed")

	utils.WriteJsonHealthCheck(w, http.StatusOK, response)
}

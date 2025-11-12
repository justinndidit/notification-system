package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	validator "github.com/go-playground/validator/v10"
	redis "github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	"github.com/justinndidit/notificationSystem/orchestrator/internal/dtos"
	"github.com/justinndidit/notificationSystem/orchestrator/internal/services"
	"github.com/justinndidit/notificationSystem/orchestrator/internal/utils"
	"github.com/rs/zerolog"
)

type NotificationHandler struct {
	logger       *zerolog.Logger
	redisClient  *redis.Client
	orchestrator *services.Orchestrator
}

func NewNotificationHandler(log *zerolog.Logger, rdb *redis.Client, orchestrator *services.Orchestrator) *NotificationHandler {
	return &NotificationHandler{
		logger:       log,
		redisClient:  rdb,
		orchestrator: orchestrator,
	}
}

func (h *NotificationHandler) HandleNotificationRequest(w http.ResponseWriter, r *http.Request) {
	var body dtos.NotificationRequest
	defer r.Body.Close()

	// 1. Decode and validate request
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.logger.Error().Err(err).Msg("Error decoding request body")
		rb := utils.WriteResponseFailed(nil, err.Error(), "Invalid Request body", nil)
		utils.WriteJson(w, http.StatusBadRequest, rb)
		return
	}

	validate := validator.New()
	if err := validate.Struct(body); err != nil {
		h.logger.Error().Err(err).Msg("Failed to validate request body")
		rb := utils.WriteResponseFailed(nil, err.Error(), "Invalid Request body", nil)
		utils.WriteJson(w, http.StatusBadRequest, rb)
		return
	}

	// 2. Extract headers
	idempotencyKey := r.Header.Get("X-Idempotency-Key")
	correlationID := r.Header.Get("X-Correlation-ID")

	if idempotencyKey == "" {
		h.logger.Error().Msg("Missing X-Idempotency-Key header")
		rb := utils.WriteResponseFailed(nil, "Missing Idempotency key in requets header", "Invalid Request", nil)
		utils.WriteJson(w, http.StatusBadRequest, rb)
		return
	}

	if correlationID == "" {
		h.logger.Warn().Msg("No correlationID, generating...")
		correlationID = uuid.New().String()
	}

	// 3. Check idempotency
	val, err := h.redisClient.Get(r.Context(), idempotencyKey).Result()
	if err != nil && err != redis.Nil {
		h.logger.Error().Err(err).Msg("Error retrieving idempotency key from redis")
		rb := utils.WriteResponseFailed(nil, err.Error(), "Error retrieving idempotency key from cache", nil)
		utils.WriteJson(w, http.StatusInternalServerError, rb)
		return
	}

	if val != "" {
		h.logger.Info().Str("key", idempotencyKey).Msg("Duplicate request detected")

		data := map[string]any{
			"idempotency_key": idempotencyKey,
			"correlation_id":  val, // The stored correlationID
		}
		rb := utils.WriteResponseSuccess(data, "", "Duplicate request detected", nil)
		utils.WriteJson(w, http.StatusOK, rb)
		return
	}

	// 4. Cache idempotency key
	err = h.redisClient.Set(r.Context(), idempotencyKey, correlationID, 24*time.Hour).Err()
	if err != nil {
		h.logger.Error().Err(err).Str("key", idempotencyKey).Msg("Error caching idempotency key")
		rb := utils.WriteResponseFailed(nil, err.Error(), "Internal Server Error", nil)
		utils.WriteJson(w, http.StatusInternalServerError, rb)
		return
	}

	h.logger.Info().Msg("Passed")

	// 5. Enrich notification data asynchronously
	go h.orchestrator.EnrichAndPublish(context.Background(), body, correlationID, idempotencyKey)

	// 6. Return immediate response
	data := map[string]any{
		"correlation_id":  correlationID,
		"idempotency_key": idempotencyKey,
		"status":          "processing",
	}
	rb := utils.WriteResponseSuccess(data, "", "Notification accepted and being processed", nil)
	utils.WriteJson(w, http.StatusAccepted, rb)
}

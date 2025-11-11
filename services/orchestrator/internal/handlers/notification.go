package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	validator "github.com/go-playground/validator/v10"
	redis "github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	"github.com/justinndidit/notificationSystem/orchestrator/internal/dtos"
	"github.com/justinndidit/notificationSystem/orchestrator/internal/repositories"
	"github.com/justinndidit/notificationSystem/orchestrator/internal/utils"
	"github.com/rs/zerolog"
)

type NotificationHandler struct {
	logger      *zerolog.Logger
	repo        repositories.NotificationRepo
	redisClient *redis.Client
}

func NewNotificationHandler(log *zerolog.Logger, repo repositories.NotificationRepo, rdb *redis.Client) *NotificationHandler {
	return &NotificationHandler{
		logger:      log,
		repo:        repo,
		redisClient: rdb,
	}
}

func (h *NotificationHandler) HandleNotificationRequest(w http.ResponseWriter, r *http.Request) {
	var body dtos.NotificationRequest
	defer r.Body.Close()

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.logger.Error().Err(err).Msg("Error decoding request body")
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	validate := validator.New()
	if err := validate.Struct(body); err != nil {
		h.logger.Error().Err(err).Msg("Failed to validate request body")
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	idempotencyKey := r.Header.Get("X-Idempotency-Key")
	correlationID := r.Header.Get("X-Correlation-ID")

	if idempotencyKey == "" {
		h.logger.Error().Msg("Missing X-Idempotency-Key header")
		http.Error(w, "X-Idempotency-Key header required", http.StatusBadRequest)
		return
	}

	if correlationID == "" {
		h.logger.Warn().Msg("No correlationID, generating...")
		correlationID = uuid.New().String()
	}

	// Check idempotency
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
		}
		//rb := utils.WriteResponse("Already Processed", http.StatusOK, "Duplicate request detected", data, nil)
		rb := utils.WriteResponseSuccess(data, "", "Duplicate request detected", nil)
		utils.WriteJson(w, http.StatusOK, rb)
		return
	}

	err = h.redisClient.Set(r.Context(), idempotencyKey, correlationID, 1*time.Hour).Err()
	if err != nil {
		h.logger.Error().Err(err).Str("key", idempotencyKey).Msg("Error caching idempotency key")

		rb := utils.WriteResponseFailed(nil, err.Error(), "Internal Server Error", nil)
		//rb := utils.WriteResponse("Server Error", http.StatusInternalServerError, "Internal server error", nil, err.Error())
		utils.WriteJson(w, http.StatusInternalServerError, rb)
		return
	}

	// Process notification...
	// TODO: Send to queue, process async, etc.

	data := map[string]any{
		"correlation_id": correlationID,
		//TODO: enrich data field
	}
	rb := utils.WriteResponseSuccess(data, "", "Notification is being processed, Please check back later", nil)
	//rb := utils.WriteResponse("accepted", http.StatusAccepted, "Notification is being processed, please checkbacklater",data,nil)
	utils.WriteJson(w, http.StatusAccepted, rb)
}

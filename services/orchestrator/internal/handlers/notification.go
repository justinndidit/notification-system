package handlers

import (
	"context"
	"encoding/json"
	"net/http"

	validator "github.com/go-playground/validator/v10"
	redis "github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	"github.com/justinndidit/notificationSystem/orchestrator/internal/dtos"
	"github.com/justinndidit/notificationSystem/orchestrator/internal/metrics"
	"github.com/justinndidit/notificationSystem/orchestrator/internal/services"
	"github.com/justinndidit/notificationSystem/orchestrator/internal/utils"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/trace"
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

	// The correlation_id column is UUID NOT NULL, so a non-UUID header cannot be
	// persisted. Reject it here rather than silently substituting a new value,
	// which would sever the caller's ability to correlate the request.
	var correlationUUID uuid.UUID
	if correlationID == "" {
		h.logger.Warn().Msg("No correlationID, generating...")
		correlationUUID = uuid.New()
		correlationID = correlationUUID.String()
	} else {
		parsed, err := uuid.Parse(correlationID)
		if err != nil {
			h.logger.Error().Err(err).Str("correlation_id", correlationID).Msg("Invalid X-Correlation-ID header")
			rb := utils.WriteResponseFailed(nil, err.Error(), "X-Correlation-ID must be a UUID", nil)
			utils.WriteJson(w, http.StatusBadRequest, rb)
			return
		}
		correlationUUID = parsed
	}

	// 3. Claim the idempotency key atomically. SETNX both detects duplicates and
	// reserves the key in one round trip; the previous GET-then-SET let two
	// concurrent requests observe an empty key and both proceed.
	claimed, existing, err := h.orchestrator.Idempotency().Claim(r.Context(), idempotencyKey, correlationID)
	if err != nil {
		h.logger.Error().Err(err).Str("key", idempotencyKey).Msg("Error claiming idempotency key")
		rb := utils.WriteResponseFailed(nil, err.Error(), "Internal Server Error", nil)
		utils.WriteJson(w, http.StatusInternalServerError, rb)
		return
	}

	if !claimed {
		metrics.DuplicatesSuppressed.Inc()
		h.logger.Info().Str("key", idempotencyKey).Msg("Duplicate request detected")

		data := map[string]any{
			"idempotency_key": idempotencyKey,
		}
		if existing != nil {
			data["correlation_id"] = existing.CorrelationID
			data["status"] = existing.State
			if existing.NotificationID != "" {
				data["notification_id"] = existing.NotificationID
			}
		}

		rb := utils.WriteResponseSuccess(data, "", "Duplicate request detected", nil)
		utils.WriteJson(w, http.StatusOK, rb)
		return
	}

	h.logger.Info().Msg("Passed")

	// 5. Enrich notification data asynchronously
	// Enrichment outlives the request, so it cannot use the request context —
	// that is cancelled the moment the 202 is written. Carrying the span context
	// onto a background context keeps the work in the caller's trace without
	// inheriting its cancellation, so one trace runs from submission through to
	// delivery rather than breaking at the async boundary.
	enrichCtx := trace.ContextWithSpanContext(
		context.Background(),
		trace.SpanContextFromContext(r.Context()),
	)

	go h.orchestrator.EnrichAndPublish(enrichCtx, body, correlationUUID, idempotencyKey)

	// 6. Return immediate response
	data := map[string]any{
		"correlation_id":  correlationID,
		"idempotency_key": idempotencyKey,
		"status":          "processing",
	}
	rb := utils.WriteResponseSuccess(data, "", "Notification accepted and being processed", nil)
	utils.WriteJson(w, http.StatusAccepted, rb)
}

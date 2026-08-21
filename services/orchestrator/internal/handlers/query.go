package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	validator "github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/justinndidit/notificationSystem/orchestrator/internal/dtos"
	"github.com/justinndidit/notificationSystem/orchestrator/internal/services"
	"github.com/justinndidit/notificationSystem/orchestrator/internal/utils"
)

// HandleGetNotification serves GET /notifications/{id}
func (h *NotificationHandler) HandleGetNotification(w http.ResponseWriter, r *http.Request) {
	id, ok := h.parseIDParam(w, r)
	if !ok {
		return
	}

	view, err := h.orchestrator.GetNotification(r.Context(), id)
	if err != nil {
		h.writeLookupError(w, err, "notification")
		return
	}

	utils.WriteJson(w, http.StatusOK,
		utils.WriteResponseSuccess(view, "", "Notification retrieved", nil))
}

// HandleGetNotificationByCorrelation serves GET /notifications/correlation/{correlationID}
//
// This is the polling endpoint: callers receive a correlation ID at submission
// time, before the notification row exists.
func (h *NotificationHandler) HandleGetNotificationByCorrelation(w http.ResponseWriter, r *http.Request) {
	correlationID := chi.URLParam(r, "correlationID")

	if _, err := uuid.Parse(correlationID); err != nil {
		rb := utils.WriteResponseFailed(nil, err.Error(), "correlation_id must be a UUID", nil)
		utils.WriteJson(w, http.StatusBadRequest, rb)
		return
	}

	view, cached, err := h.orchestrator.GetNotificationByCorrelation(r.Context(), correlationID)
	if err != nil {
		h.writeLookupError(w, err, "notification")
		return
	}

	data := map[string]any{"correlation_id": correlationID}
	if view != nil {
		data["notification"] = view
		data["status"] = view.Status
	}
	if cached != nil {
		// Surfaced separately so a caller can tell a cached transition from the
		// persisted record when the two have not converged yet.
		data["cached_status"] = cached
		if view == nil {
			data["status"] = cached.Status
		}
	}

	utils.WriteJson(w, http.StatusOK,
		utils.WriteResponseSuccess(data, "", "Notification status retrieved", nil))
}

// HandleListNotifications serves GET /notifications?user_id=&limit=&cursor=
func (h *NotificationHandler) HandleListNotifications(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		rb := utils.WriteResponseFailed(nil, "user_id is required", "Missing user_id", nil)
		utils.WriteJson(w, http.StatusBadRequest, rb)
		return
	}
	if _, err := uuid.Parse(userID); err != nil {
		rb := utils.WriteResponseFailed(nil, err.Error(), "user_id must be a UUID", nil)
		utils.WriteJson(w, http.StatusBadRequest, rb)
		return
	}

	limit := 0
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 {
			rb := utils.WriteResponseFailed(nil, "limit must be a positive integer", "Invalid limit", nil)
			utils.WriteJson(w, http.StatusBadRequest, rb)
			return
		}
		limit = parsed
	}

	var cursor *time.Time
	if raw := r.URL.Query().Get("cursor"); raw != "" {
		parsed, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			rb := utils.WriteResponseFailed(nil, err.Error(), "cursor must be an RFC3339 timestamp", nil)
			utils.WriteJson(w, http.StatusBadRequest, rb)
			return
		}
		cursor = &parsed
	}

	views, page, err := h.orchestrator.ListUserNotifications(r.Context(), userID, limit, cursor)
	if err != nil {
		h.logger.Error().Err(err).Str("user_id", userID).Msg("Failed to list notifications")
		rb := utils.WriteResponseFailed(nil, err.Error(), "Failed to list notifications", nil)
		utils.WriteJson(w, http.StatusInternalServerError, rb)
		return
	}

	data := map[string]any{
		"notifications": views,
		"page":          page,
	}

	utils.WriteJson(w, http.StatusOK,
		utils.WriteResponseSuccess(data, "", "Notifications retrieved", nil))
}

// HandleGetNotificationEvents serves GET /notifications/{id}/events
func (h *NotificationHandler) HandleGetNotificationEvents(w http.ResponseWriter, r *http.Request) {
	id, ok := h.parseIDParam(w, r)
	if !ok {
		return
	}

	events, err := h.orchestrator.GetNotificationEvents(r.Context(), id)
	if err != nil {
		h.writeLookupError(w, err, "notification")
		return
	}

	data := map[string]any{
		"notification_id": id.String(),
		"events":          events,
	}

	utils.WriteJson(w, http.StatusOK,
		utils.WriteResponseSuccess(data, "", "Events retrieved", nil))
}

// HandleRetryNotification serves POST /notifications/{id}/retry
func (h *NotificationHandler) HandleRetryNotification(w http.ResponseWriter, r *http.Request) {
	id, ok := h.parseIDParam(w, r)
	if !ok {
		return
	}

	view, err := h.orchestrator.RetryNotification(r.Context(), id)
	switch {
	case errors.Is(err, services.ErrNotFound):
		rb := utils.WriteResponseFailed(nil, err.Error(), "Notification not found", nil)
		utils.WriteJson(w, http.StatusNotFound, rb)
		return
	case errors.Is(err, services.ErrNotRetryable):
		// 409: the request is well-formed, the notification is just not in a
		// state where retrying means anything.
		rb := utils.WriteResponseFailed(nil, err.Error(), "Notification cannot be retried", nil)
		utils.WriteJson(w, http.StatusConflict, rb)
		return
	case err != nil:
		h.logger.Error().Err(err).Str("notification_id", id.String()).Msg("Retry failed")
		rb := utils.WriteResponseFailed(nil, err.Error(), "Failed to retry notification", nil)
		utils.WriteJson(w, http.StatusInternalServerError, rb)
		return
	}

	utils.WriteJson(w, http.StatusAccepted,
		utils.WriteResponseSuccess(view, "", "Notification requeued", nil))
}

// HandleStatusCallback serves POST /notifications/status
//
// Channel workers report delivery outcomes here. Without it a notification stays
// at "queued" regardless of what actually happened downstream.
func (h *NotificationHandler) HandleStatusCallback(w http.ResponseWriter, r *http.Request) {
	var body dtos.StatusCallbackRequest
	defer r.Body.Close()

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		rb := utils.WriteResponseFailed(nil, err.Error(), "Invalid request body", nil)
		utils.WriteJson(w, http.StatusBadRequest, rb)
		return
	}

	if err := validator.New().Struct(body); err != nil {
		rb := utils.WriteResponseFailed(nil, err.Error(), "Invalid request body", nil)
		utils.WriteJson(w, http.StatusBadRequest, rb)
		return
	}

	err := h.orchestrator.RecordDeliveryStatus(r.Context(), body)
	switch {
	case errors.Is(err, services.ErrNotFound):
		rb := utils.WriteResponseFailed(nil, err.Error(), "Notification not found", nil)
		utils.WriteJson(w, http.StatusNotFound, rb)
		return
	case err != nil:
		h.logger.Error().Err(err).
			Str("notification_id", body.NotificationID).
			Str("status", body.Status).
			Msg("Failed to record delivery status")
		rb := utils.WriteResponseFailed(nil, err.Error(), "Failed to record status", nil)
		utils.WriteJson(w, http.StatusBadRequest, rb)
		return
	}

	data := map[string]any{
		"notification_id": body.NotificationID,
		"status":          body.Status,
	}

	utils.WriteJson(w, http.StatusOK,
		utils.WriteResponseSuccess(data, "", "Status recorded", nil))
}

// parseIDParam reads and validates the {id} path parameter.
func (h *NotificationHandler) parseIDParam(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	raw := chi.URLParam(r, "id")

	id, err := uuid.Parse(raw)
	if err != nil {
		rb := utils.WriteResponseFailed(nil, err.Error(), "notification id must be a UUID", nil)
		utils.WriteJson(w, http.StatusBadRequest, rb)
		return uuid.UUID{}, false
	}

	return id, true
}

// writeLookupError maps a service error onto a status code.
func (h *NotificationHandler) writeLookupError(w http.ResponseWriter, err error, subject string) {
	if errors.Is(err, services.ErrNotFound) {
		rb := utils.WriteResponseFailed(nil, err.Error(), "Notification not found", nil)
		utils.WriteJson(w, http.StatusNotFound, rb)
		return
	}

	h.logger.Error().Err(err).Msgf("Failed to retrieve %s", subject)
	rb := utils.WriteResponseFailed(nil, err.Error(), "Failed to retrieve notification", nil)
	utils.WriteJson(w, http.StatusInternalServerError, rb)
}

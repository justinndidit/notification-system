package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/justinndidit/notificationSystem/orchestrator/internal/dtos"
	"github.com/justinndidit/notificationSystem/orchestrator/internal/models"
	"github.com/justinndidit/notificationSystem/orchestrator/internal/repositories"
)

// ErrNotFound is returned when a notification does not exist, so handlers can
// map it to 404 without inspecting error strings.
var ErrNotFound = errors.New("notification not found")

// ErrNotRetryable is returned when a retry is requested for a notification that
// is not in a retryable state.
var ErrNotRetryable = errors.New("notification is not retryable")

const defaultPageLimit = 20
const maxPageLimit = 100

// GetNotification returns one notification by its ID.
func (o *Orchestrator) GetNotification(ctx context.Context, id uuid.UUID) (*dtos.NotificationView, error) {
	notif, err := o.notifRepo.GetByID(ctx, id)
	if err != nil {
		return nil, translateNotFound(err)
	}

	view := toNotificationView(notif)
	return &view, nil
}

// GetNotificationByCorrelation resolves a notification from the correlation ID
// the caller was handed at submission time.
//
// Redis is consulted first: this is the polling path, and a status cache entry
// is written on every transition. Postgres is the fallback and the source of
// truth — the cache only ever short-circuits a lookup, never contradicts it.
func (o *Orchestrator) GetNotificationByCorrelation(ctx context.Context, correlationID string) (*dtos.NotificationView, *CachedStatus, error) {
	cached := o.readCachedStatus(ctx, correlationID)

	notif, err := o.notifRepo.GetByCorrelationID(ctx, correlationID)
	if errors.Is(err, repositories.ErrNotFound) {
		// A cached status with no row means enrichment failed before the insert.
		if cached != nil {
			return nil, cached, nil
		}
		return nil, nil, ErrNotFound
	}
	if err != nil {
		return nil, cached, err
	}

	view := toNotificationView(notif)
	return &view, cached, nil
}

// ListUserNotifications returns a page of a user's notifications, newest first.
func (o *Orchestrator) ListUserNotifications(
	ctx context.Context,
	userID string,
	limit int,
	cursor *time.Time,
) ([]dtos.NotificationView, dtos.CursorPage, error) {
	if limit <= 0 {
		limit = defaultPageLimit
	}
	if limit > maxPageLimit {
		limit = maxPageLimit
	}

	notifs, next, err := o.notifRepo.GetUserNotificationsWithCursor(ctx, userID, limit, cursor)
	if err != nil {
		return nil, dtos.CursorPage{}, err
	}

	views := make([]dtos.NotificationView, 0, len(notifs))
	for i := range notifs {
		views = append(views, toNotificationView(&notifs[i]))
	}

	return views, dtos.CursorPage{
		NextCursor: next,
		HasMore:    next != nil,
		Limit:      limit,
	}, nil
}

// GetNotificationEvents returns the audit timeline for one notification.
func (o *Orchestrator) GetNotificationEvents(ctx context.Context, id uuid.UUID) ([]dtos.NotificationEventView, error) {
	if _, err := o.notifRepo.GetByID(ctx, id); err != nil {
		return nil, translateNotFound(err)
	}

	events, err := o.eventRepo.GetEventsByNotificationID(ctx, id)
	if err != nil {
		return nil, err
	}

	views := make([]dtos.NotificationEventView, 0, len(events))
	for _, e := range events {
		views = append(views, dtos.NotificationEventView{
			EventType:     e.EventType,
			Channel:       e.Channel,
			EventData:     e.EventData,
			Provider:      e.Provider,
			ProviderMsgID: e.ProviderMsgID,
			EventAt:       e.EventAt,
		})
	}

	return views, nil
}

// RecordDeliveryStatus applies a delivery outcome reported by a channel worker.
//
// This is what closes the loop: until a worker can report back, a notification
// is stuck at "queued" no matter what actually happened to it.
func (o *Orchestrator) RecordDeliveryStatus(ctx context.Context, req dtos.StatusCallbackRequest) error {
	notifID, err := uuid.Parse(req.NotificationID)
	if err != nil {
		return fmt.Errorf("invalid notification_id: %w", err)
	}

	notif, err := o.notifRepo.GetByID(ctx, notifID)
	if err != nil {
		return translateNotFound(err)
	}

	status, eventType, ok := mapCallbackStatus(req.Status)
	if !ok {
		return fmt.Errorf("unsupported status %q", req.Status)
	}

	if status == dtos.StatusFailed {
		errMsg := req.Error
		if errMsg == "" {
			errMsg = "delivery failed"
		}
		if err := o.notifRepo.UpdateFailure(ctx, notifID, "DELIVERY_ERROR", errMsg); err != nil {
			return err
		}
	} else if err := o.notifRepo.UpdateStatus(ctx, notifID, status); err != nil {
		return err
	}

	eventData := models.JSONMap{"reported_by": "worker"}
	if req.Error != "" {
		eventData["error"] = req.Error
	}
	if req.Provider != "" {
		eventData["provider"] = req.Provider
	}
	if req.ProviderMsgID != "" {
		eventData["provider_message_id"] = req.ProviderMsgID
	}

	o.eventRepo.CreateEventSimple(ctx, notifID, notif.CorrelationID, eventType, eventData)
	o.storeNotificationStatus(ctx, notif.CorrelationID.String(), status, req.Error)

	o.logger.Info().
		Str("notification_id", req.NotificationID).
		Str("correlation_id", notif.CorrelationID.String()).
		Str("status", status).
		Msg("Recorded delivery status")

	return nil
}

// RetryNotification republishes a failed notification.
//
// The message stored at enrichment time is republished verbatim rather than
// rebuilt: re-deriving it would silently pick up a template or preference that
// has changed since, which is not what "retry" should mean.
func (o *Orchestrator) RetryNotification(ctx context.Context, id uuid.UUID) (*dtos.NotificationView, error) {
	notif, err := o.notifRepo.GetByID(ctx, id)
	if err != nil {
		return nil, translateNotFound(err)
	}

	if notif.Status != dtos.StatusFailed {
		return nil, fmt.Errorf("%w: status is %q, expected %q", ErrNotRetryable, notif.Status, dtos.StatusFailed)
	}
	if notif.RetryCount >= notif.MaxRetries {
		return nil, fmt.Errorf("%w: retry count %d has reached the maximum of %d",
			ErrNotRetryable, notif.RetryCount, notif.MaxRetries)
	}
	if len(notif.EnrichedPayload) == 0 {
		return nil, fmt.Errorf("%w: no stored payload to republish", ErrNotRetryable)
	}

	var enriched dtos.EnrichedNotification
	raw, err := json.Marshal(notif.EnrichedPayload)
	if err != nil {
		return nil, fmt.Errorf("failed to read stored payload: %w", err)
	}
	if err := json.Unmarshal(raw, &enriched); err != nil {
		return nil, fmt.Errorf("stored payload is not a valid enriched notification: %w", err)
	}

	if err := o.publishToQueue(ctx, enriched); err != nil {
		return nil, err
	}

	if err := o.notifRepo.UpdateStatus(ctx, id, dtos.StatusQueued); err != nil {
		return nil, err
	}

	o.eventRepo.CreateEventSimple(ctx, id, notif.CorrelationID, dtos.EventRetried, models.JSONMap{
		"previous_status": notif.Status,
		"retry_count":     notif.RetryCount,
	})
	o.storeNotificationStatus(ctx, notif.CorrelationID.String(), dtos.StatusQueued, "")

	o.logger.Info().
		Str("notification_id", id.String()).
		Str("correlation_id", notif.CorrelationID.String()).
		Msg("Notification requeued")

	refreshed, err := o.notifRepo.GetByID(ctx, id)
	if err != nil {
		view := toNotificationView(notif)
		return &view, nil
	}

	view := toNotificationView(refreshed)
	return &view, nil
}

// translateNotFound maps the repository's not-found sentinel onto this
// package's, so handlers depend only on the service layer.
func translateNotFound(err error) error {
	if errors.Is(err, repositories.ErrNotFound) {
		return ErrNotFound
	}
	return err
}

// CachedStatus is the Redis-cached view of a notification's progress.
type CachedStatus struct {
	Status    string `json:"status"`
	Error     string `json:"error,omitempty"`
	UpdatedAt int64  `json:"updated_at"`
}

func (o *Orchestrator) readCachedStatus(ctx context.Context, correlationID string) *CachedStatus {
	key := fmt.Sprintf("notification:status:%s", correlationID)

	raw, err := o.redisClient.Get(ctx, key).Bytes()
	if err != nil {
		// A cache miss or a Redis outage is not an error here: Postgres answers
		// the query either way.
		return nil
	}

	var cached CachedStatus
	if err := json.Unmarshal(raw, &cached); err != nil {
		o.logger.Warn().Err(err).Str("key", key).Msg("Discarding malformed cached status")
		return nil
	}

	return &cached
}

// mapCallbackStatus translates a worker-reported status into the stored status
// and the event to append.
func mapCallbackStatus(reported string) (status, eventType string, ok bool) {
	switch reported {
	case "sent":
		return dtos.StatusSent, dtos.EventSent, true
	case "delivered":
		return dtos.StatusSent, dtos.EventDelivered, true
	case "failed":
		return dtos.StatusFailed, dtos.EventFailed, true
	case "bounced":
		return dtos.StatusFailed, dtos.EventBounced, true
	default:
		return "", "", false
	}
}

func toNotificationView(n *models.Notification) dtos.NotificationView {
	return dtos.NotificationView{
		NotificationID: n.ID.String(),
		CorrelationID:  n.CorrelationID.String(),
		IdempotencyKey: n.IdempotencyKey,
		UserID:         n.UserID.String(),
		TemplateID:     n.TemplateID.String(),
		Channel:        string(n.Channel),
		Status:         n.Status,
		Priority:       n.Priority,
		Recipient:      n.Recipient,
		RetryCount:     n.RetryCount,
		MaxRetries:     n.MaxRetries,
		ErrorCode:      n.ErrorCode,
		ErrorMessage:   n.ErrorMessage,
		Provider:       n.Provider,
		EnrichedAt:     n.EnrichedAt,
		QueuedAt:       n.QueuedAt,
		SentAt:         n.SentAt,
		DeliveredAt:    n.DeliveredAt,
		FailedAt:       n.FailedAt,
		CreatedAt:      n.CreatedAt,
		UpdatedAt:      n.UpdatedAt,
	}
}

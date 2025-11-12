package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justinndidit/notificationSystem/orchestrator/internal/models"
	"github.com/rs/zerolog"
)

type EventRepository struct {
	pool   *pgxpool.Pool
	logger *zerolog.Logger
}

func NewEventRepository(pool *pgxpool.Pool, logger *zerolog.Logger) *EventRepository {
	return &EventRepository{
		pool:   pool,
		logger: logger,
	}
}

// CreateEvent records a notification event
func (r *EventRepository) CreateEvent(ctx context.Context, event *models.NotificationEvent) error {
	query := `
		INSERT INTO notification_events (
			id, notification_id, correlation_id, event_type, channel,
			event_data, provider, provider_message_id, user_agent, ip_address, event_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
		)
	`

	_, err := r.pool.Exec(ctx, query,
		event.ID,
		event.NotificationID,
		event.CorrelationID,
		event.EventType,
		event.Channel,
		event.EventData,
		event.Provider,
		event.ProviderMsgID,
		event.UserAgent,
		event.IPAddress,
		event.EventAt,
	)

	if err != nil {
		r.logger.Error().Err(err).Msg("Failed to create event")
		return fmt.Errorf("failed to create event: %w", err)
	}

	return nil
}

// CreateEventSimple is a helper for creating events with minimal data
func (r *EventRepository) CreateEventSimple(ctx context.Context, notificationID, correlationID uuid.UUID, eventType string, eventData models.JSONMap) error {
	event := &models.NotificationEvent{
		ID:             uuid.New(),
		NotificationID: notificationID,
		CorrelationID:  correlationID,
		EventType:      eventType,
		EventData:      eventData,
		EventAt:        time.Now(),
	}

	return r.CreateEvent(ctx, event)
}

// GetEventsByNotificationID retrieves all events for a notification
func (r *EventRepository) GetEventsByNotificationID(ctx context.Context, notificationID uuid.UUID) ([]models.NotificationEvent, error) {
	query := `
		SELECT
			id, notification_id, correlation_id, event_type, channel,
			event_data, provider, provider_message_id, user_agent, ip_address, event_at
		FROM notification_events
		WHERE notification_id = $1
		ORDER BY event_at ASC
	`

	rows, err := r.pool.Query(ctx, query, notificationID)
	if err != nil {
		r.logger.Error().Err(err).Str("notification_id", notificationID.String()).Msg("Failed to get events")
		return nil, fmt.Errorf("failed to get events: %w", err)
	}
	defer rows.Close()

	events := make([]models.NotificationEvent, 0)
	for rows.Next() {
		var event models.NotificationEvent
		err := rows.Scan(
			&event.ID,
			&event.NotificationID,
			&event.CorrelationID,
			&event.EventType,
			&event.Channel,
			&event.EventData,
			&event.Provider,
			&event.ProviderMsgID,
			&event.UserAgent,
			&event.IPAddress,
			&event.EventAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan event: %w", err)
		}
		events = append(events, event)
	}

	return events, nil
}

// GetEventsByCorrelationID retrieves all events for a correlation ID
func (r *EventRepository) GetEventsByCorrelationID(ctx context.Context, correlationID uuid.UUID) ([]models.NotificationEvent, error) {
	query := `
		SELECT
			id, notification_id, correlation_id, event_type, channel,
			event_data, provider, provider_message_id, user_agent, ip_address, event_at
		FROM notification_events
		WHERE correlation_id = $1
		ORDER BY event_at ASC
	`

	rows, err := r.pool.Query(ctx, query, correlationID)
	if err != nil {
		r.logger.Error().Err(err).Str("correlation_id", correlationID.String()).Msg("Failed to get events")
		return nil, fmt.Errorf("failed to get events: %w", err)
	}
	defer rows.Close()

	events := make([]models.NotificationEvent, 0)
	for rows.Next() {
		var event models.NotificationEvent
		err := rows.Scan(
			&event.ID,
			&event.NotificationID,
			&event.CorrelationID,
			&event.EventType,
			&event.Channel,
			&event.EventData,
			&event.Provider,
			&event.ProviderMsgID,
			&event.UserAgent,
			&event.IPAddress,
			&event.EventAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan event: %w", err)
		}
		events = append(events, event)
	}

	return events, nil
}

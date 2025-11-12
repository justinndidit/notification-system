package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justinndidit/notificationSystem/orchestrator/internal/dtos"
	"github.com/justinndidit/notificationSystem/orchestrator/internal/models"
	"github.com/rs/zerolog"
)

type NotificationRepository struct {
	pool   *pgxpool.Pool
	logger *zerolog.Logger
}

func NewNotificationRepository(pool *pgxpool.Pool, logger *zerolog.Logger) *NotificationRepository {
	return &NotificationRepository{
		pool:   pool,
		logger: logger,
	}
}

// CreateNotification inserts a new notification with idempotency check
func (r *NotificationRepository) CreateNotification(ctx context.Context, notif *models.Notification) error {
	query := `
		INSERT INTO notifications (
			id, user_id, template_id, correlation_id, idempotency_key,
			channel, status, priority, variables, metadata, enriched_payload,
			recipient, retry_count, max_retries, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16
		)
	`

	result, err := r.pool.Exec(ctx, query,
		notif.ID,
		notif.UserID,
		notif.TemplateID,
		notif.CorrelationID,
		notif.IdempotencyKey,
		notif.Channel,
		notif.Status,
		notif.Priority,
		notif.Variables,
		notif.Metadata,
		notif.EnrichedPayload,
		notif.Recipient,
		notif.RetryCount,
		notif.MaxRetries,
		notif.CreatedAt,
		notif.UpdatedAt,
	)

	if err != nil {
		r.logger.Error().Err(err).Msg("Failed to create notification")
		return fmt.Errorf("failed to create notification: %w", err)
	}

	// Check if actually inserted (not a duplicate)
	if result.RowsAffected() == 0 {
		r.logger.Warn().
			Str("idempotency_key", *notif.IdempotencyKey).
			Msg("Duplicate notification creation attempt")
		return fmt.Errorf("duplicate notification with idempotency key")
	}

	r.logger.Info().
		Str("notification_id", notif.ID.String()).
		Str("correlation_id", notif.CorrelationID.String()).
		Msg("Notification created successfully")

	return nil
}

// UpdateStatusWithTimestamp updates notification status with appropriate timestamp
// This consolidates status updates to prevent race conditions
func (r *NotificationRepository) UpdateStatusWithTimestamp(ctx context.Context, id uuid.UUID, status string) error {
	var timestampColumn string
	switch status {
	case "enriching":
		timestampColumn = "enriched_at"
	case "queued":
		timestampColumn = "queued_at"
	case "sent":
		timestampColumn = "sent_at"
	case "delivered":
		timestampColumn = "delivered_at"
	case "failed":
		timestampColumn = "failed_at"
	default:
		timestampColumn = ""
	}

	var query string
	if timestampColumn != "" {
		query = fmt.Sprintf(`
			UPDATE notifications
			SET status = $1,
				%s = COALESCE(%s, NOW()),
				updated_at = NOW()
			WHERE id = $2 AND deleted_at IS NULL
		`, timestampColumn, timestampColumn)
	} else {
		query = `
			UPDATE notifications
			SET status = $1,
				updated_at = NOW()
			WHERE id = $2 AND deleted_at IS NULL
		`
	}

	result, err := r.pool.Exec(ctx, query, status, id)
	if err != nil {
		r.logger.Error().Err(err).Str("id", id.String()).Str("status", status).Msg("Failed to update status")
		return fmt.Errorf("failed to update status: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("notification not found or already deleted")
	}

	return nil
}

// UpdateStatus updates notification status (kept for backward compatibility)
func (r *NotificationRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status string) error {
	return r.UpdateStatusWithTimestamp(ctx, id, status)
}

// UpdateEnrichedPayload stores the enriched notification data WITHOUT changing status
// Status should be updated separately via UpdateStatus
func (r *NotificationRepository) UpdateEnrichedPayload(ctx context.Context, id uuid.UUID, payload models.JSONMap) error {
	query := `
		UPDATE notifications
		SET enriched_payload = $1,
		    updated_at = NOW()
		WHERE id = $2 AND deleted_at IS NULL
	`

	result, err := r.pool.Exec(ctx, query, payload, id)
	if err != nil {
		r.logger.Error().Err(err).Str("id", id.String()).Msg("Failed to update enriched payload")
		return fmt.Errorf("failed to update enriched payload: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("notification not found or already deleted")
	}

	return nil
}

// CreateNotificationWithTransaction creates a notification within a transaction
func (r *NotificationRepository) CreateNotificationWithTransaction(ctx context.Context, tx pgx.Tx, notif *models.Notification) error {
	query := `
		INSERT INTO notifications (
			id, user_id, template_id, correlation_id, idempotency_key,
			channel, status, priority, variables, metadata, enriched_payload,
			recipient, retry_count, max_retries, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16
		)
	`

	_, err := tx.Exec(ctx, query,
		notif.ID,
		notif.UserID,
		notif.TemplateID,
		notif.CorrelationID,
		notif.IdempotencyKey,
		notif.Channel,
		notif.Status,
		notif.Priority,
		notif.Variables,
		notif.Metadata,
		notif.EnrichedPayload,
		notif.Recipient,
		notif.RetryCount,
		notif.MaxRetries,
		notif.CreatedAt,
		notif.UpdatedAt,
	)

	if err != nil {
		r.logger.Error().Err(err).Msg("Failed to create notification in transaction")
		return fmt.Errorf("failed to create notification: %w", err)
	}

	return nil
}

// BeginTx starts a new transaction
func (r *NotificationRepository) BeginTx(ctx context.Context) (pgx.Tx, error) {
	return r.pool.Begin(ctx)
}

// GetByID retrieves a notification by ID
func (r *NotificationRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.Notification, error) {
	query := `
		SELECT
			id, user_id, template_id, correlation_id, idempotency_key,
			channel, status, priority, variables, metadata, enriched_payload,
			recipient, enriched_at, queued_at, sent_at, delivered_at, failed_at,
			error_code, error_message, retry_count, max_retries,
			provider, provider_message_id, created_at, updated_at, deleted_at
		FROM notifications
		WHERE id = $1 AND deleted_at IS NULL
	`

	var notif models.Notification
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&notif.ID,
		&notif.UserID,
		&notif.TemplateID,
		&notif.CorrelationID,
		&notif.IdempotencyKey,
		&notif.Channel,
		&notif.Status,
		&notif.Priority,
		&notif.Variables,
		&notif.Metadata,
		&notif.EnrichedPayload,
		&notif.Recipient,
		&notif.EnrichedAt,
		&notif.QueuedAt,
		&notif.SentAt,
		&notif.DeliveredAt,
		&notif.FailedAt,
		&notif.ErrorCode,
		&notif.ErrorMessage,
		&notif.RetryCount,
		&notif.MaxRetries,
		&notif.Provider,
		&notif.ProviderMsgID,
		&notif.CreatedAt,
		&notif.UpdatedAt,
		&notif.DeletedAt,
	)

	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("notification not found")
	}
	if err != nil {
		r.logger.Error().Err(err).Str("id", id.String()).Msg("Failed to get notification")
		return nil, fmt.Errorf("failed to get notification: %w", err)
	}

	return &notif, nil
}

// GetByCorrelationID retrieves a notification by correlation ID
func (r *NotificationRepository) GetByCorrelationID(ctx context.Context, correlationID string) (*models.Notification, error) {
	query := `
		SELECT
			id, user_id, template_id, correlation_id, idempotency_key,
			channel, status, priority, variables, metadata, enriched_payload,
			recipient, enriched_at, queued_at, sent_at, delivered_at, failed_at,
			error_code, error_message, retry_count, max_retries,
			provider, provider_message_id, created_at, updated_at, deleted_at
		FROM notifications
		WHERE correlation_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC
		LIMIT 1
	`

	var notif models.Notification
	err := r.pool.QueryRow(ctx, query, correlationID).Scan(
		&notif.ID,
		&notif.UserID,
		&notif.TemplateID,
		&notif.CorrelationID,
		&notif.IdempotencyKey,
		&notif.Channel,
		&notif.Status,
		&notif.Priority,
		&notif.Variables,
		&notif.Metadata,
		&notif.EnrichedPayload,
		&notif.Recipient,
		&notif.EnrichedAt,
		&notif.QueuedAt,
		&notif.SentAt,
		&notif.DeliveredAt,
		&notif.FailedAt,
		&notif.ErrorCode,
		&notif.ErrorMessage,
		&notif.RetryCount,
		&notif.MaxRetries,
		&notif.Provider,
		&notif.ProviderMsgID,
		&notif.CreatedAt,
		&notif.UpdatedAt,
		&notif.DeletedAt,
	)

	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("notification not found")
	}
	if err != nil {
		r.logger.Error().Err(err).Str("correlation_id", correlationID).Msg("Failed to get notification")
		return nil, fmt.Errorf("failed to get notification: %w", err)
	}

	return &notif, nil
}

// GetByIdempotencyKey checks if a notification exists with given idempotency key
func (r *NotificationRepository) GetByIdempotencyKey(ctx context.Context, key string) (*models.Notification, error) {
	query := `
		SELECT
			id, user_id, template_id, correlation_id, idempotency_key,
			channel, status, priority, variables, metadata, enriched_payload,
			recipient, enriched_at, queued_at, sent_at, delivered_at, failed_at,
			error_code, error_message, retry_count, max_retries,
			provider, provider_message_id, created_at, updated_at, deleted_at
		FROM notifications
		WHERE idempotency_key = $1
		  AND created_at > NOW() - INTERVAL '24 hours'
		  AND deleted_at IS NULL
		ORDER BY created_at DESC
		LIMIT 1
	`

	var notif models.Notification
	err := r.pool.QueryRow(ctx, query, key).Scan(
		&notif.ID,
		&notif.UserID,
		&notif.TemplateID,
		&notif.CorrelationID,
		&notif.IdempotencyKey,
		&notif.Channel,
		&notif.Status,
		&notif.Priority,
		&notif.Variables,
		&notif.Metadata,
		&notif.EnrichedPayload,
		&notif.Recipient,
		&notif.EnrichedAt,
		&notif.QueuedAt,
		&notif.SentAt,
		&notif.DeliveredAt,
		&notif.FailedAt,
		&notif.ErrorCode,
		&notif.ErrorMessage,
		&notif.RetryCount,
		&notif.MaxRetries,
		&notif.Provider,
		&notif.ProviderMsgID,
		&notif.CreatedAt,
		&notif.UpdatedAt,
		&notif.DeletedAt,
	)

	if err == pgx.ErrNoRows {
		return nil, nil // Not found is not an error for idempotency check
	}
	if err != nil {
		r.logger.Error().Err(err).Str("key", key).Msg("Failed to check idempotency key")
		return nil, fmt.Errorf("failed to check idempotency key: %w", err)
	}

	return &notif, nil
}

// GetUserNotificationsWithCursor retrieves notifications using cursor-based pagination
// More efficient than offset-based pagination for large datasets
func (r *NotificationRepository) GetUserNotificationsWithCursor(ctx context.Context, userID string, limit int, cursor *time.Time) ([]models.Notification, *time.Time, error) {
	var query string
	var args []interface{}

	if cursor == nil {
		query = `
			SELECT
				id, user_id, template_id, correlation_id, idempotency_key,
				channel, status, priority, variables, metadata, enriched_payload,
				recipient, enriched_at, queued_at, sent_at, delivered_at, failed_at,
				error_code, error_message, retry_count, max_retries,
				provider, provider_message_id, created_at, updated_at, deleted_at
			FROM notifications
			WHERE user_id = $1 AND deleted_at IS NULL
			ORDER BY created_at DESC
			LIMIT $2
		`
		args = []interface{}{userID, limit}
	} else {
		query = `
			SELECT
				id, user_id, template_id, correlation_id, idempotency_key,
				channel, status, priority, variables, metadata, enriched_payload,
				recipient, enriched_at, queued_at, sent_at, delivered_at, failed_at,
				error_code, error_message, retry_count, max_retries,
				provider, provider_message_id, created_at, updated_at, deleted_at
			FROM notifications
			WHERE user_id = $1 AND created_at < $2 AND deleted_at IS NULL
			ORDER BY created_at DESC
			LIMIT $3
		`
		args = []interface{}{userID, cursor, limit}
	}

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		r.logger.Error().Err(err).Msg("Failed to get user notifications")
		return nil, nil, fmt.Errorf("failed to get user notifications: %w", err)
	}
	defer rows.Close()

	notifications := make([]models.Notification, 0)
	var nextCursor *time.Time

	for rows.Next() {
		var notif models.Notification
		err := rows.Scan(
			&notif.ID,
			&notif.UserID,
			&notif.TemplateID,
			&notif.CorrelationID,
			&notif.IdempotencyKey,
			&notif.Channel,
			&notif.Status,
			&notif.Priority,
			&notif.Variables,
			&notif.Metadata,
			&notif.EnrichedPayload,
			&notif.Recipient,
			&notif.EnrichedAt,
			&notif.QueuedAt,
			&notif.SentAt,
			&notif.DeliveredAt,
			&notif.FailedAt,
			&notif.ErrorCode,
			&notif.ErrorMessage,
			&notif.RetryCount,
			&notif.MaxRetries,
			&notif.Provider,
			&notif.ProviderMsgID,
			&notif.CreatedAt,
			&notif.UpdatedAt,
			&notif.DeletedAt,
		)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to scan notification: %w", err)
		}
		notifications = append(notifications, notif)
		nextCursor = &notif.CreatedAt
	}

	if len(notifications) < limit {
		nextCursor = nil // No more pages
	}

	return notifications, nextCursor, nil
}

// GetUserNotifications retrieves notifications for a user with pagination (legacy method)
func (r *NotificationRepository) GetUserNotifications(ctx context.Context, userID string, limit, offset int) ([]models.Notification, int64, error) {
	// For better performance, use estimated count or cache the total
	var total int64 = -1 // Return -1 to indicate count wasn't fetched

	// Get paginated results
	query := `
		SELECT
			id, user_id, template_id, correlation_id, idempotency_key,
			channel, status, priority, variables, metadata, enriched_payload,
			recipient, enriched_at, queued_at, sent_at, delivered_at, failed_at,
			error_code, error_message, retry_count, max_retries,
			provider, provider_message_id, created_at, updated_at, deleted_at
		FROM notifications
		WHERE user_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := r.pool.Query(ctx, query, userID, limit, offset)
	if err != nil {
		r.logger.Error().Err(err).Msg("Failed to get user notifications")
		return nil, 0, fmt.Errorf("failed to get user notifications: %w", err)
	}
	defer rows.Close()

	notifications := make([]models.Notification, 0)
	for rows.Next() {
		var notif models.Notification
		err := rows.Scan(
			&notif.ID,
			&notif.UserID,
			&notif.TemplateID,
			&notif.CorrelationID,
			&notif.IdempotencyKey,
			&notif.Channel,
			&notif.Status,
			&notif.Priority,
			&notif.Variables,
			&notif.Metadata,
			&notif.EnrichedPayload,
			&notif.Recipient,
			&notif.EnrichedAt,
			&notif.QueuedAt,
			&notif.SentAt,
			&notif.DeliveredAt,
			&notif.FailedAt,
			&notif.ErrorCode,
			&notif.ErrorMessage,
			&notif.RetryCount,
			&notif.MaxRetries,
			&notif.Provider,
			&notif.ProviderMsgID,
			&notif.CreatedAt,
			&notif.UpdatedAt,
			&notif.DeletedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan notification: %w", err)
		}
		notifications = append(notifications, notif)
	}

	return notifications, total, nil
}

// UpdateFailure records a notification failure
func (r *NotificationRepository) UpdateFailure(ctx context.Context, id uuid.UUID, errorCode, errorMsg string) error {
	query := `
		UPDATE notifications
		SET status = 'failed',
		    error_code = $1,
		    error_message = $2,
		    retry_count = retry_count + 1,
		    failed_at = COALESCE(failed_at, NOW()),
		    updated_at = NOW()
		WHERE id = $3 AND deleted_at IS NULL
	`

	result, err := r.pool.Exec(ctx, query, errorCode, errorMsg, id)
	if err != nil {
		r.logger.Error().Err(err).Str("id", id.String()).Msg("Failed to update failure")
		return fmt.Errorf("failed to update failure: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("notification not found or already deleted")
	}

	return nil
}

// GetFailedForRetry retrieves failed notifications that can be retried
func (r *NotificationRepository) GetFailedForRetry(ctx context.Context, limit int) ([]models.Notification, error) {
	query := `
		SELECT
			id, user_id, template_id, correlation_id, idempotency_key,
			channel, status, priority, variables, metadata, enriched_payload,
			recipient, enriched_at, queued_at, sent_at, delivered_at, failed_at,
			error_code, error_message, retry_count, max_retries,
			provider, provider_message_id, created_at, updated_at, deleted_at
		FROM notifications
		WHERE status = 'failed'
		  AND retry_count < max_retries
		  AND deleted_at IS NULL
		  AND failed_at > NOW() - INTERVAL '24 hours'
		ORDER BY priority DESC, created_at ASC
		LIMIT $1
		FOR UPDATE SKIP LOCKED
	`

	rows, err := r.pool.Query(ctx, query, limit)
	if err != nil {
		r.logger.Error().Err(err).Msg("Failed to get failed notifications")
		return nil, fmt.Errorf("failed to get failed notifications: %w", err)
	}
	defer rows.Close()

	notifications := make([]models.Notification, 0)
	for rows.Next() {
		var notif models.Notification
		err := rows.Scan(
			&notif.ID,
			&notif.UserID,
			&notif.TemplateID,
			&notif.CorrelationID,
			&notif.IdempotencyKey,
			&notif.Channel,
			&notif.Status,
			&notif.Priority,
			&notif.Variables,
			&notif.Metadata,
			&notif.EnrichedPayload,
			&notif.Recipient,
			&notif.EnrichedAt,
			&notif.QueuedAt,
			&notif.SentAt,
			&notif.DeliveredAt,
			&notif.FailedAt,
			&notif.ErrorCode,
			&notif.ErrorMessage,
			&notif.RetryCount,
			&notif.MaxRetries,
			&notif.Provider,
			&notif.ProviderMsgID,
			&notif.CreatedAt,
			&notif.UpdatedAt,
			&notif.DeletedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan notification: %w", err)
		}
		notifications = append(notifications, notif)
	}

	return notifications, nil
}

// GetStatsByDateRange retrieves notification statistics
func (r *NotificationRepository) GetStatsByDateRange(ctx context.Context, startDate, endDate time.Time) ([]dtos.NotificationStats, error) {
	query := `
		SELECT
		    date_trunc('day', created_at) as date,
		    channel,
		    status,
		    COUNT(*) as count,
		    AVG(EXTRACT(EPOCH FROM (COALESCE(sent_at, NOW()) - created_at))) as avg_processing_time_seconds
		FROM notifications
		WHERE created_at BETWEEN $1 AND $2
		  AND deleted_at IS NULL
		GROUP BY date_trunc('day', created_at), channel, status
		ORDER BY date DESC, channel, status
	`

	rows, err := r.pool.Query(ctx, query, startDate, endDate)
	if err != nil {
		r.logger.Error().Err(err).Msg("Failed to get notification stats")
		return nil, fmt.Errorf("failed to get notification stats: %w", err)
	}
	defer rows.Close()

	stats := make([]dtos.NotificationStats, 0)
	for rows.Next() {
		var stat dtos.NotificationStats
		err := rows.Scan(
			&stat.Date,
			&stat.Channel,
			&stat.Status,
			&stat.Count,
			&stat.AvgProcessingTime,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan stats: %w", err)
		}
		stats = append(stats, stat)
	}

	return stats, nil
}

// SoftDelete soft deletes a notification
func (r *NotificationRepository) SoftDelete(ctx context.Context, id uuid.UUID) error {
	query := `
		UPDATE notifications
		SET deleted_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL
	`

	result, err := r.pool.Exec(ctx, query, id)
	if err != nil {
		r.logger.Error().Err(err).Str("id", id.String()).Msg("Failed to delete notification")
		return fmt.Errorf("failed to delete notification: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("notification not found or already deleted")
	}

	return nil
}

package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/justinndidit/notificationSystem/orchestrator/internal/dtos"
)

type Notification struct {
	ID             uuid.UUID             `db:"id"`
	UserID         uuid.UUID             `db:"user_id"`
	TemplateID     uuid.UUID             `db:"template_id"`
	CorrelationID  uuid.UUID             `db:"correlation_id"`
	IdempotencyKey *string               `db:"idempotency_key"`
	Channel        dtos.NotificationType `db:"channel"`
	Status         string                `db:"status"`
	Priority       string                `db:"priority"`

	Variables       dtos.UserData `db:"variables"`
	Metadata        JSONMap       `db:"metadata"`         // Changed
	EnrichedPayload JSONMap       `db:"enriched_payload"` // Changed

	Recipient     *string    `db:"recipient"`
	EnrichedAt    *time.Time `db:"enriched_at"`
	QueuedAt      *time.Time `db:"queued_at"`
	SentAt        *time.Time `db:"sent_at"`
	DeliveredAt   *time.Time `db:"delivered_at"`
	FailedAt      *time.Time `db:"failed_at"`
	ErrorCode     *string    `db:"error_code"`
	ErrorMessage  *string    `db:"error_message"`
	RetryCount    int        `db:"retry_count"`
	MaxRetries    int        `db:"max_retries"`
	Provider      *string    `db:"provider"`
	ProviderMsgID *string    `db:"provider_message_id"`
	CreatedAt     time.Time  `db:"created_at"`
	UpdatedAt     time.Time  `db:"updated_at"`
	DeletedAt     *time.Time `db:"deleted_at"`
}

// Update NotificationEvent to use JSONMap
type NotificationEvent struct {
	ID             uuid.UUID `db:"id"`
	NotificationID uuid.UUID `db:"notification_id"`
	CorrelationID  uuid.UUID `db:"correlation_id"`
	EventType      string    `db:"event_type"`
	Channel        *string   `db:"channel"`
	EventData      JSONMap   `db:"event_data"` // Changed from []byte
	Provider       *string   `db:"provider"`
	ProviderMsgID  *string   `db:"provider_message_id"`
	UserAgent      *string   `db:"user_agent"`
	IPAddress      *string   `db:"ip_address"`
	EventAt        time.Time `db:"event_at"`
}

// Add to internal/models/notification.go

// JSONMap handles map[string]any for JSONB columns
type JSONMap map[string]any

func (j JSONMap) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	return json.Marshal(j)
}

func (j *JSONMap) Scan(value interface{}) error {
	if value == nil {
		*j = nil
		return nil
	}

	bytes, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("failed to unmarshal JSONMap: %v", value)
	}

	return json.Unmarshal(bytes, j)
}

// Update Notification struct to use JSONMap

package internal

import (
	"time"
)

// PushNotificationMessage is the push-relevant subset of the orchestrator's
// EnrichedNotification. Field names and JSON tags must stay in step with
// docs/contracts/enriched-notification.md — the orchestrator is the producer
// and this struct is one consumer's view of the same message.
//
// Tokens, Title and Body arrive already resolved and rendered: this worker
// never looks up a device registration or compiles a template.
type PushNotificationMessage struct {
	NotificationID string `json:"notification_id"`
	CorrelationID  string `json:"correlation_id"`
	IdempotencyKey string `json:"idempotency_key"`
	UserID         string `json:"user_id"`
	Channel        string `json:"channel"`

	Title  string        `json:"title"`
	Body   string        `json:"body"`
	Tokens []DeviceToken `json:"tokens"`

	Priority  string         `json:"priority,omitempty"` // low | normal | high | urgent
	Metadata  map[string]any `json:"metadata,omitempty"`
	CreatedAt time.Time      `json:"created_at"`

	// Channel-specific options, not currently emitted by the orchestrator.
	Data        map[string]interface{} `json:"data,omitempty"`
	ImageURL    string                 `json:"image_url,omitempty"`
	TTL         int                    `json:"ttl,omitempty"` // seconds
	Sound       string                 `json:"sound,omitempty"`
	Badge       int                    `json:"badge,omitempty"`
	ClickAction string                 `json:"click_action,omitempty"`
}

// DeviceToken represents a user's device token
type DeviceToken struct {
	Token    string `json:"token"`
	Platform string `json:"platform"` // android, ios
}

// PushResult represents the result of sending a push notification
type PushResult struct {
	NotificationID string    `json:"notification_id"`
	Token          string    `json:"token"`
	Platform       string    `json:"platform"`
	Success        bool      `json:"success"`
	Error          string    `json:"error,omitempty"`
	MessageID      string    `json:"message_id,omitempty"`
	SentAt         time.Time `json:"sent_at"`
}

// DeliveryStatus represents the status of a notification delivery
type DeliveryStatus struct {
	NotificationID string       `json:"notification_id"`
	TotalTokens    int          `json:"total_tokens"`
	SuccessCount   int          `json:"success_count"`
	FailureCount   int          `json:"failure_count"`
	Results        []PushResult `json:"results"`
	CompletedAt    time.Time    `json:"completed_at"`
}

// Constants for platforms and priorities
const (
	PlatformAndroid = "android"
	PlatformIOS     = "ios"

	PriorityHigh   = "high"
	PriorityNormal = "normal"
)

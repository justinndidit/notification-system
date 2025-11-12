package internal

import (
	"time"
)

// PushNotificationMessage represents a message from the queue
type PushNotificationMessage struct {
	NotificationID string                 `json:"notification_id"`
	UserID         string                 `json:"user_id"`
	Title          string                 `json:"title"`
	Body           string                 `json:"body"`
	Data           map[string]interface{} `json:"data,omitempty"`
	ImageURL       string                 `json:"image_url,omitempty"`
	Tokens         []DeviceToken          `json:"tokens"`
	Priority       string                 `json:"priority,omitempty"` // high, normal
	TTL            int                    `json:"ttl,omitempty"`      // Time to live in seconds
	Sound          string                 `json:"sound,omitempty"`
	Badge          int                    `json:"badge,omitempty"`
	ClickAction    string                 `json:"click_action,omitempty"`
	CreatedAt      time.Time              `json:"created_at"`
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

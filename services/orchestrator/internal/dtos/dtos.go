package dtos

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"
)

type HTTPResponse struct {
	Success bool            `json:"success" validate:"required"`
	Data    interface{}     `json:"data,omitempty"`
	Error   string          `json:"error,omitempty"`
	Message string          `json:"message" validate:"required"`
	Meta    *PaginationMeta `json:"meta"`
}

// UserDeliveryProfile is what GET /user/{id}/delivery-profile returns: where to
// reach a user and whether they have consented, in one call.
type UserDeliveryProfile struct {
	UserID       string        `json:"user_id"`
	Name         string        `json:"name"`
	Email        string        `json:"email"`
	DeviceTokens []DeviceToken `json:"device_tokens"`
	EmailOptIn   bool          `json:"email_opt_in"`
	PushOptIn    bool          `json:"push_opt_in"`
	DailyLimit   int           `json:"daily_limit"`
	Language     string        `json:"language"`
}

// RenderedContent is one entry from POST /template/{id}/render — the template
// compiled for a single channel.
type RenderedContent struct {
	Channel string `json:"channel"` // EMAIL | PUSH
	Subject string `json:"subject,omitempty"`
	HTML    string `json:"html,omitempty"`
	Title   string `json:"title,omitempty"`
	Body    string `json:"body,omitempty"`
}

// DeviceToken is one FCM registration token and the platform it belongs to.
type DeviceToken struct {
	Token    string `json:"token"`
	Platform string `json:"platform"` // android | ios
}

type UserPreferenceData struct {
	TemplateID  string `json:"id" validate:"required"`
	UserID      string `json:"user_id" validate:"required"`
	EmailOption bool   `json:"email_opt_in" validate:"required"`
	PushOption  bool   `json:"push_opt_in" validate:"required"`
	DailyLimit  int    `json:"daily_limit" validate:"required"`
	Language    string `json:"language" validate:"required"`
}

type TemplateVersion struct {
	ID         string    `json:"id" validate:"required"`
	TemplateID string    `json:"template_id" validate:"required"`
	Versions   int       `json:"version" validate:"required"`
	Subject    string    `json:"subject" validate:"required"`
	Title      string    `json:"title" validate:"required"`
	Body       string    `json:"body" validate:"required"`
	Variables  UserData  `json:"variables" validate:"required"`
	CreatedAt  time.Time `json:"created_at" validate:"required"`
	UpdatedAt  time.Time `json:"updated_at" validate:"required"`
}

type TemplateData struct {
	ID        string            `json:"id" validate:"required"`
	Name      string            `json:"name" validate:"required"`
	Event     string            `json:"event" validate:"required"`
	Channel   []string          `json:"channel" validate:"required"`
	Language  string            `json:"language" validate:"required"`
	IsActive  bool              `json:"isActive" validate:"required"`
	CreatedAt time.Time         `json:"created_at" validate:"required"`
	UpdatedAt time.Time         `json:"updated_at" validate:"required"`
	Versions  []TemplateVersion `json:"versions" validate:"required"`
}

type PaginationMeta struct {
	Total       int  `json:"total" validate:"required"`
	Limit       int  `json:"limit" validate:"required"`
	Page        int  `json:"page" validate:"required"`
	TotalPages  int  `json:"total_pages" validate:"required"`
	HasNext     bool `json:"has_next" validate:"required"`
	HasPrevious bool `json:"has_previous" validate:"required"`
}

type NotificationRequest struct {
	NotificationType NotificationType `json:"notification_type" validate:"required"`
	UserID           string           `json:"user_id" validate:"required,uuid"`
	TemplateCode     string           `json:"template_code" validate:"required,uuid"` //template id
	Variables        UserData         `json:"variables" validate:"required"`
	RequestID        string           `json:"request_id" validate:"required"`
	Priority         int              `json:"priority" validate:"required"`
	MetaData         map[string]any   `json:"metadata,omitempty"`
}

type NotificationType string

const (
	Email NotificationType = "email"
	Push  NotificationType = "push"
)

type UserData struct {
	Name string         `json:"name"`
	Link string         `json:"link"`
	Meta map[string]any `json:"meta,omitempty"`
}

type NotificationPriority int

const (
	Low NotificationPriority = iota + 1
	Normal
	High
	Urgent
)

func NotificationPriorityToString(p NotificationPriority) string {
	switch p {
	case Low:
		return "low"
	case Normal:
		return "normal"
	case High:
		return "high"
	case Urgent:
		return "urgent"
	default:
		return "normal" // default fallback
	}
}

type NotificationStats struct {
	Date              time.Time `db:"date"`
	Channel           string    `db:"channel"`
	Status            string    `db:"status"`
	Count             int64     `db:"count"`
	AvgProcessingTime *float64  `db:"avg_processing_time_seconds"`
}

// Status constants
const (
	StatusPending    = "pending"
	StatusEnriching  = "enriching"
	StatusQueued     = "queued"
	StatusProcessing = "processing"
	StatusSent       = "sent"
	StatusFailed     = "failed"
	StatusCancelled  = "cancelled"
)

// Event type constants
const (
	EventCreated      = "created"
	EventEnriched     = "enriched"
	EventQueued       = "queued"
	EventSent         = "sent"
	EventDelivered    = "delivered"
	EventFailed       = "failed"
	EventOpened       = "opened"
	EventClicked      = "clicked"
	EventBounced      = "bounced"
	EventUnsubscribed = "unsubscribed"
	EventCancelled    = "cancelled"
	EventRetried      = "retried"
)

type EnrichedNotification struct {
	NotificationID string `json:"notification_id"`
	CorrelationID  string `json:"correlation_id"`
	IdempotencyKey string `json:"idempotency_key"`
	UserID         string `json:"user_id"`
	TemplateCode   string `json:"template_code"`
	Channel        string `json:"channel"`
	Priority       string `json:"priority"`
	// Recipient is the resolved destination for this channel (email address for
	// email). Workers must not have to look this up themselves.
	Recipient string `json:"recipient"`
	// Tokens is populated for push notifications so the worker never has to
	// look up device registrations itself.
	Tokens []DeviceToken `json:"tokens"`
	// Rendered content. Workers send these verbatim — no template engine
	// is needed in any channel worker.
	Subject         string             `json:"subject,omitempty"`
	Title           string             `json:"title,omitempty"`
	Body            string             `json:"body,omitempty"`
	UserPreferences UserPreferenceData `json:"user_preferences"`
	Template        TemplateData       `json:"template"`
	Variables       UserData           `json:"variables"`
	Metadata        map[string]any     `json:"metadata"`
	CreatedAt       time.Time          `json:"created_at"`
}

// Add to internal/dtos/notification.go
// Value implements driver.Valuer for database insert/update
func (u UserData) Value() (driver.Value, error) {
	return json.Marshal(u)
}

// Scan implements sql.Scanner for database select.
//
// pgx hands JSONB back as either []byte or string depending on the codec in
// play, so both must be accepted — asserting only []byte fails at runtime on
// every read.
func (u *UserData) Scan(value interface{}) error {
	if value == nil {
		return nil
	}

	switch v := value.(type) {
	case []byte:
		return json.Unmarshal(v, u)
	case string:
		return json.Unmarshal([]byte(v), u)
	default:
		return fmt.Errorf("failed to unmarshal UserData: unsupported type %T", value)
	}
}

// Value implements driver.Valuer for NotificationType
func (n NotificationType) Value() (driver.Value, error) {
	return string(n), nil
}

// Scan implements sql.Scanner for NotificationType
func (n *NotificationType) Scan(value interface{}) error {
	if value == nil {
		return nil
	}

	str, ok := value.(string)
	if !ok {
		return fmt.Errorf("failed to scan NotificationType: %v", value)
	}

	*n = NotificationType(str)
	return nil
}

// ---------------------------------------------------------------------------
// Status and query API
// ---------------------------------------------------------------------------

// NotificationView is the public shape of a notification. It deliberately omits
// enriched_payload, which holds the recipient's contact details and the fully
// rendered message body.
type NotificationView struct {
	NotificationID string     `json:"notification_id"`
	CorrelationID  string     `json:"correlation_id"`
	IdempotencyKey *string    `json:"idempotency_key,omitempty"`
	UserID         string     `json:"user_id"`
	TemplateID     string     `json:"template_id"`
	Channel        string     `json:"channel"`
	Status         string     `json:"status"`
	Priority       string     `json:"priority"`
	Recipient      *string    `json:"recipient,omitempty"`
	RetryCount     int        `json:"retry_count"`
	MaxRetries     int        `json:"max_retries"`
	ErrorCode      *string    `json:"error_code,omitempty"`
	ErrorMessage   *string    `json:"error_message,omitempty"`
	Provider       *string    `json:"provider,omitempty"`
	EnrichedAt     *time.Time `json:"enriched_at,omitempty"`
	QueuedAt       *time.Time `json:"queued_at,omitempty"`
	SentAt         *time.Time `json:"sent_at,omitempty"`
	DeliveredAt    *time.Time `json:"delivered_at,omitempty"`
	FailedAt       *time.Time `json:"failed_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// NotificationEventView is one entry in a notification's audit timeline.
type NotificationEventView struct {
	EventType     string         `json:"event_type"`
	Channel       *string        `json:"channel,omitempty"`
	EventData     map[string]any `json:"event_data,omitempty"`
	Provider      *string        `json:"provider,omitempty"`
	ProviderMsgID *string        `json:"provider_message_id,omitempty"`
	EventAt       time.Time      `json:"event_at"`
}

// CursorPage carries cursor pagination for notification listings. The cursor is
// a created_at timestamp rather than an offset, so paging stays correct while
// new notifications arrive.
type CursorPage struct {
	NextCursor *time.Time `json:"next_cursor,omitempty"`
	HasMore    bool       `json:"has_more"`
	Limit      int        `json:"limit"`
}

// StatusCallbackRequest is what a channel worker POSTs back after attempting
// delivery.
type StatusCallbackRequest struct {
	NotificationID string `json:"notification_id" validate:"required,uuid"`
	Status         string `json:"status" validate:"required"`
	Error          string `json:"error,omitempty"`
	Provider       string `json:"provider,omitempty"`
	ProviderMsgID  string `json:"provider_message_id,omitempty"`
}

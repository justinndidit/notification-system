package dtos

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type HTTPResponse struct {
	Success bool            `json:"success" validate:"required"`
	Data    interface{}     `json:"data,omitempty"`
	Error   string          `json:"error,omitempty"`
	Message string          `json:"message" validate:"required"`
	Meta    *PaginationMeta `json:"meta"`
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
	UserID           string           `json:"user_id" validate:"required"`
	TemplateCode     string           `json:"template_code" validate:"required"` //template id
	Variables        UserData         `json:"variables" validate:"required"`
	RequestID        string           `json:"request_id" validate:"required"`
	Priority         int              `json:"priority" validate:"required"`
	MetaData         map[string]any   `json:"metadata,omitempty"`
}

type NotificationRequestDTO struct {
	NotificationType NotificationType //channels
	UserID           string
	TemplateCode     string //template id
	Variables        UserData
	RequestID        string
	Priority         int
	MetaData         map[string]any
	ScheduledFor     *time.Time
	CorrelationID    string
}

type NotificationType string
type CorrelationOID string

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

type TemplateServiceRequest struct {
	TemplateID string `json:"template_id"`
}

type TemplateServiceResponse struct {
}

// Represents the data from the Template Response
type Template struct {
}

// Extended version
type BatchNotificationRequest struct {
	NotificationType string                    `json:"notification_type"`
	TemplateCode     uuid.UUID                 `json:"template_code"`
	UserIDs          []uuid.UUID               `json:"user_ids"`        // list of recipients
	CommonVariables  map[string]any            `json:"variables"`       // shared variables
	Personalization  map[string]map[string]any `json:"personalization"` // optional per-user overrides
	RequestID        string                    `json:"request_id"`      // idempotency for batch
	Priority         string                    `json:"priority"`
	MetaData         map[string]any            `json:"metadata"`
	ScheduledFor     *time.Time                `json:"scheduled_for,omitempty"`
	ChunkSize        int                       `json:"chunk_size,omitempty"` // optional, default 1000
}

// type NotificationWithEvents struct {
// 	Notification
// 	Events []NotificationEvent `db:"-"` // Not from DB directly
// }

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
	NotificationID  string             `json:"notification_id"`
	CorrelationID   string             `json:"correlation_id"`
	IdempotencyKey  string             `json:"idempotency_key"`
	UserID          string             `json:"user_id"`
	TemplateCode    string             `json:"template_code"`
	Channel         string             `json:"channel"`
	Priority        string             `json:"priority"`
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

// Scan implements sql.Scanner for database select
func (u *UserData) Scan(value interface{}) error {
	if value == nil {
		return nil
	}

	bytes, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("failed to unmarshal UserData: %v", value)
	}

	return json.Unmarshal(bytes, u)
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

package dtos

import (
	"time"

	"github.com/google/uuid"
)

type HTTPResponse struct {
	Success bool                   `json:"success" validate:"required"`
	Data    map[string]interface{} `json:"data,omitempty"`
	Error   string                 `json:"error,omitempty"`
	Message string                 `json:"message" validate:"required"`
	Meta    *PaginationMeta        `json:"meta"`
}

type UserPreferenceData struct {
}

type TemplateData struct {
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

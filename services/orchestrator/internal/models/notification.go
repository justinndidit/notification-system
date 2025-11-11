package models

import "github.com/justinndidit/notificationSystem/orchestrator/internal/dtos"

type Notification struct {
	UserID        string                `db:"user_id"`
	TemplateID    string                `db:"template_id"`
	CorrelationID string                `db:"correlation_id"`
	Channel       dtos.NotificationType `db:"channel"`
	Priority      string                `db:"priority"`
	Variables     dtos.UserData         `db:"payload"`
}

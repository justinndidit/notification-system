package repositories

import (
	"github.com/justinndidit/notificationSystem/orchestrator/internal/database"
	"github.com/justinndidit/notificationSystem/orchestrator/internal/models"
	"github.com/rs/zerolog"
)

type NotificationRepo struct {
	db     *database.Database
	logger *zerolog.Logger
}

func NewRepository(log *zerolog.Logger, db *database.Database) *NotificationRepo {
	return &NotificationRepo{
		db:     db,
		logger: log,
	}
}

func (r *NotificationRepo) InsertNotification(notification *models.Notification) {

}

package app

import (
	"github.com/go-redis/redis/v8"
	"github.com/justinndidit/notificationSystem/orchestrator/internal/config"
	"github.com/justinndidit/notificationSystem/orchestrator/internal/database"
	"github.com/justinndidit/notificationSystem/orchestrator/internal/handlers"
	"github.com/justinndidit/notificationSystem/orchestrator/internal/repositories"
	"github.com/rs/zerolog"
)

type App struct {
	Logger      *zerolog.Logger
	RedisClient *redis.Client
	DB          *database.Database
	NRepo       *repositories.NotificationRepo
	Config      *config.Config
	NHandler    *handlers.NotificationHandler
	HHandler    *handlers.HealthHandler
}

func NewApp(primary *config.Config,
	log *zerolog.Logger,
	rdb *redis.Client,
	db *database.Database,
	notificationRepo *repositories.NotificationRepo,
	nHandler *handlers.NotificationHandler,
	hHandler *handlers.HealthHandler) *App {
	return &App{
		Logger:      log,
		RedisClient: rdb,
		DB:          db,
		NRepo:       notificationRepo,
		Config:      primary,
		NHandler:    nHandler,
		HHandler:    hHandler,
	}
}

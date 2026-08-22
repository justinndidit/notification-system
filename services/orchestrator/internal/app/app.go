package app

import (
	"github.com/go-redis/redis/v8"
	"github.com/justinndidit/notificationSystem/orchestrator/internal/auth"
	"github.com/justinndidit/notificationSystem/orchestrator/internal/config"
	"github.com/justinndidit/notificationSystem/orchestrator/internal/database"
	"github.com/justinndidit/notificationSystem/orchestrator/internal/handlers"
	"github.com/rs/zerolog"
)

type App struct {
	Logger      *zerolog.Logger
	RedisClient *redis.Client
	DB          *database.Database
	Config      *config.Config
	NHandler    *handlers.NotificationHandler
	HHandler    *handlers.HealthHandler
	// Verifies inbound service tokens on routes workers call directly.
	ServiceVerifier *auth.Verifier
}

func NewApp(primary *config.Config,
	log *zerolog.Logger,
	rdb *redis.Client,
	db *database.Database,
	nHandler *handlers.NotificationHandler,
	hHandler *handlers.HealthHandler,
	serviceVerifier *auth.Verifier) *App {
	return &App{
		Logger:          log,
		RedisClient:     rdb,
		DB:              db,
		Config:          primary,
		NHandler:        nHandler,
		HHandler:        hHandler,
		ServiceVerifier: serviceVerifier,
	}
}

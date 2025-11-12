package internal

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	Service  ServiceConfig
	RabbitMQ RabbitMQConfig
	FCM      FCMConfig
	APNS     APNSConfig
	Redis    RedisConfig
}

type ServiceConfig struct {
	Name        string
	Port        string
	Environment string
	LogLevel    string
}

type RabbitMQConfig struct {
	URL       string
	Queue     string
	Exchange  string
	Prefetch  int
	Reconnect bool
}

type FCMConfig struct {
	ServerKey  string
	Enabled    bool
	MaxRetries int
	BatchSize  int
}

type APNSConfig struct {
	KeyID      string
	TeamID     string
	BundleID   string
	KeyPath    string
	Production bool
	Enabled    bool
}

type RedisConfig struct {
	URL      string
	Password string
	DB       int
}

func Load() (*Config, error) {
	cfg := &Config{
		Service: ServiceConfig{
			Name:        getEnv("SERVICE_NAME", "push-service"),
			Port:        getEnv("PORT", "8080"),
			Environment: getEnv("ENVIRONMENT", "development"),
			LogLevel:    getEnv("LOG_LEVEL", "info"),
		},
		RabbitMQ: RabbitMQConfig{
			URL:       getEnv("RABBITMQ_URL", "amqp://guest:guest@rabbitmq:5672/"),
			Queue:     getEnv("RABBITMQ_QUEUE", "push_notifications"),
			Exchange:  getEnv("RABBITMQ_EXCHANGE", "notifications"),
			Prefetch:  getEnvAsInt("RABBITMQ_PREFETCH", 10),
			Reconnect: getEnvAsBool("RABBITMQ_RECONNECT", true),
		},
		FCM: FCMConfig{
			ServerKey:  getEnv("FCM_SERVER_KEY", ""),
			Enabled:    getEnvAsBool("FCM_ENABLED", true),
			MaxRetries: getEnvAsInt("FCM_MAX_RETRIES", 3),
			BatchSize:  getEnvAsInt("FCM_BATCH_SIZE", 500),
		},
		APNS: APNSConfig{
			KeyID:      getEnv("APNS_KEY_ID", ""),
			TeamID:     getEnv("APNS_TEAM_ID", ""),
			BundleID:   getEnv("APNS_BUNDLE_ID", ""),
			KeyPath:    getEnv("APNS_KEY_PATH", ""),
			Production: getEnvAsBool("APNS_PRODUCTION", false),
			Enabled:    getEnvAsBool("APNS_ENABLED", false),
		},
		Redis: RedisConfig{
			URL:      getEnv("REDIS_URL", "redis:6379"),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       getEnvAsInt("REDIS_DB", 0),
		},
	}

	// Validate required configuration
	if cfg.RabbitMQ.URL == "" {
		return nil, fmt.Errorf("RABBITMQ_URL is required")
	}

	if cfg.FCM.Enabled && cfg.FCM.ServerKey == "" {
		return nil, fmt.Errorf("FCM_SERVER_KEY is required when FCM is enabled")
	}

	return cfg, nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvAsInt(key string, defaultValue int) int {
	valueStr := getEnv(key, "")
	if value, err := strconv.Atoi(valueStr); err == nil {
		return value
	}
	return defaultValue
}

func getEnvAsBool(key string, defaultValue bool) bool {
	valueStr := getEnv(key, "")
	if value, err := strconv.ParseBool(valueStr); err == nil {
		return value
	}
	return defaultValue
}

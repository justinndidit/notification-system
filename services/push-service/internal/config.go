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
	// FCM v1 API Configuration
	ProjectID          string
	ServiceAccountJSON string // JSON content of service account file
	ServiceAccountPath string // Path to service account file
	Enabled            bool
	MaxRetries         int
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
			ProjectID:          getEnv("FCM_PROJECT_ID", ""),
			ServiceAccountJSON: getEnv("FCM_SERVICE_ACCOUNT_JSON", ""),
			ServiceAccountPath: getEnv("FCM_SERVICE_ACCOUNT_PATH", ""),
			Enabled:            getEnvAsBool("FCM_ENABLED", true),
			MaxRetries:         getEnvAsInt("FCM_MAX_RETRIES", 3),
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

	// Load service account JSON from file if path is provided
	if cfg.FCM.Enabled {
		if cfg.FCM.ServiceAccountJSON == "" && cfg.FCM.ServiceAccountPath != "" {
			content, err := os.ReadFile(cfg.FCM.ServiceAccountPath)
			if err != nil {
				return nil, fmt.Errorf("failed to read FCM service account file: %w", err)
			}
			cfg.FCM.ServiceAccountJSON = string(content)
		}

		if cfg.FCM.ServiceAccountJSON == "" {
			return nil, fmt.Errorf("FCM_SERVICE_ACCOUNT_JSON or FCM_SERVICE_ACCOUNT_PATH is required when FCM is enabled")
		}

		if cfg.FCM.ProjectID == "" {
			return nil, fmt.Errorf("FCM_PROJECT_ID is required when FCM is enabled")
		}
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

package config

import (
	"fmt"
	"time"

	_ "github.com/joho/godotenv/autoload"
	"github.com/knadh/koanf/parsers/dotenv"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

type DatabaseConfig struct {
	Host            string `koanf:"host" validate:"required"`
	Port            int    `koanf:"port" validate:"required"`
	User            string `koanf:"user" validate:"required"`
	Password        string `koanf:"password"`
	Name            string `koanf:"name" validate:"required"`
	SSLMode         string `koanf:"ssl_mode" validate:"required"`
	MaxOpenConns    int    `koanf:"max_open_conns" validate:"required"`
	MaxIdleConns    int    `koanf:"max_idle_conns" validate:"required"`
	ConnMaxLifetime int    `koanf:"conn_max_lifetime" validate:"required"`  // seconds
	ConnMaxIdleTime int    `koanf:"conn_max_idle_time" validate:"required"` // seconds
}

type RedisConfig struct {
	Address  string `koanf:"address" validate:"required"`
	Password string `koanf:"password"`
	DB       int    `koanf:"db"`
}

type RabbitMQConfig struct {
	URL           string `koanf:"url" validate:"required"`
	ExchangeName  string `koanf:"exchange_name" validate:"required"`
	ExchangeType  string `koanf:"exchange_type" validate:"required"`
	QueueName     string `koanf:"queue_name" validate:"required"`
	RoutingKey    string `koanf:"routing_key" validate:"required"`
	PrefetchCount int    `koanf:"prefetch_count"`
}

type ServerConfig struct {
	Port               string        `koanf:"port" validate:"required"`
	ReadTimeout        time.Duration `koanf:"read_timeout" validate:"required"`  // seconds
	WriteTimeout       time.Duration `koanf:"write_timeout" validate:"required"` // seconds
	IdleTimeout        time.Duration `koanf:"idle_timeout" validate:"required"`  // seconds
	CORSAllowedOrigins []string      `koanf:"cors_allowed_origins" validate:"required"`
}

// type ConsulConfig struct {
// 	Address       string `koanf:"address" validate:"required"`
// 	ServiceName   string `koanf:"service_name" validate:"required"`
// 	ServicePort   int    `koanf:"service_port" validate:"required"`
// 	ServiceHost   string `koanf:"service_host" validate:"required"`
// 	HealthCheck   string `koanf:"health_check" validate:"required"`
// 	CheckInterval string `koanf:"check_interval"`
// 	Enabled       bool   `koanf:"enabled"`
// }

type ExternalServices struct {
	UserServiceName     string `koanf:"user_service_name" validate:"required"`
	TemplateServiceName string `koanf:"template_service_name" validate:"required"`
}

type Config struct {
	Database DatabaseConfig `koanf:"database"`
	Redis    RedisConfig    `koanf:"redis"`
	RabbitMQ RabbitMQConfig `koanf:"rabbitmq"`
	Server   ServerConfig   `koanf:"server"`
	// Consul           ConsulConfig     `koanf:"consul"`
	ExternalServices ExternalServices `koanf:"external_services"`
}

func LoadConfig() (*Config, error) {
	k := koanf.New(".")

	// Load from .env file first
	if err := k.Load(file.Provider(".env"), dotenv.Parser()); err != nil {
		return nil, fmt.Errorf("error loading .env file: %w", err)
	}

	// Load from environment variables (overrides .env)
	// Use "__" as delimiter for nested configs
	if err := k.Load(env.Provider("", ".", func(s string) string {
		return s
	}), nil); err != nil {
		return nil, fmt.Errorf("error loading environment variables: %w", err)
	}

	var config Config

	// Unmarshal database config
	if err := k.Unmarshal("database", &config.Database); err != nil {
		return nil, fmt.Errorf("error unmarshaling database config: %w", err)
	}

	// Unmarshal redis config
	if err := k.Unmarshal("redis", &config.Redis); err != nil {
		return nil, fmt.Errorf("error unmarshaling redis config: %w", err)
	}

	// Unmarshal rabbitmq config
	if err := k.Unmarshal("rabbitmq", &config.RabbitMQ); err != nil {
		return nil, fmt.Errorf("error unmarshaling rabbitmq config: %w", err)
	}

	// Unmarshal server config
	if err := k.Unmarshal("server", &config.Server); err != nil {
		return nil, fmt.Errorf("error unmarshaling server config: %w", err)
	}

	// Convert seconds to time.Duration for server timeouts
	config.Server.ReadTimeout = time.Duration(k.Int("server.read_timeout")) * time.Second
	config.Server.WriteTimeout = time.Duration(k.Int("server.write_timeout")) * time.Second
	config.Server.IdleTimeout = time.Duration(k.Int("server.idle_timeout")) * time.Second

	// Unmarshal consul config
	// if err := k.Unmarshal("consul", &config.Consul); err != nil {
	// 	return nil, fmt.Errorf("error unmarshaling consul config: %w", err)
	// }

	// Unmarshal external services config
	if err := k.Unmarshal("external_services", &config.ExternalServices); err != nil {
		return nil, fmt.Errorf("error unmarshaling external services config: %w", err)
	}

	return &config, nil
}

// GetDatabaseDSN returns the PostgreSQL connection string
func (c *DatabaseConfig) GetDatabaseDSN() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		c.Host, c.Port, c.User, c.Password, c.Name, c.SSLMode,
	)
}

// GetRedisAddress returns the Redis connection address
func (c *RedisConfig) GetRedisAddress() string {
	return c.Address
}

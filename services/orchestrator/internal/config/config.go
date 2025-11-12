package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
	_ "github.com/joho/godotenv/autoload"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/v2"
	"github.com/rs/zerolog"
	"github.com/streadway/amqp"
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

// type ExternalServices struct {
// 	UserServiceAddress     string `koanf:"user_service_address" validate:"required"`
// 	TemplateServiceAddress string `koanf:"template_service_address" validate:"required"`
// }

type Config struct {
	Database DatabaseConfig `koanf:"database"`
	Redis    RedisConfig    `koanf:"redis"`
	RabbitMQ RabbitMQConfig `koanf:"rabbitmq"`
	Server   ServerConfig   `koanf:"server"`
	// Consul           ConsulConfig     `koanf:"consul"`
	// External ExternalServices `koanf:"external_services"`
}

func LoadConfig() (*Config, error) {
	logger := zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr}).With().Timestamp().Logger()
	k := koanf.New(".")

	// This is the *only* loader you need.
	// 1. Prefix: "ORCHESTRATOR_"
	// 2. Delimiter: "."
	// 3. Callback: Converts keys to lowercase
	err := k.Load(env.Provider("ORCHESTRATOR_", ".", func(key string) string {
		// "ORCHESTRATOR_SERVER.PORT" -> "server.port"
		return strings.ToLower(strings.TrimPrefix(key, "ORCHESTRATOR_"))
	}), nil)

	if err != nil {
		logger.Fatal().Err(err).Msg("could not load environment variables")
	}

	mainConfig := &Config{}
	if err = k.Unmarshal("", mainConfig); err != nil {
		logger.Fatal().Err(err).Msg("could not unmarshal main config")
	}

	validate := validator.New()
	if err = validate.Struct(mainConfig); err != nil {
		logger.Fatal().Err(err).Msg("config validation failed")
	}

	logger.Info().Msg("config validation passed")
	return mainConfig, nil
}

// func parseMapString(value string) (map[string]string, bool) {
// 	if !strings.HasPrefix(value, "map[") || !strings.HasSuffix(value, "]") {
// 		return nil, false
// 	}

// 	content := strings.TrimPrefix(value, "map[")
// 	content = strings.TrimSuffix(content, "]")

// 	result := make(map[string]string)

// 	if content == "" {
// 		return result, true
// 	}

// 	i := 0
// 	for i < len(content) {
// 		keyStart := i
// 		for i < len(content) && content[i] != ':' {
// 			i++
// 		}
// 		if i >= len(content) {
// 			break
// 		}

// 		key := strings.TrimSpace(content[keyStart:i])
// 		i++

// 		valueStart := i
// 		if i+4 <= len(content) && content[i:i+4] == "map[" {
// 			bracketCount := 0
// 			for i < len(content) {
// 				if i+4 <= len(content) && content[i:i+4] == "map[" {
// 					bracketCount++
// 					i += 4
// 				} else if content[i] == ']' {
// 					bracketCount--
// 					i++
// 					if bracketCount == 0 {
// 						break
// 					}
// 				} else {
// 					i++
// 				}
// 			}
// 		} else {
// 			for i < len(content) && content[i] != ' ' {
// 				i++
// 			}
// 		}

// 		value := strings.TrimSpace(content[valueStart:i])

// 		if nestedMap, isNested := parseMapString(value); isNested {
// 			for nestedKey, nestedValue := range nestedMap {
// 				result[key+"."+nestedKey] = nestedValue
// 			}
// 		} else {
// 			result[key] = value
// 		}

// 		for i < len(content) && content[i] == ' ' {
// 			i++
// 		}
// 	}

// 	return result, true
// }

// func LoadConfig() (*Config, error) {
// 	logger := zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr}).With().Timestamp().Logger()

// 	k := koanf.New(".")

// 	envVars := make(map[string]string)
// 	for _, env := range os.Environ() {
// 		parts := strings.SplitN(env, "=", 2)
// 		if len(parts) == 2 && strings.HasPrefix(parts[0], "ORCHESTRATOR_") {
// 			key := parts[0]
// 			value := parts[1]

// 			configKey := strings.ToLower(strings.TrimPrefix(key, "ORCHESTRATOR_"))

// 			if mapData, isMap := parseMapString(value); isMap {
// 				for mapKey, mapValue := range mapData {
// 					flatKey := configKey + "." + strings.ToLower(mapKey)
// 					envVars[flatKey] = mapValue
// 				}
// 			} else {
// 				envVars[configKey] = value
// 			}
// 		}
// 	}

// 	err := k.Load(env.ProviderWithValue("ORCHESTRATOR_", ".", func(key, value string) (string, any) {
// 		return strings.ToLower(strings.TrimPrefix(key, "ORCHESTRATOR_")), value
// 	}), nil)
// 	if err != nil {
// 		logger.Fatal().Err(err).Msg("could not load initial env variables")
// 	}

// 	for key, value := range envVars {
// 		k.Set(key, value)
// 	}

// 	mainConfig := &Config{}

// 	err = k.Unmarshal("", mainConfig)
// 	if err != nil {
// 		logger.Fatal().Err(err).Msg("could not unmarshal main config")
// 	}

// 	validate := validator.New()

// 	err = validate.Struct(mainConfig)
// 	if err != nil {
// 		logger.Fatal().Err(err).Msg("config validation failed")
// 	}
// 	logger.Info().Msg("config validation passed")

// 	return mainConfig, nil
// }

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

func SetupRabbitMQ(cfg RabbitMQConfig) (*amqp.Channel, error) {
	// Connect to RabbitMQ
	conn, err := amqp.Dial(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to RabbitMQ: %w", err)
	}

	// Create channel
	ch, err := conn.Channel()
	if err != nil {
		return nil, fmt.Errorf("failed to open channel: %w", err)
	}

	// Declare exchange (topic exchange for routing)
	err = ch.ExchangeDeclare(
		cfg.ExchangeName, // name: "notifications"
		"topic",          // type: topic allows wildcard routing
		true,             // durable
		false,            // auto-deleted
		false,            // internal
		false,            // no-wait
		nil,              // arguments
	)
	if err != nil {
		return nil, fmt.Errorf("failed to declare exchange: %w", err)
	}

	// Define queues for each channel
	queues := []struct {
		name       string
		routingKey string
	}{
		{
			name:       "email_queue",
			routingKey: "notification.email",
		},
		{
			name:       "push_queue",
			routingKey: "notification.push",
		},
		{
			name:       "sms_queue",
			routingKey: "notification.sms",
		},
		// Add orchestrator queue if needed (for confirmations/failures)
		{
			name:       cfg.QueueName,  // orchestrator_queue
			routingKey: cfg.RoutingKey, // notification.* (all notifications)
		},
	}

	// Declare and bind each queue
	for _, q := range queues {
		queue, err := ch.QueueDeclare(
			q.name, // queue name
			true,   // durable
			false,  // delete when unused
			false,  // exclusive
			false,  // no-wait
			nil,    // arguments
		)
		if err != nil {
			return nil, fmt.Errorf("failed to declare queue %s: %w", q.name, err)
		}

		// Bind queue to exchange with routing key
		err = ch.QueueBind(
			queue.Name,       // queue name
			q.routingKey,     // routing key
			cfg.ExchangeName, // exchange
			false,
			nil,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to bind queue %s: %w", q.name, err)
		}
	}

	// Set QoS (prefetch count)
	if cfg.PrefetchCount > 0 {
		err = ch.Qos(
			cfg.PrefetchCount, // prefetch count
			0,                 // prefetch size
			false,             // global
		)
		if err != nil {
			return nil, fmt.Errorf("failed to set QoS: %w", err)
		}
	}

	return ch, nil
}

// ```

// ## How the Routing Works:

// ### 1. **Exchange Type: Topic**
// ```
// Exchange: "notifications" (topic)
// ```

// ### 2. **Routing Keys**
// ```
// notification.email  → email_queue   (email-service consumes)
// notification.push   → push_queue    (push-service consumes)
// notification.sms    → sms_queue     (sms-service consumes)
// notification.*      → orchestrator_queue (optional: for monitoring)
// ```

// ### 3. **Message Flow**
// ```
// ┌─────────────┐
// │ Orchestrator│
// └──────┬──────┘
//        │ Publishes with routing key
//        ▼
// ┌──────────────────┐
// │  Exchange        │
// │  "notifications" │ (Topic)
// └────┬────┬────┬───┘
//      │    │    │
//      │    │    └─────────────────┐
//      │    │                      │
//      ▼    ▼                      ▼
// ┌─────┐ ┌──────┐          ┌──────────┐
// │email│ │push  │          │sms       │
// │queue│ │queue │          │queue     │
// └──┬──┘ └───┬──┘          └────┬─────┘
//    │        │                  │
//    ▼        ▼                  ▼
// ┌──────┐ ┌──────┐          ┌──────┐
// │Email │ │Push  │          │SMS   │
// │Worker│ │Worker│          │Worker│
// └──────┘ └──────┘          └──────┘

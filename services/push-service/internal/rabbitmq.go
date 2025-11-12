package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"push-service/internal/config"
	"push-service/internal/logger"
	"push-service/internal/models"

	amqp "github.com/rabbitmq/amqp091-go"
)

type Consumer struct {
	conn           *amqp.Connection
	channel        *amqp.Channel
	config         *config.RabbitMQConfig
	logger         *logger.Logger
	messageHandler MessageHandler
	done           chan bool
}

type MessageHandler func(ctx context.Context, msg *models.PushNotificationMessage) error

func NewConsumer(cfg *config.RabbitMQConfig, log *logger.Logger, handler MessageHandler) (*Consumer, error) {
	c := &Consumer{
		config:         cfg,
		logger:         log,
		messageHandler: handler,
		done:           make(chan bool),
	}

	if err := c.connect(); err != nil {
		return nil, err
	}

	return c, nil
}

func (c *Consumer) connect() error {
	var err error

	c.logger.Info("Connecting to RabbitMQ", "url", c.config.URL)

	c.conn, err = amqp.Dial(c.config.URL)
	if err != nil {
		return fmt.Errorf("failed to connect to RabbitMQ: %w", err)
	}

	c.channel, err = c.conn.Channel()
	if err != nil {
		return fmt.Errorf("failed to open channel: %w", err)
	}

	// Set QoS (prefetch count)
	err = c.channel.Qos(
		c.config.Prefetch, // prefetch count
		0,                 // prefetch size
		false,             // global
	)
	if err != nil {
		return fmt.Errorf("failed to set QoS: %w", err)
	}

	// Declare queue (idempotent)
	_, err = c.channel.QueueDeclare(
		c.config.Queue, // name
		true,           // durable
		false,          // delete when unused
		false,          // exclusive
		false,          // no-wait
		nil,            // arguments
	)
	if err != nil {
		return fmt.Errorf("failed to declare queue: %w", err)
	}

	c.logger.Info("Successfully connected to RabbitMQ", "queue", c.config.Queue)
	return nil
}

func (c *Consumer) Start(ctx context.Context) error {
	msgs, err := c.channel.Consume(
		c.config.Queue, // queue
		"",             // consumer tag
		false,          // auto-ack
		false,          // exclusive
		false,          // no-local
		false,          // no-wait
		nil,            // args
	)
	if err != nil {
		return fmt.Errorf("failed to register consumer: %w", err)
	}

	c.logger.Info("Push service consumer started, waiting for messages...")

	go func() {
		for {
			select {
			case <-ctx.Done():
				c.logger.Info("Consumer context cancelled, stopping...")
				c.done <- true
				return
			case msg, ok := <-msgs:
				if !ok {
					c.logger.Warn("Message channel closed, attempting to reconnect...")
					if c.config.Reconnect {
						if err := c.reconnect(ctx); err != nil {
							c.logger.Error("Failed to reconnect", "error", err)
							c.done <- true
							return
						}
						// Restart consumption after reconnection
						if err := c.Start(ctx); err != nil {
							c.logger.Error("Failed to restart consumer", "error", err)
							c.done <- true
							return
						}
					}
					return
				}

				c.handleMessage(ctx, msg)
			}
		}
	}()

	<-c.done
	return nil
}

func (c *Consumer) handleMessage(ctx context.Context, delivery amqp.Delivery) {
	startTime := time.Now()

	c.logger.Info("Received push notification message",
		"message_id", delivery.MessageId,
		"delivery_tag", delivery.DeliveryTag)

	var message models.PushNotificationMessage
	if err := json.Unmarshal(delivery.Body, &message); err != nil {
		c.logger.Error("Failed to unmarshal message",
			"error", err,
			"body", string(delivery.Body))

		// Reject and don't requeue malformed messages
		delivery.Nack(false, false)
		return
	}

	// Process the message
	if err := c.messageHandler(ctx, &message); err != nil {
		c.logger.Error("Failed to process message",
			"notification_id", message.NotificationID,
			"error", err,
			"duration", time.Since(startTime))

		// Requeue the message for retry
		delivery.Nack(false, true)
		return
	}

	// Acknowledge successful processing
	if err := delivery.Ack(false); err != nil {
		c.logger.Error("Failed to acknowledge message", "error", err)
	}

	c.logger.Info("Message processed successfully",
		"notification_id", message.NotificationID,
		"duration", time.Since(startTime))
}

func (c *Consumer) reconnect(ctx context.Context) error {
	c.logger.Info("Attempting to reconnect to RabbitMQ...")

	maxRetries := 5
	for i := 0; i < maxRetries; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err := c.connect(); err != nil {
			c.logger.Warn("Reconnection attempt failed",
				"attempt", i+1,
				"max_retries", maxRetries,
				"error", err)

			time.Sleep(time.Duration(i+1) * 2 * time.Second)
			continue
		}

		c.logger.Info("Successfully reconnected to RabbitMQ")
		return nil
	}

	return fmt.Errorf("failed to reconnect after %d attempts", maxRetries)
}

func (c *Consumer) Close() error {
	c.logger.Info("Closing RabbitMQ connection...")

	if c.channel != nil {
		if err := c.channel.Close(); err != nil {
			c.logger.Error("Error closing channel", "error", err)
		}
	}

	if c.conn != nil {
		if err := c.conn.Close(); err != nil {
			c.logger.Error("Error closing connection", "error", err)
			return err
		}
	}

	return nil
}

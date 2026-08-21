package internal

import (
	"context"
	"encoding/json"
	"fmt"

	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/rs/zerolog"
)

// maxDeliveryAttempts bounds how many times a failing message is retried before
// it is dead-lettered.
const maxDeliveryAttempts = 5

type Consumer struct {
	conn           *amqp.Connection
	channel        *amqp.Channel
	config         *RabbitMQConfig
	logger         *zerolog.Logger
	messageHandler MessageHandler
	done           chan bool
}

type MessageHandler func(ctx context.Context, msg *PushNotificationMessage) error

func NewConsumer(cfg *RabbitMQConfig, log *zerolog.Logger, handler MessageHandler) (*Consumer, error) {
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

	c.logger.Info().Str("url", c.config.URL).Msg("Connecting to RabbitMQ")

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

	// Declare queue (idempotent).
	//
	// The arguments must match the orchestrator's declaration exactly — RabbitMQ
	// rejects a redeclare with different arguments — so the dead-letter exchange
	// is repeated here.
	_, err = c.channel.QueueDeclare(
		c.config.Queue, // name
		true,           // durable
		false,          // delete when unused
		false,          // exclusive
		false,          // no-wait
		amqp.Table{
			"x-dead-letter-exchange": c.config.Exchange + ".dlx",
		},
	)
	if err != nil {
		return fmt.Errorf("failed to declare queue: %w", err)
	}

	c.logger.Info().Str("queue", c.config.Queue).Msg("Successfully connected to RabbitMQ")
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

	c.logger.Info().Msg("Push service consumer started, waiting for messages...")

	go func() {
		for {
			select {
			case <-ctx.Done():
				c.logger.Info().Msg("Consumer context cancelled, stopping...")
				c.done <- true
				return
			case msg, ok := <-msgs:
				if !ok {
					c.logger.Warn().Msg("Message channel closed, attempting to reconnect...")
					if c.config.Reconnect {
						if err := c.reconnect(ctx); err != nil {
							c.logger.Error().Err(err).Msg("Failed to reconnect")
							c.done <- true
							return
						}
						// Restart consumption after reconnection
						if err := c.Start(ctx); err != nil {
							c.logger.Error().Err(err).Msg("Failed to restart consumer")
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
	// startTime := time.Now()

	c.logger.Info().
		Str("message_id", delivery.MessageId).
		// Str("delivery_tag", string(delivery.DeliveryTag)).
		Msg("Received push notification message")

	var message PushNotificationMessage
	if err := json.Unmarshal(delivery.Body, &message); err != nil {
		c.logger.Error().Err(err).Str("body", string(delivery.Body)).Msg("Failed to unmarshal message")

		// Reject and don't requeue malformed messages
		delivery.Nack(false, false)
		return
	}

	// Process the message
	if err := c.messageHandler(ctx, &message); err != nil {
		attempts := deliveryAttempts(delivery)

		if attempts >= maxDeliveryAttempts {
			// Give up and let the broker dead-letter it. Requeueing forever, as
			// this used to, pins a consumer on a message that will never succeed.
			c.logger.Error().Err(err).
				Str("notification_id", message.NotificationID).
				Int("attempts", attempts).
				Msg("Exhausted delivery attempts, dead-lettering message")

			delivery.Nack(false, false)
			return
		}

		c.logger.Warn().Err(err).
			Str("notification_id", message.NotificationID).
			Int("attempt", attempts).
			Int("max_attempts", maxDeliveryAttempts).
			Msg("Failed to process message, requeueing")

		delivery.Nack(false, true)
		return
	}

	// Acknowledge successful processing
	if err := delivery.Ack(false); err != nil {
		c.logger.Error().Err(err).Msg("Failed to acknowledge message")
	}

	c.logger.Info().Str("notification_id", message.NotificationID).
		Msg("Message processed successfully")
}

// deliveryAttempts reports how many times the broker has delivered this message.
//
// RabbitMQ increments x-delivery-count only for quorum queues; on classic queues
// Redelivered is the single bit of history available, so a redelivered message
// is treated as its second attempt.
func deliveryAttempts(delivery amqp.Delivery) int {
	if raw, ok := delivery.Headers["x-delivery-count"]; ok {
		switch v := raw.(type) {
		case int32:
			return int(v) + 1
		case int64:
			return int(v) + 1
		case int:
			return v + 1
		}
	}

	if delivery.Redelivered {
		return 2
	}

	return 1
}

func (c *Consumer) reconnect(ctx context.Context) error {
	c.logger.Info().Msg("Attempting to reconnect to RabbitMQ...")

	maxRetries := 5
	for i := 0; i < maxRetries; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err := c.connect(); err != nil {
			c.logger.Warn().Err(err).
				// Str("err", string(i+1)).
				// Str("max_retries", string(maxRetries)).
				Msg("Reconnection attempt failed")

			time.Sleep(time.Duration(i+1) * 2 * time.Second)
			continue
		}

		c.logger.Info().Msg("Successfully reconnected to RabbitMQ")
		return nil
	}

	return fmt.Errorf("failed to reconnect after %d attempts", maxRetries)
}

func (c *Consumer) Close() error {
	c.logger.Info().Msg("Closing RabbitMQ connection...")

	if c.channel != nil {
		if err := c.channel.Close(); err != nil {
			c.logger.Error().Err(err).Msg("Error closing channel")
		}
	}

	if c.conn != nil {
		if err := c.conn.Close(); err != nil {
			c.logger.Error().Err(err).Msg("Error closing connection")
			return err
		}
	}

	return nil
}

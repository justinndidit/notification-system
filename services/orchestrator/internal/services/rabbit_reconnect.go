package services

import (
	"fmt"

	"github.com/justinndidit/notificationSystem/orchestrator/internal/config"
	"github.com/streadway/amqp"
)

// watchChannel marks the publisher unhealthy as soon as the broker closes the
// channel, so the next publish rebuilds rather than failing repeatedly against
// a dead connection.
func (o *Orchestrator) watchChannel(closed chan *amqp.Error) {
	go func() {
		reason, ok := <-closed
		if !ok {
			return
		}

		o.publishMu.Lock()
		o.channelSick = true
		o.publishMu.Unlock()

		o.logger.Warn().
			Str("reason", fmt.Sprintf("%v", reason)).
			Msg("RabbitMQ channel closed; will reconnect on next publish")
	}()
}

// ensureChannel rebuilds the connection when the current one is unusable.
//
// The caller must hold publishMu. Without this the orchestrator kept a dead
// channel for the life of the process: the outbox correctly held every message
// through a broker restart, but could never drain, so recovery required
// restarting the service by hand.
func (o *Orchestrator) ensureChannel() error {
	if !o.channelSick && o.rabbitChannel != nil {
		return nil
	}

	o.logger.Info().Msg("Rebuilding RabbitMQ connection")

	// Drop the old connection. Closing the connection closes its channels; a
	// failure here means it was already gone, which is the case we are in.
	if o.rabbitConn != nil {
		_ = o.rabbitConn.Close()
	}

	conn, ch, err := config.SetupRabbitMQ(o.rabbitCfg)
	if err != nil {
		return fmt.Errorf("failed to reconnect to RabbitMQ: %w", err)
	}

	o.rabbitConn = conn
	o.rabbitChannel = ch
	o.confirms = ch.NotifyPublish(make(chan amqp.Confirmation, 1))
	o.returns = ch.NotifyReturn(make(chan amqp.Return, 8))
	o.channelClose = ch.NotifyClose(make(chan *amqp.Error, 1))
	o.channelSick = false

	o.watchChannel(o.channelClose)
	o.watchReturns(o.returns)

	o.logger.Info().Msg("RabbitMQ connection re-established")

	return nil
}

// watchReturns logs messages the broker could not route. They are still acked,
// so this is the only place an unroutable message surfaces.
func (o *Orchestrator) watchReturns(returns chan amqp.Return) {
	go func() {
		for ret := range returns {
			o.logger.Error().
				Str("message_id", ret.MessageId).
				Str("correlation_id", ret.CorrelationId).
				Str("routing_key", ret.RoutingKey).
				Str("reply_text", ret.ReplyText).
				Msg("Message was unroutable and returned by the broker — no queue is bound for this routing key")
		}
	}()
}

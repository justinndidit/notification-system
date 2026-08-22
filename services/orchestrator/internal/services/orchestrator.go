package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justinndidit/notificationSystem/orchestrator/internal/config"
	"github.com/justinndidit/notificationSystem/orchestrator/internal/dtos"
	"github.com/justinndidit/notificationSystem/orchestrator/internal/metrics"
	"github.com/justinndidit/notificationSystem/orchestrator/internal/models"
	"github.com/justinndidit/notificationSystem/orchestrator/internal/repositories"
	"github.com/rs/zerolog"
	"github.com/streadway/amqp"
)

// publishConfirmTimeout bounds how long a publish waits for the broker to
// acknowledge it before being treated as failed.
const publishConfirmTimeout = 10 * time.Second

type Orchestrator struct {
	logger         *zerolog.Logger
	templateClient *TemplateClient
	userClient     *UserClient
	redisClient    *redis.Client
	rabbitChannel  *amqp.Channel
	exchangeName   string
	notifRepo      *repositories.NotificationRepository
	eventRepo      *repositories.EventRepository
	outboxRepo     *repositories.OutboxRepository
	idempotency    *IdempotencyStore

	// Publishes are serialised so each confirmation can be matched to the
	// publish that produced it.
	publishMu sync.Mutex
	confirms  chan amqp.Confirmation
	returns   chan amqp.Return

	// Everything needed to rebuild the connection after a broker restart.
	rabbitConn   *amqp.Connection
	rabbitCfg    config.RabbitMQConfig
	channelSick  bool
	channelClose chan *amqp.Error
}

func NewOrchestrator(
	logger *zerolog.Logger,
	templateClient *TemplateClient,
	userClient *UserClient,
	redisClient *redis.Client,
	rabbitConn *amqp.Connection,
	rabbitChannel *amqp.Channel,
	rabbitCfg config.RabbitMQConfig,
	dbPool *pgxpool.Pool,
) *Orchestrator {
	o := &Orchestrator{
		logger:         logger,
		templateClient: templateClient,
		userClient:     userClient,
		redisClient:    redisClient,
		rabbitChannel:  rabbitChannel,
		rabbitConn:     rabbitConn,
		rabbitCfg:      rabbitCfg,
		exchangeName:   rabbitCfg.ExchangeName,
		notifRepo:      repositories.NewNotificationRepository(dbPool, logger),
		eventRepo:      repositories.NewEventRepository(dbPool, logger),
		outboxRepo:     repositories.NewOutboxRepository(dbPool, logger),
		idempotency:    NewIdempotencyStore(redisClient, logger),
		confirms:       rabbitChannel.NotifyPublish(make(chan amqp.Confirmation, 1)),
		returns:        rabbitChannel.NotifyReturn(make(chan amqp.Return, 8)),
		channelClose:   rabbitChannel.NotifyClose(make(chan *amqp.Error, 1)),
	}

	o.watchChannel(o.channelClose)

	o.watchReturns(o.returns)

	return o
}

// Idempotency exposes the claim store to handlers.
func (o *Orchestrator) Idempotency() *IdempotencyStore {
	return o.idempotency
}

func (o *Orchestrator) EnrichAndPublish(ctx context.Context, req dtos.NotificationRequest, correlationID uuid.UUID, idempotencyKey string) {
	correlationStr := correlationID.String()

	// The claim is released unless this function reaches the end successfully,
	// so a request that fails part-way can be retried with the same key.
	// A panic here would otherwise take the process down: this runs in its own
	// goroutine, detached from the request.
	channelLabel := string(req.NotificationType)
	metrics.NotificationsReceived.WithLabelValues(channelLabel).Inc()
	enrichmentStart := time.Now()

	succeeded := false
	notifIDForClaim := ""
	defer func() {
		if r := recover(); r != nil {
			o.logger.Error().
				Interface("panic", r).
				Str("correlation_id", correlationStr).
				Msg("Recovered from panic during enrichment")
			o.storeNotificationStatus(ctx, correlationStr, dtos.StatusFailed, "internal error during enrichment")
		}

		metrics.EnrichmentDuration.WithLabelValues(channelLabel).Observe(time.Since(enrichmentStart).Seconds())

		if succeeded {
			o.idempotency.Complete(ctx, idempotencyKey, correlationStr, notifIDForClaim)
			return
		}
		o.idempotency.Release(ctx, idempotencyKey)
	}()

	o.logger.Info().
		Str("correlation_id", correlationStr).
		Msg("Starting enrichment process")

	// Generate notification ID
	notifID := uuid.New()
	notifIDForClaim = notifID.String()

	// The request DTO validates these as UUIDs, so a parse failure here means
	// the handler was bypassed. Fail loudly rather than persisting a placeholder.
	userUUID, err := uuid.Parse(req.UserID)
	if err != nil {
		o.logger.Error().Err(err).Str("user_id", req.UserID).Msg("Invalid user_id")
		o.storeNotificationStatus(ctx, correlationStr, "failed", "invalid user_id")
		return
	}

	templateUUID, err := uuid.Parse(req.TemplateCode)
	if err != nil {
		o.logger.Error().Err(err).Str("template_code", req.TemplateCode).Msg("Invalid template_code")
		o.storeNotificationStatus(ctx, correlationStr, "failed", "invalid template_code")
		return
	}

	// Create initial notification record in database

	notification := &models.Notification{
		ID:             notifID,
		UserID:         userUUID,
		TemplateID:     templateUUID,
		CorrelationID:  correlationID,
		IdempotencyKey: &idempotencyKey,
		Channel:        req.NotificationType,
		Status:         dtos.StatusPending,
		Priority:       dtos.NotificationPriorityToString(dtos.NotificationPriority(req.Priority)),
		Variables:      req.Variables,
		Metadata:       models.JSONMap(req.MetaData),
		RetryCount:     0,
		MaxRetries:     3,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	// Save notification to database
	if err := o.notifRepo.CreateNotification(ctx, notification); err != nil {
		o.logger.Error().Err(err).Msg("Failed to create notification record")
		o.storeNotificationStatus(ctx, correlationStr, "failed", err.Error())
		return
	}

	// Record creation event
	o.eventRepo.CreateEventSimple(ctx, notifID, correlationID, dtos.EventCreated, models.JSONMap{
		"channel":  string(notification.Channel),
		"priority": notification.Priority,
	})

	// Update status to enriching
	o.notifRepo.UpdateStatus(ctx, notifID, notification.CreatedAt, dtos.StatusEnriching)

	// Fetch user preferences and template concurrently
	type fetchResult struct {
		user     dtos.HTTPResponse
		template dtos.HTTPResponse
		userErr  error
		tempErr  error
	}

	resultChan := make(chan fetchResult, 1)

	// In EnrichAndPublish...
	go func() {
		var result fetchResult
		var wg sync.WaitGroup
		wg.Add(2)

		userChan := make(chan dtos.HTTPResponse, 1)
		templateChan := make(chan dtos.HTTPResponse, 1)

		go o.userClient.FetchDeliveryProfile(ctx, req.UserID, &wg, userChan)

		go o.templateClient.FetchTemplateById(ctx, req.TemplateCode, &wg, templateChan)

		wg.Wait()
		close(userChan) // Close channels after WaitGroup
		close(templateChan)

		// Simple, blocking reads are now safe because wg.Wait() is done
		result.user = <-userChan
		result.template = <-templateChan

		// Check for empty struct (if a client failed to send)
		if (result.user == dtos.HTTPResponse{}) {
			result.userErr = fmt.Errorf("no user response received")
		}
		if (result.template == dtos.HTTPResponse{}) {
			result.tempErr = fmt.Errorf("no template response received")
		}

		resultChan <- result
	}()

	// Wait for results
	result := <-resultChan

	// Check for failures
	if result.userErr != nil || !result.user.Success {
		errMsg := result.user.Error
		if result.userErr != nil {
			errMsg = result.userErr.Error()
		}

		o.logger.Error().
			Str("correlation_id", correlationStr).
			Str("error", errMsg).
			Msg("Failed to fetch user preferences")

		o.notifRepo.UpdateFailure(ctx, notifID, notification.CreatedAt, "USER_FETCH_ERROR", errMsg)
		o.eventRepo.CreateEventSimple(ctx, notifID, correlationID, dtos.EventFailed, models.JSONMap{
			"error": errMsg,
			"stage": "user_fetch",
		})
		o.storeNotificationStatus(ctx, correlationStr, "failed", errMsg)
		return
	}

	if result.tempErr != nil || !result.template.Success {
		errMsg := result.template.Error
		if result.tempErr != nil {
			errMsg = result.tempErr.Error()
		}

		o.logger.Error().
			Str("correlation_id", correlationStr).
			Str("error", errMsg).
			Msg("Failed to fetch template")

		o.notifRepo.UpdateFailure(ctx, notifID, notification.CreatedAt, "TEMPLATE_FETCH_ERROR", errMsg)
		o.eventRepo.CreateEventSimple(ctx, notifID, correlationID, dtos.EventFailed, models.JSONMap{
			"error": errMsg,
			"stage": "template_fetch",
		})
		o.storeNotificationStatus(ctx, correlationStr, "failed", errMsg)
		return
	}

	// Parse responses
	var profile dtos.UserDeliveryProfile
	var template dtos.TemplateData

	userDataBytes, _ := json.Marshal(result.user.Data)
	if err := json.Unmarshal(userDataBytes, &profile); err != nil {
		o.logger.Error().Err(err).Msg("Failed to parse user preferences")
		o.notifRepo.UpdateFailure(ctx, notifID, notification.CreatedAt, "PARSE_ERROR", "Invalid user delivery profile format")
		o.eventRepo.CreateEventSimple(ctx, notifID, correlationID, dtos.EventFailed, models.JSONMap{
			"error": err.Error(),
			"stage": "profile_parse",
		})
		o.storeNotificationStatus(ctx, correlationStr, "failed", "Invalid user delivery profile format")
		return
	}

	templateDataBytes, _ := json.Marshal(result.template.Data)
	if err := json.Unmarshal(templateDataBytes, &template); err != nil {
		o.logger.Error().Err(err).Msg("Failed to parse template")
		o.notifRepo.UpdateFailure(ctx, notifID, notification.CreatedAt, "PARSE_ERROR", "Invalid template format")
		o.eventRepo.CreateEventSimple(ctx, notifID, correlationID, dtos.EventFailed, models.JSONMap{
			"error": err.Error(),
			"stage": "template_parse",
		})
		o.storeNotificationStatus(ctx, correlationStr, "failed", "Invalid template format")
		return
	}

	// Honour the user's consent for this channel. An opt-out is a normal
	// outcome, not a failure — record it as cancelled and stop.
	if !o.channelAllowed(req.NotificationType, profile) {
		o.logger.Info().
			Str("correlation_id", correlationStr).
			Str("channel", string(req.NotificationType)).
			Msg("User has opted out of this channel")

		o.notifRepo.UpdateStatus(ctx, notifID, notification.CreatedAt, dtos.StatusCancelled)
		o.eventRepo.CreateEventSimple(ctx, notifID, correlationID, dtos.EventCancelled, models.JSONMap{
			"reason":  "user_opted_out",
			"channel": string(req.NotificationType),
		})
		metrics.EnrichmentTotal.WithLabelValues(channelLabel, metrics.ResultCancelled, "consent").Inc()
		metrics.StatusTransitions.WithLabelValues(dtos.StatusCancelled).Inc()
		o.storeNotificationStatus(ctx, correlationStr, dtos.StatusCancelled, "user opted out of this channel")

		// An opt-out is a settled outcome, not a failure. Keep the claim so a
		// resubmission with the same key is recognised rather than reprocessed.
		succeeded = true
		return
	}

	// Resolve the destination for this channel so the worker doesn't have to.
	recipient, err := o.resolveRecipient(req.NotificationType, profile)
	if err != nil {
		o.logger.Error().Err(err).Str("correlation_id", correlationStr).Msg("Cannot resolve recipient")
		o.notifRepo.UpdateFailure(ctx, notifID, notification.CreatedAt, "NO_RECIPIENT", err.Error())
		o.eventRepo.CreateEventSimple(ctx, notifID, correlationID, dtos.EventFailed, models.JSONMap{
			"error": err.Error(),
			"stage": "recipient_resolution",
		})
		o.storeNotificationStatus(ctx, correlationStr, "failed", err.Error())
		return
	}

	// Render the template here so every worker receives finished content and
	// no channel has to carry a template engine of its own.
	rendered, err := o.renderForChannel(ctx, req, profile, correlationStr)
	if err != nil {
		o.logger.Error().Err(err).Str("correlation_id", correlationStr).Msg("Failed to render template")
		o.notifRepo.UpdateFailure(ctx, notifID, notification.CreatedAt, "RENDER_ERROR", err.Error())
		o.eventRepo.CreateEventSimple(ctx, notifID, correlationID, dtos.EventFailed, models.JSONMap{
			"error": err.Error(),
			"stage": "render",
		})
		o.storeNotificationStatus(ctx, correlationStr, "failed", err.Error())
		return
	}

	userPrefs := dtos.UserPreferenceData{
		UserID:      profile.UserID,
		EmailOption: profile.EmailOptIn,
		PushOption:  profile.PushOptIn,
		DailyLimit:  profile.DailyLimit,
		Language:    profile.Language,
	}

	// Build enriched notification for queue
	enrichedNotification := dtos.EnrichedNotification{
		NotificationID:  notifID.String(),
		CorrelationID:   correlationStr,
		IdempotencyKey:  idempotencyKey,
		UserID:          req.UserID,
		TemplateCode:    req.TemplateCode,
		Channel:         string(req.NotificationType),
		Priority:        notification.Priority,
		Recipient:       recipient,
		Tokens:          profile.DeviceTokens,
		Subject:         rendered.Subject,
		Title:           rendered.Title,
		Body:            renderedBody(rendered),
		UserPreferences: userPrefs,
		Template:        template,
		Variables:       req.Variables,
		Metadata:        req.MetaData,
		CreatedAt:       time.Now(),
	}

	// Commit the enrichment result and the intent to publish together. The
	// publisher drains the outbox separately, so a crash from here on loses
	// nothing: the row is durable and will be picked up.
	//
	// The message is also stored verbatim, so a retry republishes exactly what
	// was sent rather than re-deriving it from data that may have changed since.
	payload, err := toJSONMap(enrichedNotification)
	if err != nil {
		o.logger.Error().Err(err).Msg("Failed to encode enriched payload")
		o.failNotification(ctx, notifID, correlationID, notification.CreatedAt, correlationStr, "ENCODE_ERROR", "encode", channelLabel, err)
		return
	}

	if err := o.commitToOutbox(ctx, notification, payload, recipient, enrichedNotification.Channel); err != nil {
		o.logger.Error().
			Err(err).
			Str("correlation_id", correlationStr).
			Msg("Failed to enqueue notification for publication")

		o.failNotification(ctx, notifID, correlationID, notification.CreatedAt, correlationStr, "OUTBOX_ERROR", "outbox_enqueue", channelLabel, err)
		return
	}

	o.eventRepo.CreateEventSimple(ctx, notifID, correlationID, dtos.EventEnriched, nil)
	o.eventRepo.CreateEventSimple(ctx, notifID, correlationID, dtos.EventQueued, nil)
	o.storeNotificationStatus(ctx, correlationStr, dtos.StatusQueued, "")

	metrics.EnrichmentTotal.WithLabelValues(channelLabel, metrics.ResultSuccess, "").Inc()
	metrics.StatusTransitions.WithLabelValues(dtos.StatusQueued).Inc()

	succeeded = true

	o.logger.Info().
		Str("correlation_id", correlationStr).
		Str("notification_id", notifID.String()).
		Msg("Notification enriched and published successfully")
}

// renderForChannel asks the template service to compile the template and
// returns the entry matching this notification's channel.
func (o *Orchestrator) renderForChannel(
	ctx context.Context,
	req dtos.NotificationRequest,
	profile dtos.UserDeliveryProfile,
	correlationStr string,
) (dtos.RenderedContent, error) {
	renderContext := map[string]any{
		"user": map[string]any{
			"id":    profile.UserID,
			"name":  profile.Name,
			"email": profile.Email,
		},
		"name": req.Variables.Name,
		"link": req.Variables.Link,
	}
	for k, v := range req.Variables.Meta {
		renderContext[k] = v
	}

	resp, err := o.templateClient.RenderTemplate(ctx, req.TemplateCode, renderContext)
	if err != nil {
		return dtos.RenderedContent{}, err
	}
	if !resp.Success {
		return dtos.RenderedContent{}, fmt.Errorf("template service rejected render: %s", resp.Error)
	}

	var entries []dtos.RenderedContent
	raw, _ := json.Marshal(resp.Data)
	if err := json.Unmarshal(raw, &entries); err != nil {
		return dtos.RenderedContent{}, fmt.Errorf("invalid render response: %w", err)
	}

	want := strings.ToUpper(string(req.NotificationType))
	for _, entry := range entries {
		if strings.ToUpper(entry.Channel) == want {
			return entry, nil
		}
	}

	o.logger.Warn().
		Str("correlation_id", correlationStr).
		Str("channel", string(req.NotificationType)).
		Msg("Template does not declare this channel")

	return dtos.RenderedContent{}, fmt.Errorf("template %s declares no %s content", req.TemplateCode, req.NotificationType)
}

// renderedBody picks the body field appropriate to the channel that produced it.
func renderedBody(r dtos.RenderedContent) string {
	if r.HTML != "" {
		return r.HTML
	}
	return r.Body
}

// commitToOutbox writes the notification's queued state and its outbox entry in
// one transaction.
func (o *Orchestrator) commitToOutbox(
	ctx context.Context,
	notification *models.Notification,
	payload models.JSONMap,
	recipient string,
	channel string,
) error {
	tx, err := o.outboxRepo.Pool().Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := o.notifRepo.MarkQueuedTx(ctx, tx, notification.ID, notification.CreatedAt, payload, recipient); err != nil {
		return err
	}

	routingKey := fmt.Sprintf("notification.%s", channel)
	if err := o.outboxRepo.EnqueueTx(ctx, tx, notification.ID, notification.CorrelationID, routingKey, payload); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// failNotification records a terminal enrichment failure in one place.
func (o *Orchestrator) failNotification(
	ctx context.Context,
	notifID, correlationID uuid.UUID,
	createdAt time.Time,
	correlationStr, code, stage, channel string,
	cause error,
) {
	o.notifRepo.UpdateFailure(ctx, notifID, createdAt, code, cause.Error())
	o.eventRepo.CreateEventSimple(ctx, notifID, correlationID, dtos.EventFailed, models.JSONMap{
		"error": cause.Error(),
		"stage": stage,
	})
	o.storeNotificationStatus(ctx, correlationStr, dtos.StatusFailed, cause.Error())

	metrics.EnrichmentTotal.WithLabelValues(channel, metrics.ResultFailure, stage).Inc()
	metrics.StatusTransitions.WithLabelValues(dtos.StatusFailed).Inc()
}

// toJSONMap round-trips a value through JSON so it can be stored in a JSONB column.
func toJSONMap(v any) (models.JSONMap, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}

	var out models.JSONMap
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}

	return out, nil
}

// channelAllowed reports whether the user has consented to this channel.
func (o *Orchestrator) channelAllowed(channel dtos.NotificationType, profile dtos.UserDeliveryProfile) bool {
	switch channel {
	case dtos.Email:
		return profile.EmailOptIn
	case dtos.Push:
		return profile.PushOptIn
	default:
		return false
	}
}

// resolveRecipient turns a user profile into the concrete destination for a channel.
func (o *Orchestrator) resolveRecipient(channel dtos.NotificationType, profile dtos.UserDeliveryProfile) (string, error) {
	switch channel {
	case dtos.Email:
		if profile.Email == "" {
			return "", fmt.Errorf("user %s has no email address", profile.UserID)
		}
		return profile.Email, nil
	case dtos.Push:
		if len(profile.DeviceTokens) == 0 {
			return "", fmt.Errorf("user %s has no registered device tokens", profile.UserID)
		}
		// Recipient is informational for push; the tokens themselves carry delivery.
		return profile.DeviceTokens[0].Token, nil
	default:
		return "", fmt.Errorf("unsupported channel %q", channel)
	}
}

// publishToQueue publishes enriched notification to RabbitMQ with channel-specific routing
func (o *Orchestrator) publishToQueue(_ context.Context, notification dtos.EnrichedNotification) error {
	body, err := json.Marshal(notification)
	if err != nil {
		return fmt.Errorf("failed to marshal notification: %w", err)
	}

	// Routing key determines which queue receives the message
	// Format: notification.{channel}
	// Examples: notification.email → email-service queue
	//           notification.push → push-service queue
	routingKey := fmt.Sprintf("notification.%s", notification.Channel)

	o.logger.Info().
		Str("routing_key", routingKey).
		Str("channel", notification.Channel).
		Str("notification_id", notification.NotificationID).
		Msg("Publishing notification to queue")

	publishStart := time.Now()

	// Serialised so the confirmation read below belongs to this publish.
	o.publishMu.Lock()
	defer o.publishMu.Unlock()

	if err := o.ensureChannel(); err != nil {
		return err
	}

	err = o.rabbitChannel.Publish(
		o.exchangeName, // exchange name (e.g., "notifications")
		routingKey,     // routing key (e.g., "notification.email")
		true,           // mandatory: return instead of discarding unroutable messages
		false,          // immediate
		amqp.Publishing{
			ContentType:   "application/json",
			Body:          body,
			DeliveryMode:  amqp.Persistent, // Survive broker restart
			MessageId:     notification.NotificationID,
			CorrelationId: notification.CorrelationID,
			Timestamp:     time.Now(),
			Headers: amqp.Table{
				"channel":  notification.Channel,
				"priority": notification.Priority,
			},
		},
	)

	if err != nil {
		o.channelSick = true
		return fmt.Errorf("failed to publish to queue: %w", err)
	}

	// Wait for the broker to confirm it took responsibility for the message.
	// Without this the orchestrator marks a notification "queued" that RabbitMQ
	// may never have accepted.
	select {
	case confirmation, ok := <-o.confirms:
		if !ok {
			return fmt.Errorf("publisher confirm channel closed before acknowledgement")
		}
		if !confirmation.Ack {
			return fmt.Errorf("broker rejected the message (nack, delivery tag %d)", confirmation.DeliveryTag)
		}
	case <-time.After(publishConfirmTimeout):
		return fmt.Errorf("timed out waiting %s for publisher confirmation", publishConfirmTimeout)
	}

	metrics.PublishDuration.Observe(time.Since(publishStart).Seconds())
	metrics.PublishTotal.WithLabelValues(notification.Channel, metrics.ResultSuccess).Inc()

	o.logger.Info().
		Str("routing_key", routingKey).
		Str("notification_id", notification.NotificationID).
		Msg("Successfully published notification")

	return nil
}

// storeNotificationStatus stores status in Redis for quick lookup
func (o *Orchestrator) storeNotificationStatus(ctx context.Context, correlationID, status, errorMsg string) {
	key := fmt.Sprintf("notification:status:%s", correlationID)
	statusData := map[string]interface{}{
		"status":     status,
		"error":      errorMsg,
		"updated_at": time.Now().Unix(),
	}

	data, err := json.Marshal(statusData)
	if err != nil {
		o.logger.Error().Err(err).Msg("Failed to marshal status data")
		return
	}

	err = o.redisClient.Set(ctx, key, data, 24*time.Hour).Err()
	if err != nil {
		o.logger.Error().Err(err).Msg("Failed to store status in Redis")
	}
}

func (o *Orchestrator) Shutdown(ctx context.Context) error {
	o.logger.Info().Msg("Shutting down orchestrator")

	// Close RabbitMQ channel
	if o.rabbitChannel != nil {
		o.rabbitChannel.Close()
	}

	// Close Redis
	if o.redisClient != nil {
		o.redisClient.Close()
	}

	return nil
}

package services

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justinndidit/notificationSystem/orchestrator/internal/dtos"
	"github.com/justinndidit/notificationSystem/orchestrator/internal/models"
	"github.com/justinndidit/notificationSystem/orchestrator/internal/repositories"
	"github.com/rs/zerolog"
	"github.com/streadway/amqp"
)

type Orchestrator struct {
	logger         *zerolog.Logger
	templateClient *TemplateClient
	userClient     *UserClient
	redisClient    *redis.Client
	rabbitChannel  *amqp.Channel
	exchangeName   string
	notifRepo      *repositories.NotificationRepository
	eventRepo      *repositories.EventRepository
}

func NewOrchestrator(
	logger *zerolog.Logger,
	templateClient *TemplateClient,
	userClient *UserClient,
	redisClient *redis.Client,
	rabbitChannel *amqp.Channel,
	exchangeName string,
	dbPool *pgxpool.Pool,
) *Orchestrator {
	return &Orchestrator{
		logger:         logger,
		templateClient: templateClient,
		userClient:     userClient,
		redisClient:    redisClient,
		rabbitChannel:  rabbitChannel,
		exchangeName:   exchangeName,
		notifRepo:      repositories.NewNotificationRepository(dbPool, logger),
		eventRepo:      repositories.NewEventRepository(dbPool, logger),
	}
}

func (o *Orchestrator) EnrichAndPublish(ctx context.Context, req dtos.NotificationRequest, correlationID, idempotencyKey string) {
	o.logger.Info().
		Str("correlation_id", correlationID).
		Msg("Starting enrichment process")

	// Generate notification ID
	notifID := uuid.New()

	// Create initial notification record in database
	//TODO:convert strings to dto

	notification := &models.Notification{
		ID:             notifID,
		UserID:         uuid.New(), //req.UserID,
		TemplateID:     uuid.New(), //req.TemplateCode,
		CorrelationID:  uuid.New(), //correlationID,
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
		o.storeNotificationStatus(ctx, correlationID, "failed", err.Error())
		return
	}

	// Record creation event
	correlationUUID, _ := uuid.Parse(correlationID)
	o.eventRepo.CreateEventSimple(ctx, notifID, correlationUUID, dtos.EventCreated, models.JSONMap{
		"channel":  string(notification.Channel),
		"priority": notification.Priority,
	})

	// Update status to enriching
	o.notifRepo.UpdateStatus(ctx, notifID, dtos.StatusEnriching)

	// Fetch user preferences and template concurrently
	// IMPROVED: Better pattern for concurrent fetches
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

		go o.userClient.FetchUserPreference(ctx, req.UserID, &wg, userChan) // NO defer wg.Done()

		go o.templateClient.FetchTemplateById(ctx, req.TemplateCode, &wg, templateChan) // NO defer wg.Done()

		// wg.Wait()

		// // Read results from channels
		// select {
		// case result.user = <-userChan:
		// default:
		// 	result.userErr = fmt.Errorf("no user response received")
		// }

		// select {
		// case result.template = <-templateChan:
		// default:
		// 	result.tempErr = fmt.Errorf("no template response received")
		// }

		// resultChan <- result
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
			Str("correlation_id", correlationID).
			Str("error", errMsg).
			Msg("Failed to fetch user preferences")

		o.notifRepo.UpdateFailure(ctx, notifID, "USER_FETCH_ERROR", errMsg)
		o.eventRepo.CreateEventSimple(ctx, notifID, correlationUUID, dtos.EventFailed, models.JSONMap{
			"error": errMsg,
			"stage": "user_fetch",
		})
		o.storeNotificationStatus(ctx, correlationID, "failed", errMsg)
		return
	}

	if result.tempErr != nil || !result.template.Success {
		errMsg := result.template.Error
		if result.tempErr != nil {
			errMsg = result.tempErr.Error()
		}

		o.logger.Error().
			Str("correlation_id", correlationID).
			Str("error", errMsg).
			Msg("Failed to fetch template")

		o.notifRepo.UpdateFailure(ctx, notifID, "TEMPLATE_FETCH_ERROR", errMsg)
		o.eventRepo.CreateEventSimple(ctx, notifID, correlationUUID, dtos.EventFailed, models.JSONMap{
			"error": errMsg,
			"stage": "template_fetch",
		})
		o.storeNotificationStatus(ctx, correlationID, "failed", errMsg)
		return
	}

	// Parse responses
	var userPrefs dtos.UserPreferenceData
	var template dtos.TemplateData

	userDataBytes, _ := json.Marshal(result.user.Data)
	if err := json.Unmarshal(userDataBytes, &userPrefs); err != nil {
		o.logger.Error().Err(err).Msg("Failed to parse user preferences")
		o.notifRepo.UpdateFailure(ctx, notifID, "PARSE_ERROR", "Invalid user data format")
		o.eventRepo.CreateEventSimple(ctx, notifID, correlationUUID, dtos.EventFailed, models.JSONMap{
			"error": err.Error(),
			"stage": "user_parse",
		})
		o.storeNotificationStatus(ctx, correlationID, "failed", "Invalid user data format")
		return
	}

	templateDataBytes, _ := json.Marshal(result.template.Data)
	if err := json.Unmarshal(templateDataBytes, &template); err != nil {
		o.logger.Error().Err(err).Msg("Failed to parse template")
		o.notifRepo.UpdateFailure(ctx, notifID, "PARSE_ERROR", "Invalid template format")
		o.eventRepo.CreateEventSimple(ctx, notifID, correlationUUID, dtos.EventFailed, models.JSONMap{
			"error": err.Error(),
			"stage": "template_parse",
		})
		o.storeNotificationStatus(ctx, correlationID, "failed", "Invalid template format")
		return
	}

	// Store enriched payload in database
	enrichedPayload := models.JSONMap{
		"user_preferences": userPrefs,
		"template":         template,
		"variables":        req.Variables,
	}

	if err := o.notifRepo.UpdateEnrichedPayload(ctx, notifID, enrichedPayload); err != nil {
		o.logger.Error().Err(err).Msg("Failed to update enriched payload")
		return
	}

	// Record enriched event
	o.eventRepo.CreateEventSimple(ctx, notifID, correlationUUID, dtos.EventEnriched, nil)

	// Build enriched notification for queue
	enrichedNotification := dtos.EnrichedNotification{
		NotificationID:  notifID.String(),
		CorrelationID:   correlationID,
		IdempotencyKey:  idempotencyKey,
		UserID:          req.UserID,
		TemplateCode:    req.TemplateCode,
		Channel:         string(req.NotificationType),
		Priority:        notification.Priority,
		UserPreferences: userPrefs,
		Template:        template,
		Variables:       req.Variables,
		Metadata:        req.MetaData,
		CreatedAt:       time.Now(),
	}

	// Publish to RabbitMQ
	if err := o.publishToQueue(ctx, enrichedNotification); err != nil {
		o.logger.Error().
			Err(err).
			Str("correlation_id", correlationID).
			Msg("Failed to publish to queue")

		o.notifRepo.UpdateFailure(ctx, notifID, "QUEUE_ERROR", err.Error())
		o.eventRepo.CreateEventSimple(ctx, notifID, correlationUUID, dtos.EventFailed, models.JSONMap{
			"error": err.Error(),
			"stage": "queue_publish",
		})
		o.storeNotificationStatus(ctx, correlationID, "failed", err.Error())
		return
	}

	// Update status to queued
	o.notifRepo.UpdateStatus(ctx, notifID, dtos.StatusQueued)

	// Record queued event
	o.eventRepo.CreateEventSimple(ctx, notifID, correlationUUID, dtos.EventQueued, nil)

	// Store success status in Redis
	o.storeNotificationStatus(ctx, correlationID, "queued", "")

	o.logger.Info().
		Str("correlation_id", correlationID).
		Str("notification_id", notifID.String()).
		Msg("Notification enriched and published successfully")
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

	err = o.rabbitChannel.Publish(
		o.exchangeName, // exchange name (e.g., "notifications")
		routingKey,     // routing key (e.g., "notification.email")
		false,          // mandatory
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
		return fmt.Errorf("failed to publish to queue: %w", err)
	}

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

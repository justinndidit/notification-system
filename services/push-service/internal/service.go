package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"push-service/internal/config"
	"push-service/internal/logger"
	"push-service/internal/models"
)

type PushService struct {
	fcmClient FCMClient
	storage   RedisStorage
	logger    *logger.Logger
	config    *config.Config
}

func NewPushService(cfg *config.Config, log *logger.Logger) (*PushService, error) {
	var fcmClient FCMClient
	if cfg.FCM.Enabled {
		fcmClient = NewFCMClient(&cfg.FCM, log)
	}

	redisStorage, err := storage.NewRedisStorage(&cfg.Redis, log)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize redis storage: %w", err)
	}

	return &PushService{
		fcmClient: fcmClient,
		storage:   redisStorage,
		logger:    log,
		config:    cfg,
	}, nil
}

func (s *PushService) ProcessNotification(ctx context.Context, msg *models.PushNotificationMessage) error {
	s.logger.Info("Processing push notification",
		"notification_id", msg.NotificationID,
		"user_id", msg.UserID,
		"total_tokens", len(msg.Tokens))

	if len(msg.Tokens) == 0 {
		s.logger.Warn("No tokens provided for notification",
			"notification_id", msg.NotificationID)
		return nil
	}

	// Separate tokens by platform
	androidTokens := make([]string, 0)
	iosTokens := make([]string, 0)

	for _, deviceToken := range msg.Tokens {
		switch deviceToken.Platform {
		case models.PlatformAndroid:
			androidTokens = append(androidTokens, deviceToken.Token)
		case models.PlatformIOS:
			iosTokens = append(iosTokens, deviceToken.Token)
		default:
			s.logger.Warn("Unknown platform",
				"platform", deviceToken.Platform,
				"token", deviceToken.Token)
		}
	}

	allResults := make([]models.PushResult, 0)

	// Send to Android devices via FCM
	if len(androidTokens) > 0 && s.fcmClient != nil && s.config.FCM.Enabled {
		s.logger.Info("Sending to Android devices",
			"notification_id", msg.NotificationID,
			"count", len(androidTokens))

		results, err := s.fcmClient.Send(ctx, msg, androidTokens)
		if err != nil {
			s.logger.Error("Failed to send FCM notifications",
				"notification_id", msg.NotificationID,
				"error", err)
		} else {
			allResults = append(allResults, results...)
		}
	}

	// TODO: Send to iOS devices via APNS
	if len(iosTokens) > 0 {
		s.logger.Info("iOS push notifications not yet implemented",
			"notification_id", msg.NotificationID,
			"count", len(iosTokens))

		// Create placeholder results for iOS
		for _, token := range iosTokens {
			allResults = append(allResults, models.PushResult{
				NotificationID: msg.NotificationID,
				Token:          token,
				Platform:       models.PlatformIOS,
				Success:        false,
				Error:          "iOS push not implemented",
				SentAt:         time.Now(),
			})
		}
	}

	// Calculate delivery status
	status := s.calculateDeliveryStatus(msg, allResults)

	// Store results in Redis
	if err := s.storeDeliveryStatus(ctx, status); err != nil {
		s.logger.Error("Failed to store delivery status",
			"notification_id", msg.NotificationID,
			"error", err)
	}

	s.logger.Info("Push notification processing completed",
		"notification_id", msg.NotificationID,
		"total_tokens", status.TotalTokens,
		"success", status.SuccessCount,
		"failure", status.FailureCount)

	return nil
}

func (s *PushService) calculateDeliveryStatus(msg *models.PushNotificationMessage, results []models.PushResult) *models.DeliveryStatus {
	successCount := 0
	failureCount := 0

	for _, result := range results {
		if result.Success {
			successCount++
		} else {
			failureCount++
		}
	}

	return &models.DeliveryStatus{
		NotificationID: msg.NotificationID,
		TotalTokens:    len(msg.Tokens),
		SuccessCount:   successCount,
		FailureCount:   failureCount,
		Results:        results,
		CompletedAt:    time.Now(),
	}
}

func (s *PushService) storeDeliveryStatus(ctx context.Context, status *models.DeliveryStatus) error {
	key := fmt.Sprintf("push:delivery:%s", status.NotificationID)

	data, err := json.Marshal(status)
	if err != nil {
		return fmt.Errorf("failed to marshal delivery status: %w", err)
	}

	// Store with 7 days TTL
	ttl := 7 * 24 * time.Hour
	return s.storage.Set(ctx, key, data, ttl)
}

func (s *PushService) GetDeliveryStatus(ctx context.Context, notificationID string) (*models.DeliveryStatus, error) {
	key := fmt.Sprintf("push:delivery:%s", notificationID)

	data, err := s.storage.Get(ctx, key)
	if err != nil {
		return nil, err
	}

	var status models.DeliveryStatus
	if err := json.Unmarshal(data, &status); err != nil {
		return nil, fmt.Errorf("failed to unmarshal delivery status: %w", err)
	}

	return &status, nil
}

func (s *PushService) Close() error {
	if s.storage != nil {
		return s.storage.Close()
	}
	return nil
}

package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/rs/zerolog"
)

type PushService struct {
	fcmClient *FCMClient
	storage   *RedisStorage
	logger    *zerolog.Logger
	config    *Config
}

func NewPushService(cfg *Config, log *zerolog.Logger) (*PushService, error) {
	var fcmClient FCMClient
	if cfg.FCM.Enabled {
		fcmClient = *NewFCMClient(&cfg.FCM, log)
	}

	redisStorage, err := NewRedisStorage(&cfg.Redis, log)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize redis storage: %w", err)
	}

	return &PushService{
		fcmClient: &fcmClient,
		storage:   redisStorage,
		logger:    log,
		config:    cfg,
	}, nil
}

func (s *PushService) ProcessNotification(ctx context.Context, msg *PushNotificationMessage) error {
	s.logger.Info().Str("notification_id", msg.NotificationID).Str("user_id", msg.UserID).Msg("Processing push notification")

	if len(msg.Tokens) == 0 {
		s.logger.Warn().Str("notification_id", msg.NotificationID).Msg("No tokens provided for notification")

		return nil
	}

	// Separate tokens by platform
	androidTokens := make([]string, 0)
	iosTokens := make([]string, 0)

	for _, deviceToken := range msg.Tokens {
		switch deviceToken.Platform {
		case PlatformAndroid:
			androidTokens = append(androidTokens, deviceToken.Token)
		case PlatformIOS:
			iosTokens = append(iosTokens, deviceToken.Token)
		default:
			s.logger.Warn().Msg(fmt.Sprintf("Unknown platform: platform: %s, token: %s", deviceToken.Platform, deviceToken.Token))
		}
	}

	allResults := make([]PushResult, 0)

	// Send to Android devices via FCM
	if len(androidTokens) > 0 && s.fcmClient != nil && s.config.FCM.Enabled {
		s.logger.Info().Str("notification_id", msg.NotificationID).Msg("Sending to Android devices")

		results, err := s.fcmClient.Send(ctx, msg, androidTokens)
		if err != nil {
			s.logger.Error().Err(err).
				Str("notification_id", msg.NotificationID).
				Msg("Failed to send FCM notifications")
		} else {
			allResults = append(allResults, results...)
		}
	}

	// TODO: Send to iOS devices via APNS
	if len(iosTokens) > 0 {
		s.logger.Info().Str("notification_id", msg.NotificationID).Msg("iOS push notifications not yet implemented")

		// Create placeholder results for iOS
		for _, token := range iosTokens {
			allResults = append(allResults, PushResult{
				NotificationID: msg.NotificationID,
				Token:          token,
				Platform:       PlatformIOS,
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
		s.logger.Error().Err(err).
			Str("notification_id", msg.NotificationID).
			Msg("Failed to store delivery status")

	}

	s.logger.Info().
		Str("notification_id", msg.NotificationID).
		// Str("total_tokens", string(status.TotalTokens)).
		// Str("success", string(status.SuccessCount)).
		// Str("failure", string(status.FailureCount)).
		Msg("Push notification processing completed")

	return nil
}

func (s *PushService) calculateDeliveryStatus(msg *PushNotificationMessage, results []PushResult) *DeliveryStatus {
	successCount := 0
	failureCount := 0

	for _, result := range results {
		if result.Success {
			successCount++
		} else {
			failureCount++
		}
	}

	return &DeliveryStatus{
		NotificationID: msg.NotificationID,
		TotalTokens:    len(msg.Tokens),
		SuccessCount:   successCount,
		FailureCount:   failureCount,
		Results:        results,
		CompletedAt:    time.Now(),
	}
}

func (s *PushService) storeDeliveryStatus(ctx context.Context, status *DeliveryStatus) error {
	key := fmt.Sprintf("push:delivery:%s", status.NotificationID)

	data, err := json.Marshal(status)
	if err != nil {
		return fmt.Errorf("failed to marshal delivery status: %w", err)
	}

	// Store with 7 days TTL
	ttl := 7 * 24 * time.Hour
	return s.storage.Set(ctx, key, data, ttl)
}

func (s *PushService) GetDeliveryStatus(ctx context.Context, notificationID string) (*DeliveryStatus, error) {
	key := fmt.Sprintf("push:delivery:%s", notificationID)

	data, err := s.storage.Get(ctx, key)
	if err != nil {
		return nil, err
	}

	var status DeliveryStatus
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

package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/rs/zerolog"
)

const fcmURL = "https://fcm.googleapis.com/fcm/send"

type FCMClient struct {
	serverKey  string
	httpClient *http.Client
	logger     *zerolog.Logger
	config     *FCMConfig
}

type fcmPayload struct {
	To              string                 `json:"to,omitempty"`
	RegistrationIDs []string               `json:"registration_ids,omitempty"`
	Notification    fcmNotification        `json:"notification"`
	Data            map[string]interface{} `json:"data,omitempty"`
	Priority        string                 `json:"priority,omitempty"`
	TimeToLive      int                    `json:"time_to_live,omitempty"`
}

type fcmNotification struct {
	Title       string `json:"title"`
	Body        string `json:"body"`
	Image       string `json:"image,omitempty"`
	Sound       string `json:"sound,omitempty"`
	ClickAction string `json:"click_action,omitempty"`
	Badge       string `json:"badge,omitempty"`
}

type fcmResponse struct {
	MulticastID  int64       `json:"multicast_id"`
	Success      int         `json:"success"`
	Failure      int         `json:"failure"`
	CanonicalIDs int         `json:"canonical_ids"`
	Results      []fcmResult `json:"results"`
}

type fcmResult struct {
	MessageID      string `json:"message_id,omitempty"`
	RegistrationID string `json:"registration_id,omitempty"`
	Error          string `json:"error,omitempty"`
}

func NewFCMClient(cfg *FCMConfig, log *zerolog.Logger) *FCMClient {
	return &FCMClient{
		serverKey: cfg.ServerKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger: log,
		config: cfg,
	}
}

func (f *FCMClient) Send(ctx context.Context, msg *PushNotificationMessage, tokens []string) ([]PushResult, error) {
	if len(tokens) == 0 {
		return nil, nil
	}

	results := make([]PushResult, 0, len(tokens))

	// FCM supports batch sending up to 1000 tokens, but we'll use configurable batch size
	batchSize := f.config.BatchSize
	if batchSize > 1000 {
		batchSize = 1000
	}

	for i := 0; i < len(tokens); i += batchSize {
		end := i + batchSize
		if end > len(tokens) {
			end = len(tokens)
		}

		batchTokens := tokens[i:end]
		batchResults, err := f.sendBatch(ctx, msg, batchTokens)
		if err != nil {
			f.logger.Error().Err(err).Msg("Failed to send FCM batch")

			// Create failed results for this batch
			for _, token := range batchTokens {
				results = append(results, PushResult{
					NotificationID: msg.NotificationID,
					Token:          token,
					Platform:       PlatformAndroid,
					Success:        false,
					Error:          err.Error(),
					SentAt:         time.Now(),
				})
			}
			continue
		}

		results = append(results, batchResults...)
	}

	return results, nil
}

func (f *FCMClient) sendBatch(ctx context.Context, msg *PushNotificationMessage, tokens []string) ([]PushResult, error) {
	payload := fcmPayload{
		RegistrationIDs: tokens,
		Notification: fcmNotification{
			Title:       msg.Title,
			Body:        msg.Body,
			Image:       msg.ImageURL,
			Sound:       msg.Sound,
			ClickAction: msg.ClickAction,
		},
		Data:     msg.Data,
		Priority: f.mapPriority(msg.Priority),
	}

	if msg.TTL > 0 {
		payload.TimeToLive = msg.TTL
	}

	if msg.Badge > 0 {
		payload.Notification.Badge = fmt.Sprintf("%d", msg.Badge)
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal FCM payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", fcmURL, bytes.NewBuffer(payloadBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create FCM request: %w", err)
	}

	req.Header.Set("Authorization", "key="+f.serverKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send FCM request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read FCM response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("FCM returned non-200 status: %d, body: %s", resp.StatusCode, string(body))
	}

	var fcmResp fcmResponse
	if err := json.Unmarshal(body, &fcmResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal FCM response: %w", err)
	}

	// Parse results
	results := make([]PushResult, 0, len(tokens))
	for i, token := range tokens {
		result := PushResult{
			NotificationID: msg.NotificationID,
			Token:          token,
			Platform:       PlatformAndroid,
			SentAt:         time.Now(),
		}

		if i < len(fcmResp.Results) {
			fcmResult := fcmResp.Results[i]
			if fcmResult.Error != "" {
				result.Success = false
				result.Error = fcmResult.Error
			} else {
				result.Success = true
				result.MessageID = fcmResult.MessageID
			}
		} else {
			result.Success = false
			result.Error = "no result returned from FCM"
		}

		results = append(results, result)
	}

	// f.logger.Info().Msg("FCM batch sent").
	// 	"notification_id", msg.NotificationID,
	// 	"tokens_sent", len(tokens),
	// 	"success", fcmResp.Success,
	// 	"failure", fcmResp.Failure)
	f.logger.Info().Str("notification_id", msg.NotificationID).Msg("FCM batch sent")
	return results, nil
}

func (f *FCMClient) mapPriority(priority string) string {
	if priority == PriorityHigh {
		return "high"
	}
	return "normal"
}

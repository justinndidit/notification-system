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
	"golang.org/x/oauth2/google"
)

// FCM v1 API endpoint
const fcmV1URLTemplate = "https://fcm.googleapis.com/v1/projects/%s/messages:send"

type FCMClient struct {
	projectID   string
	httpClient  *http.Client
	logger      *zerolog.Logger
	config      *FCMConfig
	credentials *google.Credentials
}

// FCM v1 API message structure
type fcmV1Message struct {
	Message fcmV1MessagePayload `json:"message"`
}

type fcmV1MessagePayload struct {
	Token        string             `json:"token"`
	Notification *fcmV1Notification `json:"notification,omitempty"`
	Data         map[string]string  `json:"data,omitempty"`
	Android      *fcmV1Android      `json:"android,omitempty"`
	APNS         *fcmV1APNS         `json:"apns,omitempty"`
}

type fcmV1Notification struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	Image string `json:"image,omitempty"`
}

type fcmV1Android struct {
	Priority     string                    `json:"priority,omitempty"`
	Notification *fcmV1AndroidNotification `json:"notification,omitempty"`
	TTL          string                    `json:"ttl,omitempty"`
}

type fcmV1AndroidNotification struct {
	Sound       string `json:"sound,omitempty"`
	ClickAction string `json:"click_action,omitempty"`
	Icon        string `json:"icon,omitempty"`
	Color       string `json:"color,omitempty"`
}

type fcmV1APNS struct {
	Headers map[string]string      `json:"headers,omitempty"`
	Payload map[string]interface{} `json:"payload,omitempty"`
}

type fcmV1Response struct {
	Name  string `json:"name"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error,omitempty"`
}

func NewFCMClient(cfg *FCMConfig, log *zerolog.Logger) (*FCMClient, error) {
	// Load service account credentials
	credentials, err := google.CredentialsFromJSON(
		context.Background(),
		[]byte(cfg.ServiceAccountJSON),
		"https://www.googleapis.com/auth/firebase.messaging",
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load credentials: %w", err)
	}

	return &FCMClient{
		projectID: cfg.ProjectID,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger:      log,
		config:      cfg,
		credentials: credentials,
	}, nil
}

func (f *FCMClient) Send(ctx context.Context, msg *PushNotificationMessage, tokens []string) ([]PushResult, error) {
	if len(tokens) == 0 {
		return nil, nil
	}

	results := make([]PushResult, 0, len(tokens))

	// FCM v1 API doesn't support batch sending
	// We need to send individual requests for each token
	for _, token := range tokens {
		result := f.sendSingleMessage(ctx, msg, token)
		results = append(results, result)
	}

	return results, nil
}

func (f *FCMClient) sendSingleMessage(ctx context.Context, msg *PushNotificationMessage, token string) PushResult {
	result := PushResult{
		NotificationID: msg.NotificationID,
		Token:          token,
		Platform:       PlatformAndroid,
		SentAt:         time.Now(),
	}

	// Get OAuth2 token
	accessToken, err := f.credentials.TokenSource.Token()
	if err != nil {
		f.logger.Error().Err(err).Msg("Failed to get access token")
		result.Success = false
		result.Error = fmt.Sprintf("auth error: %v", err)
		return result
	}

	// Build FCM v1 message
	fcmMessage := f.buildFCMv1Message(msg, token)

	// Marshal to JSON
	payloadBytes, err := json.Marshal(fcmMessage)
	if err != nil {
		f.logger.Error().Err(err).Msg("Failed to marshal FCM message")
		result.Success = false
		result.Error = fmt.Sprintf("marshal error: %v", err)
		return result
	}

	// Create request
	url := fmt.Sprintf(fcmV1URLTemplate, f.projectID)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(payloadBytes))
	if err != nil {
		f.logger.Error().Err(err).Msg("Failed to create request")
		result.Success = false
		result.Error = fmt.Sprintf("request error: %v", err)
		return result
	}

	// Set headers
	req.Header.Set("Authorization", "Bearer "+accessToken.AccessToken)
	req.Header.Set("Content-Type", "application/json")

	// Send request
	resp, err := f.httpClient.Do(req)
	if err != nil {
		f.logger.Error().Err(err).Msg("Failed to send FCM request")
		result.Success = false
		result.Error = fmt.Sprintf("send error: %v", err)
		return result
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		f.logger.Error().Err(err).Msg("Failed to read response")
		result.Success = false
		result.Error = fmt.Sprintf("read error: %v", err)
		return result
	}

	// Parse response
	var fcmResp fcmV1Response
	if err := json.Unmarshal(body, &fcmResp); err != nil {
		f.logger.Error().Err(err).Str("body", string(body)).Msg("Failed to unmarshal response")
		result.Success = false
		result.Error = fmt.Sprintf("unmarshal error: %v", err)
		return result
	}

	// Check for errors
	if fcmResp.Error != nil {
		f.logger.Warn().Str("error_message", fcmResp.Error.Message).Str("token", token).Msg("FCM returned error")
		// 	"token", token,
		// 	"error_code", fcmResp.Error.Code,
		// )
		result.Success = false
		result.Error = fcmResp.Error.Message
		return result
	}

	// Success
	result.Success = true
	result.MessageID = fcmResp.Name
	f.logger.Info().Str("notification_id", msg.NotificationID).Str("message_id", fcmResp.Name).Str("token", token).Msg("FCM message sent successfully")

	return result
}

func (f *FCMClient) buildFCMv1Message(msg *PushNotificationMessage, token string) *fcmV1Message {
	fcmMsg := &fcmV1Message{
		Message: fcmV1MessagePayload{
			Token: token,
			Notification: &fcmV1Notification{
				Title: msg.Title,
				Body:  msg.Body,
				Image: msg.ImageURL,
			},
		},
	}

	// Convert data map[string]interface{} to map[string]string for FCM
	if msg.Data != nil {
		dataStrings := make(map[string]string)
		for k, v := range msg.Data {
			dataStrings[k] = fmt.Sprintf("%v", v)
		}
		fcmMsg.Message.Data = dataStrings
	}

	// Android specific configuration
	android := &fcmV1Android{}

	// Priority
	if msg.Priority == PriorityHigh {
		android.Priority = "high"
	} else {
		android.Priority = "normal"
	}

	// TTL
	if msg.TTL > 0 {
		android.TTL = fmt.Sprintf("%ds", msg.TTL)
	}

	// Android notification
	androidNotif := &fcmV1AndroidNotification{}
	if msg.Sound != "" {
		androidNotif.Sound = msg.Sound
	}
	if msg.ClickAction != "" {
		androidNotif.ClickAction = msg.ClickAction
	}

	if androidNotif.Sound != "" || androidNotif.ClickAction != "" {
		android.Notification = androidNotif
	}

	fcmMsg.Message.Android = android

	return fcmMsg
}

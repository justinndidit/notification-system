package services

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/justinndidit/notificationSystem/orchestrator/internal/dtos"
	"github.com/rs/zerolog"
)

type BaseHTTPClient struct {
	logger     *zerolog.Logger
	httpClient *http.Client
}

func NewBaseHTTPClient(logger *zerolog.Logger) *BaseHTTPClient {
	return &BaseHTTPClient{
		logger: logger,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}
}

// Shared retry logic
// BaseHTTPClient
func (b *BaseHTTPClient) DoWithRetry(ctx context.Context, url string, resultChan chan<- dtos.HTTPResponse, errorMsg string) {
	var body dtos.HTTPResponse // ✅ Decode into HTTPResponse directly

	operation := func() error {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return backoff.Permanent(err)
		}

		resp, err := b.httpClient.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		// Decode the full HTTPResponse
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			return err
		}

		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			return backoff.Permanent(fmt.Errorf("client error: %d", resp.StatusCode))
		}

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("server error: %d", resp.StatusCode)
		}

		return nil
	}

	backOff := backoff.NewExponentialBackOff()
	backOff.MaxElapsedTime = 30 * time.Second

	err := backoff.Retry(operation, backoff.WithContext(backOff, ctx))

	if err != nil {
		b.logger.Error().Err(err).Msg("Request failed after retries")
		resultChan <- dtos.HTTPResponse{
			Success: false,
			Error:   err.Error(),
			Message: errorMsg,
		}
		return
	}

	// Send the decoded response
	resultChan <- body
}

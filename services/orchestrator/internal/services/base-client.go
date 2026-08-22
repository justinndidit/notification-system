package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/justinndidit/notificationSystem/orchestrator/internal/dtos"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

type BaseHTTPClient struct {
	logger     *zerolog.Logger
	httpClient *http.Client
	// Supplies a fresh service token per request. Tokens expire, so this cannot
	// be a static header captured at construction.
	tokenProvider func() (string, error)
}

func NewBaseHTTPClient(logger *zerolog.Logger, tokenProvider func() (string, error)) *BaseHTTPClient {
	return &BaseHTTPClient{
		logger:        logger,
		tokenProvider: tokenProvider,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			// otelhttp propagates trace context on every outbound call and
			// records a client span, so enrichment shows up as children of the
			// request that triggered it rather than as unattributed latency.
			Transport: otelhttp.NewTransport(&http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
			}),
		},
	}
}

// execute runs one HTTP request with exponential backoff, decoding the standard
// envelope every service returns. 4xx is permanent (retrying a bad request only
// wastes capacity); 5xx and transport errors are retried.
func (b *BaseHTTPClient) execute(ctx context.Context, method, url string, payload []byte) (dtos.HTTPResponse, error) {
	var body dtos.HTTPResponse

	operation := func() error {
		var reader io.Reader
		if payload != nil {
			reader = bytes.NewReader(payload)
		}

		req, err := http.NewRequestWithContext(ctx, method, url, reader)
		if err != nil {
			return backoff.Permanent(err)
		}

		if b.tokenProvider != nil {
			token, tokenErr := b.tokenProvider()
			if tokenErr != nil {
				return backoff.Permanent(fmt.Errorf("could not obtain a service token: %w", tokenErr))
			}
			req.Header.Set("Authorization", "Bearer "+token)
		}
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		resp, err := b.httpClient.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			return err
		}

		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			return backoff.Permanent(fmt.Errorf("client error: %d, %v", resp.StatusCode, resp.Status))
		}

		if resp.StatusCode < 200 || resp.StatusCode > 299 {
			return fmt.Errorf("server error: %d", resp.StatusCode)
		}

		return nil
	}

	backOff := backoff.NewExponentialBackOff()
	backOff.MaxElapsedTime = 30 * time.Second

	if err := backoff.Retry(operation, backoff.WithContext(backOff, ctx)); err != nil {
		return dtos.HTTPResponse{}, err
	}

	return body, nil
}

// DoWithRetry performs a GET and reports the result on a channel, so several
// enrichment calls can run concurrently.
func (b *BaseHTTPClient) DoWithRetry(ctx context.Context, url string, resultChan chan<- dtos.HTTPResponse, errorMsg string) {
	body, err := b.execute(ctx, http.MethodGet, url, nil)
	if err != nil {
		b.logger.Error().Err(err).Str("url", url).Msg("Request failed after retries")
		resultChan <- dtos.HTTPResponse{
			Success: false,
			Error:   err.Error(),
			Message: errorMsg,
		}
		return
	}

	resultChan <- body
}

// PostJSON performs a POST and returns the decoded envelope directly.
func (b *BaseHTTPClient) PostJSON(ctx context.Context, url string, payload any) (dtos.HTTPResponse, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return dtos.HTTPResponse{}, fmt.Errorf("failed to encode request payload: %w", err)
	}

	return b.execute(ctx, http.MethodPost, url, encoded)
}

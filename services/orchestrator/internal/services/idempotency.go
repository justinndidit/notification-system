package services

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/rs/zerolog"
)

// Idempotency record states.
const (
	IdempotencyProcessing = "processing"
	IdempotencyCompleted  = "completed"
)

// idempotencyTTL is how long a completed record is remembered. A client
// retrying within this window gets the original outcome rather than a second
// notification.
const idempotencyTTL = 24 * time.Hour

// IdempotencyRecord is what a claimed key holds.
type IdempotencyRecord struct {
	State          string `json:"state"`
	CorrelationID  string `json:"correlation_id"`
	NotificationID string `json:"notification_id,omitempty"`
	ClaimedAt      int64  `json:"claimed_at"`
}

// IdempotencyStore claims request keys so the same submission is not processed
// twice.
//
// The claim is written before work starts and released if that work fails, so a
// failed request can be retried with the same key. Writing the key and never
// releasing it — the previous behaviour — meant a request that failed during
// enrichment was answered "duplicate" for 24 hours and never sent.
type IdempotencyStore struct {
	redis  *redis.Client
	logger *zerolog.Logger
}

func NewIdempotencyStore(rdb *redis.Client, logger *zerolog.Logger) *IdempotencyStore {
	return &IdempotencyStore{redis: rdb, logger: logger}
}

func idempotencyRedisKey(key string) string {
	return fmt.Sprintf("idempotency:%s", key)
}

// Claim atomically reserves a key.
//
// Returns claimed=true when this caller owns the request. When it returns
// false, existing describes the in-flight or completed request that already
// holds the key. SETNX makes this atomic: a plain GET-then-SET let two
// concurrent requests both observe an empty key and both proceed.
func (s *IdempotencyStore) Claim(ctx context.Context, key, correlationID string) (claimed bool, existing *IdempotencyRecord, err error) {
	record := IdempotencyRecord{
		State:         IdempotencyProcessing,
		CorrelationID: correlationID,
		ClaimedAt:     time.Now().Unix(),
	}

	encoded, err := json.Marshal(record)
	if err != nil {
		return false, nil, fmt.Errorf("failed to encode idempotency record: %w", err)
	}

	ok, err := s.redis.SetNX(ctx, idempotencyRedisKey(key), encoded, idempotencyTTL).Result()
	if err != nil {
		return false, nil, fmt.Errorf("failed to claim idempotency key: %w", err)
	}
	if ok {
		return true, nil, nil
	}

	return false, s.Get(ctx, key), nil
}

// Get reads a claimed record, returning nil when the key is absent or unreadable.
func (s *IdempotencyStore) Get(ctx context.Context, key string) *IdempotencyRecord {
	raw, err := s.redis.Get(ctx, idempotencyRedisKey(key)).Bytes()
	if err != nil {
		return nil
	}

	var record IdempotencyRecord
	if err := json.Unmarshal(raw, &record); err != nil {
		s.logger.Warn().Err(err).Str("key", key).Msg("Discarding malformed idempotency record")
		return nil
	}

	return &record
}

// Complete marks a claim as successfully processed, so later retries with the
// same key are recognised as duplicates rather than re-sent.
func (s *IdempotencyStore) Complete(ctx context.Context, key, correlationID, notificationID string) {
	record := IdempotencyRecord{
		State:          IdempotencyCompleted,
		CorrelationID:  correlationID,
		NotificationID: notificationID,
		ClaimedAt:      time.Now().Unix(),
	}

	encoded, err := json.Marshal(record)
	if err != nil {
		s.logger.Error().Err(err).Str("key", key).Msg("Failed to encode idempotency completion")
		return
	}

	if err := s.redis.Set(ctx, idempotencyRedisKey(key), encoded, idempotencyTTL).Err(); err != nil {
		s.logger.Error().Err(err).Str("key", key).Msg("Failed to record idempotency completion")
	}
}

// Release drops a claim so the request can be retried with the same key.
//
// Called when processing fails. Leaving the claim in place would make the
// failure permanent for the length of the TTL.
func (s *IdempotencyStore) Release(ctx context.Context, key string) {
	if err := s.redis.Del(ctx, idempotencyRedisKey(key)).Err(); err != nil {
		s.logger.Error().Err(err).Str("key", key).Msg("Failed to release idempotency claim")
	}
}
